package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

type GoogleDriveUploader struct {
	service     *drive.Service
	folderID    string
	dailyFolders map[string]string // Cache for daily folder IDs
}

// tokenSaver implements oauth2.TokenSource and automatically saves tokens to disk
type tokenSaver struct {
	tokenSource oauth2.TokenSource
	tokFile     string
}

// Token returns a token and saves it to disk if it's been refreshed
func (ts *tokenSaver) Token() (*oauth2.Token, error) {
	token, err := ts.tokenSource.Token()
	if err != nil {
		return nil, err
	}
	
	// Save the token (which may have been refreshed)
	saveToken(ts.tokFile, token)
	return token, nil
}

// NewGoogleDriveUploader creates a new Google Drive uploader
func NewGoogleDriveUploader(credentialsPath, tokenPath, folderID string) (*GoogleDriveUploader, error) {
	ctx := context.Background()

	// Read credentials file
	credentialsData, err := os.ReadFile(credentialsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read credentials file: %w", err)
	}

	config, err := google.ConfigFromJSON(credentialsData, drive.DriveFileScope)
	if err != nil {
		return nil, fmt.Errorf("failed to parse client secret file: %w", err)
	}

	client, err := getClient(config, tokenPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get OAuth client: %w", err)
	}

	service, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("failed to create Drive service: %w", err)
	}

	return &GoogleDriveUploader{
		service:      service,
		folderID:     folderID,
		dailyFolders: make(map[string]string),
	}, nil
}

// getClient retrieves a token, saves it, then returns the generated client
func getClient(config *oauth2.Config, tokFile string) (*http.Client, error) {
	// The file token.json stores the user's access and refresh tokens.
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		tok, err = getTokenFromWeb(config)
		if err != nil {
			return nil, fmt.Errorf("failed to get token from web: %v", err)
		}
		saveToken(tokFile, tok)
	}
	
	// Use ReuseTokenSource to automatically save refreshed tokens
	tokenSource := oauth2.ReuseTokenSource(tok, &tokenSaver{
		tokenSource: config.TokenSource(context.Background(), tok),
		tokFile:     tokFile,
	})
	
	return oauth2.NewClient(context.Background(), tokenSource), nil
}

// getTokenFromWeb requests a token from the web, then returns the retrieved token
func getTokenFromWeb(config *oauth2.Config) (*oauth2.Token, error) {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser then type the authorization code: \n%v\n", authURL)

	// Check if running in interactive terminal
	if !isTerminalInteractive() {
		log.Printf("Non-interactive environment detected. Please run the following command locally to generate token.json:")
		log.Printf("cd recorder && USE_GOOGLE_DRIVE=true go run .")
		log.Printf("Then restart the container.")
		return nil, fmt.Errorf("cannot read authorization code in non-interactive environment")
	}

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		return nil, fmt.Errorf("unable to read authorization code: %v", err)
	}

	tok, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve token from web: %v", err)
	}
	return tok, nil
}

// isTerminalInteractive checks if we're running in an interactive terminal
func isTerminalInteractive() bool {
	// Check if stdin is available and looks like a terminal
	stat, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	// Check if it's a character device (terminal) and not just a pipe/file
	return (stat.Mode() & os.ModeCharDevice) != 0
}

// tokenFromFile retrieves a token from a local file
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

// saveToken saves a token to a file path
func saveToken(path string, token *oauth2.Token) {
	fmt.Printf("Saving credential file to: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

// getDailyFolderID gets or creates a daily folder for the given date
func (gdu *GoogleDriveUploader) getDailyFolderID(date time.Time) (string, error) {
	// Format date as M/D/YY (e.g., 8/23/25)
	dateStr := date.Format("1/2/06")
	
	// Check cache first
	if folderID, exists := gdu.dailyFolders[dateStr]; exists {
		return folderID, nil
	}

	// Search for existing folder
	folderID, err := gdu.findFolder(dateStr, gdu.folderID)
	if err != nil {
		return "", fmt.Errorf("failed to search for folder: %w", err)
	}

	// Create folder if it doesn't exist
	if folderID == "" {
		folderID, err = gdu.createFolder(dateStr, gdu.folderID)
		if err != nil {
			return "", fmt.Errorf("failed to create folder: %w", err)
		}
		log.Printf("Created daily folder: %s", dateStr)
	} else {
		log.Printf("Using existing daily folder: %s", dateStr)
	}

	// Cache the folder ID
	gdu.dailyFolders[dateStr] = folderID
	return folderID, nil
}

// findFolder searches for a folder by name within a parent folder
func (gdu *GoogleDriveUploader) findFolder(folderName, parentID string) (string, error) {
	query := fmt.Sprintf("name='%s' and mimeType='application/vnd.google-apps.folder' and parents in '%s' and trashed=false", folderName, parentID)
	
	fileList, err := gdu.service.Files.List().Q(query).Fields("files(id, name)").Do()
	if err != nil {
		return "", err
	}

	if len(fileList.Files) > 0 {
		return fileList.Files[0].Id, nil
	}

	return "", nil // Folder not found
}

// createFolder creates a new folder in Google Drive
func (gdu *GoogleDriveUploader) createFolder(folderName, parentID string) (string, error) {
	folder := &drive.File{
		Name:     folderName,
		MimeType: "application/vnd.google-apps.folder",
		Parents:  []string{parentID},
	}

	createdFolder, err := gdu.service.Files.Create(folder).Fields("id").Do()
	if err != nil {
		return "", err
	}

	return createdFolder.Id, nil
}

// UploadFile uploads a file to Google Drive in a daily folder
func (gdu *GoogleDriveUploader) UploadFile(filePath string) error {
	// Open the file
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", filePath, err)
	}
	defer file.Close()

	// Get file info
	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}

	// Get the file's modification date to determine which daily folder to use
	fileDate := fileInfo.ModTime()
	
	// Get or create the daily folder
	dailyFolderID, err := gdu.getDailyFolderID(fileDate)
	if err != nil {
		return fmt.Errorf("failed to get daily folder: %w", err)
	}

	// Create the file metadata
	fileName := filepath.Base(filePath)
	driveFile := &drive.File{
		Name:    fileName,
		Parents: []string{dailyFolderID},
	}

	// Upload the file
	_, err = gdu.service.Files.Create(driveFile).Media(file).Do()
	if err != nil {
		return fmt.Errorf("failed to upload file to Google Drive: %w", err)
	}

	dateStr := fileDate.Format("1/2/06")
	log.Printf("Successfully uploaded %s (%.2f MB) to Google Drive folder %s", 
		fileName, float64(fileInfo.Size())/(1024*1024), dateStr)
	return nil
}

// DeleteLocalFile removes the local file after successful upload
func (gdu *GoogleDriveUploader) DeleteLocalFile(filePath string) error {
	err := os.Remove(filePath)
	if err != nil {
		return fmt.Errorf("failed to delete local file %s: %w", filePath, err)
	}
	log.Printf("Deleted local file: %s", filepath.Base(filePath))
	return nil
}
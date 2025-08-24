package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type VideoRecorder struct {
	go2rtcURL        string
	recordingsDir    string
	recordingInterval int
	gdUploader       *GoogleDriveUploader
	useGoogleDrive   bool
}

func NewVideoRecorder(go2rtcURL, recordingsDir string, intervalSeconds int, gdUploader *GoogleDriveUploader, useGoogleDrive bool) *VideoRecorder {
	return &VideoRecorder{
		go2rtcURL:        go2rtcURL,
		recordingsDir:    recordingsDir,
		recordingInterval: intervalSeconds,
		gdUploader:       gdUploader,
		useGoogleDrive:   useGoogleDrive,
	}
}

func (vr *VideoRecorder) Initialize() error {
	// Ensure recordings directory exists (we still need it as temporary storage)
	err := os.MkdirAll(vr.recordingsDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create recordings directory: %w", err)
	}

	storageType := "local"
	if vr.useGoogleDrive {
		storageType = "Google Drive"
	}

	log.Printf("Video recorder initialized - URL: %s, Interval: %ds, Dir: %s, Storage: %s", 
		vr.go2rtcURL, vr.recordingInterval, vr.recordingsDir, storageType)
	return nil
}

func (vr *VideoRecorder) Run(ctx context.Context) error {
	log.Println("Starting video recording from go2rtc...")
	
	ticker := time.NewTicker(time.Duration(vr.recordingInterval) * time.Second)
	defer ticker.Stop()

	// Start first recording immediately
	go vr.recordSegment(ctx)

	for {
		select {
		case <-ctx.Done():
			log.Println("Context cancelled, stopping recorder")
			return nil
		case <-ticker.C:
			// Start new recording segment
			go vr.recordSegment(ctx)
		}
	}
}

func (vr *VideoRecorder) recordSegment(ctx context.Context) {
	// Generate filename with timestamp
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	filename := fmt.Sprintf("recording_%s.mp4", timestamp)
	filepath := filepath.Join(vr.recordingsDir, filename)

	log.Printf("Starting recording: %s", filename)

	// Create context with timeout for this recording segment
	recordCtx, cancel := context.WithTimeout(ctx, time.Duration(vr.recordingInterval+5)*time.Second)
	defer cancel()

	// Use ffmpeg to record from go2rtc stream
	cmd := exec.CommandContext(recordCtx, "ffmpeg",
		"-i", vr.go2rtcURL,
		"-t", fmt.Sprintf("%d", vr.recordingInterval), // Duration in seconds
		"-c", "copy", // Copy streams without re-encoding
		"-f", "mp4",
		"-y", // Overwrite output file if it exists
		filepath,
	)

	// Capture stderr for logging
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		log.Printf("Error recording %s: %v", filename, err)
		return
	}

	// Check if file was created and get its size
	if stat, err := os.Stat(filepath); err == nil {
		log.Printf("Recording completed: %s (%.2f MB)", 
			filename, float64(stat.Size())/(1024*1024))

		// Upload to Google Drive if enabled
		if vr.useGoogleDrive && vr.gdUploader != nil {
			go vr.uploadToGoogleDrive(filepath)
		}
	} else {
		log.Printf("Recording completed but couldn't stat file: %s", filename)
	}
}

// uploadToGoogleDrive uploads a video file to Google Drive and optionally deletes the local copy
func (vr *VideoRecorder) uploadToGoogleDrive(filePath string) {
	filename := filepath.Base(filePath)
	log.Printf("Starting Google Drive upload: %s", filename)

	// Upload to Google Drive
	err := vr.gdUploader.UploadFile(filePath)
	if err != nil {
		log.Printf("Failed to upload %s to Google Drive: %v", filename, err)
		return
	}

	// Delete local file after successful upload
	err = vr.gdUploader.DeleteLocalFile(filePath)
	if err != nil {
		log.Printf("Warning: Failed to delete local file %s after upload: %v", filename, err)
	}
}
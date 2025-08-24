package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
)

func main() {
	go2rtcURL := os.Getenv("GO2RTC_URL")
	if go2rtcURL == "" {
		go2rtcURL = "http://go2rtc:1984/api/stream.mp4?src=camera1"
	}

	recordingInterval := 60 // default 60 seconds
	if intervalStr := os.Getenv("RECORDING_INTERVAL"); intervalStr != "" {
		if interval, err := strconv.Atoi(intervalStr); err == nil {
			recordingInterval = interval
		}
	}

	recordingsDir := os.Getenv("RECORDINGS_DIR")
	if recordingsDir == "" {
		recordingsDir = "/recordings"
	}

	// Google Drive configuration
	useGoogleDrive := os.Getenv("USE_GOOGLE_DRIVE") == "true"
	credentialsPath := os.Getenv("GOOGLE_DRIVE_CREDENTIALS")
	tokenPath := os.Getenv("GOOGLE_DRIVE_TOKEN")
	folderID := os.Getenv("GOOGLE_DRIVE_FOLDER_ID")

	var gdUploader *GoogleDriveUploader
	if useGoogleDrive {
		if credentialsPath == "" {
			credentialsPath = "./secret.json"
		}
		if tokenPath == "" {
			tokenPath = "./token.json"
		}
		if folderID == "" {
			folderID = "1ISvMPz5kMsHG6Jp7Vo82tU9oxFlh6oUI"
		}

		var err error
		gdUploader, err = NewGoogleDriveUploader(credentialsPath, tokenPath, folderID)
		if err != nil {
			log.Printf("Warning: Failed to initialize Google Drive uploader: %v", err)
			log.Println("Falling back to local recording only")
			useGoogleDrive = false
		} else {
			log.Println("Google Drive uploader initialized successfully")
		}
	}

	recorder := NewVideoRecorder(go2rtcURL, recordingsDir, recordingInterval, gdUploader, useGoogleDrive)

	err := recorder.Initialize()
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Received shutdown signal, stopping...")
		cancel()
	}()

	err = recorder.Run(ctx)
	if err != nil {
		log.Fatal(err)
	}
}
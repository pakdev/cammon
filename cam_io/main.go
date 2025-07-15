package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	capture := NewImageCapture("rtsp://10.0.0.201/Streaming/Channels/101", "localhost:9092", "camera-images")

	err := capture.Initialize()
	if err != nil {
		log.Fatal(err)
	}
	defer capture.Stop()

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

	err = capture.Run(ctx)
	if err != nil {
		log.Fatal(err)
	}
}
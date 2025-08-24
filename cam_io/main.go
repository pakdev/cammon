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
		go2rtcURL = "http://go2rtc:1984/api/frame.jpeg?src=camera1"
	}

	kafkaBroker := os.Getenv("KAFKA_BROKER")
	if kafkaBroker == "" {
		kafkaBroker = "localhost:9092"
	}
	
	captureInterval := 3 // default 3 seconds
	if intervalStr := os.Getenv("CAPTURE_INTERVAL"); intervalStr != "" {
		if interval, err := strconv.Atoi(intervalStr); err == nil {
			captureInterval = interval
		}
	}

	capture := NewSimpleCapture(go2rtcURL, kafkaBroker, "camera-images", captureInterval)

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

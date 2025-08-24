package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	kafkaBroker := os.Getenv("KAFKA_BROKER")
	if kafkaBroker == "" {
		kafkaBroker = "localhost:9092"
	}
	
	detector := NewSimpleDetector(kafkaBroker, "camera-images", "detections")

	err := detector.Initialize()
	if err != nil {
		log.Fatal(err)
	}
	defer detector.Stop()

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

	err = detector.Run(ctx)
	if err != nil {
		log.Fatal(err)
	}
}
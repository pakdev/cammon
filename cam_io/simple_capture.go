package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image/jpeg"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

type ImageMessage struct {
	Timestamp int64  `json:"timestamp"`
	ImageData []byte `json:"image_data"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
}

type SimpleCapture struct {
	go2rtcURL      string
	kafkaBroker    string
	kafkaTopic     string
	captureInterval time.Duration
	
	producer *kafka.Producer
	client   *http.Client
}

func NewSimpleCapture(go2rtcURL, kafkaBroker, kafkaTopic string, intervalSeconds int) *SimpleCapture {
	return &SimpleCapture{
		go2rtcURL:       go2rtcURL,
		kafkaBroker:     kafkaBroker,
		kafkaTopic:      kafkaTopic,
		captureInterval: time.Duration(intervalSeconds) * time.Second,
		client:          &http.Client{Timeout: 10 * time.Second},
	}
}

func (sc *SimpleCapture) Initialize() error {
	var err error
	
	sc.producer, err = kafka.NewProducer(&kafka.ConfigMap{"bootstrap.servers": sc.kafkaBroker})
	if err != nil {
		return err
	}
	
	log.Printf("Simple capture initialized - URL: %s, Interval: %v", sc.go2rtcURL, sc.captureInterval)
	return nil
}

func (sc *SimpleCapture) Run(ctx context.Context) error {
	log.Printf("Starting image capture from go2rtc...")
	
	ticker := time.NewTicker(sc.captureInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			log.Println("Context cancelled, stopping capture")
			return nil
		case <-ticker.C:
			err := sc.captureAndSend()
			if err != nil {
				log.Printf("Error capturing image: %v", err)
				// Continue on error, don't exit
			}
		}
	}
}

func (sc *SimpleCapture) captureAndSend() error {
	// Get JPEG image from go2rtc
	resp, err := sc.client.Get(sc.go2rtcURL)
	if err != nil {
		return fmt.Errorf("failed to get image: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP error: %d", resp.StatusCode)
	}
	
	// Read image data
	imageData, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read image data: %w", err)
	}
	
	// Decode to get dimensions
	img, err := jpeg.Decode(bytes.NewReader(imageData))
	if err != nil {
		return fmt.Errorf("failed to decode JPEG: %w", err)
	}
	
	bounds := img.Bounds()
	message := ImageMessage{
		Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
		ImageData: imageData,
		Width:     bounds.Dx(),
		Height:    bounds.Dy(),
	}
	
	messageBytes, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}
	
	err = sc.producer.Produce(&kafka.Message{
		TopicPartition: kafka.TopicPartition{Topic: &sc.kafkaTopic, Partition: kafka.PartitionAny},
		Value:          messageBytes,
	}, nil)
	
	if err != nil {
		return fmt.Errorf("failed to send to Kafka: %w", err)
	}
	
	log.Printf("Sent image to Kafka topic %s (size: %dx%d, %d bytes)", 
		sc.kafkaTopic, bounds.Dx(), bounds.Dy(), len(imageData))
	
	return nil
}

func (sc *SimpleCapture) Stop() {
	if sc.producer != nil {
		sc.producer.Close()
	}
	log.Println("Simple capture stopped")
}
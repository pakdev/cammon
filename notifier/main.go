package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/IBM/sarama"
)

type Detection struct {
	Timestamp   int64    `json:"timestamp"` // Unix timestamp in milliseconds
	Objects     []Object `json:"detections"`
	ImageBase64 string   `json:"image_base64,omitempty"`
}

type Object struct {
	Label      string      `json:"label"`
	Confidence float64     `json:"confidence"`
	Box        BoundingBox `json:"bounding_box"`
}

type BoundingBox struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

type Config struct {
	KafkaBroker  string
	HAWebhookURL string
	HAToken      string
}

func main() {
	config := Config{
		KafkaBroker:  getEnvOrDefault("KAFKA_BROKER", "localhost:9092"),
		HAWebhookURL: getEnvOrDefault("HA_WEBHOOK_URL", ""),
		HAToken:      getEnvOrDefault("HA_TOKEN", ""),
	}

	if config.HAWebhookURL == "" {
		log.Fatal("HA_WEBHOOK_URL environment variable is required")
	}

	consumer, err := setupKafkaConsumer(config.KafkaBroker)
	if err != nil {
		log.Fatalf("Failed to setup Kafka consumer: %v", err)
	}
	defer consumer.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutting down...")
		cancel()
	}()

	log.Println("Starting notification service...")
	log.Printf("Calling consumeDetections with config: %+v", config)
	if err := consumeDetections(ctx, consumer, config); err != nil {
		log.Fatalf("Error consuming detections: %v", err)
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func setupKafkaConsumer(broker string) (sarama.Consumer, error) {
	config := sarama.NewConfig()
	config.Consumer.Return.Errors = true
	config.Consumer.Offsets.Initial = sarama.OffsetOldest

	consumer, err := sarama.NewConsumer([]string{broker}, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create consumer: %w", err)
	}

	return consumer, nil
}

func consumeDetections(ctx context.Context, consumer sarama.Consumer, config Config) error {
	log.Printf("Creating partition consumer for topic 'detections', partition 0...")
	partitionConsumer, err := consumer.ConsumePartition("detections", 0, sarama.OffsetOldest)
	if err != nil {
		return fmt.Errorf("failed to create partition consumer: %w", err)
	}
	defer partitionConsumer.Close()
	log.Printf("Partition consumer created successfully")

	log.Printf("Starting to consume messages from partition 0...")
	for {
		select {
		case <-ctx.Done():
			return nil
		case message := <-partitionConsumer.Messages():
			log.Printf("Received message: offset=%d, key=%s", message.Offset, string(message.Key))
			if err := handleDetection(message.Value, config); err != nil {
				log.Printf("Error handling detection: %v", err)
			}
		case err := <-partitionConsumer.Errors():
			log.Printf("Consumer error: %v", err)
		}
	}
}

func handleDetection(messageValue []byte, config Config) error {
	log.Printf("Received message")
	var detection Detection
	if err := json.Unmarshal(messageValue, &detection); err != nil {
		return fmt.Errorf("failed to unmarshal detection: %w", err)
	}

	timestamp := time.Unix(0, detection.Timestamp*int64(time.Millisecond))
	log.Printf("Received detection with %d objects at %v", len(detection.Objects), timestamp)

	// Send notification to Home Assistant
	if err := sendHANotification(detection, timestamp, config); err != nil {
		return fmt.Errorf("failed to send HA notification: %w", err)
	}

	return nil
}

// sendHANotification is implemented in ha_notifier.go

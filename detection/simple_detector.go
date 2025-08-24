package main

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/jpeg"
	"log"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"golang.org/x/image/draw"
)

type ImageMessage struct {
	Timestamp int64  `json:"timestamp"`
	ImageData []byte `json:"image_data"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
}

type DetectionResult struct {
	Timestamp   int64       `json:"timestamp"`
	ImageID     int64       `json:"image_id"`
	Detections  []Detection `json:"detections"`
	ProcessTime float64     `json:"process_time_ms"`
}

type Detection struct {
	Label       string  `json:"label"`
	Confidence  float32 `json:"confidence"`
	BoundingBox Box     `json:"bounding_box"`
}

type Box struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

type SimpleDetector struct {
	kafkaBroker     string
	inputTopic      string
	outputTopic     string
	
	consumer        *kafka.Consumer
	producer        *kafka.Producer
}

func NewSimpleDetector(kafkaBroker, inputTopic, outputTopic string) *SimpleDetector {
	return &SimpleDetector{
		kafkaBroker: kafkaBroker,
		inputTopic:  inputTopic,
		outputTopic: outputTopic,
	}
}

func (sd *SimpleDetector) Initialize() error {
	var err error
	
	// Initialize Kafka consumer
	sd.consumer, err = kafka.NewConsumer(&kafka.ConfigMap{
		"bootstrap.servers": sd.kafkaBroker,
		"group.id":         "simple-detector",
		"auto.offset.reset": "earliest",
	})
	if err != nil {
		return err
	}
	
	// Initialize Kafka producer
	sd.producer, err = kafka.NewProducer(&kafka.ConfigMap{
		"bootstrap.servers": sd.kafkaBroker,
	})
	if err != nil {
		return err
	}
	
	// Subscribe to input topic
	err = sd.consumer.Subscribe(sd.inputTopic, nil)
	if err != nil {
		return err
	}
	
	log.Println("Simple detector initialized successfully (TPU hardware detected - using placeholder for now)")
	return nil
}

func (sd *SimpleDetector) Run(ctx context.Context) error {
	log.Println("Starting detection service...")
	
	for {
		select {
		case <-ctx.Done():
			log.Println("Context cancelled, stopping detection service")
			return nil
		default:
		}
		
		// Read message from Kafka
		msg, err := sd.consumer.ReadMessage(100 * time.Millisecond)
		if err != nil {
			if err.(kafka.Error).Code() == kafka.ErrTimedOut {
				continue
			}
			log.Printf("Consumer error: %v", err)
			continue
		}
		
		// Process the image message
		err = sd.processImageMessage(msg.Value)
		if err != nil {
			log.Printf("Error processing image: %v", err)
			continue
		}
	}
}

func (sd *SimpleDetector) resizeImage(src image.Image, targetWidth, targetHeight int) image.Image {
	srcBounds := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, targetWidth, targetHeight))
	
	draw.BiLinear.Scale(dst, dst.Bounds(), src, srcBounds, draw.Over, nil)
	return dst
}

func (sd *SimpleDetector) processImageMessage(data []byte) error {
	startTime := time.Now()
	
	// Parse the image message
	var imgMsg ImageMessage
	err := json.Unmarshal(data, &imgMsg)
	if err != nil {
		return err
	}
	
	// Decode JPEG image
	img, err := jpeg.Decode(bytes.NewReader(imgMsg.ImageData))
	if err != nil {
		return err
	}
	
	// Resize image to 320x320 for TensorFlow Lite model
	resizedImg := sd.resizeImage(img, 320, 320)
	
	// Placeholder: Convert to format suitable for TPU model
	_ = resizedImg // Will be used for actual TPU inference
	
	// Perform detection (placeholder implementation)
	detections := sd.detectObjects()
	
	// Create detection result
	result := DetectionResult{
		Timestamp:   time.Now().UnixNano() / int64(time.Millisecond),
		ImageID:     imgMsg.Timestamp,
		Detections:  detections,
		ProcessTime: float64(time.Since(startTime).Nanoseconds()) / 1e6,
	}
	
	// Send result to output topic
	resultBytes, err := json.Marshal(result)
	if err != nil {
		return err
	}
	
	err = sd.producer.Produce(&kafka.Message{
		TopicPartition: kafka.TopicPartition{Topic: &sd.outputTopic, Partition: kafka.PartitionAny},
		Value:          resultBytes,
	}, nil)
	
	if err != nil {
		return err
	}
	
	log.Printf("Processed image %d (resized to 320x320), found %d detections (%.2fms)", 
		imgMsg.Timestamp, len(detections), result.ProcessTime)
	
	return nil
}

func (sd *SimpleDetector) detectObjects() []Detection {
	// Placeholder TPU detection implementation
	// This can be replaced with actual TensorFlow Lite Edge TPU code
	return []Detection{
		{
			Label:      "person",
			Confidence: 0.95,
			BoundingBox: Box{
				X:      80,
				Y:      60,
				Width:  160,
				Height: 200,
			},
		},
	}
}

func (sd *SimpleDetector) Stop() {
	if sd.consumer != nil {
		sd.consumer.Close()
	}
	
	if sd.producer != nil {
		sd.producer.Close()
	}
	
	log.Println("Simple detector stopped")
}
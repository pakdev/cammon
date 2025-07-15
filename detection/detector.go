package main

import (
	"bytes"
	"context"
	"encoding/json"
	"image/jpeg"
	"log"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"gocv.io/x/gocv"
)

type ImageMessage struct {
	Timestamp int64  `json:"timestamp"`
	ImageData []byte `json:"image_data"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
}

type DetectionResult struct {
	Timestamp   int64        `json:"timestamp"`
	ImageID     int64        `json:"image_id"`
	Detections  []Detection  `json:"detections"`
	ProcessTime float64      `json:"process_time_ms"`
}

type Detection struct {
	Label      string  `json:"label"`
	Confidence float32 `json:"confidence"`
	BoundingBox Box    `json:"bounding_box"`
}

type Box struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

type PersonDetector struct {
	kafkaBroker     string
	inputTopic      string
	outputTopic     string
	
	consumer        *kafka.Consumer
	producer        *kafka.Producer
	net             gocv.Net
}

func NewPersonDetector(kafkaBroker, inputTopic, outputTopic string) *PersonDetector {
	return &PersonDetector{
		kafkaBroker: kafkaBroker,
		inputTopic:  inputTopic,
		outputTopic: outputTopic,
	}
}

func (pd *PersonDetector) Initialize() error {
	var err error
	
	// Initialize Kafka consumer
	pd.consumer, err = kafka.NewConsumer(&kafka.ConfigMap{
		"bootstrap.servers": pd.kafkaBroker,
		"group.id":         "person-detector",
		"auto.offset.reset": "earliest",
	})
	if err != nil {
		return err
	}
	
	// Initialize Kafka producer
	pd.producer, err = kafka.NewProducer(&kafka.ConfigMap{
		"bootstrap.servers": pd.kafkaBroker,
	})
	if err != nil {
		return err
	}
	
	// Subscribe to input topic
	err = pd.consumer.Subscribe(pd.inputTopic, nil)
	if err != nil {
		return err
	}
	
	// Initialize TensorFlow Lite model for Coral TPU
	// Note: This assumes you have a person detection model
	// You would typically load a model like MobileNet SSD or similar
	pd.net = gocv.ReadNet("models/mobilenet_ssd_v2_coco_quant_postprocess_edgetpu.tflite", "")
	if pd.net.Empty() {
		log.Println("Warning: Could not load TensorFlow Lite model. Using placeholder detection.")
	}
	
	log.Println("Person detector initialized successfully")
	return nil
}

func (pd *PersonDetector) Run(ctx context.Context) error {
	log.Println("Starting person detection service...")
	
	for {
		select {
		case <-ctx.Done():
			log.Println("Context cancelled, stopping detection service")
			return nil
		default:
		}
		
		// Read message from Kafka
		msg, err := pd.consumer.ReadMessage(100 * time.Millisecond)
		if err != nil {
			if err.(kafka.Error).Code() == kafka.ErrTimedOut {
				continue
			}
			log.Printf("Consumer error: %v", err)
			continue
		}
		
		// Process the image message
		err = pd.processImageMessage(msg.Value)
		if err != nil {
			log.Printf("Error processing image: %v", err)
			continue
		}
	}
}

func (pd *PersonDetector) processImageMessage(data []byte) error {
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
	
	// Convert to OpenCV Mat
	mat, err := gocv.ImageToMatRGB(img)
	if err != nil {
		return err
	}
	defer mat.Close()
	
	// Perform person detection
	detections := pd.detectPersons(mat)
	
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
	
	err = pd.producer.Produce(&kafka.Message{
		TopicPartition: kafka.TopicPartition{Topic: &pd.outputTopic, Partition: kafka.PartitionAny},
		Value:          resultBytes,
	}, nil)
	
	if err != nil {
		return err
	}
	
	log.Printf("Processed image %d, found %d detections (%.2fms)", 
		imgMsg.Timestamp, len(detections), result.ProcessTime)
	
	return nil
}

func (pd *PersonDetector) detectPersons(mat gocv.Mat) []Detection {
	// Placeholder implementation
	// In a real implementation, you would:
	// 1. Preprocess the image (resize, normalize)
	// 2. Run inference on the Coral TPU
	// 3. Post-process the results (NMS, filtering)
	// 4. Return person detections
	
	if pd.net.Empty() {
		// Placeholder detection for demonstration
		return []Detection{
			{
				Label:      "person",
				Confidence: 0.85,
				BoundingBox: Box{
					X:      100,
					Y:      100,
					Width:  200,
					Height: 400,
				},
			},
		}
	}
	
	// Real TensorFlow Lite inference would go here
	// For now, return empty detections
	return []Detection{}
}

func (pd *PersonDetector) Stop() {
	if pd.consumer != nil {
		pd.consumer.Close()
	}
	
	if pd.producer != nil {
		pd.producer.Close()
	}
	
	if !pd.net.Empty() {
		pd.net.Close()
	}
	
	log.Println("Person detector stopped")
}
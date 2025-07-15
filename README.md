# Camera Monitoring System

A Go monorepo for real-time camera monitoring with AI-powered person detection.

## Architecture

This project consists of two main modules:

### 1. `cam_io` - Camera Input/Output Module
- **Purpose**: Captures images from RTSP streams and publishes them to Kafka
- **Features**:
  - H.264 video decoding using FFmpeg
  - RTSP stream processing
  - Kafka producer for image streaming
  - Context-based graceful shutdown

### 2. `detection` - Person Detection Module  
- **Purpose**: Consumes images from Kafka and performs person detection using Google Coral TPU
- **Features**:
  - Kafka consumer for image processing
  - TensorFlow Lite integration with Coral TPU support
  - Person detection using pre-trained models
  - Results publishing to separate Kafka topic

## Project Structure

```
cammon/
├── go.work                    # Go workspace file
├── cam_io/                    # Camera I/O module
│   ├── go.mod
│   ├── main.go               # RTSP capture service
│   ├── capture.go            # Image capture logic
│   └── decoders/
│       └── h264_decoder.go   # H.264 decoder (FFmpeg wrapper)
├── detection/                 # Detection module
│   ├── go.mod
│   ├── main.go               # Detection service
│   ├── detector.go           # Person detection logic
│   └── models/               # TensorFlow Lite models
└── README.md
```

## Prerequisites

### System Dependencies
- **FFmpeg**: Required for H.264 decoding
  ```bash
  # Ubuntu/Debian
  sudo apt-get install libavcodec-dev libavutil-dev libswscale-dev
  
  # macOS
  brew install ffmpeg
  ```

- **OpenCV**: Required for image processing
  ```bash
  # Ubuntu/Debian  
  sudo apt-get install libopencv-dev
  
  # macOS
  brew install opencv
  ```

- **Kafka**: Message broker for image streaming
  ```bash
  # Start Kafka locally (example with Docker)
  docker run -p 9092:9092 apache/kafka:2.8.0
  ```

### Go Dependencies
- Go 1.24.5 or later
- All dependencies are managed via Go modules

## Setup

1. **Clone and initialize workspace**:
   ```bash
   git clone <repository>
   cd cammon
   go work sync
   ```

2. **Download TensorFlow Lite model** (for detection module):
   ```bash
   # Download a person detection model for Coral TPU
   cd detection/models
   wget https://github.com/google-coral/test_data/raw/master/mobilenet_ssd_v2_coco_quant_postprocess_edgetpu.tflite
   ```

## Usage

### Running the Camera Capture Service

```bash
# Start the RTSP capture service
cd cam_io
go run .
```

This will:
- Connect to RTSP stream at `rtsp://10.0.0.201/Streaming/Channels/101`
- Decode H.264 video frames
- Publish images to Kafka topic `camera-images`

### Running the Person Detection Service

```bash
# Start the detection service  
cd detection
go run .
```

This will:
- Consume images from Kafka topic `camera-images`
- Run person detection on each image
- Publish detection results to Kafka topic `detections`

### Building Both Services

```bash
# Build all modules
go work sync

# Build specific modules
cd cam_io && go build -o ../bin/cam_io
cd detection && go build -o ../bin/detection
```

## Configuration

### Camera Configuration
Edit `cam_io/main.go` to change:
- RTSP URL
- Kafka broker address
- Output topic name

### Detection Configuration  
Edit `detection/main.go` to change:
- Kafka broker address
- Input/output topic names
- Model file path

## Data Flow

```
RTSP Stream → cam_io → Kafka (camera-images) → detection → Kafka (detections)
```

1. **cam_io** captures frames from RTSP stream
2. Frames are encoded as JSON and sent to `camera-images` topic
3. **detection** consumes images from `camera-images` 
4. Each image is processed for person detection
5. Detection results are published to `detections` topic

## Message Formats

### Image Message (camera-images topic)
```json
{
  "timestamp": 1752473336825,
  "image_data": "base64_encoded_jpeg_data",
  "width": 1920,
  "height": 1080
}
```

### Detection Result (detections topic)
```json
{
  "timestamp": 1752473336825,
  "image_id": 1752473336825,
  "process_time_ms": 45.2,
  "detections": [
    {
      "label": "person",
      "confidence": 0.85,
      "bounding_box": {
        "x": 100,
        "y": 100, 
        "width": 200,
        "height": 400
      }
    }
  ]
}
```

## Development

### Adding New Modules
1. Create new directory in project root
2. Initialize Go module: `go mod init github.com/cammon/new_module`
3. Add to `go.work`: `go work use ./new_module`

### Testing
```bash
# Test all modules
go work sync
go test ./...

# Test specific module
cd cam_io && go test ./...
```

## Troubleshooting

### FFmpeg Issues
- Ensure FFmpeg development libraries are installed
- Check that `pkg-config` can find the libraries

### Kafka Connection Issues  
- Verify Kafka is running on specified broker address
- Check topic exists and is accessible
- Ensure proper network connectivity

### Coral TPU Issues
- Verify Coral TPU drivers are installed
- Check that TensorFlow Lite model is compatible with TPU
- Ensure proper USB/PCIe connection to TPU device
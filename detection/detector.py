#!/usr/bin/env python3

import json
import pathlib
import time
import logging
import os
from typing import List, Dict, Any, Optional, Tuple
from io import BytesIO
import numpy as np
from PIL import Image, ImageDraw
from kafka import KafkaConsumer, KafkaProducer

import tflite_runtime.interpreter as tflite

logger = logging.getLogger(__name__)

try:
    from pycoral.utils.edgetpu import make_interpreter
    from pycoral.adapters import common, detect
    CORAL_AVAILABLE = True
    logger.info("PyCoral library loaded successfully")
except ImportError as e:
    CORAL_AVAILABLE = False
    logger.warning(f"PyCoral library not available: {e}")
    logger.info("Falling back to CPU-only inference")

class Detection:
    def __init__(self, label: str, confidence: float, bbox: Tuple[int, int, int, int]):
        self.label = label
        self.confidence = confidence
        self.bbox = bbox  # (x, y, width, height)
    
    def to_dict(self) -> Dict[str, Any]:
        return {
            "label": self.label,
            "confidence": float(self.confidence),
            "bounding_box": {
                "x": int(self.bbox[0]),
                "y": int(self.bbox[1]), 
                "width": int(self.bbox[2]),
                "height": int(self.bbox[3])
            }
        }

class TPUDetector:
    def __init__(self, kafka_broker: str, input_topic: str, output_topic: str, model_dir: Optional[str] = None, annotated_topic: str = "detections"):
        self.kafka_broker = kafka_broker
        self.input_topic = input_topic
        self.output_topic = output_topic
        self.annotated_topic = annotated_topic
        
        self.consumer: Optional[KafkaConsumer] = None
        self.producer: Optional[KafkaProducer] = None
        self.interpreter: Optional[tflite.Interpreter] = None
        
        self.this_dir = pathlib.Path(__file__).parent
        
        # Set model directory based on mode
        if model_dir:
            self.model_dir = model_dir
        elif kafka_broker:
            # Docker/Kafka mode - use container path
            self.model_dir = "/app/models"
        else:
            # Standalone mode - use host path
            self.model_dir = self.this_dir / "models"
        
        self.input_height = 448 
        self.input_width = 448 
        self.use_tpu = False
        self.model_path = None
        self.labels = {}
        
    def initialize(self) -> None:
        """Initialize Kafka connections and TPU model"""
        logger.info("Initializing TPU detector...")
        
        # Initialize Kafka connections only if broker is specified
        if self.kafka_broker:
            # Initialize Kafka consumer
            self.consumer = KafkaConsumer(
                self.input_topic,
                bootstrap_servers=[self.kafka_broker],
                group_id='tpu-detector-python',
                auto_offset_reset='earliest',
                value_deserializer=lambda x: json.loads(x.decode('utf-8'))
            )
            
            # Initialize Kafka producer
            self.producer = KafkaProducer(
                bootstrap_servers=[self.kafka_broker],
                value_serializer=lambda x: json.dumps(x).encode('utf-8')
            )
            logger.info("Kafka connections initialized")
        else:
            logger.info("Standalone mode - Kafka connections skipped")
        
        # Load TPU model
        self._load_model()
        self._load_labels()
        
        logger.info(f"TPU detector initialized successfully")
        logger.info(f"Model: {self.model_path}")
        logger.info(f"Input size: {self.input_width}x{self.input_height}")
        logger.info(f"Using TPU: {self.use_tpu}")
        
    def _load_model(self) -> None:
        """Load TensorFlow Lite Edge TPU model"""
        model_dir = self.model_dir
        
        # Look for models (prefer TPU models if available)
        tpu_models = [
            "efficientdet_lite2_448_ptq_edgetpu.tflite",
        ]
        
        cpu_models = [
            "efficientdet_lite2_448_ptq.tflite"
        ]
        
        # Find available model
        self.model_path = None
        model_type = "unknown"
        
        # Try TPU models first only if PyCoral is available
        if CORAL_AVAILABLE:
            for model_name in tpu_models:
                path = os.path.join(model_dir, model_name)
                if os.path.exists(path):
                    self.model_path = path
                    model_type = "TPU"
                    logger.info(f"Found TPU model: {model_name}")
                    break
        
        # Fallback to CPU models
        if not self.model_path:
            for model_name in cpu_models:
                path = os.path.join(model_dir, model_name)
                if os.path.exists(path):
                    self.model_path = path
                    model_type = "CPU"
                    logger.info(f"Found CPU model: {model_name}")
                    break
                    
        if not self.model_path:
            raise FileNotFoundError(f"No compatible model found in {model_dir}")
        
        # Try to create Edge TPU interpreter for TPU models
        if CORAL_AVAILABLE and model_type == "TPU":
            try:
                self.interpreter = make_interpreter(self.model_path)
                self.use_tpu = True
                logger.info("Edge TPU interpreter created successfully")
            except Exception as e:
                logger.warning(f"Failed to create Edge TPU interpreter: {e}")
                logger.info("Falling back to CPU interpreter")
                self.interpreter = None
                self.use_tpu = False
        else:
            if model_type == "TPU" and not CORAL_AVAILABLE:
                logger.warning("TPU model found but PyCoral not available")
            self.interpreter = None
            self.use_tpu = False
            
        # Fallback to CPU interpreter
        if not self.interpreter:
            try:
                self.interpreter = tflite.Interpreter(model_path=self.model_path)
                self.use_tpu = False
                if model_type == "TPU":
                    logger.warning("Using CPU interpreter for TPU model (may fail)")
            except Exception as e:
                if model_type == "TPU":
                    logger.error(f"TPU model cannot run on CPU: {e}")
                    raise RuntimeError(f"TPU model requires Edge TPU hardware and PyCoral library")
                else:
                    raise e
            
        # Allocate tensors
        self.interpreter.allocate_tensors()
        
        # Get input/output details
        input_details = self.interpreter.get_input_details()
        output_details = self.interpreter.get_output_details()
        
        # Verify input shape
        input_shape = input_details[0]['shape']
        if len(input_shape) == 4:
            self.input_height = input_shape[1]
            self.input_width = input_shape[2]
        
        logger.info(f"Model input shape: {input_shape}")
        logger.info(f"Model has {len(output_details)} output tensors")
        
    def _load_labels(self) -> None:
        """Load COCO labels"""
        label_path = os.path.join(self.model_dir, "coco_labels.txt")
        if os.path.exists(label_path):
            with open(label_path, 'r') as f:
                lines = f.read().strip().split('\n')
                for i, label in enumerate(lines):
                    self.labels[i] = label
            logger.info(f"Loaded {len(self.labels)} class labels")
        else:
            # Default COCO labels
            self.labels = {
                0: 'person', 1: 'bicycle', 2: 'car', 3: 'motorcycle', 4: 'airplane',
                5: 'bus', 6: 'train', 7: 'truck', 8: 'boat', 9: 'traffic light',
                10: 'fire hydrant', 11: 'stop sign', 12: 'parking meter', 13: 'bench',
                14: 'bird', 15: 'cat', 16: 'dog', 17: 'horse', 18: 'sheep', 19: 'cow'
            }
            logger.info("Using default COCO labels")
    
    def _preprocess_image(self, image_input) -> Tuple[np.ndarray, Image.Image]:
        """Preprocess image for model input
        
        Args:
            image_input: Either bytes (from Kafka) or file path string
            
        Returns:
            Tuple of (numpy array for inference, PIL Image for visualization)
        """
        if isinstance(image_input, pathlib.Path):
            image = Image.open(str(image_input))
        elif isinstance(image_input, str):
            # Load image from file path
            image = Image.open(image_input)
        else:
            # Load image from bytes (Kafka mode)
            image = Image.open(BytesIO(image_input))
        
        # Convert to RGB if needed
        if image.mode != 'RGB':
            image = image.convert('RGB')
            
        # Resize to model input size
        scaled_image = image.resize((self.input_width, self.input_height), Image.LANCZOS)
        
        # Convert to numpy array
        image_np = np.array(scaled_image, dtype=np.uint8)
        
        # Add batch dimension
        if len(image_np.shape) == 3:
            image_np = np.expand_dims(image_np, axis=0)
            
        return image_np, scaled_image
    
    def _draw_bounding_boxes(self, image: Image.Image, detections: List[Detection]) -> Image.Image:
        """Draw bounding boxes on the scaled image
        
        Args:
            image: PIL Image (scaled to model input size)
            detections: List of detection results
            
        Returns:
            PIL Image with bounding boxes drawn
        """
        # Create a copy of the image to draw on
        annotated_image = image.copy()
        draw = ImageDraw.Draw(annotated_image)
        
        for detection in detections:
            x, y, width, height = detection.bbox
            
            # Calculate rectangle coordinates
            left = x
            top = y
            right = x + width
            bottom = y + height
            
            # Draw red rectangle with 3px width
            for i in range(3):
                draw.rectangle([left-i, top-i, right+i, bottom+i], outline='red', fill=None)
            
            # Optionally add label text (commented out for cleaner look)
            # label_text = f"{detection.label}: {detection.confidence:.2f}"
            # draw.text((left, top-15), label_text, fill='red')
        
        return annotated_image
    
    def _run_inference(self, image_np: np.ndarray) -> List[Detection]:
        """Run inference on preprocessed image"""
        start_time = time.time()
        
        # Set input tensor
        input_details = self.interpreter.get_input_details()
        self.interpreter.set_tensor(input_details[0]['index'], image_np)
        
        # Run inference
        self.interpreter.invoke()
        
        # Get output tensors
        output_details = self.interpreter.get_output_details()
        
        # Extract detection results
        if self.use_tpu and CORAL_AVAILABLE:
            # Use PyCoral for Edge TPU results
            detections = detect.get_objects(self.interpreter, score_threshold=0.3)
            results = self._parse_coral_detections(detections)
        else:
            # Parse standard TensorFlow Lite outputs
            results = self._parse_tflite_outputs(output_details)
        
        inference_time = (time.time() - start_time) * 1000  # Convert to ms
        logger.debug(f"Inference time: {inference_time:.2f}ms")
        
        return results
    
    def _parse_coral_detections(self, detections) -> List[Detection]:
        """Parse detections from PyCoral"""
        results = []
        for detection in detections:
            bbox = detection.bbox
            class_id = detection.id
            confidence = detection.score
            
            # PyCoral bbox coordinates are already in pixel format, not normalized
            x = int(bbox.xmin)
            y = int(bbox.ymin) 
            width = int(bbox.xmax - bbox.xmin)
            height = int(bbox.ymax - bbox.ymin)
            
            # Clamp coordinates to image bounds
            x = max(0, min(x, self.input_width - 1))
            y = max(0, min(y, self.input_height - 1))
            width = max(1, min(width, self.input_width - x))
            height = max(1, min(height, self.input_height - y))
            
            label = self.labels.get(class_id, f"class_{class_id}")
            
            results.append(Detection(label, confidence, (x, y, width, height)))
            
        return results
    
    def _parse_tflite_outputs(self, output_details) -> List[Detection]:
        """Parse outputs from standard TensorFlow Lite model"""
        results = []
        
        if len(output_details) >= 4:
            # Standard SSD output format
            # boxes [1, N, 4], classes [1, N], scores [1, N], num_detections [1]
            boxes = self.interpreter.get_tensor(output_details[0]['index'])[0]
            classes = self.interpreter.get_tensor(output_details[1]['index'])[0]
            scores = self.interpreter.get_tensor(output_details[2]['index'])[0]
            num_detections = int(self.interpreter.get_tensor(output_details[3]['index'])[0])
            
            for i in range(min(num_detections, len(scores))):
                if scores[i] < 0.3:  # Confidence threshold
                    continue
                    
                # Extract bounding box (normalized coordinates)
                ymin, xmin, ymax, xmax = boxes[i]
                
                # Convert to pixel coordinates
                x = int(xmin * self.input_width)
                y = int(ymin * self.input_height)
                width = int((xmax - xmin) * self.input_width)
                height = int((ymax - ymin) * self.input_height)
                
                # Get class label
                class_id = int(classes[i])
                label = self.labels.get(class_id, f"class_{class_id}")
                
                results.append(Detection(label, scores[i], (x, y, width, height)))
        
        return results
    
    def _process_message(self, message) -> None:
        """Process a single image message"""
        try:
            start_time = time.time()
            
            # Extract image data - Go service sends as base64 in JSON
            import base64
            if 'image_data' in message:
                # Base64 format from Go service JSON marshaling
                image_data = base64.b64decode(message['image_data'])
            else:
                # Fallback for other formats
                image_data = base64.b64decode(message['data'])
            timestamp = message['timestamp']
            
            # Preprocess image
            image_np, scaled_image = self._preprocess_image(image_data)
            
            # Run inference
            detections = self._run_inference(image_np)
            
            # Draw bounding boxes on scaled image
            annotated_image = self._draw_bounding_boxes(scaled_image, detections)
            
            # Convert annotated image to base64 for Kafka
            import base64
            from io import BytesIO
            buffer = BytesIO()
            annotated_image.save(buffer, format='JPEG', quality=85)
            annotated_image_data = base64.b64encode(buffer.getvalue()).decode('utf-8')
            
            # Create result
            result = {
                'timestamp': int(time.time() * 1000),
                'image_id': timestamp,
                'detections': [det.to_dict() for det in detections],
                'process_time_ms': (time.time() - start_time) * 1000
            }
            
            # Send to output topic
            self.producer.send(self.output_topic, value=result)
            
            # Send annotated image to detections topic
            annotated_result = {
                'timestamp': int(time.time() * 1000),
                'image_id': timestamp,
                'image_data': annotated_image_data,
                'detections': [det.to_dict() for det in detections],
                'processed_size': {
                    'width': int(self.input_width),
                    'height': int(self.input_height)
                },
                'process_time_ms': (time.time() - start_time) * 1000,
                'accelerator': "TPU" if self.use_tpu else "CPU"
            }
            
            self.producer.send(self.annotated_topic, value=annotated_result)
            
            accelerator = "TPU" if self.use_tpu else "CPU"
            logger.info(f"Processed image {timestamp} ({self.input_width}x{self.input_height}), "
                       f"found {len(detections)} detections "
                       f"({result['process_time_ms']:.2f}ms on {accelerator}) - sent to {self.annotated_topic}")
                       
        except Exception as e:
            logger.error(f"Error processing message: {e}")
    
    def run(self) -> None:
        """Main processing loop for Kafka mode"""
        if not self.consumer:
            raise RuntimeError("Kafka consumer not initialized. Use process_images() for standalone mode.")
            
        logger.info("Starting TPU detection service...")
        
        try:
            for message in self.consumer:
                self._process_message(message.value)
        except KeyboardInterrupt:
            logger.info("Stopping detection service...")
    
    def process_images(self, image_paths: List[str]) -> List[Dict[str, Any]]:
        """Process images in standalone mode
        
        Args:
            image_paths: List of image file paths
            
        Returns:
            List of detection results
        """
        results = []
        
        logger.info(f"Processing {len(image_paths)} images...")
        
        for i, image_path_str in enumerate(image_paths, 1):
            try:
                # Handle both absolute and relative paths
                if os.path.isabs(image_path_str):
                    image_path = pathlib.Path(image_path_str)
                else:
                    # Use current working directory for relative paths
                    image_path = pathlib.Path(os.getcwd()) / image_path_str
                
                logger.info(f"Processing image {i}/{len(image_paths)}: {image_path}")
                
                start_time = time.time()
                
                # Check if file exists
                if not image_path.exists():
                    logger.error(f"Image file not found: {image_path}")
                    continue
                
                # Get original image info
                with Image.open(image_path) as img:
                    original_width, original_height = img.size
                
                # Preprocess image
                image_np, scaled_image = self._preprocess_image(image_path)
                
                # Run inference
                detections = self._run_inference(image_np)
                
                # Draw bounding boxes on scaled image
                annotated_image = self._draw_bounding_boxes(scaled_image, detections)
                
                # Save annotated image
                annotated_path = image_path.parent / f"{image_path.stem}_annotated{image_path.suffix}"
                annotated_image.save(annotated_path)
                logger.info(f"Saved annotated image: {annotated_path}")
                
                # Create result
                result = {
                    'image_path': str(image_path),
                    'annotated_path': str(annotated_path),
                    'original_size': {
                        'width': int(original_width),
                        'height': int(original_height)
                    },
                    'processed_size': {
                        'width': int(self.input_width),
                        'height': int(self.input_height)
                    },
                    'timestamp': int(time.time() * 1000),
                    'detections': [det.to_dict() for det in detections],
                    'process_time_ms': float((time.time() - start_time) * 1000),
                    'accelerator': 'TPU' if self.use_tpu else 'CPU'
                }
                
                results.append(result)
                
                logger.info(f"Found {len(detections)} detections "
                          f"({result['process_time_ms']:.2f}ms on {result['accelerator']})")
                
                # Print detections summary
                for det in detections:
                    logger.info(f"  - {det.label}: {det.confidence:.3f} "
                              f"at [{det.bbox[0]}, {det.bbox[1]}, {det.bbox[2]}, {det.bbox[3]}]")
                              
            except Exception as e:
                logger.error(f"Error processing {image_path}: {e}")
                # Add error result  
                results.append({
                    'image_path': str(image_path_str),
                    'error': str(e),
                    'timestamp': int(time.time() * 1000)
                })
        
        logger.info(f"Processed {len(results)} images successfully")
        return results
        
    def stop(self) -> None:
        """Clean up resources"""
        if self.consumer:
            self.consumer.close()
        if self.producer:
            self.producer.close()
        
        logger.info("TPU detector stopped")
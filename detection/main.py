#!/usr/bin/env python3

import os
import signal
import sys
import logging
import argparse
from detector import TPUDetector

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)

def signal_handler(signum, frame):
    logger.info("Received shutdown signal, stopping...")
    sys.exit(0)

def parse_args():
    parser = argparse.ArgumentParser(
        description='TPU Object Detection Service',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # Kafka mode (default)
  python main.py
  
  # Standalone mode with single image
  python main.py --images /path/to/image.jpg
  
  # Standalone mode with multiple images
  python main.py --images /path/to/img1.jpg /path/to/img2.jpg /path/to/img3.jpg
  
  # Standalone mode with output file
  python main.py --images /path/to/image.jpg --output results.json
        """
    )
    
    parser.add_argument(
        '--images', 
        nargs='+',
        help='One or more image file paths for standalone processing'
    )
    
    parser.add_argument(
        '--output',
        help='Output file for detection results (JSON format)'
    )
    
    parser.add_argument(
        '--kafka-broker',
        default=os.getenv('KAFKA_BROKER', 'localhost:9092'),
        help='Kafka broker address (default: localhost:9092)'
    )
    
    parser.add_argument(
        '--input-topic',
        default=os.getenv('INPUT_TOPIC', 'camera-images'),
        help='Kafka input topic (default: camera-images)'
    )
    
    parser.add_argument(
        '--output-topic', 
        default=os.getenv('OUTPUT_TOPIC', 'detections'),
        help='Kafka output topic (default: detections)'
    )
    
    parser.add_argument(
        '--model-dir',
        help='Model directory path (auto-detected if not provided)'
    )
    
    parser.add_argument(
        '--annotated-topic',
        default=os.getenv('ANNOTATED_TOPIC', 'detections'),
        help='Kafka topic for annotated images (default: detections)'
    )
    
    return parser.parse_args()

def run_kafka_mode(args):
    """Run in Kafka streaming mode"""
    logger.info(f"Starting TPU detector in Kafka mode")
    logger.info(f"Kafka broker: {args.kafka_broker}")
    logger.info(f"Input topic: {args.input_topic}, Output topic: {args.output_topic}")
    logger.info(f"Annotated images topic: {args.annotated_topic}")
    
    detector = TPUDetector(
        kafka_broker=args.kafka_broker,
        input_topic=args.input_topic,
        output_topic=args.output_topic,
        model_dir=args.model_dir,
        annotated_topic=args.annotated_topic
    )
    
    try:
        detector.initialize()
        detector.run()
    except KeyboardInterrupt:
        logger.info("Interrupted by user")
    except Exception as e:
        logger.error(f"Detector error: {e}")
        raise
    finally:
        detector.stop()

def run_standalone_mode(args):
    """Run in standalone file processing mode"""
    logger.info(f"Starting TPU detector in standalone mode")
    logger.info(f"Processing {len(args.images)} image(s)")
    
    detector = TPUDetector(
        kafka_broker=None,  # No Kafka needed
        input_topic=None,
        output_topic=None,
        model_dir=args.model_dir,
        annotated_topic=None  # No Kafka needed
    )
    
    try:
        detector.initialize()
        results = detector.process_images(args.images)
        
        if args.output:
            # Save results to file
            import json
            with open(args.output, 'w') as f:
                json.dump(results, f, indent=2)
            logger.info(f"Results saved to: {args.output}")
        else:
            # Print results to console
            import json
            print(json.dumps(results, indent=2))
            
    except Exception as e:
        logger.error(f"Processing error: {e}")
        raise
    finally:
        detector.stop()

def main():
    # Set up signal handlers for graceful shutdown
    signal.signal(signal.SIGINT, signal_handler)
    signal.signal(signal.SIGTERM, signal_handler)
    
    args = parse_args()
    
    if args.images:
        # Standalone mode - process image files
        run_standalone_mode(args)
    else:
        # Kafka streaming mode
        run_kafka_mode(args)

if __name__ == "__main__":
    main()
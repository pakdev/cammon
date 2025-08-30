# Notifier Service

This Go module reads detection events from the Kafka topic "detections" and sends notifications to Home Assistant companion app.

## Features

- Consumes detection messages from Kafka
- Sends notifications via Home Assistant webhook or companion app API
- Supports both webhook and companion app notification methods
- Graceful shutdown handling
- Configurable via environment variables

## Configuration

The following environment variables are supported:

- `KAFKA_BROKER`: Kafka broker address (default: localhost:9092)
- `HA_WEBHOOK_URL`: Home Assistant webhook URL (required)
- `HA_TOKEN`: Home Assistant long-lived access token (optional for webhooks)

## Usage

### With Docker Compose

The service is automatically included in the docker-compose.yml file:

```yaml
notifier:
  build:
    context: ./notifier
    dockerfile: Dockerfile
  container_name: notifier
  depends_on:
    - kafka
    - homeassistant
  environment:
    - KAFKA_BROKER=kafka:29092
    - HA_WEBHOOK_URL=http://localhost:8123/api/webhook/detection_alert
    - HA_TOKEN=your_ha_token_here
  restart: unless-stopped
```

### Home Assistant Setup

#### Option 1: Webhook Automation

Create a webhook automation in Home Assistant:

1. Go to Settings → Automations & Scenes → Automations
2. Create a new automation with webhook trigger
3. Set webhook ID to: `detection_alert`
4. Add actions to send notifications

#### Option 2: Companion App Notifications

Use the companion app notification service:

1. Set `HA_WEBHOOK_URL` to: `http://localhost:8123/api/services/notify/mobile_app_your_device`
2. Set `HA_TOKEN` to your long-lived access token
3. The service will send notifications directly to your device

## Message Format

The service expects detection messages in the following JSON format:

```json
{
  "timestamp": "2025-01-20T15:04:05Z",
  "objects": [
    {
      "label": "person",
      "confidence": 0.95,
      "box": {
        "x1": 100.0,
        "y1": 100.0,
        "x2": 200.0,
        "y2": 200.0
      }
    }
  ]
}
```

## Building and Running

### Local Development

```bash
cd notifier
go mod tidy
go run .
```

### Docker

```bash
docker build -t notifier .
docker run -e KAFKA_BROKER=localhost:9092 -e HA_WEBHOOK_URL=your_webhook_url notifier
```
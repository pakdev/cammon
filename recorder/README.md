# Video Recorder

This service records video segments from a go2rtc stream and can optionally upload them to Google Drive instead of storing them locally.

## Configuration

### Basic Configuration

- `GO2RTC_URL`: URL to the go2rtc stream (default: `http://go2rtc:1984/api/stream.mp4?src=camera1`)
- `RECORDING_INTERVAL`: Recording interval in seconds (default: 60)
- `RECORDINGS_DIR`: Local directory for recordings (default: `/recordings`)

### Google Drive Configuration

To enable Google Drive uploads instead of local storage, set the following environment variables:

- `USE_GOOGLE_DRIVE=true`: Enable Google Drive uploads
- `GOOGLE_DRIVE_CREDENTIALS`: Path to Google Drive API credentials JSON file (default: `/credentials/credentials.json`)
- `GOOGLE_DRIVE_TOKEN`: Path to store OAuth token (default: `/credentials/token.json`)
- `GOOGLE_DRIVE_FOLDER_ID`: (Optional) Google Drive folder ID to upload recordings to

## Setting up Google Drive API

1. Go to the [Google Cloud Console](https://console.cloud.google.com/)
2. Create a new project or select an existing one
3. Enable the Google Drive API
4. Create credentials (OAuth 2.0 Client ID) for a desktop application
5. Download the credentials JSON file and place it at the path specified by `GOOGLE_DRIVE_CREDENTIALS`
6. On first run with Google Drive enabled, you'll need to authorize the application by visiting a URL and entering the authorization code

## Docker Compose Example

```yaml
version: '3.8'
services:
  recorder:
    build: .
    environment:
      - USE_GOOGLE_DRIVE=true
      - GOOGLE_DRIVE_CREDENTIALS=/credentials/credentials.json
      - GOOGLE_DRIVE_TOKEN=/credentials/token.json
      - GOOGLE_DRIVE_FOLDER_ID=your-folder-id-here
      - RECORDING_INTERVAL=300
    volumes:
      - ./credentials:/credentials:ro
      - ./temp_recordings:/recordings
    depends_on:
      - go2rtc
```

## Behavior

- When Google Drive is enabled, recordings are first saved locally, then uploaded to Google Drive
- After successful upload, the local file is automatically deleted
- If Google Drive upload fails, the local file is retained
- If Google Drive initialization fails, the service falls back to local recording only
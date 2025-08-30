FROM golang:1.23-alpine AS builder

# Install build dependencies
RUN apk add --no-cache \
    gcc \
    g++ \
    musl-dev

WORKDIR /app

# Copy go mod files
COPY go.mod ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o recorder .

# Final stage
FROM alpine:latest

# Install runtime dependencies (ffmpeg for video recording)
RUN apk add --no-cache \
    ffmpeg \
    ca-certificates

WORKDIR /app

# Copy the binary from builder stage
COPY --from=builder /app/recorder .

# Create recordings directory
RUN mkdir -p /recordings

# Copy credentials file if it exists
COPY secret.json* ./

# Run the application
CMD ["./recorder"]
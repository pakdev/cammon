FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache \
    gcc \
    g++ \
    musl-dev \
    pkgconfig \
    librdkafka-dev

WORKDIR /app

# Copy go mod files
COPY go.mod ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -tags musl -o detection .

# Final stage
FROM alpine:latest

# Install runtime dependencies (TPU library will be mounted from host)
RUN apk add --no-cache \
    librdkafka \
    ca-certificates

WORKDIR /app

# Copy the binary from builder stage
COPY --from=builder /app/detection .

# Create models directory
RUN mkdir -p /app/models

# Run the application
CMD ["./detection"]
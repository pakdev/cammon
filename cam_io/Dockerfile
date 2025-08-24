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
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -tags musl -o cam_io .

# Final stage
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache \
    librdkafka \
    ca-certificates

WORKDIR /app

# Copy the binary from builder stage
COPY --from=builder /app/cam_io .

# Run the application
CMD ["./cam_io"]
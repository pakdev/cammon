FROM python:3.9-slim

# Install system dependencies
RUN apt-get update && apt-get install -y \
    curl \
    gnupg \
    ca-certificates \
    wget \
    build-essential \
    && rm -rf /var/lib/apt/lists/*

# Add Coral Edge TPU repository and install Edge TPU runtime
RUN curl -fsSL https://packages.cloud.google.com/apt/doc/apt-key.gpg | gpg --dearmor -o /etc/apt/trusted.gpg.d/coral-edgetpu-archive-keyring.gpg && \
    echo "deb https://packages.cloud.google.com/apt coral-edgetpu-stable main" > /etc/apt/sources.list.d/coral-edgetpu.list && \
    apt-get update && \
    apt-get install -y libedgetpu1-std && \
    rm -rf /var/lib/apt/lists/*

# Set working directory
WORKDIR /app

# Copy requirements first for better caching
COPY requirements.txt .

# Install Python dependencies with exact versions for PyCoral compatibility
RUN pip install --no-cache-dir kafka-python==2.0.2 Pillow==10.0.1 numpy==1.20.3

# Remove any existing installations and install fresh
RUN pip uninstall -y pycoral tflite-runtime || true

# Install the exact tflite-runtime version that PyCoral 2.0 expects
RUN pip install --extra-index-url https://google-coral.github.io/py-repo/ tflite_runtime==2.5.0.post1

# Install PyCoral from the Coral repository
RUN pip install --extra-index-url https://google-coral.github.io/py-repo/ pycoral~=2.0

# Copy application code
COPY main.py detector.py ./

# Create models directory
RUN mkdir -p /app/models

# Make main.py executable
RUN chmod +x main.py

# Run the application
CMD ["python", "main.py"]
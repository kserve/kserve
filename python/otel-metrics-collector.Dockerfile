# Use lightweight Python image
FROM python:3.9-slim

# Install dependencies
RUN pip install requests opentelemetry-api opentelemetry-sdk opentelemetry-exporter-otlp prometheus_client

# Copy the script into the container
WORKDIR /app
COPY otel_metrics_collector.py .

# Run the script
CMD ["python", "otel_metrics_collector.py"]

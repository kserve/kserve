import time
import os
import requests
from prometheus_client.parser import text_string_to_metric_families
from opentelemetry import metrics
from opentelemetry.sdk.metrics import MeterProvider
from opentelemetry.exporter.otlp.proto.grpc.metric_exporter import OTLPMetricExporter
from opentelemetry.sdk.metrics.export import PeriodicExportingMetricReader

# Read environment variables
OTEL_GRPC_ENDPOINT = os.getenv("OTEL_GRPC_ENDPOINT")
PROMETHEUS_URL = os.getenv("PROMETHEUS_URL", "http://localhost:8080/metrics")

# Create the OTLP metric exporter (adjust the endpoint as needed)
otlp_exporter = OTLPMetricExporter(endpoint=OTEL_GRPC_ENDPOINT, insecure=True)

# Set up OpenTelemetry Meter
reader = PeriodicExportingMetricReader(otlp_exporter, export_interval_millis=5000)
provider = MeterProvider(metric_readers=[reader])
metrics.set_meter_provider(provider)
meter = metrics.get_meter("metrics-collector")

def fetch_prometheus_metrics():
    """Fetch metrics from Prometheus endpoint"""
    try:
        response = requests.get(PROMETHEUS_URL)
        response.raise_for_status()
        return response.text
    except requests.RequestException as e:
        print(f"Error fetching metrics: {e}")
        return None

def convert_and_push_metrics():
    """Convert Prometheus metrics to OpenTelemetry format and export"""
    prometheus_data = fetch_prometheus_metrics()
    if not prometheus_data:
        return

    for family in text_string_to_metric_families(prometheus_data):
        otel_counter = meter.create_counter(name=family.name, description=family.documentation, unit="1")
        for sample in family.samples:
            labels = sample.labels
            value = sample.value
            otel_counter.add(value, attributes=labels)
    

if __name__ == "__main__":
    while True:
        convert_and_push_metrics()
        time.sleep(5)  # Fetch and push every 5 seconds

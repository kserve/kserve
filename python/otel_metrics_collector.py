# Copyright 2023 The KServe Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

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
METRICS_URL = os.getenv("METRICS_URL", "http://localhost:8080/metrics")
POLLING_INTERVAL = os.getenv("POLLING_INTERVAL", 5)

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
        response = requests.get(METRICS_URL)
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
        time.sleep(POLLING_INTERVAL)  # Fetch and push every 5 seconds

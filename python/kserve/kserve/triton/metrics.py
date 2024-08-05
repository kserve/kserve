# Copyright 2024 The KServe Authors.
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

import tritonserver
from prometheus_client import parser
from prometheus_client.core import GaugeMetricFamily, CounterMetricFamily
from prometheus_client.metrics_core import (
    InfoMetricFamily,
    HistogramMetricFamily,
    SummaryMetricFamily,
    GaugeHistogramMetricFamily,
    StateSetMetricFamily,
    UnknownMetricFamily,
)
from prometheus_client.registry import Collector
from tritonserver import MetricFormat


class TritonMetricsCollector(Collector):
    def __init__(self, server: tritonserver.Server, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self._server = server

    def collect(self):
        families = parser.text_string_to_metric_families(
            self._server.metrics(metric_format=MetricFormat.PROMETHEUS)
        )
        for family in families:
            if family.type == "gauge":
                metric_family = GaugeMetricFamily(
                    name=family.name,
                    documentation=family.documentation,
                    unit=family.unit,
                )
            elif family.type == "counter":
                metric_family = CounterMetricFamily(
                    name=family.name,
                    documentation=family.documentation,
                    unit=family.unit,
                )
            elif family.type == "summary":
                metric_family = SummaryMetricFamily(
                    name=family.name,
                    documentation=family.documentation,
                    unit=family.unit,
                )
            elif family.type == "histogram":
                metric_family = HistogramMetricFamily(
                    name=family.name,
                    documentation=family.documentation,
                    unit=family.unit,
                )
            elif family.type == "info":
                metric_family = InfoMetricFamily(
                    name=family.name,
                    documentation=family.documentation,
                )
            elif family.type == "gaugehistogram":
                metric_family = GaugeHistogramMetricFamily(
                    name=family.name,
                    documentation=family.documentation,
                    unit=family.unit,
                )
            elif family.type == "stateset":
                metric_family = StateSetMetricFamily(
                    name=family.name,
                    documentation=family.documentation,
                )
            elif family.type == "unknown":
                metric_family = UnknownMetricFamily(
                    name=family.name,
                    documentation=family.documentation,
                    unit=family.unit,
                )
            else:
                continue

            for sample in family.samples:
                metric_family.add_sample(
                    sample.name,
                    value=sample.value,
                    labels=sample.labels,
                    timestamp=sample.timestamp,
                    exemplar=sample.exemplar,
                )
            yield metric_family

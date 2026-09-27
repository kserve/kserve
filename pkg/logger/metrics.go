/*
Copyright 2026 The KServe Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package logger

import "github.com/prometheus/client_golang/prometheus"

const (
	metricsNamespace = "kserve"
	metricsSubsystem = "logger"
)

var (
	EventsSentTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsSubsystem,
		Name:      "events_sent_total",
		Help:      "Total number of CloudEvents successfully delivered to the log sink (HTTP 200).",
	}, []string{"type"})

	EventsFailedTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsSubsystem,
		Name:      "events_failed_total",
		Help:      "Total number of CloudEvents that failed to deliver to the log sink (connection error, timeout, or non-200 response).",
	}, []string{"type"})

	// TODO: wire this in once #6173 (bounded dispatch) lands -- QueueLogRequest
	// doesn't have a drop path yet, so this stays at 0.
	EventsDroppedTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsSubsystem,
		Name:      "events_dropped_total",
		Help:      "Total number of log requests dropped before delivery because the work queue was full.",
	}, []string{"type"})

	DeliveryDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsSubsystem,
		Name:      "delivery_duration_seconds",
		Help:      "Time taken to deliver a CloudEvent to the log sink, regardless of outcome.",
		Buckets:   prometheus.DefBuckets,
	}, []string{"type"})

	WorkQueueDepth = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsSubsystem,
		Name:      "work_queue_depth",
		Help:      "Number of log requests currently buffered in the work queue, waiting for a worker.",
	})
)

func init() {
	prometheus.MustRegister(EventsSentTotal, EventsFailedTotal, EventsDroppedTotal, DeliveryDuration, WorkQueueDepth)
}

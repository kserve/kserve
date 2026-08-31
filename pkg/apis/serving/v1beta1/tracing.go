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

package v1beta1

import "k8s.io/utils/ptr"

const (
	DefaultTracingExporterEndpoint = "http://otel-collector:4317"
	DefaultTracingSampler          = "parentbased_traceidratio"
	DefaultTracingSamplerArg       = "0.05"
	DefaultTracingExporter         = "otlp"
)

// TracingSpec defines the distributed tracing configuration.
// When present (even as an empty object `{}`), tracing is enabled with defaults.
// When omitted, tracing is disabled.
type TracingSpec struct {
	// ExporterEndpoint is the OTLP exporter endpoint.
	// Maps to the OTEL_EXPORTER_OTLP_ENDPOINT environment variable.
	// Default: "http://otel-collector:4317"
	// +optional
	ExporterEndpoint *string `json:"exporterEndpoint,omitempty"`

	// Sampler specifies the sampler to use for traces.
	// Maps to the OTEL_TRACES_SAMPLER environment variable.
	// Default: "parentbased_traceidratio"
	// +optional
	Sampler *string `json:"sampler,omitempty"`

	// SamplerArg is an argument passed to the traces sampler, such as a sampling ratio.
	// Maps to the OTEL_TRACES_SAMPLER_ARG environment variable.
	// Default: "0.05"
	// +optional
	SamplerArg *string `json:"samplerArg,omitempty"`

	// Exporter specifies which exporter is used for traces.
	// Maps to the OTEL_TRACES_EXPORTER environment variable.
	// Default: "otlp"
	// +optional
	Exporter *string `json:"exporter,omitempty"`
}

func (t *TracingSpec) Default() {
	if t.ExporterEndpoint == nil {
		t.ExporterEndpoint = ptr.To(DefaultTracingExporterEndpoint)
	}
	if t.Sampler == nil {
		t.Sampler = ptr.To(DefaultTracingSampler)
	}
	if t.SamplerArg == nil {
		t.SamplerArg = ptr.To(DefaultTracingSamplerArg)
	}
	if t.Exporter == nil {
		t.Exporter = ptr.To(DefaultTracingExporter)
	}
}

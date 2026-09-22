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

package tracing

const (
	EnvOtelServiceName                = "OTEL_SERVICE_NAME"
	EnvOtelExporterEndpoint           = "OTEL_EXPORTER_OTLP_ENDPOINT"
	EnvOtelTracesExporter             = "OTEL_TRACES_EXPORTER"
	EnvOtelTracesSampler              = "OTEL_TRACES_SAMPLER"
	EnvOtelTracesSamplerArg           = "OTEL_TRACES_SAMPLER_ARG"
	EnvOtelResourceAttributes         = "OTEL_RESOURCE_ATTRIBUTES"
	EnvOtelResourceAttributesNodeName = "OTEL_RESOURCE_ATTRIBUTES_NODE_NAME"
	EnvOtelResourceAttributesPodName  = "OTEL_RESOURCE_ATTRIBUTES_POD_NAME"

	EnvMLServerTracingServer = "MLSERVER_TRACING_SERVER"
)

// HasArg checks whether any element in args starts with the given flag name.
// It handles both --flag=value and --flag value forms.
func HasArg(args []string, flag string) bool {
	for _, a := range args {
		if a == flag || len(a) > len(flag) && a[:len(flag)+1] == flag+"=" {
			return true
		}
	}
	return false
}

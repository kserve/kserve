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

package llmisvc

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/tracing"
	"github.com/kserve/kserve/pkg/utils"
)

const (
	defaultSchedulerServiceName = "inference-scheduler"
	defaultServerServiceName    = "inference-server"
)

// otelResourceAttributeEnvVars returns the standard k8s resource attribute env
// vars that are always injected when tracing is enabled. These use the downward
// API to populate node and pod names, then compose OTEL_RESOURCE_ATTRIBUTES.
func otelResourceAttributeEnvVars(namespace, llmisvcName string) []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name: tracing.EnvOtelResourceAttributesNodeName,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					APIVersion: "v1",
					FieldPath:  "spec.nodeName",
				},
			},
		},
		{
			Name: tracing.EnvOtelResourceAttributesPodName,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					APIVersion: "v1",
					FieldPath:  "metadata.name",
				},
			},
		},
		{
			Name: tracing.EnvOtelResourceAttributes,
			Value: "k8s.namespace.name=" + namespace +
				",k8s.node.name=$(" + tracing.EnvOtelResourceAttributesNodeName + ")" +
				",k8s.pod.name=$(" + tracing.EnvOtelResourceAttributesPodName + ")" +
				",llmisvc.name=" + llmisvcName,
		},
	}
}

// injectSchedulerTracing injects tracing args and env vars into the scheduler
// (EPP) deployment's main container. The scheduler uses the --tracing=true flag
// and standard OTEL_* env vars. Returns true if the container was mutated.
//
// TracingSpec field values are expected to be populated by the well-known config
// merge (kserve-config-llm-tracing) before this function is called.
func injectSchedulerTracing(t *v1alpha2.TracingSpec, namespace, llmisvcName string, container *corev1.Container) bool {
	if t == nil {
		return false
	}

	if !tracing.HasArg(container.Args, "--tracing") &&
		!tracing.HasArg(container.Args, "-tracing") {
		container.Args = append(container.Args, "--tracing=true")
	}

	resourceAttrs := otelResourceAttributeEnvVars(namespace, llmisvcName)
	tracingEnvVars := make([]corev1.EnvVar, 0, 5+len(resourceAttrs))
	tracingEnvVars = append(tracingEnvVars,
		corev1.EnvVar{Name: tracing.EnvOtelServiceName, Value: defaultSchedulerServiceName},
		corev1.EnvVar{Name: tracing.EnvOtelExporterEndpoint, Value: ptr.Deref(t.ExporterEndpoint, "")},
		corev1.EnvVar{Name: tracing.EnvOtelTracesExporter, Value: ptr.Deref(t.Exporter, "")},
		corev1.EnvVar{Name: tracing.EnvOtelTracesSampler, Value: ptr.Deref(t.Sampler, "")},
		corev1.EnvVar{Name: tracing.EnvOtelTracesSamplerArg, Value: ptr.Deref(t.SamplerArg, "")},
	)
	tracingEnvVars = append(tracingEnvVars, resourceAttrs...)

	container.Env = utils.AppendEnvVarIfNotExists(container.Env, tracingEnvVars...)
	return true
}

// injectServerTracing injects tracing args and env vars into the inference
// server (vLLM) deployment's main container. The server uses
// --otlp-traces-endpoint and --collect-detailed-traces args plus OTEL_* env vars.
// roleSuffix should be "-decode" or "-prefill". Returns true if the container was mutated.
//
// TracingSpec field values are expected to be populated by the well-known config
// merge (kserve-config-llm-tracing) before this function is called.
func injectServerTracing(t *v1alpha2.TracingSpec, namespace, llmisvcName, roleSuffix string, container *corev1.Container) bool {
	if t == nil {
		return false
	}

	endpoint := ptr.Deref(t.ExporterEndpoint, "")
	if endpoint == "" {
		return false
	}

	if !tracing.HasArg(container.Args, "--otlp-traces-endpoint") {
		container.Args = append(container.Args, "--otlp-traces-endpoint", endpoint)
	}

	if !tracing.HasArg(container.Args, "--collect-detailed-traces") {
		container.Args = append(container.Args, "--collect-detailed-traces", "all")
	}

	serviceName := defaultServerServiceName + roleSuffix
	resourceAttrs := otelResourceAttributeEnvVars(namespace, llmisvcName)

	tracingEnvVars := make([]corev1.EnvVar, 0, 5+len(resourceAttrs))
	tracingEnvVars = append(tracingEnvVars,
		corev1.EnvVar{Name: tracing.EnvOtelServiceName, Value: serviceName},
		corev1.EnvVar{Name: tracing.EnvOtelExporterEndpoint, Value: endpoint},
		corev1.EnvVar{Name: tracing.EnvOtelTracesExporter, Value: ptr.Deref(t.Exporter, "")},
		corev1.EnvVar{Name: tracing.EnvOtelTracesSampler, Value: ptr.Deref(t.Sampler, "")},
		corev1.EnvVar{Name: tracing.EnvOtelTracesSamplerArg, Value: ptr.Deref(t.SamplerArg, "")},
	)
	tracingEnvVars = append(tracingEnvVars, resourceAttrs...)

	container.Env = utils.AppendEnvVarIfNotExists(container.Env, tracingEnvVars...)
	return true
}

// injectServerTracingIntoPodSpec finds the "main" container in a PodSpec and
// injects server tracing into it. Returns true if the container was mutated.
func injectServerTracingIntoPodSpec(t *v1alpha2.TracingSpec, namespace, llmisvcName, roleSuffix string, podSpec *corev1.PodSpec) bool {
	for i := range podSpec.Containers {
		if podSpec.Containers[i].Name == "main" {
			return injectServerTracing(t, namespace, llmisvcName, roleSuffix, &podSpec.Containers[i])
		}
	}
	return false
}

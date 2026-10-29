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
	defaultSchedulerServiceName      = "inference-scheduler"
	defaultServerServiceName         = "inference-server"
	defaultRoutingSidecarServiceName = "routing-sidecar"
	defaultTokenizerServiceName      = "tokenizer"
	tracingConfigVersionEnvVar       = "KSERVE_TRACING_CONFIG_VERSION"
)

// New presets opt in to component tracing so controller upgrades leave old
// pinned Pod templates unchanged. A different injection contract needs a new version.
func supportsComponentTracing(container *corev1.Container) bool {
	version, _ := utils.GetEnvVarValue(container.Env, tracingConfigVersionEnvVar)
	return version == "1"
}

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
	return injectGoTracing(t, namespace, llmisvcName, defaultSchedulerServiceName, container)
}

func injectGoTracing(t *v1alpha2.TracingSpec, namespace, llmisvcName, serviceName string, container *corev1.Container) bool {
	if t == nil {
		return false
	}

	// Checking Command is opt-in too, so old presets keep their existing Args.
	commandOverride := supportsComponentTracing(container) &&
		(tracing.HasArg(container.Command, "--tracing") || tracing.HasArg(container.Command, "-tracing"))
	if !tracing.HasArg(container.Args, "--tracing") &&
		!tracing.HasArg(container.Args, "-tracing") && !commandOverride {
		container.Args = append(container.Args, "--tracing=true")
	}

	resourceAttrs := otelResourceAttributeEnvVars(namespace, llmisvcName)
	tracingEnvVars := make([]corev1.EnvVar, 0, 5+len(resourceAttrs))
	tracingEnvVars = append(tracingEnvVars,
		corev1.EnvVar{Name: tracing.EnvOtelServiceName, Value: serviceName},
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
	if !injectVLLMTracing(t, namespace, llmisvcName, defaultServerServiceName+roleSuffix, container) {
		return false
	}
	if !tracing.HasArg(container.Args, "--collect-detailed-traces") {
		container.Args = append(container.Args, "--collect-detailed-traces", "all")
	}
	return true
}

func injectVLLMTracing(t *v1alpha2.TracingSpec, namespace, llmisvcName, serviceName string, container *corev1.Container) bool {
	if t == nil {
		return false
	}

	endpoint := ptr.Deref(t.ExporterEndpoint, "")
	if endpoint == "" {
		return false
	}

	commandOverride := supportsComponentTracing(container) && tracing.HasArg(container.Command, "--otlp-traces-endpoint")
	if !tracing.HasArg(container.Args, "--otlp-traces-endpoint") && !commandOverride {
		container.Args = append(container.Args, "--otlp-traces-endpoint", endpoint)
	}

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

// injectServerTracingIntoPodSpec configures the inference server and routing sidecar.
func injectServerTracingIntoPodSpec(t *v1alpha2.TracingSpec, namespace, llmisvcName, roleSuffix string, podSpec *corev1.PodSpec) bool {
	mutated := false
	if sidecar := routingSidecar(podSpec); sidecar != nil && supportsComponentTracing(sidecar) {
		mutated = injectGoTracing(t, namespace, llmisvcName, defaultRoutingSidecarServiceName, sidecar)
	}
	for i := range podSpec.Containers {
		if podSpec.Containers[i].Name == "main" {
			return injectServerTracing(t, namespace, llmisvcName, roleSuffix, &podSpec.Containers[i]) || mutated
		}
	}
	return mutated
}

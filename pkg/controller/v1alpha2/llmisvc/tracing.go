/*
Copyright 2025 The KServe Authors.

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
	"slices"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
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
			Name: "OTEL_RESOURCE_ATTRIBUTES_NODE_NAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					APIVersion: "v1",
					FieldPath:  "spec.nodeName",
				},
			},
		},
		{
			Name: "OTEL_RESOURCE_ATTRIBUTES_POD_NAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					APIVersion: "v1",
					FieldPath:  "metadata.name",
				},
			},
		},
		{
			Name: "OTEL_RESOURCE_ATTRIBUTES",
			Value: "k8s.namespace.name=" + namespace +
				",k8s.node.name=$(OTEL_RESOURCE_ATTRIBUTES_NODE_NAME)" +
				",k8s.pod.name=$(OTEL_RESOURCE_ATTRIBUTES_POD_NAME)" +
				",llmisvc.name=" + llmisvcName,
		},
	}
}

// injectSchedulerTracing injects tracing args and env vars into the scheduler
// (EPP) deployment's main container. The scheduler uses the --tracing=true flag
// and standard OTEL_* env vars.
//
// TracingSpec field values are expected to be populated by the well-known config
// merge (kserve-config-llm-tracing) before this function is called.
func injectSchedulerTracing(t *v1alpha2.TracingSpec, namespace, llmisvcName string, container *corev1.Container) {
	if t == nil {
		return
	}

	if !slices.Contains(container.Args, "--tracing=true") &&
		!slices.Contains(container.Args, "-tracing=true") &&
		!slices.Contains(container.Args, "--tracing") {
		container.Args = append(container.Args, "--tracing=true")
	}

	resourceAttrs := otelResourceAttributeEnvVars(namespace, llmisvcName)
	tracingEnvVars := make([]corev1.EnvVar, 0, 5+len(resourceAttrs))
	tracingEnvVars = append(tracingEnvVars,
		corev1.EnvVar{Name: "OTEL_SERVICE_NAME", Value: defaultSchedulerServiceName},
		corev1.EnvVar{Name: "OTEL_EXPORTER_OTLP_ENDPOINT", Value: ptr.Deref(t.ExporterEndpoint, "")},
		corev1.EnvVar{Name: "OTEL_TRACES_EXPORTER", Value: ptr.Deref(t.Exporter, "")},
		corev1.EnvVar{Name: "OTEL_TRACES_SAMPLER", Value: ptr.Deref(t.Sampler, "")},
		corev1.EnvVar{Name: "OTEL_TRACES_SAMPLER_ARG", Value: ptr.Deref(t.SamplerArg, "")},
	)
	tracingEnvVars = append(tracingEnvVars, resourceAttrs...)

	container.Env = mergeEnvVars(container.Env, tracingEnvVars)
}

// injectServerTracing injects tracing args and env vars into the inference
// server (vLLM) deployment's main container. The server uses
// --otlp-traces-endpoint and --collect-detailed-traces args plus OTEL_* env vars.
// roleSuffix should be "-decode" or "-prefill".
//
// TracingSpec field values are expected to be populated by the well-known config
// merge (kserve-config-llm-tracing) before this function is called.
func injectServerTracing(t *v1alpha2.TracingSpec, namespace, llmisvcName, roleSuffix string, container *corev1.Container) {
	if t == nil {
		return
	}

	endpoint := ptr.Deref(t.ExporterEndpoint, "")
	if endpoint == "" {
		return
	}

	if !hasArg(container.Args, "--otlp-traces-endpoint") {
		container.Args = append(container.Args, "--otlp-traces-endpoint", endpoint)
	}

	if !hasArg(container.Args, "--collect-detailed-traces") {
		container.Args = append(container.Args, "--collect-detailed-traces", "all")
	}

	serviceName := defaultServerServiceName + roleSuffix
	resourceAttrs := otelResourceAttributeEnvVars(namespace, llmisvcName)

	tracingEnvVars := make([]corev1.EnvVar, 0, 5+len(resourceAttrs))
	tracingEnvVars = append(tracingEnvVars,
		corev1.EnvVar{Name: "OTEL_SERVICE_NAME", Value: serviceName},
		corev1.EnvVar{Name: "OTEL_EXPORTER_OTLP_ENDPOINT", Value: endpoint},
		corev1.EnvVar{Name: "OTEL_TRACES_EXPORTER", Value: ptr.Deref(t.Exporter, "")},
		corev1.EnvVar{Name: "OTEL_TRACES_SAMPLER", Value: ptr.Deref(t.Sampler, "")},
		corev1.EnvVar{Name: "OTEL_TRACES_SAMPLER_ARG", Value: ptr.Deref(t.SamplerArg, "")},
	)
	tracingEnvVars = append(tracingEnvVars, resourceAttrs...)

	container.Env = mergeEnvVars(container.Env, tracingEnvVars)
}

// injectServerTracingIntoPodSpec finds the "main" container in a PodSpec and
// injects server tracing into it. This is a convenience wrapper for multi-node
// workloads where the caller works with PodSpecs directly.
func injectServerTracingIntoPodSpec(t *v1alpha2.TracingSpec, namespace, llmisvcName, roleSuffix string, podSpec *corev1.PodSpec) {
	for i := range podSpec.Containers {
		if podSpec.Containers[i].Name == "main" {
			injectServerTracing(t, namespace, llmisvcName, roleSuffix, &podSpec.Containers[i])
			return
		}
	}
}

// mergeEnvVars appends env vars from src into dst, skipping any that already
// exist in dst (by name). This ensures user-provided env vars take precedence.
func mergeEnvVars(dst, src []corev1.EnvVar) []corev1.EnvVar {
	existing := make(map[string]struct{}, len(dst))
	for _, e := range dst {
		existing[e.Name] = struct{}{}
	}
	for _, e := range src {
		if _, ok := existing[e.Name]; !ok {
			dst = append(dst, e)
		}
	}
	return dst
}

// hasArg checks whether any element in args starts with the given flag name.
// It handles both --flag=value and --flag value forms.
func hasArg(args []string, flag string) bool {
	for _, a := range args {
		if a == flag || len(a) > len(flag) && a[:len(flag)+1] == flag+"=" {
			return true
		}
	}
	return false
}

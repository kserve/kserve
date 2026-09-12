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

package utils

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/tracing"
	"github.com/kserve/kserve/pkg/utils"
)

func otelResourceAttributeEnvVars(namespace, isvcName, component, variant string) []corev1.EnvVar {
	attrs := "k8s.namespace.name=" + namespace +
		",k8s.node.name=$(" + tracing.EnvOtelResourceAttributesNodeName + ")" +
		",k8s.pod.name=$(" + tracing.EnvOtelResourceAttributesPodName + ")" +
		",isvc.name=" + isvcName +
		",isvc.component=" + component
	if variant != "" {
		attrs += ",isvc.predictor.variant=" + variant
	}

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
			Name:  tracing.EnvOtelResourceAttributes,
			Value: attrs,
		},
	}
}

// InjectPredictorTracing injects OTEL tracing env vars and runtime-specific
// configuration into a predictor container. Returns true if the container was
// mutated.
//
// variant identifies the canary variant name (empty for the primary predictor).
// serverType is the serving runtime type (e.g. constants.ServerTypeVLLMServer).
func InjectPredictorTracing(t *v1beta1.TracingSpec, namespace, isvcName, variant, serverType string, container *corev1.Container) bool {
	if t == nil {
		return false
	}

	endpoint := ptr.Deref(t.ExporterEndpoint, "")
	resourceAttrs := otelResourceAttributeEnvVars(namespace, isvcName, "predictor", variant)
	tracingEnvVars := make([]corev1.EnvVar, 0, 6+len(resourceAttrs))
	tracingEnvVars = append(tracingEnvVars,
		corev1.EnvVar{Name: tracing.EnvOtelServiceName, Value: isvcName + "-predictor"},
		corev1.EnvVar{Name: tracing.EnvOtelExporterEndpoint, Value: endpoint},
		corev1.EnvVar{Name: tracing.EnvOtelTracesExporter, Value: ptr.Deref(t.Exporter, "")},
		corev1.EnvVar{Name: tracing.EnvOtelTracesSampler, Value: ptr.Deref(t.Sampler, "")},
		corev1.EnvVar{Name: tracing.EnvOtelTracesSamplerArg, Value: ptr.Deref(t.SamplerArg, "")},
	)
	tracingEnvVars = append(tracingEnvVars, resourceAttrs...)

	switch serverType {
	case constants.ServerTypeVLLMServer:
		if !tracing.HasArg(container.Args, "--otlp-traces-endpoint") {
			container.Args = append(container.Args, "--otlp-traces-endpoint", endpoint)
		}
		if !tracing.HasArg(container.Args, "--collect-detailed-traces") {
			container.Args = append(container.Args, "--collect-detailed-traces", "all")
		}
	case constants.ServerTypeMLServer:
		tracingEnvVars = append(tracingEnvVars,
			corev1.EnvVar{Name: tracing.EnvMLServerTracingServer, Value: endpoint},
		)
	}

	container.Env = utils.AppendEnvVarIfNotExists(container.Env, tracingEnvVars...)
	return true
}

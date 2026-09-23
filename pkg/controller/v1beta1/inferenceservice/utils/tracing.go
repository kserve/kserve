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

// otelResourceAttributeEnvVars returns the environment variables that configure
// OTEL resource attributes for an InferenceService component. When variant is
// non-empty, it is included as the predictor canary variant attribute.
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

// injectTracingEnvVars injects the OTEL environment shared by all traced
// InferenceService components into the supplied container. The tracing
// specification must be non-nil.
func injectTracingEnvVars(t *v1beta1.TracingSpec, namespace, isvcName, component, variant string, container *corev1.Container) {
	endpoint := ptr.Deref(t.ExporterEndpoint, "")
	resourceAttrs := otelResourceAttributeEnvVars(namespace, isvcName, component, variant)
	tracingEnvVars := make([]corev1.EnvVar, 0, 5+len(resourceAttrs))
	tracingEnvVars = append(tracingEnvVars,
		corev1.EnvVar{Name: tracing.EnvOtelServiceName, Value: isvcName + "-" + component},
		corev1.EnvVar{Name: tracing.EnvOtelExporterEndpoint, Value: endpoint},
		corev1.EnvVar{Name: tracing.EnvOtelTracesExporter, Value: ptr.Deref(t.Exporter, "")},
		corev1.EnvVar{Name: tracing.EnvOtelTracesSampler, Value: ptr.Deref(t.Sampler, "")},
		corev1.EnvVar{Name: tracing.EnvOtelTracesSamplerArg, Value: ptr.Deref(t.SamplerArg, "")},
	)
	tracingEnvVars = append(tracingEnvVars, resourceAttrs...)
	container.Env = utils.AppendEnvVarIfNotExists(container.Env, tracingEnvVars...)
}

// injectPredictorTracing adds tracing configuration specific to the predictor's
// serving runtime. Runtimes without runtime-specific tracing configuration are
// left unchanged.
func injectPredictorTracing(t *v1beta1.TracingSpec, isvcName, serverType string, container *corev1.Container) {
	endpoint := ptr.Deref(t.ExporterEndpoint, "")
	switch serverType {
	case constants.ServerTypeVLLMServer:
		if !tracing.HasArg(container.Args, "--otlp-traces-endpoint") {
			container.Args = append(container.Args, "--otlp-traces-endpoint", endpoint)
		}
		if !tracing.HasArg(container.Args, "--collect-detailed-traces") {
			container.Args = append(container.Args, "--collect-detailed-traces", "all")
		}
	case constants.ServerTypeTritonServer:
		if !tracing.HasArg(container.Args, "--trace-config") {
			container.Args = append(container.Args,
				"--trace-config", "mode=opentelemetry",
				"--trace-config", "opentelemetry,url="+endpoint,
				"--trace-config", "opentelemetry,resource=service.name="+isvcName+"-predictor",
				"--trace-config", "level=TIMESTAMPS",
			)
		}
	case constants.ServerTypeMLServer:
		container.Env = utils.AppendEnvVarIfNotExists(container.Env,
			corev1.EnvVar{Name: tracing.EnvMLServerTracingServer, Value: endpoint},
		)
	}
}

// InjectComponentTracing injects OTEL tracing environment variables and, for
// predictors, runtime-specific configuration. When component is empty, it is
// inferred from the well-known predictor and transformer container names. An
// unrecognized container name in that case is left unchanged and causes the
// function to return false. The function returns false when tracing is disabled
// and true after tracing has been injected.
//
// variant identifies the canary variant name (empty for the primary predictor).
// serverType is the serving runtime type (e.g. constants.ServerTypeVLLMServer).
// component identifies the InferenceService component to configure when it is
// not empty.
func InjectComponentTracing(t *v1beta1.TracingSpec, namespace, isvcName, variant, serverType, component string, container *corev1.Container) bool {
	if t == nil {
		return false
	}

	if component == "" {
		switch container.Name {
		case constants.InferenceServiceContainerName:
			component = string(v1beta1.PredictorComponent)
		case constants.TransformerContainerName:
			component = string(v1beta1.TransformerComponent)
		default:
			return false
		}
	}

	injectTracingEnvVars(t, namespace, isvcName, component, variant, container)

	if component == string(v1beta1.PredictorComponent) {
		injectPredictorTracing(t, isvcName, serverType, container)
	}

	return true
}

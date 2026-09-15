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
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/tracing"
)

func envToMap(envVars []corev1.EnvVar) map[string]string {
	m := make(map[string]string, len(envVars))
	for _, e := range envVars {
		m[e.Name] = e.Value
	}
	return m
}

func TestInjectPredictorTracing(t *testing.T) {
	tests := []struct {
		name       string
		spec       *v1beta1.TracingSpec
		namespace  string
		isvcName   string
		variant    string
		serverType string
		container  *corev1.Container
		wantMut    bool
		check      func(g *GomegaWithT, c *corev1.Container)
	}{
		{
			name:      "nil TracingSpec returns false",
			spec:      nil,
			container: &corev1.Container{Name: constants.InferenceServiceContainerName},
			wantMut:   false,
			check: func(g *GomegaWithT, c *corev1.Container) {
				g.Expect(c.Env).To(BeEmpty())
				g.Expect(c.Args).To(BeEmpty())
			},
		},
		{
			name:      "populated TracingSpec sets all OTEL env vars",
			spec:      fullTracingSpec(),
			namespace: "prod-ns",
			isvcName:  "llm-prod",
			container: &corev1.Container{Name: constants.InferenceServiceContainerName},
			wantMut:   true,
			check: func(g *GomegaWithT, c *corev1.Container) {
				envMap := envToMap(c.Env)
				g.Expect(envMap).To(HaveKeyWithValue(tracing.EnvOtelServiceName, "llm-prod-predictor"))
				g.Expect(envMap).To(HaveKeyWithValue(tracing.EnvOtelExporterEndpoint, "http://collector:4317"))
				g.Expect(envMap).To(HaveKeyWithValue(tracing.EnvOtelTracesExporter, "otlp"))
				g.Expect(envMap).To(HaveKeyWithValue(tracing.EnvOtelTracesSampler, "parentbased_traceidratio"))
				g.Expect(envMap).To(HaveKeyWithValue(tracing.EnvOtelTracesSamplerArg, "0.1"))
			},
		},
		{
			name:       "vLLM predictor with endpoint injects CLI args",
			spec:       fullTracingSpec(),
			namespace:  "ns",
			isvcName:   "vllm-isvc",
			serverType: constants.ServerTypeVLLMServer,
			container:  &corev1.Container{Name: constants.InferenceServiceContainerName},
			wantMut:    true,
			check: func(g *GomegaWithT, c *corev1.Container) {
				g.Expect(c.Args).To(ContainElements("--otlp-traces-endpoint", "http://collector:4317"))
				g.Expect(c.Args).To(ContainElements("--collect-detailed-traces", "all"))
			},
		},
		{
			name:       "non-vLLM predictor gets env vars only",
			spec:       fullTracingSpec(),
			namespace:  "ns",
			isvcName:   "triton-isvc",
			serverType: "triton",
			container:  &corev1.Container{Name: constants.InferenceServiceContainerName},
			wantMut:    true,
			check: func(g *GomegaWithT, c *corev1.Container) {
				g.Expect(c.Args).To(BeEmpty())
				envMap := envToMap(c.Env)
				g.Expect(envMap).To(HaveKey(tracing.EnvOtelServiceName))
				g.Expect(envMap).NotTo(HaveKey(tracing.EnvMLServerTracingServer))
			},
		},
		{
			name:       "MLServer predictor with endpoint injects MLSERVER_TRACING_SERVER and OTEL env vars",
			spec:       fullTracingSpec(),
			namespace:  "ns",
			isvcName:   "mlserver-isvc",
			serverType: constants.ServerTypeMLServer,
			container:  &corev1.Container{Name: constants.InferenceServiceContainerName},
			wantMut:    true,
			check: func(g *GomegaWithT, c *corev1.Container) {
				envMap := envToMap(c.Env)
				g.Expect(envMap).To(HaveKeyWithValue(tracing.EnvMLServerTracingServer, "http://collector:4317"))
				g.Expect(envMap).To(HaveKeyWithValue(tracing.EnvOtelServiceName, "mlserver-isvc-predictor"))
				g.Expect(envMap).To(HaveKeyWithValue(tracing.EnvOtelExporterEndpoint, "http://collector:4317"))
				g.Expect(envMap).To(HaveKeyWithValue(tracing.EnvOtelTracesExporter, "otlp"))
				g.Expect(envMap).To(HaveKeyWithValue(tracing.EnvOtelTracesSampler, "parentbased_traceidratio"))
				g.Expect(envMap).To(HaveKeyWithValue(tracing.EnvOtelTracesSamplerArg, "0.1"))
				g.Expect(envMap).To(HaveKey(tracing.EnvOtelResourceAttributes))
				g.Expect(c.Args).To(BeEmpty())
			},
		},
		{
			name:      "canary variant included in resource attributes",
			spec:      fullTracingSpec(),
			namespace: "ns",
			isvcName:  "my-isvc",
			variant:   "canary-v2",
			container: &corev1.Container{Name: constants.InferenceServiceContainerName},
			wantMut:   true,
			check: func(g *GomegaWithT, c *corev1.Container) {
				envMap := envToMap(c.Env)
				g.Expect(envMap[tracing.EnvOtelResourceAttributes]).To(ContainSubstring("isvc.predictor.variant=canary-v2"))
			},
		},
		{
			name:      "primary predictor has no variant in resource attributes",
			spec:      fullTracingSpec(),
			namespace: "ns",
			isvcName:  "my-isvc",
			variant:   "",
			container: &corev1.Container{Name: constants.InferenceServiceContainerName},
			wantMut:   true,
			check: func(g *GomegaWithT, c *corev1.Container) {
				envMap := envToMap(c.Env)
				g.Expect(envMap[tracing.EnvOtelResourceAttributes]).NotTo(ContainSubstring("isvc.predictor.variant"))
			},
		},
		{
			name:      "user-set OTEL env var not overwritten",
			spec:      fullTracingSpec(),
			namespace: "ns",
			isvcName:  "my-isvc",
			container: &corev1.Container{
				Name: constants.InferenceServiceContainerName,
				Env: []corev1.EnvVar{
					{Name: tracing.EnvOtelServiceName, Value: "user-override"},
				},
			},
			wantMut: true,
			check: func(g *GomegaWithT, c *corev1.Container) {
				envMap := envToMap(c.Env)
				g.Expect(envMap).To(HaveKeyWithValue(tracing.EnvOtelServiceName, "user-override"))
				g.Expect(envMap).To(HaveKey(tracing.EnvOtelExporterEndpoint))
			},
		},
		{
			name:       "idempotent re-injection does not duplicate env vars or args",
			spec:       fullTracingSpec(),
			namespace:  "ns",
			isvcName:   "my-isvc",
			serverType: constants.ServerTypeVLLMServer,
			container:  &corev1.Container{Name: constants.InferenceServiceContainerName},
			wantMut:    true,
			check: func(g *GomegaWithT, c *corev1.Container) {
				envCountBefore := len(c.Env)
				argsCountBefore := len(c.Args)

				InjectComponentTracing(fullTracingSpec(), "ns", "my-isvc", "", constants.ServerTypeVLLMServer, "", c)

				g.Expect(c.Env).To(HaveLen(envCountBefore))
				g.Expect(c.Args).To(HaveLen(argsCountBefore))
			},
		},
		{
			name:      "unrecognized sidecar is not modified",
			spec:      fullTracingSpec(),
			namespace: "ns",
			isvcName:  "my-isvc",
			container: &corev1.Container{
				Name: "logging-sidecar",
				Args: []string{"--log-level=info"},
				Env:  []corev1.EnvVar{{Name: "USER_ENV", Value: "preserved"}},
			},
			wantMut: false,
			check: func(g *GomegaWithT, c *corev1.Container) {
				g.Expect(c.Args).To(Equal([]string{"--log-level=info"}))
				g.Expect(c.Env).To(Equal([]corev1.EnvVar{{Name: "USER_ENV", Value: "preserved"}}))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			mutated := InjectComponentTracing(tt.spec, tt.namespace, tt.isvcName, tt.variant, tt.serverType, "", tt.container)
			g.Expect(mutated).To(Equal(tt.wantMut))
			tt.check(g, tt.container)
		})
	}
}

func TestInjectTransformerTracing(t *testing.T) {
	g := NewGomegaWithT(t)
	container := &corev1.Container{
		Name: "transformer",
		Args: []string{"--predictor_host", "predictor.default"},
		Env:  []corev1.EnvVar{{Name: "USER_ENV", Value: "preserved"}},
	}

	mutated := InjectComponentTracing(fullTracingSpec(), "prod-ns", "llm-prod", "canary-v2", constants.ServerTypeVLLMServer, string(v1beta1.TransformerComponent), container)

	g.Expect(mutated).To(BeTrue())
	envMap := envToMap(container.Env)
	g.Expect(envMap).To(HaveKeyWithValue(tracing.EnvOtelServiceName, "llm-prod-transformer"))
	g.Expect(envMap).To(HaveKeyWithValue(tracing.EnvOtelExporterEndpoint, "http://collector:4317"))
	g.Expect(envMap).To(HaveKeyWithValue(tracing.EnvOtelTracesExporter, "otlp"))
	g.Expect(envMap).To(HaveKeyWithValue(tracing.EnvOtelTracesSampler, "parentbased_traceidratio"))
	g.Expect(envMap).To(HaveKeyWithValue(tracing.EnvOtelTracesSamplerArg, "0.1"))
	g.Expect(envMap[tracing.EnvOtelResourceAttributes]).To(ContainSubstring("isvc.component=transformer"))
	g.Expect(envMap[tracing.EnvOtelResourceAttributes]).To(ContainSubstring("isvc.predictor.variant=canary-v2"))
	g.Expect(envMap).To(HaveKeyWithValue("USER_ENV", "preserved"))
	g.Expect(envMap).NotTo(HaveKey(tracing.EnvMLServerTracingServer))
	g.Expect(container.Args).To(Equal([]string{"--predictor_host", "predictor.default"}))
}

func TestInjectTransformerTracing_NilSpec(t *testing.T) {
	g := NewGomegaWithT(t)
	container := &corev1.Container{Name: "transformer"}

	mutated := InjectComponentTracing(nil, "ns", "isvc", "", "", string(v1beta1.TransformerComponent), container)

	g.Expect(mutated).To(BeFalse())
	g.Expect(container.Env).To(BeEmpty())
	g.Expect(container.Args).To(BeEmpty())
}

func TestOtelResourceAttributeEnvVars(t *testing.T) {
	g := NewGomegaWithT(t)
	envVars := otelResourceAttributeEnvVars("my-namespace", "my-isvc", "predictor", "")

	g.Expect(envVars).To(HaveLen(3))

	g.Expect(envVars[0].Name).To(Equal(tracing.EnvOtelResourceAttributesNodeName))
	g.Expect(envVars[0].ValueFrom.FieldRef.FieldPath).To(Equal("spec.nodeName"))

	g.Expect(envVars[1].Name).To(Equal(tracing.EnvOtelResourceAttributesPodName))
	g.Expect(envVars[1].ValueFrom.FieldRef.FieldPath).To(Equal("metadata.name"))

	g.Expect(envVars[2].Name).To(Equal(tracing.EnvOtelResourceAttributes))
	g.Expect(envVars[2].Value).To(ContainSubstring("k8s.namespace.name=my-namespace"))
	g.Expect(envVars[2].Value).To(ContainSubstring("isvc.name=my-isvc"))
	g.Expect(envVars[2].Value).To(ContainSubstring("isvc.component=predictor"))
	g.Expect(envVars[2].Value).NotTo(ContainSubstring("isvc.predictor.variant"))
}

func TestOtelResourceAttributeEnvVars_WithVariant(t *testing.T) {
	g := NewGomegaWithT(t)
	envVars := otelResourceAttributeEnvVars("ns", "isvc", "predictor", "canary-v1")

	g.Expect(envVars[2].Value).To(ContainSubstring("isvc.predictor.variant=canary-v1"))
}

func fullTracingSpec() *v1beta1.TracingSpec {
	return &v1beta1.TracingSpec{
		ExporterEndpoint: ptr.To("http://collector:4317"),
		Sampler:          ptr.To("parentbased_traceidratio"),
		SamplerArg:       ptr.To("0.1"),
		Exporter:         ptr.To("otlp"),
	}
}

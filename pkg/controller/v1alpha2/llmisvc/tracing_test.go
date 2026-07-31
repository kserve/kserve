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
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

func TestInjectSchedulerTracing_NilSpec(t *testing.T) {
	g := NewGomegaWithT(t)
	container := &corev1.Container{Name: "main"}
	mutated := injectSchedulerTracing(nil, "ns", "my-svc", container)
	g.Expect(mutated).To(BeFalse())
	g.Expect(container.Args).To(BeEmpty())
	g.Expect(container.Env).To(BeEmpty())
}

func TestInjectSchedulerTracing_EmptySpec(t *testing.T) {
	g := NewGomegaWithT(t)
	container := &corev1.Container{Name: "main"}
	mutated := injectSchedulerTracing(&v1alpha2.TracingSpec{}, "test-ns", "my-svc", container)
	g.Expect(mutated).To(BeTrue())

	g.Expect(container.Args).To(ContainElement("--tracing=true"))

	envMap := envToMap(container.Env)
	g.Expect(envMap).To(HaveKeyWithValue("OTEL_SERVICE_NAME", defaultSchedulerServiceName))
	g.Expect(envMap).To(HaveKeyWithValue("OTEL_EXPORTER_OTLP_ENDPOINT", ""))
	g.Expect(envMap).To(HaveKeyWithValue("OTEL_TRACES_EXPORTER", ""))
	g.Expect(envMap).To(HaveKeyWithValue("OTEL_TRACES_SAMPLER", ""))
	g.Expect(envMap).To(HaveKeyWithValue("OTEL_TRACES_SAMPLER_ARG", ""))
	g.Expect(envMap).To(HaveKey("OTEL_RESOURCE_ATTRIBUTES"))
	g.Expect(envMap["OTEL_RESOURCE_ATTRIBUTES"]).To(ContainSubstring("k8s.namespace.name=test-ns"))
	g.Expect(envMap["OTEL_RESOURCE_ATTRIBUTES"]).To(ContainSubstring("llmisvc.name=my-svc"))
}

func TestInjectSchedulerTracing_CustomFields(t *testing.T) {
	g := NewGomegaWithT(t)
	container := &corev1.Container{Name: "main"}
	spec := &v1alpha2.TracingSpec{
		ExporterEndpoint: ptr.To("http://collector:4317"),
		Sampler:          ptr.To("parentbased_traceidratio"),
		SamplerArg:       ptr.To("0.1"),
		Exporter:         ptr.To("otlp"),
	}
	injectSchedulerTracing(spec, "prod-ns", "llm-prod", container)

	envMap := envToMap(container.Env)
	g.Expect(envMap).To(HaveKeyWithValue("OTEL_SERVICE_NAME", defaultSchedulerServiceName))
	g.Expect(envMap).To(HaveKeyWithValue("OTEL_EXPORTER_OTLP_ENDPOINT", "http://collector:4317"))
	g.Expect(envMap).To(HaveKeyWithValue("OTEL_TRACES_SAMPLER", "parentbased_traceidratio"))
	g.Expect(envMap).To(HaveKeyWithValue("OTEL_TRACES_SAMPLER_ARG", "0.1"))
	g.Expect(envMap).To(HaveKeyWithValue("OTEL_TRACES_EXPORTER", "otlp"))
}

func TestInjectSchedulerTracing_DoesNotDuplicateTracingArg(t *testing.T) {
	g := NewGomegaWithT(t)
	container := &corev1.Container{
		Name: "main",
		Args: []string{"--tracing=true"},
	}
	injectSchedulerTracing(&v1alpha2.TracingSpec{}, "ns", "svc", container)

	count := 0
	for _, a := range container.Args {
		if a == "--tracing=true" {
			count++
		}
	}
	g.Expect(count).To(Equal(1))
}

func TestInjectSchedulerTracing_PreservesExistingEnv(t *testing.T) {
	g := NewGomegaWithT(t)
	container := &corev1.Container{
		Name: "main",
		Env: []corev1.EnvVar{
			{Name: "OTEL_SERVICE_NAME", Value: "user-override"},
		},
	}
	injectSchedulerTracing(&v1alpha2.TracingSpec{}, "ns", "svc", container)

	envMap := envToMap(container.Env)
	g.Expect(envMap).To(HaveKeyWithValue("OTEL_SERVICE_NAME", "user-override"))
}

func TestInjectServerTracing_NilSpec(t *testing.T) {
	g := NewGomegaWithT(t)
	container := &corev1.Container{Name: "main"}
	mutated := injectServerTracing(nil, "ns", "svc", "-decode", container)
	g.Expect(mutated).To(BeFalse())
	g.Expect(container.Args).To(BeEmpty())
	g.Expect(container.Env).To(BeEmpty())
}

func TestInjectServerTracing_EmptySpec_NoEndpoint_Skips(t *testing.T) {
	g := NewGomegaWithT(t)
	container := &corev1.Container{Name: "main"}
	mutated := injectServerTracing(&v1alpha2.TracingSpec{}, "test-ns", "my-llm", "-decode", container)
	g.Expect(mutated).To(BeFalse())
	g.Expect(container.Args).To(BeEmpty())
	g.Expect(container.Env).To(BeEmpty())
}

func TestInjectServerTracing_WithEndpoint_Decode(t *testing.T) {
	g := NewGomegaWithT(t)
	container := &corev1.Container{Name: "main"}
	mutated := injectServerTracing(&v1alpha2.TracingSpec{
		ExporterEndpoint: ptr.To("http://collector:4317"),
	}, "test-ns", "my-llm", "-decode", container)
	g.Expect(mutated).To(BeTrue())

	g.Expect(container.Args).To(ContainElements("--otlp-traces-endpoint", "http://collector:4317"))
	g.Expect(container.Args).To(ContainElements("--collect-detailed-traces", "all"))

	envMap := envToMap(container.Env)
	g.Expect(envMap).To(HaveKeyWithValue("OTEL_SERVICE_NAME", defaultServerServiceName+"-decode"))
	g.Expect(envMap).To(HaveKey("OTEL_RESOURCE_ATTRIBUTES"))
	g.Expect(envMap["OTEL_RESOURCE_ATTRIBUTES"]).To(ContainSubstring("llmisvc.name=my-llm"))
}

func TestInjectServerTracing_WithEndpoint_Prefill(t *testing.T) {
	g := NewGomegaWithT(t)
	container := &corev1.Container{Name: "main"}
	injectServerTracing(&v1alpha2.TracingSpec{
		ExporterEndpoint: ptr.To("http://collector:4317"),
	}, "test-ns", "my-llm", "-prefill", container)

	envMap := envToMap(container.Env)
	g.Expect(envMap).To(HaveKeyWithValue("OTEL_SERVICE_NAME", defaultServerServiceName+"-prefill"))
}

func TestInjectServerTracing_CustomEndpoint(t *testing.T) {
	g := NewGomegaWithT(t)
	container := &corev1.Container{Name: "main"}
	spec := &v1alpha2.TracingSpec{
		ExporterEndpoint: ptr.To("http://my-collector:4317"),
		Sampler:          ptr.To("always_on"),
		SamplerArg:       ptr.To("1.0"),
		Exporter:         ptr.To("otlp"),
	}
	injectServerTracing(spec, "prod", "llm-prod", "-decode", container)

	g.Expect(container.Args).To(ContainElements("--otlp-traces-endpoint", "http://my-collector:4317"))

	envMap := envToMap(container.Env)
	g.Expect(envMap).To(HaveKeyWithValue("OTEL_EXPORTER_OTLP_ENDPOINT", "http://my-collector:4317"))
	g.Expect(envMap).To(HaveKeyWithValue("OTEL_TRACES_SAMPLER", "always_on"))
	g.Expect(envMap).To(HaveKeyWithValue("OTEL_TRACES_SAMPLER_ARG", "1.0"))
	g.Expect(envMap).To(HaveKeyWithValue("OTEL_TRACES_EXPORTER", "otlp"))
	g.Expect(envMap).To(HaveKeyWithValue("OTEL_SERVICE_NAME", defaultServerServiceName+"-decode"))
}

func TestInjectServerTracing_DoesNotDuplicateArgs(t *testing.T) {
	g := NewGomegaWithT(t)
	container := &corev1.Container{
		Name: "main",
		Args: []string{"--otlp-traces-endpoint", "http://existing:4317", "--collect-detailed-traces", "model"},
	}
	injectServerTracing(&v1alpha2.TracingSpec{
		ExporterEndpoint: ptr.To("http://new:4317"),
	}, "ns", "svc", "-decode", container)

	count := 0
	for _, a := range container.Args {
		if a == "--otlp-traces-endpoint" {
			count++
		}
	}
	g.Expect(count).To(Equal(1))
	g.Expect(container.Args[1]).To(Equal("http://existing:4317"))
}

func TestInjectServerTracing_PreservesExistingEnv(t *testing.T) {
	g := NewGomegaWithT(t)
	container := &corev1.Container{
		Name: "main",
		Env: []corev1.EnvVar{
			{Name: "OTEL_TRACES_SAMPLER", Value: "user-sampler"},
		},
	}
	injectServerTracing(&v1alpha2.TracingSpec{
		Sampler: ptr.To("injected-sampler"),
	}, "ns", "svc", "-decode", container)

	envMap := envToMap(container.Env)
	g.Expect(envMap).To(HaveKeyWithValue("OTEL_TRACES_SAMPLER", "user-sampler"))
}

func TestInjectServerTracingIntoPodSpec_FindsMainContainer(t *testing.T) {
	g := NewGomegaWithT(t)
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{Name: "sidecar"},
			{Name: "main"},
		},
	}
	mutated := injectServerTracingIntoPodSpec(&v1alpha2.TracingSpec{
		ExporterEndpoint: ptr.To("http://collector:4317"),
	}, "ns", "svc", "-decode", podSpec)
	g.Expect(mutated).To(BeTrue())

	g.Expect(podSpec.Containers[0].Env).To(BeEmpty())
	g.Expect(podSpec.Containers[1].Env).NotTo(BeEmpty())

	envMap := envToMap(podSpec.Containers[1].Env)
	g.Expect(envMap).To(HaveKeyWithValue("OTEL_SERVICE_NAME", defaultServerServiceName+"-decode"))
}

func TestInjectServerTracingIntoPodSpec_NoMainContainer(t *testing.T) {
	g := NewGomegaWithT(t)
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{Name: "sidecar"},
		},
	}
	mutated := injectServerTracingIntoPodSpec(&v1alpha2.TracingSpec{}, "ns", "svc", "-decode", podSpec)
	g.Expect(mutated).To(BeFalse())
	g.Expect(podSpec.Containers[0].Env).To(BeEmpty())
}

func TestOtelResourceAttributeEnvVars(t *testing.T) {
	g := NewGomegaWithT(t)
	envVars := otelResourceAttributeEnvVars("my-namespace", "my-service")

	g.Expect(envVars).To(HaveLen(3))

	g.Expect(envVars[0].Name).To(Equal("OTEL_RESOURCE_ATTRIBUTES_NODE_NAME"))
	g.Expect(envVars[0].ValueFrom.FieldRef.FieldPath).To(Equal("spec.nodeName"))

	g.Expect(envVars[1].Name).To(Equal("OTEL_RESOURCE_ATTRIBUTES_POD_NAME"))
	g.Expect(envVars[1].ValueFrom.FieldRef.FieldPath).To(Equal("metadata.name"))

	g.Expect(envVars[2].Name).To(Equal("OTEL_RESOURCE_ATTRIBUTES"))
	g.Expect(envVars[2].Value).To(ContainSubstring("k8s.namespace.name=my-namespace"))
	g.Expect(envVars[2].Value).To(ContainSubstring("llmisvc.name=my-service"))
	g.Expect(envVars[2].Value).To(ContainSubstring("k8s.node.name=$(OTEL_RESOURCE_ATTRIBUTES_NODE_NAME)"))
	g.Expect(envVars[2].Value).To(ContainSubstring("k8s.pod.name=$(OTEL_RESOURCE_ATTRIBUTES_POD_NAME)"))
}

func TestMergeEnvVars(t *testing.T) {
	g := NewGomegaWithT(t)

	dst := []corev1.EnvVar{
		{Name: "EXISTING", Value: "keep"},
		{Name: "OTEL_SERVICE_NAME", Value: "user-value"},
	}
	src := []corev1.EnvVar{
		{Name: "OTEL_SERVICE_NAME", Value: "injected-value"},
		{Name: "NEW_VAR", Value: "new"},
	}

	result := mergeEnvVars(dst, src)

	envMap := envToMap(result)
	g.Expect(envMap).To(HaveKeyWithValue("EXISTING", "keep"))
	g.Expect(envMap).To(HaveKeyWithValue("OTEL_SERVICE_NAME", "user-value"))
	g.Expect(envMap).To(HaveKeyWithValue("NEW_VAR", "new"))
	g.Expect(result).To(HaveLen(3))
}

func TestHasArg(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		flag   string
		expect bool
	}{
		{"exact match", []string{"--tracing=true"}, "--tracing", true},
		{"flag=value form", []string{"--otlp-traces-endpoint=http://x"}, "--otlp-traces-endpoint", true},
		{"flag alone", []string{"--tracing"}, "--tracing", true},
		{"no match", []string{"--other"}, "--tracing", false},
		{"empty args", []string{}, "--tracing", false},
		{"partial prefix doesn't match", []string{"--tracing-extra"}, "--tracing", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			g.Expect(hasArg(tt.args, tt.flag)).To(Equal(tt.expect))
		})
	}
}

func envToMap(envVars []corev1.EnvVar) map[string]string {
	m := make(map[string]string, len(envVars))
	for _, e := range envVars {
		m[e.Name] = e.Value
	}
	return m
}

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

	kedav1alpha1 "github.com/kedacore/keda/v2/apis/keda/v1alpha1"
	wvav1alpha1 "github.com/llm-d/llm-d-workload-variant-autoscaler/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/apis"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

// newTestLLMISVC creates a minimal LLMInferenceService for testing.
func newTestLLMISVC(name, namespace string) *v1alpha2.LLMInferenceService {
	return &v1alpha2.LLMInferenceService{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "serving.kserve.io/v1alpha2",
			Kind:       "LLMInferenceService",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       "test-uid-1234",
		},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Model: v1alpha2.LLMModelSpec{
				URI:  apis.URL{Scheme: "hf", Host: "meta-llama/Llama-3.1-8B"},
				Name: ptr.To("meta-llama/Llama-3.1-8B"),
			},
		},
	}
}

func TestExpectedHPA(t *testing.T) {
	tests := []struct {
		name           string
		llmSvc         *v1alpha2.LLMInferenceService
		scaling        *v1alpha2.ScalingSpec
		deploymentName string
		vaName         string
		hpaName        string
		validate       func(t *testing.T, hpa *autoscalingv2.HorizontalPodAutoscaler)
	}{
		{
			name:           "default minReplicas when nil",
			llmSvc:         newTestLLMISVC("test-svc", "test-ns"),
			scaling:        &v1alpha2.ScalingSpec{MaxReplicas: 5, WVA: &v1alpha2.WVASpec{ActuatorSpec: v1alpha2.ActuatorSpec{HPA: &v1alpha2.HPAScalingSpec{}}}},
			deploymentName: "test-svc-kserve",
			vaName:         "test-svc-kserve-va",
			hpaName:        "test-svc-kserve-hpa",
			validate: func(t *testing.T, hpa *autoscalingv2.HorizontalPodAutoscaler) {
				require.NotNil(t, hpa.Spec.MinReplicas)
				assert.Equal(t, int32(1), *hpa.Spec.MinReplicas, "default minReplicas should be 1")
			},
		},
		{
			name:   "explicit minReplicas",
			llmSvc: newTestLLMISVC("test-svc", "test-ns"),
			scaling: &v1alpha2.ScalingSpec{
				MinReplicas: ptr.To(int32(3)),
				MaxReplicas: 10,
				WVA:         &v1alpha2.WVASpec{ActuatorSpec: v1alpha2.ActuatorSpec{HPA: &v1alpha2.HPAScalingSpec{}}},
			},
			deploymentName: "test-svc-kserve",
			vaName:         "test-svc-kserve-va",
			hpaName:        "test-svc-kserve-hpa",
			validate: func(t *testing.T, hpa *autoscalingv2.HorizontalPodAutoscaler) {
				require.NotNil(t, hpa.Spec.MinReplicas)
				assert.Equal(t, int32(3), *hpa.Spec.MinReplicas)
				assert.Equal(t, int32(10), hpa.Spec.MaxReplicas)
			},
		},
		{
			name:           "scaleTargetRef points to correct deployment",
			llmSvc:         newTestLLMISVC("my-model", "prod"),
			scaling:        &v1alpha2.ScalingSpec{MaxReplicas: 5, WVA: &v1alpha2.WVASpec{ActuatorSpec: v1alpha2.ActuatorSpec{HPA: &v1alpha2.HPAScalingSpec{}}}},
			deploymentName: "my-model-kserve",
			vaName:         "my-model-kserve-va",
			hpaName:        "my-model-kserve-hpa",
			validate: func(t *testing.T, hpa *autoscalingv2.HorizontalPodAutoscaler) {
				assert.Equal(t, "apps/v1", hpa.Spec.ScaleTargetRef.APIVersion)
				assert.Equal(t, "Deployment", hpa.Spec.ScaleTargetRef.Kind)
				assert.Equal(t, "my-model-kserve", hpa.Spec.ScaleTargetRef.Name)
			},
		},
		{
			name:           "metric selector uses variant_name from VA",
			llmSvc:         newTestLLMISVC("test-svc", "test-ns"),
			scaling:        &v1alpha2.ScalingSpec{MaxReplicas: 5, WVA: &v1alpha2.WVASpec{ActuatorSpec: v1alpha2.ActuatorSpec{HPA: &v1alpha2.HPAScalingSpec{}}}},
			deploymentName: "test-svc-kserve",
			vaName:         "test-svc-kserve-va",
			hpaName:        "test-svc-kserve-hpa",
			validate: func(t *testing.T, hpa *autoscalingv2.HorizontalPodAutoscaler) {
				require.Len(t, hpa.Spec.Metrics, 1)
				metric := hpa.Spec.Metrics[0]
				assert.Equal(t, autoscalingv2.ExternalMetricSourceType, metric.Type)
				require.NotNil(t, metric.External)
				assert.Equal(t, "wva_desired_replicas", metric.External.Metric.Name)
				require.NotNil(t, metric.External.Metric.Selector)
				assert.Equal(t, "test-svc-kserve-va", metric.External.Metric.Selector.MatchLabels["variant_name"])
				assert.Equal(t, autoscalingv2.ValueMetricType, metric.External.Target.Type)
				assert.Equal(t, resource.NewQuantity(1, resource.DecimalSI), metric.External.Target.Value)
			},
		},
		{
			name:   "custom HPA behavior is forwarded",
			llmSvc: newTestLLMISVC("test-svc", "test-ns"),
			scaling: &v1alpha2.ScalingSpec{
				MaxReplicas: 5,
				WVA: &v1alpha2.WVASpec{
					ActuatorSpec: v1alpha2.ActuatorSpec{
						HPA: &v1alpha2.HPAScalingSpec{
							Behavior: &autoscalingv2.HorizontalPodAutoscalerBehavior{
								ScaleDown: &autoscalingv2.HPAScalingRules{
									StabilizationWindowSeconds: ptr.To(int32(120)),
								},
							},
						},
					},
				},
			},
			deploymentName: "test-svc-kserve",
			vaName:         "test-svc-kserve-va",
			hpaName:        "test-svc-kserve-hpa",
			validate: func(t *testing.T, hpa *autoscalingv2.HorizontalPodAutoscaler) {
				require.NotNil(t, hpa.Spec.Behavior)
				require.NotNil(t, hpa.Spec.Behavior.ScaleDown)
				assert.Equal(t, int32(120), *hpa.Spec.Behavior.ScaleDown.StabilizationWindowSeconds)
			},
		},
		{
			name:           "nil behavior when not specified",
			llmSvc:         newTestLLMISVC("test-svc", "test-ns"),
			scaling:        &v1alpha2.ScalingSpec{MaxReplicas: 5, WVA: &v1alpha2.WVASpec{ActuatorSpec: v1alpha2.ActuatorSpec{HPA: &v1alpha2.HPAScalingSpec{}}}},
			deploymentName: "test-svc-kserve",
			vaName:         "test-svc-kserve-va",
			hpaName:        "test-svc-kserve-hpa",
			validate: func(t *testing.T, hpa *autoscalingv2.HorizontalPodAutoscaler) {
				assert.Nil(t, hpa.Spec.Behavior)
			},
		},
		{
			name:           "owner reference is set",
			llmSvc:         newTestLLMISVC("test-svc", "test-ns"),
			scaling:        &v1alpha2.ScalingSpec{MaxReplicas: 5, WVA: &v1alpha2.WVASpec{ActuatorSpec: v1alpha2.ActuatorSpec{HPA: &v1alpha2.HPAScalingSpec{}}}},
			deploymentName: "test-svc-kserve",
			vaName:         "test-svc-kserve-va",
			hpaName:        "test-svc-kserve-hpa",
			validate: func(t *testing.T, hpa *autoscalingv2.HorizontalPodAutoscaler) {
				require.Len(t, hpa.OwnerReferences, 1)
				assert.Equal(t, "test-svc", hpa.OwnerReferences[0].Name)
				assert.Equal(t, "LLMInferenceService", hpa.OwnerReferences[0].Kind)
				assert.True(t, *hpa.OwnerReferences[0].Controller)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hpa := expectedHPA(tt.llmSvc, tt.scaling, tt.deploymentName, tt.vaName, tt.hpaName)
			assert.Equal(t, tt.hpaName, hpa.Name)
			assert.Equal(t, tt.llmSvc.Namespace, hpa.Namespace)
			tt.validate(t, hpa)
		})
	}
}

func TestExpectedScaledObject(t *testing.T) {
	tests := []struct {
		name           string
		llmSvc         *v1alpha2.LLMInferenceService
		scaling        *v1alpha2.ScalingSpec
		config         *Config
		deploymentName string
		vaName         string
		soName         string
		validate       func(t *testing.T, so *kedav1alpha1.ScaledObject)
	}{
		{
			name:   "default minReplicas when nil",
			llmSvc: newTestLLMISVC("test-svc", "test-ns"),
			scaling: &v1alpha2.ScalingSpec{
				MaxReplicas: 5,
				WVA:         &v1alpha2.WVASpec{ActuatorSpec: v1alpha2.ActuatorSpec{KEDA: &v1alpha2.KEDAScalingSpec{}}},
			},
			config:         &Config{WVAAutoscalingConfig: &WVAAutoscalingConfig{Prometheus: PrometheusConfig{URL: "http://prom:9090"}}},
			deploymentName: "test-svc-kserve",
			vaName:         "test-svc-kserve-va",
			soName:         "test-svc-kserve-keda",
			validate: func(t *testing.T, so *kedav1alpha1.ScaledObject) {
				require.NotNil(t, so.Spec.MinReplicaCount)
				assert.Equal(t, int32(1), *so.Spec.MinReplicaCount)
			},
		},
		{
			name:   "explicit minReplicas and maxReplicas",
			llmSvc: newTestLLMISVC("test-svc", "test-ns"),
			scaling: &v1alpha2.ScalingSpec{
				MinReplicas: ptr.To(int32(2)),
				MaxReplicas: 8,
				WVA:         &v1alpha2.WVASpec{ActuatorSpec: v1alpha2.ActuatorSpec{KEDA: &v1alpha2.KEDAScalingSpec{}}},
			},
			config:         &Config{WVAAutoscalingConfig: &WVAAutoscalingConfig{Prometheus: PrometheusConfig{URL: "http://prom:9090"}}},
			deploymentName: "test-svc-kserve",
			vaName:         "test-svc-kserve-va",
			soName:         "test-svc-kserve-keda",
			validate: func(t *testing.T, so *kedav1alpha1.ScaledObject) {
				assert.Equal(t, int32(2), *so.Spec.MinReplicaCount)
				require.NotNil(t, so.Spec.MaxReplicaCount)
				assert.Equal(t, int32(8), *so.Spec.MaxReplicaCount)
			},
		},
		{
			name:   "scaleTargetRef points to correct deployment",
			llmSvc: newTestLLMISVC("my-model", "prod"),
			scaling: &v1alpha2.ScalingSpec{
				MaxReplicas: 5,
				WVA:         &v1alpha2.WVASpec{ActuatorSpec: v1alpha2.ActuatorSpec{KEDA: &v1alpha2.KEDAScalingSpec{}}},
			},
			config:         &Config{WVAAutoscalingConfig: &WVAAutoscalingConfig{Prometheus: PrometheusConfig{URL: "http://prom:9090"}}},
			deploymentName: "my-model-kserve-prefill",
			vaName:         "my-model-kserve-prefill-va",
			soName:         "my-model-kserve-prefill-keda",
			validate: func(t *testing.T, so *kedav1alpha1.ScaledObject) {
				require.NotNil(t, so.Spec.ScaleTargetRef)
				assert.Equal(t, "apps/v1", so.Spec.ScaleTargetRef.APIVersion)
				assert.Equal(t, "Deployment", so.Spec.ScaleTargetRef.Kind)
				assert.Equal(t, "my-model-kserve-prefill", so.Spec.ScaleTargetRef.Name)
			},
		},
		{
			name:   "prometheus trigger has correct metadata",
			llmSvc: newTestLLMISVC("test-svc", "test-ns"),
			scaling: &v1alpha2.ScalingSpec{
				MaxReplicas: 5,
				WVA:         &v1alpha2.WVASpec{ActuatorSpec: v1alpha2.ActuatorSpec{KEDA: &v1alpha2.KEDAScalingSpec{}}},
			},
			config:         &Config{WVAAutoscalingConfig: &WVAAutoscalingConfig{Prometheus: PrometheusConfig{URL: "https://prom.monitoring:9090", TLSInsecureSkipVerify: true}}},
			deploymentName: "test-svc-kserve",
			vaName:         "test-svc-kserve-va",
			soName:         "test-svc-kserve-keda",
			validate: func(t *testing.T, so *kedav1alpha1.ScaledObject) {
				require.Len(t, so.Spec.Triggers, 1)
				trigger := so.Spec.Triggers[0]
				assert.Equal(t, "prometheus", trigger.Type)
				assert.Equal(t, "wva-desired-replicas", trigger.Name)
				assert.Equal(t, "https://prom.monitoring:9090", trigger.Metadata["serverAddress"])
				assert.Equal(t, `wva_desired_replicas{variant_name="test-svc-kserve-va",exported_namespace="test-ns"}`, trigger.Metadata["query"])
				assert.Equal(t, "1", trigger.Metadata["threshold"])
				assert.Equal(t, "true", trigger.Metadata["unsafeSsl"])
			},
		},
		{
			name:   "unsafeSsl is false when TLS verification enabled",
			llmSvc: newTestLLMISVC("test-svc", "test-ns"),
			scaling: &v1alpha2.ScalingSpec{
				MaxReplicas: 5,
				WVA:         &v1alpha2.WVASpec{ActuatorSpec: v1alpha2.ActuatorSpec{KEDA: &v1alpha2.KEDAScalingSpec{}}},
			},
			config:         &Config{WVAAutoscalingConfig: &WVAAutoscalingConfig{Prometheus: PrometheusConfig{URL: "http://prom:9090", TLSInsecureSkipVerify: false}}},
			deploymentName: "test-svc-kserve",
			vaName:         "test-svc-kserve-va",
			soName:         "test-svc-kserve-keda",
			validate: func(t *testing.T, so *kedav1alpha1.ScaledObject) {
				assert.Equal(t, "false", so.Spec.Triggers[0].Metadata["unsafeSsl"])
			},
		},
		{
			name:   "optional KEDA fields forwarded",
			llmSvc: newTestLLMISVC("test-svc", "test-ns"),
			scaling: &v1alpha2.ScalingSpec{
				MaxReplicas: 5,
				WVA: &v1alpha2.WVASpec{ActuatorSpec: v1alpha2.ActuatorSpec{KEDA: &v1alpha2.KEDAScalingSpec{
					PollingInterval: ptr.To(int32(15)),
					CooldownPeriod:  ptr.To(int32(60)),
				}}},
			},
			config:         &Config{WVAAutoscalingConfig: &WVAAutoscalingConfig{Prometheus: PrometheusConfig{URL: "http://prom:9090"}}},
			deploymentName: "test-svc-kserve",
			vaName:         "test-svc-kserve-va",
			soName:         "test-svc-kserve-keda",
			validate: func(t *testing.T, so *kedav1alpha1.ScaledObject) {
				require.NotNil(t, so.Spec.PollingInterval)
				assert.Equal(t, int32(15), *so.Spec.PollingInterval)
				require.NotNil(t, so.Spec.CooldownPeriod)
				assert.Equal(t, int32(60), *so.Spec.CooldownPeriod)
			},
		},
		{
			name:   "owner reference is set",
			llmSvc: newTestLLMISVC("test-svc", "test-ns"),
			scaling: &v1alpha2.ScalingSpec{
				MaxReplicas: 5,
				WVA:         &v1alpha2.WVASpec{ActuatorSpec: v1alpha2.ActuatorSpec{KEDA: &v1alpha2.KEDAScalingSpec{}}},
			},
			config:         &Config{WVAAutoscalingConfig: &WVAAutoscalingConfig{Prometheus: PrometheusConfig{URL: "http://prom:9090"}}},
			deploymentName: "test-svc-kserve",
			vaName:         "test-svc-kserve-va",
			soName:         "test-svc-kserve-keda",
			validate: func(t *testing.T, so *kedav1alpha1.ScaledObject) {
				require.Len(t, so.OwnerReferences, 1)
				assert.Equal(t, "test-svc", so.OwnerReferences[0].Name)
				assert.Equal(t, "LLMInferenceService", so.OwnerReferences[0].Kind)
				assert.True(t, *so.OwnerReferences[0].Controller)
			},
		},
		{
			name:   "no auth fields when not configured",
			llmSvc: newTestLLMISVC("test-svc", "test-ns"),
			scaling: &v1alpha2.ScalingSpec{
				MaxReplicas: 5,
				WVA:         &v1alpha2.WVASpec{ActuatorSpec: v1alpha2.ActuatorSpec{KEDA: &v1alpha2.KEDAScalingSpec{}}},
			},
			config:         &Config{WVAAutoscalingConfig: &WVAAutoscalingConfig{Prometheus: PrometheusConfig{URL: "http://prom:9090"}}},
			deploymentName: "test-svc-kserve",
			vaName:         "test-svc-kserve-va",
			soName:         "test-svc-kserve-keda",
			validate: func(t *testing.T, so *kedav1alpha1.ScaledObject) {
				trigger := so.Spec.Triggers[0]
				assert.NotContains(t, trigger.Metadata, "authModes")
				assert.Nil(t, trigger.AuthenticationRef)
			},
		},
		{
			name:   "TriggerAuthentication auth fields are wired into trigger",
			llmSvc: newTestLLMISVC("test-svc", "test-ns"),
			scaling: &v1alpha2.ScalingSpec{
				MaxReplicas: 5,
				WVA:         &v1alpha2.WVASpec{ActuatorSpec: v1alpha2.ActuatorSpec{KEDA: &v1alpha2.KEDAScalingSpec{}}},
			},
			config: &Config{WVAAutoscalingConfig: &WVAAutoscalingConfig{
				Prometheus: PrometheusConfig{
					URL:             "https://prom.monitoring:9090",
					AuthModes:       "bearer",
					TriggerAuthName: "prom-bearer-auth",
					TriggerAuthKind: "TriggerAuthentication",
				},
			}},
			deploymentName: "test-svc-kserve",
			vaName:         "test-svc-kserve-va",
			soName:         "test-svc-kserve-keda",
			validate: func(t *testing.T, so *kedav1alpha1.ScaledObject) {
				trigger := so.Spec.Triggers[0]
				assert.Equal(t, "bearer", trigger.Metadata["authModes"])
				require.NotNil(t, trigger.AuthenticationRef)
				assert.Equal(t, "prom-bearer-auth", trigger.AuthenticationRef.Name)
				assert.Equal(t, "TriggerAuthentication", trigger.AuthenticationRef.Kind)
			},
		},
		{
			name:   "ClusterTriggerAuthentication auth fields are wired into trigger",
			llmSvc: newTestLLMISVC("test-svc", "test-ns"),
			scaling: &v1alpha2.ScalingSpec{
				MaxReplicas: 5,
				WVA:         &v1alpha2.WVASpec{ActuatorSpec: v1alpha2.ActuatorSpec{KEDA: &v1alpha2.KEDAScalingSpec{}}},
			},
			config: &Config{WVAAutoscalingConfig: &WVAAutoscalingConfig{
				Prometheus: PrometheusConfig{
					URL:             "https://prom.monitoring:9090",
					AuthModes:       "bearer",
					TriggerAuthName: "cluster-prom-auth",
					TriggerAuthKind: "ClusterTriggerAuthentication",
				},
			}},
			deploymentName: "test-svc-kserve",
			vaName:         "test-svc-kserve-va",
			soName:         "test-svc-kserve-keda",
			validate: func(t *testing.T, so *kedav1alpha1.ScaledObject) {
				trigger := so.Spec.Triggers[0]
				assert.Equal(t, "bearer", trigger.Metadata["authModes"])
				require.NotNil(t, trigger.AuthenticationRef)
				assert.Equal(t, "cluster-prom-auth", trigger.AuthenticationRef.Name)
				assert.Equal(t, "ClusterTriggerAuthentication", trigger.AuthenticationRef.Kind)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			so := expectedScaledObject(tt.llmSvc, tt.scaling, tt.config, tt.deploymentName, tt.vaName, tt.soName)
			assert.Equal(t, tt.soName, so.Name)
			assert.Equal(t, tt.llmSvc.Namespace, so.Namespace)
			tt.validate(t, so)
		})
	}
}

func TestExpectedVA(t *testing.T) {
	tests := []struct {
		name           string
		llmSvc         *v1alpha2.LLMInferenceService
		scaling        *v1alpha2.ScalingSpec
		deploymentName string
		vaName         string
		workloadLabels map[string]string
		validate       func(t *testing.T, va *wvav1alpha1.VariantAutoscaling)
	}{
		{
			name:   "scaleTargetRef points to deployment",
			llmSvc: newTestLLMISVC("my-model", "prod"),
			scaling: &v1alpha2.ScalingSpec{
				WVA: &v1alpha2.WVASpec{VariantCost: "10.0"},
			},
			deploymentName: "my-model-kserve",
			vaName:         "my-model-kserve-va",
			validate: func(t *testing.T, va *wvav1alpha1.VariantAutoscaling) {
				assert.Equal(t, autoscalingv1.CrossVersionObjectReference{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "my-model-kserve",
				}, va.Spec.ScaleTargetRef)
			},
		},
		{
			name:   "modelID from model.name when set",
			llmSvc: newTestLLMISVC("test-svc", "test-ns"),
			scaling: &v1alpha2.ScalingSpec{
				WVA: &v1alpha2.WVASpec{VariantCost: "5.0"},
			},
			deploymentName: "test-svc-kserve",
			vaName:         "test-svc-kserve-va",
			validate: func(t *testing.T, va *wvav1alpha1.VariantAutoscaling) {
				assert.Equal(t, "meta-llama/Llama-3.1-8B", va.Spec.ModelID)
			},
		},
		{
			name: "modelID falls back to URI when name is nil",
			llmSvc: func() *v1alpha2.LLMInferenceService {
				svc := newTestLLMISVC("test-svc", "test-ns")
				svc.Spec.Model.Name = nil
				return svc
			}(),
			scaling: &v1alpha2.ScalingSpec{
				WVA: &v1alpha2.WVASpec{VariantCost: "10.0"},
			},
			deploymentName: "test-svc-kserve",
			vaName:         "test-svc-kserve-va",
			validate: func(t *testing.T, va *wvav1alpha1.VariantAutoscaling) {
				expectedURI := apis.URL{Scheme: "hf", Host: "meta-llama/Llama-3.1-8B"}
				assert.Equal(t, expectedURI.String(), va.Spec.ModelID, "should match URI.String() output")
			},
		},
		{
			name:   "variantCost forwarded correctly",
			llmSvc: newTestLLMISVC("test-svc", "test-ns"),
			scaling: &v1alpha2.ScalingSpec{
				WVA: &v1alpha2.WVASpec{VariantCost: "42.5"},
			},
			deploymentName: "test-svc-kserve",
			vaName:         "test-svc-kserve-va",
			validate: func(t *testing.T, va *wvav1alpha1.VariantAutoscaling) {
				assert.Equal(t, "42.5", va.Spec.VariantCost)
			},
		},
		{
			name:   "owner reference is set",
			llmSvc: newTestLLMISVC("test-svc", "test-ns"),
			scaling: &v1alpha2.ScalingSpec{
				WVA: &v1alpha2.WVASpec{VariantCost: "10.0"},
			},
			deploymentName: "test-svc-kserve",
			vaName:         "test-svc-kserve-va",
			validate: func(t *testing.T, va *wvav1alpha1.VariantAutoscaling) {
				require.Len(t, va.OwnerReferences, 1)
				assert.Equal(t, "test-svc", va.OwnerReferences[0].Name)
				assert.Equal(t, "LLMInferenceService", va.OwnerReferences[0].Kind)
			},
		},
		{
			name:   "acceleratorName label taken from workload labels when present",
			llmSvc: newTestLLMISVC("test-svc", "test-ns"),
			scaling: &v1alpha2.ScalingSpec{
				WVA: &v1alpha2.WVASpec{VariantCost: "10.0"},
			},
			deploymentName: "test-svc-kserve",
			vaName:         "test-svc-kserve-va",
			workloadLabels: map[string]string{
				acceleratorNameLabelKey: "H100",
			},
			validate: func(t *testing.T, va *wvav1alpha1.VariantAutoscaling) {
				assert.Equal(t, "H100", va.Labels[acceleratorNameLabelKey])
			},
		},
		{
			name:   "acceleratorName label is unknown when not present in workload labels",
			llmSvc: newTestLLMISVC("test-svc", "test-ns"),
			scaling: &v1alpha2.ScalingSpec{
				WVA: &v1alpha2.WVASpec{VariantCost: "10.0"},
			},
			deploymentName: "test-svc-kserve",
			vaName:         "test-svc-kserve-va",
			workloadLabels: nil,
			validate: func(t *testing.T, va *wvav1alpha1.VariantAutoscaling) {
				assert.Equal(t, "unknown", va.Labels[acceleratorNameLabelKey])
			},
		},
		{
			name:   "acceleratorName label is unknown when present but empty in workload labels",
			llmSvc: newTestLLMISVC("test-svc", "test-ns"),
			scaling: &v1alpha2.ScalingSpec{
				WVA: &v1alpha2.WVASpec{VariantCost: "10.0"},
			},
			deploymentName: "test-svc-kserve",
			vaName:         "test-svc-kserve-va",
			workloadLabels: map[string]string{
				acceleratorNameLabelKey: "",
			},
			validate: func(t *testing.T, va *wvav1alpha1.VariantAutoscaling) {
				assert.Equal(t, "unknown", va.Labels[acceleratorNameLabelKey])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			va := expectedVA(tt.llmSvc, tt.scaling, tt.deploymentName, tt.vaName, tt.workloadLabels)
			assert.Equal(t, tt.vaName, va.Name)
			assert.Equal(t, tt.llmSvc.Namespace, va.Namespace)
			tt.validate(t, va)
		})
	}
}

func TestValidateAutoscalingConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *WVAAutoscalingConfig
		wantErr string
	}{
		{
			name:    "nil config returns error",
			cfg:     nil,
			wantErr: "autoscaling-wva-controller-config.prometheus.url is required",
		},
		{
			name:    "missing prometheus.url returns error",
			cfg:     &WVAAutoscalingConfig{},
			wantErr: "autoscaling-wva-controller-config.prometheus.url is required",
		},
		{
			name: "no auth fields is valid",
			cfg:  &WVAAutoscalingConfig{Prometheus: PrometheusConfig{URL: "http://prom:9090"}},
		},
		{
			name: "both auth fields set is valid",
			cfg: &WVAAutoscalingConfig{
				Prometheus: PrometheusConfig{
					URL:             "https://prom:9090",
					AuthModes:       "bearer",
					TriggerAuthName: "prom-auth",
				},
			},
		},
		{
			name: "authModes set without triggerAuthName returns error",
			cfg: &WVAAutoscalingConfig{
				Prometheus: PrometheusConfig{
					URL:       "https://prom:9090",
					AuthModes: "bearer",
				},
			},
			wantErr: "autoscaling-wva-controller-config.prometheus.authModes and autoscaling-wva-controller-config.prometheus.triggerAuthName must both be set or both be empty",
		},
		{
			name: "triggerAuthName set without authModes returns error",
			cfg: &WVAAutoscalingConfig{
				Prometheus: PrometheusConfig{
					URL:             "https://prom:9090",
					TriggerAuthName: "prom-auth",
				},
			},
			wantErr: "autoscaling-wva-controller-config.prometheus.authModes and autoscaling-wva-controller-config.prometheus.triggerAuthName must both be set or both be empty",
		},
		{
			name: "ClusterTriggerAuthentication kind with both auth fields is valid",
			cfg: &WVAAutoscalingConfig{
				Prometheus: PrometheusConfig{
					URL:             "https://prom:9090",
					AuthModes:       "bearer",
					TriggerAuthName: "cluster-prom-auth",
					TriggerAuthKind: "ClusterTriggerAuthentication",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAutoscalingConfig(tt.cfg)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSemanticHPAIsEqual(t *testing.T) {
	base := func() *autoscalingv2.HorizontalPodAutoscaler {
		return &autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "test"}},
			Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
				MaxReplicas: 5,
				MinReplicas: ptr.To(int32(1)),
			},
		}
	}

	t.Run("equal specs returns true", func(t *testing.T) {
		assert.True(t, semanticHPAIsEqual(base(), base()))
	})

	t.Run("different maxReplicas returns false", func(t *testing.T) {
		modified := base()
		modified.Spec.MaxReplicas = 10
		assert.False(t, semanticHPAIsEqual(base(), modified))
	})

	t.Run("different labels returns false", func(t *testing.T) {
		modified := base()
		modified.Labels = map[string]string{"app": "other"}
		assert.False(t, semanticHPAIsEqual(base(), modified))
	})

	t.Run("removed field in expected is detected", func(t *testing.T) {
		expected := base()
		expected.Spec.MinReplicas = nil
		assert.False(t, semanticHPAIsEqual(expected, base()))
	})

	t.Run("extra label on curr is detected", func(t *testing.T) {
		curr := base()
		curr.Labels["extra"] = "value"
		assert.False(t, semanticHPAIsEqual(base(), curr))
	})

	t.Run("removed annotation in expected is detected", func(t *testing.T) {
		expected := base()
		expected.Annotations = nil
		curr := base()
		curr.Annotations = map[string]string{"note": "old"}
		assert.False(t, semanticHPAIsEqual(expected, curr))
	})
}

func TestSemanticScaledObjectIsEqual(t *testing.T) {
	base := func() *kedav1alpha1.ScaledObject {
		return &kedav1alpha1.ScaledObject{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "test"}},
			Spec: kedav1alpha1.ScaledObjectSpec{
				MaxReplicaCount: ptr.To(int32(5)),
				Triggers: []kedav1alpha1.ScaleTriggers{
					{Type: "prometheus", Metadata: map[string]string{"query": "up"}},
				},
			},
		}
	}

	t.Run("equal specs returns true", func(t *testing.T) {
		assert.True(t, semanticScaledObjectIsEqual(base(), base()))
	})

	t.Run("different trigger returns false", func(t *testing.T) {
		modified := base()
		modified.Spec.Triggers[0].Metadata["query"] = "down"
		assert.False(t, semanticScaledObjectIsEqual(base(), modified))
	})

	t.Run("different labels returns false", func(t *testing.T) {
		modified := base()
		modified.Labels = map[string]string{"app": "other"}
		assert.False(t, semanticScaledObjectIsEqual(base(), modified))
	})

	t.Run("removed optional field in expected is detected", func(t *testing.T) {
		expected := base()
		expected.Spec.MaxReplicaCount = nil
		assert.False(t, semanticScaledObjectIsEqual(expected, base()))
	})

	t.Run("extra label on curr is detected", func(t *testing.T) {
		curr := base()
		curr.Labels["extra"] = "value"
		assert.False(t, semanticScaledObjectIsEqual(base(), curr))
	})
}

func TestSemanticVAIsEqual(t *testing.T) {
	base := func() *wvav1alpha1.VariantAutoscaling {
		return &wvav1alpha1.VariantAutoscaling{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "test"}},
			Spec: wvav1alpha1.VariantAutoscalingSpec{
				ModelID:     "meta-llama/Llama-3.1-8B",
				VariantCost: "10.0",
			},
		}
	}

	t.Run("equal specs returns true", func(t *testing.T) {
		assert.True(t, semanticVAIsEqual(base(), base()))
	})

	t.Run("different modelID returns false", func(t *testing.T) {
		modified := base()
		modified.Spec.ModelID = "other-model"
		assert.False(t, semanticVAIsEqual(base(), modified))
	})

	t.Run("different labels returns false", func(t *testing.T) {
		modified := base()
		modified.Labels = map[string]string{"app": "other"}
		assert.False(t, semanticVAIsEqual(base(), modified))
	})

	t.Run("removed variantCost in expected is detected", func(t *testing.T) {
		expected := base()
		expected.Spec.VariantCost = ""
		assert.False(t, semanticVAIsEqual(expected, base()))
	})

	t.Run("extra label on curr is detected", func(t *testing.T) {
		curr := base()
		curr.Labels["extra"] = "value"
		assert.False(t, semanticVAIsEqual(base(), curr))
	})

	t.Run("removed annotation in expected is detected", func(t *testing.T) {
		expected := base()
		expected.Annotations = nil
		curr := base()
		curr.Annotations = map[string]string{"note": "old"}
		assert.False(t, semanticVAIsEqual(expected, curr))
	})
}

func TestPreserveKEDAManagedMetadata(t *testing.T) {
	hook := PreserveKEDAManagedMetadata()

	// Extract the AfterDryRunFunc from the UpdateOption by applying it to updateOptions.
	opts := &updateOptions[*kedav1alpha1.ScaledObject]{}
	hook(opts)
	require.Len(t, opts.afterDryRunFns, 1)
	fn := opts.afterDryRunFns[0]

	t.Run("copies KEDA label from curr into expected", func(t *testing.T) {
		expected := &kedav1alpha1.ScaledObject{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "test"}},
		}
		curr := &kedav1alpha1.ScaledObject{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
				"app":                                    "test",
				kedav1alpha1.ScaledObjectOwnerAnnotation: "my-so",
			}},
		}
		fn(expected, expected.DeepCopy(), curr)
		assert.Equal(t, "my-so", expected.Labels[kedav1alpha1.ScaledObjectOwnerAnnotation])
		assert.Equal(t, "test", expected.Labels["app"])
	})

	t.Run("no-op when curr has no KEDA label", func(t *testing.T) {
		expected := &kedav1alpha1.ScaledObject{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "test"}},
		}
		curr := &kedav1alpha1.ScaledObject{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "test"}},
		}
		fn(expected, expected.DeepCopy(), curr)
		assert.Equal(t, map[string]string{"app": "test"}, expected.Labels)
	})

	t.Run("initializes nil labels map when KEDA label present on curr", func(t *testing.T) {
		expected := &kedav1alpha1.ScaledObject{}
		curr := &kedav1alpha1.ScaledObject{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
				kedav1alpha1.ScaledObjectOwnerAnnotation: "my-so",
			}},
		}
		fn(expected, expected.DeepCopy(), curr)
		assert.Equal(t, "my-so", expected.Labels[kedav1alpha1.ScaledObjectOwnerAnnotation])
	})

	t.Run("copies finalizers from curr into expected", func(t *testing.T) {
		expected := &kedav1alpha1.ScaledObject{}
		curr := &kedav1alpha1.ScaledObject{
			ObjectMeta: metav1.ObjectMeta{Finalizers: []string{"finalizer.keda.sh"}},
		}
		fn(expected, expected.DeepCopy(), curr)
		assert.Equal(t, []string{"finalizer.keda.sh"}, expected.Finalizers)
	})

	t.Run("no-op when curr has no finalizers", func(t *testing.T) {
		expected := &kedav1alpha1.ScaledObject{}
		curr := &kedav1alpha1.ScaledObject{}
		fn(expected, expected.DeepCopy(), curr)
		assert.Empty(t, expected.Finalizers)
	})

	t.Run("preserves both KEDA label and finalizers together", func(t *testing.T) {
		expected := &kedav1alpha1.ScaledObject{}
		curr := &kedav1alpha1.ScaledObject{
			ObjectMeta: metav1.ObjectMeta{
				Labels:     map[string]string{kedav1alpha1.ScaledObjectOwnerAnnotation: "my-so"},
				Finalizers: []string{"finalizer.keda.sh"},
			},
		}
		fn(expected, expected.DeepCopy(), curr)
		assert.Equal(t, "my-so", expected.Labels[kedav1alpha1.ScaledObjectOwnerAnnotation])
		assert.Equal(t, []string{"finalizer.keda.sh"}, expected.Finalizers)
	})
}

func TestNamingHelpers(t *testing.T) {
	svc := newTestLLMISVC("sim-llama", "llm-d-dev")

	t.Run("main deployment name (standard)", func(t *testing.T) {
		assert.Equal(t, "sim-llama-kserve", mainDeploymentName(svc))
	})

	t.Run("prefill deployment name", func(t *testing.T) {
		assert.Equal(t, "sim-llama-kserve-prefill", prefillDeploymentName(svc))
	})

	t.Run("main HPA name", func(t *testing.T) {
		assert.Equal(t, "sim-llama-kserve-hpa", mainHPAName(svc))
	})

	t.Run("prefill HPA name", func(t *testing.T) {
		assert.Equal(t, "sim-llama-kserve-prefill-hpa", prefillHPAName(svc))
	})

	t.Run("main VA name", func(t *testing.T) {
		assert.Equal(t, "sim-llama-kserve-va", mainVAName(svc))
	})

	t.Run("prefill VA name", func(t *testing.T) {
		assert.Equal(t, "sim-llama-kserve-prefill-va", prefillVAName(svc))
	})

	t.Run("main ScaledObject name", func(t *testing.T) {
		assert.Equal(t, "sim-llama-kserve-keda", mainScaledObjectName(svc))
	})

	t.Run("prefill ScaledObject name", func(t *testing.T) {
		assert.Equal(t, "sim-llama-kserve-prefill-keda", prefillScaledObjectName(svc))
	})
}

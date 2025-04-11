/*
Copyright 2022 The KServe Authors.

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

import (
	"fmt"
	"testing"

	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakeclientset "k8s.io/client-go/kubernetes/fake"

	"github.com/kserve/kserve/pkg/constants"
)

var (
	KserveIngressGateway       = "kserve/kserve-ingress-gateway"
	KnativeIngressGateway      = "knative-serving/knative-ingress-gateway"
	KnativeLocalGatewayService = "test-destination"
	KnativeLocalGateway        = "knative-serving/knative-local-gateway"
	LocalGatewayService        = "knative-local-gateway.istio-system.svc.cluster.local"
	UrlScheme                  = "https"
	IngressDomain              = "example.com"
	AdditionalDomain           = "additional-example.com"
	AdditionalDomainExtra      = "additional-example-extra.com"
	IngressConfigData          = fmt.Sprintf(`{
	    "kserveIngressGateway" : "%s",
		"ingressGateway" : "%s",
		"knativeLocalGatewayService" : "%s",
		"localGateway" : "%s",
		"localGatewayService" : "%s",
		"ingressDomain": "%s",
		"urlScheme": "https",
        "additionalIngressDomains": ["%s","%s"]
	}`, KserveIngressGateway, KnativeIngressGateway, KnativeLocalGatewayService, KnativeLocalGateway, LocalGatewayService, IngressDomain,
		AdditionalDomain, AdditionalDomainExtra)
	ServiceConfigData = fmt.Sprintf(`{
		"serviceClusterIPNone" : %t
	}`, true)

	ISCVWithData = fmt.Sprintf(`{
		"serviceAnnotationDisallowedList": ["%s","%s"],
		"serviceLabelDisallowedList": ["%s","%s"]
	}`, "my.custom.annotation/1", "my.custom.annotation/2",
		"my.custom.label.1", "my.custom.label.2")

	ISCVNoData = fmt.Sprintf(`{
		"serviceAnnotationDisallowedList": %s,
		"serviceLabelDisallowedList": %s
	}`, []string{}, []string{})

	MultiNodeConfigData = `{
		"customGPUResourceTypeList": [
			"custom.com/gpu-1",
			"custom.com/gpu-2"
		]
	}`
	MultiNodeConfigNoData = `{}`
)

func TestNewInferenceServiceConfig(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	clientset := fakeclientset.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
	})
	isvcConfigMap, err := GetInferenceServiceConfigMap(t.Context(), clientset)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	isvcConfig, err := NewInferenceServicesConfig(isvcConfigMap)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	g.Expect(isvcConfig).ShouldNot(gomega.BeNil())
}

func TestNewMultiNodeConfigWithNoData(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	clientset := fakeclientset.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
		Data: map[string]string{
			MultiNodeConfigKeyName: MultiNodeConfigNoData,
		},
	})

	configMap, err := GetInferenceServiceConfigMap(t.Context(), clientset)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())

	multiNodeCfg, err := NewMultiNodeConfig(configMap)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	g.Expect(multiNodeCfg).ShouldNot(gomega.BeNil())
	g.Expect(multiNodeCfg.CustomGPUResourceTypeList).To(gomega.Equal([]string{}))
	g.Expect(constants.DefaultGPUResourceTypeList).To(gomega.Equal([]string{"nvidia.com/gpu", "amd.com/gpu", "intel.com/gpu", "habana.ai/gaudi"}))
}

func TestNewMultiNodeConfigWithoutData(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	clientset := fakeclientset.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
		Data:       map[string]string{},
	})

	configMap, err := GetInferenceServiceConfigMap(t.Context(), clientset)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())

	multiNodeCfg, err := NewMultiNodeConfig(configMap)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	g.Expect(multiNodeCfg).ShouldNot(gomega.BeNil())
	g.Expect(multiNodeCfg.CustomGPUResourceTypeList).To(gomega.Equal([]string{}))
	g.Expect(constants.DefaultGPUResourceTypeList).To(gomega.Equal([]string{"nvidia.com/gpu", "amd.com/gpu", "intel.com/gpu", "habana.ai/gaudi"}))
}

func TestNewMultiNodeConfigWithData(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	clientset := fakeclientset.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
		Data: map[string]string{
			MultiNodeConfigKeyName: MultiNodeConfigData,
		},
	})

	configMap, err := GetInferenceServiceConfigMap(t.Context(), clientset)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())

	multiNodeCfg, err := NewMultiNodeConfig(configMap)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	g.Expect(multiNodeCfg).ShouldNot(gomega.BeNil())
	g.Expect(multiNodeCfg.CustomGPUResourceTypeList).To(gomega.Equal([]string{"custom.com/gpu-1", "custom.com/gpu-2"}))
	g.Expect(constants.DefaultGPUResourceTypeList).To(gomega.Equal([]string{"nvidia.com/gpu", "amd.com/gpu", "intel.com/gpu", "habana.ai/gaudi", "custom.com/gpu-1", "custom.com/gpu-2"}))
}

func TestNewIngressConfig(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	clientset := fakeclientset.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
		Data: map[string]string{
			IngressConfigKeyName: IngressConfigData,
		},
	})
	configMap, err := GetInferenceServiceConfigMap(t.Context(), clientset)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	ingressCfg, err := NewIngressConfig(configMap)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	g.Expect(ingressCfg).ShouldNot(gomega.BeNil())

	g.Expect(ingressCfg.IngressGateway).To(gomega.Equal(KnativeIngressGateway))
	g.Expect(ingressCfg.KnativeLocalGatewayService).To(gomega.Equal(KnativeLocalGatewayService))
	g.Expect(ingressCfg.LocalGateway).To(gomega.Equal(KnativeLocalGateway))
	g.Expect(ingressCfg.LocalGatewayServiceName).To(gomega.Equal(LocalGatewayService))
	g.Expect(ingressCfg.UrlScheme).To(gomega.Equal(UrlScheme))
	g.Expect(ingressCfg.IngressDomain).To(gomega.Equal(IngressDomain))
	g.Expect(*ingressCfg.AdditionalIngressDomains).To(gomega.Equal([]string{AdditionalDomain, AdditionalDomainExtra}))
}

func TestNewIngressConfigDefaultKnativeService(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	clientset := fakeclientset.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
		Data: map[string]string{
			IngressConfigKeyName: fmt.Sprintf(`{
				"kserveIngressGateway" : "%s",
				"ingressGateway" : "%s",
				"localGateway" : "%s",
				"localGatewayService" : "%s",
				"ingressDomain": "%s",
				"urlScheme": "https",
        		"additionalIngressDomains": ["%s","%s"]
			}`, KserveIngressGateway, KnativeIngressGateway, KnativeLocalGateway, LocalGatewayService, IngressDomain,
				AdditionalDomain, AdditionalDomainExtra),
		},
	})
	configMap, err := GetInferenceServiceConfigMap(t.Context(), clientset)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	ingressCfg, err := NewIngressConfig(configMap)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	g.Expect(ingressCfg).ShouldNot(gomega.BeNil())
	g.Expect(ingressCfg.KnativeLocalGatewayService).To(gomega.Equal(LocalGatewayService))
}

func TestNewDeployConfig(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	clientset := fakeclientset.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
	})
	isvcConfigMap, err := GetInferenceServiceConfigMap(t.Context(), clientset)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	deployConfig, err := NewDeployConfig(isvcConfigMap)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	g.Expect(deployConfig).ShouldNot(gomega.BeNil())
}

func TestNewServiceConfig(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	// nothing declared
	empty := fakeclientset.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
	})
	isvcConfigMap, err := GetInferenceServiceConfigMap(t.Context(), empty)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	emp, err := NewServiceConfig(isvcConfigMap)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	g.Expect(emp).ShouldNot(gomega.BeNil())

	// with value
	withTrue := fakeclientset.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
		Data: map[string]string{
			ServiceConfigName: ServiceConfigData,
		},
	})
	isvcConfigMap, err = GetInferenceServiceConfigMap(t.Context(), withTrue)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	wt, err := NewServiceConfig(isvcConfigMap)

	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	g.Expect(wt).ShouldNot(gomega.BeNil())
	g.Expect(wt.ServiceClusterIPNone).Should(gomega.BeTrue())

	// no value, should be nil
	noValue := fakeclientset.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
		Data: map[string]string{
			ServiceConfigName: `{}`,
		},
	})
	isvcConfigMap, err = GetInferenceServiceConfigMap(t.Context(), noValue)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	nv, err := NewServiceConfig(isvcConfigMap)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	g.Expect(nv).ShouldNot(gomega.BeNil())
	g.Expect(nv.ServiceClusterIPNone).Should(gomega.BeFalse())
}

func TestInferenceServiceDisallowedLists(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	clientset := fakeclientset.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
		Data: map[string]string{
			InferenceServiceConfigKeyName: ISCVWithData,
		},
	})
	isvcConfigMap, err := GetInferenceServiceConfigMap(t.Context(), clientset)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	isvcConfigWithData, err := NewInferenceServicesConfig(isvcConfigMap)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	g.Expect(isvcConfigWithData).ShouldNot(gomega.BeNil())

	//nolint:gocritic
	annotations := append(constants.ServiceAnnotationDisallowedList, []string{"my.custom.annotation/1", "my.custom.annotation/2"}...)
	g.Expect(isvcConfigWithData.ServiceAnnotationDisallowedList).To(gomega.Equal(annotations))
	//nolint:gocritic
	labels := append(constants.RevisionTemplateLabelDisallowedList, []string{"my.custom.label.1", "my.custom.label.2"}...)
	g.Expect(isvcConfigWithData.ServiceLabelDisallowedList).To(gomega.Equal(labels))

	// with no data
	clientsetWithoutData := fakeclientset.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
		Data: map[string]string{
			InferenceServiceConfigKeyName: ISCVNoData,
		},
	})
	isvcConfigMap, err = GetInferenceServiceConfigMap(t.Context(), clientsetWithoutData)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	isvcConfigWithoutData, err := NewInferenceServicesConfig(isvcConfigMap)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	g.Expect(isvcConfigWithoutData).ShouldNot(gomega.BeNil())
	g.Expect(isvcConfigWithoutData.ServiceAnnotationDisallowedList).To(gomega.Equal(constants.ServiceAnnotationDisallowedList))
	g.Expect(isvcConfigWithoutData.ServiceLabelDisallowedList).To(gomega.Equal(constants.RevisionTemplateLabelDisallowedList))
}

func TestValidateIngressGateway(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	tests := []struct {
		name          string
		ingressConfig *IngressConfig
		expectedError string
	}{
		{
			name: "valid ingress gateway",
			ingressConfig: &IngressConfig{
				KserveIngressGateway: "kserve/kserve-ingress-gateway",
			},
			expectedError: "",
		},
		{
			name: "missing kserveIngressGateway",
			ingressConfig: &IngressConfig{
				KserveIngressGateway: "",
			},
			expectedError: ErrKserveIngressGatewayRequired,
		},
		{
			name: "invalid format for kserveIngressGateway",
			ingressConfig: &IngressConfig{
				KserveIngressGateway: "invalid-format",
			},
			expectedError: ErrInvalidKserveIngressGatewayFormat,
		},
		{
			name: "invalid namespace in kserveIngressGateway",
			ingressConfig: &IngressConfig{
				KserveIngressGateway: "invalid_namespace/kserve-ingress-gateway",
			},
			expectedError: ErrInvalidKserveIngressGatewayNamespace,
		},
		{
			name: "invalid name in kserveIngressGateway",
			ingressConfig: &IngressConfig{
				KserveIngressGateway: "kserve/invalid_name",
			},
			expectedError: ErrInvalidKserveIngressGatewayName,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateIngressGateway(tt.ingressConfig)
			if tt.expectedError == "" {
				g.Expect(err).ShouldNot(gomega.HaveOccurred())
			} else {
				g.Expect(err).Should(gomega.HaveOccurred())
				g.Expect(err.Error()).Should(gomega.ContainSubstring(tt.expectedError))
			}
		})
	}
}

func TestNewSecurityConfig(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	// Test with no security configuration
	clientsetNoConfig := fakeclientset.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
	})
	configMapNoConfig, err := GetInferenceServiceConfigMap(context.Background(), clientsetNoConfig)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	securityConfigEmpty, err := NewSecurityConfig(configMapNoConfig)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	g.Expect(securityConfigEmpty).ShouldNot(gomega.BeNil())
	g.Expect(securityConfigEmpty.AutoMountServiceAccountToken).To(gomega.BeFalse())

	// Test with security configuration - autoMountServiceAccountToken: true
	clientsetWithConfig := fakeclientset.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
		Data: map[string]string{
			SecurityConfigName: `{"autoMountServiceAccountToken": true}`,
		},
	})
	configMapWithConfig, err := GetInferenceServiceConfigMap(context.Background(), clientsetWithConfig)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	securityConfig, err := NewSecurityConfig(configMapWithConfig)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	g.Expect(securityConfig).ShouldNot(gomega.BeNil())
	g.Expect(securityConfig.AutoMountServiceAccountToken).To(gomega.BeTrue())

	// Test with security configuration - autoMountServiceAccountToken: false
	clientsetWithFalseConfig := fakeclientset.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
		Data: map[string]string{
			SecurityConfigName: `{"autoMountServiceAccountToken": false}`,
		},
	})
	configMapWithFalseConfig, err := GetInferenceServiceConfigMap(context.Background(), clientsetWithFalseConfig)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	securityConfigFalse, err := NewSecurityConfig(configMapWithFalseConfig)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	g.Expect(securityConfigFalse).ShouldNot(gomega.BeNil())
	g.Expect(securityConfigFalse.AutoMountServiceAccountToken).To(gomega.BeFalse())

	// Test with invalid JSON
	clientsetInvalidConfig := fakeclientset.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
		Data: map[string]string{
			SecurityConfigName: `{invalid-json}`,
		},
	})
	configMapInvalidConfig, err := GetInferenceServiceConfigMap(context.Background(), clientsetInvalidConfig)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	_, err = NewSecurityConfig(configMapInvalidConfig)
	g.Expect(err).Should(gomega.HaveOccurred())
}

func TestNewOtelCollectorConfig(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	// Test with no configuration
	clientsetNoConfig := fakeclientset.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
	})
	configMapNoConfig, err := GetInferenceServiceConfigMap(context.Background(), clientsetNoConfig)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	otelConfigEmpty, err := NewOtelCollectorConfig(configMapNoConfig)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	g.Expect(otelConfigEmpty).ShouldNot(gomega.BeNil())
	g.Expect(otelConfigEmpty.ScrapeInterval).To(gomega.BeEmpty())
	g.Expect(otelConfigEmpty.MetricReceiverEndpoint).To(gomega.BeEmpty())
	g.Expect(otelConfigEmpty.MetricScalerEndpoint).To(gomega.BeEmpty())

	// Test with valid configuration
	validConfig := `{
		"scrapeInterval": "15s",
		"metricReceiverEndpoint": "http://otel-collector:4318",
		"metricScalerEndpoint": "http://metric-scaler:8080"
	}`
	clientsetValidConfig := fakeclientset.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
		Data: map[string]string{
			OtelCollectorConfigName: validConfig,
		},
	})
	configMapValidConfig, err := GetInferenceServiceConfigMap(context.Background(), clientsetValidConfig)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	otelConfig, err := NewOtelCollectorConfig(configMapValidConfig)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	g.Expect(otelConfig).ShouldNot(gomega.BeNil())
	g.Expect(otelConfig.ScrapeInterval).To(gomega.Equal("15s"))
	g.Expect(otelConfig.MetricReceiverEndpoint).To(gomega.Equal("http://otel-collector:4318"))
	g.Expect(otelConfig.MetricScalerEndpoint).To(gomega.Equal("http://metric-scaler:8080"))

	// Test with partial configuration
	partialConfig := `{
		"scrapeInterval": "30s"
	}`
	clientsetPartialConfig := fakeclientset.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
		Data: map[string]string{
			OtelCollectorConfigName: partialConfig,
		},
	})
	configMapPartialConfig, err := GetInferenceServiceConfigMap(context.Background(), clientsetPartialConfig)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	otelPartialConfig, err := NewOtelCollectorConfig(configMapPartialConfig)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	g.Expect(otelPartialConfig).ShouldNot(gomega.BeNil())
	g.Expect(otelPartialConfig.ScrapeInterval).To(gomega.Equal("30s"))
	g.Expect(otelPartialConfig.MetricReceiverEndpoint).To(gomega.BeEmpty())
	g.Expect(otelPartialConfig.MetricScalerEndpoint).To(gomega.BeEmpty())

	// Test with invalid JSON
	invalidConfig := `{invalid-json}`
	clientsetInvalidConfig := fakeclientset.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
		Data: map[string]string{
			OtelCollectorConfigName: invalidConfig,
		},
	})
	configMapInvalidConfig, err := GetInferenceServiceConfigMap(context.Background(), clientsetInvalidConfig)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	_, err = NewOtelCollectorConfig(configMapInvalidConfig)
	g.Expect(err).Should(gomega.HaveOccurred())
	g.Expect(err.Error()).Should(gomega.ContainSubstring("unable to parse otel config json"))
}

func TestNewDeployConfigWithNoData(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	clientset := fakeclientset.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
		Data:       map[string]string{},
	})

	configMap, err := GetInferenceServiceConfigMap(context.Background(), clientset)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())

	deployConfig, err := NewDeployConfig(configMap)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	g.Expect(deployConfig).ShouldNot(gomega.BeNil())
	g.Expect(deployConfig.DefaultDeploymentMode).To(gomega.Equal(""))
}

func TestNewDeployConfigWithValidDeploymentModes(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	testCases := []struct {
		name        string
		mode        string
		expectError bool
	}{
		{
			name:        "Serverless mode",
			mode:        string(constants.Serverless),
			expectError: false,
		},
		{
			name:        "RawDeployment mode",
			mode:        string(constants.RawDeployment),
			expectError: false,
		},
		{
			name:        "ModelMesh mode",
			mode:        string(constants.ModelMeshDeployment),
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			clientset := fakeclientset.NewSimpleClientset(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
				Data: map[string]string{
					DeployConfigName: fmt.Sprintf(`{"defaultDeploymentMode": "%s"}`, tc.mode),
				},
			})

			configMap, err := GetInferenceServiceConfigMap(context.Background(), clientset)
			g.Expect(err).ShouldNot(gomega.HaveOccurred())

			deployConfig, err := NewDeployConfig(configMap)
			if tc.expectError {
				g.Expect(err).Should(gomega.HaveOccurred())
			} else {
				g.Expect(err).ShouldNot(gomega.HaveOccurred())
				g.Expect(deployConfig).ShouldNot(gomega.BeNil())
				g.Expect(deployConfig.DefaultDeploymentMode).To(gomega.Equal(tc.mode))
			}
		})
	}
}

func TestNewDeployConfigWithInvalidData(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	testCases := []struct {
		name           string
		configData     string
		expectedErrMsg string
	}{
		{
			name:           "Empty deployment mode",
			configData:     `{"defaultDeploymentMode": ""}`,
			expectedErrMsg: "defaultDeploymentMode is required",
		},
		{
			name:           "Invalid deployment mode",
			configData:     `{"defaultDeploymentMode": "InvalidMode"}`,
			expectedErrMsg: "invalid deployment mode",
		},
		{
			name:           "Invalid JSON",
			configData:     `{invalid-json}`,
			expectedErrMsg: "unable to parse deploy config json",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			clientset := fakeclientset.NewSimpleClientset(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
				Data: map[string]string{
					DeployConfigName: tc.configData,
				},
			})

			configMap, err := GetInferenceServiceConfigMap(context.Background(), clientset)
			g.Expect(err).ShouldNot(gomega.HaveOccurred())

			_, err = NewDeployConfig(configMap)
			g.Expect(err).Should(gomega.HaveOccurred())
			g.Expect(err.Error()).Should(gomega.ContainSubstring(tc.expectedErrMsg))
		})
	}
}

func TestNewDeployConfigWithValidData(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	clientset := fakeclientset.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
		Data: map[string]string{
			DeployConfigName: `{"defaultDeploymentMode": "Serverless"}`,
		},
	})

	configMap, err := GetInferenceServiceConfigMap(context.Background(), clientset)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())

	deployConfig, err := NewDeployConfig(configMap)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	g.Expect(deployConfig).ShouldNot(gomega.BeNil())
	g.Expect(deployConfig.DefaultDeploymentMode).To(gomega.Equal("Serverless"))
}

func TestNewIngressConfigWithMinimumRequiredData(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	clientset := fakeclientset.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
		Data: map[string]string{
			IngressConfigKeyName: `{"ingressGateway": "test-gateway"}`,
		},
	})

	configMap, err := GetInferenceServiceConfigMap(context.Background(), clientset)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())

	ingressConfig, err := NewIngressConfig(configMap)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	g.Expect(ingressConfig).ShouldNot(gomega.BeNil())
	g.Expect(ingressConfig.IngressGateway).To(gomega.Equal("test-gateway"))
	g.Expect(ingressConfig.DomainTemplate).To(gomega.Equal(DefaultDomainTemplate))
	g.Expect(ingressConfig.IngressDomain).To(gomega.Equal(DefaultIngressDomain))
	g.Expect(ingressConfig.UrlScheme).To(gomega.Equal(DefaultUrlScheme))
}

func TestNewIngressConfigWithGatewayAPI(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	tests := []struct {
		name          string
		ingressConfig string
		expectError   bool
		errorMessage  string
	}{
		{
			name: "valid gateway API config",
			ingressConfig: `{
				"enableGatewayApi": true,
				"kserveIngressGateway": "kserve/gateway",
				"ingressGateway": "test-gateway"
			}`,
			expectError: false,
		},
		{
			name: "missing kserveIngressGateway",
			ingressConfig: `{
				"enableGatewayApi": true,
				"ingressGateway": "test-gateway"
			}`,
			expectError:  true,
			errorMessage: "kserveIngressGateway is required",
		},
		{
			name: "invalid kserveIngressGateway format",
			ingressConfig: `{
				"enableGatewayApi": true,
				"kserveIngressGateway": "invalid-format",
				"ingressGateway": "test-gateway"
			}`,
			expectError:  true,
			errorMessage: ErrInvalidKserveIngressGatewayFormat,
		},
		{
			name: "invalid kserveIngressGateway namespace",
			ingressConfig: `{
				"enableGatewayApi": true,
				"kserveIngressGateway": "invalid_namespace/gateway",
				"ingressGateway": "test-gateway"
			}`,
			expectError:  true,
			errorMessage: ErrInvalidKserveIngressGatewayNamespace,
		},
		{
			name: "invalid kserveIngressGateway name",
			ingressConfig: `{
				"enableGatewayApi": true,
				"kserveIngressGateway": "kserve/invalid_name",
				"ingressGateway": "test-gateway"
			}`,
			expectError:  true,
			errorMessage: ErrInvalidKserveIngressGatewayName,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientset := fakeclientset.NewSimpleClientset(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
				Data: map[string]string{
					IngressConfigKeyName: tt.ingressConfig,
				},
			})

			configMap, err := GetInferenceServiceConfigMap(context.Background(), clientset)
			g.Expect(err).ShouldNot(gomega.HaveOccurred())

			ingressConfig, err := NewIngressConfig(configMap)
			if tt.expectError {
				g.Expect(err).Should(gomega.HaveOccurred())
				g.Expect(err.Error()).Should(gomega.ContainSubstring(tt.errorMessage))
			} else {
				g.Expect(err).ShouldNot(gomega.HaveOccurred())
				g.Expect(ingressConfig).ShouldNot(gomega.BeNil())
				g.Expect(ingressConfig.EnableGatewayAPI).To(gomega.BeTrue())
			}
		})
	}
}

func TestNewIngressConfigWithPathTemplate(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	tests := []struct {
		name          string
		ingressConfig string
		expectError   bool
		errorMessage  string
	}{
		{
			name: "valid path template config",
			ingressConfig: `{
				"ingressGateway": "test-gateway",
				"pathTemplate": "/v1/models/{{ .Name }}",
				"ingressDomain": "example.com"
			}`,
			expectError: false,
		},
		{
			name: "invalid path template",
			ingressConfig: `{
				"ingressGateway": "test-gateway",
				"pathTemplate": "/v1/models/{{ .Name",
				"ingressDomain": "example.com"
			}`,
			expectError:  true,
			errorMessage: "unable to parse pathTemplate",
		},
		{
			name: "missing ingressDomain with pathTemplate",
			ingressConfig: `{
				"ingressGateway": "test-gateway",
				"pathTemplate": "/v1/models/{{ .Name }}"
			}`,
			expectError:  true,
			errorMessage: "ingressDomain is required if pathTemplate is given",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientset := fakeclientset.NewSimpleClientset(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
				Data: map[string]string{
					IngressConfigKeyName: tt.ingressConfig,
				},
			})

			configMap, err := GetInferenceServiceConfigMap(context.Background(), clientset)
			g.Expect(err).ShouldNot(gomega.HaveOccurred())

			ingressConfig, err := NewIngressConfig(configMap)
			if tt.expectError {
				g.Expect(err).Should(gomega.HaveOccurred())
				g.Expect(err.Error()).Should(gomega.ContainSubstring(tt.errorMessage))
			} else {
				g.Expect(err).ShouldNot(gomega.HaveOccurred())
				g.Expect(ingressConfig).ShouldNot(gomega.BeNil())
			}
		})
	}
}

func TestNewIngressConfigWithFullConfiguration(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	fullConfig := `{
		"enableGatewayApi": true,
		"kserveIngressGateway": "kserve/gateway",
		"ingressGateway": "test-gateway",
		"knativeLocalGatewayService": "custom-local-gateway-service",
		"localGateway": "custom-local-gateway",
		"localGatewayService": "custom-local-gateway-service.local",
		"ingressDomain": "custom.example.com",
		"ingressClassName": "nginx",
		"additionalIngressDomains": ["additional1.com", "additional2.com"],
		"domainTemplate": "{{ .Name }}.{{ .Namespace }}.{{ .IngressDomain }}",
		"urlScheme": "https",
		"disableIstioVirtualHost": true,
		"pathTemplate": "/v1/models/{{ .Name }}",
		"disableIngressCreation": true
	}`

	clientset := fakeclientset.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
		Data: map[string]string{
			IngressConfigKeyName: fullConfig,
		},
	})

	configMap, err := GetInferenceServiceConfigMap(context.Background(), clientset)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())

	ingressConfig, err := NewIngressConfig(configMap)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	g.Expect(ingressConfig).ShouldNot(gomega.BeNil())

	// Verify all fields are properly populated
	g.Expect(ingressConfig.EnableGatewayAPI).To(gomega.BeTrue())
	g.Expect(ingressConfig.KserveIngressGateway).To(gomega.Equal("kserve/gateway"))
	g.Expect(ingressConfig.IngressGateway).To(gomega.Equal("test-gateway"))
	g.Expect(ingressConfig.KnativeLocalGatewayService).To(gomega.Equal("custom-local-gateway-service"))
	g.Expect(ingressConfig.LocalGateway).To(gomega.Equal("custom-local-gateway"))
	g.Expect(ingressConfig.LocalGatewayServiceName).To(gomega.Equal("custom-local-gateway-service.local"))
	g.Expect(ingressConfig.IngressDomain).To(gomega.Equal("custom.example.com"))
	g.Expect(*ingressConfig.IngressClassName).To(gomega.Equal("nginx"))
	g.Expect(*ingressConfig.AdditionalIngressDomains).To(gomega.Equal([]string{"additional1.com", "additional2.com"}))
	g.Expect(ingressConfig.DomainTemplate).To(gomega.Equal("{{ .Name }}.{{ .Namespace }}.{{ .IngressDomain }}"))
	g.Expect(ingressConfig.UrlScheme).To(gomega.Equal("https"))
	g.Expect(ingressConfig.DisableIstioVirtualHost).To(gomega.BeTrue())
	g.Expect(ingressConfig.PathTemplate).To(gomega.Equal("/v1/models/{{ .Name }}"))
	g.Expect(ingressConfig.DisableIngressCreation).To(gomega.BeTrue())
}

func TestNewIngressConfigWithKnativeLocalGatewayService(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	// Test when KnativeLocalGatewayService is not set, it should use LocalGatewayServiceName
	configWithoutKnative := `{
		"ingressGateway": "test-gateway",
		"localGatewayService": "local-gateway-service.local"
	}`

	clientset := fakeclientset.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
		Data: map[string]string{
			IngressConfigKeyName: configWithoutKnative,
		},
	})

	configMap, err := GetInferenceServiceConfigMap(context.Background(), clientset)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())

	ingressConfig, err := NewIngressConfig(configMap)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	g.Expect(ingressConfig).ShouldNot(gomega.BeNil())
	g.Expect(ingressConfig.KnativeLocalGatewayService).To(gomega.Equal("local-gateway-service.local"))

	// Test when both are set, it should use KnativeLocalGatewayService value
	configWithBoth := `{
		"ingressGateway": "test-gateway",
		"knativeLocalGatewayService": "knative-local-service",
		"localGatewayService": "local-gateway-service.local"
	}`

	clientset = fakeclientset.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
		Data: map[string]string{
			IngressConfigKeyName: configWithBoth,
		},
	})

	configMap, err = GetInferenceServiceConfigMap(context.Background(), clientset)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())

	ingressConfig, err = NewIngressConfig(configMap)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	g.Expect(ingressConfig).ShouldNot(gomega.BeNil())
	g.Expect(ingressConfig.KnativeLocalGatewayService).To(gomega.Equal("knative-local-service"))
}

func TestNewIngressConfigWithInvalidJSON(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	clientset := fakeclientset.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
		Data: map[string]string{
			IngressConfigKeyName: `invalid json format`,
		},
	})

	configMap, err := GetInferenceServiceConfigMap(context.Background(), clientset)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())

	_, err = NewIngressConfig(configMap)
	g.Expect(err).Should(gomega.HaveOccurred())
	g.Expect(err.Error()).Should(gomega.ContainSubstring("unable to parse ingress config json"))
}

func TestNewIngressConfigMissingIngressGateway(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	clientset := fakeclientset.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
		Data: map[string]string{
			IngressConfigKeyName: `{}`,
		},
	})

	configMap, err := GetInferenceServiceConfigMap(context.Background(), clientset)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())

	_, err = NewIngressConfig(configMap)
	g.Expect(err).Should(gomega.HaveOccurred())
	g.Expect(err.Error()).Should(gomega.ContainSubstring("ingressGateway is required"))
}

func TestNewDeployConfigWithValidDeploymentModesExtended(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	testCases := []struct {
		name              string
		deploymentMode    string
		expectedMode      string
		expectError       bool
		expectedErrorText string
	}{
		{
			name:           "valid Serverless mode",
			deploymentMode: `{"defaultDeploymentMode": "Serverless"}`,
			expectedMode:   "Serverless",
			expectError:    false,
		},
		{
			name:           "valid RawDeployment mode",
			deploymentMode: `{"defaultDeploymentMode": "RawDeployment"}`,
			expectedMode:   "RawDeployment",
			expectError:    false,
		},
		{
			name:           "valid ModelMesh mode",
			deploymentMode: `{"defaultDeploymentMode": "ModelMesh"}`,
			expectedMode:   "ModelMesh",
			expectError:    false,
		},
		{
			name:              "invalid deployment mode",
			deploymentMode:    `{"defaultDeploymentMode": "InvalidMode"}`,
			expectError:       true,
			expectedErrorText: "invalid deployment mode",
		},
		{
			name:              "empty deployment mode",
			deploymentMode:    `{"defaultDeploymentMode": ""}`,
			expectError:       true,
			expectedErrorText: "defaultDeploymentMode is required",
		},
		{
			name:              "invalid JSON",
			deploymentMode:    `{invalid-json}`,
			expectError:       true,
			expectedErrorText: "unable to parse deploy config json",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			clientset := fakeclientset.NewSimpleClientset(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
				Data: map[string]string{
					DeployConfigName: tc.deploymentMode,
				},
			})

			configMap, err := GetInferenceServiceConfigMap(context.Background(), clientset)
			g.Expect(err).ShouldNot(gomega.HaveOccurred())

			deployConfig, err := NewDeployConfig(configMap)

			if tc.expectError {
				g.Expect(err).Should(gomega.HaveOccurred())
				g.Expect(err.Error()).Should(gomega.ContainSubstring(tc.expectedErrorText))
			} else {
				g.Expect(err).ShouldNot(gomega.HaveOccurred())
				g.Expect(deployConfig).ShouldNot(gomega.BeNil())
				g.Expect(deployConfig.DefaultDeploymentMode).To(gomega.Equal(tc.expectedMode))
			}
		})
	}
}

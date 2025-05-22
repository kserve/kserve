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
	"context"
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
	}`, false)

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
	isvcConfigMap, err := GetInferenceServiceConfigMap(context.Background(), clientset)
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

	configMap, err := GetInferenceServiceConfigMap(context.Background(), clientset)
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

	configMap, err := GetInferenceServiceConfigMap(context.Background(), clientset)
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

	configMap, err := GetInferenceServiceConfigMap(context.Background(), clientset)
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
	configMap, err := GetInferenceServiceConfigMap(context.Background(), clientset)
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
	configMap, err := GetInferenceServiceConfigMap(context.Background(), clientset)
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
	isvcConfigMap, err := GetInferenceServiceConfigMap(context.Background(), clientset)
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
	isvcConfigMap, err := GetInferenceServiceConfigMap(context.Background(), empty)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	emp, err := NewServiceConfig(isvcConfigMap)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	g.Expect(emp).ShouldNot(gomega.BeNil())
	g.Expect(emp.ServiceClusterIPNone).Should(gomega.BeTrue()) // In ODH the default is <true>

	// with value
	withTrue := fakeclientset.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
		Data: map[string]string{
			ServiceConfigName: ServiceConfigData,
		},
	})
	isvcConfigMap, err = GetInferenceServiceConfigMap(context.Background(), withTrue)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	wt, err := NewServiceConfig(isvcConfigMap)

	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	g.Expect(wt).ShouldNot(gomega.BeNil())
	g.Expect(wt.ServiceClusterIPNone).Should(gomega.BeFalse())

	// no value, should be nil
	noValue := fakeclientset.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
		Data: map[string]string{
			ServiceConfigName: `{}`,
		},
	})
	isvcConfigMap, err = GetInferenceServiceConfigMap(context.Background(), noValue)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	nv, err := NewServiceConfig(isvcConfigMap)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	g.Expect(nv).ShouldNot(gomega.BeNil())
	g.Expect(nv.ServiceClusterIPNone).Should(gomega.BeTrue()) // In ODH the default is <true>
}

func TestInferenceServiceDisallowedLists(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	clientset := fakeclientset.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
		Data: map[string]string{
			InferenceServiceConfigKeyName: ISCVWithData,
		},
	})
	isvcConfigMap, err := GetInferenceServiceConfigMap(context.Background(), clientset)
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
	isvcConfigMap, err = GetInferenceServiceConfigMap(context.Background(), clientsetWithoutData)
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

func TestNewOtelCollectorConfig(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	t.Run("returns default config when otel config is missing", func(t *testing.T) {
		cm := &corev1.ConfigMap{
			Data: map[string]string{},
		}
		cfg, err := NewOtelCollectorConfig(cm)
		g.Expect(err).ShouldNot(gomega.HaveOccurred())
		g.Expect(cfg).ShouldNot(gomega.BeNil())
		g.Expect(cfg.ScrapeInterval).To(gomega.BeEmpty())
		g.Expect(cfg.MetricReceiverEndpoint).To(gomega.BeEmpty())
		g.Expect(cfg.MetricScalerEndpoint).To(gomega.BeEmpty())
	})

	t.Run("returns config when otel config is present", func(t *testing.T) {
		cm := &corev1.ConfigMap{
			Data: map[string]string{
				OtelCollectorConfigName: `{
					"scrapeInterval": "30s",
					"metricReceiverEndpoint": "localhost:4317",
					"metricScalerEndpoint": "localhost:8080"
				}`,
			},
		}
		cfg, err := NewOtelCollectorConfig(cm)
		g.Expect(err).ShouldNot(gomega.HaveOccurred())
		g.Expect(cfg).ShouldNot(gomega.BeNil())
		g.Expect(cfg.ScrapeInterval).To(gomega.Equal("30s"))
		g.Expect(cfg.MetricReceiverEndpoint).To(gomega.Equal("localhost:4317"))
		g.Expect(cfg.MetricScalerEndpoint).To(gomega.Equal("localhost:8080"))
	})

	t.Run("returns error on invalid otel config json", func(t *testing.T) {
		cm := &corev1.ConfigMap{
			Data: map[string]string{
				OtelCollectorConfigName: `invalid-json`,
			},
		}
		cfg, err := NewOtelCollectorConfig(cm)
		g.Expect(err).Should(gomega.HaveOccurred())
		g.Expect(cfg).To(gomega.BeNil())
	})
}

func TestNewDeployConfig_WithValidConfig(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	validModes := []string{
		string(constants.Serverless),
		string(constants.RawDeployment),
		string(constants.ModelMeshDeployment),
	}
	for _, mode := range validModes {
		cm := &corev1.ConfigMap{
			Data: map[string]string{
				DeployConfigName: fmt.Sprintf(`{"defaultDeploymentMode":"%s"}`, mode),
			},
		}
		cfg, err := NewDeployConfig(cm)
		g.Expect(err).ShouldNot(gomega.HaveOccurred())
		g.Expect(cfg).ShouldNot(gomega.BeNil())
		g.Expect(cfg.DefaultDeploymentMode).To(gomega.Equal(mode))
	}
}

func TestNewDeployConfig_MissingDefaultDeploymentMode(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	cm := &corev1.ConfigMap{
		Data: map[string]string{
			DeployConfigName: `{}`,
		},
	}
	cfg, err := NewDeployConfig(cm)
	g.Expect(err).Should(gomega.HaveOccurred())
	g.Expect(cfg).To(gomega.BeNil())
	g.Expect(err.Error()).To(gomega.ContainSubstring("defaultDeploymentMode is required"))
}

func TestNewDeployConfig_InvalidDeploymentMode(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	cm := &corev1.ConfigMap{
		Data: map[string]string{
			DeployConfigName: `{"defaultDeploymentMode":"invalid-mode"}`,
		},
	}
	cfg, err := NewDeployConfig(cm)
	g.Expect(err).Should(gomega.HaveOccurred())
	g.Expect(cfg).To(gomega.BeNil())
	g.Expect(err.Error()).To(gomega.ContainSubstring("invalid deployment mode"))
}

func TestNewDeployConfig_InvalidJSON(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	cm := &corev1.ConfigMap{
		Data: map[string]string{
			DeployConfigName: `invalid-json`,
		},
	}
	cfg, err := NewDeployConfig(cm)
	g.Expect(err).Should(gomega.HaveOccurred())
	g.Expect(cfg).To(gomega.BeNil())
	g.Expect(err.Error()).To(gomega.ContainSubstring("unable to parse deploy config json"))
}

func TestNewDeployConfig_EmptyConfigMap(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	cm := &corev1.ConfigMap{
		Data: map[string]string{},
	}
	cfg, err := NewDeployConfig(cm)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	g.Expect(cfg).ShouldNot(gomega.BeNil())
	g.Expect(cfg.DefaultDeploymentMode).To(gomega.BeEmpty())
}

func TestNewLocalModelConfig(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	t.Run("returns default config when localModel config is missing", func(t *testing.T) {
		cm := &corev1.ConfigMap{
			Data: map[string]string{},
		}
		cfg, err := NewLocalModelConfig(cm)
		g.Expect(err).ShouldNot(gomega.HaveOccurred())
		g.Expect(cfg).ShouldNot(gomega.BeNil())
		g.Expect(cfg.Enabled).To(gomega.BeFalse())
		g.Expect(cfg.JobNamespace).To(gomega.BeEmpty())
	})

	t.Run("returns config when localModel config is present", func(t *testing.T) {
		fsGroup := int64(1000)
		jobTTL := int32(3600)
		reconFreq := int64(60)
		cm := &corev1.ConfigMap{
			Data: map[string]string{
				LocalModelConfigName: fmt.Sprintf(`{
					"enabled": true,
					"jobNamespace": "test-ns",
					"defaultJobImage": "test-image",
					"fsGroup": %d,
					"jobTTLSecondsAfterFinished": %d,
					"reconcilationFrequencyInSecs": %d,
					"disableVolumeManagement": true
				}`, fsGroup, jobTTL, reconFreq),
			},
		}
		cfg, err := NewLocalModelConfig(cm)
		g.Expect(err).ShouldNot(gomega.HaveOccurred())
		g.Expect(cfg).ShouldNot(gomega.BeNil())
		g.Expect(cfg.Enabled).To(gomega.BeTrue())
		g.Expect(cfg.JobNamespace).To(gomega.Equal("test-ns"))
		g.Expect(cfg.DefaultJobImage).To(gomega.Equal("test-image"))
		g.Expect(cfg.FSGroup).ToNot(gomega.BeNil())
		g.Expect(*cfg.FSGroup).To(gomega.Equal(fsGroup))
		g.Expect(cfg.JobTTLSecondsAfterFinished).ToNot(gomega.BeNil())
		g.Expect(*cfg.JobTTLSecondsAfterFinished).To(gomega.Equal(jobTTL))
		g.Expect(cfg.ReconcilationFrequencyInSecs).ToNot(gomega.BeNil())
		g.Expect(*cfg.ReconcilationFrequencyInSecs).To(gomega.Equal(reconFreq))
		g.Expect(cfg.DisableVolumeManagement).To(gomega.BeTrue())
	})

	t.Run("returns error on invalid localModel config json", func(t *testing.T) {
		cm := &corev1.ConfigMap{
			Data: map[string]string{
				LocalModelConfigName: `invalid-json`,
			},
		}
		cfg, err := NewLocalModelConfig(cm)
		g.Expect(err).Should(gomega.HaveOccurred())
		g.Expect(cfg).To(gomega.BeNil())
	})
}

func TestNewSecurityConfig(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	t.Run("returns default config when security config is missing", func(t *testing.T) {
		cm := &corev1.ConfigMap{
			Data: map[string]string{},
		}
		cfg, err := NewSecurityConfig(cm)
		g.Expect(err).ShouldNot(gomega.HaveOccurred())
		g.Expect(cfg).ShouldNot(gomega.BeNil())
		g.Expect(cfg.AutoMountServiceAccountToken).To(gomega.BeFalse())
	})

	t.Run("returns config when security config is present", func(t *testing.T) {
		cm := &corev1.ConfigMap{
			Data: map[string]string{
				SecurityConfigName: `{"autoMountServiceAccountToken": true}`,
			},
		}
		cfg, err := NewSecurityConfig(cm)
		g.Expect(err).ShouldNot(gomega.HaveOccurred())
		g.Expect(cfg).ShouldNot(gomega.BeNil())
		g.Expect(cfg.AutoMountServiceAccountToken).To(gomega.BeTrue())
	})

	t.Run("returns error on invalid security config json", func(t *testing.T) {
		cm := &corev1.ConfigMap{
			Data: map[string]string{
				SecurityConfigName: `invalid-json`,
			},
		}
		cfg, err := NewSecurityConfig(cm)
		g.Expect(err).Should(gomega.HaveOccurred())
		g.Expect(cfg).To(gomega.BeNil())
	})
}

func TestNewIngressConfig_Validation(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	t.Run("returns error on invalid ingress config json", func(t *testing.T) {
		cm := &corev1.ConfigMap{
			Data: map[string]string{
				IngressConfigKeyName: `invalid-json`,
			},
		}
		cfg, err := NewIngressConfig(cm)
		g.Expect(err).Should(gomega.HaveOccurred())
		g.Expect(cfg).To(gomega.BeNil())
	})

	t.Run("returns error if ingressGateway is missing", func(t *testing.T) {
		cm := &corev1.ConfigMap{
			Data: map[string]string{
				IngressConfigKeyName: `{
					"kserveIngressGateway": "kserve/kserve-ingress-gateway"
				}`,
			},
		}
		cfg, err := NewIngressConfig(cm)
		g.Expect(err).Should(gomega.HaveOccurred())
		g.Expect(cfg).To(gomega.BeNil())
		g.Expect(err.Error()).To(gomega.ContainSubstring("ingressGateway is required"))
	})

	t.Run("returns error if pathTemplate is invalid template", func(t *testing.T) {
		cm := &corev1.ConfigMap{
			Data: map[string]string{
				IngressConfigKeyName: `{
					"kserveIngressGateway": "kserve/kserve-ingress-gateway",
					"ingressGateway": "knative-serving/knative-ingress-gateway",
					"pathTemplate": "{{ .Name }"
				}`,
			},
		}
		cfg, err := NewIngressConfig(cm)
		g.Expect(err).Should(gomega.HaveOccurred())
		g.Expect(cfg).To(gomega.BeNil())
		g.Expect(err.Error()).To(gomega.ContainSubstring("unable to parse pathTemplate"))
	})

	t.Run("returns error if pathTemplate is set but ingressDomain is missing", func(t *testing.T) {
		cm := &corev1.ConfigMap{
			Data: map[string]string{
				IngressConfigKeyName: `{
					"kserveIngressGateway": "kserve/kserve-ingress-gateway",
					"ingressGateway": "knative-serving/knative-ingress-gateway",
					"pathTemplate": "/foo/bar"
				}`,
			},
		}
		cfg, err := NewIngressConfig(cm)
		g.Expect(err).Should(gomega.HaveOccurred())
		g.Expect(cfg).To(gomega.BeNil())
		g.Expect(err.Error()).To(gomega.ContainSubstring("ingressDomain is required if pathTemplate is given"))
	})

	t.Run("returns error if EnableGatewayAPI is true and kserveIngressGateway is missing", func(t *testing.T) {
		cm := &corev1.ConfigMap{
			Data: map[string]string{
				IngressConfigKeyName: `{
					"enableGatewayApi": true,
					"ingressGateway": "knative-serving/knative-ingress-gateway"
				}`,
			},
		}
		cfg, err := NewIngressConfig(cm)
		g.Expect(err).Should(gomega.HaveOccurred())
		g.Expect(cfg).To(gomega.BeNil())
		g.Expect(err.Error()).To(gomega.ContainSubstring("kserveIngressGateway is required"))
	})

	t.Run("returns error if EnableGatewayAPI is true and kserveIngressGateway is invalid", func(t *testing.T) {
		cm := &corev1.ConfigMap{
			Data: map[string]string{
				IngressConfigKeyName: `{
					"enableGatewayApi": true,
					"kserveIngressGateway": "invalid-format",
					"ingressGateway": "knative-serving/knative-ingress-gateway"
				}`,
			},
		}
		cfg, err := NewIngressConfig(cm)
		g.Expect(err).Should(gomega.HaveOccurred())
		g.Expect(cfg).To(gomega.BeNil())
		g.Expect(err.Error()).To(gomega.ContainSubstring("should be in the format"))
	})

	t.Run("returns config with defaults when config map is empty", func(t *testing.T) {
		cm := &corev1.ConfigMap{
			Data: map[string]string{},
		}
		cfg, err := NewIngressConfig(cm)
		g.Expect(err).ShouldNot(gomega.HaveOccurred())
		g.Expect(cfg).ShouldNot(gomega.BeNil())
		g.Expect(cfg.DomainTemplate).To(gomega.Equal(DefaultDomainTemplate))
		g.Expect(cfg.IngressDomain).To(gomega.Equal(DefaultIngressDomain))
		g.Expect(cfg.UrlScheme).To(gomega.Equal(DefaultUrlScheme))
	})

	t.Run("sets KnativeLocalGatewayService from LocalGatewayServiceName if missing", func(t *testing.T) {
		cm := &corev1.ConfigMap{
			Data: map[string]string{
				IngressConfigKeyName: `{
					"kserveIngressGateway": "kserve/kserve-ingress-gateway",
					"ingressGateway": "knative-serving/knative-ingress-gateway",
					"localGatewayService": "my-local-gateway-service"
				}`,
			},
		}
		cfg, err := NewIngressConfig(cm)
		g.Expect(err).ShouldNot(gomega.HaveOccurred())
		g.Expect(cfg).ShouldNot(gomega.BeNil())
		g.Expect(cfg.KnativeLocalGatewayService).To(gomega.Equal("my-local-gateway-service"))
	})

	t.Run("returns error if pathTemplate is valid but ingressDomain is empty", func(t *testing.T) {
		cm := &corev1.ConfigMap{
			Data: map[string]string{
				IngressConfigKeyName: `{
					"kserveIngressGateway": "kserve/kserve-ingress-gateway",
					"ingressGateway": "knative-serving/knative-ingress-gateway",
					"pathTemplate": "/foo/bar"
				}`,
			},
		}
		cfg, err := NewIngressConfig(cm)
		g.Expect(err).Should(gomega.HaveOccurred())
		g.Expect(cfg).To(gomega.BeNil())
		g.Expect(err.Error()).To(gomega.ContainSubstring("ingressDomain is required if pathTemplate is given"))
	})

	t.Run("returns config when all required fields are present", func(t *testing.T) {
		cm := &corev1.ConfigMap{
			Data: map[string]string{
				IngressConfigKeyName: `{
					"kserveIngressGateway": "kserve/kserve-ingress-gateway",
					"ingressGateway": "knative-serving/knative-ingress-gateway",
					"ingressDomain": "mydomain.com",
					"urlScheme": "https"
				}`,
			},
		}
		cfg, err := NewIngressConfig(cm)
		g.Expect(err).ShouldNot(gomega.HaveOccurred())
		g.Expect(cfg).ShouldNot(gomega.BeNil())
		g.Expect(cfg.KserveIngressGateway).To(gomega.Equal("kserve/kserve-ingress-gateway"))
		g.Expect(cfg.IngressGateway).To(gomega.Equal("knative-serving/knative-ingress-gateway"))
		g.Expect(cfg.IngressDomain).To(gomega.Equal("mydomain.com"))
		g.Expect(cfg.UrlScheme).To(gomega.Equal("https"))
	})
}

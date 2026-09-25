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
	"strconv"
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

func TestNewKernelCacheConfigDefaults(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	for _, configMap := range []*corev1.ConfigMap{
		{},
		{Data: map[string]string{KernelCacheConfigName: `{}`}},
	} {
		config, err := NewKernelCacheConfig(configMap)
		g.Expect(err).ShouldNot(gomega.HaveOccurred())
		g.Expect(config.Enabled).To(gomega.BeFalse())
		g.Expect(config.DefaultSidecarInjection).To(gomega.BeTrue())
		g.Expect(config.DefaultMountType).To(gomega.Equal(DefaultKernelCacheMountType))
		g.Expect(config.DefaultNodeGroup).To(gomega.BeEmpty())
		g.Expect(config.JobNamespace).To(gomega.Equal(DefaultKernelCacheJobNamespace))
		g.Expect(config.MCVImage).To(gomega.Equal(DefaultKernelCacheMCVImage))
		g.Expect(config.PrefetchImage).To(gomega.Equal(DefaultKernelCachePrefetchImage))
		g.Expect(config.MCVCaptureReadinessTimeoutSeconds).To(gomega.Equal(DefaultKernelCacheMCVCaptureReadinessTimeoutSeconds))
		g.Expect(config.AbandonedCapturePolicy).To(gomega.Equal(DefaultKernelCacheAbandonedCapturePolicy))
		g.Expect(config.Registry.Endpoint).To(gomega.BeEmpty())
		g.Expect(config.Registry.CAConfigMapRef).To(gomega.BeNil())
		g.Expect(config.Registry.Auth.Type).To(gomega.Equal(KernelCacheRegistryAuthTypeNone))
		g.Expect(config.Registry.Auth.TokenTTLSeconds).To(gomega.Equal(int64(0)))
		g.Expect(config.Registry.Auth.PushRoleRef).To(gomega.BeNil())
		g.Expect(config.Registry.Auth.PullRoleRef).To(gomega.BeNil())
		g.Expect(config.JobTTLSecondsAfterFinished).ToNot(gomega.BeNil())
		g.Expect(*config.JobTTLSecondsAfterFinished).To(gomega.Equal(DefaultKernelCacheJobTTLSeconds))
		g.Expect(config.ReconcileIntervalSeconds).ToNot(gomega.BeNil())
		g.Expect(*config.ReconcileIntervalSeconds).To(gomega.Equal(DefaultKernelCacheReconcileIntervalSeconds))
	}
}

func TestNewKernelCacheConfigUsesServiceAccountTokenRegistry(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	configMap := &corev1.ConfigMap{Data: map[string]string{
		KernelCacheConfigName: `{
			"registry": {
				"endpoint": "registry.example:5000",
				"caConfigMapRef": {"name": "custom-ca", "key": "bundle.pem"},
				"auth": {
					"type": "serviceAccountToken",
					"pushRoleRef": {"kind": "ClusterRole", "name": "registry-pusher"},
					"pullRoleRef": {"kind": "ClusterRole", "name": "registry-puller"}
				}
			}
		}`,
	}}

	config, err := NewKernelCacheConfig(configMap)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	g.Expect(config.Registry.Endpoint).To(gomega.Equal("registry.example:5000"))
	g.Expect(config.Registry.CAConfigMapRef).To(gomega.Equal(&KernelCacheConfigMapKeyRef{
		Name: "custom-ca",
		Key:  "bundle.pem",
	}))
	g.Expect(config.Registry.Auth.Type).To(gomega.Equal(KernelCacheRegistryAuthTypeServiceAccountToken))
	g.Expect(config.Registry.Auth.TokenTTLSeconds).To(gomega.Equal(DefaultKernelCacheRegistryTokenTTLSeconds))
	g.Expect(config.Registry.Auth.PushRoleRef).To(gomega.Equal(&KernelCacheRegistryRoleRef{
		Kind: "ClusterRole",
		Name: "registry-pusher",
	}))
	g.Expect(config.Registry.Auth.PullRoleRef).To(gomega.Equal(&KernelCacheRegistryRoleRef{
		Kind: "ClusterRole",
		Name: "registry-puller",
	}))
}

func TestNewKernelCacheConfigRejectsUnsupportedRegistryAuthType(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	configMap := &corev1.ConfigMap{Data: map[string]string{
		KernelCacheConfigName: `{"registry":{"auth":{"type":"unsupported"}}}`,
	}}

	_, err := NewKernelCacheConfig(configMap)
	g.Expect(err).To(gomega.MatchError(`unsupported registry.auth.type "unsupported"`))
}

func TestKernelCacheRegistryConfigRejectsInvalidServiceAccountTokenSettings(t *testing.T) {
	tests := []struct {
		name        string
		config      KernelCacheRegistryConfig
		expectedErr string
	}{
		{
			name: "missing endpoint",
			config: KernelCacheRegistryConfig{
				Auth: KernelCacheRegistryAuth{
					Type: KernelCacheRegistryAuthTypeServiceAccountToken,
				},
			},
			expectedErr: "registry.endpoint must be a registry host with optional port",
		},
		{
			name: "missing role reference",
			config: KernelCacheRegistryConfig{
				Endpoint: "registry.example:5000",
				Auth: KernelCacheRegistryAuth{
					Type: KernelCacheRegistryAuthTypeServiceAccountToken,
				},
			},
			expectedErr: "registry.auth.pushRoleRef requires kind and name",
		},
		{
			name: "invalid token ttl",
			config: KernelCacheRegistryConfig{
				Endpoint: "registry.example:5000",
				Auth: KernelCacheRegistryAuth{
					Type:            KernelCacheRegistryAuthTypeServiceAccountToken,
					TokenTTLSeconds: 300,
					PushRoleRef:     &KernelCacheRegistryRoleRef{Kind: "Role", Name: "pusher"},
					PullRoleRef:     &KernelCacheRegistryRoleRef{Kind: "Role", Name: "puller"},
				},
			},
			expectedErr: "registry.auth.tokenTTLSeconds must be between 600 and 3600",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			g := gomega.NewWithT(t)
			g.Expect(test.config.Validate()).To(gomega.MatchError(test.expectedErr))
		})
	}
}

func TestNewKernelCacheConfigRejectsInvalidJSON(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	configMap := &corev1.ConfigMap{
		Data: map[string]string{KernelCacheConfigName: `not-json`},
	}

	config, err := NewKernelCacheConfig(configMap)
	g.Expect(config).To(gomega.BeNil())
	g.Expect(err).To(gomega.HaveOccurred())
	g.Expect(err.Error()).To(gomega.ContainSubstring("unable to unmarshal kernelcache"))
}

func TestNewKernelCacheConfigRejectsInvalidMCVCaptureReadinessTimeout(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	configMap := &corev1.ConfigMap{Data: map[string]string{
		KernelCacheConfigName: `{"mcvCaptureReadinessTimeoutSeconds":0}`,
	}}

	_, err := NewKernelCacheConfig(configMap)
	g.Expect(err).To(gomega.MatchError("kernelcache.mcvCaptureReadinessTimeoutSeconds must be greater than zero"))
}

func TestNewKernelCacheConfigRejectsPVCDefaultMountType(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	configMap := &corev1.ConfigMap{Data: map[string]string{
		KernelCacheConfigName: `{"defaultMountType":"pvc"}`,
	}}

	_, err := NewKernelCacheConfig(configMap)
	g.Expect(err).To(gomega.MatchError(`kernelcache.defaultMountType must be oci, got "pvc"`))
}

func TestNewKernelCacheConfigUsesConfiguredMCVCaptureReadinessTimeout(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	configMap := &corev1.ConfigMap{Data: map[string]string{
		KernelCacheConfigName: `{"mcvCaptureReadinessTimeoutSeconds":900}`,
	}}

	config, err := NewKernelCacheConfig(configMap)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	g.Expect(config.MCVCaptureReadinessTimeoutSeconds).To(gomega.Equal(int64(900)))
}

func TestNewKernelCacheConfigUsesConfiguredValues(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	configMap := &corev1.ConfigMap{Data: map[string]string{
		KernelCacheConfigName: `{
			"enabled": true,
			"defaultSidecarInjection": false,
			"defaultMountType": "oci",
			"defaultNodeGroup": "gpu-nodes",
			"jobNamespace": "kernel-cache-jobs",
			"mcvImage": "example/mcv:test",
			"mcvCaptureReadinessTimeoutSeconds": 900,
			"prefetchImage": "example/prefetch:test",
			"jobTTLSecondsAfterFinished": 600,
			"reconcileIntervalSeconds": 300,
			"abandonedCapturePolicy": "delete"
		}`,
	}}

	config, err := NewKernelCacheConfig(configMap)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	g.Expect(config.Enabled).To(gomega.BeTrue())
	g.Expect(config.DefaultSidecarInjection).To(gomega.BeFalse())
	g.Expect(config.DefaultMountType).To(gomega.Equal("oci"))
	g.Expect(config.DefaultNodeGroup).To(gomega.Equal("gpu-nodes"))
	g.Expect(config.JobNamespace).To(gomega.Equal("kernel-cache-jobs"))
	g.Expect(config.MCVImage).To(gomega.Equal("example/mcv:test"))
	g.Expect(config.MCVCaptureReadinessTimeoutSeconds).To(gomega.Equal(int64(900)))
	g.Expect(config.PrefetchImage).To(gomega.Equal("example/prefetch:test"))
	g.Expect(config.JobTTLSecondsAfterFinished).ToNot(gomega.BeNil())
	g.Expect(*config.JobTTLSecondsAfterFinished).To(gomega.Equal(int32(600)))
	g.Expect(config.ReconcileIntervalSeconds).ToNot(gomega.BeNil())
	g.Expect(*config.ReconcileIntervalSeconds).To(gomega.Equal(int64(300)))
	g.Expect(config.AbandonedCapturePolicy).To(gomega.Equal("delete"))
}

func TestNewKernelCacheConfigRestoresDefaultsForEmptyValues(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	configMap := &corev1.ConfigMap{Data: map[string]string{
		KernelCacheConfigName: `{
			"defaultMountType": "",
			"jobNamespace": "",
			"jobTTLSecondsAfterFinished": null,
			"reconcileIntervalSeconds": null
		}`,
	}}

	config, err := NewKernelCacheConfig(configMap)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	g.Expect(config.DefaultMountType).To(gomega.Equal(DefaultKernelCacheMountType))
	g.Expect(config.JobNamespace).To(gomega.Equal(DefaultKernelCacheJobNamespace))
	g.Expect(*config.JobTTLSecondsAfterFinished).To(gomega.Equal(DefaultKernelCacheJobTTLSeconds))
	g.Expect(*config.ReconcileIntervalSeconds).To(gomega.Equal(DefaultKernelCacheReconcileIntervalSeconds))
}

func TestNewKernelCacheConfigRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name        string
		config      string
		expectedErr string
	}{
		{
			name:        "invalid mount type",
			config:      `{"defaultMountType":"pvc"}`,
			expectedErr: `kernelcache.defaultMountType must be oci, got "pvc"`,
		},
		{
			name:        "invalid readiness timeout",
			config:      `{"mcvCaptureReadinessTimeoutSeconds":0}`,
			expectedErr: "kernelcache.mcvCaptureReadinessTimeoutSeconds must be greater than zero",
		},
		{
			name:        "invalid abandoned capture policy",
			config:      `{"abandonedCapturePolicy":"invalid"}`,
			expectedErr: "kernelcache.abandonedCapturePolicy must be retain or delete",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			g := gomega.NewWithT(t)
			configMap := &corev1.ConfigMap{Data: map[string]string{
				KernelCacheConfigName: test.config,
			}}

			_, err := NewKernelCacheConfig(configMap)
			g.Expect(err).To(gomega.MatchError(test.expectedErr))
		})
	}
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
		string(constants.Knative),
		string(constants.Standard),
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

func TestNewDeployConfig_LegacyDeploymentMode(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	validModes := []string{
		string(constants.LegacyRawDeployment),
		string(constants.LegacyServerless),
	}
	expected := []string{
		string(constants.Standard),
		string(constants.Knative),
	}
	for i, mode := range validModes {
		cm := &corev1.ConfigMap{
			Data: map[string]string{
				DeployConfigName: fmt.Sprintf(`{"defaultDeploymentMode":"%s"}`, mode),
			},
		}
		cfg, err := NewDeployConfig(cm)
		g.Expect(err).ShouldNot(gomega.HaveOccurred())
		g.Expect(cfg).ShouldNot(gomega.BeNil())
		g.Expect(cfg.DefaultDeploymentMode).To(gomega.Equal(expected[i]))
	}
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

func TestGetStorageInitializerConfigs(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	baseJSON := func(extra string) string {
		return `{"image":"kserve/storage-initializer:latest","memoryRequest":"100Mi","memoryLimit":"1Gi","cpuRequest":"100m","cpuLimit":"1","caBundleConfigMapName":"","caBundleVolumeMountPath":"/etc/ssl/custom-certs"` + extra + `}`
	}

	t.Run("parses new OCI fields when both set", func(t *testing.T) {
		cm := &corev1.ConfigMap{
			Data: map[string]string{
				StorageInitializerConfigMapKeyName: baseJSON(`,"enableOciModelSupport":true,"ociModelMode":"native"`),
			},
		}
		cfg, err := GetStorageInitializerConfigs(cm)
		g.Expect(err).ShouldNot(gomega.HaveOccurred())
		g.Expect(cfg.EnableOciModelSupport).To(gomega.BeTrue())
		g.Expect(cfg.OciModelMode).To(gomega.Equal("native"))
	})

	t.Run("backcompat: only enableModelcar=true, no ociModelMode", func(t *testing.T) {
		cm := &corev1.ConfigMap{
			Data: map[string]string{
				StorageInitializerConfigMapKeyName: baseJSON(`,"enableModelcar":true`),
			},
		}
		cfg, err := GetStorageInitializerConfigs(cm)
		g.Expect(err).ShouldNot(gomega.HaveOccurred())
		g.Expect(cfg.EnableOciImageSource).To(gomega.BeTrue())
		g.Expect(cfg.OciModelMode).To(gomega.Equal(""))
	})

	t.Run("neither OCI flag set: both false and mode empty", func(t *testing.T) {
		cm := &corev1.ConfigMap{
			Data: map[string]string{
				StorageInitializerConfigMapKeyName: baseJSON(``),
			},
		}
		cfg, err := GetStorageInitializerConfigs(cm)
		g.Expect(err).ShouldNot(gomega.HaveOccurred())
		g.Expect(cfg.EnableOciImageSource).To(gomega.BeFalse())
		g.Expect(cfg.EnableOciModelSupport).To(gomega.BeFalse())
		g.Expect(cfg.OciModelMode).To(gomega.Equal(""))
	})

	t.Run("invalid ociModelMode returns error", func(t *testing.T) {
		cm := &corev1.ConfigMap{
			Data: map[string]string{
				StorageInitializerConfigMapKeyName: baseJSON(`,"ociModelMode":"invalid-mode"`),
			},
		}
		_, err := GetStorageInitializerConfigs(cm)
		g.Expect(err).Should(gomega.HaveOccurred())
		g.Expect(err.Error()).To(gomega.ContainSubstring("ociModelMode"))
		g.Expect(err.Error()).To(gomega.ContainSubstring("invalid-mode"))
	})

	t.Run("ociModelMode fetch is valid", func(t *testing.T) {
		cm := &corev1.ConfigMap{
			Data: map[string]string{
				StorageInitializerConfigMapKeyName: baseJSON(`,"ociModelMode":"fetch"`),
			},
		}
		cfg, err := GetStorageInitializerConfigs(cm)
		g.Expect(err).ShouldNot(gomega.HaveOccurred())
		g.Expect(cfg.OciModelMode).To(gomega.Equal("fetch"))
	})
}

func TestNewIngressConfigLoRAModelRoutingStrategy(t *testing.T) {
	for _, tt := range []struct {
		name    string
		value   string // omitted from the ingress JSON when empty
		want    string
		wantErr string
	}{
		{name: "defaults to exact when omitted", want: constants.LoRAModelRoutingStrategyExact},
		{name: "normalizes case and whitespace", value: " ReGeX ", want: constants.LoRAModelRoutingStrategyRegex},
		{name: "rejects unsupported values", value: "regexp", wantErr: `loraModelRoutingStrategy must be "exact" or "regex", got "regexp"`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			g := gomega.NewGomegaWithT(t)
			ingress := `{"ingressGateway": "knative-serving/knative-ingress-gateway"`
			if tt.value != "" {
				ingress += `, "loraModelRoutingStrategy": ` + strconv.Quote(tt.value)
			}
			ingress += `}`

			cfg, err := NewIngressConfig(&corev1.ConfigMap{Data: map[string]string{IngressConfigKeyName: ingress}})

			if tt.wantErr != "" {
				g.Expect(err).To(gomega.MatchError(gomega.ContainSubstring(tt.wantErr)))
				g.Expect(cfg).To(gomega.BeNil())
				return
			}
			g.Expect(err).ToNot(gomega.HaveOccurred())
			g.Expect(cfg.LoRAModelRoutingStrategy).To(gomega.Equal(tt.want))
		})
	}
}

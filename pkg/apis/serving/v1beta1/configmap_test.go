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
	g.Expect(wt.ServiceClusterIPNone).Should(gomega.BeTrue())

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

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
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakeclientset "k8s.io/client-go/kubernetes/fake"

	"github.com/kserve/kserve/pkg/constants"
)

var (
	KnativeIngressGateway      = "knative-serving/knative-ingress-gateway"
	KnativeLocalGatewayService = "test-destination"
	KnativeLocalGateway        = "knative-serving/knative-local-gateway"
	LocalGatewayService        = "knative-local-gateway.istio-system.svc.cluster.local"
	UrlScheme                  = "https"
	IngressDomain              = "example.com"
	AdditionalDomain           = "additional-example.com"
	AdditionalDomainExtra      = "additional-example-extra.com"
	IngressConfigData          = fmt.Sprintf(`{
		"ingressGateway" : "%s",
		"knativeLocalGatewayService" : "%s",
		"localGateway" : "%s",
		"localGatewayService" : "%s",
		"ingressDomain": "%s",
		"urlScheme": "https",
        "additionalIngressDomains": ["%s","%s"]
	}`, KnativeIngressGateway, KnativeLocalGatewayService, KnativeLocalGateway, LocalGatewayService, IngressDomain,
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
)

func TestNewInferenceServiceConfig(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	clientset := fakeclientset.NewSimpleClientset(&v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
	})
	isvcConfigMap, err := GetInferenceServiceConfigMap(context.Background(), clientset)
	g.Expect(err).Should(gomega.BeNil())
	isvcConfig, err := NewInferenceServicesConfig(isvcConfigMap)
	g.Expect(err).Should(gomega.BeNil())
	g.Expect(isvcConfig).ShouldNot(gomega.BeNil())
}

func TestNewIngressConfig(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	clientset := fakeclientset.NewSimpleClientset(&v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
		Data: map[string]string{
			IngressConfigKeyName: IngressConfigData,
		},
	})
	configMap, err := GetInferenceServiceConfigMap(context.Background(), clientset)
	g.Expect(err).Should(gomega.BeNil())
	ingressCfg, err := NewIngressConfig(configMap)
	g.Expect(err).Should(gomega.BeNil())
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
	clientset := fakeclientset.NewSimpleClientset(&v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
		Data: map[string]string{
			IngressConfigKeyName: fmt.Sprintf(`{
				"ingressGateway" : "%s",
				"localGateway" : "%s",
				"localGatewayService" : "%s",
				"ingressDomain": "%s",
				"urlScheme": "https",
        		"additionalIngressDomains": ["%s","%s"]
			}`, KnativeIngressGateway, KnativeLocalGateway, LocalGatewayService, IngressDomain,
				AdditionalDomain, AdditionalDomainExtra),
		},
	})
	configMap, err := GetInferenceServiceConfigMap(context.Background(), clientset)
	g.Expect(err).Should(gomega.BeNil())
	ingressCfg, err := NewIngressConfig(configMap)
	g.Expect(err).Should(gomega.BeNil())
	g.Expect(ingressCfg).ShouldNot(gomega.BeNil())
	g.Expect(ingressCfg.KnativeLocalGatewayService).To(gomega.Equal(LocalGatewayService))
}

func TestNewDeployConfig(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	clientset := fakeclientset.NewSimpleClientset(&v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
	})
	isvcConfigMap, err := GetInferenceServiceConfigMap(context.Background(), clientset)
	g.Expect(err).Should(gomega.BeNil())
	deployConfig, err := NewDeployConfig(isvcConfigMap)
	g.Expect(err).Should(gomega.BeNil())
	g.Expect(deployConfig).ShouldNot(gomega.BeNil())
}

func TestNewServiceConfig(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	// nothing declared
	empty := fakeclientset.NewSimpleClientset(&v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
	})
	isvcConfigMap, err := GetInferenceServiceConfigMap(context.Background(), empty)
	g.Expect(err).Should(gomega.BeNil())
	emp, err := NewServiceConfig(isvcConfigMap)
	g.Expect(err).Should(gomega.BeNil())
	g.Expect(emp).ShouldNot(gomega.BeNil())

	// with value
	withTrue := fakeclientset.NewSimpleClientset(&v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
		Data: map[string]string{
			ServiceConfigName: ServiceConfigData,
		},
	})
	isvcConfigMap, err = GetInferenceServiceConfigMap(context.Background(), withTrue)
	g.Expect(err).Should(gomega.BeNil())
	wt, err := NewServiceConfig(isvcConfigMap)

	g.Expect(err).Should(gomega.BeNil())
	g.Expect(wt).ShouldNot(gomega.BeNil())
	g.Expect(wt.ServiceClusterIPNone).Should(gomega.BeTrue())

	// no value, should be nil
	noValue := fakeclientset.NewSimpleClientset(&v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
		Data: map[string]string{
			ServiceConfigName: `{}`,
		},
	})
	isvcConfigMap, err = GetInferenceServiceConfigMap(context.Background(), noValue)
	g.Expect(err).Should(gomega.BeNil())
	nv, err := NewServiceConfig(isvcConfigMap)
	g.Expect(err).Should(gomega.BeNil())
	g.Expect(nv).ShouldNot(gomega.BeNil())
	g.Expect(nv.ServiceClusterIPNone).Should(gomega.BeFalse())
}

func TestInferenceServiceDisallowedLists(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	clientset := fakeclientset.NewSimpleClientset(&v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
		Data: map[string]string{
			InferenceServiceConfigKeyName: ISCVWithData,
		},
	})
	isvcConfigMap, err := GetInferenceServiceConfigMap(context.Background(), clientset)
	g.Expect(err).Should(gomega.BeNil())
	isvcConfigWithData, err := NewInferenceServicesConfig(isvcConfigMap)
	g.Expect(err).Should(gomega.BeNil())
	g.Expect(isvcConfigWithData).ShouldNot(gomega.BeNil())

	// nolint:gocritic
	annotations := append(constants.ServiceAnnotationDisallowedList, []string{"my.custom.annotation/1", "my.custom.annotation/2"}...)
	g.Expect(isvcConfigWithData.ServiceAnnotationDisallowedList).To(gomega.Equal(annotations))
	// nolint:gocritic
	labels := append(constants.RevisionTemplateLabelDisallowedList, []string{"my.custom.label.1", "my.custom.label.2"}...)
	g.Expect(isvcConfigWithData.ServiceLabelDisallowedList).To(gomega.Equal(labels))

	// with no data
	clientsetWithoutData := fakeclientset.NewSimpleClientset(&v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
		Data: map[string]string{
			InferenceServiceConfigKeyName: ISCVNoData,
		},
	})
	isvcConfigMap, err = GetInferenceServiceConfigMap(context.Background(), clientsetWithoutData)
	g.Expect(err).Should(gomega.BeNil())
	isvcConfigWithoutData, err := NewInferenceServicesConfig(isvcConfigMap)
	g.Expect(err).Should(gomega.BeNil())
	g.Expect(isvcConfigWithoutData).ShouldNot(gomega.BeNil())
	g.Expect(isvcConfigWithoutData.ServiceAnnotationDisallowedList).To(gomega.Equal(constants.ServiceAnnotationDisallowedList))
	g.Expect(isvcConfigWithoutData.ServiceLabelDisallowedList).To(gomega.Equal(constants.RevisionTemplateLabelDisallowedList))
}

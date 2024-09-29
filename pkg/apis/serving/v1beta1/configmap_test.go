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

	"github.com/kserve/kserve/pkg/constants"
	"github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
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
)

func TestNewInferenceServiceConfig(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	clientset := fakeclientset.NewSimpleClientset(&v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
	})
	isvcConfig, err := NewInferenceServicesConfig(clientset)
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
	ingressCfg, err := NewIngressConfig(clientset)
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
	ingressCfg, err := NewIngressConfig(clientset)
	g.Expect(err).Should(gomega.BeNil())
	g.Expect(ingressCfg).ShouldNot(gomega.BeNil())
	g.Expect(ingressCfg.KnativeLocalGatewayService).To(gomega.Equal(LocalGatewayService))
}

func TestNewDeployConfig(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	clientset := fakeclientset.NewSimpleClientset(&v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
	})
	deployConfig, err := NewDeployConfig(clientset)
	g.Expect(err).Should(gomega.BeNil())
	g.Expect(deployConfig).ShouldNot(gomega.BeNil())
}

func TestNewServiceConfig(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	// nothing declared
	empty := fakeclientset.NewSimpleClientset(&v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
	})
	emp, err := NewServiceConfig(empty)
	g.Expect(err).Should(gomega.BeNil())
	g.Expect(emp).ShouldNot(gomega.BeNil())

	// with value
	withTrue := fakeclientset.NewSimpleClientset(&v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
		Data: map[string]string{
			ServiceConfigName: ServiceConfigData,
		},
	})
	wt, err := NewServiceConfig(withTrue)
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
	nv, err := NewServiceConfig(noValue)
	g.Expect(err).Should(gomega.BeNil())
	g.Expect(nv).ShouldNot(gomega.BeNil())
	g.Expect(nv.ServiceClusterIPNone).Should(gomega.BeFalse())

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
				g.Expect(err).Should(gomega.BeNil())
			} else {
				g.Expect(err).ShouldNot(gomega.BeNil())
				g.Expect(err.Error()).Should(gomega.ContainSubstring(tt.expectedError))
			}
		})
	}
}

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
	ctx "context"
	logger "log"
	"testing"

	"github.com/kserve/kserve/pkg/constants"
	"github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func createFakeClient() client.WithWatch {
	clientBuilder := fakeclient.NewClientBuilder()
	fakeClient := clientBuilder.Build()
	configMap := &v1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.InferenceServiceConfigMapName,
			Namespace: constants.KServeNamespace,
		},
		Immutable: nil,
		Data:      map[string]string{},
		BinaryData: map[string][]byte{
			ExplainerConfigKeyName: []byte(`{                                                                                                                                                                                                               │
				     "alibi": {                                                                                                                                                                                                  │
				        "image" : "kserve/alibi-explainer",                                                                                                                                                                     │
				         "defaultImageVersion": "latest"                                                                                                                                                                         │
				     },                                                                                                                                                                                                          │
				     "aix": {                                                                                                                                                                                                    │
				         "image" : "kserve/aix-explainer",                                                                                                                                                                       │
				         "defaultImageVersion": "latest"                                                                                                                                                                         │
				     },                                                                                                                                                                                                          │
				     "art": {                                                                                                                                                                                                    │
				         "image" : "kserve/art-explainer",                                                                                                                                                                       │
				         "defaultImageVersion": "latest"                                                                                                                                                                         │
				     }                                                                                                                                                                                                           │
			}`),
			IngressConfigKeyName: []byte(`{                                                                                                                                                                                                               │
     				"ingressGateway" : "knative-serving/knative-ingress-gateway",                                                                                                                                               │
     				"ingressService" : "istio-ingressgateway.istio-system.svc.cluster.local",                                                                                                                                   │
     				"localGateway" : "knative-serving/knative-local-gateway",                                                                                                                                                   │
     				"localGatewayService" : "knative-local-gateway.istio-system.svc.cluster.local",                                                                                                                             │
     				"ingressDomain"  : "example.com",                                                                                                                                                                           │
     				"ingressClassName" : "istio",                                                                                                                                                                               │
     				"domainTemplate": "{{ .Name }}-{{ .Namespace }}.{{ .IngressDomain }}",                                                                                                                                      │
     				"urlScheme": "http"                                                                                                                                                                                         │
 			}`),
			DeployConfigName: []byte(`{                                                                                                                                                                                                               │
   				"defaultDeploymentMode": "Serverless"                                                                                                                                                                         │
 			}`),
		},
	}
	err := fakeClient.Create(ctx.TODO(), configMap)
	if err != nil {
		logger.Fatalf("Unable to create configmap: %v", err)
	}
	return fakeClient
}

func TestNewInferenceServiceConfig(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	fakeClient := createFakeClient()

	isvcConfig, err := NewInferenceServicesConfig(fakeClient)
	g.Expect(err).Should(gomega.BeNil())
	g.Expect(isvcConfig).ShouldNot(gomega.BeNil())
}

func TestNewIngressConfig(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	fakeClient := createFakeClient()

	ingressCfg, err := NewIngressConfig(fakeClient)
	g.Expect(err).Should(gomega.BeNil())
	g.Expect(ingressCfg).ShouldNot(gomega.BeNil())
}

func TestNewDeployConfig(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	fakeClient := createFakeClient()

	deployConfig, err := NewDeployConfig(fakeClient)
	g.Expect(err).Should(gomega.BeNil())
	g.Expect(deployConfig).ShouldNot(gomega.BeNil())
}

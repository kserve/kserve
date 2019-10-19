/*
Copyright 2019 kubeflow.org.

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

package istio

import (
	"github.com/google/go-cmp/cmp"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha2"
	istiov1alpha3 "knative.dev/pkg/apis/istio/v1alpha3"
	"testing"
)

func TestCreateVirtualService(t *testing.T) {
	cases := []struct {
		name            string
		defaultStatus   *v1alpha2.EndpointStatusMap
		canaryStatus    *v1alpha2.EndpointStatusMap
		expectedStatus  *v1alpha2.VirtualServiceStatus
		expectedService *istiov1alpha3.VirtualService
	}{{
		name:            "empty status should not be ready",
		defaultStatus:   &v1alpha2.EndpointStatusMap{},
		canaryStatus:    nil,
		expectedStatus:  createFailedStatus(PredictorStatusUnknown, PredictorMissingMessage),
		expectedService: nil,
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			testIsvc := &v1alpha2.InferenceService{
				Spec: v1alpha2.InferenceServiceSpec{
					Default: v1alpha2.EndpointSpec{
						Predictor:   createMockPredictorSpec(tc.defaultStatus),
						Explainer:   createMockExplainerSpec(tc.defaultStatus),
						Transformer: createMockTransformerSpec(tc.defaultStatus),
					},
				},
				Status: v1alpha2.InferenceServiceStatus{
					Default: tc.defaultStatus,
					Canary:  tc.canaryStatus,
				},
			}

			if tc.canaryStatus != nil {
				canarySpec := &v1alpha2.EndpointSpec{
					Predictor:   createMockPredictorSpec(tc.canaryStatus),
					Explainer:   createMockExplainerSpec(tc.canaryStatus),
					Transformer: createMockTransformerSpec(tc.canaryStatus),
				}
				testIsvc.Spec.Canary = canarySpec
			}

			serviceBuilder := VirtualServiceBuilder{
				ingressConfig: &IngressConfig{
					IngressGateway:     "someIngressGateway",
					IngressServiceName: "someIngressServiceName",
				},
			}
			actualService, actualStatus := serviceBuilder.CreateVirtualService(testIsvc)

			if diff := cmp.Diff(tc.expectedStatus, actualStatus); diff != "" {
				t.Errorf("Test %q unexpected status (-want +got): %v", tc.name, diff)
			}

			if diff := cmp.Diff(tc.expectedService, actualService); diff != "" {
				t.Errorf("Test %q unexpected service (-want +got): %v", tc.name, diff)
			}
		})
	}
}

func createMockPredictorSpec(endpointStatusMap *v1alpha2.EndpointStatusMap) v1alpha2.PredictorSpec {
	return v1alpha2.PredictorSpec{}
}
func createMockExplainerSpec(endpointStatusMap *v1alpha2.EndpointStatusMap) *v1alpha2.ExplainerSpec {
	return nil
}
func createMockTransformerSpec(endpointStatusMap *v1alpha2.EndpointStatusMap) *v1alpha2.TransformerSpec {
	return nil
}

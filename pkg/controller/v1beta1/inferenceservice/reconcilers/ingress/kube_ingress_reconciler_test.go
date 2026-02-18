/*
Copyright 2021 The KServe Authors.

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

package ingress

import (
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
)

func TestCreateRawIngressWithPathTemplate(t *testing.T) {
	g := NewGomegaWithT(t)
	scheme := runtime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)
	_ = netv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	ingressClassName := "istio"
	ingressConfig := &v1beta1.IngressConfig{
		IngressDomain:       "example.com",
		DomainTemplate:      "{{.IngressDomain}}",
		IngressClassName:    &ingressClassName,
		IngressPathTemplate: "/namespace/{{.Namespace}}/endpoint/{{.Name}}",
	}

	pathTypePrefix := netv1.PathTypePrefix

	testCases := map[string]struct {
		isvc          *v1beta1.InferenceService
		expectedRules []netv1.IngressRule
		shouldFail    bool
	}{
		"Predictor not ready": {
			isvc: &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-isvc",
					Namespace: "default",
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{},
				},
				Status: v1beta1.InferenceServiceStatus{
					Status: duckv1.Status{
						Conditions: []apis.Condition{
							{
								Type:   v1beta1.PredictorReady,
								Status: corev1.ConditionFalse,
							},
						},
					},
				},
			},
			expectedRules: nil,
			shouldFail:    false,
		},
		"Predictor ready": {
			isvc: &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-isvc",
					Namespace: "default",
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{},
				},
				Status: v1beta1.InferenceServiceStatus{
					Status: duckv1.Status{
						Conditions: []apis.Condition{
							{
								Type:   v1beta1.PredictorReady,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
			},
			expectedRules: []netv1.IngressRule{
				{
					Host: "example.com",
					IngressRuleValue: netv1.IngressRuleValue{
						HTTP: &netv1.HTTPIngressRuleValue{
							Paths: []netv1.HTTPIngressPath{
								{
									Path:     "/namespace/default/endpoint/my-isvc",
									PathType: &pathTypePrefix,
									Backend: netv1.IngressBackend{
										Service: &netv1.IngressServiceBackend{
											Name: "my-isvc-predictor",
											Port: netv1.ServiceBackendPort{Number: 80},
										},
									},
								},
							},
						},
					},
				},
				{
					Host: "example.com",
					IngressRuleValue: netv1.IngressRuleValue{
						HTTP: &netv1.HTTPIngressRuleValue{
							Paths: []netv1.HTTPIngressPath{
								{
									Path:     "/namespace/default/endpoint/my-isvc-predictor",
									PathType: &pathTypePrefix,
									Backend: netv1.IngressBackend{
										Service: &netv1.IngressServiceBackend{
											Name: "my-isvc-predictor",
											Port: netv1.ServiceBackendPort{Number: 80},
										},
									},
								},
							},
						},
					},
				},
			},
			shouldFail: false,
		},
		"Transformer defined but not ready": {
			isvc: &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-isvc",
					Namespace: "default",
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor:   v1beta1.PredictorSpec{},
					Transformer: &v1beta1.TransformerSpec{},
				},
				Status: v1beta1.InferenceServiceStatus{
					Status: duckv1.Status{
						Conditions: []apis.Condition{
							{
								Type:   v1beta1.PredictorReady,
								Status: corev1.ConditionTrue,
							},
							{
								Type:   v1beta1.TransformerReady,
								Status: corev1.ConditionFalse,
							},
						},
					},
				},
			},
			expectedRules: nil,
			shouldFail:    false,
		},
		"Transformer ready": {
			isvc: &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-isvc",
					Namespace: "default",
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor:   v1beta1.PredictorSpec{},
					Transformer: &v1beta1.TransformerSpec{},
				},
				Status: v1beta1.InferenceServiceStatus{
					Status: duckv1.Status{
						Conditions: []apis.Condition{
							{
								Type:   v1beta1.PredictorReady,
								Status: corev1.ConditionTrue,
							},
							{
								Type:   v1beta1.TransformerReady,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
			},
			expectedRules: []netv1.IngressRule{
				{
					Host: "example.com",
					IngressRuleValue: netv1.IngressRuleValue{
						HTTP: &netv1.HTTPIngressRuleValue{
							Paths: []netv1.HTTPIngressPath{
								{
									Path:     "/namespace/default/endpoint/my-isvc",
									PathType: &pathTypePrefix,
									Backend: netv1.IngressBackend{
										Service: &netv1.IngressServiceBackend{
											Name: "my-isvc-transformer",
											Port: netv1.ServiceBackendPort{Number: 80},
										},
									},
								},
							},
						},
					},
				},
				{
					Host: "example.com",
					IngressRuleValue: netv1.IngressRuleValue{
						HTTP: &netv1.HTTPIngressRuleValue{
							Paths: []netv1.HTTPIngressPath{
								{
									Path:     "/namespace/default/endpoint/my-isvc-transformer",
									PathType: &pathTypePrefix,
									Backend: netv1.IngressBackend{
										Service: &netv1.IngressServiceBackend{
											Name: "my-isvc-predictor",
											Port: netv1.ServiceBackendPort{Number: 80},
										},
									},
								},
							},
						},
					},
				},
				{
					Host: "example.com",
					IngressRuleValue: netv1.IngressRuleValue{
						HTTP: &netv1.HTTPIngressRuleValue{
							Paths: []netv1.HTTPIngressPath{
								{
									Path:     "/namespace/default/endpoint/my-isvc-predictor",
									PathType: &pathTypePrefix,
									Backend: netv1.IngressBackend{
										Service: &netv1.IngressServiceBackend{
											Name: "my-isvc-predictor",
											Port: netv1.ServiceBackendPort{Number: 80},
										},
									},
								},
							},
						},
					},
				},
			},
			shouldFail: false,
		},
		"Explainer defined but not ready": {
			isvc: &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-isvc",
					Namespace: "default",
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{},
					Explainer: &v1beta1.ExplainerSpec{},
				},
				Status: v1beta1.InferenceServiceStatus{
					Status: duckv1.Status{
						Conditions: []apis.Condition{
							{
								Type:   v1beta1.PredictorReady,
								Status: corev1.ConditionTrue,
							},
							{
								Type:   v1beta1.ExplainerReady,
								Status: corev1.ConditionFalse,
							},
						},
					},
				},
			},
			expectedRules: nil,
			shouldFail:    false,
		},
		"Explainer ready (no transformer)": {
			isvc: &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-isvc",
					Namespace: "default",
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{},
					Explainer: &v1beta1.ExplainerSpec{},
				},
				Status: v1beta1.InferenceServiceStatus{
					Status: duckv1.Status{
						Conditions: []apis.Condition{
							{
								Type:   v1beta1.PredictorReady,
								Status: corev1.ConditionTrue,
							},
							{
								Type:   v1beta1.ExplainerReady,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
			},
			expectedRules: []netv1.IngressRule{
				{
					Host: "example.com",
					IngressRuleValue: netv1.IngressRuleValue{
						HTTP: &netv1.HTTPIngressRuleValue{
							Paths: []netv1.HTTPIngressPath{
								{
									Path:     "/namespace/default/endpoint/my-isvc",
									PathType: &pathTypePrefix,
									Backend: netv1.IngressBackend{
										Service: &netv1.IngressServiceBackend{
											Name: "my-isvc-predictor",
											Port: netv1.ServiceBackendPort{Number: 80},
										},
									},
								},
							},
						},
					},
				},
				{
					Host: "example.com",
					IngressRuleValue: netv1.IngressRuleValue{
						HTTP: &netv1.HTTPIngressRuleValue{
							Paths: []netv1.HTTPIngressPath{
								{
									Path:     "/namespace/default/endpoint/my-isvc-explainer",
									PathType: &pathTypePrefix,
									Backend: netv1.IngressBackend{
										Service: &netv1.IngressServiceBackend{
											Name: "my-isvc-explainer",
											Port: netv1.ServiceBackendPort{Number: 80},
										},
									},
								},
							},
						},
					},
				},
				{
					Host: "example.com",
					IngressRuleValue: netv1.IngressRuleValue{
						HTTP: &netv1.HTTPIngressRuleValue{
							Paths: []netv1.HTTPIngressPath{
								{
									Path:     "/namespace/default/endpoint/my-isvc-predictor",
									PathType: &pathTypePrefix,
									Backend: netv1.IngressBackend{
										Service: &netv1.IngressServiceBackend{
											Name: "my-isvc-predictor",
											Port: netv1.ServiceBackendPort{Number: 80},
										},
									},
								},
							},
						},
					},
				},
			},
			shouldFail: false,
		},
		"Transformer and Explainer{ ready": {
			isvc: &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-isvc",
					Namespace: "default",
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor:   v1beta1.PredictorSpec{},
					Transformer: &v1beta1.TransformerSpec{},
					Explainer:   &v1beta1.ExplainerSpec{},
				},
				Status: v1beta1.InferenceServiceStatus{
					Status: duckv1.Status{
						Conditions: []apis.Condition{
							{
								Type:   v1beta1.PredictorReady,
								Status: corev1.ConditionTrue,
							},
							{
								Type:   v1beta1.TransformerReady,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
			},
			expectedRules: []netv1.IngressRule{
				{
					Host: "example.com",
					IngressRuleValue: netv1.IngressRuleValue{
						HTTP: &netv1.HTTPIngressRuleValue{
							Paths: []netv1.HTTPIngressPath{
								{
									Path:     "/namespace/default/endpoint/my-isvc-transformer",
									PathType: &pathTypePrefix,
									Backend: netv1.IngressBackend{
										Service: &netv1.IngressServiceBackend{
											Name: "my-isvc-explainer",
											Port: netv1.ServiceBackendPort{Number: 80},
										},
									},
								},
							},
						},
					},
				},
				{
					Host: "example.com",
					IngressRuleValue: netv1.IngressRuleValue{
						HTTP: &netv1.HTTPIngressRuleValue{
							Paths: []netv1.HTTPIngressPath{
								{
									Path:     "/namespace/default/endpoint/my-isvc",
									PathType: &pathTypePrefix,
									Backend: netv1.IngressBackend{
										Service: &netv1.IngressServiceBackend{
											Name: "my-isvc-transformer",
											Port: netv1.ServiceBackendPort{Number: 80},
										},
									},
								},
							},
						},
					},
				},
				{
					Host: "example.com",
					IngressRuleValue: netv1.IngressRuleValue{
						HTTP: &netv1.HTTPIngressRuleValue{
							Paths: []netv1.HTTPIngressPath{
								{
									Path:     "/namespace/default/endpoint/my-isvc-transformer",
									PathType: &pathTypePrefix,
									Backend: netv1.IngressBackend{
										Service: &netv1.IngressServiceBackend{
											Name: "my-isvc-predictor",
											Port: netv1.ServiceBackendPort{Number: 80},
										},
									},
								},
							},
						},
					},
				},
				{
					Host: "example.com",
					IngressRuleValue: netv1.IngressRuleValue{
						HTTP: &netv1.HTTPIngressRuleValue{
							Paths: []netv1.HTTPIngressPath{
								{
									Path:     "/namespace/default/endpoint/my-isvc-predictor",
									PathType: &pathTypePrefix,
									Backend: netv1.IngressBackend{
										Service: &netv1.IngressServiceBackend{
											Name: "my-isvc-predictor",
											Port: netv1.ServiceBackendPort{Number: 80},
										},
									},
								},
							},
						},
					},
				},
			},
			shouldFail: false,
		},
	}

	// tests := []struct {
	// 	name          string
	// 	isvc          *v1beta1.InferenceService
	// 	expectedRules []netv1.IngressRule
	// 	shouldFail    bool
	// }{

	// }

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			ingress, err := createRawIngress(scheme, tc.isvc, ingressConfig, &v1beta1.InferenceServicesConfig{})
			if tc.shouldFail {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
				if tc.expectedRules == nil {
					g.Expect(ingress).To(BeNil())
				} else {
					g.Expect(ingress).ToNot(BeNil())
					g.Expect(ingress.Name).To(BeComparableTo(tc.isvc.Name))
					g.Expect(ingress.Namespace).To(BeComparableTo(tc.isvc.Namespace))
					g.Expect(ingress.Spec.IngressClassName).To(BeComparableTo(ingressConfig.IngressClassName))
					g.Expect(ingress.Spec.Rules).To(HaveLen(len(tc.expectedRules)))
					g.Expect(ingress.Spec.Rules).To(BeComparableTo(tc.expectedRules))
				}
			}
		})
	}
}

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

package knative

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	knservingv1alpha1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"github.com/knative/serving/pkg/apis/serving/v1beta1"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestKnativeRoute(t *testing.T) {
	scenarios := map[string]struct {
		kfService     v1alpha1.KFService
		expectedRoute *knservingv1alpha1.Route
		shouldFail    bool
	}{
		"RunLatestModel": {
			kfService: v1alpha1.KFService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mnist",
					Namespace: "default",
				},
				Spec: v1alpha1.KFServiceSpec{
					Default: v1alpha1.ModelSpec{
						Tensorflow: &v1alpha1.TensorflowSpec{
							ModelURI:       "s3://test/mnist/export",
							RuntimeVersion: "1.13.0",
						},
					},
				},
			},
			expectedRoute: &knservingv1alpha1.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "mnist",
					Namespace:   "default",
					Annotations: make(map[string]string),
				},
				Spec: knservingv1alpha1.RouteSpec{
					Traffic: []knservingv1alpha1.TrafficTarget{
						{
							TrafficTarget: v1beta1.TrafficTarget{
								ConfigurationName: "mnist-default",
								Percent:           100,
							},
						},
					},
				},
			},
		},
		"RunCanaryModel": {
			kfService: v1alpha1.KFService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mnist",
					Namespace: "default",
				},
				Spec: v1alpha1.KFServiceSpec{
					Default: v1alpha1.ModelSpec{
						Tensorflow: &v1alpha1.TensorflowSpec{
							ModelURI:       "s3://test/mnist/export",
							RuntimeVersion: "1.13.0",
						},
					},
					CanaryTrafficPercent: 20,
					Canary: &v1alpha1.ModelSpec{
						Tensorflow: &v1alpha1.TensorflowSpec{
							ModelURI:       "s3://test/mnist-2/export",
							RuntimeVersion: "1.13.0",
						},
					},
				},
				Status: v1alpha1.KFServiceStatus{
					Default: v1alpha1.StatusConfigurationSpec{
						Name: "v1",
					},
				},
			},
			expectedRoute: &knservingv1alpha1.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "mnist",
					Namespace:   "default",
					Annotations: make(map[string]string),
				},
				Spec: knservingv1alpha1.RouteSpec{
					Traffic: []knservingv1alpha1.TrafficTarget{
						{
							TrafficTarget: v1beta1.TrafficTarget{
								ConfigurationName: "mnist-default",
								Percent:           80,
							},
						},
						{
							TrafficTarget: v1beta1.TrafficTarget{
								ConfigurationName: "mnist-canary",
								Percent:           20,
							},
						},
					},
				},
			},
		},
		"TestAnnotations": {
			kfService: v1alpha1.KFService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mnist",
					Namespace: "default",
					Annotations: map[string]string{
						"sourceName": "srcName",
						"prop1":      "val1",
						"kubectl.kubernetes.io/last-applied-configuration": "test1",
					},
				},
				Spec: v1alpha1.KFServiceSpec{
					Default: v1alpha1.ModelSpec{
						Tensorflow: &v1alpha1.TensorflowSpec{
							ModelURI:       "s3://test/mnist/export",
							RuntimeVersion: "1.13.0",
						},
					},
					CanaryTrafficPercent: 20,
					Canary: &v1alpha1.ModelSpec{
						Tensorflow: &v1alpha1.TensorflowSpec{
							ModelURI:       "s3://test/mnist-2/export",
							RuntimeVersion: "1.13.0",
						},
					},
				},
				Status: v1alpha1.KFServiceStatus{
					Default: v1alpha1.StatusConfigurationSpec{
						Name: "v1",
					},
				},
			},
			expectedRoute: &knservingv1alpha1.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mnist",
					Namespace: "default",
					Annotations: map[string]string{
						"sourceName": "srcName",
						"prop1":      "val1",
					},
				},
				Spec: knservingv1alpha1.RouteSpec{
					Traffic: []knservingv1alpha1.TrafficTarget{
						{
							TrafficTarget: v1beta1.TrafficTarget{
								ConfigurationName: "mnist-default",
								Percent:           80,
							},
						},
						{
							TrafficTarget: v1beta1.TrafficTarget{
								ConfigurationName: "mnist-canary",
								Percent:           20,
							},
						},
					},
				},
			},
		},
	}

	for name, scenario := range scenarios {
		routeBuilder := NewRouteBuilder()
		route := routeBuilder.CreateKnativeRoute(&scenario.kfService)
		// Validate
		if scenario.shouldFail {
			t.Errorf("Test %q failed: returned success but expected error", name)
		} else {
			if diff := cmp.Diff(scenario.expectedRoute, route); diff != "" {
				t.Errorf("Test %q unexpected default configuration (-want +got): %v", name, diff)
			}
		}

	}
}

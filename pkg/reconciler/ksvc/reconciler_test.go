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

package ksvc

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	knservingv1alpha1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"github.com/knative/serving/pkg/apis/serving/v1beta1"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1"
	"github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestKnativeConfigurationReconcile(t *testing.T) {
	existingConfiguration := &knservingv1alpha1.Configuration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mnist",
			Namespace: "default",
		},
		Spec: knservingv1alpha1.ConfigurationSpec{
			RevisionTemplate: &knservingv1alpha1.RevisionTemplateSpec{
				Spec: knservingv1alpha1.RevisionSpec{
					Container: &v1.Container{
						Image: v1alpha1.TensorflowServingImageName + ":" +
							v1alpha1.DefaultTensorflowServingVersion,
						Command: []string{v1alpha1.TensorflowEntrypointCommand},
						Args: []string{
							"--port=" + v1alpha1.TensorflowServingGRPCPort,
							"--rest_api_port=" + v1alpha1.TensorflowServingRestPort,
							"--model_name=mnist",
							"--model_base_path=s3://test/mnist/export",
						},
					},
				},
			},
		},
	}
	g := gomega.NewGomegaWithT(t)
	scenarios := map[string]struct {
		desiredConfiguration *knservingv1alpha1.Configuration
		update               bool
		shouldFail           bool
	}{
		"Reconcile new model serving ": {
			update: false,
			desiredConfiguration: &knservingv1alpha1.Configuration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mnist",
					Namespace: "default",
				},
				Spec: knservingv1alpha1.ConfigurationSpec{
					RevisionTemplate: &knservingv1alpha1.RevisionTemplateSpec{
						Spec: knservingv1alpha1.RevisionSpec{
							Container: &v1.Container{
								Image: v1alpha1.TensorflowServingImageName + ":" +
									v1alpha1.DefaultTensorflowServingVersion,
								Command: []string{v1alpha1.TensorflowEntrypointCommand},
								Args: []string{
									"--port=" + v1alpha1.TensorflowServingGRPCPort,
									"--rest_api_port=" + v1alpha1.TensorflowServingRestPort,
									"--model_name=mnist",
									"--model_base_path=s3://test/mnist/export",
								},
							},
						},
					},
				},
			},
		},
		"Reconcile model path, labels and annotations update": {
			update: true,
			desiredConfiguration: &knservingv1alpha1.Configuration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mnist",
					Namespace: "default",
					Labels: map[string]string{
						"serving.knative.dev/configuration": "dream",
					},
					Annotations: map[string]string{
						"serving.knative.dev/lastPinned": "1111111111",
					},
				},
				Spec: knservingv1alpha1.ConfigurationSpec{
					RevisionTemplate: &knservingv1alpha1.RevisionTemplateSpec{
						Spec: knservingv1alpha1.RevisionSpec{
							Container: &v1.Container{
								Image: v1alpha1.TensorflowServingImageName + ":" +
									v1alpha1.DefaultTensorflowServingVersion,
								Command: []string{v1alpha1.TensorflowEntrypointCommand},
								Args: []string{
									"--port=" + v1alpha1.TensorflowServingGRPCPort,
									"--rest_api_port=" + v1alpha1.TensorflowServingRestPort,
									"--model_name=mnist",
									"--model_base_path=s3://test/mnist-v2/export",
								},
							},
						},
					},
				},
			},
		},
	}

	serviceReconciler := NewServiceReconciler(c)
	for name, scenario := range scenarios {
		if scenario.update {
			g.Expect(c.Create(context.TODO(), existingConfiguration)).NotTo(gomega.HaveOccurred())
		}
		configuration, err := serviceReconciler.ReconcileConfiguarion(context.TODO(), scenario.desiredConfiguration)
		// Validate
		if scenario.shouldFail && err == nil {
			t.Errorf("Test %q failed: returned success but expected error", name)
		}
		if !scenario.shouldFail {
			if err != nil {
				t.Errorf("Test %q failed: returned error: %v", name, err)
			}
			if diff := cmp.Diff(scenario.desiredConfiguration.Spec, configuration.Spec); diff != "" {
				t.Errorf("Test %q unexpected configuration spec (-want +got): %v", name, diff)
			}
			if diff := cmp.Diff(scenario.desiredConfiguration.ObjectMeta.Labels, configuration.ObjectMeta.Labels); diff != "" {
				t.Errorf("Test %q unexpected configuration labels (-want +got): %v", name, diff)
			}
			if diff := cmp.Diff(scenario.desiredConfiguration.ObjectMeta.Annotations, configuration.ObjectMeta.Annotations); diff != "" {
				t.Errorf("Test %q unexpected configuration annotations (-want +got): %v", name, diff)
			}
		}
		g.Expect(c.Delete(context.TODO(), existingConfiguration)).NotTo(gomega.HaveOccurred())
	}
}

func TestKnativeRouteReconcile(t *testing.T) {
	existingRoute := &knservingv1alpha1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mnist",
			Namespace: "default",
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
	}
	g := gomega.NewGomegaWithT(t)
	scenarios := map[string]struct {
		desiredRoute *knservingv1alpha1.Route
		update       bool
		shouldFail   bool
	}{
		"Reconcile new model serving": {
			update: false,
			desiredRoute: &knservingv1alpha1.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mnist",
					Namespace: "default",
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
		"Reconcile route update": {
			update: true,
			desiredRoute: &knservingv1alpha1.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mnist",
					Namespace: "default",
					Labels: map[string]string{
						"serving.knative.dev/route": "dream",
					},
					Annotations: map[string]string{
						"cherub": "rock",
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

	serviceReconciler := NewServiceReconciler(c)
	for name, scenario := range scenarios {
		if scenario.update {
			g.Expect(c.Create(context.TODO(), existingRoute)).NotTo(gomega.HaveOccurred())
		}
		route, err := serviceReconciler.ReconcileRoute(context.TODO(), scenario.desiredRoute)
		// Validate
		if scenario.shouldFail && err == nil {
			t.Errorf("Test %q failed: returned success but expected error", name)
		}
		if !scenario.shouldFail {
			if err != nil {
				t.Errorf("Test %q failed: returned error: %v", name, err)
			}
			if diff := cmp.Diff(scenario.desiredRoute.Spec, route.Spec); diff != "" {
				t.Errorf("Test %q unexpected route spec (-want +got): %v", name, diff)
			}
			if diff := cmp.Diff(scenario.desiredRoute.ObjectMeta.Labels, route.ObjectMeta.Labels); diff != "" {
				t.Errorf("Test %q unexpected route labels (-want +got): %v", name, diff)
			}
			if diff := cmp.Diff(scenario.desiredRoute.ObjectMeta.Annotations, route.ObjectMeta.Annotations); diff != "" {
				t.Errorf("Test %q unexpected route annotations (-want +got): %v", name, diff)
			}
		}
		g.Expect(c.Delete(context.TODO(), existingRoute)).NotTo(gomega.HaveOccurred())
	}
}

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
	"context"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	knservingv1alpha1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"github.com/knative/serving/pkg/apis/serving/v1beta1"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1"
	"github.com/kubeflow/kfserving/pkg/constants"
	testutils "github.com/kubeflow/kfserving/pkg/testing"
	"github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const timeout = time.Second * 5

func TestKnativeConfigurationReconcile(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	mgr, err := manager.New(cfg, manager.Options{})
	stopMgr, mgrStopped := testutils.StartTestManager(mgr, g)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	c := mgr.GetClient()

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	configurationReconciler := NewConfigurationReconciler(c, mgr.GetScheme(), &v1.ConfigMap{})

	scenarios := map[string]struct {
		kfsvc          v1alpha1.KFService
		desiredDefault knservingv1alpha1.Configuration
		desiredCanary  *knservingv1alpha1.Configuration
		update         bool
	}{

		// "Reconcile creates default and canary configurations": {
		// 	update: false,
		// 	desiredConfiguration: &knservingv1alpha1.Configuration{
		// 		ObjectMeta: metav1.ObjectMeta{
		// 			Name:      "mnist",
		// 			Namespace: "default",
		// 			Labels: map[string]string{
		// 				"serving.knative.dev/configuration": "dream",
		// 			},
		// 			Annotations: map[string]string{
		// 				"serving.knative.dev/lastPinned": "1111111111",
		// 			},
		// 		},
		// 		Spec: knservingv1alpha1.ConfigurationSpec{
		// 			RevisionTemplate: &knservingv1alpha1.RevisionTemplateSpec{
		// 				Spec: knservingv1alpha1.RevisionSpec{
		// 					Container: &v1.Container{
		// 						Image: v1alpha1.TensorflowServingImageName + ":" +
		// 							v1alpha1.DefaultTensorflowRuntimeVersion,
		// 						Command: []string{v1alpha1.TensorflowEntrypointCommand},
		// 						Args: []string{
		// 							"--port=" + v1alpha1.TensorflowServingGRPCPort,
		// 							"--rest_api_port=" + v1alpha1.TensorflowServingRestPort,
		// 							"--model_name=mnist",
		// 							"--model_base_path=s3://test/mnist-v2/export",
		// 						},
		// 					},
		// 				},
		// 			},
		// 		},
		// 	},
		// },
		// "Reconcile updates default and canary configurations": {
		// 	update: true,
		// 	desiredConfiguration: &knservingv1alpha1.Configuration{
		// 		ObjectMeta: metav1.ObjectMeta{
		// 			Name:      "mnist",
		// 			Namespace: "default",
		// 			Labels: map[string]string{
		// 				"serving.knative.dev/configuration": "dream",
		// 			},
		// 			Annotations: map[string]string{
		// 				"serving.knative.dev/lastPinned": "1111111111",
		// 			},
		// 		},
		// 		Spec: knservingv1alpha1.ConfigurationSpec{
		// 			RevisionTemplate: &knservingv1alpha1.RevisionTemplateSpec{
		// 				Spec: knservingv1alpha1.RevisionSpec{
		// 					Container: &v1.Container{
		// 						Image: v1alpha1.TensorflowServingImageName + ":" +
		// 							v1alpha1.DefaultTensorflowRuntimeVersion,
		// 						Command: []string{v1alpha1.TensorflowEntrypointCommand},
		// 						Args: []string{
		// 							"--port=" + v1alpha1.TensorflowServingGRPCPort,
		// 							"--rest_api_port=" + v1alpha1.TensorflowServingRestPort,
		// 							"--model_name=mnist",
		// 							"--model_base_path=s3://test/mnist-v2/export",
		// 						},
		// 					},
		// 				},
		// 			},
		// 		},
		// 	},
		// },
		"Reconcile ignores canary if unspecified": {
			update: false,
			kfsvc: v1alpha1.KFService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mnist",
					Namespace: "default",
				},
				Spec: v1alpha1.KFServiceSpec{
					Default: v1alpha1.ModelSpec{
						Tensorflow: &v1alpha1.TensorflowSpec{
							RuntimeVersion: v1alpha1.DefaultTensorflowRuntimeVersion,
							ModelURI:       "gs://testuri",
						},
					},
				},
			},
			desiredDefault: knservingv1alpha1.Configuration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mnist-default",
					Namespace: "default",
				},
				Spec: knservingv1alpha1.ConfigurationSpec{
					RevisionTemplate: &knservingv1alpha1.RevisionTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"serving.kubeflow.org/kfservice": "mnist"},
							Annotations: map[string]string{
								"autoscaling.knative.dev/class":  "kpa.autoscaling.knative.dev",
								"autoscaling.knative.dev/target": "1",
							},
						},
						Spec: knservingv1alpha1.RevisionSpec{
							RevisionSpec: v1beta1.RevisionSpec{
								TimeoutSeconds: &constants.DefaultTimeout,
							},
							Container: &v1.Container{
								Image:   v1alpha1.TensorflowServingImageName + ":" + v1alpha1.DefaultTensorflowRuntimeVersion,
								Command: []string{v1alpha1.TensorflowEntrypointCommand},
								Args: []string{
									"--port=" + v1alpha1.TensorflowServingGRPCPort,
									"--rest_api_port=" + v1alpha1.TensorflowServingRestPort,
									"--model_name=mnist",
									"--model_base_path=gs://testuri",
								},
							},
						},
					},
				},
			},
			desiredCanary: nil,
		},
	}
	for name, scenario := range scenarios {
		t.Logf("Scenario: %s", name)
		g.Expect(c.Create(context.TODO(), &scenario.kfsvc)).NotTo(gomega.HaveOccurred())
		defer c.Delete(context.TODO(), &scenario.kfsvc)
		if scenario.update {
			g.Expect(c.Create(context.TODO(), &scenario.desiredDefault)).NotTo(gomega.HaveOccurred())
			defer c.Delete(context.TODO(), &scenario.desiredDefault)
			if scenario.desiredCanary != nil {
				g.Expect(c.Create(context.TODO(), scenario.desiredCanary)).NotTo(gomega.HaveOccurred())
				defer c.Delete(context.TODO(), scenario.desiredCanary)
			}
		}

		if err := configurationReconciler.Reconcile(&scenario.kfsvc); err != nil {
			t.Errorf("Test %q failed: returned error: %v", name, err)
		}

		// Assert default
		actualDefault := knservingv1alpha1.Configuration{}
		g.Eventually(func() error {
			return c.Get(context.TODO(), types.NamespacedName{
				Name:      constants.DefaultConfigurationName(scenario.kfsvc.Name),
				Namespace: scenario.kfsvc.Namespace,
			}, &actualDefault)
		}, timeout).Should(gomega.Succeed())

		if diff := cmp.Diff(scenario.desiredDefault.Spec, actualDefault.Spec); diff != "" {
			t.Errorf("Test %q unexpected configuration spec (-want +got): %v", name, diff)
		}
		if diff := cmp.Diff(scenario.desiredDefault.ObjectMeta.Labels, actualDefault.ObjectMeta.Labels); diff != "" {
			t.Errorf("Test %q unexpected configuration labels (-want +got): %v", name, diff)
		}
		if diff := cmp.Diff(scenario.desiredDefault.ObjectMeta.Annotations, actualDefault.ObjectMeta.Annotations); diff != "" {
			t.Errorf("Test %q unexpected configuration annotations (-want +got): %v", name, diff)
		}

		// Assert Canary
		if scenario.desiredCanary != nil {
			actualCanary := knservingv1alpha1.Configuration{}
			g.Eventually(func() error {
				return c.Get(context.TODO(), types.NamespacedName{
					Name:      constants.CanaryConfigurationName(scenario.kfsvc.Name),
					Namespace: scenario.kfsvc.Namespace,
				}, &actualCanary)
			}, timeout).Should(gomega.Succeed())
			if diff := cmp.Diff(scenario.desiredCanary.Spec, actualCanary.Spec); diff != "" {
				t.Errorf("Test %q unexpected configuration spec (-want +got): %v", name, diff)
			}
			if diff := cmp.Diff(scenario.desiredCanary.ObjectMeta.Labels, actualCanary.ObjectMeta.Labels); diff != "" {
				t.Errorf("Test %q unexpected configuration labels (-want +got): %v", name, diff)
			}
			if diff := cmp.Diff(scenario.desiredCanary.ObjectMeta.Annotations, actualCanary.ObjectMeta.Annotations); diff != "" {
				t.Errorf("Test %q unexpected configuration annotations (-want +got): %v", name, diff)
			}
		}
	}
}

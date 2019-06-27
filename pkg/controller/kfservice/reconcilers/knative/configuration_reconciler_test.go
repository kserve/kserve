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
	"fmt"
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
	"sigs.k8s.io/controller-runtime/pkg/client"
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
		desiredDefault *knservingv1alpha1.Configuration
		desiredCanary  *knservingv1alpha1.Configuration
	}{
		"Reconcile creates default and canary configurations": {
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
					Canary: &v1alpha1.ModelSpec{
						Tensorflow: &v1alpha1.TensorflowSpec{
							RuntimeVersion: v1alpha1.DefaultTensorflowRuntimeVersion,
							ModelURI:       "gs://testuri2",
						},
					},
				},
			},
			desiredDefault: &knservingv1alpha1.Configuration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mnist-default",
					Namespace: "default",
				},
				Spec: knservingv1alpha1.ConfigurationSpec{
					RevisionTemplate: &knservingv1alpha1.RevisionTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"serving.kubeflow.org/kfservice": "mnist"},
							Annotations: map[string]string{
								"autoscaling.knative.dev/class":                             "kpa.autoscaling.knative.dev",
								"autoscaling.knative.dev/target":                            "1",
								"internal.serving.kubeflow.org/model-initializer-sourceURI": "gs://testuri",
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
									"--model_base_path=" + constants.DefaultModelLocalMountPath,
								},
							},
						},
					},
				},
			},
			desiredCanary: &knservingv1alpha1.Configuration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mnist-canary",
					Namespace: "default",
				},
				Spec: knservingv1alpha1.ConfigurationSpec{
					RevisionTemplate: &knservingv1alpha1.RevisionTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"serving.kubeflow.org/kfservice": "mnist"},
							Annotations: map[string]string{
								"autoscaling.knative.dev/class":                             "kpa.autoscaling.knative.dev",
								"autoscaling.knative.dev/target":                            "1",
								"internal.serving.kubeflow.org/model-initializer-sourceURI": "gs://testuri2",
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
									"--model_base_path=" + constants.DefaultModelLocalMountPath,
								},
							},
						},
					},
				},
			},
		},
		"Reconcile ignores canary if unspecified": {
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
			desiredDefault: &knservingv1alpha1.Configuration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mnist-default",
					Namespace: "default",
				},
				Spec: knservingv1alpha1.ConfigurationSpec{
					RevisionTemplate: &knservingv1alpha1.RevisionTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"serving.kubeflow.org/kfservice": "mnist"},
							Annotations: map[string]string{
								"autoscaling.knative.dev/class":                             "kpa.autoscaling.knative.dev",
								"autoscaling.knative.dev/target":                            "1",
								"internal.serving.kubeflow.org/model-initializer-sourceURI": "gs://testuri",
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
									"--model_base_path=" + constants.DefaultModelLocalMountPath,
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

		if err := configurationReconciler.Reconcile(&scenario.kfsvc); err != nil {
			t.Errorf("Test %q failed: returned error: %v", name, err)
		}

		g.Eventually(func() error { return awaitDesired(c, scenario.desiredDefault) }, timeout).Should(gomega.Succeed())
		g.Eventually(func() error { return awaitDesired(c, scenario.desiredCanary) }, timeout).Should(gomega.Succeed())

		g.Expect(c.Delete(context.TODO(), &scenario.kfsvc)).NotTo(gomega.HaveOccurred())
	}
}

func awaitDesired(c client.Client, desired *knservingv1alpha1.Configuration) error {
	if desired == nil {
		return nil
	}
	actual := knservingv1alpha1.Configuration{}
	if err := c.Get(context.TODO(), types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, &actual); err != nil {
		return err
	}
	if diff := cmp.Diff(desired.Spec, actual.Spec); diff != "" {
		return fmt.Errorf("Unexpected configuration spec (-want +got): %v", diff)
	}
	if diff := cmp.Diff(desired.ObjectMeta.Labels, actual.ObjectMeta.Labels); diff != "" {
		return fmt.Errorf("Unexpected configuration labels (-want +got): %v", diff)
	}
	if diff := cmp.Diff(desired.ObjectMeta.Annotations, actual.ObjectMeta.Annotations); diff != "" {
		return fmt.Errorf("Unexpected configuration annotations (-want +got): %v", diff)
	}
	return nil
}

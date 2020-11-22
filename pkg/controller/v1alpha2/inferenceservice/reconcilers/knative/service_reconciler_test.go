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
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha2"
	"github.com/kubeflow/kfserving/pkg/constants"
	testutils "github.com/kubeflow/kfserving/pkg/testing"
	"github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	knservingv1 "knative.dev/serving/pkg/apis/serving/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const (
	timeout                         = time.Second * 5
	DefaultTensorflowRuntimeVersion = "latest"
)

var (
	containerConcurrency int64 = 0
)

func TestKnativeServiceReconcile(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	mgr, err := manager.New(cfg, manager.Options{MetricsBindAddress: "0"})
	g.Expect(err).NotTo(gomega.HaveOccurred())
	stopMgr, mgrStopped := testutils.StartTestManager(mgr, g)
	c := mgr.GetClient()

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	TensorflowServingImageName := "tensorflow/serving"
	configs := map[string]string{
		"predictors": `{
           "tensorflow" : {
             "image" : "tensorflow/serving"
           },
           "sklearn" : {
             "image" : "kfserving/sklearnserver"
           },
           "xgboost" : {
             "image" : "kfserving/xgbserver"
           }
        }`,
		"ingress": `{
          "ingressGateway" : "test-gateway",
          "ingressService" : "test-destination"
        }`,
	}

	serviceReconciler := NewServiceReconciler(c, mgr.GetScheme(), &v1.ConfigMap{
		Data: configs,
	})
	scenarios := map[string]struct {
		isvc           v1alpha2.InferenceService
		desiredDefault *knservingv1.Service
		desiredCanary  *knservingv1.Service
	}{
		"Reconcile creates default and canary service": {
			isvc: v1alpha2.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mnist",
					Namespace: "default",
				},
				Spec: v1alpha2.InferenceServiceSpec{
					Default: v1alpha2.EndpointSpec{
						Predictor: v1alpha2.PredictorSpec{
							Tensorflow: &v1alpha2.TensorflowSpec{
								RuntimeVersion: DefaultTensorflowRuntimeVersion,
								StorageURI:     "gs://testuri",
							},
						},
					},
					Canary: &v1alpha2.EndpointSpec{
						Predictor: v1alpha2.PredictorSpec{
							Tensorflow: &v1alpha2.TensorflowSpec{
								RuntimeVersion: DefaultTensorflowRuntimeVersion,
								StorageURI:     "gs://testuri2",
							},
						},
					},
				},
			},
			desiredDefault: &knservingv1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.DefaultPredictorServiceName("mnist"),
					Namespace: "default",
				},
				Spec: knservingv1.ServiceSpec{
					ConfigurationSpec: knservingv1.ConfigurationSpec{
						Template: knservingv1.RevisionTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"serving.kubeflow.org/inferenceservice": "mnist",
									constants.KServiceEndpointLabel:  constants.InferenceServiceDefault,
									constants.KServiceModelLabel:     "mnist",
									constants.KServiceComponentLabel: constants.Predictor.String(),
								},
								Annotations: map[string]string{
									"autoscaling.knative.dev/class":                               "kpa.autoscaling.knative.dev",
									"autoscaling.knative.dev/minScale":                            "1",
									"autoscaling.knative.dev/target":                              "1",
									"internal.serving.kubeflow.org/storage-initializer-sourceuri": "gs://testuri",
								},
							},
							Spec: knservingv1.RevisionSpec{
								ContainerConcurrency: &containerConcurrency,
								TimeoutSeconds:       &constants.DefaultPredictorTimeout,
								PodSpec: v1.PodSpec{
									Containers: []v1.Container{
										{
											Image:   TensorflowServingImageName + ":" + DefaultTensorflowRuntimeVersion,
											Command: []string{v1alpha2.TensorflowEntrypointCommand},
											Name:    constants.InferenceServiceContainerName,
											Args: []string{
												"--port=" + v1alpha2.TensorflowServingGRPCPort,
												"--rest_api_port=" + v1alpha2.TensorflowServingRestPort,
												"--model_name=mnist",
												"--model_base_path=" + constants.DefaultModelLocalMountPath,
												"--rest_api_timeout_in_ms=0",
											},
											LivenessProbe: &v1.Probe{
												Handler: v1.Handler{
													HTTPGet: &v1.HTTPGetAction{
														Path: "/v1/models/mnist",
													},
												},
												InitialDelaySeconds: constants.DefaultReadinessTimeout,
												PeriodSeconds:       10,
												FailureThreshold:    3,
												SuccessThreshold:    1,
												TimeoutSeconds:      1,
											},
										},
									},
								},
							},
						},
					},
				},
			},
			desiredCanary: &knservingv1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.CanaryPredictorServiceName("mnist"),
					Namespace: "default",
				},
				Spec: knservingv1.ServiceSpec{
					ConfigurationSpec: knservingv1.ConfigurationSpec{
						Template: knservingv1.RevisionTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"serving.kubeflow.org/inferenceservice": "mnist",
									constants.KServiceEndpointLabel:  constants.InferenceServiceCanary,
									constants.KServiceModelLabel:     "mnist",
									constants.KServiceComponentLabel: constants.Predictor.String(),
								},
								Annotations: map[string]string{
									"autoscaling.knative.dev/class":                               "kpa.autoscaling.knative.dev",
									"autoscaling.knative.dev/minScale":                            "1",
									"autoscaling.knative.dev/target":                              "1",
									"internal.serving.kubeflow.org/storage-initializer-sourceuri": "gs://testuri2",
								},
							},
							Spec: knservingv1.RevisionSpec{
								ContainerConcurrency: &containerConcurrency,
								TimeoutSeconds:       &constants.DefaultPredictorTimeout,
								PodSpec: v1.PodSpec{
									Containers: []v1.Container{
										{
											Image:   TensorflowServingImageName + ":" + DefaultTensorflowRuntimeVersion,
											Name:    constants.InferenceServiceContainerName,
											Command: []string{v1alpha2.TensorflowEntrypointCommand},
											Args: []string{
												"--port=" + v1alpha2.TensorflowServingGRPCPort,
												"--rest_api_port=" + v1alpha2.TensorflowServingRestPort,
												"--model_name=mnist",
												"--model_base_path=" + constants.DefaultModelLocalMountPath,
												"--rest_api_timeout_in_ms=0",
											},
											LivenessProbe: &v1.Probe{
												Handler: v1.Handler{
													HTTPGet: &v1.HTTPGetAction{
														Path: "/v1/models/mnist",
													},
												},
												InitialDelaySeconds: constants.DefaultReadinessTimeout,
												PeriodSeconds:       10,
												FailureThreshold:    3,
												SuccessThreshold:    1,
												TimeoutSeconds:      1,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		"Reconcile ignores canary if unspecified": {
			isvc: v1alpha2.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mnist",
					Namespace: "default",
				},
				Spec: v1alpha2.InferenceServiceSpec{
					Default: v1alpha2.EndpointSpec{
						Predictor: v1alpha2.PredictorSpec{
							Tensorflow: &v1alpha2.TensorflowSpec{
								RuntimeVersion: DefaultTensorflowRuntimeVersion,
								StorageURI:     "gs://testuri",
							},
						},
					},
				},
			},
			desiredDefault: &knservingv1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.DefaultPredictorServiceName("mnist"),
					Namespace: "default",
				},
				Spec: knservingv1.ServiceSpec{
					ConfigurationSpec: knservingv1.ConfigurationSpec{
						Template: knservingv1.RevisionTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"serving.kubeflow.org/inferenceservice": "mnist",
									constants.KServiceEndpointLabel:  constants.InferenceServiceDefault,
									constants.KServiceModelLabel:     "mnist",
									constants.KServiceComponentLabel: constants.Predictor.String(),
								},
								Annotations: map[string]string{
									"autoscaling.knative.dev/class":                               "kpa.autoscaling.knative.dev",
									"autoscaling.knative.dev/minScale":                            "1",
									"autoscaling.knative.dev/target":                              "1",
									"internal.serving.kubeflow.org/storage-initializer-sourceuri": "gs://testuri",
								},
							},
							Spec: knservingv1.RevisionSpec{
								ContainerConcurrency: &containerConcurrency,
								TimeoutSeconds:       &constants.DefaultPredictorTimeout,
								PodSpec: v1.PodSpec{
									Containers: []v1.Container{
										{
											Image:   TensorflowServingImageName + ":" + DefaultTensorflowRuntimeVersion,
											Name:    constants.InferenceServiceContainerName,
											Command: []string{v1alpha2.TensorflowEntrypointCommand},
											Args: []string{
												"--port=" + v1alpha2.TensorflowServingGRPCPort,
												"--rest_api_port=" + v1alpha2.TensorflowServingRestPort,
												"--model_name=mnist",
												"--model_base_path=" + constants.DefaultModelLocalMountPath,
												"--rest_api_timeout_in_ms=0",
											},
											LivenessProbe: &v1.Probe{
												Handler: v1.Handler{
													HTTPGet: &v1.HTTPGetAction{
														Path: "/v1/models/mnist",
													},
												},
												InitialDelaySeconds: constants.DefaultReadinessTimeout,
												PeriodSeconds:       10,
												FailureThreshold:    3,
												SuccessThreshold:    1,
												TimeoutSeconds:      1,
											},
										},
									},
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
		g.Expect(c.Create(context.TODO(), &scenario.isvc)).NotTo(gomega.HaveOccurred())

		if err := serviceReconciler.Reconcile(&scenario.isvc); err != nil {
			t.Errorf("Test %q failed: returned error: %v", name, err)
		}

		g.Eventually(func() error { return awaitDesired(c, scenario.desiredDefault) }, timeout).Should(gomega.Succeed())
		g.Eventually(func() error { return awaitDesired(c, scenario.desiredCanary) }, timeout).Should(gomega.Succeed())

		g.Expect(c.Delete(context.TODO(), &scenario.isvc)).NotTo(gomega.HaveOccurred())
	}
}

func awaitDesired(c client.Client, desired *knservingv1.Service) error {
	if desired == nil {
		return nil
	}
	actual := knservingv1.Service{}
	if err := c.Get(context.TODO(), types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, &actual); err != nil {
		return err
	}
	if diff := cmp.Diff(desired.Spec, actual.Spec); diff != "" {
		return fmt.Errorf("Unexpected service spec (-want +got): %v", diff)
	}
	if diff := cmp.Diff(desired.ObjectMeta.Labels, actual.ObjectMeta.Labels); diff != "" {
		return fmt.Errorf("Unexpected service labels (-want +got): %v", diff)
	}
	return nil
}

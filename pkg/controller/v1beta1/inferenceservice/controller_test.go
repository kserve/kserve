/*
Copyright 2020 kubeflow.org.
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

package inferenceservice

import (
	"context"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"
	"github.com/kubeflow/kfserving/pkg/constants"
	. "github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	knservingv1 "knative.dev/serving/pkg/apis/serving/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"time"
)

var _ = Describe("v1beta1 inference service controller", func() {
	// Define utility constants for object names and testing timeouts/durations and intervals.
	const (
		timeout  = time.Second * 10
		duration = time.Second * 10
		interval = time.Millisecond * 250
	)
	var (
		configs = map[string]string{
			"predictors": `{
               "tensorflow": {
                  "image": "tensorflow/serving"
               },
               "sklearn": {
                  "image": "kfserving/sklearnserver"
               },
               "xgboost": {
                  "image": "kfserving/xgbserver"
               }
	         }`,
			"explainers": `{
               "alibi": {
                  "image": "kfserving/alibi-explainer",
			      "defaultImageVersion": "latest"
               }
            }`,
			"ingress": `{
               "ingressGateway": "knative-serving/knative-ingress-gateway",
               "ingressService": "test-destination"
            }`,
		}
	)
	Context("When creating inference service with predictor", func() {
		It("Should have knative service created", func() {
			By("By creating a new InferenceService")
			// Create configmap
			var configMap = &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KFServingNamespace,
				},
				Data: configs,
			}
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			serviceName := "foo"
			var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: serviceName, Namespace: "default"}}
			var serviceKey = expectedRequest.NamespacedName
			var storageUri = "s3://test/mnist/export"
			ctx := context.Background()
			isvc := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceKey.Name,
					Namespace: serviceKey.Namespace,
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
							MinReplicas: v1beta1.GetIntReference(1),
							MaxReplicas: 3,
						},
						Tensorflow: &v1beta1.TFServingSpec{
							PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
								StorageURI:     &storageUri,
								RuntimeVersion: proto.String("1.14.0"),
								Container: v1.Container{
									Name: "kfs",
								},
							},
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
			inferenceService := &v1beta1.InferenceService{}

			// We'll need to retry getting this newly created CronJob, given that creation may not immediately happen.
			Eventually(func() bool {
				err := k8sClient.Get(ctx, serviceKey, inferenceService)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			actualService := &knservingv1.Service{}
			predictorServiceKey := types.NamespacedName{Name: constants.PredictorServiceName(serviceKey.Name),
				Namespace: serviceKey.Namespace}
			Eventually(func() error { return k8sClient.Get(context.TODO(), predictorServiceKey, actualService) }, timeout).
				Should(Succeed())
			fmt.Printf("knative service %+v\n", actualService)

			expectedService := &knservingv1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      predictorServiceKey.Name,
					Namespace: predictorServiceKey.Namespace,
				},
				Spec: knservingv1.ServiceSpec{
					ConfigurationSpec: knservingv1.ConfigurationSpec{
						Template: knservingv1.RevisionTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"serving.kubeflow.org/inferenceservice": serviceName,
									constants.KServiceComponentLabel: constants.Predictor.String(),
								},
								Annotations: map[string]string{
									"autoscaling.knative.dev/target":                           "1",
									"autoscaling.knative.dev/class":                            "kpa.autoscaling.knative.dev",
									"autoscaling.knative.dev/maxScale":                         "3",
									"autoscaling.knative.dev/minScale":                         "1",
									constants.StorageInitializerSourceUriInternalAnnotationKey: *isvc.Spec.Predictor.Tensorflow.StorageURI,
								},
							},
							Spec: knservingv1.RevisionSpec{
								ContainerConcurrency: isvc.Spec.Predictor.ContainerConcurrency,
								TimeoutSeconds:       isvc.Spec.Predictor.TimeoutSeconds,
								PodSpec: v1.PodSpec{
									Containers: []v1.Container{
										{
											Image: "tensorflow/serving:" +
												*isvc.Spec.Predictor.Tensorflow.RuntimeVersion,
											Name:    constants.InferenceServiceContainerName,
											Command: []string{v1beta1.TensorflowEntrypointCommand},
											Args: []string{
												"--port=" + v1beta1.TensorflowServingGRPCPort,
												"--rest_api_port=" + v1beta1.TensorflowServingRestPort,
												"--model_name=" + isvc.Name,
												"--model_base_path=" + constants.DefaultModelLocalMountPath,
											},
											//TODO default liveness probe
											/*LivenessProbe: &v1.Probe{
												Handler: v1.Handler{
													HTTPGet: &v1.HTTPGetAction{
														Path: "/v1/models/" + defaultInstance.Name,
													},
												},
												InitialDelaySeconds: constants.DefaultReadinessTimeout,
												PeriodSeconds:       10,
												FailureThreshold:    3,
												SuccessThreshold:    1,
												TimeoutSeconds:      1,
											},*/
										},
									},
								},
							},
						},
					},
				},
			}
			Expect(actualService.Spec).To(gomega.Equal(expectedService.Spec))
		})
	})
})

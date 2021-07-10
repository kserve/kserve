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
	"github.com/golang/protobuf/proto"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"
	"github.com/kubeflow/kfserving/pkg/constants"
	. "github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"time"
)

var _ = Describe("v1beta1 inference service controller", func() {
	// Define utility constants for object names and testing timeouts/durations and intervals.
	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
		domain   = "example.com"
	)
	var (
		defaultResource = v1.ResourceRequirements{
			Limits: v1.ResourceList{
				v1.ResourceCPU:    resource.MustParse("1"),
				v1.ResourceMemory: resource.MustParse("2Gi"),
			},
			Requests: v1.ResourceList{
				v1.ResourceCPU:    resource.MustParse("1"),
				v1.ResourceMemory: resource.MustParse("2Gi"),
			},
		}
		configs = map[string]string{
			"predictors": `{
               "tensorflow": {
                  "image": "tensorflow/serving",
				  "defaultTimeout": "60",
 				  "multiModelServer": false
               },
               "sklearn": {
                 "v1": {
                  	"image": "kfserving/sklearnserver",
					"multiModelServer": true
                 },
                 "v2": {
                  	"image": "kfserving/sklearnserver",
					"multiModelServer": true
                 }
               },
               "xgboost": {
				  	"image": "kfserving/xgbserver",
				  	"multiModelServer": true
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
               "ingressService": "test-destination",
               "localGateway": "knative-serving/knative-local-gateway",
               "localGatewayService": "knative-local-gateway.istio-system.svc.cluster.local"
            }`,
		}
	)
	Context("When creating inference service with raw kube predictor", func() {
		It("Should have ingress/service/deployment created", func() {
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
			serviceName := "raw-foo"
			var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: serviceName, Namespace: "default"}}
			var serviceKey = expectedRequest.NamespacedName
			var storageUri = "s3://test/mnist/export"
			ctx := context.Background()
			isvc := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceKey.Name,
					Namespace: serviceKey.Namespace,
					Annotations: map[string]string{
						"serving.kubeflow.org/raw": "true",
					},
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
									Name:      "kfs",
									Resources: defaultResource,
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
			inferenceService := &v1beta1.InferenceService{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, serviceKey, inferenceService)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			actualDeployment := &appsv1.Deployment{}
			predictorDeploymentKey := types.NamespacedName{Name: constants.DefaultPredictorServiceName(serviceKey.Name),
				Namespace: serviceKey.Namespace}
			Eventually(func() error { return k8sClient.Get(context.TODO(), predictorDeploymentKey, actualDeployment) }, timeout).
				Should(Succeed())
			var replicas int32 = 1
			var revisionHistory int32 = 10
			var progressDeadlineSeconds int32 = 600
			var gracePeriod int64 = 30
			expectedDeployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      predictorDeploymentKey.Name,
					Namespace: predictorDeploymentKey.Namespace,
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: &replicas,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "isvc." + predictorDeploymentKey.Name,
						},
					},
					Template: v1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Name:      predictorDeploymentKey.Name,
							Namespace: "default",
							Labels: map[string]string{
								"app":                                 "isvc." + predictorDeploymentKey.Name,
								constants.KServiceComponentLabel:      constants.Predictor.String(),
								constants.InferenceServicePodLabelKey: serviceName,
							},
							Annotations: map[string]string{
								constants.StorageInitializerSourceUriInternalAnnotationKey: *isvc.Spec.Predictor.Tensorflow.StorageURI,
								"serving.kubeflow.org/raw":                                 "true",
							},
						},
						Spec: v1.PodSpec{
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
										"--rest_api_timeout_in_ms=60000",
									},
									Resources: defaultResource,
									ReadinessProbe: &v1.Probe{
										Handler: v1.Handler{
											TCPSocket: &v1.TCPSocketAction{
												Port: intstr.IntOrString{
													IntVal: 8080,
												},
											},
										},
										InitialDelaySeconds: 0,
										TimeoutSeconds:      1,
										PeriodSeconds:       10,
										SuccessThreshold:    1,
										FailureThreshold:    3,
									},
									TerminationMessagePath:   "/dev/termination-log",
									TerminationMessagePolicy: "File",
									ImagePullPolicy:          "IfNotPresent",
								},
							},
							SchedulerName:                 "default-scheduler",
							RestartPolicy:                 "Always",
							TerminationGracePeriodSeconds: &gracePeriod,
							DNSPolicy:                     "ClusterFirst",
							SecurityContext: &v1.PodSecurityContext{
								SELinuxOptions:      nil,
								WindowsOptions:      nil,
								RunAsUser:           nil,
								RunAsGroup:          nil,
								RunAsNonRoot:        nil,
								SupplementalGroups:  nil,
								FSGroup:             nil,
								Sysctls:             nil,
								FSGroupChangePolicy: nil,
								SeccompProfile:      nil,
							},
						},
					},
					Strategy: appsv1.DeploymentStrategy{
						Type: "RollingUpdate",
						RollingUpdate: &appsv1.RollingUpdateDeployment{
							MaxUnavailable: &intstr.IntOrString{Type: 1, IntVal: 0, StrVal: "25%"},
							MaxSurge:       &intstr.IntOrString{Type: 1, IntVal: 0, StrVal: "25%"},
						},
					},
					RevisionHistoryLimit:    &revisionHistory,
					ProgressDeadlineSeconds: &progressDeadlineSeconds,
				},
			}
			Expect(actualDeployment.Spec).To(gomega.Equal(expectedDeployment.Spec))
		})
	})
})

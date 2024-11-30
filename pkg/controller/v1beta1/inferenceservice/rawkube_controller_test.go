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

package inferenceservice

import (
	"context"
	"fmt"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/proto"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	v1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
)

var _ = Describe("v1beta1 inference service controller", func() {
	// Define utility constants for object names and testing timeouts/durations and intervals.
	const (
		timeout  = time.Second * 30
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
		kserveGateway = types.NamespacedName{Name: "kserve-ingress-gateway", Namespace: "kserve"}
	)

	Context("When creating inference service with raw kube predictor", func() {
		configs := map[string]string{
			"explainers": `{
				"alibi": {
					"image": "kserve/alibi-explainer",
					"defaultImageVersion": "latest"
				}
			}`,
			"ingress": `{
				"enableGatewayApi": true,
				"kserveIngressGateway": "kserve/kserve-ingress-gateway",
				"ingressGateway": "knative-serving/knative-ingress-gateway",
				"localGateway": "knative-serving/knative-local-gateway",
				"localGatewayService": "knative-local-gateway.istio-system.svc.cluster.local",
				"additionalIngressDomains": ["additional.example.com"]
			}`,
			"storageInitializer": `{
				"image" : "kserve/storage-initializer:latest",
				"memoryRequest": "100Mi",
				"memoryLimit": "1Gi",
				"cpuRequest": "100m",
				"cpuLimit": "1",
				"CaBundleConfigMapName": "",
				"caBundleVolumeMountPath": "/etc/ssl/custom-certs",
				"enableDirectPvcVolumeMount": false
			}`,
		}

		It("Should have httproute/service/deployment/hpa created", func() {
			By("By creating a new InferenceService")
			// Create configmap
			var configMap = &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KServeNamespace,
				},
				Data: configs,
			}
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)
			// Create ServingRuntime
			servingRuntime := &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tf-serving-raw",
					Namespace: "default",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{
							Name:       "tensorflow",
							Version:    proto.String("1"),
							AutoSelect: proto.Bool(true),
						},
					},
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []v1.Container{
							{
								Name:    "kserve-container",
								Image:   "tensorflow/serving:1.14.0",
								Command: []string{"/usr/bin/tensorflow_model_server"},
								Args: []string{
									"--port=9000",
									"--rest_api_port=8080",
									"--model_base_path=/mnt/models",
									"--rest_api_timeout_in_ms=60000",
								},
								Resources: defaultResource,
							},
						},
					},
					Disabled: proto.Bool(false),
				},
			}
			k8sClient.Create(context.TODO(), servingRuntime)
			defer k8sClient.Delete(context.TODO(), servingRuntime)
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
						"serving.kserve.io/deploymentMode":              "RawDeployment",
						"serving.kserve.io/autoscalerClass":             "hpa",
						"serving.kserve.io/metrics":                     "cpu",
						"serving.kserve.io/targetUtilizationPercentage": "75",
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
									Name:      constants.InferenceServiceContainerName,
									Resources: defaultResource,
								},
							},
						},
					},
				},
			}
			isvc.DefaultInferenceService(nil, nil, &v1beta1.SecurityConfig{AutoMountServiceAccountToken: false}, nil)
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
			defer k8sClient.Delete(ctx, isvc)

			inferenceService := &v1beta1.InferenceService{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, serviceKey, inferenceService)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			actualDeployment := &appsv1.Deployment{}
			predictorDeploymentKey := types.NamespacedName{Name: constants.PredictorServiceName(serviceKey.Name),
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
								constants.StorageInitializerSourceUriInternalAnnotationKey: *isvc.Spec.Predictor.Model.StorageURI,
								"serving.kserve.io/deploymentMode":                         "RawDeployment",
								"serving.kserve.io/autoscalerClass":                        "hpa",
								"serving.kserve.io/metrics":                                "cpu",
								"serving.kserve.io/targetUtilizationPercentage":            "75",
							},
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Image: "tensorflow/serving:" +
										*isvc.Spec.Predictor.Model.RuntimeVersion,
									Name:    constants.InferenceServiceContainerName,
									Command: []string{v1beta1.TensorflowEntrypointCommand},
									Args: []string{
										"--port=" + v1beta1.TensorflowServingGRPCPort,
										"--rest_api_port=" + v1beta1.TensorflowServingRestPort,
										"--model_base_path=" + constants.DefaultModelLocalMountPath,
										"--rest_api_timeout_in_ms=60000",
									},
									Resources: defaultResource,
									ReadinessProbe: &v1.Probe{
										ProbeHandler: v1.ProbeHandler{
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
							AutomountServiceAccountToken: proto.Bool(false),
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
			Expect(actualDeployment.Spec).To(BeComparableTo(expectedDeployment.Spec))

			//check service
			actualService := &v1.Service{}
			predictorServiceKey := types.NamespacedName{Name: constants.PredictorServiceName(serviceKey.Name),
				Namespace: serviceKey.Namespace}
			Eventually(func() error { return k8sClient.Get(context.TODO(), predictorServiceKey, actualService) }, timeout).
				Should(Succeed())

			expectedService := &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      predictorServiceKey.Name,
					Namespace: predictorServiceKey.Namespace,
				},
				Spec: v1.ServiceSpec{
					Ports: []v1.ServicePort{
						{
							Name:       constants.PredictorServiceName(serviceName),
							Protocol:   "TCP",
							Port:       80,
							TargetPort: intstr.IntOrString{Type: 0, IntVal: 8080, StrVal: ""},
						},
					},
					Type:            "ClusterIP",
					SessionAffinity: "None",
					Selector: map[string]string{
						"app": fmt.Sprintf("isvc.%s", constants.PredictorServiceName(serviceName)),
					},
				},
			}
			actualService.Spec.ClusterIP = ""
			actualService.Spec.ClusterIPs = nil
			actualService.Spec.IPFamilies = nil
			actualService.Spec.IPFamilyPolicy = nil
			actualService.Spec.InternalTrafficPolicy = nil
			Expect(actualService.Spec).To(BeComparableTo(expectedService.Spec))

			//check isvc status
			updatedDeployment := actualDeployment.DeepCopy()
			updatedDeployment.Status.Conditions = []appsv1.DeploymentCondition{
				{
					Type:   appsv1.DeploymentAvailable,
					Status: v1.ConditionTrue,
				},
			}
			Expect(k8sClient.Status().Update(context.TODO(), updatedDeployment)).NotTo(HaveOccurred())

			//check http route
			actualTopLevelHttpRoute := &gatewayapiv1.HTTPRoute{}
			Eventually(func() error {
				return k8sClient.Get(context.TODO(), types.NamespacedName{Name: serviceKey.Name,
					Namespace: serviceKey.Namespace}, actualTopLevelHttpRoute)
			}, timeout).Should(Succeed())
			topLevelHost := fmt.Sprintf("%s-%s.%s", serviceKey.Name, serviceKey.Namespace, "example.com")
			expectedTopLevelHttpRoute := gatewayapiv1.HTTPRoute{
				Spec: gatewayapiv1.HTTPRouteSpec{
					Hostnames: []gatewayapiv1.Hostname{gatewayapiv1.Hostname(topLevelHost), "additional.example.com"},
					Rules: []gatewayapiv1.HTTPRouteRule{
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer(constants.FallbackPrefix()),
									},
								},
							},
							Filters: []gatewayapiv1.HTTPRouteFilter{
								{
									Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
										Set: []gatewayapiv1.HTTPHeader{
											{
												Name:  constants.IsvcNameHeader,
												Value: serviceKey.Name,
											},
											{
												Name:  constants.IsvcNamespaceHeader,
												Value: serviceKey.Namespace,
											},
										},
									},
								},
							},
							BackendRefs: []gatewayapiv1.HTTPBackendRef{
								{
									BackendRef: gatewayapiv1.BackendRef{
										BackendObjectReference: gatewayapiv1.BackendObjectReference{
											Group:     (*gatewayapiv1.Group)(utils.ToPointer("")),
											Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      gatewayapiv1.ObjectName(predictorServiceKey.Name),
											Namespace: (*gatewayapiv1.Namespace)(utils.ToPointer(serviceKey.Namespace)),
											Port:      (*gatewayapiv1.PortNumber)(utils.ToPointer(int32(constants.CommonDefaultHttpPort))),
										},
										Weight: utils.ToPointer(int32(1)),
									},
								},
							},
							Timeouts: &gatewayapiv1.HTTPRouteTimeouts{
								Request: utils.ToPointer(gatewayapiv1.Duration("0s")),
							},
						},
					},
					CommonRouteSpec: gatewayapiv1.CommonRouteSpec{
						ParentRefs: []gatewayapiv1.ParentReference{
							{
								Name:      gatewayapiv1.ObjectName(kserveGateway.Name),
								Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.GatewayKind)),
								Group:     (*gatewayapiv1.Group)(&gatewayapiv1.GroupVersion.Group),
								Namespace: utils.ToPointer(gatewayapiv1.Namespace(kserveGateway.Namespace)),
							},
						},
					},
				},
			}
			Expect(actualTopLevelHttpRoute.Spec).To(BeComparableTo(expectedTopLevelHttpRoute.Spec))

			actualPredictorHttpRoute := &gatewayapiv1.HTTPRoute{}
			Eventually(func() error {
				return k8sClient.Get(context.TODO(), types.NamespacedName{Name: predictorServiceKey.Name,
					Namespace: serviceKey.Namespace}, actualPredictorHttpRoute)
			}, timeout).Should(Succeed())
			predictorHost := fmt.Sprintf("%s-%s.%s", predictorServiceKey.Name, serviceKey.Namespace, "example.com")
			expectedPredictorHttpRoute := gatewayapiv1.HTTPRoute{
				Spec: gatewayapiv1.HTTPRouteSpec{
					Hostnames: []gatewayapiv1.Hostname{gatewayapiv1.Hostname(predictorHost)},
					Rules: []gatewayapiv1.HTTPRouteRule{
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer(constants.FallbackPrefix()),
									},
								},
							},
							Filters: []gatewayapiv1.HTTPRouteFilter{
								{
									Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
										Set: []gatewayapiv1.HTTPHeader{
											{
												Name:  constants.IsvcNameHeader,
												Value: serviceKey.Name,
											},
											{
												Name:  constants.IsvcNamespaceHeader,
												Value: serviceKey.Namespace,
											},
										},
									},
								},
							},
							BackendRefs: []gatewayapiv1.HTTPBackendRef{
								{
									BackendRef: gatewayapiv1.BackendRef{
										BackendObjectReference: gatewayapiv1.BackendObjectReference{
											Group:     (*gatewayapiv1.Group)(utils.ToPointer("")),
											Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      gatewayapiv1.ObjectName(predictorServiceKey.Name),
											Namespace: (*gatewayapiv1.Namespace)(utils.ToPointer(serviceKey.Namespace)),
											Port:      (*gatewayapiv1.PortNumber)(utils.ToPointer(int32(constants.CommonDefaultHttpPort))),
										},
										Weight: utils.ToPointer(int32(1)),
									},
								},
							},
							Timeouts: &gatewayapiv1.HTTPRouteTimeouts{
								Request: utils.ToPointer(gatewayapiv1.Duration("0s")),
							},
						},
					},
					CommonRouteSpec: gatewayapiv1.CommonRouteSpec{
						ParentRefs: []gatewayapiv1.ParentReference{
							{
								Name:      gatewayapiv1.ObjectName(kserveGateway.Name),
								Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.GatewayKind)),
								Group:     (*gatewayapiv1.Group)(&gatewayapiv1.GroupVersion.Group),
								Namespace: utils.ToPointer(gatewayapiv1.Namespace(kserveGateway.Namespace)),
							},
						},
					},
				},
			}
			Expect(actualPredictorHttpRoute.Spec).To(BeComparableTo(expectedPredictorHttpRoute.Spec))

			// Mark the Ingress as accepted to make isvc ready
			httpRouteStatus := gatewayapiv1.HTTPRouteStatus{
				RouteStatus: gatewayapiv1.RouteStatus{
					Parents: []gatewayapiv1.RouteParentStatus{
						{
							ParentRef: gatewayapiv1.ParentReference{
								Name:      gatewayapiv1.ObjectName(kserveGateway.Name),
								Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.GatewayKind)),
								Group:     (*gatewayapiv1.Group)(&gatewayapiv1.GroupVersion.Group),
								Namespace: utils.ToPointer(gatewayapiv1.Namespace(kserveGateway.Namespace)),
							},
							ControllerName: "istio.io/gateway-controller",
							Conditions: []metav1.Condition{
								{
									Type:               string(gatewayapiv1.ListenerConditionAccepted),
									Status:             metav1.ConditionTrue,
									Reason:             "Accepted",
									Message:            "Route was valid",
									LastTransitionTime: metav1.Now(),
								},
							},
						},
					},
				},
			}
			actualPredictorHttpRoute.Status = httpRouteStatus
			Expect(k8sClient.Status().Update(context.Background(), actualPredictorHttpRoute)).NotTo(HaveOccurred())
			actualTopLevelHttpRoute.Status = httpRouteStatus
			Expect(k8sClient.Status().Update(context.Background(), actualTopLevelHttpRoute)).NotTo(HaveOccurred())

			// verify if InferenceService status is updated
			expectedIsvcStatus := v1beta1.InferenceServiceStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{
							Type:   v1beta1.IngressReady,
							Status: "True",
						},
						{
							Type:   v1beta1.PredictorReady,
							Status: "True",
						},
						{
							Type:   apis.ConditionReady,
							Status: "True",
						},
					},
				},
				URL: &apis.URL{
					Scheme: "http",
					Host:   "raw-foo-default.example.com",
				},
				Address: &duckv1.Addressable{
					URL: &apis.URL{
						Scheme: "http",
						Host:   fmt.Sprintf("%s-predictor.%s.svc.cluster.local", serviceKey.Name, serviceKey.Namespace),
					},
				},
				Components: map[v1beta1.ComponentType]v1beta1.ComponentStatusSpec{
					v1beta1.PredictorComponent: {
						LatestCreatedRevision: "",
						URL: &apis.URL{
							Scheme: "http",
							Host:   "raw-foo-predictor-default.example.com",
						},
					},
				},
				ModelStatus: v1beta1.ModelStatus{
					TransitionStatus:    "InProgress",
					ModelRevisionStates: &v1beta1.ModelRevisionStates{TargetModelState: "Pending"},
				},
			}
			Eventually(func() string {
				isvc := &v1beta1.InferenceService{}
				if err := k8sClient.Get(context.TODO(), serviceKey, isvc); err != nil {
					return err.Error()
				}
				return cmp.Diff(&expectedIsvcStatus, &isvc.Status, cmpopts.IgnoreTypes(apis.VolatileTime{}))
			}, timeout).Should(BeEmpty())

			//check HPA
			var minReplicas int32 = 1
			var maxReplicas int32 = 3
			var cpuUtilization int32 = 75
			var stabilizationWindowSeconds int32 = 0
			selectPolicy := autoscalingv2.MaxChangePolicySelect
			actualHPA := &autoscalingv2.HorizontalPodAutoscaler{}
			predictorHPAKey := types.NamespacedName{Name: predictorServiceKey.Name,
				Namespace: serviceKey.Namespace}
			Eventually(func() error { return k8sClient.Get(context.TODO(), predictorHPAKey, actualHPA) }, timeout).
				Should(Succeed())
			expectedHPA := &autoscalingv2.HorizontalPodAutoscaler{
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       predictorServiceKey.Name,
					},
					MinReplicas: &minReplicas,
					MaxReplicas: maxReplicas,
					Metrics: []autoscalingv2.MetricSpec{
						{
							Type: autoscalingv2.ResourceMetricSourceType,
							Resource: &autoscalingv2.ResourceMetricSource{
								Name: v1.ResourceCPU,
								Target: autoscalingv2.MetricTarget{
									Type:               "Utilization",
									AverageUtilization: &cpuUtilization,
								},
							},
						},
					},
					Behavior: &autoscalingv2.HorizontalPodAutoscalerBehavior{
						ScaleUp: &autoscalingv2.HPAScalingRules{
							StabilizationWindowSeconds: &stabilizationWindowSeconds,
							SelectPolicy:               &selectPolicy,
							Policies: []autoscalingv2.HPAScalingPolicy{
								{
									Type:          "Pods",
									Value:         4,
									PeriodSeconds: 15,
								},
								{
									Type:          "Percent",
									Value:         100,
									PeriodSeconds: 15,
								},
							},
						},
						ScaleDown: &autoscalingv2.HPAScalingRules{
							StabilizationWindowSeconds: nil,
							SelectPolicy:               &selectPolicy,
							Policies: []autoscalingv2.HPAScalingPolicy{
								{
									Type:          "Percent",
									Value:         100,
									PeriodSeconds: 15,
								},
							},
						},
					},
				},
			}
			Expect(actualHPA.Spec).To(BeComparableTo(expectedHPA.Spec))
		})
		It("Should have httproute/service/deployment/hpa created with DeploymentStrategy", func() {
			By("By creating a new InferenceService with DeploymentStrategy in PredictorSpec")
			// Create configmap
			var configMap = &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KServeNamespace,
				},
				Data: configs,
			}
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)
			// Create ServingRuntime
			servingRuntime := &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tf-serving-raw",
					Namespace: "default",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{
							Name:       "tensorflow",
							Version:    proto.String("1"),
							AutoSelect: proto.Bool(true),
						},
					},
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []v1.Container{
							{
								Name:    "kserve-container",
								Image:   "tensorflow/serving:1.14.0",
								Command: []string{"/usr/bin/tensorflow_model_server"},
								Args: []string{
									"--port=9000",
									"--rest_api_port=8080",
									"--model_base_path=/mnt/models",
									"--rest_api_timeout_in_ms=60000",
								},
								Resources: defaultResource,
							},
						},
					},
					Disabled: proto.Bool(false),
				},
			}
			k8sClient.Create(context.TODO(), servingRuntime)
			defer k8sClient.Delete(context.TODO(), servingRuntime)
			serviceName := "raw-foo-customized"
			var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: serviceName, Namespace: "default"}}
			var serviceKey = expectedRequest.NamespacedName
			var storageUri = "s3://test/mnist/export"
			predictorDeploymentKey := types.NamespacedName{Name: constants.PredictorServiceName(serviceKey.Name),
				Namespace: serviceKey.Namespace}
			var replicas int32 = 1
			var revisionHistory int32 = 10
			var progressDeadlineSeconds int32 = 600
			var gracePeriod int64 = 30
			ctx := context.Background()
			isvc := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceKey.Name,
					Namespace: serviceKey.Namespace,
					Annotations: map[string]string{
						"serving.kserve.io/deploymentMode":              "RawDeployment",
						"serving.kserve.io/autoscalerClass":             "hpa",
						"serving.kserve.io/metrics":                     "cpu",
						"serving.kserve.io/targetUtilizationPercentage": "75",
					},
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
							MinReplicas: v1beta1.GetIntReference(1),
							MaxReplicas: 3,
							DeploymentStrategy: &appsv1.DeploymentStrategy{
								Type: appsv1.RecreateDeploymentStrategyType,
							}},
						Tensorflow: &v1beta1.TFServingSpec{
							PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
								StorageURI:     &storageUri,
								RuntimeVersion: proto.String("1.14.0"),
								Container: v1.Container{
									Name:      constants.InferenceServiceContainerName,
									Resources: defaultResource,
								},
							},
						},
					},
				},
			}
			isvc.DefaultInferenceService(nil, nil, &v1beta1.SecurityConfig{AutoMountServiceAccountToken: false}, nil)
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
			defer k8sClient.Delete(ctx, isvc)

			inferenceService := &v1beta1.InferenceService{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, serviceKey, inferenceService)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			actualDeployment := &appsv1.Deployment{}

			Eventually(func() error { return k8sClient.Get(context.TODO(), predictorDeploymentKey, actualDeployment) }, timeout).
				Should(Succeed())

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
								constants.StorageInitializerSourceUriInternalAnnotationKey: *isvc.Spec.Predictor.Model.StorageURI,
								"serving.kserve.io/deploymentMode":                         "RawDeployment",
								"serving.kserve.io/autoscalerClass":                        "hpa",
								"serving.kserve.io/metrics":                                "cpu",
								"serving.kserve.io/targetUtilizationPercentage":            "75",
							},
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Image: "tensorflow/serving:" +
										*isvc.Spec.Predictor.Model.RuntimeVersion,
									Name:    constants.InferenceServiceContainerName,
									Command: []string{v1beta1.TensorflowEntrypointCommand},
									Args: []string{
										"--port=" + v1beta1.TensorflowServingGRPCPort,
										"--rest_api_port=" + v1beta1.TensorflowServingRestPort,
										"--model_base_path=" + constants.DefaultModelLocalMountPath,
										"--rest_api_timeout_in_ms=60000",
									},
									Resources: defaultResource,
									ReadinessProbe: &v1.Probe{
										ProbeHandler: v1.ProbeHandler{
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
							AutomountServiceAccountToken: proto.Bool(false),
						},
					},
					// This is now customized and different from defaults set via `setDefaultDeploymentSpec`.
					Strategy: appsv1.DeploymentStrategy{
						Type: appsv1.RecreateDeploymentStrategyType,
					},
					RevisionHistoryLimit:    &revisionHistory,
					ProgressDeadlineSeconds: &progressDeadlineSeconds,
				},
			}
			Expect(actualDeployment.Spec).To(BeComparableTo(expectedDeployment.Spec))

			//check service
			actualService := &v1.Service{}
			predictorServiceKey := types.NamespacedName{Name: constants.PredictorServiceName(serviceKey.Name),
				Namespace: serviceKey.Namespace}
			Eventually(func() error { return k8sClient.Get(context.TODO(), predictorServiceKey, actualService) }, timeout).
				Should(Succeed())

			expectedService := &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      predictorServiceKey.Name,
					Namespace: predictorServiceKey.Namespace,
				},
				Spec: v1.ServiceSpec{
					Ports: []v1.ServicePort{
						{
							Name:       constants.PredictorServiceName(serviceName),
							Protocol:   "TCP",
							Port:       80,
							TargetPort: intstr.IntOrString{Type: 0, IntVal: 8080, StrVal: ""},
						},
					},
					Type:            "ClusterIP",
					SessionAffinity: "None",
					Selector: map[string]string{
						"app": fmt.Sprintf("isvc.%s", constants.PredictorServiceName(serviceName)),
					},
				},
			}
			actualService.Spec.ClusterIP = ""
			actualService.Spec.ClusterIPs = nil
			actualService.Spec.IPFamilies = nil
			actualService.Spec.IPFamilyPolicy = nil
			actualService.Spec.InternalTrafficPolicy = nil
			Expect(actualService.Spec).To(BeComparableTo(expectedService.Spec))

			//check isvc status
			updatedDeployment := actualDeployment.DeepCopy()
			updatedDeployment.Status.Conditions = []appsv1.DeploymentCondition{
				{
					Type:   appsv1.DeploymentAvailable,
					Status: v1.ConditionTrue,
				},
			}
			Expect(k8sClient.Status().Update(context.TODO(), updatedDeployment)).NotTo(HaveOccurred())

			//check http route
			actualToplevelHttpRoute := &gatewayapiv1.HTTPRoute{}
			Eventually(func() error {
				return k8sClient.Get(context.TODO(), types.NamespacedName{Name: serviceKey.Name,
					Namespace: serviceKey.Namespace}, actualToplevelHttpRoute)
			}, timeout).
				Should(Succeed())
			topLevelHost := fmt.Sprintf("%s-%s.%s", serviceKey.Name, serviceKey.Namespace, "example.com")
			expectedToplevelHttpRoute := gatewayapiv1.HTTPRoute{
				Spec: gatewayapiv1.HTTPRouteSpec{
					Hostnames: []gatewayapiv1.Hostname{gatewayapiv1.Hostname(topLevelHost), "additional.example.com"},
					Rules: []gatewayapiv1.HTTPRouteRule{
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer(constants.FallbackPrefix()),
									},
								},
							},
							Filters: []gatewayapiv1.HTTPRouteFilter{
								{
									Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
										Set: []gatewayapiv1.HTTPHeader{
											{
												Name:  constants.IsvcNameHeader,
												Value: serviceKey.Name,
											},
											{
												Name:  constants.IsvcNamespaceHeader,
												Value: serviceKey.Namespace,
											},
										},
									},
								},
							},
							BackendRefs: []gatewayapiv1.HTTPBackendRef{
								{
									BackendRef: gatewayapiv1.BackendRef{
										BackendObjectReference: gatewayapiv1.BackendObjectReference{
											Group:     (*gatewayapiv1.Group)(utils.ToPointer("")),
											Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      gatewayapiv1.ObjectName(predictorServiceKey.Name),
											Namespace: (*gatewayapiv1.Namespace)(utils.ToPointer(serviceKey.Namespace)),
											Port:      (*gatewayapiv1.PortNumber)(utils.ToPointer(int32(constants.CommonDefaultHttpPort))),
										},
										Weight: utils.ToPointer(int32(1)),
									},
								},
							},
							Timeouts: &gatewayapiv1.HTTPRouteTimeouts{
								Request: utils.ToPointer(gatewayapiv1.Duration("0s")),
							},
						},
					},
					CommonRouteSpec: gatewayapiv1.CommonRouteSpec{
						ParentRefs: []gatewayapiv1.ParentReference{
							{
								Name:      gatewayapiv1.ObjectName(kserveGateway.Name),
								Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.GatewayKind)),
								Group:     (*gatewayapiv1.Group)(&gatewayapiv1.GroupVersion.Group),
								Namespace: utils.ToPointer(gatewayapiv1.Namespace(kserveGateway.Namespace)),
							},
						},
					},
				},
			}
			Expect(actualToplevelHttpRoute.Spec).To(BeComparableTo(expectedToplevelHttpRoute.Spec))

			actualPredictorHttpRoute := &gatewayapiv1.HTTPRoute{}
			Eventually(func() error {
				return k8sClient.Get(context.TODO(), types.NamespacedName{Name: predictorServiceKey.Name,
					Namespace: serviceKey.Namespace}, actualPredictorHttpRoute)
			}, timeout).
				Should(Succeed())
			predictorHost := fmt.Sprintf("%s-%s.%s", predictorServiceKey.Name, serviceKey.Namespace, "example.com")
			expectedPredictorHttpRoute := gatewayapiv1.HTTPRoute{
				Spec: gatewayapiv1.HTTPRouteSpec{
					Hostnames: []gatewayapiv1.Hostname{gatewayapiv1.Hostname(predictorHost)},
					Rules: []gatewayapiv1.HTTPRouteRule{
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer(constants.FallbackPrefix()),
									},
								},
							},
							Filters: []gatewayapiv1.HTTPRouteFilter{
								{
									Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
										Set: []gatewayapiv1.HTTPHeader{
											{
												Name:  constants.IsvcNameHeader,
												Value: serviceKey.Name,
											},
											{
												Name:  constants.IsvcNamespaceHeader,
												Value: serviceKey.Namespace,
											},
										},
									},
								},
							},
							BackendRefs: []gatewayapiv1.HTTPBackendRef{
								{
									BackendRef: gatewayapiv1.BackendRef{
										BackendObjectReference: gatewayapiv1.BackendObjectReference{
											Group:     (*gatewayapiv1.Group)(utils.ToPointer("")),
											Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      gatewayapiv1.ObjectName(predictorServiceKey.Name),
											Namespace: (*gatewayapiv1.Namespace)(utils.ToPointer(serviceKey.Namespace)),
											Port:      (*gatewayapiv1.PortNumber)(utils.ToPointer(int32(constants.CommonDefaultHttpPort))),
										},
										Weight: utils.ToPointer(int32(1)),
									},
								},
							},
							Timeouts: &gatewayapiv1.HTTPRouteTimeouts{
								Request: utils.ToPointer(gatewayapiv1.Duration("0s")),
							},
						},
					},
					CommonRouteSpec: gatewayapiv1.CommonRouteSpec{
						ParentRefs: []gatewayapiv1.ParentReference{
							{
								Name:      gatewayapiv1.ObjectName(kserveGateway.Name),
								Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.GatewayKind)),
								Group:     (*gatewayapiv1.Group)(&gatewayapiv1.GroupVersion.Group),
								Namespace: utils.ToPointer(gatewayapiv1.Namespace(kserveGateway.Namespace)),
							},
						},
					},
				},
			}
			Expect(actualPredictorHttpRoute.Spec).To(BeComparableTo(expectedPredictorHttpRoute.Spec))

			// Mark the Ingress as accepted to make isvc ready
			httpRouteStatus := gatewayapiv1.HTTPRouteStatus{
				RouteStatus: gatewayapiv1.RouteStatus{
					Parents: []gatewayapiv1.RouteParentStatus{
						{
							ParentRef: gatewayapiv1.ParentReference{
								Name:      gatewayapiv1.ObjectName(kserveGateway.Name),
								Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.GatewayKind)),
								Group:     (*gatewayapiv1.Group)(&gatewayapiv1.GroupVersion.Group),
								Namespace: utils.ToPointer(gatewayapiv1.Namespace(kserveGateway.Namespace)),
							},
							ControllerName: "istio.io/gateway-controller",
							Conditions: []metav1.Condition{
								{
									Type:               string(gatewayapiv1.ListenerConditionAccepted),
									Status:             metav1.ConditionTrue,
									Reason:             "Accepted",
									Message:            "Route was valid",
									LastTransitionTime: metav1.Now(),
								},
							},
						},
					},
				},
			}
			actualPredictorHttpRoute.Status = httpRouteStatus
			Expect(k8sClient.Status().Update(context.Background(), actualPredictorHttpRoute)).NotTo(HaveOccurred())
			actualToplevelHttpRoute.Status = httpRouteStatus
			Expect(k8sClient.Status().Update(context.Background(), actualToplevelHttpRoute)).NotTo(HaveOccurred())

			// verify if InferenceService status is updated
			expectedIsvcStatus := v1beta1.InferenceServiceStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{
							Type:   v1beta1.IngressReady,
							Status: "True",
						},
						{
							Type:   v1beta1.PredictorReady,
							Status: "True",
						},
						{
							Type:   apis.ConditionReady,
							Status: "True",
						},
					},
				},
				URL: &apis.URL{
					Scheme: "http",
					Host:   "raw-foo-customized-default.example.com",
				},
				Address: &duckv1.Addressable{
					URL: &apis.URL{
						Scheme: "http",
						Host:   fmt.Sprintf("%s-predictor.%s.svc.cluster.local", serviceKey.Name, serviceKey.Namespace),
					},
				},
				Components: map[v1beta1.ComponentType]v1beta1.ComponentStatusSpec{
					v1beta1.PredictorComponent: {
						LatestCreatedRevision: "",
						URL: &apis.URL{
							Scheme: "http",
							Host:   "raw-foo-customized-predictor-default.example.com",
						},
					},
				},
				ModelStatus: v1beta1.ModelStatus{
					TransitionStatus:    "InProgress",
					ModelRevisionStates: &v1beta1.ModelRevisionStates{TargetModelState: "Pending"},
				},
			}
			Eventually(func() string {
				isvc := &v1beta1.InferenceService{}
				if err := k8sClient.Get(context.TODO(), serviceKey, isvc); err != nil {
					return err.Error()
				}
				return cmp.Diff(&expectedIsvcStatus, &isvc.Status, cmpopts.IgnoreTypes(apis.VolatileTime{}))
			}, timeout).Should(BeEmpty())

			//check HPA
			var minReplicas int32 = 1
			var maxReplicas int32 = 3
			var cpuUtilization int32 = 75
			var stabilizationWindowSeconds int32 = 0
			selectPolicy := autoscalingv2.MaxChangePolicySelect
			actualHPA := &autoscalingv2.HorizontalPodAutoscaler{}
			predictorHPAKey := types.NamespacedName{Name: predictorServiceKey.Name,
				Namespace: serviceKey.Namespace}
			Eventually(func() error { return k8sClient.Get(context.TODO(), predictorHPAKey, actualHPA) }, timeout).
				Should(Succeed())
			expectedHPA := &autoscalingv2.HorizontalPodAutoscaler{
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       predictorServiceKey.Name,
					},
					MinReplicas: &minReplicas,
					MaxReplicas: maxReplicas,
					Metrics: []autoscalingv2.MetricSpec{
						{
							Type: autoscalingv2.ResourceMetricSourceType,
							Resource: &autoscalingv2.ResourceMetricSource{
								Name: v1.ResourceCPU,
								Target: autoscalingv2.MetricTarget{
									Type:               "Utilization",
									AverageUtilization: &cpuUtilization,
								},
							},
						},
					},
					Behavior: &autoscalingv2.HorizontalPodAutoscalerBehavior{
						ScaleUp: &autoscalingv2.HPAScalingRules{
							StabilizationWindowSeconds: &stabilizationWindowSeconds,
							SelectPolicy:               &selectPolicy,
							Policies: []autoscalingv2.HPAScalingPolicy{
								{
									Type:          "Pods",
									Value:         4,
									PeriodSeconds: 15,
								},
								{
									Type:          "Percent",
									Value:         100,
									PeriodSeconds: 15,
								},
							},
						},
						ScaleDown: &autoscalingv2.HPAScalingRules{
							StabilizationWindowSeconds: nil,
							SelectPolicy:               &selectPolicy,
							Policies: []autoscalingv2.HPAScalingPolicy{
								{
									Type:          "Percent",
									Value:         100,
									PeriodSeconds: 15,
								},
							},
						},
					},
				},
			}
			Expect(actualHPA.Spec).To(BeComparableTo(expectedHPA.Spec))
		})
		It("Should have httproute/service/deployment created", func() {
			By("By creating a new InferenceService with AutoscalerClassExternal")
			// Create configmap
			var configMap = &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KServeNamespace,
				},
				Data: configs,
			}
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)
			// Create ServingRuntime
			servingRuntime := &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tf-serving-raw",
					Namespace: "default",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{
							Name:       "tensorflow",
							Version:    proto.String("1"),
							AutoSelect: proto.Bool(true),
						},
					},
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []v1.Container{
							{
								Name:    "kserve-container",
								Image:   "tensorflow/serving:1.14.0",
								Command: []string{"/usr/bin/tensorflow_model_server"},
								Args: []string{
									"--port=9000",
									"--rest_api_port=8080",
									"--model_base_path=/mnt/models",
									"--rest_api_timeout_in_ms=60000",
								},
								Resources: defaultResource,
							},
						},
					},
					Disabled: proto.Bool(false),
				},
			}
			k8sClient.Create(context.TODO(), servingRuntime)
			defer k8sClient.Delete(context.TODO(), servingRuntime)
			serviceName := "raw-foo-2"
			var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: serviceName, Namespace: "default"}}
			var serviceKey = expectedRequest.NamespacedName
			var storageUri = "s3://test/mnist/export"
			ctx := context.Background()
			isvc := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceKey.Name,
					Namespace: serviceKey.Namespace,
					Annotations: map[string]string{
						"serving.kserve.io/deploymentMode":  "RawDeployment",
						"serving.kserve.io/autoscalerClass": "external",
					},
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						Tensorflow: &v1beta1.TFServingSpec{
							PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
								StorageURI:     &storageUri,
								RuntimeVersion: proto.String("1.14.0"),
								Container: v1.Container{
									Name:      constants.InferenceServiceContainerName,
									Resources: defaultResource,
								},
							},
						},
					},
				},
			}
			isvc.DefaultInferenceService(nil, nil, &v1beta1.SecurityConfig{AutoMountServiceAccountToken: false}, nil)
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
			defer k8sClient.Delete(ctx, isvc)

			inferenceService := &v1beta1.InferenceService{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, serviceKey, inferenceService)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			actualDeployment := &appsv1.Deployment{}
			predictorDeploymentKey := types.NamespacedName{Name: constants.PredictorServiceName(serviceKey.Name),
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
								constants.StorageInitializerSourceUriInternalAnnotationKey: *isvc.Spec.Predictor.Model.StorageURI,
								"serving.kserve.io/deploymentMode":                         "RawDeployment",
								"serving.kserve.io/autoscalerClass":                        "external",
							},
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Image: "tensorflow/serving:" +
										*isvc.Spec.Predictor.Model.RuntimeVersion,
									Name:    constants.InferenceServiceContainerName,
									Command: []string{v1beta1.TensorflowEntrypointCommand},
									Args: []string{
										"--port=" + v1beta1.TensorflowServingGRPCPort,
										"--rest_api_port=" + v1beta1.TensorflowServingRestPort,
										"--model_base_path=" + constants.DefaultModelLocalMountPath,
										"--rest_api_timeout_in_ms=60000",
									},
									Resources: defaultResource,
									ReadinessProbe: &v1.Probe{
										ProbeHandler: v1.ProbeHandler{
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
							AutomountServiceAccountToken: proto.Bool(false),
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
			Expect(actualDeployment.Spec).To(BeComparableTo(expectedDeployment.Spec))

			//check service
			actualService := &v1.Service{}
			predictorServiceKey := types.NamespacedName{Name: constants.PredictorServiceName(serviceKey.Name),
				Namespace: serviceKey.Namespace}
			Eventually(func() error { return k8sClient.Get(context.TODO(), predictorServiceKey, actualService) }, timeout).
				Should(Succeed())

			expectedService := &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      predictorServiceKey.Name,
					Namespace: predictorServiceKey.Namespace,
				},
				Spec: v1.ServiceSpec{
					Ports: []v1.ServicePort{
						{
							Name:       "raw-foo-2-predictor",
							Protocol:   "TCP",
							Port:       80,
							TargetPort: intstr.IntOrString{Type: 0, IntVal: 8080, StrVal: ""},
						},
					},
					Type:            "ClusterIP",
					SessionAffinity: "None",
					Selector: map[string]string{
						"app": "isvc.raw-foo-2-predictor",
					},
				},
			}
			actualService.Spec.ClusterIP = ""
			actualService.Spec.ClusterIPs = nil
			actualService.Spec.IPFamilies = nil
			actualService.Spec.IPFamilyPolicy = nil
			actualService.Spec.InternalTrafficPolicy = nil
			Expect(actualService.Spec).To(BeComparableTo(expectedService.Spec))

			//check isvc status
			updatedDeployment := actualDeployment.DeepCopy()
			updatedDeployment.Status.Conditions = []appsv1.DeploymentCondition{
				{
					Type:   appsv1.DeploymentAvailable,
					Status: v1.ConditionTrue,
				},
			}
			Expect(k8sClient.Status().Update(context.TODO(), updatedDeployment)).NotTo(HaveOccurred())

			//check http Route
			actualToplevelHttpRoute := &gatewayapiv1.HTTPRoute{}
			Eventually(func() error {
				return k8sClient.Get(context.TODO(), types.NamespacedName{Name: serviceKey.Name,
					Namespace: serviceKey.Namespace}, actualToplevelHttpRoute)
			}, timeout).
				Should(Succeed())
			topLevelHost := fmt.Sprintf("%s-%s.%s", serviceKey.Name, serviceKey.Namespace, "example.com")
			expectedToplevelHttpRoute := gatewayapiv1.HTTPRoute{
				Spec: gatewayapiv1.HTTPRouteSpec{
					Hostnames: []gatewayapiv1.Hostname{gatewayapiv1.Hostname(topLevelHost), "additional.example.com"},
					Rules: []gatewayapiv1.HTTPRouteRule{
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer(constants.FallbackPrefix()),
									},
								},
							},
							Filters: []gatewayapiv1.HTTPRouteFilter{
								{
									Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
										Set: []gatewayapiv1.HTTPHeader{
											{
												Name:  constants.IsvcNameHeader,
												Value: serviceKey.Name,
											},
											{
												Name:  constants.IsvcNamespaceHeader,
												Value: serviceKey.Namespace,
											},
										},
									},
								},
							},
							BackendRefs: []gatewayapiv1.HTTPBackendRef{
								{
									BackendRef: gatewayapiv1.BackendRef{
										BackendObjectReference: gatewayapiv1.BackendObjectReference{
											Group:     (*gatewayapiv1.Group)(utils.ToPointer("")),
											Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      gatewayapiv1.ObjectName(predictorServiceKey.Name),
											Namespace: (*gatewayapiv1.Namespace)(utils.ToPointer(serviceKey.Namespace)),
											Port:      (*gatewayapiv1.PortNumber)(utils.ToPointer(int32(constants.CommonDefaultHttpPort))),
										},
										Weight: utils.ToPointer(int32(1)),
									},
								},
							},
							Timeouts: &gatewayapiv1.HTTPRouteTimeouts{
								Request: utils.ToPointer(gatewayapiv1.Duration("0s")),
							},
						},
					},
					CommonRouteSpec: gatewayapiv1.CommonRouteSpec{
						ParentRefs: []gatewayapiv1.ParentReference{
							{
								Name:      gatewayapiv1.ObjectName(kserveGateway.Name),
								Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.GatewayKind)),
								Group:     (*gatewayapiv1.Group)(&gatewayapiv1.GroupVersion.Group),
								Namespace: utils.ToPointer(gatewayapiv1.Namespace(kserveGateway.Namespace)),
							},
						},
					},
				},
			}
			Expect(actualToplevelHttpRoute.Spec).To(BeComparableTo(expectedToplevelHttpRoute.Spec))

			actualPredictorHttpRoute := &gatewayapiv1.HTTPRoute{}
			Eventually(func() error {
				return k8sClient.Get(context.TODO(), types.NamespacedName{Name: predictorServiceKey.Name,
					Namespace: serviceKey.Namespace}, actualPredictorHttpRoute)
			}, timeout).
				Should(Succeed())
			predictorHost := fmt.Sprintf("%s-%s.%s", predictorServiceKey.Name, serviceKey.Namespace, "example.com")
			expectedPredictorHttpRoute := gatewayapiv1.HTTPRoute{
				Spec: gatewayapiv1.HTTPRouteSpec{
					Hostnames: []gatewayapiv1.Hostname{gatewayapiv1.Hostname(predictorHost)},
					Rules: []gatewayapiv1.HTTPRouteRule{
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer(constants.FallbackPrefix()),
									},
								},
							},
							Filters: []gatewayapiv1.HTTPRouteFilter{
								{
									Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
										Set: []gatewayapiv1.HTTPHeader{
											{
												Name:  constants.IsvcNameHeader,
												Value: serviceKey.Name,
											},
											{
												Name:  constants.IsvcNamespaceHeader,
												Value: serviceKey.Namespace,
											},
										},
									},
								},
							},
							BackendRefs: []gatewayapiv1.HTTPBackendRef{
								{
									BackendRef: gatewayapiv1.BackendRef{
										BackendObjectReference: gatewayapiv1.BackendObjectReference{
											Group:     (*gatewayapiv1.Group)(utils.ToPointer("")),
											Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      gatewayapiv1.ObjectName(predictorServiceKey.Name),
											Namespace: (*gatewayapiv1.Namespace)(utils.ToPointer(serviceKey.Namespace)),
											Port:      (*gatewayapiv1.PortNumber)(utils.ToPointer(int32(constants.CommonDefaultHttpPort))),
										},
										Weight: utils.ToPointer(int32(1)),
									},
								},
							},
							Timeouts: &gatewayapiv1.HTTPRouteTimeouts{
								Request: utils.ToPointer(gatewayapiv1.Duration("0s")),
							},
						},
					},
					CommonRouteSpec: gatewayapiv1.CommonRouteSpec{
						ParentRefs: []gatewayapiv1.ParentReference{
							{
								Name:      gatewayapiv1.ObjectName(kserveGateway.Name),
								Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.GatewayKind)),
								Group:     (*gatewayapiv1.Group)(&gatewayapiv1.GroupVersion.Group),
								Namespace: utils.ToPointer(gatewayapiv1.Namespace(kserveGateway.Namespace)),
							},
						},
					},
				},
			}
			Expect(actualPredictorHttpRoute.Spec).To(BeComparableTo(expectedPredictorHttpRoute.Spec))

			// Mark the Ingress as accepted to make isvc ready
			httpRouteStatus := gatewayapiv1.HTTPRouteStatus{
				RouteStatus: gatewayapiv1.RouteStatus{
					Parents: []gatewayapiv1.RouteParentStatus{
						{
							ParentRef: gatewayapiv1.ParentReference{
								Name:      gatewayapiv1.ObjectName(kserveGateway.Name),
								Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.GatewayKind)),
								Group:     (*gatewayapiv1.Group)(&gatewayapiv1.GroupVersion.Group),
								Namespace: utils.ToPointer(gatewayapiv1.Namespace(kserveGateway.Namespace)),
							},
							ControllerName: "istio.io/gateway-controller",
							Conditions: []metav1.Condition{
								{
									Type:               string(gatewayapiv1.ListenerConditionAccepted),
									Status:             metav1.ConditionTrue,
									Reason:             "Accepted",
									Message:            "Route was valid",
									LastTransitionTime: metav1.Now(),
								},
							},
						},
					},
				},
			}
			actualPredictorHttpRoute.Status = httpRouteStatus
			Expect(k8sClient.Status().Update(context.Background(), actualPredictorHttpRoute)).NotTo(HaveOccurred())
			actualToplevelHttpRoute.Status = httpRouteStatus
			Expect(k8sClient.Status().Update(context.Background(), actualToplevelHttpRoute)).NotTo(HaveOccurred())

			// verify if InferenceService status is updated
			expectedIsvcStatus := v1beta1.InferenceServiceStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{
							Type:   v1beta1.IngressReady,
							Status: "True",
						},
						{
							Type:   v1beta1.PredictorReady,
							Status: "True",
						},
						{
							Type:   apis.ConditionReady,
							Status: "True",
						},
					},
				},
				URL: &apis.URL{
					Scheme: "http",
					Host:   "raw-foo-2-default.example.com",
				},
				Address: &duckv1.Addressable{
					URL: &apis.URL{
						Scheme: "http",
						Host:   fmt.Sprintf("%s-predictor.%s.svc.cluster.local", serviceKey.Name, serviceKey.Namespace),
					},
				},
				Components: map[v1beta1.ComponentType]v1beta1.ComponentStatusSpec{
					v1beta1.PredictorComponent: {
						LatestCreatedRevision: "",
						URL: &apis.URL{
							Scheme: "http",
							Host:   "raw-foo-2-predictor-default.example.com",
						},
					},
				},
				ModelStatus: v1beta1.ModelStatus{
					TransitionStatus:    "InProgress",
					ModelRevisionStates: &v1beta1.ModelRevisionStates{TargetModelState: "Pending"},
				},
			}
			Eventually(func() string {
				isvc := &v1beta1.InferenceService{}
				if err := k8sClient.Get(context.TODO(), serviceKey, isvc); err != nil {
					return err.Error()
				}
				return cmp.Diff(&expectedIsvcStatus, &isvc.Status, cmpopts.IgnoreTypes(apis.VolatileTime{}))
			}, timeout).Should(BeEmpty())

			//check HPA is not created
			actualHPA := &autoscalingv2.HorizontalPodAutoscaler{}
			predictorHPAKey := types.NamespacedName{Name: constants.DefaultPredictorServiceName(serviceKey.Name),
				Namespace: serviceKey.Namespace}
			Eventually(func() error { return k8sClient.Get(context.TODO(), predictorHPAKey, actualHPA) }, timeout).
				Should(HaveOccurred())
		})
	})
	Context("When creating inference service with raw kube predictor and ingress creation disabled", func() {
		configs := map[string]string{
			"explainers": `{
	             "alibi": {
	                "image": "kfserving/alibi-explainer",
			      "defaultImageVersion": "latest"
	             }
	          }`,
			"ingress": `{
			   "kserveIngressGateway": "kserve/kserve-ingress-gateway",
               "ingressGateway": "knative-serving/knative-ingress-gateway",
               "localGateway": "knative-serving/knative-local-gateway",
               "localGatewayService": "knative-local-gateway.istio-system.svc.cluster.local",
               "ingressDomain": "example.com",
			   "additionalIngressDomains": ["additional.example.com"],
			   "disableIngressCreation": true
            }`,
		}

		It("Should have service/deployment/hpa created and http route should not be created", func() {
			By("By creating a new InferenceService")
			// Create configmap
			var configMap = &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KServeNamespace,
				},
				Data: configs,
			}
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)
			// Create ServingRuntime
			servingRuntime := &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tf-serving-raw",
					Namespace: "default",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{
							Name:       "tensorflow",
							Version:    proto.String("1"),
							AutoSelect: proto.Bool(true),
						},
					},
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []v1.Container{
							{
								Name:    "kserve-container",
								Image:   "tensorflow/serving:1.14.0",
								Command: []string{"/usr/bin/tensorflow_model_server"},
								Args: []string{
									"--port=9000",
									"--rest_api_port=8080",
									"--model_base_path=/mnt/models",
									"--rest_api_timeout_in_ms=60000",
								},
								Resources: defaultResource,
							},
						},
					},
					Disabled: proto.Bool(false),
				},
			}
			k8sClient.Create(context.TODO(), servingRuntime)
			defer k8sClient.Delete(context.TODO(), servingRuntime)
			// Create InferenceService
			serviceName := "raw-foo-no-ingress-class"
			var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: serviceName, Namespace: "default"}}
			var serviceKey = expectedRequest.NamespacedName
			var storageUri = "s3://test/mnist/export"
			ctx := context.Background()
			isvc := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceKey.Name,
					Namespace: serviceKey.Namespace,
					Annotations: map[string]string{
						"serving.kserve.io/deploymentMode":              "RawDeployment",
						"serving.kserve.io/autoscalerClass":             "hpa",
						"serving.kserve.io/metrics":                     "cpu",
						"serving.kserve.io/targetUtilizationPercentage": "75",
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
									Name:      constants.InferenceServiceContainerName,
									Resources: defaultResource,
								},
							},
						},
					},
				},
			}
			isvc.DefaultInferenceService(nil, nil, &v1beta1.SecurityConfig{AutoMountServiceAccountToken: false}, nil)
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
			defer k8sClient.Delete(ctx, isvc)

			inferenceService := &v1beta1.InferenceService{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, serviceKey, inferenceService)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			actualDeployment := &appsv1.Deployment{}
			predictorDeploymentKey := types.NamespacedName{Name: constants.PredictorServiceName(serviceKey.Name),
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
								constants.StorageInitializerSourceUriInternalAnnotationKey: *isvc.Spec.Predictor.Model.StorageURI,
								"serving.kserve.io/deploymentMode":                         "RawDeployment",
								"serving.kserve.io/autoscalerClass":                        "hpa",
								"serving.kserve.io/metrics":                                "cpu",
								"serving.kserve.io/targetUtilizationPercentage":            "75",
							},
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Image: "tensorflow/serving:" +
										*isvc.Spec.Predictor.Model.RuntimeVersion,
									Name:    constants.InferenceServiceContainerName,
									Command: []string{v1beta1.TensorflowEntrypointCommand},
									Args: []string{
										"--port=" + v1beta1.TensorflowServingGRPCPort,
										"--rest_api_port=" + v1beta1.TensorflowServingRestPort,
										"--model_base_path=" + constants.DefaultModelLocalMountPath,
										"--rest_api_timeout_in_ms=60000",
									},
									Resources: defaultResource,
									ReadinessProbe: &v1.Probe{
										ProbeHandler: v1.ProbeHandler{
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
							AutomountServiceAccountToken: proto.Bool(false),
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
			Expect(actualDeployment.Spec).To(BeComparableTo(expectedDeployment.Spec))

			//check service
			actualService := &v1.Service{}
			predictorServiceKey := types.NamespacedName{Name: constants.PredictorServiceName(serviceKey.Name),
				Namespace: serviceKey.Namespace}
			Eventually(func() error { return k8sClient.Get(context.TODO(), predictorServiceKey, actualService) }, timeout).
				Should(Succeed())

			expectedService := &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      predictorServiceKey.Name,
					Namespace: predictorServiceKey.Namespace,
				},
				Spec: v1.ServiceSpec{
					Ports: []v1.ServicePort{
						{
							Name:       constants.PredictorServiceName(serviceName),
							Protocol:   "TCP",
							Port:       80,
							TargetPort: intstr.IntOrString{Type: 0, IntVal: 8080, StrVal: ""},
						},
					},
					Type:            "ClusterIP",
					SessionAffinity: "None",
					Selector: map[string]string{
						"app": fmt.Sprintf("isvc.%s", constants.PredictorServiceName(serviceName)),
					},
				},
			}
			actualService.Spec.ClusterIP = ""
			actualService.Spec.ClusterIPs = nil
			actualService.Spec.IPFamilies = nil
			actualService.Spec.IPFamilyPolicy = nil
			actualService.Spec.InternalTrafficPolicy = nil
			Expect(actualService.Spec).To(BeComparableTo(expectedService.Spec))

			//check isvc status
			updatedDeployment := actualDeployment.DeepCopy()
			updatedDeployment.Status.Conditions = []appsv1.DeploymentCondition{
				{
					Type:   appsv1.DeploymentAvailable,
					Status: v1.ConditionTrue,
				},
			}
			Expect(k8sClient.Status().Update(context.TODO(), updatedDeployment)).NotTo(HaveOccurred())

			//check ingress not created
			actualToplevelHttpRoute := &gatewayapiv1.HTTPRoute{}
			Consistently(func() error {
				return k8sClient.Get(context.TODO(), types.NamespacedName{Name: serviceKey.Name,
					Namespace: serviceKey.Namespace}, actualToplevelHttpRoute)
			}, timeout).
				Should(Not(Succeed()))
			actualPredictorHttpRoute := &gatewayapiv1.HTTPRoute{}
			Consistently(func() error {
				return k8sClient.Get(context.TODO(), types.NamespacedName{Name: predictorServiceKey.Name,
					Namespace: serviceKey.Namespace}, actualPredictorHttpRoute)
			}, timeout).
				Should(Not(Succeed()))

			// verify if InferenceService status is updated
			expectedIsvcStatus := v1beta1.InferenceServiceStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{
							Type:   v1beta1.IngressReady,
							Status: "True",
						},
						{
							Type:   v1beta1.PredictorReady,
							Status: "True",
						},
						{
							Type:   apis.ConditionReady,
							Status: "True",
						},
					},
				},
				URL: &apis.URL{
					Scheme: "http",
					Host:   fmt.Sprintf("%s-default.example.com", serviceName),
				},
				Address: &duckv1.Addressable{
					URL: &apis.URL{
						Scheme: "http",
						Host:   fmt.Sprintf("%s-predictor.%s.svc.cluster.local", serviceKey.Name, serviceKey.Namespace),
					},
				},
				Components: map[v1beta1.ComponentType]v1beta1.ComponentStatusSpec{
					v1beta1.PredictorComponent: {
						LatestCreatedRevision: "",
						URL: &apis.URL{
							Scheme: "http",
							Host:   fmt.Sprintf("%s-predictor-default.example.com", serviceName),
						},
					},
				},
				ModelStatus: v1beta1.ModelStatus{
					TransitionStatus:    "InProgress",
					ModelRevisionStates: &v1beta1.ModelRevisionStates{TargetModelState: "Pending"},
				},
			}
			Eventually(func() string {
				isvc := &v1beta1.InferenceService{}
				if err := k8sClient.Get(context.TODO(), serviceKey, isvc); err != nil {
					return err.Error()
				}
				return cmp.Diff(&expectedIsvcStatus, &isvc.Status, cmpopts.IgnoreTypes(apis.VolatileTime{}))
			}, timeout).Should(BeEmpty())

			//check HPA
			var minReplicas int32 = 1
			var maxReplicas int32 = 3
			var cpuUtilization int32 = 75
			var stabilizationWindowSeconds int32 = 0
			selectPolicy := autoscalingv2.MaxChangePolicySelect
			actualHPA := &autoscalingv2.HorizontalPodAutoscaler{}
			predictorHPAKey := types.NamespacedName{Name: predictorServiceKey.Name,
				Namespace: serviceKey.Namespace}
			Eventually(func() error { return k8sClient.Get(context.TODO(), predictorHPAKey, actualHPA) }, timeout).
				Should(Succeed())
			expectedHPA := &autoscalingv2.HorizontalPodAutoscaler{
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       predictorServiceKey.Name,
					},
					MinReplicas: &minReplicas,
					MaxReplicas: maxReplicas,
					Metrics: []autoscalingv2.MetricSpec{
						{
							Type: autoscalingv2.ResourceMetricSourceType,
							Resource: &autoscalingv2.ResourceMetricSource{
								Name: v1.ResourceCPU,
								Target: autoscalingv2.MetricTarget{
									Type:               "Utilization",
									AverageUtilization: &cpuUtilization,
								},
							},
						},
					},
					Behavior: &autoscalingv2.HorizontalPodAutoscalerBehavior{
						ScaleUp: &autoscalingv2.HPAScalingRules{
							StabilizationWindowSeconds: &stabilizationWindowSeconds,
							SelectPolicy:               &selectPolicy,
							Policies: []autoscalingv2.HPAScalingPolicy{
								{
									Type:          "Pods",
									Value:         4,
									PeriodSeconds: 15,
								},
								{
									Type:          "Percent",
									Value:         100,
									PeriodSeconds: 15,
								},
							},
						},
						ScaleDown: &autoscalingv2.HPAScalingRules{
							StabilizationWindowSeconds: nil,
							SelectPolicy:               &selectPolicy,
							Policies: []autoscalingv2.HPAScalingPolicy{
								{
									Type:          "Percent",
									Value:         100,
									PeriodSeconds: 15,
								},
							},
						},
					},
				},
			}
			Expect(actualHPA.Spec).To(BeComparableTo(expectedHPA.Spec))
		})
	})
	Context("When creating inference service with raw kube predictor with domain template", func() {
		configs := map[string]string{
			"explainers": `{
               "alibi": {
                  "image": "kfserving/alibi-explainer",
			      "defaultImageVersion": "latest"
               }
            }`,
			"ingress": `{
			   "enableGatewayApi": true,
			   "kserveIngressGateway": "kserve/kserve-ingress-gateway",
               "ingressGateway": "knative-serving/knative-ingress-gateway",
               "localGateway": "knative-serving/knative-local-gateway",
               "localGatewayService": "knative-local-gateway.istio-system.svc.cluster.local",
               "ingressDomain": "example.com",
               "domainTemplate": "{{ .Name }}.{{ .Namespace }}.{{ .IngressDomain }}",
			   "additionalIngressDomains": ["additional.example.com"]
            }`,
		}

		It("Should have httproute/service/deployment/hpa created", func() {
			By("By creating a new InferenceService")
			// Create configmap
			var configMap = &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KServeNamespace,
				},
				Data: configs,
			}
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)
			// Create ServingRuntime
			servingRuntime := &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tf-serving-raw",
					Namespace: "default",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{
							Name:       "tensorflow",
							Version:    proto.String("1"),
							AutoSelect: proto.Bool(true),
						},
					},
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []v1.Container{
							{
								Name:    "kserve-container",
								Image:   "tensorflow/serving:1.14.0",
								Command: []string{"/usr/bin/tensorflow_model_server"},
								Args: []string{
									"--port=9000",
									"--rest_api_port=8080",
									"--model_base_path=/mnt/models",
									"--rest_api_timeout_in_ms=60000",
								},
								Resources: defaultResource,
							},
						},
					},
					Disabled: proto.Bool(false),
				},
			}
			k8sClient.Create(context.TODO(), servingRuntime)
			defer k8sClient.Delete(context.TODO(), servingRuntime)
			// Create InferenceService
			serviceName := "model"
			var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: serviceName, Namespace: "default"}}
			var serviceKey = expectedRequest.NamespacedName
			var storageUri = "s3://test/mnist/export"
			ctx := context.Background()
			isvc := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceKey.Name,
					Namespace: serviceKey.Namespace,
					Annotations: map[string]string{
						"serving.kserve.io/deploymentMode":              "RawDeployment",
						"serving.kserve.io/autoscalerClass":             "hpa",
						"serving.kserve.io/metrics":                     "cpu",
						"serving.kserve.io/targetUtilizationPercentage": "75",
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
									Name:      constants.InferenceServiceContainerName,
									Resources: defaultResource,
								},
							},
						},
					},
				},
			}
			isvc.DefaultInferenceService(nil, nil, &v1beta1.SecurityConfig{AutoMountServiceAccountToken: false}, nil)
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
			defer k8sClient.Delete(ctx, isvc)

			inferenceService := &v1beta1.InferenceService{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, serviceKey, inferenceService)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			actualDeployment := &appsv1.Deployment{}
			predictorDeploymentKey := types.NamespacedName{Name: constants.PredictorServiceName(serviceKey.Name),
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
								constants.StorageInitializerSourceUriInternalAnnotationKey: *isvc.Spec.Predictor.Model.StorageURI,
								"serving.kserve.io/deploymentMode":                         "RawDeployment",
								"serving.kserve.io/autoscalerClass":                        "hpa",
								"serving.kserve.io/metrics":                                "cpu",
								"serving.kserve.io/targetUtilizationPercentage":            "75",
							},
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Image: "tensorflow/serving:" +
										*isvc.Spec.Predictor.Model.RuntimeVersion,
									Name:    constants.InferenceServiceContainerName,
									Command: []string{v1beta1.TensorflowEntrypointCommand},
									Args: []string{
										"--port=" + v1beta1.TensorflowServingGRPCPort,
										"--rest_api_port=" + v1beta1.TensorflowServingRestPort,
										"--model_base_path=" + constants.DefaultModelLocalMountPath,
										"--rest_api_timeout_in_ms=60000",
									},
									Resources: defaultResource,
									ReadinessProbe: &v1.Probe{
										ProbeHandler: v1.ProbeHandler{
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
							AutomountServiceAccountToken: proto.Bool(false),
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
			Expect(actualDeployment.Spec).To(BeComparableTo(expectedDeployment.Spec))

			//check service
			actualService := &v1.Service{}
			predictorServiceKey := types.NamespacedName{Name: constants.PredictorServiceName(serviceKey.Name),
				Namespace: serviceKey.Namespace}
			Eventually(func() error { return k8sClient.Get(context.TODO(), predictorServiceKey, actualService) }, timeout).
				Should(Succeed())

			expectedService := &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      predictorServiceKey.Name,
					Namespace: predictorServiceKey.Namespace,
				},
				Spec: v1.ServiceSpec{
					Ports: []v1.ServicePort{
						{
							Name:       constants.PredictorServiceName(serviceName),
							Protocol:   "TCP",
							Port:       80,
							TargetPort: intstr.IntOrString{Type: 0, IntVal: 8080, StrVal: ""},
						},
					},
					Type:            "ClusterIP",
					SessionAffinity: "None",
					Selector: map[string]string{
						"app": fmt.Sprintf("isvc.%s", constants.PredictorServiceName(serviceName)),
					},
				},
			}
			actualService.Spec.ClusterIP = ""
			actualService.Spec.ClusterIPs = nil
			actualService.Spec.IPFamilies = nil
			actualService.Spec.IPFamilyPolicy = nil
			actualService.Spec.InternalTrafficPolicy = nil
			Expect(actualService.Spec).To(BeComparableTo(expectedService.Spec))

			//check isvc status
			updatedDeployment := actualDeployment.DeepCopy()
			updatedDeployment.Status.Conditions = []appsv1.DeploymentCondition{
				{
					Type:   appsv1.DeploymentAvailable,
					Status: v1.ConditionTrue,
				},
			}
			Expect(k8sClient.Status().Update(context.TODO(), updatedDeployment)).NotTo(HaveOccurred())

			//check http route
			actualToplevelHttpRoute := &gatewayapiv1.HTTPRoute{}
			Eventually(func() error {
				return k8sClient.Get(context.TODO(), types.NamespacedName{Name: serviceKey.Name,
					Namespace: serviceKey.Namespace}, actualToplevelHttpRoute)
			}, timeout).
				Should(Succeed())
			topLevelHost := fmt.Sprintf("%s.%s.%s", serviceKey.Name, serviceKey.Namespace, "example.com")
			expectedToplevelHttpRoute := gatewayapiv1.HTTPRoute{
				Spec: gatewayapiv1.HTTPRouteSpec{
					Hostnames: []gatewayapiv1.Hostname{gatewayapiv1.Hostname(topLevelHost), "additional.example.com"},
					Rules: []gatewayapiv1.HTTPRouteRule{
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer(constants.FallbackPrefix()),
									},
								},
							},
							Filters: []gatewayapiv1.HTTPRouteFilter{
								{
									Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
										Set: []gatewayapiv1.HTTPHeader{
											{
												Name:  constants.IsvcNameHeader,
												Value: serviceKey.Name,
											},
											{
												Name:  constants.IsvcNamespaceHeader,
												Value: serviceKey.Namespace,
											},
										},
									},
								},
							},
							BackendRefs: []gatewayapiv1.HTTPBackendRef{
								{
									BackendRef: gatewayapiv1.BackendRef{
										BackendObjectReference: gatewayapiv1.BackendObjectReference{
											Group:     (*gatewayapiv1.Group)(utils.ToPointer("")),
											Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      gatewayapiv1.ObjectName(predictorServiceKey.Name),
											Namespace: (*gatewayapiv1.Namespace)(utils.ToPointer(serviceKey.Namespace)),
											Port:      (*gatewayapiv1.PortNumber)(utils.ToPointer(int32(constants.CommonDefaultHttpPort))),
										},
										Weight: utils.ToPointer(int32(1)),
									},
								},
							},
							Timeouts: &gatewayapiv1.HTTPRouteTimeouts{
								Request: utils.ToPointer(gatewayapiv1.Duration("0s")),
							},
						},
					},
					CommonRouteSpec: gatewayapiv1.CommonRouteSpec{
						ParentRefs: []gatewayapiv1.ParentReference{
							{
								Name:      gatewayapiv1.ObjectName(kserveGateway.Name),
								Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.GatewayKind)),
								Group:     (*gatewayapiv1.Group)(&gatewayapiv1.GroupVersion.Group),
								Namespace: utils.ToPointer(gatewayapiv1.Namespace(kserveGateway.Namespace)),
							},
						},
					},
				},
			}
			Expect(actualToplevelHttpRoute.Spec).To(BeComparableTo(expectedToplevelHttpRoute.Spec))

			actualPredictorHttpRoute := &gatewayapiv1.HTTPRoute{}
			Eventually(func() error {
				return k8sClient.Get(context.TODO(), types.NamespacedName{Name: predictorServiceKey.Name,
					Namespace: serviceKey.Namespace}, actualPredictorHttpRoute)
			}, timeout).
				Should(Succeed())
			predictorHost := fmt.Sprintf("%s.%s.%s", predictorServiceKey.Name, serviceKey.Namespace, "example.com")
			expectedPredictorHttpRoute := gatewayapiv1.HTTPRoute{
				Spec: gatewayapiv1.HTTPRouteSpec{
					Hostnames: []gatewayapiv1.Hostname{gatewayapiv1.Hostname(predictorHost)},
					Rules: []gatewayapiv1.HTTPRouteRule{
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer(constants.FallbackPrefix()),
									},
								},
							},
							Filters: []gatewayapiv1.HTTPRouteFilter{
								{
									Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
										Set: []gatewayapiv1.HTTPHeader{
											{
												Name:  constants.IsvcNameHeader,
												Value: serviceKey.Name,
											},
											{
												Name:  constants.IsvcNamespaceHeader,
												Value: serviceKey.Namespace,
											},
										},
									},
								},
							},
							BackendRefs: []gatewayapiv1.HTTPBackendRef{
								{
									BackendRef: gatewayapiv1.BackendRef{
										BackendObjectReference: gatewayapiv1.BackendObjectReference{
											Group:     (*gatewayapiv1.Group)(utils.ToPointer("")),
											Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      gatewayapiv1.ObjectName(predictorServiceKey.Name),
											Namespace: (*gatewayapiv1.Namespace)(utils.ToPointer(serviceKey.Namespace)),
											Port:      (*gatewayapiv1.PortNumber)(utils.ToPointer(int32(constants.CommonDefaultHttpPort))),
										},
										Weight: utils.ToPointer(int32(1)),
									},
								},
							},
							Timeouts: &gatewayapiv1.HTTPRouteTimeouts{
								Request: utils.ToPointer(gatewayapiv1.Duration("0s")),
							},
						},
					},
					CommonRouteSpec: gatewayapiv1.CommonRouteSpec{
						ParentRefs: []gatewayapiv1.ParentReference{
							{
								Name:      gatewayapiv1.ObjectName(kserveGateway.Name),
								Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.GatewayKind)),
								Group:     (*gatewayapiv1.Group)(&gatewayapiv1.GroupVersion.Group),
								Namespace: utils.ToPointer(gatewayapiv1.Namespace(kserveGateway.Namespace)),
							},
						},
					},
				},
			}
			Expect(actualPredictorHttpRoute.Spec).To(BeComparableTo(expectedPredictorHttpRoute.Spec))

			// Mark the Ingress as accepted to make isvc ready
			httpRouteStatus := gatewayapiv1.HTTPRouteStatus{
				RouteStatus: gatewayapiv1.RouteStatus{
					Parents: []gatewayapiv1.RouteParentStatus{
						{
							ParentRef: gatewayapiv1.ParentReference{
								Name:      gatewayapiv1.ObjectName(kserveGateway.Name),
								Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.GatewayKind)),
								Group:     (*gatewayapiv1.Group)(&gatewayapiv1.GroupVersion.Group),
								Namespace: utils.ToPointer(gatewayapiv1.Namespace(kserveGateway.Namespace)),
							},
							ControllerName: "istio.io/gateway-controller",
							Conditions: []metav1.Condition{
								{
									Type:               string(gatewayapiv1.ListenerConditionAccepted),
									Status:             metav1.ConditionTrue,
									Reason:             "Accepted",
									Message:            "Route was valid",
									LastTransitionTime: metav1.Now(),
								},
							},
						},
					},
				},
			}
			actualPredictorHttpRoute.Status = httpRouteStatus
			Expect(k8sClient.Status().Update(context.Background(), actualPredictorHttpRoute)).NotTo(HaveOccurred())
			actualToplevelHttpRoute.Status = httpRouteStatus
			Expect(k8sClient.Status().Update(context.Background(), actualToplevelHttpRoute)).NotTo(HaveOccurred())

			// verify if InferenceService status is updated
			expectedIsvcStatus := v1beta1.InferenceServiceStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{
							Type:   v1beta1.IngressReady,
							Status: "True",
						},
						{
							Type:   v1beta1.PredictorReady,
							Status: "True",
						},
						{
							Type:   apis.ConditionReady,
							Status: "True",
						},
					},
				},
				URL: &apis.URL{
					Scheme: "http",
					Host:   fmt.Sprintf("%s.%s.%s", serviceName, serviceKey.Namespace, domain),
				},
				Address: &duckv1.Addressable{
					URL: &apis.URL{
						Scheme: "http",
						Host:   fmt.Sprintf("%s-predictor.%s.svc.cluster.local", serviceKey.Name, serviceKey.Namespace),
					},
				},
				Components: map[v1beta1.ComponentType]v1beta1.ComponentStatusSpec{
					v1beta1.PredictorComponent: {
						LatestCreatedRevision: "",
						URL: &apis.URL{
							Scheme: "http",
							Host:   fmt.Sprintf("%s-predictor.%s.%s", serviceName, serviceKey.Namespace, domain),
						},
					},
				},
				ModelStatus: v1beta1.ModelStatus{
					TransitionStatus:    "InProgress",
					ModelRevisionStates: &v1beta1.ModelRevisionStates{TargetModelState: "Pending"},
				},
			}
			Eventually(func() string {
				isvc := &v1beta1.InferenceService{}
				if err := k8sClient.Get(context.TODO(), serviceKey, isvc); err != nil {
					return err.Error()
				}
				return cmp.Diff(&expectedIsvcStatus, &isvc.Status, cmpopts.IgnoreTypes(apis.VolatileTime{}))
			}, timeout).Should(BeEmpty())

			//check HPA
			var minReplicas int32 = 1
			var maxReplicas int32 = 3
			var cpuUtilization int32 = 75
			var stabilizationWindowSeconds int32 = 0
			selectPolicy := autoscalingv2.MaxChangePolicySelect
			actualHPA := &autoscalingv2.HorizontalPodAutoscaler{}
			predictorHPAKey := types.NamespacedName{Name: predictorServiceKey.Name,
				Namespace: serviceKey.Namespace}
			Eventually(func() error { return k8sClient.Get(context.TODO(), predictorHPAKey, actualHPA) }, timeout).
				Should(Succeed())
			expectedHPA := &autoscalingv2.HorizontalPodAutoscaler{
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       predictorServiceKey.Name,
					},
					MinReplicas: &minReplicas,
					MaxReplicas: maxReplicas,
					Metrics: []autoscalingv2.MetricSpec{
						{
							Type: autoscalingv2.ResourceMetricSourceType,
							Resource: &autoscalingv2.ResourceMetricSource{
								Name: v1.ResourceCPU,
								Target: autoscalingv2.MetricTarget{
									Type:               "Utilization",
									AverageUtilization: &cpuUtilization,
								},
							},
						},
					},
					Behavior: &autoscalingv2.HorizontalPodAutoscalerBehavior{
						ScaleUp: &autoscalingv2.HPAScalingRules{
							StabilizationWindowSeconds: &stabilizationWindowSeconds,
							SelectPolicy:               &selectPolicy,
							Policies: []autoscalingv2.HPAScalingPolicy{
								{
									Type:          "Pods",
									Value:         4,
									PeriodSeconds: 15,
								},
								{
									Type:          "Percent",
									Value:         100,
									PeriodSeconds: 15,
								},
							},
						},
						ScaleDown: &autoscalingv2.HPAScalingRules{
							StabilizationWindowSeconds: nil,
							SelectPolicy:               &selectPolicy,
							Policies: []autoscalingv2.HPAScalingPolicy{
								{
									Type:          "Percent",
									Value:         100,
									PeriodSeconds: 15,
								},
							},
						},
					},
				},
			}
			Expect(actualHPA.Spec).To(BeComparableTo(expectedHPA.Spec))
		})
	})
	Context("When creating inference service with raw kube predictor and transformer", func() {
		configs := map[string]string{
			"explainers": `{
				"alibi": {
					"image": "kserve/alibi-explainer",
					"defaultImageVersion": "latest"
				}
			}`,
			"ingress": `{
				"enableGatewayApi": true,
				"kserveIngressGateway": "kserve/kserve-ingress-gateway",
				"ingressGateway": "knative-serving/knative-ingress-gateway",
				"localGateway": "knative-serving/knative-local-gateway",
				"localGatewayService": "knative-local-gateway.istio-system.svc.cluster.local",
				"additionalIngressDomains": ["additional.example.com"]
			}`,
			"storageInitializer": `{
				"image" : "kserve/storage-initializer:latest",
				"memoryRequest": "100Mi",
				"memoryLimit": "1Gi",
				"cpuRequest": "100m",
				"cpuLimit": "1",
				"CaBundleConfigMapName": "",
				"caBundleVolumeMountPath": "/etc/ssl/custom-certs",
				"enableDirectPvcVolumeMount": false
			}`,
		}

		It("Should have httproute/service/deployment/hpa created for transformer and predictor", func() {
			By("By creating a new InferenceService")
			// Create configmap
			var configMap = &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KServeNamespace,
				},
				Data: configs,
			}
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)
			// Create ServingRuntime
			servingRuntime := &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tf-serving-raw",
					Namespace: "default",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{
							Name:       "tensorflow",
							Version:    proto.String("1"),
							AutoSelect: proto.Bool(true),
						},
					},
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []v1.Container{
							{
								Name:    "kserve-container",
								Image:   "tensorflow/serving:1.14.0",
								Command: []string{"/usr/bin/tensorflow_model_server"},
								Args: []string{
									"--port=9000",
									"--rest_api_port=8080",
									"--model_base_path=/mnt/models",
									"--rest_api_timeout_in_ms=60000",
								},
								Resources: defaultResource,
							},
						},
					},
					Disabled: proto.Bool(false),
				},
			}
			k8sClient.Create(context.TODO(), servingRuntime)
			defer k8sClient.Delete(context.TODO(), servingRuntime)
			serviceName := "raw-foo-trans"
			namespace := "default"
			var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: serviceName, Namespace: namespace}}
			var serviceKey = expectedRequest.NamespacedName
			var predictorServiceKey = types.NamespacedName{Name: constants.PredictorServiceName(serviceName),
				Namespace: namespace}
			var transformerServiceKey = types.NamespacedName{Name: constants.TransformerServiceName(serviceName),
				Namespace: namespace}
			var storageUri = "s3://test/mnist/export"
			var minReplicas int32 = 1
			var maxReplicas int32 = 3
			var transformerMinReplicas int32 = 1
			var transformerMaxReplicas int32 = 2
			var transformerCpuUtilization int32 = 80
			var transformerStabilizationWindowSeconds int32 = 0
			transformerSelectPolicy := autoscalingv2.MaxChangePolicySelect
			ctx := context.Background()
			isvc := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceKey.Name,
					Namespace: serviceKey.Namespace,
					Annotations: map[string]string{
						"serving.kserve.io/deploymentMode":              "RawDeployment",
						"serving.kserve.io/autoscalerClass":             "hpa",
						"serving.kserve.io/metrics":                     "cpu",
						"serving.kserve.io/targetUtilizationPercentage": "75",
					},
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
							MinReplicas: utils.ToPointer(int(minReplicas)),
							MaxReplicas: int(maxReplicas),
						},
						Tensorflow: &v1beta1.TFServingSpec{
							PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
								StorageURI:     &storageUri,
								RuntimeVersion: proto.String("1.14.0"),
								Container: v1.Container{
									Name:      constants.InferenceServiceContainerName,
									Resources: defaultResource,
								},
							},
						},
					},
					Transformer: &v1beta1.TransformerSpec{
						ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
							MinReplicas: utils.ToPointer(int(transformerMinReplicas)),
							MaxReplicas: int(transformerMaxReplicas),
							ScaleTarget: utils.ToPointer(int(transformerCpuUtilization)),
						},
						PodSpec: v1beta1.PodSpec{
							Containers: []v1.Container{
								{
									Image:     "transformer:v1",
									Resources: defaultResource,
									Args: []string{
										"--port=8080",
									},
								},
							},
						},
					},
				},
			}
			isvc.DefaultInferenceService(nil, nil, &v1beta1.SecurityConfig{AutoMountServiceAccountToken: false}, nil)
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
			defer k8sClient.Delete(ctx, isvc)

			inferenceService := &v1beta1.InferenceService{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, serviceKey, inferenceService)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			// check predictor deployment
			actualPredictorDeployment := &appsv1.Deployment{}
			predictorDeploymentKey := types.NamespacedName{Name: predictorServiceKey.Name,
				Namespace: serviceKey.Namespace}
			Eventually(func() error { return k8sClient.Get(context.TODO(), predictorDeploymentKey, actualPredictorDeployment) }, timeout).
				Should(Succeed())
			var replicas int32 = 1
			var revisionHistory int32 = 10
			var progressDeadlineSeconds int32 = 600
			var gracePeriod int64 = 30
			expectedPredictorDeployment := &appsv1.Deployment{
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
								constants.StorageInitializerSourceUriInternalAnnotationKey: *isvc.Spec.Predictor.Model.StorageURI,
								"serving.kserve.io/deploymentMode":                         "RawDeployment",
								"serving.kserve.io/autoscalerClass":                        "hpa",
								"serving.kserve.io/metrics":                                "cpu",
								"serving.kserve.io/targetUtilizationPercentage":            "75",
							},
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Image: "tensorflow/serving:" +
										*isvc.Spec.Predictor.Model.RuntimeVersion,
									Name:    constants.InferenceServiceContainerName,
									Command: []string{v1beta1.TensorflowEntrypointCommand},
									Args: []string{
										"--port=" + v1beta1.TensorflowServingGRPCPort,
										"--rest_api_port=" + v1beta1.TensorflowServingRestPort,
										"--model_base_path=" + constants.DefaultModelLocalMountPath,
										"--rest_api_timeout_in_ms=60000",
									},
									Resources: defaultResource,
									ReadinessProbe: &v1.Probe{
										ProbeHandler: v1.ProbeHandler{
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
							AutomountServiceAccountToken: proto.Bool(false),
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
			Expect(actualPredictorDeployment.Spec).To(BeComparableTo(expectedPredictorDeployment.Spec))

			// check transformer deployment
			actualTransformerDeployment := &appsv1.Deployment{}
			transformerDeploymentKey := types.NamespacedName{Name: transformerServiceKey.Name,
				Namespace: serviceKey.Namespace}
			Eventually(func() error {
				return k8sClient.Get(context.TODO(), transformerDeploymentKey, actualTransformerDeployment)
			}, timeout).
				Should(Succeed())
			expectedTransformerDeployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      transformerDeploymentKey.Name,
					Namespace: transformerDeploymentKey.Namespace,
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: &replicas,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "isvc." + transformerDeploymentKey.Name,
						},
					},
					Template: v1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Name:      transformerDeploymentKey.Name,
							Namespace: "default",
							Labels: map[string]string{
								"app":                                 "isvc." + transformerDeploymentKey.Name,
								constants.KServiceComponentLabel:      constants.Transformer.String(),
								constants.InferenceServicePodLabelKey: serviceName,
							},
							Annotations: map[string]string{
								"serving.kserve.io/deploymentMode":              "RawDeployment",
								"serving.kserve.io/autoscalerClass":             "hpa",
								"serving.kserve.io/metrics":                     "cpu",
								"serving.kserve.io/targetUtilizationPercentage": "75",
							},
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Image: "transformer:v1",
									Name:  constants.InferenceServiceContainerName,
									Args: []string{
										"--port=8080",
										"--model_name",
										serviceKey.Name,
										"--predictor_host",
										fmt.Sprintf("%s.%s", predictorServiceKey.Name, predictorServiceKey.Namespace),
										"--http_port",
										"8080",
									},
									Resources: defaultResource,
									ReadinessProbe: &v1.Probe{
										ProbeHandler: v1.ProbeHandler{
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
							AutomountServiceAccountToken: proto.Bool(false),
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
			Expect(actualTransformerDeployment.Spec).To(BeComparableTo(expectedTransformerDeployment.Spec))

			//check predictor service
			actualPredictorService := &v1.Service{}
			Eventually(func() error { return k8sClient.Get(context.TODO(), predictorServiceKey, actualPredictorService) }, timeout).
				Should(Succeed())
			expectedPredictorService := &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      predictorServiceKey.Name,
					Namespace: predictorServiceKey.Namespace,
				},
				Spec: v1.ServiceSpec{
					Ports: []v1.ServicePort{
						{
							Name:       predictorServiceKey.Name,
							Protocol:   "TCP",
							Port:       80,
							TargetPort: intstr.IntOrString{Type: 0, IntVal: 8080, StrVal: ""},
						},
					},
					Type:            "ClusterIP",
					SessionAffinity: "None",
					Selector: map[string]string{
						"app": fmt.Sprintf("isvc.%s", predictorServiceKey.Name),
					},
				},
			}
			actualPredictorService.Spec.ClusterIP = ""
			actualPredictorService.Spec.ClusterIPs = nil
			actualPredictorService.Spec.IPFamilies = nil
			actualPredictorService.Spec.IPFamilyPolicy = nil
			actualPredictorService.Spec.InternalTrafficPolicy = nil
			Expect(actualPredictorService.Spec).To(BeComparableTo(expectedPredictorService.Spec))

			//check transformer service
			actualTransformerService := &v1.Service{}
			Eventually(func() error { return k8sClient.Get(context.TODO(), transformerServiceKey, actualTransformerService) }, timeout).
				Should(Succeed())
			expectedTransformerService := &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      transformerServiceKey.Name,
					Namespace: transformerServiceKey.Namespace,
				},
				Spec: v1.ServiceSpec{
					Ports: []v1.ServicePort{
						{
							Name:       transformerServiceKey.Name,
							Protocol:   "TCP",
							Port:       80,
							TargetPort: intstr.IntOrString{Type: 0, IntVal: 8080, StrVal: ""},
						},
					},
					Type:            "ClusterIP",
					SessionAffinity: "None",
					Selector: map[string]string{
						"app": fmt.Sprintf("isvc.%s", transformerServiceKey.Name),
					},
				},
			}
			actualTransformerService.Spec.ClusterIP = ""
			actualTransformerService.Spec.ClusterIPs = nil
			actualTransformerService.Spec.IPFamilies = nil
			actualTransformerService.Spec.IPFamilyPolicy = nil
			actualTransformerService.Spec.InternalTrafficPolicy = nil
			Expect(actualTransformerService.Spec).To(BeComparableTo(expectedTransformerService.Spec))

			// update deployment status to make isvc ready
			updatedPredictorDeployment := actualPredictorDeployment.DeepCopy()
			updatedPredictorDeployment.Status.Conditions = []appsv1.DeploymentCondition{
				{
					Type:   appsv1.DeploymentAvailable,
					Status: v1.ConditionTrue,
				},
			}
			Expect(k8sClient.Status().Update(context.TODO(), updatedPredictorDeployment)).NotTo(HaveOccurred())
			updatedTransformerDeployment := actualTransformerDeployment.DeepCopy()
			updatedTransformerDeployment.Status.Conditions = []appsv1.DeploymentCondition{
				{
					Type:   appsv1.DeploymentAvailable,
					Status: v1.ConditionTrue,
				},
			}
			Expect(k8sClient.Status().Update(context.TODO(), updatedTransformerDeployment)).NotTo(HaveOccurred())

			//check http route
			actualToplevelHttpRoute := &gatewayapiv1.HTTPRoute{}
			Eventually(func() error {
				return k8sClient.Get(context.TODO(), types.NamespacedName{Name: serviceKey.Name,
					Namespace: serviceKey.Namespace}, actualToplevelHttpRoute)
			}, timeout).
				Should(Succeed())
			topLevelHost := fmt.Sprintf("%s-%s.%s", serviceKey.Name, serviceKey.Namespace, "example.com")
			expectedToplevelHttpRoute := gatewayapiv1.HTTPRoute{
				Spec: gatewayapiv1.HTTPRouteSpec{
					Hostnames: []gatewayapiv1.Hostname{gatewayapiv1.Hostname(topLevelHost), "additional.example.com"},
					Rules: []gatewayapiv1.HTTPRouteRule{
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer(constants.FallbackPrefix()),
									},
								},
							},
							Filters: []gatewayapiv1.HTTPRouteFilter{
								{
									Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
										Set: []gatewayapiv1.HTTPHeader{
											{
												Name:  constants.IsvcNameHeader,
												Value: serviceKey.Name,
											},
											{
												Name:  constants.IsvcNamespaceHeader,
												Value: serviceKey.Namespace,
											},
										},
									},
								},
							},
							BackendRefs: []gatewayapiv1.HTTPBackendRef{
								{
									BackendRef: gatewayapiv1.BackendRef{
										BackendObjectReference: gatewayapiv1.BackendObjectReference{
											Group:     (*gatewayapiv1.Group)(utils.ToPointer("")),
											Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      gatewayapiv1.ObjectName(transformerServiceKey.Name),
											Namespace: (*gatewayapiv1.Namespace)(utils.ToPointer(serviceKey.Namespace)),
											Port:      (*gatewayapiv1.PortNumber)(utils.ToPointer(int32(constants.CommonDefaultHttpPort))),
										},
										Weight: utils.ToPointer(int32(1)),
									},
								},
							},
							Timeouts: &gatewayapiv1.HTTPRouteTimeouts{
								Request: utils.ToPointer(gatewayapiv1.Duration("0s")),
							},
						},
					},
					CommonRouteSpec: gatewayapiv1.CommonRouteSpec{
						ParentRefs: []gatewayapiv1.ParentReference{
							{
								Name:      gatewayapiv1.ObjectName(kserveGateway.Name),
								Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.GatewayKind)),
								Group:     (*gatewayapiv1.Group)(&gatewayapiv1.GroupVersion.Group),
								Namespace: utils.ToPointer(gatewayapiv1.Namespace(kserveGateway.Namespace)),
							},
						},
					},
				},
			}
			Expect(actualToplevelHttpRoute.Spec).To(BeComparableTo(expectedToplevelHttpRoute.Spec))

			actualPredictorHttpRoute := &gatewayapiv1.HTTPRoute{}
			Eventually(func() error {
				return k8sClient.Get(context.TODO(), types.NamespacedName{Name: predictorServiceKey.Name,
					Namespace: serviceKey.Namespace}, actualPredictorHttpRoute)
			}, timeout).
				Should(Succeed())
			predictorHost := fmt.Sprintf("%s-%s.%s", predictorServiceKey.Name, serviceKey.Namespace, "example.com")
			expectedPredictorHttpRoute := gatewayapiv1.HTTPRoute{
				Spec: gatewayapiv1.HTTPRouteSpec{
					Hostnames: []gatewayapiv1.Hostname{gatewayapiv1.Hostname(predictorHost)},
					Rules: []gatewayapiv1.HTTPRouteRule{
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer(constants.FallbackPrefix()),
									},
								},
							},
							Filters: []gatewayapiv1.HTTPRouteFilter{
								{
									Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
										Set: []gatewayapiv1.HTTPHeader{
											{
												Name:  constants.IsvcNameHeader,
												Value: serviceKey.Name,
											},
											{
												Name:  constants.IsvcNamespaceHeader,
												Value: serviceKey.Namespace,
											},
										},
									},
								},
							},
							BackendRefs: []gatewayapiv1.HTTPBackendRef{
								{
									BackendRef: gatewayapiv1.BackendRef{
										BackendObjectReference: gatewayapiv1.BackendObjectReference{
											Group:     (*gatewayapiv1.Group)(utils.ToPointer("")),
											Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      gatewayapiv1.ObjectName(predictorServiceKey.Name),
											Namespace: (*gatewayapiv1.Namespace)(utils.ToPointer(serviceKey.Namespace)),
											Port:      (*gatewayapiv1.PortNumber)(utils.ToPointer(int32(constants.CommonDefaultHttpPort))),
										},
										Weight: utils.ToPointer(int32(1)),
									},
								},
							},
							Timeouts: &gatewayapiv1.HTTPRouteTimeouts{
								Request: utils.ToPointer(gatewayapiv1.Duration("0s")),
							},
						},
					},
					CommonRouteSpec: gatewayapiv1.CommonRouteSpec{
						ParentRefs: []gatewayapiv1.ParentReference{
							{
								Name:      gatewayapiv1.ObjectName(kserveGateway.Name),
								Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.GatewayKind)),
								Group:     (*gatewayapiv1.Group)(&gatewayapiv1.GroupVersion.Group),
								Namespace: utils.ToPointer(gatewayapiv1.Namespace(kserveGateway.Namespace)),
							},
						},
					},
				},
			}
			Expect(actualPredictorHttpRoute.Spec).To(BeComparableTo(expectedPredictorHttpRoute.Spec))

			actualTransformerHttpRoute := &gatewayapiv1.HTTPRoute{}
			Eventually(func() error {
				return k8sClient.Get(context.TODO(), types.NamespacedName{Name: transformerServiceKey.Name,
					Namespace: serviceKey.Namespace}, actualTransformerHttpRoute)
			}, timeout).
				Should(Succeed())
			transformerHost := fmt.Sprintf("%s-%s.%s", transformerServiceKey.Name, serviceKey.Namespace, "example.com")
			expectedTransformerHttpRoute := gatewayapiv1.HTTPRoute{
				Spec: gatewayapiv1.HTTPRouteSpec{
					Hostnames: []gatewayapiv1.Hostname{gatewayapiv1.Hostname(transformerHost)},
					Rules: []gatewayapiv1.HTTPRouteRule{
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer(constants.FallbackPrefix()),
									},
								},
							},
							Filters: []gatewayapiv1.HTTPRouteFilter{
								{
									Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
										Set: []gatewayapiv1.HTTPHeader{
											{
												Name:  constants.IsvcNameHeader,
												Value: serviceKey.Name,
											},
											{
												Name:  constants.IsvcNamespaceHeader,
												Value: serviceKey.Namespace,
											},
										},
									},
								},
							},
							BackendRefs: []gatewayapiv1.HTTPBackendRef{
								{
									BackendRef: gatewayapiv1.BackendRef{
										BackendObjectReference: gatewayapiv1.BackendObjectReference{
											Group:     (*gatewayapiv1.Group)(utils.ToPointer("")),
											Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      gatewayapiv1.ObjectName(transformerServiceKey.Name),
											Namespace: (*gatewayapiv1.Namespace)(utils.ToPointer(serviceKey.Namespace)),
											Port:      (*gatewayapiv1.PortNumber)(utils.ToPointer(int32(constants.CommonDefaultHttpPort))),
										},
										Weight: utils.ToPointer(int32(1)),
									},
								},
							},
							Timeouts: &gatewayapiv1.HTTPRouteTimeouts{
								Request: utils.ToPointer(gatewayapiv1.Duration("0s")),
							},
						},
					},
					CommonRouteSpec: gatewayapiv1.CommonRouteSpec{
						ParentRefs: []gatewayapiv1.ParentReference{
							{
								Name:      gatewayapiv1.ObjectName(kserveGateway.Name),
								Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.GatewayKind)),
								Group:     (*gatewayapiv1.Group)(&gatewayapiv1.GroupVersion.Group),
								Namespace: utils.ToPointer(gatewayapiv1.Namespace(kserveGateway.Namespace)),
							},
						},
					},
				},
			}
			Expect(actualTransformerHttpRoute.Spec).To(BeComparableTo(expectedTransformerHttpRoute.Spec))

			// Mark the Ingress as accepted to make isvc ready
			httpRouteStatus := gatewayapiv1.HTTPRouteStatus{
				RouteStatus: gatewayapiv1.RouteStatus{
					Parents: []gatewayapiv1.RouteParentStatus{
						{
							ParentRef: gatewayapiv1.ParentReference{
								Name:      gatewayapiv1.ObjectName(kserveGateway.Name),
								Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.GatewayKind)),
								Group:     (*gatewayapiv1.Group)(&gatewayapiv1.GroupVersion.Group),
								Namespace: utils.ToPointer(gatewayapiv1.Namespace(kserveGateway.Namespace)),
							},
							ControllerName: "istio.io/gateway-controller",
							Conditions: []metav1.Condition{
								{
									Type:               string(gatewayapiv1.ListenerConditionAccepted),
									Status:             metav1.ConditionTrue,
									Reason:             "Accepted",
									Message:            "Route was valid",
									LastTransitionTime: metav1.Now(),
								},
							},
						},
					},
				},
			}
			actualPredictorHttpRoute.Status = httpRouteStatus
			Expect(k8sClient.Status().Update(context.Background(), actualPredictorHttpRoute)).NotTo(HaveOccurred())
			actualTransformerHttpRoute.Status = httpRouteStatus
			Expect(k8sClient.Status().Update(context.Background(), actualTransformerHttpRoute)).NotTo(HaveOccurred())
			actualToplevelHttpRoute.Status = httpRouteStatus
			Expect(k8sClient.Status().Update(context.Background(), actualToplevelHttpRoute)).NotTo(HaveOccurred())

			// verify if InferenceService status is updated
			expectedIsvcStatus := v1beta1.InferenceServiceStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{
							Type:   v1beta1.IngressReady,
							Status: "True",
						},
						{
							Type:   v1beta1.PredictorReady,
							Status: "True",
						},
						{
							Type:   apis.ConditionReady,
							Status: "True",
						},
						{
							Type:     v1beta1.TransformerReady,
							Status:   "True",
							Severity: "Info",
						},
					},
				},
				URL: &apis.URL{
					Scheme: "http",
					Host:   fmt.Sprintf("%s-%s.example.com", serviceKey.Name, serviceKey.Namespace),
				},
				Address: &duckv1.Addressable{
					URL: &apis.URL{
						Scheme: "http",
						Host:   fmt.Sprintf("%s.%s.svc.cluster.local", transformerServiceKey.Name, serviceKey.Namespace),
					},
				},
				Components: map[v1beta1.ComponentType]v1beta1.ComponentStatusSpec{
					v1beta1.PredictorComponent: {
						LatestCreatedRevision: "",
						URL: &apis.URL{
							Scheme: "http",
							Host:   fmt.Sprintf("%s-%s.example.com", predictorServiceKey.Name, serviceKey.Namespace),
						},
					},
					v1beta1.TransformerComponent: {
						LatestCreatedRevision: "",
						URL: &apis.URL{
							Scheme: "http",
							Host:   fmt.Sprintf("%s-%s.example.com", transformerServiceKey.Name, serviceKey.Namespace),
						},
					},
				},
				ModelStatus: v1beta1.ModelStatus{
					TransitionStatus:    "InProgress",
					ModelRevisionStates: &v1beta1.ModelRevisionStates{TargetModelState: "Pending"},
				},
			}
			Eventually(func() string {
				isvc := &v1beta1.InferenceService{}
				if err := k8sClient.Get(context.TODO(), serviceKey, isvc); err != nil {
					return err.Error()
				}
				return cmp.Diff(&expectedIsvcStatus, &isvc.Status, cmpopts.IgnoreTypes(apis.VolatileTime{}))
			}, timeout).Should(BeEmpty())

			//check predictor HPA
			var cpuUtilization int32 = 75
			var stabilizationWindowSeconds int32 = 0
			selectPolicy := autoscalingv2.MaxChangePolicySelect
			actualPredictorHPA := &autoscalingv2.HorizontalPodAutoscaler{}
			predictorHPAKey := types.NamespacedName{Name: predictorServiceKey.Name,
				Namespace: serviceKey.Namespace}
			Eventually(func() error { return k8sClient.Get(context.TODO(), predictorHPAKey, actualPredictorHPA) }, timeout).
				Should(Succeed())
			expectedPredictorHPA := &autoscalingv2.HorizontalPodAutoscaler{
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       predictorServiceKey.Name,
					},
					MinReplicas: &minReplicas,
					MaxReplicas: maxReplicas,
					Metrics: []autoscalingv2.MetricSpec{
						{
							Type: autoscalingv2.ResourceMetricSourceType,
							Resource: &autoscalingv2.ResourceMetricSource{
								Name: v1.ResourceCPU,
								Target: autoscalingv2.MetricTarget{
									Type:               "Utilization",
									AverageUtilization: &cpuUtilization,
								},
							},
						},
					},
					Behavior: &autoscalingv2.HorizontalPodAutoscalerBehavior{
						ScaleUp: &autoscalingv2.HPAScalingRules{
							StabilizationWindowSeconds: &stabilizationWindowSeconds,
							SelectPolicy:               &selectPolicy,
							Policies: []autoscalingv2.HPAScalingPolicy{
								{
									Type:          "Pods",
									Value:         4,
									PeriodSeconds: 15,
								},
								{
									Type:          "Percent",
									Value:         100,
									PeriodSeconds: 15,
								},
							},
						},
						ScaleDown: &autoscalingv2.HPAScalingRules{
							StabilizationWindowSeconds: nil,
							SelectPolicy:               &selectPolicy,
							Policies: []autoscalingv2.HPAScalingPolicy{
								{
									Type:          "Percent",
									Value:         100,
									PeriodSeconds: 15,
								},
							},
						},
					},
				},
			}
			Expect(actualPredictorHPA.Spec).To(BeComparableTo(expectedPredictorHPA.Spec))

			// check transformer HPA
			actualTransformerHPA := &autoscalingv2.HorizontalPodAutoscaler{}
			transformerHPAKey := types.NamespacedName{Name: transformerServiceKey.Name,
				Namespace: serviceKey.Namespace}
			Eventually(func() error { return k8sClient.Get(context.TODO(), transformerHPAKey, actualTransformerHPA) }, timeout).
				Should(Succeed())
			expectedTransformerHPA := &autoscalingv2.HorizontalPodAutoscaler{
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       transformerServiceKey.Name,
					},
					MinReplicas: &transformerMinReplicas,
					MaxReplicas: transformerMaxReplicas,
					Metrics: []autoscalingv2.MetricSpec{
						{
							Type: autoscalingv2.ResourceMetricSourceType,
							Resource: &autoscalingv2.ResourceMetricSource{
								Name: v1.ResourceCPU,
								Target: autoscalingv2.MetricTarget{
									Type:               "Utilization",
									AverageUtilization: &transformerCpuUtilization,
								},
							},
						},
					},
					Behavior: &autoscalingv2.HorizontalPodAutoscalerBehavior{
						ScaleUp: &autoscalingv2.HPAScalingRules{
							StabilizationWindowSeconds: &transformerStabilizationWindowSeconds,
							SelectPolicy:               &transformerSelectPolicy,
							Policies: []autoscalingv2.HPAScalingPolicy{
								{
									Type:          "Pods",
									Value:         4,
									PeriodSeconds: 15,
								},
								{
									Type:          "Percent",
									Value:         100,
									PeriodSeconds: 15,
								},
							},
						},
						ScaleDown: &autoscalingv2.HPAScalingRules{
							StabilizationWindowSeconds: nil,
							SelectPolicy:               &transformerSelectPolicy,
							Policies: []autoscalingv2.HPAScalingPolicy{
								{
									Type:          "Percent",
									Value:         100,
									PeriodSeconds: 15,
								},
							},
						},
					},
				},
			}
			Expect(actualTransformerHPA.Spec).To(BeComparableTo(expectedTransformerHPA.Spec))
		})
	})
	Context("When creating inference service with raw kube predictor and explainer", func() {
		configs := map[string]string{
			"explainers": `{
				"art": {
					"image": "kserve/art-explainer",
					"defaultImageVersion": "latest"
				}
			}`,
			"ingress": `{
				"enableGatewayApi": true,
				"kserveIngressGateway": "kserve/kserve-ingress-gateway",
				"ingressGateway": "knative-serving/knative-ingress-gateway",
				"localGateway": "knative-serving/knative-local-gateway",
				"localGatewayService": "knative-local-gateway.istio-system.svc.cluster.local",
				"additionalIngressDomains": ["additional.example.com"]
			}`,
			"storageInitializer": `{
				"image" : "kserve/storage-initializer:latest",
				"memoryRequest": "100Mi",
				"memoryLimit": "1Gi",
				"cpuRequest": "100m",
				"cpuLimit": "1",
				"CaBundleConfigMapName": "",
				"caBundleVolumeMountPath": "/etc/ssl/custom-certs",
				"enableDirectPvcVolumeMount": false
			}`,
		}

		It("Should have httproute/service/deployment/hpa created for explainer and predictor", func() {
			By("By creating a new InferenceService")
			// Create configmap
			var configMap = &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KServeNamespace,
				},
				Data: configs,
			}
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)
			// Create ServingRuntime
			servingRuntime := &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tf-serving-raw",
					Namespace: "default",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{
							Name:       "tensorflow",
							Version:    proto.String("1"),
							AutoSelect: proto.Bool(true),
						},
					},
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []v1.Container{
							{
								Name:    "kserve-container",
								Image:   "tensorflow/serving:1.14.0",
								Command: []string{"/usr/bin/tensorflow_model_server"},
								Args: []string{
									"--port=9000",
									"--rest_api_port=8080",
									"--model_base_path=/mnt/models",
									"--rest_api_timeout_in_ms=60000",
								},
								Resources: defaultResource,
							},
						},
					},
					Disabled: proto.Bool(false),
				},
			}
			k8sClient.Create(context.TODO(), servingRuntime)
			defer k8sClient.Delete(context.TODO(), servingRuntime)
			serviceName := "raw-foo-exp"
			namespace := "default"
			var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: serviceName, Namespace: namespace}}
			var serviceKey = expectedRequest.NamespacedName
			var predictorServiceKey = types.NamespacedName{Name: constants.PredictorServiceName(serviceName),
				Namespace: namespace}
			var explainerServiceKey = types.NamespacedName{Name: constants.ExplainerServiceName(serviceName),
				Namespace: namespace}
			var storageUri = "s3://test/mnist/export"
			var minReplicas int32 = 1
			var maxReplicas int32 = 3
			var explainerMinReplicas int32 = 1
			var explainerMaxReplicas int32 = 2
			var explainerCpuUtilization int32 = 80
			var explainerStabilizationWindowSeconds int32 = 0
			ExplainerSelectPolicy := autoscalingv2.MaxChangePolicySelect
			ctx := context.Background()
			isvc := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceKey.Name,
					Namespace: serviceKey.Namespace,
					Annotations: map[string]string{
						"serving.kserve.io/deploymentMode":              "RawDeployment",
						"serving.kserve.io/autoscalerClass":             "hpa",
						"serving.kserve.io/metrics":                     "cpu",
						"serving.kserve.io/targetUtilizationPercentage": "75",
					},
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
							MinReplicas: utils.ToPointer(int(minReplicas)),
							MaxReplicas: int(maxReplicas),
						},
						Tensorflow: &v1beta1.TFServingSpec{
							PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
								StorageURI:     &storageUri,
								RuntimeVersion: proto.String("1.14.0"),
								Container: v1.Container{
									Name:      constants.InferenceServiceContainerName,
									Resources: defaultResource,
								},
							},
						},
					},
					Explainer: &v1beta1.ExplainerSpec{
						ART: &v1beta1.ARTExplainerSpec{
							Type: v1beta1.ARTSquareAttackExplainer,
							ExplainerExtensionSpec: v1beta1.ExplainerExtensionSpec{
								Config: map[string]string{"nb_classes": "10"},
								Container: v1.Container{
									Name:      constants.InferenceServiceContainerName,
									Resources: defaultResource,
								},
							},
						},
						ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
							MinReplicas: utils.ToPointer(int(explainerMinReplicas)),
							MaxReplicas: int(explainerMaxReplicas),
							ScaleTarget: utils.ToPointer(int(explainerCpuUtilization)),
						},
					},
				},
			}
			isvcConfig, err := v1beta1.NewInferenceServicesConfig(clientset)
			Expect(err).NotTo(HaveOccurred())
			isvc.DefaultInferenceService(isvcConfig, nil, &v1beta1.SecurityConfig{AutoMountServiceAccountToken: false}, nil)
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
			defer k8sClient.Delete(ctx, isvc)

			inferenceService := &v1beta1.InferenceService{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, serviceKey, inferenceService)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			// check predictor deployment
			actualPredictorDeployment := &appsv1.Deployment{}
			predictorDeploymentKey := types.NamespacedName{Name: predictorServiceKey.Name,
				Namespace: serviceKey.Namespace}
			Eventually(func() error { return k8sClient.Get(context.TODO(), predictorDeploymentKey, actualPredictorDeployment) }, timeout).
				Should(Succeed())
			var replicas int32 = 1
			var revisionHistory int32 = 10
			var progressDeadlineSeconds int32 = 600
			var gracePeriod int64 = 30
			expectedPredictorDeployment := &appsv1.Deployment{
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
								constants.StorageInitializerSourceUriInternalAnnotationKey: *isvc.Spec.Predictor.Model.StorageURI,
								"serving.kserve.io/deploymentMode":                         "RawDeployment",
								"serving.kserve.io/autoscalerClass":                        "hpa",
								"serving.kserve.io/metrics":                                "cpu",
								"serving.kserve.io/targetUtilizationPercentage":            "75",
							},
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Image: "tensorflow/serving:" +
										*isvc.Spec.Predictor.Model.RuntimeVersion,
									Name:    constants.InferenceServiceContainerName,
									Command: []string{v1beta1.TensorflowEntrypointCommand},
									Args: []string{
										"--port=" + v1beta1.TensorflowServingGRPCPort,
										"--rest_api_port=" + v1beta1.TensorflowServingRestPort,
										"--model_base_path=" + constants.DefaultModelLocalMountPath,
										"--rest_api_timeout_in_ms=60000",
									},
									Resources: defaultResource,
									ReadinessProbe: &v1.Probe{
										ProbeHandler: v1.ProbeHandler{
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
							AutomountServiceAccountToken: proto.Bool(false),
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
			Expect(actualPredictorDeployment.Spec).To(BeComparableTo(expectedPredictorDeployment.Spec))

			// check Explainer deployment
			actualExplainerDeployment := &appsv1.Deployment{}
			explainerDeploymentKey := types.NamespacedName{Name: explainerServiceKey.Name,
				Namespace: serviceKey.Namespace}
			Eventually(func() error {
				return k8sClient.Get(context.TODO(), explainerDeploymentKey, actualExplainerDeployment)
			}, timeout).
				Should(Succeed())
			expectedExplainerDeployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      explainerDeploymentKey.Name,
					Namespace: explainerDeploymentKey.Namespace,
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: &replicas,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "isvc." + explainerDeploymentKey.Name,
						},
					},
					Template: v1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Name:      explainerDeploymentKey.Name,
							Namespace: "default",
							Labels: map[string]string{
								"app":                                 "isvc." + explainerDeploymentKey.Name,
								constants.KServiceComponentLabel:      constants.Explainer.String(),
								constants.InferenceServicePodLabelKey: serviceName,
							},
							Annotations: map[string]string{
								"serving.kserve.io/deploymentMode":              "RawDeployment",
								"serving.kserve.io/autoscalerClass":             "hpa",
								"serving.kserve.io/metrics":                     "cpu",
								"serving.kserve.io/targetUtilizationPercentage": "75",
							},
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Image: "kserve/art-explainer:latest",
									Name:  constants.InferenceServiceContainerName,
									Args: []string{
										"--model_name",
										serviceKey.Name,
										"--http_port",
										"8080",
										"--predictor_host",
										fmt.Sprintf("%s.%s", predictorServiceKey.Name, predictorServiceKey.Namespace),
										"--adversary_type",
										"SquareAttack",
										"--nb_classes",
										"10",
									},
									Resources: defaultResource,
									ReadinessProbe: &v1.Probe{
										ProbeHandler: v1.ProbeHandler{
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
							AutomountServiceAccountToken: proto.Bool(false),
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
			Expect(actualExplainerDeployment.Spec).To(BeComparableTo(expectedExplainerDeployment.Spec))

			//check predictor service
			actualPredictorService := &v1.Service{}
			Eventually(func() error { return k8sClient.Get(context.TODO(), predictorServiceKey, actualPredictorService) }, timeout).
				Should(Succeed())
			expectedPredictorService := &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      predictorServiceKey.Name,
					Namespace: predictorServiceKey.Namespace,
				},
				Spec: v1.ServiceSpec{
					Ports: []v1.ServicePort{
						{
							Name:       predictorServiceKey.Name,
							Protocol:   "TCP",
							Port:       80,
							TargetPort: intstr.IntOrString{Type: 0, IntVal: 8080, StrVal: ""},
						},
					},
					Type:            "ClusterIP",
					SessionAffinity: "None",
					Selector: map[string]string{
						"app": fmt.Sprintf("isvc.%s", predictorServiceKey.Name),
					},
				},
			}
			actualPredictorService.Spec.ClusterIP = ""
			actualPredictorService.Spec.ClusterIPs = nil
			actualPredictorService.Spec.IPFamilies = nil
			actualPredictorService.Spec.IPFamilyPolicy = nil
			actualPredictorService.Spec.InternalTrafficPolicy = nil
			Expect(actualPredictorService.Spec).To(BeComparableTo(expectedPredictorService.Spec))

			//check Explainer service
			actualExplainerService := &v1.Service{}
			Eventually(func() error { return k8sClient.Get(context.TODO(), explainerServiceKey, actualExplainerService) }, timeout).
				Should(Succeed())
			expectedExplainerService := &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      explainerServiceKey.Name,
					Namespace: explainerServiceKey.Namespace,
				},
				Spec: v1.ServiceSpec{
					Ports: []v1.ServicePort{
						{
							Name:       explainerServiceKey.Name,
							Protocol:   "TCP",
							Port:       80,
							TargetPort: intstr.IntOrString{Type: 0, IntVal: 8080, StrVal: ""},
						},
					},
					Type:            "ClusterIP",
					SessionAffinity: "None",
					Selector: map[string]string{
						"app": fmt.Sprintf("isvc.%s", explainerServiceKey.Name),
					},
				},
			}
			actualExplainerService.Spec.ClusterIP = ""
			actualExplainerService.Spec.ClusterIPs = nil
			actualExplainerService.Spec.IPFamilies = nil
			actualExplainerService.Spec.IPFamilyPolicy = nil
			actualExplainerService.Spec.InternalTrafficPolicy = nil
			Expect(actualExplainerService.Spec).To(BeComparableTo(expectedExplainerService.Spec))

			// update deployment status to make isvc ready
			updatedPredictorDeployment := actualPredictorDeployment.DeepCopy()
			updatedPredictorDeployment.Status.Conditions = []appsv1.DeploymentCondition{
				{
					Type:   appsv1.DeploymentAvailable,
					Status: v1.ConditionTrue,
				},
			}
			Expect(k8sClient.Status().Update(context.TODO(), updatedPredictorDeployment)).NotTo(HaveOccurred())
			updatedExplainerDeployment := actualExplainerDeployment.DeepCopy()
			updatedExplainerDeployment.Status.Conditions = []appsv1.DeploymentCondition{
				{
					Type:   appsv1.DeploymentAvailable,
					Status: v1.ConditionTrue,
				},
			}
			Expect(k8sClient.Status().Update(context.TODO(), updatedExplainerDeployment)).NotTo(HaveOccurred())

			//check http route
			actualToplevelHttpRoute := &gatewayapiv1.HTTPRoute{}
			Eventually(func() error {
				return k8sClient.Get(context.TODO(), types.NamespacedName{Name: serviceKey.Name,
					Namespace: serviceKey.Namespace}, actualToplevelHttpRoute)
			}, timeout).
				Should(Succeed())
			topLevelHost := fmt.Sprintf("%s-%s.%s", serviceKey.Name, serviceKey.Namespace, "example.com")
			expectedToplevelHttpRoute := gatewayapiv1.HTTPRoute{
				Spec: gatewayapiv1.HTTPRouteSpec{
					Hostnames: []gatewayapiv1.Hostname{gatewayapiv1.Hostname(topLevelHost), "additional.example.com"},
					Rules: []gatewayapiv1.HTTPRouteRule{
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer(constants.ExplainPrefix()),
									},
								},
							},
							Filters: []gatewayapiv1.HTTPRouteFilter{
								{
									Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
										Set: []gatewayapiv1.HTTPHeader{
											{
												Name:  constants.IsvcNameHeader,
												Value: serviceKey.Name,
											},
											{
												Name:  constants.IsvcNamespaceHeader,
												Value: serviceKey.Namespace,
											},
										},
									},
								},
							},
							BackendRefs: []gatewayapiv1.HTTPBackendRef{
								{
									BackendRef: gatewayapiv1.BackendRef{
										BackendObjectReference: gatewayapiv1.BackendObjectReference{
											Group:     (*gatewayapiv1.Group)(utils.ToPointer("")),
											Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      gatewayapiv1.ObjectName(explainerServiceKey.Name),
											Namespace: (*gatewayapiv1.Namespace)(utils.ToPointer(serviceKey.Namespace)),
											Port:      (*gatewayapiv1.PortNumber)(utils.ToPointer(int32(constants.CommonDefaultHttpPort))),
										},
										Weight: utils.ToPointer(int32(1)),
									},
								},
							},
							Timeouts: &gatewayapiv1.HTTPRouteTimeouts{
								Request: utils.ToPointer(gatewayapiv1.Duration("0s")),
							},
						},
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer(constants.FallbackPrefix()),
									},
								},
							},
							Filters: []gatewayapiv1.HTTPRouteFilter{
								{
									Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
										Set: []gatewayapiv1.HTTPHeader{
											{
												Name:  constants.IsvcNameHeader,
												Value: serviceKey.Name,
											},
											{
												Name:  constants.IsvcNamespaceHeader,
												Value: serviceKey.Namespace,
											},
										},
									},
								},
							},
							BackendRefs: []gatewayapiv1.HTTPBackendRef{
								{
									BackendRef: gatewayapiv1.BackendRef{
										BackendObjectReference: gatewayapiv1.BackendObjectReference{
											Group:     (*gatewayapiv1.Group)(utils.ToPointer("")),
											Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      gatewayapiv1.ObjectName(predictorServiceKey.Name),
											Namespace: (*gatewayapiv1.Namespace)(utils.ToPointer(serviceKey.Namespace)),
											Port:      (*gatewayapiv1.PortNumber)(utils.ToPointer(int32(constants.CommonDefaultHttpPort))),
										},
										Weight: utils.ToPointer(int32(1)),
									},
								},
							},
							Timeouts: &gatewayapiv1.HTTPRouteTimeouts{
								Request: utils.ToPointer(gatewayapiv1.Duration("0s")),
							},
						},
					},
					CommonRouteSpec: gatewayapiv1.CommonRouteSpec{
						ParentRefs: []gatewayapiv1.ParentReference{
							{
								Name:      gatewayapiv1.ObjectName(kserveGateway.Name),
								Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.GatewayKind)),
								Group:     (*gatewayapiv1.Group)(&gatewayapiv1.GroupVersion.Group),
								Namespace: utils.ToPointer(gatewayapiv1.Namespace(kserveGateway.Namespace)),
							},
						},
					},
				},
			}
			Expect(actualToplevelHttpRoute.Spec).To(BeComparableTo(expectedToplevelHttpRoute.Spec))

			actualPredictorHttpRoute := &gatewayapiv1.HTTPRoute{}
			Eventually(func() error {
				return k8sClient.Get(context.TODO(), types.NamespacedName{Name: predictorServiceKey.Name,
					Namespace: serviceKey.Namespace}, actualPredictorHttpRoute)
			}, timeout).
				Should(Succeed())
			predictorHost := fmt.Sprintf("%s-%s.%s", predictorServiceKey.Name, serviceKey.Namespace, "example.com")
			expectedPredictorHttpRoute := gatewayapiv1.HTTPRoute{
				Spec: gatewayapiv1.HTTPRouteSpec{
					Hostnames: []gatewayapiv1.Hostname{gatewayapiv1.Hostname(predictorHost)},
					Rules: []gatewayapiv1.HTTPRouteRule{
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer(constants.FallbackPrefix()),
									},
								},
							},
							Filters: []gatewayapiv1.HTTPRouteFilter{
								{
									Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
										Set: []gatewayapiv1.HTTPHeader{
											{
												Name:  constants.IsvcNameHeader,
												Value: serviceKey.Name,
											},
											{
												Name:  constants.IsvcNamespaceHeader,
												Value: serviceKey.Namespace,
											},
										},
									},
								},
							},
							BackendRefs: []gatewayapiv1.HTTPBackendRef{
								{
									BackendRef: gatewayapiv1.BackendRef{
										BackendObjectReference: gatewayapiv1.BackendObjectReference{
											Group:     (*gatewayapiv1.Group)(utils.ToPointer("")),
											Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      gatewayapiv1.ObjectName(predictorServiceKey.Name),
											Namespace: (*gatewayapiv1.Namespace)(utils.ToPointer(serviceKey.Namespace)),
											Port:      (*gatewayapiv1.PortNumber)(utils.ToPointer(int32(constants.CommonDefaultHttpPort))),
										},
										Weight: utils.ToPointer(int32(1)),
									},
								},
							},
							Timeouts: &gatewayapiv1.HTTPRouteTimeouts{
								Request: utils.ToPointer(gatewayapiv1.Duration("0s")),
							},
						},
					},
					CommonRouteSpec: gatewayapiv1.CommonRouteSpec{
						ParentRefs: []gatewayapiv1.ParentReference{
							{
								Name:      gatewayapiv1.ObjectName(kserveGateway.Name),
								Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.GatewayKind)),
								Group:     (*gatewayapiv1.Group)(&gatewayapiv1.GroupVersion.Group),
								Namespace: utils.ToPointer(gatewayapiv1.Namespace(kserveGateway.Namespace)),
							},
						},
					},
				},
			}
			Expect(actualPredictorHttpRoute.Spec).To(BeComparableTo(expectedPredictorHttpRoute.Spec))

			actualExplainerHttpRoute := &gatewayapiv1.HTTPRoute{}
			Eventually(func() error {
				return k8sClient.Get(context.TODO(), types.NamespacedName{Name: explainerServiceKey.Name,
					Namespace: serviceKey.Namespace}, actualExplainerHttpRoute)
			}, timeout).
				Should(Succeed())
			explainerHost := fmt.Sprintf("%s-%s.%s", explainerServiceKey.Name, serviceKey.Namespace, "example.com")
			expectedExplainerHttpRoute := gatewayapiv1.HTTPRoute{
				Spec: gatewayapiv1.HTTPRouteSpec{
					Hostnames: []gatewayapiv1.Hostname{gatewayapiv1.Hostname(explainerHost)},
					Rules: []gatewayapiv1.HTTPRouteRule{
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer(constants.FallbackPrefix()),
									},
								},
							},
							Filters: []gatewayapiv1.HTTPRouteFilter{
								{
									Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
										Set: []gatewayapiv1.HTTPHeader{
											{
												Name:  constants.IsvcNameHeader,
												Value: serviceKey.Name,
											},
											{
												Name:  constants.IsvcNamespaceHeader,
												Value: serviceKey.Namespace,
											},
										},
									},
								},
							},
							BackendRefs: []gatewayapiv1.HTTPBackendRef{
								{
									BackendRef: gatewayapiv1.BackendRef{
										BackendObjectReference: gatewayapiv1.BackendObjectReference{
											Group:     (*gatewayapiv1.Group)(utils.ToPointer("")),
											Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      gatewayapiv1.ObjectName(explainerServiceKey.Name),
											Namespace: (*gatewayapiv1.Namespace)(utils.ToPointer(serviceKey.Namespace)),
											Port:      (*gatewayapiv1.PortNumber)(utils.ToPointer(int32(constants.CommonDefaultHttpPort))),
										},
										Weight: utils.ToPointer(int32(1)),
									},
								},
							},
							Timeouts: &gatewayapiv1.HTTPRouteTimeouts{
								Request: utils.ToPointer(gatewayapiv1.Duration("0s")),
							},
						},
					},
					CommonRouteSpec: gatewayapiv1.CommonRouteSpec{
						ParentRefs: []gatewayapiv1.ParentReference{
							{
								Name:      gatewayapiv1.ObjectName(kserveGateway.Name),
								Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.GatewayKind)),
								Group:     (*gatewayapiv1.Group)(&gatewayapiv1.GroupVersion.Group),
								Namespace: utils.ToPointer(gatewayapiv1.Namespace(kserveGateway.Namespace)),
							},
						},
					},
				},
			}
			Expect(actualExplainerHttpRoute.Spec).To(BeComparableTo(expectedExplainerHttpRoute.Spec))

			// Mark the Ingress as accepted to make isvc ready
			httpRouteStatus := gatewayapiv1.HTTPRouteStatus{
				RouteStatus: gatewayapiv1.RouteStatus{
					Parents: []gatewayapiv1.RouteParentStatus{
						{
							ParentRef: gatewayapiv1.ParentReference{
								Name:      gatewayapiv1.ObjectName(kserveGateway.Name),
								Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.GatewayKind)),
								Group:     (*gatewayapiv1.Group)(&gatewayapiv1.GroupVersion.Group),
								Namespace: utils.ToPointer(gatewayapiv1.Namespace(kserveGateway.Namespace)),
							},
							ControllerName: "istio.io/gateway-controller",
							Conditions: []metav1.Condition{
								{
									Type:               string(gatewayapiv1.ListenerConditionAccepted),
									Status:             metav1.ConditionTrue,
									Reason:             "Accepted",
									Message:            "Route was valid",
									LastTransitionTime: metav1.Now(),
								},
							},
						},
					},
				},
			}
			actualPredictorHttpRoute.Status = httpRouteStatus
			Expect(k8sClient.Status().Update(context.Background(), actualPredictorHttpRoute)).NotTo(HaveOccurred())
			actualExplainerHttpRoute.Status = httpRouteStatus
			Expect(k8sClient.Status().Update(context.Background(), actualExplainerHttpRoute)).NotTo(HaveOccurred())
			actualToplevelHttpRoute.Status = httpRouteStatus
			Expect(k8sClient.Status().Update(context.Background(), actualToplevelHttpRoute)).NotTo(HaveOccurred())

			// verify if InferenceService status is updated
			expectedIsvcStatus := v1beta1.InferenceServiceStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{
							Type:     v1beta1.ExplainerReady,
							Status:   "True",
							Severity: "Info",
						},
						{
							Type:   v1beta1.IngressReady,
							Status: "True",
						},
						{
							Type:   v1beta1.PredictorReady,
							Status: "True",
						},
						{
							Type:   apis.ConditionReady,
							Status: "True",
						},
					},
				},
				URL: &apis.URL{
					Scheme: "http",
					Host:   fmt.Sprintf("%s-%s.example.com", serviceKey.Name, serviceKey.Namespace),
				},
				Address: &duckv1.Addressable{
					URL: &apis.URL{
						Scheme: "http",
						Host:   fmt.Sprintf("%s.%s.svc.cluster.local", predictorServiceKey.Name, serviceKey.Namespace),
					},
				},
				Components: map[v1beta1.ComponentType]v1beta1.ComponentStatusSpec{
					v1beta1.PredictorComponent: {
						LatestCreatedRevision: "",
						URL: &apis.URL{
							Scheme: "http",
							Host:   fmt.Sprintf("%s-%s.example.com", predictorServiceKey.Name, serviceKey.Namespace),
						},
					},
					v1beta1.ExplainerComponent: {
						LatestCreatedRevision: "",
						URL: &apis.URL{
							Scheme: "http",
							Host:   fmt.Sprintf("%s-%s.example.com", explainerServiceKey.Name, serviceKey.Namespace),
						},
					},
				},
				ModelStatus: v1beta1.ModelStatus{
					TransitionStatus:    "InProgress",
					ModelRevisionStates: &v1beta1.ModelRevisionStates{TargetModelState: "Pending"},
				},
			}
			Eventually(func() string {
				isvc := &v1beta1.InferenceService{}
				if err := k8sClient.Get(context.TODO(), serviceKey, isvc); err != nil {
					return err.Error()
				}
				return cmp.Diff(&expectedIsvcStatus, &isvc.Status, cmpopts.IgnoreTypes(apis.VolatileTime{}))
			}, timeout).Should(BeEmpty())

			//check predictor HPA
			var cpuUtilization int32 = 75
			var stabilizationWindowSeconds int32 = 0
			selectPolicy := autoscalingv2.MaxChangePolicySelect
			actualPredictorHPA := &autoscalingv2.HorizontalPodAutoscaler{}
			predictorHPAKey := types.NamespacedName{Name: predictorServiceKey.Name,
				Namespace: serviceKey.Namespace}
			Eventually(func() error { return k8sClient.Get(context.TODO(), predictorHPAKey, actualPredictorHPA) }, timeout).
				Should(Succeed())
			expectedPredictorHPA := &autoscalingv2.HorizontalPodAutoscaler{
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       predictorServiceKey.Name,
					},
					MinReplicas: &minReplicas,
					MaxReplicas: maxReplicas,
					Metrics: []autoscalingv2.MetricSpec{
						{
							Type: autoscalingv2.ResourceMetricSourceType,
							Resource: &autoscalingv2.ResourceMetricSource{
								Name: v1.ResourceCPU,
								Target: autoscalingv2.MetricTarget{
									Type:               "Utilization",
									AverageUtilization: &cpuUtilization,
								},
							},
						},
					},
					Behavior: &autoscalingv2.HorizontalPodAutoscalerBehavior{
						ScaleUp: &autoscalingv2.HPAScalingRules{
							StabilizationWindowSeconds: &stabilizationWindowSeconds,
							SelectPolicy:               &selectPolicy,
							Policies: []autoscalingv2.HPAScalingPolicy{
								{
									Type:          "Pods",
									Value:         4,
									PeriodSeconds: 15,
								},
								{
									Type:          "Percent",
									Value:         100,
									PeriodSeconds: 15,
								},
							},
						},
						ScaleDown: &autoscalingv2.HPAScalingRules{
							StabilizationWindowSeconds: nil,
							SelectPolicy:               &selectPolicy,
							Policies: []autoscalingv2.HPAScalingPolicy{
								{
									Type:          "Percent",
									Value:         100,
									PeriodSeconds: 15,
								},
							},
						},
					},
				},
			}
			Expect(actualPredictorHPA.Spec).To(BeComparableTo(expectedPredictorHPA.Spec))

			// check Explainer HPA
			actualExplainerHPA := &autoscalingv2.HorizontalPodAutoscaler{}
			explainerHPAKey := types.NamespacedName{Name: explainerServiceKey.Name,
				Namespace: serviceKey.Namespace}
			Eventually(func() error { return k8sClient.Get(context.TODO(), explainerHPAKey, actualExplainerHPA) }, timeout).
				Should(Succeed())
			expectedExplainerHPA := &autoscalingv2.HorizontalPodAutoscaler{
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       explainerServiceKey.Name,
					},
					MinReplicas: &explainerMinReplicas,
					MaxReplicas: explainerMaxReplicas,
					Metrics: []autoscalingv2.MetricSpec{
						{
							Type: autoscalingv2.ResourceMetricSourceType,
							Resource: &autoscalingv2.ResourceMetricSource{
								Name: v1.ResourceCPU,
								Target: autoscalingv2.MetricTarget{
									Type:               "Utilization",
									AverageUtilization: &explainerCpuUtilization,
								},
							},
						},
					},
					Behavior: &autoscalingv2.HorizontalPodAutoscalerBehavior{
						ScaleUp: &autoscalingv2.HPAScalingRules{
							StabilizationWindowSeconds: &explainerStabilizationWindowSeconds,
							SelectPolicy:               &ExplainerSelectPolicy,
							Policies: []autoscalingv2.HPAScalingPolicy{
								{
									Type:          "Pods",
									Value:         4,
									PeriodSeconds: 15,
								},
								{
									Type:          "Percent",
									Value:         100,
									PeriodSeconds: 15,
								},
							},
						},
						ScaleDown: &autoscalingv2.HPAScalingRules{
							StabilizationWindowSeconds: nil,
							SelectPolicy:               &ExplainerSelectPolicy,
							Policies: []autoscalingv2.HPAScalingPolicy{
								{
									Type:          "Percent",
									Value:         100,
									PeriodSeconds: 15,
								},
							},
						},
					},
				},
			}
			Expect(actualExplainerHPA.Spec).To(BeComparableTo(expectedExplainerHPA.Spec))
		})
	})
	Context("When creating inference service with raw kube path based routing predictor", func() {
		configs := map[string]string{
			"explainers": `{
				"alibi": {
					"image": "kserve/alibi-explainer",
					"defaultImageVersion": "latest"
				}
			}`,
			"ingress": `{
				"enableGatewayApi": true,
				"kserveIngressGateway": "kserve/kserve-ingress-gateway",
				"ingressGateway": "knative-serving/knative-ingress-gateway",
				"localGateway": "knative-serving/knative-local-gateway",
				"localGatewayService": "knative-local-gateway.istio-system.svc.cluster.local",
				"additionalIngressDomains": ["additional.example.com"],
				"ingressDomain": "example.com",
				"pathTemplate": "/serving/{{ .Namespace }}/{{ .Name }}"
			}`,
			"storageInitializer": `{
				"image" : "kserve/storage-initializer:latest",
				"memoryRequest": "100Mi",
				"memoryLimit": "1Gi",
				"cpuRequest": "100m",
				"cpuLimit": "1",
				"CaBundleConfigMapName": "",
				"caBundleVolumeMountPath": "/etc/ssl/custom-certs",
				"enableDirectPvcVolumeMount": false
			}`,
		}

		It("Should have httproute/service/deployment/hpa created", func() {
			By("By creating a new InferenceService")
			// Create configmap
			var configMap = &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KServeNamespace,
				},
				Data: configs,
			}
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)
			// Create ServingRuntime
			servingRuntime := &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tf-serving-raw",
					Namespace: "default",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{
							Name:       "tensorflow",
							Version:    proto.String("1"),
							AutoSelect: proto.Bool(true),
						},
					},
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []v1.Container{
							{
								Name:    "kserve-container",
								Image:   "tensorflow/serving:1.14.0",
								Command: []string{"/usr/bin/tensorflow_model_server"},
								Args: []string{
									"--port=9000",
									"--rest_api_port=8080",
									"--model_base_path=/mnt/models",
									"--rest_api_timeout_in_ms=60000",
								},
								Resources: defaultResource,
							},
						},
					},
					Disabled: proto.Bool(false),
				},
			}
			k8sClient.Create(context.TODO(), servingRuntime)
			defer k8sClient.Delete(context.TODO(), servingRuntime)
			serviceName := "raw-foo-path"
			var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: serviceName, Namespace: "default"}}
			var serviceKey = expectedRequest.NamespacedName
			var storageUri = "s3://test/mnist/export"
			ctx := context.Background()
			isvc := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceKey.Name,
					Namespace: serviceKey.Namespace,
					Annotations: map[string]string{
						"serving.kserve.io/deploymentMode":              "RawDeployment",
						"serving.kserve.io/autoscalerClass":             "hpa",
						"serving.kserve.io/metrics":                     "cpu",
						"serving.kserve.io/targetUtilizationPercentage": "75",
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
									Name:      constants.InferenceServiceContainerName,
									Resources: defaultResource,
								},
							},
						},
					},
				},
			}
			isvc.DefaultInferenceService(nil, nil, &v1beta1.SecurityConfig{AutoMountServiceAccountToken: false}, nil)
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
			defer k8sClient.Delete(ctx, isvc)

			inferenceService := &v1beta1.InferenceService{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, serviceKey, inferenceService)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			actualDeployment := &appsv1.Deployment{}
			predictorDeploymentKey := types.NamespacedName{Name: constants.PredictorServiceName(serviceKey.Name),
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
								constants.StorageInitializerSourceUriInternalAnnotationKey: *isvc.Spec.Predictor.Model.StorageURI,
								"serving.kserve.io/deploymentMode":                         "RawDeployment",
								"serving.kserve.io/autoscalerClass":                        "hpa",
								"serving.kserve.io/metrics":                                "cpu",
								"serving.kserve.io/targetUtilizationPercentage":            "75",
							},
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Image: "tensorflow/serving:" +
										*isvc.Spec.Predictor.Model.RuntimeVersion,
									Name:    constants.InferenceServiceContainerName,
									Command: []string{v1beta1.TensorflowEntrypointCommand},
									Args: []string{
										"--port=" + v1beta1.TensorflowServingGRPCPort,
										"--rest_api_port=" + v1beta1.TensorflowServingRestPort,
										"--model_base_path=" + constants.DefaultModelLocalMountPath,
										"--rest_api_timeout_in_ms=60000",
									},
									Resources: defaultResource,
									ReadinessProbe: &v1.Probe{
										ProbeHandler: v1.ProbeHandler{
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
							AutomountServiceAccountToken: proto.Bool(false),
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
			Expect(actualDeployment.Spec).To(BeComparableTo(expectedDeployment.Spec))

			//check service
			actualService := &v1.Service{}
			predictorServiceKey := types.NamespacedName{Name: constants.PredictorServiceName(serviceKey.Name),
				Namespace: serviceKey.Namespace}
			Eventually(func() error { return k8sClient.Get(context.TODO(), predictorServiceKey, actualService) }, timeout).
				Should(Succeed())

			expectedService := &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      predictorServiceKey.Name,
					Namespace: predictorServiceKey.Namespace,
				},
				Spec: v1.ServiceSpec{
					Ports: []v1.ServicePort{
						{
							Name:       constants.PredictorServiceName(serviceName),
							Protocol:   "TCP",
							Port:       80,
							TargetPort: intstr.IntOrString{Type: 0, IntVal: 8080, StrVal: ""},
						},
					},
					Type:            "ClusterIP",
					SessionAffinity: "None",
					Selector: map[string]string{
						"app": fmt.Sprintf("isvc.%s", constants.PredictorServiceName(serviceName)),
					},
				},
			}
			actualService.Spec.ClusterIP = ""
			actualService.Spec.ClusterIPs = nil
			actualService.Spec.IPFamilies = nil
			actualService.Spec.IPFamilyPolicy = nil
			actualService.Spec.InternalTrafficPolicy = nil
			Expect(actualService.Spec).To(BeComparableTo(expectedService.Spec))

			//check isvc status
			updatedDeployment := actualDeployment.DeepCopy()
			updatedDeployment.Status.Conditions = []appsv1.DeploymentCondition{
				{
					Type:   appsv1.DeploymentAvailable,
					Status: v1.ConditionTrue,
				},
			}
			Expect(k8sClient.Status().Update(context.TODO(), updatedDeployment)).NotTo(HaveOccurred())

			//check http route
			actualToplevelHttpRoute := &gatewayapiv1.HTTPRoute{}
			Eventually(func() error {
				return k8sClient.Get(context.TODO(), types.NamespacedName{Name: serviceKey.Name,
					Namespace: serviceKey.Namespace}, actualToplevelHttpRoute)
			}, timeout).
				Should(Succeed())
			topLevelHost := fmt.Sprintf("%s-%s.%s", serviceKey.Name, serviceKey.Namespace, "example.com")
			prefixUrlPath := fmt.Sprintf("/serving/%s/%s", serviceKey.Namespace, serviceKey.Name)
			expectedToplevelHttpRoute := gatewayapiv1.HTTPRoute{
				Spec: gatewayapiv1.HTTPRouteSpec{
					Hostnames: []gatewayapiv1.Hostname{gatewayapiv1.Hostname(topLevelHost), "additional.example.com", "example.com"},
					Rules: []gatewayapiv1.HTTPRouteRule{
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer(constants.FallbackPrefix()),
									},
								},
							},
							Filters: []gatewayapiv1.HTTPRouteFilter{
								{
									Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
										Set: []gatewayapiv1.HTTPHeader{
											{
												Name:  constants.IsvcNameHeader,
												Value: serviceKey.Name,
											},
											{
												Name:  constants.IsvcNamespaceHeader,
												Value: serviceKey.Namespace,
											},
										},
									},
								},
							},
							BackendRefs: []gatewayapiv1.HTTPBackendRef{
								{
									BackendRef: gatewayapiv1.BackendRef{
										BackendObjectReference: gatewayapiv1.BackendObjectReference{
											Group:     (*gatewayapiv1.Group)(utils.ToPointer("")),
											Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      gatewayapiv1.ObjectName(predictorServiceKey.Name),
											Namespace: (*gatewayapiv1.Namespace)(utils.ToPointer(serviceKey.Namespace)),
											Port:      (*gatewayapiv1.PortNumber)(utils.ToPointer(int32(constants.CommonDefaultHttpPort))),
										},
										Weight: utils.ToPointer(int32(1)),
									},
								},
							},
							Timeouts: &gatewayapiv1.HTTPRouteTimeouts{
								Request: utils.ToPointer(gatewayapiv1.Duration("0s")),
							},
						},
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer(prefixUrlPath + "/"),
									},
								},
							},
							Filters: []gatewayapiv1.HTTPRouteFilter{
								{
									Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
										Set: []gatewayapiv1.HTTPHeader{
											{
												Name:  constants.IsvcNameHeader,
												Value: serviceKey.Name,
											},
											{
												Name:  constants.IsvcNamespaceHeader,
												Value: serviceKey.Namespace,
											},
										},
									},
								},
							},
							BackendRefs: []gatewayapiv1.HTTPBackendRef{
								{
									BackendRef: gatewayapiv1.BackendRef{
										BackendObjectReference: gatewayapiv1.BackendObjectReference{
											Group:     (*gatewayapiv1.Group)(utils.ToPointer("")),
											Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      gatewayapiv1.ObjectName(predictorServiceKey.Name),
											Namespace: (*gatewayapiv1.Namespace)(utils.ToPointer(serviceKey.Namespace)),
											Port:      (*gatewayapiv1.PortNumber)(utils.ToPointer(int32(constants.CommonDefaultHttpPort))),
										},
										Weight: utils.ToPointer(int32(1)),
									},
								},
							},
							Timeouts: &gatewayapiv1.HTTPRouteTimeouts{
								Request: utils.ToPointer(gatewayapiv1.Duration("0s")),
							},
						},
					},
					CommonRouteSpec: gatewayapiv1.CommonRouteSpec{
						ParentRefs: []gatewayapiv1.ParentReference{
							{
								Name:      gatewayapiv1.ObjectName(kserveGateway.Name),
								Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.GatewayKind)),
								Group:     (*gatewayapiv1.Group)(&gatewayapiv1.GroupVersion.Group),
								Namespace: utils.ToPointer(gatewayapiv1.Namespace(kserveGateway.Namespace)),
							},
						},
					},
				},
			}
			Expect(actualToplevelHttpRoute.Spec).To(BeComparableTo(expectedToplevelHttpRoute.Spec))

			actualPredictorHttpRoute := &gatewayapiv1.HTTPRoute{}
			Eventually(func() error {
				return k8sClient.Get(context.TODO(), types.NamespacedName{Name: predictorServiceKey.Name,
					Namespace: serviceKey.Namespace}, actualPredictorHttpRoute)
			}, timeout).
				Should(Succeed())
			predictorHost := fmt.Sprintf("%s-%s.%s", predictorServiceKey.Name, serviceKey.Namespace, "example.com")
			expectedPredictorHttpRoute := gatewayapiv1.HTTPRoute{
				Spec: gatewayapiv1.HTTPRouteSpec{
					Hostnames: []gatewayapiv1.Hostname{gatewayapiv1.Hostname(predictorHost)},
					Rules: []gatewayapiv1.HTTPRouteRule{
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer(constants.FallbackPrefix()),
									},
								},
							},
							Filters: []gatewayapiv1.HTTPRouteFilter{
								{
									Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
										Set: []gatewayapiv1.HTTPHeader{
											{
												Name:  constants.IsvcNameHeader,
												Value: serviceKey.Name,
											},
											{
												Name:  constants.IsvcNamespaceHeader,
												Value: serviceKey.Namespace,
											},
										},
									},
								},
							},
							BackendRefs: []gatewayapiv1.HTTPBackendRef{
								{
									BackendRef: gatewayapiv1.BackendRef{
										BackendObjectReference: gatewayapiv1.BackendObjectReference{
											Group:     (*gatewayapiv1.Group)(utils.ToPointer("")),
											Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      gatewayapiv1.ObjectName(predictorServiceKey.Name),
											Namespace: (*gatewayapiv1.Namespace)(utils.ToPointer(serviceKey.Namespace)),
											Port:      (*gatewayapiv1.PortNumber)(utils.ToPointer(int32(constants.CommonDefaultHttpPort))),
										},
										Weight: utils.ToPointer(int32(1)),
									},
								},
							},
							Timeouts: &gatewayapiv1.HTTPRouteTimeouts{
								Request: utils.ToPointer(gatewayapiv1.Duration("0s")),
							},
						},
					},
					CommonRouteSpec: gatewayapiv1.CommonRouteSpec{
						ParentRefs: []gatewayapiv1.ParentReference{
							{
								Name:      gatewayapiv1.ObjectName(kserveGateway.Name),
								Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.GatewayKind)),
								Group:     (*gatewayapiv1.Group)(&gatewayapiv1.GroupVersion.Group),
								Namespace: utils.ToPointer(gatewayapiv1.Namespace(kserveGateway.Namespace)),
							},
						},
					},
				},
			}
			Expect(actualPredictorHttpRoute.Spec).To(BeComparableTo(expectedPredictorHttpRoute.Spec))

			// Mark the Ingress as accepted to make isvc ready
			httpRouteStatus := gatewayapiv1.HTTPRouteStatus{
				RouteStatus: gatewayapiv1.RouteStatus{
					Parents: []gatewayapiv1.RouteParentStatus{
						{
							ParentRef: gatewayapiv1.ParentReference{
								Name:      gatewayapiv1.ObjectName(kserveGateway.Name),
								Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.GatewayKind)),
								Group:     (*gatewayapiv1.Group)(&gatewayapiv1.GroupVersion.Group),
								Namespace: utils.ToPointer(gatewayapiv1.Namespace(kserveGateway.Namespace)),
							},
							ControllerName: "istio.io/gateway-controller",
							Conditions: []metav1.Condition{
								{
									Type:               string(gatewayapiv1.ListenerConditionAccepted),
									Status:             metav1.ConditionTrue,
									Reason:             "Accepted",
									Message:            "Route was valid",
									LastTransitionTime: metav1.Now(),
								},
							},
						},
					},
				},
			}
			actualPredictorHttpRoute.Status = httpRouteStatus
			Expect(k8sClient.Status().Update(context.Background(), actualPredictorHttpRoute)).NotTo(HaveOccurred())
			actualToplevelHttpRoute.Status = httpRouteStatus
			Expect(k8sClient.Status().Update(context.Background(), actualToplevelHttpRoute)).NotTo(HaveOccurred())

			// verify if InferenceService status is updated
			expectedIsvcStatus := v1beta1.InferenceServiceStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{
							Type:   v1beta1.IngressReady,
							Status: "True",
						},
						{
							Type:   v1beta1.PredictorReady,
							Status: "True",
						},
						{
							Type:   apis.ConditionReady,
							Status: "True",
						},
					},
				},
				URL: &apis.URL{
					Scheme: "http",
					Host:   fmt.Sprintf("%s-%s.example.com", serviceKey.Name, serviceKey.Namespace),
				},
				Address: &duckv1.Addressable{
					URL: &apis.URL{
						Scheme: "http",
						Host:   fmt.Sprintf("%s-predictor.%s.svc.cluster.local", serviceKey.Name, serviceKey.Namespace),
					},
				},
				Components: map[v1beta1.ComponentType]v1beta1.ComponentStatusSpec{
					v1beta1.PredictorComponent: {
						LatestCreatedRevision: "",
						URL: &apis.URL{
							Scheme: "http",
							Host:   fmt.Sprintf("%s-%s.example.com", predictorServiceKey.Name, serviceKey.Namespace),
						},
					},
				},
				ModelStatus: v1beta1.ModelStatus{
					TransitionStatus:    "InProgress",
					ModelRevisionStates: &v1beta1.ModelRevisionStates{TargetModelState: "Pending"},
				},
			}
			Eventually(func() string {
				isvc := &v1beta1.InferenceService{}
				if err := k8sClient.Get(context.TODO(), serviceKey, isvc); err != nil {
					return err.Error()
				}
				return cmp.Diff(&expectedIsvcStatus, &isvc.Status, cmpopts.IgnoreTypes(apis.VolatileTime{}))
			}, timeout).Should(BeEmpty())

			//check HPA
			var minReplicas int32 = 1
			var maxReplicas int32 = 3
			var cpuUtilization int32 = 75
			var stabilizationWindowSeconds int32 = 0
			selectPolicy := autoscalingv2.MaxChangePolicySelect
			actualHPA := &autoscalingv2.HorizontalPodAutoscaler{}
			predictorHPAKey := types.NamespacedName{Name: predictorServiceKey.Name,
				Namespace: serviceKey.Namespace}
			Eventually(func() error { return k8sClient.Get(context.TODO(), predictorHPAKey, actualHPA) }, timeout).
				Should(Succeed())
			expectedHPA := &autoscalingv2.HorizontalPodAutoscaler{
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       predictorServiceKey.Name,
					},
					MinReplicas: &minReplicas,
					MaxReplicas: maxReplicas,
					Metrics: []autoscalingv2.MetricSpec{
						{
							Type: autoscalingv2.ResourceMetricSourceType,
							Resource: &autoscalingv2.ResourceMetricSource{
								Name: v1.ResourceCPU,
								Target: autoscalingv2.MetricTarget{
									Type:               "Utilization",
									AverageUtilization: &cpuUtilization,
								},
							},
						},
					},
					Behavior: &autoscalingv2.HorizontalPodAutoscalerBehavior{
						ScaleUp: &autoscalingv2.HPAScalingRules{
							StabilizationWindowSeconds: &stabilizationWindowSeconds,
							SelectPolicy:               &selectPolicy,
							Policies: []autoscalingv2.HPAScalingPolicy{
								{
									Type:          "Pods",
									Value:         4,
									PeriodSeconds: 15,
								},
								{
									Type:          "Percent",
									Value:         100,
									PeriodSeconds: 15,
								},
							},
						},
						ScaleDown: &autoscalingv2.HPAScalingRules{
							StabilizationWindowSeconds: nil,
							SelectPolicy:               &selectPolicy,
							Policies: []autoscalingv2.HPAScalingPolicy{
								{
									Type:          "Percent",
									Value:         100,
									PeriodSeconds: 15,
								},
							},
						},
					},
				},
			}
			Expect(actualHPA.Spec).To(BeComparableTo(expectedHPA.Spec))
		})
	})
	Context("When creating inference service with raw kube path based routing predictor and transformer", func() {
		configs := map[string]string{
			"explainers": `{
				"alibi": {
					"image": "kserve/alibi-explainer",
					"defaultImageVersion": "latest"
				}
			}`,
			"ingress": `{
				"enableGatewayApi": true,
				"kserveIngressGateway": "kserve/kserve-ingress-gateway",
				"ingressGateway": "knative-serving/knative-ingress-gateway",
				"localGateway": "knative-serving/knative-local-gateway",
				"localGatewayService": "knative-local-gateway.istio-system.svc.cluster.local",
				"additionalIngressDomains": ["additional.example.com"],
				"ingressDomain": "example.com",
				"pathTemplate": "/serving/{{ .Namespace }}/{{ .Name }}"
			}`,
			"storageInitializer": `{
				"image" : "kserve/storage-initializer:latest",
				"memoryRequest": "100Mi",
				"memoryLimit": "1Gi",
				"cpuRequest": "100m",
				"cpuLimit": "1",
				"CaBundleConfigMapName": "",
				"caBundleVolumeMountPath": "/etc/ssl/custom-certs",
				"enableDirectPvcVolumeMount": false
			}`,
		}

		It("Should have ingress/service/deployment/hpa created for transformer and predictor", func() {
			By("By creating a new InferenceService")
			// Create configmap
			var configMap = &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KServeNamespace,
				},
				Data: configs,
			}
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)
			// Create ServingRuntime
			servingRuntime := &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tf-serving-raw",
					Namespace: "default",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{
							Name:       "tensorflow",
							Version:    proto.String("1"),
							AutoSelect: proto.Bool(true),
						},
					},
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []v1.Container{
							{
								Name:    "kserve-container",
								Image:   "tensorflow/serving:1.14.0",
								Command: []string{"/usr/bin/tensorflow_model_server"},
								Args: []string{
									"--port=9000",
									"--rest_api_port=8080",
									"--model_base_path=/mnt/models",
									"--rest_api_timeout_in_ms=60000",
								},
								Resources: defaultResource,
							},
						},
					},
					Disabled: proto.Bool(false),
				},
			}
			k8sClient.Create(context.TODO(), servingRuntime)
			defer k8sClient.Delete(context.TODO(), servingRuntime)
			serviceName := "raw-foo-trans-path"
			namespace := "default"
			var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: serviceName, Namespace: namespace}}
			var serviceKey = expectedRequest.NamespacedName
			var predictorServiceKey = types.NamespacedName{Name: constants.PredictorServiceName(serviceName),
				Namespace: namespace}
			var transformerServiceKey = types.NamespacedName{Name: constants.TransformerServiceName(serviceName),
				Namespace: namespace}
			var storageUri = "s3://test/mnist/export"
			var minReplicas int32 = 1
			var maxReplicas int32 = 3
			var transformerMinReplicas int32 = 1
			var transformerMaxReplicas int32 = 2
			var transformerCpuUtilization int32 = 80
			var transformerStabilizationWindowSeconds int32 = 0
			transformerSelectPolicy := autoscalingv2.MaxChangePolicySelect
			ctx := context.Background()
			isvc := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceKey.Name,
					Namespace: serviceKey.Namespace,
					Annotations: map[string]string{
						"serving.kserve.io/deploymentMode":              "RawDeployment",
						"serving.kserve.io/autoscalerClass":             "hpa",
						"serving.kserve.io/metrics":                     "cpu",
						"serving.kserve.io/targetUtilizationPercentage": "75",
					},
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
							MinReplicas: utils.ToPointer(int(minReplicas)),
							MaxReplicas: int(maxReplicas),
						},
						Tensorflow: &v1beta1.TFServingSpec{
							PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
								StorageURI:     &storageUri,
								RuntimeVersion: proto.String("1.14.0"),
								Container: v1.Container{
									Name:      constants.InferenceServiceContainerName,
									Resources: defaultResource,
								},
							},
						},
					},
					Transformer: &v1beta1.TransformerSpec{
						ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
							MinReplicas: utils.ToPointer(int(transformerMinReplicas)),
							MaxReplicas: int(transformerMaxReplicas),
							ScaleTarget: utils.ToPointer(int(transformerCpuUtilization)),
						},
						PodSpec: v1beta1.PodSpec{
							Containers: []v1.Container{
								{
									Image:     "transformer:v1",
									Resources: defaultResource,
									Args: []string{
										"--port=8080",
									},
								},
							},
						},
					},
				},
			}
			isvcConfig, err := v1beta1.NewInferenceServicesConfig(clientset)
			Expect(err).NotTo(HaveOccurred())
			isvc.DefaultInferenceService(isvcConfig, nil, &v1beta1.SecurityConfig{AutoMountServiceAccountToken: false}, nil)
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
			defer k8sClient.Delete(ctx, isvc)

			inferenceService := &v1beta1.InferenceService{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, serviceKey, inferenceService)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			// check predictor deployment
			actualPredictorDeployment := &appsv1.Deployment{}
			predictorDeploymentKey := types.NamespacedName{Name: predictorServiceKey.Name,
				Namespace: serviceKey.Namespace}
			Eventually(func() error { return k8sClient.Get(context.TODO(), predictorDeploymentKey, actualPredictorDeployment) }, timeout).
				Should(Succeed())
			var replicas int32 = 1
			var revisionHistory int32 = 10
			var progressDeadlineSeconds int32 = 600
			var gracePeriod int64 = 30
			expectedPredictorDeployment := &appsv1.Deployment{
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
								constants.StorageInitializerSourceUriInternalAnnotationKey: *isvc.Spec.Predictor.Model.StorageURI,
								"serving.kserve.io/deploymentMode":                         "RawDeployment",
								"serving.kserve.io/autoscalerClass":                        "hpa",
								"serving.kserve.io/metrics":                                "cpu",
								"serving.kserve.io/targetUtilizationPercentage":            "75",
							},
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Image: "tensorflow/serving:" +
										*isvc.Spec.Predictor.Model.RuntimeVersion,
									Name:    constants.InferenceServiceContainerName,
									Command: []string{v1beta1.TensorflowEntrypointCommand},
									Args: []string{
										"--port=" + v1beta1.TensorflowServingGRPCPort,
										"--rest_api_port=" + v1beta1.TensorflowServingRestPort,
										"--model_base_path=" + constants.DefaultModelLocalMountPath,
										"--rest_api_timeout_in_ms=60000",
									},
									Resources: defaultResource,
									ReadinessProbe: &v1.Probe{
										ProbeHandler: v1.ProbeHandler{
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
							AutomountServiceAccountToken: proto.Bool(false),
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
			Expect(actualPredictorDeployment.Spec).To(BeComparableTo(expectedPredictorDeployment.Spec))

			// check transformer deployment
			actualTransformerDeployment := &appsv1.Deployment{}
			transformerDeploymentKey := types.NamespacedName{Name: transformerServiceKey.Name,
				Namespace: serviceKey.Namespace}
			Eventually(func() error {
				return k8sClient.Get(context.TODO(), transformerDeploymentKey, actualTransformerDeployment)
			}, timeout).
				Should(Succeed())
			expectedTransformerDeployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      transformerDeploymentKey.Name,
					Namespace: transformerDeploymentKey.Namespace,
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: &replicas,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "isvc." + transformerDeploymentKey.Name,
						},
					},
					Template: v1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Name:      transformerDeploymentKey.Name,
							Namespace: "default",
							Labels: map[string]string{
								"app":                                 "isvc." + transformerDeploymentKey.Name,
								constants.KServiceComponentLabel:      constants.Transformer.String(),
								constants.InferenceServicePodLabelKey: serviceName,
							},
							Annotations: map[string]string{
								"serving.kserve.io/deploymentMode":              "RawDeployment",
								"serving.kserve.io/autoscalerClass":             "hpa",
								"serving.kserve.io/metrics":                     "cpu",
								"serving.kserve.io/targetUtilizationPercentage": "75",
							},
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Image: "transformer:v1",
									Name:  constants.InferenceServiceContainerName,
									Args: []string{
										"--port=8080",
										"--model_name",
										serviceKey.Name,
										"--predictor_host",
										fmt.Sprintf("%s.%s", predictorServiceKey.Name, predictorServiceKey.Namespace),
										"--http_port",
										"8080",
									},
									Resources: defaultResource,
									ReadinessProbe: &v1.Probe{
										ProbeHandler: v1.ProbeHandler{
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
							AutomountServiceAccountToken: proto.Bool(false),
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
			Expect(actualTransformerDeployment.Spec).To(BeComparableTo(expectedTransformerDeployment.Spec))

			//check predictor service
			actualPredictorService := &v1.Service{}
			Eventually(func() error { return k8sClient.Get(context.TODO(), predictorServiceKey, actualPredictorService) }, timeout).
				Should(Succeed())
			expectedPredictorService := &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      predictorServiceKey.Name,
					Namespace: predictorServiceKey.Namespace,
				},
				Spec: v1.ServiceSpec{
					Ports: []v1.ServicePort{
						{
							Name:       predictorServiceKey.Name,
							Protocol:   "TCP",
							Port:       80,
							TargetPort: intstr.IntOrString{Type: 0, IntVal: 8080, StrVal: ""},
						},
					},
					Type:            "ClusterIP",
					SessionAffinity: "None",
					Selector: map[string]string{
						"app": fmt.Sprintf("isvc.%s", predictorServiceKey.Name),
					},
				},
			}
			actualPredictorService.Spec.ClusterIP = ""
			actualPredictorService.Spec.ClusterIPs = nil
			actualPredictorService.Spec.IPFamilies = nil
			actualPredictorService.Spec.IPFamilyPolicy = nil
			actualPredictorService.Spec.InternalTrafficPolicy = nil
			Expect(actualPredictorService.Spec).To(BeComparableTo(expectedPredictorService.Spec))

			//check transformer service
			actualTransformerService := &v1.Service{}
			Eventually(func() error { return k8sClient.Get(context.TODO(), transformerServiceKey, actualTransformerService) }, timeout).
				Should(Succeed())
			expectedTransformerService := &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      transformerServiceKey.Name,
					Namespace: transformerServiceKey.Namespace,
				},
				Spec: v1.ServiceSpec{
					Ports: []v1.ServicePort{
						{
							Name:       transformerServiceKey.Name,
							Protocol:   "TCP",
							Port:       80,
							TargetPort: intstr.IntOrString{Type: 0, IntVal: 8080, StrVal: ""},
						},
					},
					Type:            "ClusterIP",
					SessionAffinity: "None",
					Selector: map[string]string{
						"app": fmt.Sprintf("isvc.%s", transformerServiceKey.Name),
					},
				},
			}
			actualTransformerService.Spec.ClusterIP = ""
			actualTransformerService.Spec.ClusterIPs = nil
			actualTransformerService.Spec.IPFamilies = nil
			actualTransformerService.Spec.IPFamilyPolicy = nil
			actualTransformerService.Spec.InternalTrafficPolicy = nil
			Expect(actualTransformerService.Spec).To(BeComparableTo(expectedTransformerService.Spec))

			// update deployment status to make isvc ready
			updatedPredictorDeployment := actualPredictorDeployment.DeepCopy()
			updatedPredictorDeployment.Status.Conditions = []appsv1.DeploymentCondition{
				{
					Type:   appsv1.DeploymentAvailable,
					Status: v1.ConditionTrue,
				},
			}
			Expect(k8sClient.Status().Update(context.TODO(), updatedPredictorDeployment)).NotTo(HaveOccurred())
			updatedTransformerDeployment := actualTransformerDeployment.DeepCopy()
			updatedTransformerDeployment.Status.Conditions = []appsv1.DeploymentCondition{
				{
					Type:   appsv1.DeploymentAvailable,
					Status: v1.ConditionTrue,
				},
			}
			Expect(k8sClient.Status().Update(context.TODO(), updatedTransformerDeployment)).NotTo(HaveOccurred())

			//check http route
			actualToplevelHttpRoute := &gatewayapiv1.HTTPRoute{}
			Eventually(func() error {
				return k8sClient.Get(context.TODO(), types.NamespacedName{Name: serviceKey.Name,
					Namespace: serviceKey.Namespace}, actualToplevelHttpRoute)
			}, timeout).
				Should(Succeed())
			topLevelHost := fmt.Sprintf("%s-%s.%s", serviceKey.Name, serviceKey.Namespace, "example.com")
			prefixUrlPath := fmt.Sprintf("/serving/%s/%s", serviceKey.Namespace, serviceKey.Name)
			expectedToplevelHttpRoute := gatewayapiv1.HTTPRoute{
				Spec: gatewayapiv1.HTTPRouteSpec{
					Hostnames: []gatewayapiv1.Hostname{gatewayapiv1.Hostname(topLevelHost), "additional.example.com", "example.com"},
					Rules: []gatewayapiv1.HTTPRouteRule{
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer(constants.FallbackPrefix()),
									},
								},
							},
							Filters: []gatewayapiv1.HTTPRouteFilter{
								{
									Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
										Set: []gatewayapiv1.HTTPHeader{
											{
												Name:  constants.IsvcNameHeader,
												Value: serviceKey.Name,
											},
											{
												Name:  constants.IsvcNamespaceHeader,
												Value: serviceKey.Namespace,
											},
										},
									},
								},
							},
							BackendRefs: []gatewayapiv1.HTTPBackendRef{
								{
									BackendRef: gatewayapiv1.BackendRef{
										BackendObjectReference: gatewayapiv1.BackendObjectReference{
											Group:     (*gatewayapiv1.Group)(utils.ToPointer("")),
											Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      gatewayapiv1.ObjectName(transformerServiceKey.Name),
											Namespace: (*gatewayapiv1.Namespace)(utils.ToPointer(serviceKey.Namespace)),
											Port:      (*gatewayapiv1.PortNumber)(utils.ToPointer(int32(constants.CommonDefaultHttpPort))),
										},
										Weight: utils.ToPointer(int32(1)),
									},
								},
							},
							Timeouts: &gatewayapiv1.HTTPRouteTimeouts{
								Request: utils.ToPointer(gatewayapiv1.Duration("0s")),
							},
						},
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer(prefixUrlPath + "/"),
									},
								},
							},
							Filters: []gatewayapiv1.HTTPRouteFilter{
								{
									Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
										Set: []gatewayapiv1.HTTPHeader{
											{
												Name:  constants.IsvcNameHeader,
												Value: serviceKey.Name,
											},
											{
												Name:  constants.IsvcNamespaceHeader,
												Value: serviceKey.Namespace,
											},
										},
									},
								},
							},
							BackendRefs: []gatewayapiv1.HTTPBackendRef{
								{
									BackendRef: gatewayapiv1.BackendRef{
										BackendObjectReference: gatewayapiv1.BackendObjectReference{
											Group:     (*gatewayapiv1.Group)(utils.ToPointer("")),
											Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      gatewayapiv1.ObjectName(transformerServiceKey.Name),
											Namespace: (*gatewayapiv1.Namespace)(utils.ToPointer(serviceKey.Namespace)),
											Port:      (*gatewayapiv1.PortNumber)(utils.ToPointer(int32(constants.CommonDefaultHttpPort))),
										},
										Weight: utils.ToPointer(int32(1)),
									},
								},
							},
							Timeouts: &gatewayapiv1.HTTPRouteTimeouts{
								Request: utils.ToPointer(gatewayapiv1.Duration("0s")),
							},
						},
					},
					CommonRouteSpec: gatewayapiv1.CommonRouteSpec{
						ParentRefs: []gatewayapiv1.ParentReference{
							{
								Name:      gatewayapiv1.ObjectName(kserveGateway.Name),
								Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.GatewayKind)),
								Group:     (*gatewayapiv1.Group)(&gatewayapiv1.GroupVersion.Group),
								Namespace: utils.ToPointer(gatewayapiv1.Namespace(kserveGateway.Namespace)),
							},
						},
					},
				},
			}
			Expect(actualToplevelHttpRoute.Spec).To(BeComparableTo(expectedToplevelHttpRoute.Spec))

			actualPredictorHttpRoute := &gatewayapiv1.HTTPRoute{}
			Eventually(func() error {
				return k8sClient.Get(context.TODO(), types.NamespacedName{Name: predictorServiceKey.Name,
					Namespace: serviceKey.Namespace}, actualPredictorHttpRoute)
			}, timeout).
				Should(Succeed())
			predictorHost := fmt.Sprintf("%s-%s.%s", predictorServiceKey.Name, serviceKey.Namespace, "example.com")
			expectedPredictorHttpRoute := gatewayapiv1.HTTPRoute{
				Spec: gatewayapiv1.HTTPRouteSpec{
					Hostnames: []gatewayapiv1.Hostname{gatewayapiv1.Hostname(predictorHost)},
					Rules: []gatewayapiv1.HTTPRouteRule{
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer(constants.FallbackPrefix()),
									},
								},
							},
							Filters: []gatewayapiv1.HTTPRouteFilter{
								{
									Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
										Set: []gatewayapiv1.HTTPHeader{
											{
												Name:  constants.IsvcNameHeader,
												Value: serviceKey.Name,
											},
											{
												Name:  constants.IsvcNamespaceHeader,
												Value: serviceKey.Namespace,
											},
										},
									},
								},
							},
							BackendRefs: []gatewayapiv1.HTTPBackendRef{
								{
									BackendRef: gatewayapiv1.BackendRef{
										BackendObjectReference: gatewayapiv1.BackendObjectReference{
											Group:     (*gatewayapiv1.Group)(utils.ToPointer("")),
											Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      gatewayapiv1.ObjectName(predictorServiceKey.Name),
											Namespace: (*gatewayapiv1.Namespace)(utils.ToPointer(serviceKey.Namespace)),
											Port:      (*gatewayapiv1.PortNumber)(utils.ToPointer(int32(constants.CommonDefaultHttpPort))),
										},
										Weight: utils.ToPointer(int32(1)),
									},
								},
							},
							Timeouts: &gatewayapiv1.HTTPRouteTimeouts{
								Request: utils.ToPointer(gatewayapiv1.Duration("0s")),
							},
						},
					},
					CommonRouteSpec: gatewayapiv1.CommonRouteSpec{
						ParentRefs: []gatewayapiv1.ParentReference{
							{
								Name:      gatewayapiv1.ObjectName(kserveGateway.Name),
								Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.GatewayKind)),
								Group:     (*gatewayapiv1.Group)(&gatewayapiv1.GroupVersion.Group),
								Namespace: utils.ToPointer(gatewayapiv1.Namespace(kserveGateway.Namespace)),
							},
						},
					},
				},
			}
			Expect(actualPredictorHttpRoute.Spec).To(BeComparableTo(expectedPredictorHttpRoute.Spec))

			actualTransformerHttpRoute := &gatewayapiv1.HTTPRoute{}
			Eventually(func() error {
				return k8sClient.Get(context.TODO(), types.NamespacedName{Name: transformerServiceKey.Name,
					Namespace: serviceKey.Namespace}, actualTransformerHttpRoute)
			}, timeout).
				Should(Succeed())
			transformerHost := fmt.Sprintf("%s-%s.%s", transformerServiceKey.Name, serviceKey.Namespace, "example.com")
			expectedTransformerHttpRoute := gatewayapiv1.HTTPRoute{
				Spec: gatewayapiv1.HTTPRouteSpec{
					Hostnames: []gatewayapiv1.Hostname{gatewayapiv1.Hostname(transformerHost)},
					Rules: []gatewayapiv1.HTTPRouteRule{
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer(constants.FallbackPrefix()),
									},
								},
							},
							Filters: []gatewayapiv1.HTTPRouteFilter{
								{
									Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
										Set: []gatewayapiv1.HTTPHeader{
											{
												Name:  constants.IsvcNameHeader,
												Value: serviceKey.Name,
											},
											{
												Name:  constants.IsvcNamespaceHeader,
												Value: serviceKey.Namespace,
											},
										},
									},
								},
							},
							BackendRefs: []gatewayapiv1.HTTPBackendRef{
								{
									BackendRef: gatewayapiv1.BackendRef{
										BackendObjectReference: gatewayapiv1.BackendObjectReference{
											Group:     (*gatewayapiv1.Group)(utils.ToPointer("")),
											Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      gatewayapiv1.ObjectName(transformerServiceKey.Name),
											Namespace: (*gatewayapiv1.Namespace)(utils.ToPointer(serviceKey.Namespace)),
											Port:      (*gatewayapiv1.PortNumber)(utils.ToPointer(int32(constants.CommonDefaultHttpPort))),
										},
										Weight: utils.ToPointer(int32(1)),
									},
								},
							},
							Timeouts: &gatewayapiv1.HTTPRouteTimeouts{
								Request: utils.ToPointer(gatewayapiv1.Duration("0s")),
							},
						},
					},
					CommonRouteSpec: gatewayapiv1.CommonRouteSpec{
						ParentRefs: []gatewayapiv1.ParentReference{
							{
								Name:      gatewayapiv1.ObjectName(kserveGateway.Name),
								Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.GatewayKind)),
								Group:     (*gatewayapiv1.Group)(&gatewayapiv1.GroupVersion.Group),
								Namespace: utils.ToPointer(gatewayapiv1.Namespace(kserveGateway.Namespace)),
							},
						},
					},
				},
			}
			Expect(actualTransformerHttpRoute.Spec).To(BeComparableTo(expectedTransformerHttpRoute.Spec))

			// Mark the Ingress as accepted to make isvc ready
			httpRouteStatus := gatewayapiv1.HTTPRouteStatus{
				RouteStatus: gatewayapiv1.RouteStatus{
					Parents: []gatewayapiv1.RouteParentStatus{
						{
							ParentRef: gatewayapiv1.ParentReference{
								Name:      gatewayapiv1.ObjectName(kserveGateway.Name),
								Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.GatewayKind)),
								Group:     (*gatewayapiv1.Group)(&gatewayapiv1.GroupVersion.Group),
								Namespace: utils.ToPointer(gatewayapiv1.Namespace(kserveGateway.Namespace)),
							},
							ControllerName: "istio.io/gateway-controller",
							Conditions: []metav1.Condition{
								{
									Type:               string(gatewayapiv1.ListenerConditionAccepted),
									Status:             metav1.ConditionTrue,
									Reason:             "Accepted",
									Message:            "Route was valid",
									LastTransitionTime: metav1.Now(),
								},
							},
						},
					},
				},
			}
			actualPredictorHttpRoute.Status = httpRouteStatus
			Expect(k8sClient.Status().Update(context.Background(), actualPredictorHttpRoute)).NotTo(HaveOccurred())
			actualTransformerHttpRoute.Status = httpRouteStatus
			Expect(k8sClient.Status().Update(context.Background(), actualTransformerHttpRoute)).NotTo(HaveOccurred())
			actualToplevelHttpRoute.Status = httpRouteStatus
			Expect(k8sClient.Status().Update(context.Background(), actualToplevelHttpRoute)).NotTo(HaveOccurred())

			// verify if InferenceService status is updated
			expectedIsvcStatus := v1beta1.InferenceServiceStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{
							Type:   v1beta1.IngressReady,
							Status: "True",
						},
						{
							Type:   v1beta1.PredictorReady,
							Status: "True",
						},
						{
							Type:   apis.ConditionReady,
							Status: "True",
						},
						{
							Type:     v1beta1.TransformerReady,
							Status:   "True",
							Severity: "Info",
						},
					},
				},
				URL: &apis.URL{
					Scheme: "http",
					Host:   fmt.Sprintf("%s-%s.example.com", serviceKey.Name, serviceKey.Namespace),
				},
				Address: &duckv1.Addressable{
					URL: &apis.URL{
						Scheme: "http",
						Host:   fmt.Sprintf("%s.%s.svc.cluster.local", transformerServiceKey.Name, serviceKey.Namespace),
					},
				},
				Components: map[v1beta1.ComponentType]v1beta1.ComponentStatusSpec{
					v1beta1.PredictorComponent: {
						LatestCreatedRevision: "",
						URL: &apis.URL{
							Scheme: "http",
							Host:   fmt.Sprintf("%s-%s.example.com", predictorServiceKey.Name, serviceKey.Namespace),
						},
					},
					v1beta1.TransformerComponent: {
						LatestCreatedRevision: "",
						URL: &apis.URL{
							Scheme: "http",
							Host:   fmt.Sprintf("%s-%s.example.com", transformerServiceKey.Name, serviceKey.Namespace),
						},
					},
				},
				ModelStatus: v1beta1.ModelStatus{
					TransitionStatus:    "InProgress",
					ModelRevisionStates: &v1beta1.ModelRevisionStates{TargetModelState: "Pending"},
				},
			}
			Eventually(func() string {
				isvc := &v1beta1.InferenceService{}
				if err := k8sClient.Get(context.TODO(), serviceKey, isvc); err != nil {
					return err.Error()
				}
				return cmp.Diff(&expectedIsvcStatus, &isvc.Status, cmpopts.IgnoreTypes(apis.VolatileTime{}))
			}, timeout).Should(BeEmpty())

			//check predictor HPA
			var cpuUtilization int32 = 75
			var stabilizationWindowSeconds int32 = 0
			selectPolicy := autoscalingv2.MaxChangePolicySelect
			actualPredictorHPA := &autoscalingv2.HorizontalPodAutoscaler{}
			predictorHPAKey := types.NamespacedName{Name: predictorServiceKey.Name,
				Namespace: serviceKey.Namespace}
			Eventually(func() error { return k8sClient.Get(context.TODO(), predictorHPAKey, actualPredictorHPA) }, timeout).
				Should(Succeed())
			expectedPredictorHPA := &autoscalingv2.HorizontalPodAutoscaler{
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       predictorServiceKey.Name,
					},
					MinReplicas: &minReplicas,
					MaxReplicas: maxReplicas,
					Metrics: []autoscalingv2.MetricSpec{
						{
							Type: autoscalingv2.ResourceMetricSourceType,
							Resource: &autoscalingv2.ResourceMetricSource{
								Name: v1.ResourceCPU,
								Target: autoscalingv2.MetricTarget{
									Type:               "Utilization",
									AverageUtilization: &cpuUtilization,
								},
							},
						},
					},
					Behavior: &autoscalingv2.HorizontalPodAutoscalerBehavior{
						ScaleUp: &autoscalingv2.HPAScalingRules{
							StabilizationWindowSeconds: &stabilizationWindowSeconds,
							SelectPolicy:               &selectPolicy,
							Policies: []autoscalingv2.HPAScalingPolicy{
								{
									Type:          "Pods",
									Value:         4,
									PeriodSeconds: 15,
								},
								{
									Type:          "Percent",
									Value:         100,
									PeriodSeconds: 15,
								},
							},
						},
						ScaleDown: &autoscalingv2.HPAScalingRules{
							StabilizationWindowSeconds: nil,
							SelectPolicy:               &selectPolicy,
							Policies: []autoscalingv2.HPAScalingPolicy{
								{
									Type:          "Percent",
									Value:         100,
									PeriodSeconds: 15,
								},
							},
						},
					},
				},
			}
			Expect(actualPredictorHPA.Spec).To(BeComparableTo(expectedPredictorHPA.Spec))

			// check transformer HPA
			actualTransformerHPA := &autoscalingv2.HorizontalPodAutoscaler{}
			transformerHPAKey := types.NamespacedName{Name: transformerServiceKey.Name,
				Namespace: serviceKey.Namespace}
			Eventually(func() error { return k8sClient.Get(context.TODO(), transformerHPAKey, actualTransformerHPA) }, timeout).
				Should(Succeed())
			expectedTransformerHPA := &autoscalingv2.HorizontalPodAutoscaler{
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       transformerServiceKey.Name,
					},
					MinReplicas: &transformerMinReplicas,
					MaxReplicas: transformerMaxReplicas,
					Metrics: []autoscalingv2.MetricSpec{
						{
							Type: autoscalingv2.ResourceMetricSourceType,
							Resource: &autoscalingv2.ResourceMetricSource{
								Name: v1.ResourceCPU,
								Target: autoscalingv2.MetricTarget{
									Type:               "Utilization",
									AverageUtilization: &transformerCpuUtilization,
								},
							},
						},
					},
					Behavior: &autoscalingv2.HorizontalPodAutoscalerBehavior{
						ScaleUp: &autoscalingv2.HPAScalingRules{
							StabilizationWindowSeconds: &transformerStabilizationWindowSeconds,
							SelectPolicy:               &transformerSelectPolicy,
							Policies: []autoscalingv2.HPAScalingPolicy{
								{
									Type:          "Pods",
									Value:         4,
									PeriodSeconds: 15,
								},
								{
									Type:          "Percent",
									Value:         100,
									PeriodSeconds: 15,
								},
							},
						},
						ScaleDown: &autoscalingv2.HPAScalingRules{
							StabilizationWindowSeconds: nil,
							SelectPolicy:               &transformerSelectPolicy,
							Policies: []autoscalingv2.HPAScalingPolicy{
								{
									Type:          "Percent",
									Value:         100,
									PeriodSeconds: 15,
								},
							},
						},
					},
				},
			}
			Expect(actualTransformerHPA.Spec).To(BeComparableTo(expectedTransformerHPA.Spec))
		})
	})
	Context("When creating inference service with raw kube path based routing predictor and explainer", func() {
		configs := map[string]string{
			"explainers": `{
				"art": {
					"image": "kserve/art-explainer",
					"defaultImageVersion": "latest"
				}
			}`,
			"ingress": `{
				"enableGatewayApi": true,
				"kserveIngressGateway": "kserve/kserve-ingress-gateway",
				"ingressGateway": "knative-serving/knative-ingress-gateway",
				"localGateway": "knative-serving/knative-local-gateway",
				"localGatewayService": "knative-local-gateway.istio-system.svc.cluster.local",
				"additionalIngressDomains": ["additional.example.com"],
				"ingressDomain": "example.com",
				"pathTemplate": "/serving/{{ .Namespace }}/{{ .Name }}"
			}`,
			"storageInitializer": `{
				"image" : "kserve/storage-initializer:latest",
				"memoryRequest": "100Mi",
				"memoryLimit": "1Gi",
				"cpuRequest": "100m",
				"cpuLimit": "1",
				"CaBundleConfigMapName": "",
				"caBundleVolumeMountPath": "/etc/ssl/custom-certs",
				"enableDirectPvcVolumeMount": false
			}`,
		}

		It("Should have httproute/service/deployment/hpa created for explainer and predictor", func() {
			By("By creating a new InferenceService")
			// Create configmap
			var configMap = &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KServeNamespace,
				},
				Data: configs,
			}
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)
			// Create ServingRuntime
			servingRuntime := &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tf-serving-raw",
					Namespace: "default",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{
							Name:       "tensorflow",
							Version:    proto.String("1"),
							AutoSelect: proto.Bool(true),
						},
					},
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []v1.Container{
							{
								Name:    "kserve-container",
								Image:   "tensorflow/serving:1.14.0",
								Command: []string{"/usr/bin/tensorflow_model_server"},
								Args: []string{
									"--port=9000",
									"--rest_api_port=8080",
									"--model_base_path=/mnt/models",
									"--rest_api_timeout_in_ms=60000",
								},
								Resources: defaultResource,
							},
						},
					},
					Disabled: proto.Bool(false),
				},
			}
			k8sClient.Create(context.TODO(), servingRuntime)
			defer k8sClient.Delete(context.TODO(), servingRuntime)
			serviceName := "raw-foo-exp-path"
			namespace := "default"
			var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: serviceName, Namespace: namespace}}
			var serviceKey = expectedRequest.NamespacedName
			var predictorServiceKey = types.NamespacedName{Name: constants.PredictorServiceName(serviceName),
				Namespace: namespace}
			var explainerServiceKey = types.NamespacedName{Name: constants.ExplainerServiceName(serviceName),
				Namespace: namespace}
			var storageUri = "s3://test/mnist/export"
			var minReplicas int32 = 1
			var maxReplicas int32 = 3
			var explainerMinReplicas int32 = 1
			var explainerMaxReplicas int32 = 2
			var explainerCpuUtilization int32 = 80
			var explainerStabilizationWindowSeconds int32 = 0
			ExplainerSelectPolicy := autoscalingv2.MaxChangePolicySelect
			ctx := context.Background()
			isvc := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceKey.Name,
					Namespace: serviceKey.Namespace,
					Annotations: map[string]string{
						"serving.kserve.io/deploymentMode":              "RawDeployment",
						"serving.kserve.io/autoscalerClass":             "hpa",
						"serving.kserve.io/metrics":                     "cpu",
						"serving.kserve.io/targetUtilizationPercentage": "75",
					},
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
							MinReplicas: utils.ToPointer(int(minReplicas)),
							MaxReplicas: int(maxReplicas),
						},
						Tensorflow: &v1beta1.TFServingSpec{
							PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
								StorageURI:     &storageUri,
								RuntimeVersion: proto.String("1.14.0"),
								Container: v1.Container{
									Name:      constants.InferenceServiceContainerName,
									Resources: defaultResource,
								},
							},
						},
					},
					Explainer: &v1beta1.ExplainerSpec{
						ART: &v1beta1.ARTExplainerSpec{
							Type: v1beta1.ARTSquareAttackExplainer,
							ExplainerExtensionSpec: v1beta1.ExplainerExtensionSpec{
								Config: map[string]string{"nb_classes": "10"},
								Container: v1.Container{
									Name:      constants.InferenceServiceContainerName,
									Resources: defaultResource,
								},
							},
						},
						ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
							MinReplicas: utils.ToPointer(int(explainerMinReplicas)),
							MaxReplicas: int(explainerMaxReplicas),
							ScaleTarget: utils.ToPointer(int(explainerCpuUtilization)),
						},
					},
				},
			}
			isvcConfig, err := v1beta1.NewInferenceServicesConfig(clientset)
			Expect(err).NotTo(HaveOccurred())
			isvc.DefaultInferenceService(isvcConfig, nil, &v1beta1.SecurityConfig{AutoMountServiceAccountToken: false}, nil)
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
			defer k8sClient.Delete(ctx, isvc)

			inferenceService := &v1beta1.InferenceService{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, serviceKey, inferenceService)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			// check predictor deployment
			actualPredictorDeployment := &appsv1.Deployment{}
			predictorDeploymentKey := types.NamespacedName{Name: predictorServiceKey.Name,
				Namespace: serviceKey.Namespace}
			Eventually(func() error { return k8sClient.Get(context.TODO(), predictorDeploymentKey, actualPredictorDeployment) }, timeout).
				Should(Succeed())
			var replicas int32 = 1
			var revisionHistory int32 = 10
			var progressDeadlineSeconds int32 = 600
			var gracePeriod int64 = 30
			expectedPredictorDeployment := &appsv1.Deployment{
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
								constants.StorageInitializerSourceUriInternalAnnotationKey: *isvc.Spec.Predictor.Model.StorageURI,
								"serving.kserve.io/deploymentMode":                         "RawDeployment",
								"serving.kserve.io/autoscalerClass":                        "hpa",
								"serving.kserve.io/metrics":                                "cpu",
								"serving.kserve.io/targetUtilizationPercentage":            "75",
							},
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Image: "tensorflow/serving:" +
										*isvc.Spec.Predictor.Model.RuntimeVersion,
									Name:    constants.InferenceServiceContainerName,
									Command: []string{v1beta1.TensorflowEntrypointCommand},
									Args: []string{
										"--port=" + v1beta1.TensorflowServingGRPCPort,
										"--rest_api_port=" + v1beta1.TensorflowServingRestPort,
										"--model_base_path=" + constants.DefaultModelLocalMountPath,
										"--rest_api_timeout_in_ms=60000",
									},
									Resources: defaultResource,
									ReadinessProbe: &v1.Probe{
										ProbeHandler: v1.ProbeHandler{
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
							AutomountServiceAccountToken: proto.Bool(false),
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
			Expect(actualPredictorDeployment.Spec).To(BeComparableTo(expectedPredictorDeployment.Spec))

			// check Explainer deployment
			actualExplainerDeployment := &appsv1.Deployment{}
			explainerDeploymentKey := types.NamespacedName{Name: explainerServiceKey.Name,
				Namespace: serviceKey.Namespace}
			Eventually(func() error {
				return k8sClient.Get(context.TODO(), explainerDeploymentKey, actualExplainerDeployment)
			}, timeout).
				Should(Succeed())
			expectedExplainerDeployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      explainerDeploymentKey.Name,
					Namespace: explainerDeploymentKey.Namespace,
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: &replicas,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "isvc." + explainerDeploymentKey.Name,
						},
					},
					Template: v1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Name:      explainerDeploymentKey.Name,
							Namespace: "default",
							Labels: map[string]string{
								"app":                                 "isvc." + explainerDeploymentKey.Name,
								constants.KServiceComponentLabel:      constants.Explainer.String(),
								constants.InferenceServicePodLabelKey: serviceName,
							},
							Annotations: map[string]string{
								"serving.kserve.io/deploymentMode":              "RawDeployment",
								"serving.kserve.io/autoscalerClass":             "hpa",
								"serving.kserve.io/metrics":                     "cpu",
								"serving.kserve.io/targetUtilizationPercentage": "75",
							},
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Image: "kserve/art-explainer:latest",
									Name:  constants.InferenceServiceContainerName,
									Args: []string{
										"--model_name",
										serviceKey.Name,
										"--http_port",
										"8080",
										"--predictor_host",
										fmt.Sprintf("%s.%s", predictorServiceKey.Name, predictorServiceKey.Namespace),
										"--adversary_type",
										"SquareAttack",
										"--nb_classes",
										"10",
									},
									Resources: defaultResource,
									ReadinessProbe: &v1.Probe{
										ProbeHandler: v1.ProbeHandler{
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
							AutomountServiceAccountToken: proto.Bool(false),
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
			Expect(actualExplainerDeployment.Spec).To(BeComparableTo(expectedExplainerDeployment.Spec))

			//check predictor service
			actualPredictorService := &v1.Service{}
			Eventually(func() error { return k8sClient.Get(context.TODO(), predictorServiceKey, actualPredictorService) }, timeout).
				Should(Succeed())
			expectedPredictorService := &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      predictorServiceKey.Name,
					Namespace: predictorServiceKey.Namespace,
				},
				Spec: v1.ServiceSpec{
					Ports: []v1.ServicePort{
						{
							Name:       predictorServiceKey.Name,
							Protocol:   "TCP",
							Port:       80,
							TargetPort: intstr.IntOrString{Type: 0, IntVal: 8080, StrVal: ""},
						},
					},
					Type:            "ClusterIP",
					SessionAffinity: "None",
					Selector: map[string]string{
						"app": fmt.Sprintf("isvc.%s", predictorServiceKey.Name),
					},
				},
			}
			actualPredictorService.Spec.ClusterIP = ""
			actualPredictorService.Spec.ClusterIPs = nil
			actualPredictorService.Spec.IPFamilies = nil
			actualPredictorService.Spec.IPFamilyPolicy = nil
			actualPredictorService.Spec.InternalTrafficPolicy = nil
			Expect(actualPredictorService.Spec).To(BeComparableTo(expectedPredictorService.Spec))

			//check Explainer service
			actualExplainerService := &v1.Service{}
			Eventually(func() error { return k8sClient.Get(context.TODO(), explainerServiceKey, actualExplainerService) }, timeout).
				Should(Succeed())
			expectedExplainerService := &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      explainerServiceKey.Name,
					Namespace: explainerServiceKey.Namespace,
				},
				Spec: v1.ServiceSpec{
					Ports: []v1.ServicePort{
						{
							Name:       explainerServiceKey.Name,
							Protocol:   "TCP",
							Port:       80,
							TargetPort: intstr.IntOrString{Type: 0, IntVal: 8080, StrVal: ""},
						},
					},
					Type:            "ClusterIP",
					SessionAffinity: "None",
					Selector: map[string]string{
						"app": fmt.Sprintf("isvc.%s", explainerServiceKey.Name),
					},
				},
			}
			actualExplainerService.Spec.ClusterIP = ""
			actualExplainerService.Spec.ClusterIPs = nil
			actualExplainerService.Spec.IPFamilies = nil
			actualExplainerService.Spec.IPFamilyPolicy = nil
			actualExplainerService.Spec.InternalTrafficPolicy = nil
			Expect(actualExplainerService.Spec).To(BeComparableTo(expectedExplainerService.Spec))

			// update deployment status to make isvc ready
			updatedPredictorDeployment := actualPredictorDeployment.DeepCopy()
			updatedPredictorDeployment.Status.Conditions = []appsv1.DeploymentCondition{
				{
					Type:   appsv1.DeploymentAvailable,
					Status: v1.ConditionTrue,
				},
			}
			Expect(k8sClient.Status().Update(context.TODO(), updatedPredictorDeployment)).NotTo(HaveOccurred())
			updatedExplainerDeployment := actualExplainerDeployment.DeepCopy()
			updatedExplainerDeployment.Status.Conditions = []appsv1.DeploymentCondition{
				{
					Type:   appsv1.DeploymentAvailable,
					Status: v1.ConditionTrue,
				},
			}
			Expect(k8sClient.Status().Update(context.TODO(), updatedExplainerDeployment)).NotTo(HaveOccurred())

			//check http route
			actualToplevelHttpRoute := &gatewayapiv1.HTTPRoute{}
			Eventually(func() error {
				return k8sClient.Get(context.TODO(), types.NamespacedName{Name: serviceKey.Name,
					Namespace: serviceKey.Namespace}, actualToplevelHttpRoute)
			}, timeout).
				Should(Succeed())
			topLevelHost := fmt.Sprintf("%s-%s.%s", serviceKey.Name, serviceKey.Namespace, "example.com")
			prefixUrlPath := fmt.Sprintf("/serving/%s/%s", serviceKey.Namespace, serviceKey.Name)
			expectedToplevelHttpRoute := gatewayapiv1.HTTPRoute{
				Spec: gatewayapiv1.HTTPRouteSpec{
					Hostnames: []gatewayapiv1.Hostname{gatewayapiv1.Hostname(topLevelHost), "additional.example.com", "example.com"},
					Rules: []gatewayapiv1.HTTPRouteRule{
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer(constants.ExplainPrefix()),
									},
								},
							},
							Filters: []gatewayapiv1.HTTPRouteFilter{
								{
									Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
										Set: []gatewayapiv1.HTTPHeader{
											{
												Name:  constants.IsvcNameHeader,
												Value: serviceKey.Name,
											},
											{
												Name:  constants.IsvcNamespaceHeader,
												Value: serviceKey.Namespace,
											},
										},
									},
								},
							},
							BackendRefs: []gatewayapiv1.HTTPBackendRef{
								{
									BackendRef: gatewayapiv1.BackendRef{
										BackendObjectReference: gatewayapiv1.BackendObjectReference{
											Group:     (*gatewayapiv1.Group)(utils.ToPointer("")),
											Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      gatewayapiv1.ObjectName(explainerServiceKey.Name),
											Namespace: (*gatewayapiv1.Namespace)(utils.ToPointer(serviceKey.Namespace)),
											Port:      (*gatewayapiv1.PortNumber)(utils.ToPointer(int32(constants.CommonDefaultHttpPort))),
										},
										Weight: utils.ToPointer(int32(1)),
									},
								},
							},
							Timeouts: &gatewayapiv1.HTTPRouteTimeouts{
								Request: utils.ToPointer(gatewayapiv1.Duration("0s")),
							},
						},
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer(constants.FallbackPrefix()),
									},
								},
							},
							Filters: []gatewayapiv1.HTTPRouteFilter{
								{
									Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
										Set: []gatewayapiv1.HTTPHeader{
											{
												Name:  constants.IsvcNameHeader,
												Value: serviceKey.Name,
											},
											{
												Name:  constants.IsvcNamespaceHeader,
												Value: serviceKey.Namespace,
											},
										},
									},
								},
							},
							BackendRefs: []gatewayapiv1.HTTPBackendRef{
								{
									BackendRef: gatewayapiv1.BackendRef{
										BackendObjectReference: gatewayapiv1.BackendObjectReference{
											Group:     (*gatewayapiv1.Group)(utils.ToPointer("")),
											Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      gatewayapiv1.ObjectName(predictorServiceKey.Name),
											Namespace: (*gatewayapiv1.Namespace)(utils.ToPointer(serviceKey.Namespace)),
											Port:      (*gatewayapiv1.PortNumber)(utils.ToPointer(int32(constants.CommonDefaultHttpPort))),
										},
										Weight: utils.ToPointer(int32(1)),
									},
								},
							},
							Timeouts: &gatewayapiv1.HTTPRouteTimeouts{
								Request: utils.ToPointer(gatewayapiv1.Duration("0s")),
							},
						},
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer(prefixUrlPath + constants.PathBasedExplainPrefix()),
									},
								},
							},
							Filters: []gatewayapiv1.HTTPRouteFilter{
								{
									Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
										Set: []gatewayapiv1.HTTPHeader{
											{
												Name:  constants.IsvcNameHeader,
												Value: serviceKey.Name,
											},
											{
												Name:  constants.IsvcNamespaceHeader,
												Value: serviceKey.Namespace,
											},
										},
									},
								},
							},
							BackendRefs: []gatewayapiv1.HTTPBackendRef{
								{
									BackendRef: gatewayapiv1.BackendRef{
										BackendObjectReference: gatewayapiv1.BackendObjectReference{
											Group:     (*gatewayapiv1.Group)(utils.ToPointer("")),
											Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      gatewayapiv1.ObjectName(explainerServiceKey.Name),
											Namespace: (*gatewayapiv1.Namespace)(utils.ToPointer(serviceKey.Namespace)),
											Port:      (*gatewayapiv1.PortNumber)(utils.ToPointer(int32(constants.CommonDefaultHttpPort))),
										},
										Weight: utils.ToPointer(int32(1)),
									},
								},
							},
							Timeouts: &gatewayapiv1.HTTPRouteTimeouts{
								Request: utils.ToPointer(gatewayapiv1.Duration("0s")),
							},
						},
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer(prefixUrlPath + "/"),
									},
								},
							},
							Filters: []gatewayapiv1.HTTPRouteFilter{
								{
									Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
										Set: []gatewayapiv1.HTTPHeader{
											{
												Name:  constants.IsvcNameHeader,
												Value: serviceKey.Name,
											},
											{
												Name:  constants.IsvcNamespaceHeader,
												Value: serviceKey.Namespace,
											},
										},
									},
								},
							},
							BackendRefs: []gatewayapiv1.HTTPBackendRef{
								{
									BackendRef: gatewayapiv1.BackendRef{
										BackendObjectReference: gatewayapiv1.BackendObjectReference{
											Group:     (*gatewayapiv1.Group)(utils.ToPointer("")),
											Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      gatewayapiv1.ObjectName(predictorServiceKey.Name),
											Namespace: (*gatewayapiv1.Namespace)(utils.ToPointer(serviceKey.Namespace)),
											Port:      (*gatewayapiv1.PortNumber)(utils.ToPointer(int32(constants.CommonDefaultHttpPort))),
										},
										Weight: utils.ToPointer(int32(1)),
									},
								},
							},
							Timeouts: &gatewayapiv1.HTTPRouteTimeouts{
								Request: utils.ToPointer(gatewayapiv1.Duration("0s")),
							},
						},
					},
					CommonRouteSpec: gatewayapiv1.CommonRouteSpec{
						ParentRefs: []gatewayapiv1.ParentReference{
							{
								Name:      gatewayapiv1.ObjectName(kserveGateway.Name),
								Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.GatewayKind)),
								Group:     (*gatewayapiv1.Group)(&gatewayapiv1.GroupVersion.Group),
								Namespace: utils.ToPointer(gatewayapiv1.Namespace(kserveGateway.Namespace)),
							},
						},
					},
				},
			}
			Expect(actualToplevelHttpRoute.Spec).To(BeComparableTo(expectedToplevelHttpRoute.Spec))

			actualPredictorHttpRoute := &gatewayapiv1.HTTPRoute{}
			Eventually(func() error {
				return k8sClient.Get(context.TODO(), types.NamespacedName{Name: predictorServiceKey.Name,
					Namespace: serviceKey.Namespace}, actualPredictorHttpRoute)
			}, timeout).
				Should(Succeed())
			predictorHost := fmt.Sprintf("%s-%s.%s", predictorServiceKey.Name, serviceKey.Namespace, "example.com")
			expectedPredictorHttpRoute := gatewayapiv1.HTTPRoute{
				Spec: gatewayapiv1.HTTPRouteSpec{
					Hostnames: []gatewayapiv1.Hostname{gatewayapiv1.Hostname(predictorHost)},
					Rules: []gatewayapiv1.HTTPRouteRule{
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer(constants.FallbackPrefix()),
									},
								},
							},
							Filters: []gatewayapiv1.HTTPRouteFilter{
								{
									Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
										Set: []gatewayapiv1.HTTPHeader{
											{
												Name:  constants.IsvcNameHeader,
												Value: serviceKey.Name,
											},
											{
												Name:  constants.IsvcNamespaceHeader,
												Value: serviceKey.Namespace,
											},
										},
									},
								},
							},
							BackendRefs: []gatewayapiv1.HTTPBackendRef{
								{
									BackendRef: gatewayapiv1.BackendRef{
										BackendObjectReference: gatewayapiv1.BackendObjectReference{
											Group:     (*gatewayapiv1.Group)(utils.ToPointer("")),
											Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      gatewayapiv1.ObjectName(predictorServiceKey.Name),
											Namespace: (*gatewayapiv1.Namespace)(utils.ToPointer(serviceKey.Namespace)),
											Port:      (*gatewayapiv1.PortNumber)(utils.ToPointer(int32(constants.CommonDefaultHttpPort))),
										},
										Weight: utils.ToPointer(int32(1)),
									},
								},
							},
							Timeouts: &gatewayapiv1.HTTPRouteTimeouts{
								Request: utils.ToPointer(gatewayapiv1.Duration("0s")),
							},
						},
					},
					CommonRouteSpec: gatewayapiv1.CommonRouteSpec{
						ParentRefs: []gatewayapiv1.ParentReference{
							{
								Name:      gatewayapiv1.ObjectName(kserveGateway.Name),
								Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.GatewayKind)),
								Group:     (*gatewayapiv1.Group)(&gatewayapiv1.GroupVersion.Group),
								Namespace: utils.ToPointer(gatewayapiv1.Namespace(kserveGateway.Namespace)),
							},
						},
					},
				},
			}
			Expect(actualPredictorHttpRoute.Spec).To(BeComparableTo(expectedPredictorHttpRoute.Spec))

			actualExplainerHttpRoute := &gatewayapiv1.HTTPRoute{}
			Eventually(func() error {
				return k8sClient.Get(context.TODO(), types.NamespacedName{Name: explainerServiceKey.Name,
					Namespace: serviceKey.Namespace}, actualExplainerHttpRoute)
			}, timeout).
				Should(Succeed())
			explainerHost := fmt.Sprintf("%s-%s.%s", explainerServiceKey.Name, serviceKey.Namespace, "example.com")
			expectedExplainerHttpRoute := gatewayapiv1.HTTPRoute{
				Spec: gatewayapiv1.HTTPRouteSpec{
					Hostnames: []gatewayapiv1.Hostname{gatewayapiv1.Hostname(explainerHost)},
					Rules: []gatewayapiv1.HTTPRouteRule{
						{
							Matches: []gatewayapiv1.HTTPRouteMatch{
								{
									Path: &gatewayapiv1.HTTPPathMatch{
										Type:  utils.ToPointer(gatewayapiv1.PathMatchRegularExpression),
										Value: utils.ToPointer(constants.FallbackPrefix()),
									},
								},
							},
							Filters: []gatewayapiv1.HTTPRouteFilter{
								{
									Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
										Set: []gatewayapiv1.HTTPHeader{
											{
												Name:  constants.IsvcNameHeader,
												Value: serviceKey.Name,
											},
											{
												Name:  constants.IsvcNamespaceHeader,
												Value: serviceKey.Namespace,
											},
										},
									},
								},
							},
							BackendRefs: []gatewayapiv1.HTTPBackendRef{
								{
									BackendRef: gatewayapiv1.BackendRef{
										BackendObjectReference: gatewayapiv1.BackendObjectReference{
											Group:     (*gatewayapiv1.Group)(utils.ToPointer("")),
											Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.ServiceKind)),
											Name:      gatewayapiv1.ObjectName(explainerServiceKey.Name),
											Namespace: (*gatewayapiv1.Namespace)(utils.ToPointer(serviceKey.Namespace)),
											Port:      (*gatewayapiv1.PortNumber)(utils.ToPointer(int32(constants.CommonDefaultHttpPort))),
										},
										Weight: utils.ToPointer(int32(1)),
									},
								},
							},
							Timeouts: &gatewayapiv1.HTTPRouteTimeouts{
								Request: utils.ToPointer(gatewayapiv1.Duration("0s")),
							},
						},
					},
					CommonRouteSpec: gatewayapiv1.CommonRouteSpec{
						ParentRefs: []gatewayapiv1.ParentReference{
							{
								Name:      gatewayapiv1.ObjectName(kserveGateway.Name),
								Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.GatewayKind)),
								Group:     (*gatewayapiv1.Group)(&gatewayapiv1.GroupVersion.Group),
								Namespace: utils.ToPointer(gatewayapiv1.Namespace(kserveGateway.Namespace)),
							},
						},
					},
				},
			}
			Expect(actualExplainerHttpRoute.Spec).To(BeComparableTo(expectedExplainerHttpRoute.Spec))

			// Mark the Ingress as accepted to make isvc ready
			httpRouteStatus := gatewayapiv1.HTTPRouteStatus{
				RouteStatus: gatewayapiv1.RouteStatus{
					Parents: []gatewayapiv1.RouteParentStatus{
						{
							ParentRef: gatewayapiv1.ParentReference{
								Name:      gatewayapiv1.ObjectName(kserveGateway.Name),
								Kind:      utils.ToPointer(gatewayapiv1.Kind(constants.GatewayKind)),
								Group:     (*gatewayapiv1.Group)(&gatewayapiv1.GroupVersion.Group),
								Namespace: utils.ToPointer(gatewayapiv1.Namespace(kserveGateway.Namespace)),
							},
							ControllerName: "istio.io/gateway-controller",
							Conditions: []metav1.Condition{
								{
									Type:               string(gatewayapiv1.ListenerConditionAccepted),
									Status:             metav1.ConditionTrue,
									Reason:             "Accepted",
									Message:            "Route was valid",
									LastTransitionTime: metav1.Now(),
								},
							},
						},
					},
				},
			}
			actualPredictorHttpRoute.Status = httpRouteStatus
			Expect(k8sClient.Status().Update(context.Background(), actualPredictorHttpRoute)).NotTo(HaveOccurred())
			actualExplainerHttpRoute.Status = httpRouteStatus
			Expect(k8sClient.Status().Update(context.Background(), actualExplainerHttpRoute)).NotTo(HaveOccurred())
			actualToplevelHttpRoute.Status = httpRouteStatus
			Expect(k8sClient.Status().Update(context.Background(), actualToplevelHttpRoute)).NotTo(HaveOccurred())

			// verify if InferenceService status is updated
			expectedIsvcStatus := v1beta1.InferenceServiceStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{
							Type:     v1beta1.ExplainerReady,
							Status:   "True",
							Severity: "Info",
						},
						{
							Type:   v1beta1.IngressReady,
							Status: "True",
						},
						{
							Type:   v1beta1.PredictorReady,
							Status: "True",
						},
						{
							Type:   apis.ConditionReady,
							Status: "True",
						},
					},
				},
				URL: &apis.URL{
					Scheme: "http",
					Host:   fmt.Sprintf("%s-%s.example.com", serviceKey.Name, serviceKey.Namespace),
				},
				Address: &duckv1.Addressable{
					URL: &apis.URL{
						Scheme: "http",
						Host:   fmt.Sprintf("%s.%s.svc.cluster.local", predictorServiceKey.Name, serviceKey.Namespace),
					},
				},
				Components: map[v1beta1.ComponentType]v1beta1.ComponentStatusSpec{
					v1beta1.PredictorComponent: {
						LatestCreatedRevision: "",
						URL: &apis.URL{
							Scheme: "http",
							Host:   fmt.Sprintf("%s-%s.example.com", predictorServiceKey.Name, serviceKey.Namespace),
						},
					},
					v1beta1.ExplainerComponent: {
						LatestCreatedRevision: "",
						URL: &apis.URL{
							Scheme: "http",
							Host:   fmt.Sprintf("%s-%s.example.com", explainerServiceKey.Name, serviceKey.Namespace),
						},
					},
				},
				ModelStatus: v1beta1.ModelStatus{
					TransitionStatus:    "InProgress",
					ModelRevisionStates: &v1beta1.ModelRevisionStates{TargetModelState: "Pending"},
				},
			}
			Eventually(func() string {
				isvc := &v1beta1.InferenceService{}
				if err := k8sClient.Get(context.TODO(), serviceKey, isvc); err != nil {
					return err.Error()
				}
				return cmp.Diff(&expectedIsvcStatus, &isvc.Status, cmpopts.IgnoreTypes(apis.VolatileTime{}))
			}, timeout).Should(BeEmpty())

			//check predictor HPA
			var cpuUtilization int32 = 75
			var stabilizationWindowSeconds int32 = 0
			selectPolicy := autoscalingv2.MaxChangePolicySelect
			actualPredictorHPA := &autoscalingv2.HorizontalPodAutoscaler{}
			predictorHPAKey := types.NamespacedName{Name: predictorServiceKey.Name,
				Namespace: serviceKey.Namespace}
			Eventually(func() error { return k8sClient.Get(context.TODO(), predictorHPAKey, actualPredictorHPA) }, timeout).
				Should(Succeed())
			expectedPredictorHPA := &autoscalingv2.HorizontalPodAutoscaler{
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       predictorServiceKey.Name,
					},
					MinReplicas: &minReplicas,
					MaxReplicas: maxReplicas,
					Metrics: []autoscalingv2.MetricSpec{
						{
							Type: autoscalingv2.ResourceMetricSourceType,
							Resource: &autoscalingv2.ResourceMetricSource{
								Name: v1.ResourceCPU,
								Target: autoscalingv2.MetricTarget{
									Type:               "Utilization",
									AverageUtilization: &cpuUtilization,
								},
							},
						},
					},
					Behavior: &autoscalingv2.HorizontalPodAutoscalerBehavior{
						ScaleUp: &autoscalingv2.HPAScalingRules{
							StabilizationWindowSeconds: &stabilizationWindowSeconds,
							SelectPolicy:               &selectPolicy,
							Policies: []autoscalingv2.HPAScalingPolicy{
								{
									Type:          "Pods",
									Value:         4,
									PeriodSeconds: 15,
								},
								{
									Type:          "Percent",
									Value:         100,
									PeriodSeconds: 15,
								},
							},
						},
						ScaleDown: &autoscalingv2.HPAScalingRules{
							StabilizationWindowSeconds: nil,
							SelectPolicy:               &selectPolicy,
							Policies: []autoscalingv2.HPAScalingPolicy{
								{
									Type:          "Percent",
									Value:         100,
									PeriodSeconds: 15,
								},
							},
						},
					},
				},
			}
			Expect(actualPredictorHPA.Spec).To(BeComparableTo(expectedPredictorHPA.Spec))

			// check Explainer HPA
			actualExplainerHPA := &autoscalingv2.HorizontalPodAutoscaler{}
			explainerHPAKey := types.NamespacedName{Name: explainerServiceKey.Name,
				Namespace: serviceKey.Namespace}
			Eventually(func() error { return k8sClient.Get(context.TODO(), explainerHPAKey, actualExplainerHPA) }, timeout).
				Should(Succeed())
			expectedExplainerHPA := &autoscalingv2.HorizontalPodAutoscaler{
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       explainerServiceKey.Name,
					},
					MinReplicas: &explainerMinReplicas,
					MaxReplicas: explainerMaxReplicas,
					Metrics: []autoscalingv2.MetricSpec{
						{
							Type: autoscalingv2.ResourceMetricSourceType,
							Resource: &autoscalingv2.ResourceMetricSource{
								Name: v1.ResourceCPU,
								Target: autoscalingv2.MetricTarget{
									Type:               "Utilization",
									AverageUtilization: &explainerCpuUtilization,
								},
							},
						},
					},
					Behavior: &autoscalingv2.HorizontalPodAutoscalerBehavior{
						ScaleUp: &autoscalingv2.HPAScalingRules{
							StabilizationWindowSeconds: &explainerStabilizationWindowSeconds,
							SelectPolicy:               &ExplainerSelectPolicy,
							Policies: []autoscalingv2.HPAScalingPolicy{
								{
									Type:          "Pods",
									Value:         4,
									PeriodSeconds: 15,
								},
								{
									Type:          "Percent",
									Value:         100,
									PeriodSeconds: 15,
								},
							},
						},
						ScaleDown: &autoscalingv2.HPAScalingRules{
							StabilizationWindowSeconds: nil,
							SelectPolicy:               &ExplainerSelectPolicy,
							Policies: []autoscalingv2.HPAScalingPolicy{
								{
									Type:          "Percent",
									Value:         100,
									PeriodSeconds: 15,
								},
							},
						},
					},
				},
			}
			Expect(actualExplainerHPA.Spec).To(BeComparableTo(expectedExplainerHPA.Spec))
		})
	})
	Context("When creating inference service with raw kube predictor with gateway api disabled", func() {
		configs := map[string]string{
			"explainers": `{
				"alibi": {
					"image": "kserve/alibi-explainer",
					"defaultImageVersion": "latest"
				}
			}`,
			"ingress": `{
				"enableGatewayAPI": false,
				"ingressGateway": "knative-serving/knative-ingress-gateway",
				"localGateway": "knative-serving/knative-local-gateway",
				"localGatewayService": "knative-local-gateway.istio-system.svc.cluster.local"
			}`,
			"storageInitializer": `{
				"image" : "kserve/storage-initializer:latest",
				"memoryRequest": "100Mi",
				"memoryLimit": "1Gi",
				"cpuRequest": "100m",
				"cpuLimit": "1",
				"CaBundleConfigMapName": "",
				"caBundleVolumeMountPath": "/etc/ssl/custom-certs",
				"enableDirectPvcVolumeMount": false
			}`,
		}

		It("Should have ingress/service/deployment/hpa created", func() {
			By("By creating a new InferenceService")
			// Create configmap
			var configMap = &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KServeNamespace,
				},
				Data: configs,
			}
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)
			// Create ServingRuntime
			servingRuntime := &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tf-serving-raw",
					Namespace: "default",
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{
							Name:       "tensorflow",
							Version:    proto.String("1"),
							AutoSelect: proto.Bool(true),
						},
					},
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []v1.Container{
							{
								Name:    "kserve-container",
								Image:   "tensorflow/serving:1.14.0",
								Command: []string{"/usr/bin/tensorflow_model_server"},
								Args: []string{
									"--port=9000",
									"--rest_api_port=8080",
									"--model_base_path=/mnt/models",
									"--rest_api_timeout_in_ms=60000",
								},
								Resources: defaultResource,
							},
						},
					},
					Disabled: proto.Bool(false),
				},
			}
			k8sClient.Create(context.TODO(), servingRuntime)
			defer k8sClient.Delete(context.TODO(), servingRuntime)
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
						"serving.kserve.io/deploymentMode":              "RawDeployment",
						"serving.kserve.io/autoscalerClass":             "hpa",
						"serving.kserve.io/metrics":                     "cpu",
						"serving.kserve.io/targetUtilizationPercentage": "75",
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
									Name:      constants.InferenceServiceContainerName,
									Resources: defaultResource,
								},
							},
						},
					},
				},
			}
			isvc.DefaultInferenceService(nil, nil, &v1beta1.SecurityConfig{AutoMountServiceAccountToken: false}, nil)
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
			predictorDeploymentKey := types.NamespacedName{Name: constants.PredictorServiceName(serviceKey.Name),
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
								constants.StorageInitializerSourceUriInternalAnnotationKey: *isvc.Spec.Predictor.Model.StorageURI,
								"serving.kserve.io/deploymentMode":                         "RawDeployment",
								"serving.kserve.io/autoscalerClass":                        "hpa",
								"serving.kserve.io/metrics":                                "cpu",
								"serving.kserve.io/targetUtilizationPercentage":            "75",
							},
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Image: "tensorflow/serving:" +
										*isvc.Spec.Predictor.Model.RuntimeVersion,
									Name:    constants.InferenceServiceContainerName,
									Command: []string{v1beta1.TensorflowEntrypointCommand},
									Args: []string{
										"--port=" + v1beta1.TensorflowServingGRPCPort,
										"--rest_api_port=" + v1beta1.TensorflowServingRestPort,
										"--model_base_path=" + constants.DefaultModelLocalMountPath,
										"--rest_api_timeout_in_ms=60000",
									},
									Resources: defaultResource,
									ReadinessProbe: &v1.Probe{
										ProbeHandler: v1.ProbeHandler{
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
							AutomountServiceAccountToken: proto.Bool(false),
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
			Expect(actualDeployment.Spec).To(Equal(expectedDeployment.Spec))

			//check service
			actualService := &v1.Service{}
			predictorServiceKey := types.NamespacedName{Name: constants.PredictorServiceName(serviceKey.Name),
				Namespace: serviceKey.Namespace}
			Eventually(func() error { return k8sClient.Get(context.TODO(), predictorServiceKey, actualService) }, timeout).
				Should(Succeed())

			expectedService := &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      predictorServiceKey.Name,
					Namespace: predictorServiceKey.Namespace,
				},
				Spec: v1.ServiceSpec{
					Ports: []v1.ServicePort{
						{
							Name:       constants.PredictorServiceName(serviceName),
							Protocol:   "TCP",
							Port:       80,
							TargetPort: intstr.IntOrString{Type: 0, IntVal: 8080, StrVal: ""},
						},
					},
					Type:            "ClusterIP",
					SessionAffinity: "None",
					Selector: map[string]string{
						"app": fmt.Sprintf("isvc.%s", constants.PredictorServiceName(serviceName)),
					},
				},
			}
			actualService.Spec.ClusterIP = ""
			actualService.Spec.ClusterIPs = nil
			actualService.Spec.IPFamilies = nil
			actualService.Spec.IPFamilyPolicy = nil
			actualService.Spec.InternalTrafficPolicy = nil
			Expect(actualService.Spec).To(Equal(expectedService.Spec))

			//check isvc status
			updatedDeployment := actualDeployment.DeepCopy()
			updatedDeployment.Status.Conditions = []appsv1.DeploymentCondition{
				{
					Type:   appsv1.DeploymentAvailable,
					Status: v1.ConditionTrue,
				},
			}
			Expect(k8sClient.Status().Update(context.TODO(), updatedDeployment)).NotTo(HaveOccurred())

			//check ingress
			pathType := netv1.PathTypePrefix
			actualIngress := &netv1.Ingress{}
			predictorIngressKey := types.NamespacedName{Name: serviceKey.Name,
				Namespace: serviceKey.Namespace}
			Eventually(func() error { return k8sClient.Get(context.TODO(), predictorIngressKey, actualIngress) }, timeout).
				Should(Succeed())
			expectedIngress := netv1.Ingress{
				Spec: netv1.IngressSpec{
					Rules: []netv1.IngressRule{
						{
							Host: "raw-foo-default.example.com",
							IngressRuleValue: netv1.IngressRuleValue{
								HTTP: &netv1.HTTPIngressRuleValue{
									Paths: []netv1.HTTPIngressPath{
										{
											Path:     "/",
											PathType: &pathType,
											Backend: netv1.IngressBackend{
												Service: &netv1.IngressServiceBackend{
													Name: "raw-foo-predictor",
													Port: netv1.ServiceBackendPort{
														Number: 80,
													},
												},
											},
										},
									},
								},
							},
						},
						{
							Host: "raw-foo-predictor-default.example.com",
							IngressRuleValue: netv1.IngressRuleValue{
								HTTP: &netv1.HTTPIngressRuleValue{
									Paths: []netv1.HTTPIngressPath{
										{
											Path:     "/",
											PathType: &pathType,
											Backend: netv1.IngressBackend{
												Service: &netv1.IngressServiceBackend{
													Name: "raw-foo-predictor",
													Port: netv1.ServiceBackendPort{
														Number: 80,
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
			}
			Expect(actualIngress.Spec).To(Equal(expectedIngress.Spec))
			// verify if InferenceService status is updated
			expectedIsvcStatus := v1beta1.InferenceServiceStatus{
				Status: duckv1.Status{
					Conditions: duckv1.Conditions{
						{
							Type:   v1beta1.IngressReady,
							Status: "True",
						},
						{
							Type:   v1beta1.PredictorReady,
							Status: "True",
						},
						{
							Type:   apis.ConditionReady,
							Status: "True",
						},
					},
				},
				URL: &apis.URL{
					Scheme: "http",
					Host:   "raw-foo-default.example.com",
				},
				Address: &duckv1.Addressable{
					URL: &apis.URL{
						Scheme: "http",
						Host:   fmt.Sprintf("%s-predictor.%s.svc.cluster.local", serviceKey.Name, serviceKey.Namespace),
					},
				},
				Components: map[v1beta1.ComponentType]v1beta1.ComponentStatusSpec{
					v1beta1.PredictorComponent: {
						LatestCreatedRevision: "",
						URL: &apis.URL{
							Scheme: "http",
							Host:   "raw-foo-predictor-default.example.com",
						},
					},
				},
				ModelStatus: v1beta1.ModelStatus{
					TransitionStatus:    "InProgress",
					ModelRevisionStates: &v1beta1.ModelRevisionStates{TargetModelState: "Pending"},
				},
			}
			Eventually(func() string {
				isvc := &v1beta1.InferenceService{}
				if err := k8sClient.Get(context.TODO(), serviceKey, isvc); err != nil {
					return err.Error()
				}
				return cmp.Diff(&expectedIsvcStatus, &isvc.Status, cmpopts.IgnoreTypes(apis.VolatileTime{}))
			}, timeout).Should(BeEmpty())

			//check HPA
			var minReplicas int32 = 1
			var maxReplicas int32 = 3
			var cpuUtilization int32 = 75
			var stabilizationWindowSeconds int32 = 0
			selectPolicy := autoscalingv2.MaxChangePolicySelect
			actualHPA := &autoscalingv2.HorizontalPodAutoscaler{}
			predictorHPAKey := types.NamespacedName{Name: constants.PredictorServiceName(serviceKey.Name),
				Namespace: serviceKey.Namespace}
			Eventually(func() error { return k8sClient.Get(context.TODO(), predictorHPAKey, actualHPA) }, timeout).
				Should(Succeed())
			expectedHPA := &autoscalingv2.HorizontalPodAutoscaler{
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       constants.PredictorServiceName(serviceKey.Name),
					},
					MinReplicas: &minReplicas,
					MaxReplicas: maxReplicas,
					Metrics: []autoscalingv2.MetricSpec{
						{
							Type: autoscalingv2.ResourceMetricSourceType,
							Resource: &autoscalingv2.ResourceMetricSource{
								Name: v1.ResourceCPU,
								Target: autoscalingv2.MetricTarget{
									Type:               "Utilization",
									AverageUtilization: &cpuUtilization,
								},
							},
						},
					},
					Behavior: &autoscalingv2.HorizontalPodAutoscalerBehavior{
						ScaleUp: &autoscalingv2.HPAScalingRules{
							StabilizationWindowSeconds: &stabilizationWindowSeconds,
							SelectPolicy:               &selectPolicy,
							Policies: []autoscalingv2.HPAScalingPolicy{
								{
									Type:          "Pods",
									Value:         4,
									PeriodSeconds: 15,
								},
								{
									Type:          "Percent",
									Value:         100,
									PeriodSeconds: 15,
								},
							},
						},
						ScaleDown: &autoscalingv2.HPAScalingRules{
							StabilizationWindowSeconds: nil,
							SelectPolicy:               &selectPolicy,
							Policies: []autoscalingv2.HPAScalingPolicy{
								{
									Type:          "Percent",
									Value:         100,
									PeriodSeconds: 15,
								},
							},
						},
					},
				},
			}
			Expect(actualHPA.Spec).To(Equal(expectedHPA.Spec))
		})
	})
	Context("When creating inference service with raw kube predictor with workerSpec", func() {
		var (
			ctx        context.Context
			serviceKey types.NamespacedName
			storageUri string
			isvc       *v1beta1.InferenceService
		)

		isvcNamespace := constants.KServeNamespace
		actualDefaultDeployment := &appsv1.Deployment{}
		actualWorkerDeployment := &appsv1.Deployment{}

		BeforeEach(func() {
			ctx = context.Background()
			storageUri = "pvc://llama-3-8b-pvc/hf/8b_instruction_tuned"

			// Create a ConfigMap
			configs := map[string]string{
				"ingress": `{
            		"ingressGateway": "knative-serving/knative-ingress-gateway",
            		"localGateway": "knative-serving/knative-local-gateway",
            		"localGatewayService": "knative-local-gateway.istio-system.svc.cluster.local"
        		}`,
				"storageInitializer": `{
            		"image" : "kserve/storage-initializer:latest",
           	 		"memoryRequest": "100Mi",
            		"memoryLimit": "1Gi",
            		"cpuRequest": "100m",
            		"cpuLimit": "1",
            		"CaBundleConfigMapName": "",
            		"caBundleVolumeMountPath": "/etc/ssl/custom-certs",
            		"enableDirectPvcVolumeMount": false
        		}`,
			}
			configMap := &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KServeNamespace,
				},
				Data: configs,
			}
			Expect(k8sClient.Create(ctx, configMap)).NotTo(HaveOccurred())
			DeferCleanup(func() {
				k8sClient.Delete(ctx, configMap)
			})

			// Create a ServingRuntime
			servingRuntime := &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "huggingface-server-multinode",
					Namespace: isvcNamespace,
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{
							Name:       "huggingface",
							Version:    proto.String("2"),
							AutoSelect: proto.Bool(true),
						},
					},
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []v1.Container{
							{
								Name:    constants.InferenceServiceContainerName,
								Image:   "kserve/huggingfaceserver:latest",
								Command: []string{"bash", "-c"},
								Args: []string{
									"python3 -m huggingfaceserver --model_name=${MODEL_NAME} --model_dir=${MODEL} --tensor-parallel-size=${TENSOR_PARALLEL_SIZE} --pipeline-parallel-size=${PIPELINE_PARALLEL_SIZE}",
								},
								Resources: defaultResource,
							},
						},
					},
					WorkerSpec: &v1alpha1.WorkerSpec{
						PipelineParallelSize: utils.ToPointer(2),
						TensorParallelSize:   utils.ToPointer(1),
						ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
							Containers: []v1.Container{
								{
									Name:    constants.WorkerContainerName,
									Image:   "kserve/huggingfaceserver:latest",
									Command: []string{"bash", "-c"},
									Args: []string{
										"ray start --address=$RAY_HEAD_ADDRESS --block",
									},
									Resources: defaultResource,
								},
							},
						},
					},
					Disabled: proto.Bool(false),
				},
			}
			Expect(k8sClient.Create(ctx, servingRuntime)).NotTo(HaveOccurred())
			DeferCleanup(func() {
				k8sClient.Delete(ctx, servingRuntime)
			})
		})
		It("Should have services/deployments for head/worker without an autoscaler when workerSpec is set in isvc", func() {
			By("creating a new InferenceService")
			isvcName := "raw-huggingface-multinode-1"
			predictorDeploymentName := constants.PredictorServiceName(isvcName)
			workerDeploymentName := constants.PredictorWorkerServiceName(isvcName)
			serviceKey = types.NamespacedName{Name: isvcName, Namespace: isvcNamespace}

			isvc = &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      isvcName,
					Namespace: isvcNamespace,
					Annotations: map[string]string{
						"serving.kserve.io/deploymentMode":  "RawDeployment",
						"serving.kserve.io/autoscalerClass": "external",
					},
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						Model: &v1beta1.ModelSpec{
							ModelFormat: v1beta1.ModelFormat{
								Name: "huggingface",
							},
							PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
								StorageURI: &storageUri,
							},
						},
						WorkerSpec: &v1beta1.WorkerSpec{},
					},
				},
			}
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
			DeferCleanup(func() {
				k8sClient.Delete(ctx, isvc)
			})

			// Verify inferenceService is createdi
			inferenceService := &v1beta1.InferenceService{}
			Eventually(func() bool {
				return k8sClient.Get(ctx, serviceKey, inferenceService) == nil
			}, timeout, interval).Should(BeTrue())

			// Verify if predictor deployment (default deployment) is created
			Eventually(func() bool {
				return k8sClient.Get(ctx, types.NamespacedName{Name: predictorDeploymentName, Namespace: isvcNamespace}, actualDefaultDeployment) == nil
			}, timeout, interval).Should(BeTrue())

			// Verify if worker node deployment is created.
			Eventually(func() bool {
				return k8sClient.Get(ctx, types.NamespacedName{Name: workerDeploymentName, Namespace: isvcNamespace}, actualWorkerDeployment) == nil
			}, timeout, interval).Should(BeTrue())

			// Verify deployments details
			verifyPipelineParallelSizeDeployments(actualDefaultDeployment, actualWorkerDeployment, "2", utils.ToPointer(int32(1)))

			// Check Services
			actualService := &v1.Service{}
			headServiceName := constants.GeHeadServiceName(isvcName+"-predictor", "1")
			defaultServiceName := isvcName + "-predictor"
			expectedHeadServiceName := types.NamespacedName{Name: headServiceName, Namespace: isvcNamespace}
			expectedDefaultServiceName := types.NamespacedName{Name: defaultServiceName, Namespace: isvcNamespace}

			// Verify if head service is created
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, expectedHeadServiceName, actualService); err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			Expect(actualService.Spec.ClusterIP).Should(Equal("None"))
			Expect(actualService.Spec.PublishNotReadyAddresses).Should(BeTrue())

			// Verify if predictor service (default service) is created
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, expectedDefaultServiceName, actualService); err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			// Verify there if the default autoscaler(HPA) is not created.
			actualHPA := &autoscalingv2.HorizontalPodAutoscaler{}
			predictorHPAKey := types.NamespacedName{Name: constants.PredictorServiceName(isvcName),
				Namespace: isvcNamespace}

			Eventually(func() error {
				err := k8sClient.Get(context.TODO(), predictorHPAKey, actualHPA)
				if err != nil && apierr.IsNotFound(err) {
					return nil
				}
				return fmt.Errorf("expected IsNotFound error, but got %v", err)
			}, timeout).Should(Succeed())
		})
		It("Should use WorkerSpec.PipelineParallelSize value in isvc when it is set", func() {
			By("By creating a new InferenceService")
			isvcName := "raw-huggingface-multinode-4"
			predictorDeploymentName := constants.PredictorServiceName(isvcName)
			workerDeploymentName := constants.PredictorWorkerServiceName(isvcName)
			serviceKey = types.NamespacedName{Name: isvcName, Namespace: isvcNamespace}
			// Create a infereceService
			isvc = &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      isvcName,
					Namespace: isvcNamespace,
					Annotations: map[string]string{
						"serving.kserve.io/deploymentMode":  "RawDeployment",
						"serving.kserve.io/autoscalerClass": "external",
					},
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						Model: &v1beta1.ModelSpec{
							ModelFormat: v1beta1.ModelFormat{
								Name: "huggingface",
							},
							PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
								StorageURI: &storageUri,
							},
						},
						WorkerSpec: &v1beta1.WorkerSpec{
							PipelineParallelSize: utils.ToPointer(3),
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
			DeferCleanup(func() {
				k8sClient.Delete(ctx, isvc)
			})

			// Verify inferenceService is created
			inferenceService := &v1beta1.InferenceService{}
			Eventually(func() bool {
				return k8sClient.Get(ctx, serviceKey, inferenceService) == nil
			}, timeout, interval).Should(BeTrue())

			// Verify if predictor deployment (default deployment) is created
			Eventually(func() bool {
				return k8sClient.Get(ctx, types.NamespacedName{Name: predictorDeploymentName, Namespace: isvcNamespace}, actualDefaultDeployment) == nil
			}, timeout, interval).Should(BeTrue())

			// Verify if worker node deployment is created.
			Eventually(func() bool {
				return k8sClient.Get(ctx, types.NamespacedName{Name: workerDeploymentName, Namespace: isvcNamespace}, actualWorkerDeployment) == nil
			}, timeout, interval).Should(BeTrue())

			// Verify deployments details
			verifyPipelineParallelSizeDeployments(actualDefaultDeployment, actualWorkerDeployment, "3", utils.ToPointer(int32(2)))
		})
		It("Should use WorkerSpec.TensorParallelSize value in isvc when it is set", func() {
			By("creating a new InferenceService")
			isvcName := "raw-huggingface-multinode-5"
			predictorDeploymentName := constants.PredictorServiceName(isvcName)
			workerDeploymentName := constants.PredictorWorkerServiceName(isvcName)
			serviceKey = types.NamespacedName{Name: isvcName, Namespace: isvcNamespace}

			// Create a infereceService
			isvc = &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      isvcName,
					Namespace: isvcNamespace,
					Annotations: map[string]string{
						"serving.kserve.io/deploymentMode":  "RawDeployment",
						"serving.kserve.io/autoscalerClass": "external",
					},
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						Model: &v1beta1.ModelSpec{
							ModelFormat: v1beta1.ModelFormat{
								Name: "huggingface",
							},
							PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
								StorageURI: &storageUri,
							},
						},
						WorkerSpec: &v1beta1.WorkerSpec{
							TensorParallelSize: utils.ToPointer(3),
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
			DeferCleanup(func() {
				k8sClient.Delete(ctx, isvc)
			})

			// Verify if predictor deployment (default deployment) is created
			Eventually(func() bool {
				return k8sClient.Get(ctx, types.NamespacedName{Name: predictorDeploymentName, Namespace: isvcNamespace}, actualDefaultDeployment) == nil
			}, timeout, interval).Should(BeTrue())

			// Verify if worker node deployment is created.
			Eventually(func() bool {
				return k8sClient.Get(ctx, types.NamespacedName{Name: workerDeploymentName, Namespace: isvcNamespace}, actualWorkerDeployment) == nil
			}, timeout, interval).Should(BeTrue())

			// Verify deployments details
			verifyTensorParallelSizeDeployments(actualDefaultDeployment, actualWorkerDeployment, "3", constants.NvidiaGPUResourceType)
		})
	})
})

func verifyPipelineParallelSizeDeployments(actualDefaultDeployment *appsv1.Deployment, actualWorkerDeployment *appsv1.Deployment, pipelineParallelSize string, replicas *int32) {
	// default deployment
	if pipelineParallelSizeEnvValue, exists := utils.GetEnvVarValue(actualDefaultDeployment.Spec.Template.Spec.Containers[0].Env, constants.PipelineParallelSizeEnvName); exists {
		Expect(pipelineParallelSizeEnvValue).Should(Equal(pipelineParallelSize))
	} else {
		Fail("PIPELINE_PARALLEL_SIZE environment variable is not set")
	}
	// worker node deployment
	if pipelineParallelSizeEnvValue, exists := utils.GetEnvVarValue(actualWorkerDeployment.Spec.Template.Spec.Containers[0].Env, constants.PipelineParallelSizeEnvName); exists {
		Expect(pipelineParallelSizeEnvValue).Should(Equal(pipelineParallelSize))
	} else {
		Fail("PIPELINE_PARALLEL_SIZE environment variable is not set")
	}

	Expect(actualWorkerDeployment.Spec.Replicas).Should(Equal(replicas))
}

func verifyTensorParallelSizeDeployments(actualDefaultDeployment *appsv1.Deployment, actualWorkerDeployment *appsv1.Deployment, tensorParallelSize string, gpuResourceType v1.ResourceName) {
	gpuResourceQuantity := resource.MustParse(tensorParallelSize)
	// default deployment
	if tensorParallelSizeEnvValue, exists := utils.GetEnvVarValue(actualDefaultDeployment.Spec.Template.Spec.Containers[0].Env, constants.TensorParallelSizeEnvName); exists {
		Expect(tensorParallelSizeEnvValue).Should(Equal(tensorParallelSize))
	} else {
		Fail("TENSOR_PARALLEL_SIZE environment variable is not set")
	}
	Expect(actualDefaultDeployment.Spec.Template.Spec.Containers[0].Resources.Limits[gpuResourceType]).Should(Equal(gpuResourceQuantity))
	Expect(actualDefaultDeployment.Spec.Template.Spec.Containers[0].Resources.Requests[gpuResourceType]).Should(Equal(gpuResourceQuantity))

	//worker node deployment
	Expect(actualWorkerDeployment.Spec.Template.Spec.Containers[0].Resources.Limits[gpuResourceType]).Should(Equal(gpuResourceQuantity))
	Expect(actualWorkerDeployment.Spec.Template.Spec.Containers[0].Resources.Requests[gpuResourceType]).Should(Equal(gpuResourceQuantity))
}

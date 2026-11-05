/*
Copyright 2025 The KServe Authors.

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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	otelv1beta1 "github.com/open-telemetry/opentelemetry-operator/apis/v1beta1"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
)

// Owner references are stamped inside RawKubeReconciler.Reconcile so that every
// resource a component creates is garbage-collected with the ISVC. Each caller
// (predictor, transformer, explainer, inference graph) creates its own set of
// resources, so a per-resource-type guard is needed — the central Reconcile only
// stamps the sub-reconcilers a given caller actually populated.
var _ = Describe("Owner references on reconciled resources", func() {
	Context("When a raw transformer uses an OTel autoscaler", func() {
		configs := mergeJSONField(getRawKubeTestConfigs(), "ingress", map[string]interface{}{
			"enableGatewayApi": false,
		})
		configs["opentelemetryCollector"] = `{
			"scrapeInterval": "5s",
			"metricReceiverEndpoint": "keda-otel-scaler.keda.svc:4317",
			"metricScalerEndpoint": "keda-otel-scaler.keda.svc:4318"
		}`

		It("Should set an owner reference on the transformer's OTel collector so it is garbage-collected with the ISVC", func() {
			configMap := createInferenceServiceConfigMap(configs)
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			servingRuntime := getServingRuntime("tf-serving-raw", "default")
			_ = k8sClient.Create(context.TODO(), &servingRuntime)
			defer k8sClient.Delete(context.TODO(), &servingRuntime)

			serviceName := "raw-transformer-otel"
			serviceKey := types.NamespacedName{Name: serviceName, Namespace: "default"}
			ctx := context.Background()

			// A PodMetric with the OpenTelemetry backend is what makes the reconciler
			// create an OTel collector for the component.
			otelAutoScaling := &v1beta1.AutoScalingSpec{
				Metrics: []v1beta1.MetricsSpec{
					{
						Type: v1beta1.PodMetricSourceType,
						PodMetric: &v1beta1.PodMetricSource{
							Metric: v1beta1.PodMetrics{
								Backend:     v1beta1.OpenTelemetryBackend,
								MetricNames: []string{"process_cpu_seconds_total"},
								Query:       "avg(process_cpu_seconds_total)",
							},
							Target: v1beta1.MetricTarget{
								Type:  v1beta1.ValueMetricType,
								Value: v1beta1.NewMetricQuantity(""),
							},
						},
					},
				},
			}

			isvc := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceKey.Name,
					Namespace: serviceKey.Namespace,
					Annotations: map[string]string{
						constants.DeploymentMode:          string(constants.Standard),
						constants.AutoscalerClass:         string(constants.AutoscalerClassKeda),
						"sidecar.opentelemetry.io/inject": "true",
					},
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
							MinReplicas: ptr.To(int32(1)),
							MaxReplicas: 3,
							// Predictor carries the same keda+OTel autoscaling so its own
							// reconcile succeeds (the keda class is inherited from the ISVC
							// annotation by every component).
							AutoScaling: otelAutoScaling,
						},
						Tensorflow: &v1beta1.TFServingSpec{
							PredictorExtensionSpec: getCommonPredictorExtensionSpec(),
						},
					},
					Transformer: &v1beta1.TransformerSpec{
						ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
							MinReplicas: ptr.To(int32(1)),
							MaxReplicas: 3,
							AutoScaling: otelAutoScaling,
						},
						PodSpec: v1beta1.PodSpec{
							Containers: []corev1.Container{
								{
									Image: "transformer:v1",
									Args:  []string{"--port=8080"},
								},
							},
						},
					},
				},
			}
			isvc.DefaultInferenceService(nil, nil, &v1beta1.SecurityConfig{AutoMountServiceAccountToken: false}, nil, nil)
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
			defer k8sClient.Delete(ctx, isvc)

			transformerOTelKey := types.NamespacedName{
				Name:      constants.TransformerServiceName(serviceName),
				Namespace: serviceKey.Namespace,
			}

			// Wait for the transformer's OTel collector to be created.
			actualOTelCollector := &otelv1beta1.OpenTelemetryCollector{}
			Eventually(func() error {
				return k8sClient.Get(ctx, transformerOTelKey, actualOTelCollector)
			}, timeout, interval).Should(Succeed())

			// It must be owned by the InferenceService; otherwise it is orphaned and
			// never garbage-collected when the ISVC is deleted.
			controllerRef := metav1.GetControllerOf(actualOTelCollector)
			Expect(controllerRef).NotTo(BeNil(), "transformer OTel collector has no controller owner reference (orphaned)")
			Expect(controllerRef.Kind).To(Equal("InferenceService"))
			Expect(controllerRef.Name).To(Equal(serviceName))
		})
	})

	Context("When a raw predictor uses an HPA autoscaler", func() {
		configs := getRawKubeTestConfigs()

		It("Should set owner references on the predictor's deployment, service and HPA", func() {
			configMap := createInferenceServiceConfigMap(configs)
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			servingRuntime := getServingRuntime("tf-serving-raw", "default")
			_ = k8sClient.Create(context.TODO(), &servingRuntime)
			defer k8sClient.Delete(context.TODO(), &servingRuntime)

			serviceName := "raw-predictor-ownerref"
			serviceKey := types.NamespacedName{Name: serviceName, Namespace: "default"}
			ctx := context.Background()

			isvc := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:        serviceKey.Name,
					Namespace:   serviceKey.Namespace,
					Annotations: getDefaultAnnotations(constants.AutoscalerClassHPA),
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
							MinReplicas: ptr.To(int32(1)),
							MaxReplicas: 3,
						},
						Tensorflow: &v1beta1.TFServingSpec{
							PredictorExtensionSpec: getCommonPredictorExtensionSpec(),
						},
					},
				},
			}
			isvc.DefaultInferenceService(nil, nil, &v1beta1.SecurityConfig{AutoMountServiceAccountToken: false}, nil, nil)
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
			defer k8sClient.Delete(ctx, isvc)

			predictorKey := types.NamespacedName{Name: constants.PredictorServiceName(serviceName), Namespace: "default"}

			// Every resource the predictor caller creates must be owned by the ISVC.
			expectOwnedByISVC := func(obj client.Object) {
				Eventually(func() error { return k8sClient.Get(ctx, predictorKey, obj) }, timeout, interval).Should(Succeed())
				ref := metav1.GetControllerOf(obj)
				Expect(ref).NotTo(BeNil(), "resource has no controller owner reference (orphaned)")
				Expect(ref.Kind).To(Equal("InferenceService"))
				Expect(ref.Name).To(Equal(serviceName))
			}

			expectOwnedByISVC(&appsv1.Deployment{})
			expectOwnedByISVC(&corev1.Service{})
			expectOwnedByISVC(&autoscalingv2.HorizontalPodAutoscaler{})
		})
	})

	Context("When a raw explainer uses an HPA autoscaler", func() {
		configs := getRawKubeTestConfigs()

		It("Should set an owner reference on the explainer's deployment", func() {
			configMap := createInferenceServiceConfigMap(configs)
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			servingRuntime := getServingRuntime("tf-serving-raw", "default")
			_ = k8sClient.Create(context.TODO(), &servingRuntime)
			defer k8sClient.Delete(context.TODO(), &servingRuntime)

			serviceName := "raw-explainer-ownerref"
			serviceKey := types.NamespacedName{Name: serviceName, Namespace: "default"}
			ctx := context.Background()

			isvc := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:        serviceKey.Name,
					Namespace:   serviceKey.Namespace,
					Annotations: getDefaultAnnotations(constants.AutoscalerClassHPA),
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{MinReplicas: ptr.To(int32(1))},
						Tensorflow: &v1beta1.TFServingSpec{
							PredictorExtensionSpec: getCommonPredictorExtensionSpec(),
						},
					},
					Explainer: &v1beta1.ExplainerSpec{
						ART: &v1beta1.ARTExplainerSpec{
							Type: v1beta1.ARTSquareAttackExplainer,
							ExplainerExtensionSpec: v1beta1.ExplainerExtensionSpec{
								Config: map[string]string{"nb_classes": "10"},
								Container: corev1.Container{
									Name:      constants.InferenceServiceContainerName,
									Resources: defaultResource,
								},
								RuntimeVersion: ptr.To("latest"),
							},
						},
						ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
							MinReplicas: ptr.To(int32(1)),
							MaxReplicas: 2,
						},
					},
				},
			}
			isvc.DefaultInferenceService(nil, nil, &v1beta1.SecurityConfig{AutoMountServiceAccountToken: false}, nil, nil)
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
			defer k8sClient.Delete(ctx, isvc)

			explainerKey := types.NamespacedName{Name: constants.ExplainerServiceName(serviceName), Namespace: "default"}
			deployment := &appsv1.Deployment{}
			Eventually(func() error { return k8sClient.Get(ctx, explainerKey, deployment) }, timeout, interval).Should(Succeed())

			ref := metav1.GetControllerOf(deployment)
			Expect(ref).NotTo(BeNil(), "explainer deployment has no controller owner reference (orphaned)")
			Expect(ref.Kind).To(Equal("InferenceService"))
			Expect(ref.Name).To(Equal(serviceName))
		})
	})
})

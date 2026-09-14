/*
Copyright 2026 The KServe Authors.

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
	"google.golang.org/protobuf/proto"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/tracing"
)

func envToMapFromContainer(envVars []corev1.EnvVar) map[string]string {
	m := make(map[string]string, len(envVars))
	for _, e := range envVars {
		m[e.Name] = e.Value
	}
	return m
}

func findContainer(podSpec corev1.PodSpec) *corev1.Container {
	for i := range podSpec.Containers {
		if podSpec.Containers[i].Name == constants.InferenceServiceContainerName {
			return &podSpec.Containers[i]
		}
	}
	return nil
}

var _ = Describe("Tracing injection into predictor deployments", func() {
	configs := getRawKubeTestConfigs()

	Context("When creating an InferenceService with tracing enabled", func() {
		It("Should inject OTEL env vars into the predictor Deployment", func() {
			configMap := createInferenceServiceConfigMap(configs)
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			servingRuntime := getServingRuntime("tf-tracing", "default")
			Expect(k8sClient.Create(context.TODO(), &servingRuntime)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), &servingRuntime)

			serviceName := "tracing-basic-test"
			ctx := context.Background()

			isvc := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:        serviceName,
					Namespace:   "default",
					Annotations: getDefaultAnnotations(constants.AutoscalerClassNone),
				},
				Spec: v1beta1.InferenceServiceSpec{
					Tracing: &v1beta1.TracingSpec{
						ExporterEndpoint: ptr.To("http://otel-collector:4317"),
						Sampler:          ptr.To("parentbased_traceidratio"),
						SamplerArg:       ptr.To("0.05"),
						Exporter:         ptr.To("otlp"),
					},
					Predictor: v1beta1.PredictorSpec{
						ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
							MinReplicas: ptr.To(int32(1)),
						},
						Model: &v1beta1.ModelSpec{
							ModelFormat: v1beta1.ModelFormat{Name: "tensorflow"},
							PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
								StorageURI:     proto.String("s3://test/model"),
								RuntimeVersion: proto.String("0.14.0"),
							},
						},
					},
				},
			}
			isvc.DefaultInferenceService(nil, nil, &v1beta1.SecurityConfig{AutoMountServiceAccountToken: false}, nil, nil)
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
			defer k8sClient.Delete(ctx, isvc)

			predictorKey := types.NamespacedName{
				Name:      constants.PredictorServiceName(serviceName),
				Namespace: "default",
			}

			deploy := &appsv1.Deployment{}
			Eventually(func() error {
				return k8sClient.Get(ctx, predictorKey, deploy)
			}, timeout, interval).Should(Succeed())

			container := findContainer(deploy.Spec.Template.Spec)
			Expect(container).NotTo(BeNil())

			envMap := envToMapFromContainer(container.Env)
			Expect(envMap).To(HaveKeyWithValue(tracing.EnvOtelServiceName, serviceName+"-predictor"))
			Expect(envMap).To(HaveKeyWithValue(tracing.EnvOtelExporterEndpoint, "http://otel-collector:4317"))
			Expect(envMap).To(HaveKeyWithValue(tracing.EnvOtelTracesExporter, "otlp"))
			Expect(envMap).To(HaveKeyWithValue(tracing.EnvOtelTracesSampler, "parentbased_traceidratio"))
			Expect(envMap).To(HaveKeyWithValue(tracing.EnvOtelTracesSamplerArg, "0.05"))
			Expect(envMap).To(HaveKey(tracing.EnvOtelResourceAttributes))
			Expect(envMap[tracing.EnvOtelResourceAttributes]).To(ContainSubstring("isvc.name=" + serviceName))
			Expect(envMap[tracing.EnvOtelResourceAttributes]).To(ContainSubstring("isvc.component=predictor"))
			Expect(envMap[tracing.EnvOtelResourceAttributes]).NotTo(ContainSubstring("isvc.predictor.variant"))
		})

		It("Should NOT inject OTEL env vars when tracing is nil", func() {
			configMap := createInferenceServiceConfigMap(configs)
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			servingRuntime := getServingRuntime("tf-no-tracing", "default")
			Expect(k8sClient.Create(context.TODO(), &servingRuntime)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), &servingRuntime)

			serviceName := "tracing-disabled-test"
			ctx := context.Background()

			isvc := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:        serviceName,
					Namespace:   "default",
					Annotations: getDefaultAnnotations(constants.AutoscalerClassNone),
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
							MinReplicas: ptr.To(int32(1)),
						},
						Model: &v1beta1.ModelSpec{
							ModelFormat: v1beta1.ModelFormat{Name: "tensorflow"},
							PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
								StorageURI:     proto.String("s3://test/model"),
								RuntimeVersion: proto.String("0.14.0"),
							},
						},
					},
				},
			}
			isvc.DefaultInferenceService(nil, nil, &v1beta1.SecurityConfig{AutoMountServiceAccountToken: false}, nil, nil)
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
			defer k8sClient.Delete(ctx, isvc)

			predictorKey := types.NamespacedName{
				Name:      constants.PredictorServiceName(serviceName),
				Namespace: "default",
			}

			deploy := &appsv1.Deployment{}
			Eventually(func() error {
				return k8sClient.Get(ctx, predictorKey, deploy)
			}, timeout, interval).Should(Succeed())

			container := findContainer(deploy.Spec.Template.Spec)
			Expect(container).NotTo(BeNil())

			envMap := envToMapFromContainer(container.Env)
			Expect(envMap).NotTo(HaveKey(tracing.EnvOtelServiceName))
			Expect(envMap).NotTo(HaveKey(tracing.EnvOtelExporterEndpoint))
		})
	})

	Context("When creating a vLLM InferenceService with tracing", func() {
		It("Should inject vLLM CLI args alongside OTEL env vars", func() {
			configMap := createInferenceServiceConfigMap(configs)
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			vllmRuntime := &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.VLLMServer,
					Namespace: "default",
					Annotations: map[string]string{
						constants.ServerTypeAnnotationKey: constants.ServerTypeVLLMServer,
					},
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{
							AutoSelect: ptr.To(true),
							Name:       "vLLM",
						},
					},
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []corev1.Container{
							{
								Name:      constants.InferenceServiceContainerName,
								Image:     "kserve/vllm:latest",
								Resources: defaultResource,
							},
						},
					},
					Disabled: ptr.To(false),
				},
			}
			Expect(k8sClient.Create(context.TODO(), vllmRuntime)).To(Succeed())
			defer k8sClient.Delete(context.TODO(), vllmRuntime)

			serviceName := "tracing-vllm-test"
			ctx := context.Background()
			endpoint := "http://otel-collector:4317"

			isvc := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:        serviceName,
					Namespace:   "default",
					Annotations: getDefaultAnnotations(constants.AutoscalerClassNone),
				},
				Spec: v1beta1.InferenceServiceSpec{
					Tracing: &v1beta1.TracingSpec{
						ExporterEndpoint: ptr.To(endpoint),
						Sampler:          ptr.To("parentbased_traceidratio"),
						SamplerArg:       ptr.To("0.05"),
						Exporter:         ptr.To("otlp"),
					},
					Predictor: v1beta1.PredictorSpec{
						ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
							MinReplicas: ptr.To(int32(1)),
						},
						Model: &v1beta1.ModelSpec{
							ModelFormat: v1beta1.ModelFormat{Name: "vLLM"},
							PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
								RuntimeVersion: ptr.To("latest"),
								Container: corev1.Container{
									Name: constants.InferenceServiceContainerName,
									Resources: corev1.ResourceRequirements{
										Limits: corev1.ResourceList{
											constants.NvidiaGPUResourceType: resource.MustParse("1"),
										},
										Requests: corev1.ResourceList{
											constants.NvidiaGPUResourceType: resource.MustParse("1"),
										},
									},
								},
							},
						},
					},
				},
			}
			isvc.DefaultInferenceService(nil, nil, &v1beta1.SecurityConfig{AutoMountServiceAccountToken: false}, nil, nil)
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
			defer k8sClient.Delete(ctx, isvc)

			predictorKey := types.NamespacedName{
				Name:      constants.PredictorServiceName(serviceName),
				Namespace: "default",
			}

			deploy := &appsv1.Deployment{}
			Eventually(func() error {
				return k8sClient.Get(ctx, predictorKey, deploy)
			}, timeout, interval).Should(Succeed())

			container := findContainer(deploy.Spec.Template.Spec)
			Expect(container).NotTo(BeNil())

			Expect(container.Args).To(ContainElements("--otlp-traces-endpoint", endpoint))
			Expect(container.Args).To(ContainElements("--collect-detailed-traces", "all"))

			envMap := envToMapFromContainer(container.Env)
			Expect(envMap).To(HaveKeyWithValue(tracing.EnvOtelServiceName, serviceName+"-predictor"))
			Expect(envMap).To(HaveKeyWithValue(tracing.EnvOtelExporterEndpoint, endpoint))
		})
	})

	Context("When creating an InferenceService with canary and tracing", func() {
		It("Should inject variant attribute for canary predictor", func() {
			configMap := createInferenceServiceConfigMap(configs)
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			servingRuntime := getServingRuntime("tf-canary-tracing", "default")
			Expect(k8sClient.Create(context.TODO(), &servingRuntime)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), &servingRuntime)

			serviceName := "tracing-canary-test"
			ctx := context.Background()

			isvc := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:        serviceName,
					Namespace:   "default",
					Annotations: getDefaultAnnotations(constants.AutoscalerClassNone),
				},
				Spec: v1beta1.InferenceServiceSpec{
					Tracing: &v1beta1.TracingSpec{
						ExporterEndpoint: ptr.To("http://otel-collector:4317"),
						Sampler:          ptr.To("parentbased_traceidratio"),
						SamplerArg:       ptr.To("0.05"),
						Exporter:         ptr.To("otlp"),
					},
					Predictor: v1beta1.PredictorSpec{
						ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
							MinReplicas: ptr.To(int32(2)),
						},
						Model: &v1beta1.ModelSpec{
							ModelFormat: v1beta1.ModelFormat{Name: "tensorflow"},
							PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
								StorageURI:     proto.String("s3://test/model-v1"),
								RuntimeVersion: proto.String("0.14.0"),
							},
						},
					},
					Canary: []v1beta1.CanarySpec{
						{
							TrafficPercent: 25,
							Predictor: v1beta1.PredictorSpec{
								Name: "v2",
								Model: &v1beta1.ModelSpec{
									ModelFormat: v1beta1.ModelFormat{Name: "tensorflow"},
									PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
										StorageURI:     proto.String("s3://test/model-v2"),
										RuntimeVersion: proto.String("0.14.0"),
									},
								},
							},
						},
					},
				},
			}
			isvc.DefaultInferenceService(nil, nil, &v1beta1.SecurityConfig{AutoMountServiceAccountToken: false}, nil, nil)
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
			defer k8sClient.Delete(ctx, isvc)

			// Verify stable predictor has no variant attribute
			stableKey := types.NamespacedName{
				Name:      constants.PredictorServiceName(serviceName),
				Namespace: "default",
			}
			stableDeploy := &appsv1.Deployment{}
			Eventually(func() error {
				return k8sClient.Get(ctx, stableKey, stableDeploy)
			}, timeout, interval).Should(Succeed())

			stableContainer := findContainer(stableDeploy.Spec.Template.Spec)
			Expect(stableContainer).NotTo(BeNil())

			stableEnvMap := envToMapFromContainer(stableContainer.Env)
			Expect(stableEnvMap).To(HaveKey(tracing.EnvOtelResourceAttributes))
			Expect(stableEnvMap[tracing.EnvOtelResourceAttributes]).NotTo(ContainSubstring("isvc.predictor.variant"))

			// Verify canary predictor has variant attribute
			canaryKey := types.NamespacedName{
				Name:      constants.PredictorServiceName(serviceName, "v2"),
				Namespace: "default",
			}
			canaryDeploy := &appsv1.Deployment{}
			Eventually(func() error {
				return k8sClient.Get(ctx, canaryKey, canaryDeploy)
			}, timeout, interval).Should(Succeed())

			canaryContainer := findContainer(canaryDeploy.Spec.Template.Spec)
			Expect(canaryContainer).NotTo(BeNil())

			canaryEnvMap := envToMapFromContainer(canaryContainer.Env)
			Expect(canaryEnvMap).To(HaveKey(tracing.EnvOtelResourceAttributes))
			Expect(canaryEnvMap[tracing.EnvOtelResourceAttributes]).To(ContainSubstring("isvc.predictor.variant=v2"))
			Expect(canaryEnvMap).To(HaveKeyWithValue(tracing.EnvOtelServiceName, serviceName+"-predictor"))
		})
	})

	Context("When creating an MLServer InferenceService with tracing", func() {
		It("Should inject MLSERVER_TRACING_SERVER alongside OTEL env vars", func() {
			configMap := createInferenceServiceConfigMap(configs)
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			mlserverRuntime := &v1alpha1.ServingRuntime{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.MLServer,
					Namespace: "default",
					Annotations: map[string]string{
						constants.ServerTypeAnnotationKey: constants.ServerTypeMLServer,
					},
				},
				Spec: v1alpha1.ServingRuntimeSpec{
					SupportedModelFormats: []v1alpha1.SupportedModelFormat{
						{
							Name:       "sklearn",
							AutoSelect: ptr.To(true),
						},
					},
					ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
						Containers: []corev1.Container{
							{
								Name:      constants.InferenceServiceContainerName,
								Image:     "seldonio/mlserver:latest",
								Resources: defaultResource,
							},
						},
					},
					Disabled: ptr.To(false),
				},
			}
			Expect(k8sClient.Create(context.TODO(), mlserverRuntime)).To(Succeed())
			defer k8sClient.Delete(context.TODO(), mlserverRuntime)

			serviceName := "tracing-mlserver-test"
			ctx := context.Background()
			endpoint := "http://otel-collector:4317"

			isvc := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:        serviceName,
					Namespace:   "default",
					Annotations: getDefaultAnnotations(constants.AutoscalerClassNone),
				},
				Spec: v1beta1.InferenceServiceSpec{
					Tracing: &v1beta1.TracingSpec{
						ExporterEndpoint: ptr.To(endpoint),
						Sampler:          ptr.To("parentbased_traceidratio"),
						SamplerArg:       ptr.To("0.05"),
						Exporter:         ptr.To("otlp"),
					},
					Predictor: v1beta1.PredictorSpec{
						ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
							MinReplicas: ptr.To(int32(1)),
						},
						Model: &v1beta1.ModelSpec{
							ModelFormat: v1beta1.ModelFormat{Name: "sklearn"},
							PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
								StorageURI:     proto.String("s3://test/sklearn-model"),
								RuntimeVersion: proto.String("latest"),
							},
						},
					},
				},
			}
			isvc.DefaultInferenceService(nil, nil, &v1beta1.SecurityConfig{AutoMountServiceAccountToken: false}, nil, nil)
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
			defer k8sClient.Delete(ctx, isvc)

			predictorKey := types.NamespacedName{
				Name:      constants.PredictorServiceName(serviceName),
				Namespace: "default",
			}

			deploy := &appsv1.Deployment{}
			Eventually(func() error {
				return k8sClient.Get(ctx, predictorKey, deploy)
			}, timeout, interval).Should(Succeed())

			container := findContainer(deploy.Spec.Template.Spec)
			Expect(container).NotTo(BeNil())

			envMap := envToMapFromContainer(container.Env)
			Expect(envMap).To(HaveKeyWithValue(tracing.EnvMLServerTracingServer, endpoint))
			Expect(envMap).To(HaveKeyWithValue(tracing.EnvOtelServiceName, serviceName+"-predictor"))
			Expect(envMap).To(HaveKeyWithValue(tracing.EnvOtelExporterEndpoint, endpoint))
			Expect(envMap).To(HaveKeyWithValue(tracing.EnvOtelTracesExporter, "otlp"))
			Expect(envMap).To(HaveKeyWithValue(tracing.EnvOtelTracesSampler, "parentbased_traceidratio"))
			Expect(envMap).To(HaveKeyWithValue(tracing.EnvOtelTracesSamplerArg, "0.05"))
			Expect(container.Args).NotTo(ContainElement("--otlp-traces-endpoint"))
		})
	})
})

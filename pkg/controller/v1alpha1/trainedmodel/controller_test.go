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

package trainedmodel

import (
	"context"
	"time"

	"github.com/golang/protobuf/proto"
	v1alpha1api "github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	knservingv1 "knative.dev/serving/pkg/apis/serving/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("v1beta1 TrainedModel controller", func() {
	// Define utility constants for object names and testing timeouts/durations and intervals.
	const (
		timeout   = time.Second * 20
		duration  = time.Second * 20
		interval  = time.Millisecond * 250
		domain    = "example.com"
		clusterIp = "example.svc.local.cluster"
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
				  "supportedFrameworks": ["tensorflow"],
				  "multiModelServer": true
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
		namespace       = "default"
		storageUri      = "s3//model1"
		framework       = "tensorflow"
		memory, _       = resource.ParseQuantity("1G")
		shardId         = 0
		readyConditions = duckv1.Status{
			Conditions: duckv1.Conditions{
				{
					Type:               knservingv1.ServiceConditionReady,
					Status:             v1.ConditionTrue,
					LastTransitionTime: apis.VolatileTime{Inner: metav1.NewTime(time.Now())},
				},
				{
					Type:               v1beta1.PredictorReady,
					Status:             v1.ConditionTrue,
					LastTransitionTime: apis.VolatileTime{Inner: metav1.NewTime(time.Now())},
				},
				{
					Type:               v1beta1.IngressReady,
					Status:             v1.ConditionTrue,
					LastTransitionTime: apis.VolatileTime{Inner: metav1.NewTime(time.Now())},
				},
			},
		}
	)

	Context("When creating a new TrainedModel with an unready InferenceService", func() {
		It("Should not add a model to the model configmap", func() {
			modelName := "model0-create"
			parentInferenceService := modelName + "-parent"
			tmKey := types.NamespacedName{Name: modelName, Namespace: namespace}

			// Create InferenceService configmap
			var configMap = &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KServeNamespace,
				},
				Data: configs,
			}
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			// Create the parent InferenceService
			var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: parentInferenceService, Namespace: namespace}}
			var serviceKey = expectedRequest.NamespacedName
			ctx := context.Background()
			isvc := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceKey.Name,
					Namespace: serviceKey.Namespace,
				},
			}
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())

			tmInstance := &v1alpha1api.TrainedModel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      modelName,
					Namespace: namespace,
				},
				Spec: v1alpha1api.TrainedModelSpec{
					InferenceService: parentInferenceService,
					Model: v1alpha1api.ModelSpec{
						StorageURI: storageUri,
						Framework:  framework,
						Memory:     memory,
					},
				},
			}

			Expect(k8sClient.Create(context.TODO(), tmInstance)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), tmInstance)

			Eventually(func() bool {
				tmInstanceUpdate := &v1alpha1api.TrainedModel{}
				if err := k8sClient.Get(context.TODO(), tmKey, tmInstanceUpdate); err != nil {
					return false
				}

				// Condition for inferenceserviceready should be false as isvc is not ready
				isvcReadyCondition := tmInstanceUpdate.Status.GetCondition(v1alpha1api.InferenceServiceReady)

				// Condition for IsMMSPredictor should be false as isvc is not ready
				isMMSPredictorCondition := tmInstanceUpdate.Status.GetCondition(v1alpha1api.IsMMSPredictor)

				if isvcReadyCondition != nil && isvcReadyCondition.Status == v1.ConditionFalse {
					return isMMSPredictorCondition != nil && isMMSPredictorCondition.Status == v1.ConditionFalse
				}

				return false
			}, timeout).Should(BeTrue())
		})
	})

	Context("When creating a new TrainedModel with an ready InferenceService", func() {
		It("Should add a model to the model configmap", func() {
			modelName := "model1-create"
			parentInferenceService := modelName + "-parent"
			modelConfigName := constants.ModelConfigName(parentInferenceService, shardId)
			configmapKey := types.NamespacedName{Name: modelConfigName, Namespace: namespace}
			tmKey := types.NamespacedName{Name: modelName, Namespace: namespace}

			// Create InferenceService configmap
			var configMap = &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KServeNamespace,
				},
				Data: configs,
			}
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			// Create the parent InferenceService
			var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: parentInferenceService, Namespace: namespace}}
			var serviceKey = expectedRequest.NamespacedName
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
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())

			inferenceService := &v1beta1.InferenceService{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, serviceKey, inferenceService)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			inferenceService.Status.Status = readyConditions
			Expect(k8sClient.Status().Update(context.TODO(), inferenceService)).To(BeNil())

			// Create modelConfig
			modelConfig := &v1.ConfigMap{
				TypeMeta:   metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
				ObjectMeta: metav1.ObjectMeta{Name: modelConfigName, Namespace: namespace},
				Data: map[string]string{
					constants.ModelConfigFileName: "",
				},
			}

			tmInstance := &v1alpha1api.TrainedModel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      modelName,
					Namespace: namespace,
				},
				Spec: v1alpha1api.TrainedModelSpec{
					InferenceService: parentInferenceService,
					Model: v1alpha1api.ModelSpec{
						StorageURI: storageUri,
						Framework:  framework,
						Memory:     memory,
					},
				},
			}

			Expect(k8sClient.Create(context.TODO(), modelConfig)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), modelConfig)
			Expect(k8sClient.Create(context.TODO(), tmInstance)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), tmInstance)

			// Verify that the model configmap is updated with the TrainedModel
			configmapActual := &v1.ConfigMap{}
			tmActual := &v1alpha1api.TrainedModel{}
			expected := &v1.ConfigMap{
				TypeMeta:   metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
				ObjectMeta: metav1.ObjectMeta{Name: modelConfigName, Namespace: namespace},
				Data: map[string]string{
					constants.ModelConfigFileName: `[{"modelName":"model1-create","modelSpec":{"storageUri":"s3//model1","framework":"tensorflow","memory":"1G"}}]`,
				},
			}
			Eventually(func() map[string]string {
				ctx := context.Background()
				k8sClient.Get(ctx, configmapKey, configmapActual)
				k8sClient.Get(ctx, tmKey, tmActual)
				return configmapActual.Data
			}, timeout, interval).Should(Equal(expected.Data))
		})
	})

	Context("When updating a TrainedModel", func() {
		It("Should add the model to the model configmap", func() {
			modelName := "model1-update"
			parentInferenceService := modelName + "-parent"
			modelConfigName := constants.ModelConfigName(parentInferenceService, shardId)
			configmapKey := types.NamespacedName{Name: modelConfigName, Namespace: namespace}
			tmKey := types.NamespacedName{Name: modelName, Namespace: namespace}

			// Create InferenceService configmap
			var configMap = &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KServeNamespace,
				},
				Data: configs,
			}
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			// Create the parent InferenceService
			var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: parentInferenceService, Namespace: namespace}}
			var serviceKey = expectedRequest.NamespacedName
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
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())

			inferenceService := &v1beta1.InferenceService{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, serviceKey, inferenceService)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			// Updates the url and address of inference service status
			predictorUrl, _ := apis.ParseURL("http://" + constants.InferenceServiceHostName(constants.DefaultPredictorServiceName(serviceKey.Name), serviceKey.Namespace, domain))
			clusterURL, _ := apis.ParseURL("http://" + constants.InferenceServiceHostName(constants.DefaultPredictorServiceName(serviceKey.Name), serviceKey.Namespace, clusterIp))
			inferenceService.Status.URL = predictorUrl
			inferenceService.Status.Address = &duckv1.Addressable{
				URL: clusterURL,
			}
			inferenceService.Status.Status = readyConditions
			Expect(k8sClient.Status().Update(context.TODO(), inferenceService)).To(BeNil())

			tmInstance := &v1alpha1api.TrainedModel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      modelName,
					Namespace: namespace,
				},
				Spec: v1alpha1api.TrainedModelSpec{
					InferenceService: parentInferenceService,
					Model: v1alpha1api.ModelSpec{
						StorageURI: storageUri,
						Framework:  framework,
						Memory:     memory,
					},
				},
			}

			modelConfig := &v1.ConfigMap{
				TypeMeta:   metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
				ObjectMeta: metav1.ObjectMeta{Name: modelConfigName, Namespace: namespace},
				Data: map[string]string{
					constants.ModelConfigFileName: "",
				},
			}

			Expect(k8sClient.Create(context.TODO(), modelConfig)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), modelConfig)
			Expect(k8sClient.Create(context.TODO(), tmInstance)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), tmInstance)
			tmInstanceUpdate := &v1alpha1api.TrainedModel{}
			Eventually(func() bool {
				if err := k8sClient.Get(context.TODO(), tmKey, tmInstanceUpdate); err != nil {
					return false
				}

				// Condition for inferenceserviceready should be true
				if !tmInstanceUpdate.Status.IsConditionReady(v1alpha1api.InferenceServiceReady) {
					return false
				}

				// Condition for IsMMSPredictor should be true
				if !tmInstanceUpdate.Status.IsConditionReady(v1alpha1api.IsMMSPredictor) {
					return false
				}

				// Condition FrameworkSupported should be true
				if !tmInstanceUpdate.Status.IsConditionReady(v1alpha1api.FrameworkSupported) {
					return false
				}

				// Condition for MemoryResourceAvailable should be true
				if !tmInstanceUpdate.Status.IsConditionReady(v1alpha1api.MemoryResourceAvailable) {
					return false
				}

				if len(tmInstanceUpdate.Finalizers) > 0 {
					if tmInstanceUpdate.Status.Address != nil {
						return tmInstanceUpdate.Status.Address.URL != nil && tmInstanceUpdate.Status.URL != nil
					}
				}
				return false
			}, timeout).Should(BeTrue())

			updatedModelUri := "s3//model2"
			tmInstanceUpdate.Spec.Model.StorageURI = updatedModelUri
			Expect(k8sClient.Update(context.TODO(), tmInstanceUpdate)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), tmInstanceUpdate)

			// Verify that the model configmap is updated with the TrainedModel
			configmapActual := &v1.ConfigMap{}
			tmActual := &v1alpha1api.TrainedModel{}
			expected := &v1.ConfigMap{
				TypeMeta:   metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
				ObjectMeta: metav1.ObjectMeta{Name: modelConfigName, Namespace: namespace},
				Data: map[string]string{
					constants.ModelConfigFileName: `[{"modelName":"model1-update","modelSpec":{"storageUri":"s3//model2","framework":"tensorflow","memory":"1G"}}]`,
				},
			}
			Eventually(func() map[string]string {
				ctx := context.Background()
				k8sClient.Get(ctx, configmapKey, configmapActual)
				k8sClient.Get(ctx, tmKey, tmActual)

				return configmapActual.Data
			}, timeout, interval).Should(Equal(expected.Data))
		})
	})

	Context("When deleting a TrainedModel", func() {
		It("Should update the model configmap to remove model", func() {
			modelName := "model1-delete"
			// modelconfig-model1-delete-parent-0
			parentInferenceService := modelName + "-parent"
			modelConfigName := constants.ModelConfigName(parentInferenceService, shardId)
			configmapKey := types.NamespacedName{Name: modelConfigName, Namespace: namespace}
			tmKey := types.NamespacedName{Name: modelName, Namespace: namespace}

			// Create InferenceService configmap
			var configMap = &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KServeNamespace,
				},
				Data: configs,
			}
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			// Create the parent InferenceService
			var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: parentInferenceService, Namespace: namespace}}
			var serviceKey = expectedRequest.NamespacedName
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
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())

			inferenceService := &v1beta1.InferenceService{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, serviceKey, inferenceService)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			inferenceService.Status.Status = readyConditions
			Expect(k8sClient.Status().Update(context.TODO(), inferenceService)).To(BeNil())

			tmInstance := &v1alpha1api.TrainedModel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      modelName,
					Namespace: namespace,
				},
				Spec: v1alpha1api.TrainedModelSpec{
					InferenceService: parentInferenceService,
					Model: v1alpha1api.ModelSpec{
						StorageURI: storageUri,
						Framework:  framework,
						Memory:     memory,
					},
				},
			}

			modelConfig := &v1.ConfigMap{
				TypeMeta:   metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
				ObjectMeta: metav1.ObjectMeta{Name: modelConfigName, Namespace: namespace},
				Data: map[string]string{
					constants.ModelConfigFileName: "",
				},
			}

			Expect(k8sClient.Create(context.TODO(), modelConfig)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), modelConfig)
			Expect(k8sClient.Create(context.TODO(), tmInstance)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), tmInstance)
			//tmInstanceUpdate := &v1beta1.TrainedModel{}
			//Verify that the model configmap is updated with the new TrainedModel
			configmapActual := &v1.ConfigMap{}
			tmActual := &v1alpha1api.TrainedModel{}
			expected := &v1.ConfigMap{
				TypeMeta:   metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
				ObjectMeta: metav1.ObjectMeta{Name: modelConfigName, Namespace: namespace},
				Data: map[string]string{
					constants.ModelConfigFileName: `[{"modelName":"model1-delete","modelSpec":{"storageUri":"s3//model1","framework":"tensorflow","memory":"1G"}}]`,
				},
			}
			Eventually(func() map[string]string {
				ctx := context.Background()
				k8sClient.Get(ctx, configmapKey, configmapActual)
				k8sClient.Get(ctx, tmKey, tmActual)

				return configmapActual.Data
			}, timeout, interval).Should(Equal(expected.Data))

			Expect(k8sClient.Delete(context.TODO(), tmActual)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), tmActual)

			// Verify that the model is removed from the configmap
			configmapActual = &v1.ConfigMap{}
			tmActual = &v1alpha1api.TrainedModel{}
			expected = &v1.ConfigMap{
				TypeMeta:   metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
				ObjectMeta: metav1.ObjectMeta{Name: modelConfigName, Namespace: namespace},
				Data: map[string]string{
					constants.ModelConfigFileName: "[]",
				},
			}
			Eventually(func() map[string]string {
				ctx := context.Background()
				k8sClient.Get(ctx, configmapKey, configmapActual)
				k8sClient.Get(ctx, tmKey, tmActual)
				return configmapActual.Data
			}, timeout, interval).Should(Equal(expected.Data))
		})
	})

	Context("When creating a TrainedModel with an unsupported framework", func() {
		It("Should not add the model to the model configmap", func() {
			modelName := "model0-framework-unsupported"
			parentInferenceService := modelName + "-parent"
			modelConfigName := constants.ModelConfigName(parentInferenceService, shardId)
			configmapKey := types.NamespacedName{Name: modelConfigName, Namespace: namespace}
			tmKey := types.NamespacedName{Name: modelName, Namespace: namespace}

			// Create InferenceService configmap
			var configMap = &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KServeNamespace,
				},
				Data: configs,
			}
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			// Create the parent InferenceService
			var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: parentInferenceService, Namespace: namespace}}
			var serviceKey = expectedRequest.NamespacedName
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
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())

			inferenceService := &v1beta1.InferenceService{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, serviceKey, inferenceService)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			inferenceService.Status.Status = readyConditions
			Expect(k8sClient.Status().Update(context.TODO(), inferenceService)).To(BeNil())

			// Create modelConfig
			modelConfig := &v1.ConfigMap{
				TypeMeta:   metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
				ObjectMeta: metav1.ObjectMeta{Name: modelConfigName, Namespace: namespace},
				Data: map[string]string{
					constants.ModelConfigFileName: "",
				},
			}

			unsupportedFramework := "unsupportedFramework"
			tmInstance := &v1alpha1api.TrainedModel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      modelName,
					Namespace: namespace,
				},
				Spec: v1alpha1api.TrainedModelSpec{
					InferenceService: parentInferenceService,
					Model: v1alpha1api.ModelSpec{
						StorageURI: storageUri,
						Framework:  unsupportedFramework,
						Memory:     memory,
					},
				},
			}

			Expect(k8sClient.Create(context.TODO(), modelConfig)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), modelConfig)
			Expect(k8sClient.Create(context.TODO(), tmInstance)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), tmInstance)

			Eventually(func() bool {
				tmInstanceUpdate := &v1alpha1api.TrainedModel{}
				if err := k8sClient.Get(context.TODO(), tmKey, tmInstanceUpdate); err != nil {
					return false
				}

				// Condition for inferenceserviceready should be true
				if !tmInstanceUpdate.Status.IsConditionReady(v1alpha1api.InferenceServiceReady) {
					return false
				}

				// Condition for IsMMSPredictor should be true
				if !tmInstanceUpdate.Status.IsConditionReady(v1alpha1api.IsMMSPredictor) {
					return false
				}

				frameworkSupportedCondition := tmInstanceUpdate.Status.GetCondition(v1alpha1api.FrameworkSupported)
				return frameworkSupportedCondition != nil && frameworkSupportedCondition.Status == v1.ConditionFalse

			}, timeout).Should(BeTrue())

			// Verify that the model configmap is updated with the TrainedModel
			configmapActual := &v1.ConfigMap{}
			tmActual := &v1alpha1api.TrainedModel{}
			expected := &v1.ConfigMap{
				TypeMeta:   metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
				ObjectMeta: metav1.ObjectMeta{Name: modelConfigName, Namespace: namespace},
				Data: map[string]string{
					constants.ModelConfigFileName: ``,
				},
			}

			Eventually(func() map[string]string {
				ctx := context.Background()
				k8sClient.Get(ctx, configmapKey, configmapActual)
				k8sClient.Get(ctx, tmKey, tmActual)
				return configmapActual.Data
			}, timeout, interval).Should(Equal(expected.Data))
		})
	})

	Context("When creating a new TrainedModel that requires more memory than available", func() {
		It("Should not add a model to the model configmap", func() {
			modelName := "model0-requires-too-much-memory"
			parentInferenceService := modelName + "-parent"
			modelConfigName := constants.ModelConfigName(parentInferenceService, shardId)
			configmapKey := types.NamespacedName{Name: modelConfigName, Namespace: namespace}
			tmKey := types.NamespacedName{Name: modelName, Namespace: namespace}

			// Create InferenceService configmap
			var configMap = &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KServeNamespace,
				},
				Data: configs,
			}
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			// Create the parent InferenceService
			var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: parentInferenceService, Namespace: namespace}}
			var serviceKey = expectedRequest.NamespacedName
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
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())

			inferenceService := &v1beta1.InferenceService{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, serviceKey, inferenceService)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			inferenceService.Status.Status = readyConditions
			Expect(k8sClient.Status().Update(context.TODO(), inferenceService)).To(BeNil())

			// Create modelConfig
			modelConfig := &v1.ConfigMap{
				TypeMeta:   metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
				ObjectMeta: metav1.ObjectMeta{Name: modelConfigName, Namespace: namespace},
				Data: map[string]string{
					constants.ModelConfigFileName: "",
				},
			}

			tmInstance := &v1alpha1api.TrainedModel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      modelName,
					Namespace: namespace,
				},
				Spec: v1alpha1api.TrainedModelSpec{
					InferenceService: parentInferenceService,
					Model: v1alpha1api.ModelSpec{
						StorageURI: storageUri,
						Framework:  framework,
						Memory:     resource.MustParse("3Gi"),
					},
				},
			}

			Expect(k8sClient.Create(context.TODO(), modelConfig)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), modelConfig)
			Expect(k8sClient.Create(context.TODO(), tmInstance)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), tmInstance)

			Eventually(func() bool {
				tmInstanceUpdate := &v1alpha1api.TrainedModel{}
				if err := k8sClient.Get(context.TODO(), tmKey, tmInstanceUpdate); err != nil {
					return false
				}

				// Condition for inferenceserviceready should be true
				if !tmInstanceUpdate.Status.IsConditionReady(v1alpha1api.InferenceServiceReady) {
					return false
				}

				// Condition for IsMMSPredictor should be true
				if !tmInstanceUpdate.Status.IsConditionReady(v1alpha1api.IsMMSPredictor) {
					return false
				}

				// Condition for MemoryResourceAvailable should be false
				return !tmInstanceUpdate.Status.IsConditionReady(v1alpha1api.MemoryResourceAvailable)

			}, timeout).Should(BeTrue())

			// Verify that the model configmap is updated with the TrainedModel
			configmapActual := &v1.ConfigMap{}
			tmActual := &v1alpha1api.TrainedModel{}
			expected := &v1.ConfigMap{
				TypeMeta:   metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
				ObjectMeta: metav1.ObjectMeta{Name: modelConfigName, Namespace: namespace},
				Data: map[string]string{
					constants.ModelConfigFileName: ``,
				},
			}
			Eventually(func() map[string]string {
				ctx := context.Background()
				k8sClient.Get(ctx, configmapKey, configmapActual)
				k8sClient.Get(ctx, tmKey, tmActual)

				return configmapActual.Data
			}, timeout, interval).Should(Equal(expected.Data))
		})
	})

	Context("When creating a new TrainedModel with a non-mms predictor", func() {
		It("Should not add a model to the model configmap", func() {
			modelName := "model1-non-mms"
			parentInferenceService := modelName + "-parent"
			modelConfigName := constants.ModelConfigName(parentInferenceService, shardId)
			tmKey := types.NamespacedName{Name: modelName, Namespace: namespace}

			// Create InferenceService configmap
			var configMap = &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KServeNamespace,
				},
				Data: configs,
			}
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			// Create the parent InferenceService
			var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: parentInferenceService, Namespace: namespace}}
			var serviceKey = expectedRequest.NamespacedName
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
								RuntimeVersion: proto.String("1.14.0"),
								Container: v1.Container{
									Name:      constants.InferenceServiceContainerName,
									Resources: defaultResource,
								},
								StorageURI: &storageUri,
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

			inferenceService.Status.Status = readyConditions
			Expect(k8sClient.Status().Update(context.TODO(), inferenceService)).To(BeNil())

			// Create modelConfig
			modelConfig := &v1.ConfigMap{
				TypeMeta:   metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
				ObjectMeta: metav1.ObjectMeta{Name: modelConfigName, Namespace: namespace},
				Data: map[string]string{
					constants.ModelConfigFileName: "",
				},
			}

			tmInstance := &v1alpha1api.TrainedModel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      modelName,
					Namespace: namespace,
				},
				Spec: v1alpha1api.TrainedModelSpec{
					InferenceService: parentInferenceService,
					Model: v1alpha1api.ModelSpec{
						StorageURI: storageUri,
						Framework:  framework,
						Memory:     memory,
					},
				},
			}

			Expect(k8sClient.Create(context.TODO(), modelConfig)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), modelConfig)
			Expect(k8sClient.Create(context.TODO(), tmInstance)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), tmInstance)

			Eventually(func() bool {
				tmInstanceUpdate := &v1alpha1api.TrainedModel{}
				if err := k8sClient.Get(context.TODO(), tmKey, tmInstanceUpdate); err != nil {
					return false
				}

				// Condition for inferenceserviceready should be true
				if !tmInstanceUpdate.Status.IsConditionReady(v1alpha1api.InferenceServiceReady) {
					return false
				}

				// Condition for IsMMSPredictor should be true
				return !tmInstanceUpdate.Status.IsConditionReady(v1alpha1api.IsMMSPredictor)

			}, timeout).Should(BeTrue())

		})
	})
})

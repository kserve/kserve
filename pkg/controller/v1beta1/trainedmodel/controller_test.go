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

package trainedmodel

import (
	"context"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1beta1"
	"github.com/kubeflow/kfserving/pkg/constants"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"time"
)

var _ = Describe("v1beta1 TrainedModel controller", func() {
	// Define utility constants for object names and testing timeouts/durations and intervals.
	const (
		timeout  = time.Second * 10
		duration = time.Second * 10
		interval = time.Millisecond * 250
	)

	namespace := "test"
	modelName := "model1"
	parentInferenceService := "parent"
	storageUri := "s3//model1"
	framework := "pytorch"
	memory, _ := resource.ParseQuantity("1G")
	shardId := 0
	modelConfigName := constants.ModelConfigName(parentInferenceService, shardId)
	configmapKey := types.NamespacedName{Name: modelConfigName, Namespace: namespace}
	tmKey := types.NamespacedName{Name: modelName, Namespace: namespace}


	Context("When creating a new TrainedModel", func() {
		It("Should add a model to the model configmap", func() {
			emptyModelConfig := &v1.ConfigMap{
				TypeMeta: metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
				ObjectMeta: metav1.ObjectMeta{Name: modelConfigName, Namespace: namespace},
				Data: map[string]string{
					constants.ModelConfigFileName: "",
				},
			}

			tmInstance := &v1beta1.TrainedModel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      modelName,
					Namespace: namespace,
				},
				Spec: v1beta1.TrainedModelSpec{
					InferenceService: parentInferenceService,
					Inference: v1beta1.ModelSpec{
						StorageURI: storageUri,
						Framework:  framework,
						Memory:     memory,
					},
				},
			}

			Expect(k8sClient.Create(context.TODO(), emptyModelConfig)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), emptyModelConfig)
			Expect(k8sClient.Create(context.TODO(), tmInstance)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), tmInstance)

			// Verify that the model configmap is updated with the TrainedModel
			configmapActual := &v1.ConfigMap{}
			tmActual := &v1beta1.TrainedModel{}
			Eventually(func() bool {
				ctx := context.Background()
				err := k8sClient.Get(ctx, configmapKey, configmapActual)
				if err != nil {
					return false
				}
				err = k8sClient.Get(ctx, tmKey, tmActual)
				if err != nil {
					return false
				}
				expected := &v1.ConfigMap{
					TypeMeta:   metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
					ObjectMeta: metav1.ObjectMeta{Name: modelConfigName, Namespace: namespace},
					Data: map[string]string{
						constants.ModelConfigFileName: `[{"modelName":"model1","modelSpec":{"storageUri":"s3//model1","framework":"pytorch","memory":"1G"}}]`,
					},
				}
				return Expect(expected).To(Equal(configmapActual))
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("When updating a TrainedModel", func() {
		It("Should update a model to the model configmap", func() {
			tmInstance := &v1beta1.TrainedModel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      modelName,
					Namespace: namespace,
				},
				Spec: v1beta1.TrainedModelSpec{
					InferenceService: parentInferenceService,
					Inference: v1beta1.ModelSpec{
						StorageURI: storageUri,
						Framework:  framework,
						Memory:     memory,
					},
				},
			}

			modelConfig := &v1.ConfigMap{
				TypeMeta: metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
				ObjectMeta: metav1.ObjectMeta{Name: modelConfigName, Namespace: namespace},
				Data: map[string]string{
					constants.ModelConfigFileName: "",
				},
			}

			Expect(k8sClient.Create(context.TODO(), modelConfig)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), modelConfig)
			Expect(k8sClient.Create(context.TODO(), tmInstance)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), tmInstance)
			tmInstanceUpdate := &v1beta1.TrainedModel{}
			Eventually(func() bool {
				if err := k8sClient.Get(context.TODO(), tmKey, tmInstanceUpdate); err != nil {
					return false
				}
				return true
			}, timeout).Should(BeTrue())

			updatedModelUri := "s3//model2"
			tmInstanceUpdate.Spec.Inference.StorageURI = updatedModelUri
			Expect(k8sClient.Update(context.TODO(), tmInstanceUpdate)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), tmInstanceUpdate)

			// Verify that the model configmap is updated with the TrainedModel
			configmapActual := &v1.ConfigMap{}
			tmActual := &v1beta1.TrainedModel{}
			Eventually(func() bool {
				ctx := context.Background()
				err := k8sClient.Get(ctx, configmapKey, configmapActual)
				if err != nil {
					return false
				}
				err = k8sClient.Get(ctx, tmKey, tmActual)
				if err != nil {
					return false
				}
				expected := &v1.ConfigMap{
					TypeMeta:   metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
					ObjectMeta: metav1.ObjectMeta{Name: modelConfigName, Namespace: namespace},
					Data: map[string]string{
						constants.ModelConfigFileName: `[{"modelName":"model1","modelSpec":{"storageUri":"s3//model2","framework":"pytorch","memory":"1G"}}]`,
					},
				}
				return Expect(expected).To(Equal(configmapActual))
			}, timeout, interval).Should(BeTrue())		})
	})

	Context("When deleting a TrainedModel", func() {
		It("Should remove a model to the multi-model configmap", func() {
			tmInstance := &v1beta1.TrainedModel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      modelName,
					Namespace: namespace,
				},
				Spec: v1beta1.TrainedModelSpec{
					InferenceService: parentInferenceService,
					Inference: v1beta1.ModelSpec{
						StorageURI: storageUri,
						Framework:  framework,
						Memory:     memory,
					},
				},
			}

			modelConfig := &v1.ConfigMap{
				TypeMeta: metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
				ObjectMeta: metav1.ObjectMeta{Name: modelConfigName, Namespace: namespace},
				Data: map[string]string{
					constants.ModelConfigFileName: "",
				},
			}

			Expect(k8sClient.Create(context.TODO(), modelConfig)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), modelConfig)
			Expect(k8sClient.Create(context.TODO(), tmInstance)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), tmInstance)
			tmCreated := &v1beta1.TrainedModel{}
			Eventually(func() bool {
				if err := k8sClient.Get(context.TODO(), tmKey, tmCreated); err != nil {
					return false
				}
				return true
			}, timeout).Should(BeTrue())
			Expect(k8sClient.Delete(context.TODO(), tmCreated)).NotTo(HaveOccurred())

			// Verify that the model configmap is updated with the TrainedModel
			configmapActual := &v1.ConfigMap{}
			tmActual := &v1beta1.TrainedModel{}
			Eventually(func() bool {
				ctx := context.Background()
				err := k8sClient.Get(ctx, configmapKey, configmapActual)
				if err != nil {
					return false
				}
				err = k8sClient.Get(ctx, tmKey, tmActual)
				if err != nil {
					return false
				}
				expected := &v1.ConfigMap{
					TypeMeta:   metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
					ObjectMeta: metav1.ObjectMeta{Name: modelConfigName, Namespace: namespace},
					Data: map[string]string{
						constants.ModelConfigFileName: "",
					},
				}
				return Expect(expected).To(Equal(configmapActual))
			}, timeout, interval).Should(BeTrue())		})
	})
})

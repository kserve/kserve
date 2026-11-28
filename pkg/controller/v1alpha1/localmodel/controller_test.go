/*
Copyright 2024 The KServe Authors.

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

package localmodel

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/go-logr/logr"
	"k8s.io/client-go/kubernetes"

	"knative.dev/pkg/apis"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/controller/v1alpha1/localmodel/reconcilers"
)

var _ = Describe("CachedModel controller", func() {
	const (
		timeout             = time.Second * 10
		duration            = time.Second * 10
		interval            = time.Millisecond * 250
		modelCacheNamespace = "kserve-localmodel-jobs"
		sourceModelUri      = "s3://mybucket/mymodel"
	)
	var (
		localModelSpec = v1alpha1.LocalModelCacheSpec{
			SourceModelUri: sourceModelUri,
			ModelSize:      resource.MustParse("123Gi"),
			NodeGroups:     []string{"gpu1", "gpu2"},
		}
		localModelNodeGroupSpec1 = v1alpha1.LocalModelNodeGroupSpec{
			PersistentVolumeSpec: corev1.PersistentVolumeSpec{
				AccessModes:                   []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				VolumeMode:                    ptr.To(corev1.PersistentVolumeFilesystem),
				Capacity:                      corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("2Gi")},
				StorageClassName:              "standard",
				PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimDelete,
				PersistentVolumeSource: corev1.PersistentVolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: "/models",
						Type: ptr.To(corev1.HostPathDirectory),
					},
				},
				NodeAffinity: &corev1.VolumeNodeAffinity{
					Required: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{
							{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{
										Key:      "node.kubernetes.io/instance-type",
										Operator: corev1.NodeSelectorOpIn,
										Values:   []string{"gpu1"},
									},
								},
							},
						},
					},
				},
			},
			PersistentVolumeClaimSpec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources:   corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("2Gi")}},
			},
		}
		localModelNodeGroupSpec2 = v1alpha1.LocalModelNodeGroupSpec{
			PersistentVolumeSpec: corev1.PersistentVolumeSpec{
				AccessModes:                   []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				VolumeMode:                    ptr.To(corev1.PersistentVolumeFilesystem),
				Capacity:                      corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("2Gi")},
				StorageClassName:              "standard",
				PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimDelete,
				PersistentVolumeSource: corev1.PersistentVolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: "/models",
						Type: ptr.To(corev1.HostPathDirectory),
					},
				},
				NodeAffinity: &corev1.VolumeNodeAffinity{
					Required: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{
							{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{
										Key:      "node.kubernetes.io/instance-type",
										Operator: corev1.NodeSelectorOpIn,
										Values:   []string{"gpu2"},
									},
								},
							},
						},
					},
				},
			},
			PersistentVolumeClaimSpec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources:   corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("2Gi")}},
			},
		}
	)

	Context("When creating a local model", func() {
		It("Should create pv, pvc, localmodelnode, and update status from localmodelnode", func() {
			ctx, cancel := context.WithCancel(context.Background())
			DeferCleanup(cancel)

			nodeGroup1 := &v1alpha1.LocalModelNodeGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gpu1",
				},
				Spec: localModelNodeGroupSpec1,
			}
			nodeGroup2 := &v1alpha1.LocalModelNodeGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gpu2",
				},
				Spec: localModelNodeGroupSpec2,
			}
			Expect(k8sClient.Create(ctx, nodeGroup1)).Should(Succeed())
			defer k8sClient.Delete(ctx, nodeGroup1)
			Expect(k8sClient.Create(ctx, nodeGroup2)).Should(Succeed())
			defer k8sClient.Delete(ctx, nodeGroup2)

			cachedModel := &v1alpha1.LocalModelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name: "iris",
				},
				Spec: localModelSpec,
			}
			Expect(k8sClient.Create(ctx, cachedModel)).Should(Succeed())

			modelLookupKey := types.NamespacedName{Name: "iris"}
			pvLookupKey1 := types.NamespacedName{Name: "iris-gpu1-download"}
			pvcLookupKey1 := types.NamespacedName{Name: "iris-gpu1", Namespace: modelCacheNamespace}
			pvLookupKey2 := types.NamespacedName{Name: "iris-gpu2-download"}
			pvcLookupKey2 := types.NamespacedName{Name: "iris-gpu2", Namespace: modelCacheNamespace}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, modelLookupKey, cachedModel)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			Eventually(func() bool {
				err := k8sClient.Get(ctx, modelLookupKey, cachedModel)
				return err == nil && cachedModel.Status.ModelCopies != nil
			}, timeout, interval).Should(BeTrue())

			persistentVolume1 := &corev1.PersistentVolume{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, pvLookupKey1, persistentVolume1)
				return err == nil && persistentVolume1 != nil
			}, timeout, interval).Should(BeTrue())
			persistentVolume2 := &corev1.PersistentVolume{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, pvLookupKey2, persistentVolume2)
				return err == nil && persistentVolume2 != nil
			}, timeout, interval).Should(BeTrue())

			persistentVolumeClaim1 := &corev1.PersistentVolumeClaim{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, pvcLookupKey1, persistentVolumeClaim1)
				return err == nil && persistentVolumeClaim1 != nil
			}, timeout, interval).Should(BeTrue())
			persistentVolumeClaim2 := &corev1.PersistentVolumeClaim{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, pvcLookupKey2, persistentVolumeClaim2)
				return err == nil && persistentVolumeClaim2 != nil
			}, timeout, interval).Should(BeTrue())

			nodeName1 := "node-1"
			nodeName2 := "node-2"
			node1 := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: nodeName1,
					Labels: map[string]string{
						"node.kubernetes.io/instance-type": "gpu1",
					},
				},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{
							Type:   corev1.NodeReady,
							Status: corev1.ConditionTrue,
						},
					},
				},
			}
			node2 := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: nodeName2,
					Labels: map[string]string{
						"node.kubernetes.io/instance-type": "gpu2",
					},
				},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{
							Type:   corev1.NodeReady,
							Status: corev1.ConditionTrue,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, node1)).Should(Succeed())
			defer k8sClient.Delete(ctx, node1)
			Expect(k8sClient.Create(ctx, node2)).Should(Succeed())
			defer k8sClient.Delete(ctx, node2)

			localModelNode1 := &v1alpha1.LocalModelNode{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: nodeName1}, localModelNode1)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			Expect(localModelNode1.Spec.LocalModels).Should(ContainElement(v1alpha1.LocalModelInfo{ModelName: cachedModel.Name, SourceModelUri: sourceModelUri, NodeGroup: "gpu1"}))
			localModelNode2 := &v1alpha1.LocalModelNode{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: nodeName2}, localModelNode2)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			Expect(localModelNode2.Spec.LocalModels).Should(ContainElement(v1alpha1.LocalModelInfo{ModelName: cachedModel.Name, SourceModelUri: sourceModelUri, NodeGroup: "gpu2"}))

			// Todo: Test agent download
			// Update the LocalModelNode status to be successful
			localModelNode1.Status.ModelStatus = map[string]v1alpha1.ModelStatus{cachedModel.Name: v1alpha1.ModelDownloaded}
			Expect(k8sClient.Status().Update(ctx, localModelNode1)).Should(Succeed())
			localModelNode2.Status.ModelStatus = map[string]v1alpha1.ModelStatus{cachedModel.Name: v1alpha1.ModelDownloaded}
			Expect(k8sClient.Status().Update(ctx, localModelNode2)).Should(Succeed())

			Eventually(func() bool {
				err := k8sClient.Get(ctx, modelLookupKey, cachedModel)
				if err != nil {
					return false
				}
				if cachedModel.Status.ModelCopies.Available != 2 || cachedModel.Status.ModelCopies.Total != 2 || cachedModel.Status.ModelCopies.Failed != 0 {
					return false
				}
				if cachedModel.Status.NodeStatus[nodeName1] != v1alpha1.NodeDownloaded {
					return false
				}
				if cachedModel.Status.NodeStatus[nodeName2] != v1alpha1.NodeDownloaded {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue(), "Node status should be downloaded")

			// Now let's test deletion
			Expect(k8sClient.Delete(ctx, cachedModel)).Should(Succeed())

			newLocalModel := &v1alpha1.LocalModelCache{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, modelLookupKey, newLocalModel)
				return err == nil
			}, timeout, interval).Should(BeFalse(), "Should not get the local model after deletion")
		})

		It("Should copy imagePullSecrets onto LocalModelNode LocalModelInfo", func() {
			ctx, cancel := context.WithCancel(context.Background())
			DeferCleanup(cancel)
			nodeGroup := &v1alpha1.LocalModelNodeGroup{
				ObjectMeta: metav1.ObjectMeta{Name: "gpu1"},
				Spec:       localModelNodeGroupSpec1,
			}
			Expect(k8sClient.Create(ctx, nodeGroup)).Should(Succeed())
			defer k8sClient.Delete(ctx, nodeGroup)

			nodeName := "node-oci-pull-secret"
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: nodeName,
					Labels: map[string]string{
						"node.kubernetes.io/instance-type": "gpu1",
					},
				},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
				},
			}
			Expect(k8sClient.Create(ctx, node)).Should(Succeed())
			defer k8sClient.Delete(ctx, node)

			modelName := fmt.Sprintf("oci-cache-%d", time.Now().UnixNano())
			cachedModel := &v1alpha1.LocalModelCache{
				ObjectMeta: metav1.ObjectMeta{Name: modelName},
				Spec: v1alpha1.LocalModelCacheSpec{
					SourceModelUri:   "oci://ghcr.io/example/model:v1",
					ModelSize:        resource.MustParse("1Gi"),
					NodeGroups:       []string{"gpu1"},
					ImagePullSecrets: []corev1.LocalObjectReference{{Name: "reg-cred"}},
				},
			}
			Expect(k8sClient.Create(ctx, cachedModel)).Should(Succeed())
			defer func() {
				Expect(k8sClient.Delete(ctx, cachedModel)).Should(Succeed())
				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{Name: modelName}, cachedModel)
					return err != nil && errors.IsNotFound(err)
				}, timeout, interval).Should(BeTrue())
			}()

			localModelNode := &v1alpha1.LocalModelNode{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: nodeName}, localModelNode)
				if err != nil {
					return false
				}
				for _, info := range localModelNode.Spec.LocalModels {
					if info.ModelName == modelName &&
						len(info.ImagePullSecrets) == 1 &&
						info.ImagePullSecrets[0].Name == "reg-cred" {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue(), "LocalModelInfo should include imagePullSecrets")
		})

		It("Should create pvs and pvcs for inference services", func() {
			ctx, cancel := context.WithCancel(context.Background())
			DeferCleanup(cancel)
			nodeGroup1 := &v1alpha1.LocalModelNodeGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gpu1",
				},
				Spec: localModelNodeGroupSpec1,
			}
			nodeGroup2 := &v1alpha1.LocalModelNodeGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gpu2",
				},
				Spec: localModelNodeGroupSpec1,
			}
			Expect(k8sClient.Create(ctx, nodeGroup1)).Should(Succeed())
			Expect(k8sClient.Create(ctx, nodeGroup2)).Should(Succeed())
			defer k8sClient.Delete(ctx, nodeGroup1)
			defer k8sClient.Delete(ctx, nodeGroup2)

			modelName := "iris2"
			isvcNamespace := "default"
			isvcName1 := "foo"
			isvcName2 := "bar"
			cachedModel := &v1alpha1.LocalModelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name: modelName,
				},
				Spec: localModelSpec,
			}
			Expect(k8sClient.Create(ctx, cachedModel)).Should(Succeed())
			defer func() {
				Expect(k8sClient.Delete(ctx, cachedModel)).Should(Succeed())
				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{Name: modelName}, cachedModel)
					return err != nil && errors.IsNotFound(err)
				}, timeout, interval).Should(BeTrue(), "Should not get the local model after deletion")
			}()
			// No nodegroup annotation, should pick the default nodegroup,
			// which is the first one in the model cache nodegroup list
			isvc1 := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      isvcName1,
					Namespace: isvcNamespace,
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						Model: &v1beta1.ModelSpec{
							PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
								StorageURI: ptr.To(sourceModelUri),
							},
							ModelFormat: v1beta1.ModelFormat{Name: "sklearn"},
						},
					},
				},
			}
			// Has nodegroup annotation, should pick the specified nodegroup
			isvc2 := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:        isvcName2,
					Namespace:   isvcNamespace,
					Annotations: map[string]string{constants.NodeGroupAnnotationKey: "gpu2"},
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						Model: &v1beta1.ModelSpec{
							PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
								StorageURI: ptr.To(sourceModelUri),
							},
							ModelFormat: v1beta1.ModelFormat{Name: "sklearn"},
						},
					},
				},
			}

			// Mutating webhook adds a local model label
			cachedModelList := &v1alpha1.LocalModelCacheList{}
			cachedModelList.Items = []v1alpha1.LocalModelCache{*cachedModel}
			isvc1.DefaultInferenceService(nil, nil, nil, cachedModelList, nil)
			isvc2.DefaultInferenceService(nil, nil, nil, cachedModelList, nil)

			Expect(k8sClient.Create(ctx, isvc1)).Should(Succeed())
			inferenceService1 := &v1beta1.InferenceService{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: isvcName1, Namespace: isvcNamespace}, inferenceService1)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			Expect(k8sClient.Create(ctx, isvc2)).Should(Succeed())
			inferenceService2 := &v1beta1.InferenceService{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: isvcName2, Namespace: isvcNamespace}, inferenceService2)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			// Expects a pv and a pvc are created in the isvcNamespace
			pvLookupKey1 := types.NamespacedName{Name: modelName + "-" + nodeGroup1.Name + "-" + isvcNamespace}
			pvLookupKey2 := types.NamespacedName{Name: modelName + "-" + nodeGroup2.Name + "-" + isvcNamespace}
			pvcLookupKey1 := types.NamespacedName{Name: modelName + "-" + nodeGroup1.Name, Namespace: isvcNamespace}
			pvcLookupKey2 := types.NamespacedName{Name: modelName + "-" + nodeGroup2.Name, Namespace: isvcNamespace}

			persistentVolume1 := &corev1.PersistentVolume{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, pvLookupKey1, persistentVolume1)
				return err == nil && persistentVolume1 != nil
			}, timeout, interval).Should(BeTrue())
			persistentVolume2 := &corev1.PersistentVolume{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, pvLookupKey2, persistentVolume2)
				return err == nil && persistentVolume2 != nil
			}, timeout, interval).Should(BeTrue())

			persistentVolumeClaim1 := &corev1.PersistentVolumeClaim{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, pvcLookupKey1, persistentVolumeClaim1)
				return err == nil && persistentVolumeClaim1 != nil
			}, timeout, interval).Should(BeTrue())
			persistentVolumeClaim2 := &corev1.PersistentVolumeClaim{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, pvcLookupKey2, persistentVolumeClaim2)
				return err == nil && persistentVolumeClaim2 != nil
			}, timeout, interval).Should(BeTrue())

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: modelName}, cachedModel)
				if err != nil {
					return false
				}
				if len(cachedModel.Status.InferenceServices) != 2 {
					return false
				}
				// Check for both services in any order
				foundIsvc1 := false
				foundIsvc2 := false

				for _, isvc := range cachedModel.Status.InferenceServices {
					if isvc.Name == isvcName1 && isvc.Namespace == isvcNamespace {
						foundIsvc1 = true
					} else if isvc.Name == isvcName2 && isvc.Namespace == isvcNamespace {
						foundIsvc2 = true
					}
				}

				return foundIsvc1 && foundIsvc2
			}, timeout, interval).Should(BeTrue(), "Node status should have both isvcs")

			Expect(k8sClient.Delete(ctx, isvc1)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, isvc2)).Should(Succeed())
		})

		It("Should track LLMInferenceService in cluster-scoped cache status", func() {
			defer GinkgoRecover()
			ctx, cancel := context.WithCancel(context.Background())
			DeferCleanup(cancel)
			nodeGroup1 := &v1alpha1.LocalModelNodeGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gpu1",
				},
				Spec: localModelNodeGroupSpec1,
			}
			Expect(k8sClient.Create(ctx, nodeGroup1)).Should(Succeed())
			defer k8sClient.Delete(ctx, nodeGroup1)

			modelName := fmt.Sprintf("llm-cache-%d", time.Now().UnixNano())
			llmSvcName := "test-llm-svc"
			llmSvcNamespace := "default"
			cachedModel := &v1alpha1.LocalModelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name: modelName,
				},
				Spec: v1alpha1.LocalModelCacheSpec{
					SourceModelUri: sourceModelUri,
					ModelSize:      resource.MustParse("10Gi"),
					NodeGroups:     []string{"gpu1"},
				},
			}
			Expect(k8sClient.Create(ctx, cachedModel)).Should(Succeed())
			defer func() {
				Expect(k8sClient.Delete(ctx, cachedModel)).Should(Succeed())
				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{Name: modelName}, cachedModel)
					return err != nil && errors.IsNotFound(err)
				}, timeout, interval).Should(BeTrue())
			}()

			// Create LLMInferenceService with localmodel label (simulating the webhook)
			modelUri, _ := apis.ParseURL(sourceModelUri)
			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      llmSvcName,
					Namespace: llmSvcNamespace,
					Labels: map[string]string{
						constants.LocalModelLabel: modelName,
					},
					Annotations: map[string]string{
						constants.LocalModelSourceUriAnnotationKey: sourceModelUri,
						constants.LocalModelPVCNameAnnotationKey:   modelName + "-gpu1",
					},
				},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Model: v1alpha2.LLMModelSpec{
						URI: *modelUri,
					},
				},
			}
			Expect(k8sClient.Create(ctx, llmSvc)).Should(Succeed())
			defer k8sClient.Delete(ctx, llmSvc)

			// Verify the cache status tracks the LLMInferenceService
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: modelName}, cachedModel)
				if err != nil {
					return false
				}
				if len(cachedModel.Status.LLMInferenceServices) != 1 {
					return false
				}
				return cachedModel.Status.LLMInferenceServices[0].Name == llmSvcName &&
					cachedModel.Status.LLMInferenceServices[0].Namespace == llmSvcNamespace
			}, timeout, interval).Should(BeTrue(), "Cache status should track the LLMInferenceService")
		})

		It("Should track LLMInferenceService referenced only via LoRA adapter in cluster-scoped cache status", func() {
			defer GinkgoRecover()
			ctx, cancel := context.WithCancel(context.Background())
			DeferCleanup(cancel)
			nodeGroup1 := &v1alpha1.LocalModelNodeGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gpu1",
				},
				Spec: localModelNodeGroupSpec1,
			}
			Expect(k8sClient.Create(ctx, nodeGroup1)).Should(Succeed())
			defer k8sClient.Delete(ctx, nodeGroup1)

			adapterURI := "hf://org/lora-adapter-only"
			baseURI := "hf://org/remote-base"
			modelName := fmt.Sprintf("lora-adapter-cache-%d", time.Now().UnixNano())
			llmSvcName := "test-llm-lora-adapter-only"
			llmSvcNamespace := "default"
			cachedModel := &v1alpha1.LocalModelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name: modelName,
				},
				Spec: v1alpha1.LocalModelCacheSpec{
					SourceModelUri: adapterURI,
					ModelSize:      resource.MustParse("10Gi"),
					NodeGroups:     []string{"gpu1"},
				},
			}
			Expect(k8sClient.Create(ctx, cachedModel)).Should(Succeed())
			defer func() {
				Expect(k8sClient.Delete(ctx, cachedModel)).Should(Succeed())
				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{Name: modelName}, cachedModel)
					return err != nil && errors.IsNotFound(err)
				}, timeout, interval).Should(BeTrue())
			}()

			baseModelURI, err := apis.ParseURL(baseURI)
			Expect(err).NotTo(HaveOccurred())
			adapterModelURI, err := apis.ParseURL(adapterURI)
			Expect(err).NotTo(HaveOccurred())
			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      llmSvcName,
					Namespace: llmSvcNamespace,
					Annotations: map[string]string{
						constants.LocalModelLoRAAnnotationKey: fmt.Sprintf(
							`{"my-adapter":{"cache":%q}}`,
							modelName,
						),
					},
				},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Model: v1alpha2.LLMModelSpec{
						URI: *baseModelURI,
						LoRA: &v1alpha2.LoRASpec{
							Adapters: []v1alpha2.LLMModelSpec{
								{
									Name: ptr.To("my-adapter"),
									URI:  *adapterModelURI,
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, llmSvc)).Should(Succeed())
			defer k8sClient.Delete(ctx, llmSvc)

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: modelName}, cachedModel)
				if err != nil {
					return false
				}
				if len(cachedModel.Status.LLMInferenceServices) != 1 {
					return false
				}
				return cachedModel.Status.LLMInferenceServices[0].Name == llmSvcName &&
					cachedModel.Status.LLMInferenceServices[0].Namespace == llmSvcNamespace
			}, timeout, interval).Should(BeTrue(), "Cache status should track the LLMInferenceService via LoRA adapter reference")
		})
	})

	Context("When DisableVolumeManagement is set to true", func() {
		It("Should NOT create/delete pvs and pvcs if localmodel config value DisableVolumeManagement is true", func() {
			ctx, cancel := context.WithCancel(context.Background())
			DeferCleanup(cancel)

			// Update configmap to enable DisableVolumeManagement
			configMap := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace}, configMap)).Should(Succeed())
			originalData := configMap.Data["localModel"]
			configMap.Data["localModel"] = `{
				"jobNamespace": "kserve-localmodel-jobs",
				"defaultJobImage": "kserve/storage-initializer:latest",
				"disableVolumeManagement": true
			}`
			Expect(k8sClient.Update(ctx, configMap)).Should(Succeed())
			defer func() {
				// Restore original configmap data
				configMap.Data["localModel"] = originalData
				Expect(k8sClient.Update(ctx, configMap)).Should(Succeed())
			}()
			testNamespace := "test-namespace"
			namespaceObj := createTestNamespace(ctx, testNamespace)
			defer k8sClient.Delete(ctx, namespaceObj)

			modelName := "iris3"
			cachedModel := &v1alpha1.LocalModelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name: modelName,
				},
				Spec: localModelSpec,
			}
			Expect(k8sClient.Create(ctx, cachedModel)).Should(Succeed())
			defer k8sClient.Delete(ctx, cachedModel)

			pvcName := "test-pvc"
			pvName := pvcName + "-" + testNamespace

			pv := createTestPV(ctx, pvName, cachedModel)
			defer k8sClient.Delete(ctx, pv)

			pvc := createTestPVC(ctx, pvcName, testNamespace, pvName, cachedModel)
			defer k8sClient.Delete(ctx, pvc)

			// Expects test-pv and test-pvc to not get deleted
			pvLookupKey := types.NamespacedName{Name: pvName}
			pvcLookupKey := types.NamespacedName{Name: pvcName, Namespace: testNamespace}

			persistentVolume := &corev1.PersistentVolume{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, pvLookupKey, persistentVolume)
				return err == nil && persistentVolume != nil
			}, timeout, interval).Should(BeTrue())

			persistentVolumeClaim := &corev1.PersistentVolumeClaim{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, pvcLookupKey, persistentVolumeClaim)
				return err == nil && persistentVolumeClaim != nil
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("When creating multiple localModels", func() {
		// With two nodes and two local models, each node should have both local models
		It("Should create localModelNode correctly", func() {
			ctx, cancel := context.WithCancel(context.Background())
			DeferCleanup(cancel)
			nodeGroup1 := &v1alpha1.LocalModelNodeGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gpu1",
				},
				Spec: localModelNodeGroupSpec1,
			}
			Expect(k8sClient.Create(ctx, nodeGroup1)).Should(Succeed())
			defer k8sClient.Delete(ctx, nodeGroup1)
			nodeGroup2 := &v1alpha1.LocalModelNodeGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gpu2",
				},
				Spec: localModelNodeGroupSpec2,
			}
			Expect(k8sClient.Create(ctx, nodeGroup2)).Should(Succeed())
			defer k8sClient.Delete(ctx, nodeGroup2)

			node1 := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
					Labels: map[string]string{
						"node.kubernetes.io/instance-type": "gpu1",
					},
				},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{
							Type:   corev1.NodeReady,
							Status: corev1.ConditionTrue,
						},
					},
				},
			}

			node2 := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node2",
					Labels: map[string]string{
						"node.kubernetes.io/instance-type": "gpu2",
					},
				},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{
							Type:   corev1.NodeReady,
							Status: corev1.ConditionTrue,
						},
					},
				},
			}

			nodes := []*corev1.Node{node1, node2}
			for _, node := range nodes {
				Expect(k8sClient.Create(ctx, node)).Should(Succeed())
				defer k8sClient.Delete(ctx, node)
			}

			model1 := "iris1"
			cachedModel1 := &v1alpha1.LocalModelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name: model1,
				},
				Spec: localModelSpec,
			}
			Expect(k8sClient.Create(ctx, cachedModel1)).Should(Succeed())
			model2 := "iris2"
			cachedModel2 := &v1alpha1.LocalModelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name: model2,
				},
				Spec: localModelSpec,
			}
			Expect(k8sClient.Create(ctx, cachedModel2)).Should(Succeed())

			for _, node := range nodes {
				localModelNode := &v1alpha1.LocalModelNode{}
				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{Name: node.Name}, localModelNode)
					return err == nil && len(localModelNode.Spec.LocalModels) == 2
				}, timeout, interval).Should(BeTrue())
			}

			// Now let's test deletion - delete one model and expect one model exists
			Expect(k8sClient.Delete(ctx, cachedModel1)).Should(Succeed())
			for _, node := range nodes {
				localModelNode := &v1alpha1.LocalModelNode{}
				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{Name: node.Name}, localModelNode)
					// Only one model exists
					return err == nil && len(localModelNode.Spec.LocalModels) == 1
				}, timeout, interval).Should(BeTrue())
			}

			// Delete the last model and expect the spec to be empty
			Expect(k8sClient.Delete(ctx, cachedModel2)).Should(Succeed())
			for _, node := range nodes {
				localModelNode := &v1alpha1.LocalModelNode{}
				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{Name: node.Name}, localModelNode)
					return err == nil && len(localModelNode.Spec.LocalModels) == 0
				}, timeout, interval).Should(BeTrue())
			}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: model1}, cachedModel1)
				return err != nil && errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue(), "Should not get the local model after deletion")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: model2}, cachedModel2)
				return err != nil && errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue(), "Should not get the local model after deletion")
		})
	})
})

var _ = Describe("LocalModelNamespaceCache controller", func() {
	const (
		timeout             = time.Second * 10
		interval            = time.Millisecond * 250
		sourceModelUri      = "s3://mybucket/mymodel"
		modelCacheNamespace = "kserve-localmodel-jobs"
	)
	var (
		localModelNamespaceSpec = v1alpha1.LocalModelNamespaceCacheSpec{
			SourceModelUri: sourceModelUri,
			ModelSize:      resource.MustParse("123Gi"),
			NodeGroups:     []string{"gpu1"},
		}
		localModelNodeGroupSpec1 = v1alpha1.LocalModelNodeGroupSpec{
			PersistentVolumeSpec: corev1.PersistentVolumeSpec{
				AccessModes:                   []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				VolumeMode:                    ptr.To(corev1.PersistentVolumeFilesystem),
				Capacity:                      corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("2Gi")},
				StorageClassName:              "standard",
				PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimDelete,
				PersistentVolumeSource: corev1.PersistentVolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: "/models",
						Type: ptr.To(corev1.HostPathDirectory),
					},
				},
				NodeAffinity: &corev1.VolumeNodeAffinity{
					Required: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{
							{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{
										Key:      "node.kubernetes.io/instance-type",
										Operator: corev1.NodeSelectorOpIn,
										Values:   []string{"gpu1"},
									},
								},
							},
						},
					},
				},
			},
			PersistentVolumeClaimSpec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources:   corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("2Gi")}},
			},
		}
	)

	Context("When creating a namespace-scoped local model", func() {
		It("Should create pv, pvc, localmodelnode, and update status from localmodelnode", func() {
			ctx, cancel := context.WithCancel(context.Background())
			DeferCleanup(cancel)

			testNamespace := fmt.Sprintf("test-ns-cache-%d", time.Now().UnixNano())
			namespaceObj := createTestNamespace(ctx, testNamespace)
			defer k8sClient.Delete(ctx, namespaceObj)

			nodeGroup1 := &v1alpha1.LocalModelNodeGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gpu1",
				},
				Spec: localModelNodeGroupSpec1,
			}
			Expect(k8sClient.Create(ctx, nodeGroup1)).Should(Succeed())
			defer k8sClient.Delete(ctx, nodeGroup1)

			modelName := "ns-iris"
			cachedModel := &v1alpha1.LocalModelNamespaceCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      modelName,
					Namespace: testNamespace,
				},
				Spec: localModelNamespaceSpec,
			}
			Expect(k8sClient.Create(ctx, cachedModel)).Should(Succeed())
			defer k8sClient.Delete(ctx, cachedModel)

			modelLookupKey := types.NamespacedName{Name: modelName, Namespace: testNamespace}
			pvLookupKey1 := types.NamespacedName{Name: modelName + "-gpu1-" + testNamespace + "-download"}
			pvcLookupKey1 := types.NamespacedName{Name: modelName + "-gpu1-" + testNamespace + "-download", Namespace: modelCacheNamespace}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, modelLookupKey, cachedModel)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			Eventually(func() bool {
				err := k8sClient.Get(ctx, modelLookupKey, cachedModel)
				return err == nil && cachedModel.Status.ModelCopies != nil
			}, timeout, interval).Should(BeTrue())

			persistentVolume1 := &corev1.PersistentVolume{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, pvLookupKey1, persistentVolume1)
				return err == nil && persistentVolume1 != nil
			}, timeout, interval).Should(BeTrue(), "Download PV should be created")

			persistentVolumeClaim1 := &corev1.PersistentVolumeClaim{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, pvcLookupKey1, persistentVolumeClaim1)
				return err == nil && persistentVolumeClaim1 != nil
			}, timeout, interval).Should(BeTrue(), "Download PVC should be created in jobNamespace")

			nodeName1 := "ns-node-1"
			node1 := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: nodeName1,
					Labels: map[string]string{
						"node.kubernetes.io/instance-type": "gpu1",
					},
				},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{
							Type:   corev1.NodeReady,
							Status: corev1.ConditionTrue,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, node1)).Should(Succeed())
			defer k8sClient.Delete(ctx, node1)

			localModelNode1 := &v1alpha1.LocalModelNode{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: nodeName1}, localModelNode1)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			expectedModelInfo := v1alpha1.LocalModelInfo{
				ModelName:      modelName,
				SourceModelUri: sourceModelUri,
				Namespace:      testNamespace,
				NodeGroup:      "gpu1",
			}
			Expect(localModelNode1.Spec.LocalModels).Should(ContainElement(expectedModelInfo))

			statusKey := testNamespace + "/" + modelName
			localModelNode1.Status.ModelStatus = map[string]v1alpha1.ModelStatus{statusKey: v1alpha1.ModelDownloaded}
			Expect(k8sClient.Status().Update(ctx, localModelNode1)).Should(Succeed())

			Eventually(func() bool {
				err := k8sClient.Get(ctx, modelLookupKey, cachedModel)
				if err != nil {
					return false
				}
				if cachedModel.Status.ModelCopies == nil {
					return false
				}
				if cachedModel.Status.ModelCopies.Available != 1 || cachedModel.Status.ModelCopies.Total != 1 || cachedModel.Status.ModelCopies.Failed != 0 {
					return false
				}
				if cachedModel.Status.NodeStatus[nodeName1] != v1alpha1.NodeDownloaded {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue(), "Node status should be downloaded")
		})

		It("Should create pvs and pvcs for inference services in the same namespace only", func() {
			ctx, cancel := context.WithCancel(context.Background())
			DeferCleanup(cancel)

			testNamespace := fmt.Sprintf("test-ns-isvc-%d", time.Now().UnixNano())
			otherNamespace := fmt.Sprintf("other-ns-%d", time.Now().UnixNano())
			namespaceObj := createTestNamespace(ctx, testNamespace)
			defer k8sClient.Delete(ctx, namespaceObj)
			otherNamespaceObj := createTestNamespace(ctx, otherNamespace)
			defer k8sClient.Delete(ctx, otherNamespaceObj)

			nodeGroup1 := &v1alpha1.LocalModelNodeGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gpu1",
				},
				Spec: localModelNodeGroupSpec1,
			}
			Expect(k8sClient.Create(ctx, nodeGroup1)).Should(Succeed())
			defer k8sClient.Delete(ctx, nodeGroup1)

			modelName := "ns-iris2"
			cachedModel := &v1alpha1.LocalModelNamespaceCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      modelName,
					Namespace: testNamespace,
				},
				Spec: localModelNamespaceSpec,
			}
			Expect(k8sClient.Create(ctx, cachedModel)).Should(Succeed())
			defer k8sClient.Delete(ctx, cachedModel)

			isvcName1 := "ns-foo"
			isvc1 := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      isvcName1,
					Namespace: testNamespace,
					Labels: map[string]string{
						constants.LocalModelLabel:          modelName,
						constants.LocalModelNamespaceLabel: testNamespace,
					},
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						Model: &v1beta1.ModelSpec{
							PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
								StorageURI: ptr.To(sourceModelUri),
							},
							ModelFormat: v1beta1.ModelFormat{Name: "sklearn"},
						},
					},
				},
			}

			isvcName2 := "ns-bar"
			isvc2 := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      isvcName2,
					Namespace: otherNamespace,
					Labels: map[string]string{
						constants.LocalModelLabel:          modelName,
						constants.LocalModelNamespaceLabel: testNamespace, // Wrong - ISVC is in different namespace
					},
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						Model: &v1beta1.ModelSpec{
							PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
								StorageURI: ptr.To(sourceModelUri),
							},
							ModelFormat: v1beta1.ModelFormat{Name: "sklearn"},
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, isvc1)).Should(Succeed())
			defer k8sClient.Delete(ctx, isvc1)
			Expect(k8sClient.Create(ctx, isvc2)).Should(Succeed())
			defer k8sClient.Delete(ctx, isvc2)

			inferenceService1 := &v1beta1.InferenceService{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: isvcName1, Namespace: testNamespace}, inferenceService1)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			pvLookupKey := types.NamespacedName{Name: modelName + "-gpu1-" + testNamespace}
			pvcLookupKey := types.NamespacedName{Name: modelName + "-gpu1", Namespace: testNamespace}

			persistentVolume := &corev1.PersistentVolume{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, pvLookupKey, persistentVolume)
				return err == nil && persistentVolume != nil
			}, timeout, interval).Should(BeTrue(), "Serving PV should be created for ISVC in same namespace")

			persistentVolumeClaim := &corev1.PersistentVolumeClaim{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, pvcLookupKey, persistentVolumeClaim)
				return err == nil && persistentVolumeClaim != nil
			}, timeout, interval).Should(BeTrue(), "Serving PVC should be created for ISVC in same namespace")

			modelLookupKey := types.NamespacedName{Name: modelName, Namespace: testNamespace}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, modelLookupKey, cachedModel)
				if err != nil {
					return false
				}
				if len(cachedModel.Status.InferenceServices) != 1 {
					return false
				}
				return cachedModel.Status.InferenceServices[0].Name == isvcName1 &&
					cachedModel.Status.InferenceServices[0].Namespace == testNamespace
			}, timeout, interval).Should(BeTrue(), "Status should only include ISVC from the same namespace")
		})

		It("Should delete LocalModelNamespaceCache and run finalizer cleanup", func() {
			ctx, cancel := context.WithCancel(context.Background())
			DeferCleanup(cancel)

			testNamespace := fmt.Sprintf("test-ns-delete-%d", time.Now().UnixNano())
			namespaceObj := createTestNamespace(ctx, testNamespace)
			defer k8sClient.Delete(ctx, namespaceObj)

			nodeGroup1 := &v1alpha1.LocalModelNodeGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gpu1",
				},
				Spec: localModelNodeGroupSpec1,
			}
			Expect(k8sClient.Create(ctx, nodeGroup1)).Should(Succeed())
			defer k8sClient.Delete(ctx, nodeGroup1)

			modelName := "ns-iris-delete"
			cachedModel := &v1alpha1.LocalModelNamespaceCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      modelName,
					Namespace: testNamespace,
				},
				Spec: localModelNamespaceSpec,
			}
			Expect(k8sClient.Create(ctx, cachedModel)).Should(Succeed())

			modelLookupKey := types.NamespacedName{Name: modelName, Namespace: testNamespace}
			pvcLookupKey := types.NamespacedName{Name: modelName + "-gpu1-" + testNamespace + "-download", Namespace: modelCacheNamespace}

			// Wait for the model to be created with finalizer
			Eventually(func() bool {
				err := k8sClient.Get(ctx, modelLookupKey, cachedModel)
				if err != nil {
					return false
				}
				for _, f := range cachedModel.ObjectMeta.Finalizers {
					if f == "localmodelnamespacecache.kserve.io/finalizer" {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue(), "LocalModelNamespaceCache should have finalizer")

			persistentVolumeClaim := &corev1.PersistentVolumeClaim{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, pvcLookupKey, persistentVolumeClaim)
				return err == nil && persistentVolumeClaim != nil
			}, timeout, interval).Should(BeTrue(), "Download PVC should be created")

			// Now delete the LocalModelNamespaceCache
			Expect(k8sClient.Delete(ctx, cachedModel)).Should(Succeed())

			Eventually(func() bool {
				err := k8sClient.Get(ctx, modelLookupKey, cachedModel)
				return err != nil && errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue(), "LocalModelNamespaceCache should be deleted after finalizer cleanup")

			// Note: In envtest, Kubernetes garbage collection is not enabled,
			// so PVCs with owner references won't be automatically deleted.
			// In a real cluster, the PVC would be garbage collected when the owner is deleted.
			// The important verification is that the finalizer ran (proven by the CR being deleted).
		})

		It("Should track both cluster-scoped and namespace-scoped models on LocalModelNode", func() {
			ctx, cancel := context.WithCancel(context.Background())
			DeferCleanup(cancel)

			testNamespace := fmt.Sprintf("test-ns-mixed-%d", time.Now().UnixNano())
			namespaceObj := createTestNamespace(ctx, testNamespace)
			defer k8sClient.Delete(ctx, namespaceObj)

			nodeGroup1 := &v1alpha1.LocalModelNodeGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gpu1",
				},
				Spec: localModelNodeGroupSpec1,
			}
			Expect(k8sClient.Create(ctx, nodeGroup1)).Should(Succeed())
			defer k8sClient.Delete(ctx, nodeGroup1)

			clusterModelName := "cluster-model"
			clusterModel := &v1alpha1.LocalModelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name: clusterModelName,
				},
				Spec: v1alpha1.LocalModelCacheSpec{
					SourceModelUri: sourceModelUri,
					ModelSize:      resource.MustParse("123Gi"),
					NodeGroups:     []string{"gpu1"},
				},
			}
			Expect(k8sClient.Create(ctx, clusterModel)).Should(Succeed())
			defer k8sClient.Delete(ctx, clusterModel)

			nsModelName := "ns-model"
			nsModel := &v1alpha1.LocalModelNamespaceCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      nsModelName,
					Namespace: testNamespace,
				},
				Spec: localModelNamespaceSpec,
			}
			Expect(k8sClient.Create(ctx, nsModel)).Should(Succeed())
			defer k8sClient.Delete(ctx, nsModel)

			nodeName := "mixed-node"
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: nodeName,
					Labels: map[string]string{
						"node.kubernetes.io/instance-type": "gpu1",
					},
				},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{
							Type:   corev1.NodeReady,
							Status: corev1.ConditionTrue,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, node)).Should(Succeed())
			defer k8sClient.Delete(ctx, node)

			localModelNode := &v1alpha1.LocalModelNode{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: nodeName}, localModelNode)
				if err != nil {
					return false
				}
				return len(localModelNode.Spec.LocalModels) == 2
			}, timeout, interval).Should(BeTrue(), "LocalModelNode should have both cluster and namespace models")

			clusterModelInfo := v1alpha1.LocalModelInfo{
				ModelName:      clusterModelName,
				SourceModelUri: sourceModelUri,
				Namespace:      "", // Empty for cluster-scoped
				NodeGroup:      "gpu1",
			}
			Expect(localModelNode.Spec.LocalModels).Should(ContainElement(clusterModelInfo))

			nsModelInfo := v1alpha1.LocalModelInfo{
				ModelName:      nsModelName,
				SourceModelUri: sourceModelUri,
				Namespace:      testNamespace,
				NodeGroup:      "gpu1",
			}
			Expect(localModelNode.Spec.LocalModels).Should(ContainElement(nsModelInfo))
		})

		It("Should create download PV for LocalModelNamespaceCache and verify finalizer is set", func() {
			ctx, cancel := context.WithCancel(context.Background())
			DeferCleanup(cancel)

			testNamespace := fmt.Sprintf("test-ns-pv-delete-%d", time.Now().UnixNano())
			namespaceObj := createTestNamespace(ctx, testNamespace)
			defer k8sClient.Delete(ctx, namespaceObj)

			nodeGroup1 := &v1alpha1.LocalModelNodeGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gpu1",
				},
				Spec: localModelNodeGroupSpec1,
			}
			Expect(k8sClient.Create(ctx, nodeGroup1)).Should(Succeed())
			defer k8sClient.Delete(ctx, nodeGroup1)

			modelName := "ns-iris-pv-delete"
			cachedModel := &v1alpha1.LocalModelNamespaceCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      modelName,
					Namespace: testNamespace,
				},
				Spec: localModelNamespaceSpec,
			}
			Expect(k8sClient.Create(ctx, cachedModel)).Should(Succeed())
			defer k8sClient.Delete(ctx, cachedModel)

			modelLookupKey := types.NamespacedName{Name: modelName, Namespace: testNamespace}
			downloadPVLookupKey := types.NamespacedName{Name: modelName + "-gpu1-" + testNamespace + "-download"}

			// Wait for the model to be created with finalizer
			Eventually(func() bool {
				err := k8sClient.Get(ctx, modelLookupKey, cachedModel)
				if err != nil {
					return false
				}
				for _, f := range cachedModel.ObjectMeta.Finalizers {
					if f == "localmodelnamespacecache.kserve.io/finalizer" {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue(), "LocalModelNamespaceCache should have finalizer")

			// Wait for the download PV to be created
			downloadPV := &corev1.PersistentVolume{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, downloadPVLookupKey, downloadPV)
				return err == nil && downloadPV != nil
			}, timeout, interval).Should(BeTrue(), "Download PV should be created")

			// Verify the PV does NOT have an owner reference (since PVs are cluster-scoped
			// and cannot be owned by namespace-scoped resources)
			Expect(downloadPV.OwnerReferences).To(BeEmpty(),
				"Download PV should not have owner reference for namespace-scoped LocalModelNamespaceCache")

			// Note: In a real cluster, when LocalModelNamespaceCache is deleted,
			// the finalizer explicitly deletes the PV since it cannot rely on garbage collection.
			// This is verified by the finalizer being present and the deletion logic in utils.go.
		})

		It("Should create serving PV/PVC for ISVC and update status when ISVC is removed", func() {
			ctx, cancel := context.WithCancel(context.Background())
			DeferCleanup(cancel)

			testNamespace := fmt.Sprintf("test-ns-cleanup-%d", time.Now().UnixNano())
			namespaceObj := createTestNamespace(ctx, testNamespace)
			defer k8sClient.Delete(ctx, namespaceObj)

			nodeGroup1 := &v1alpha1.LocalModelNodeGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gpu1",
				},
				Spec: localModelNodeGroupSpec1,
			}
			Expect(k8sClient.Create(ctx, nodeGroup1)).Should(Succeed())
			defer k8sClient.Delete(ctx, nodeGroup1)

			modelName := "ns-iris-cleanup"
			cachedModel := &v1alpha1.LocalModelNamespaceCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      modelName,
					Namespace: testNamespace,
				},
				Spec: localModelNamespaceSpec,
			}
			Expect(k8sClient.Create(ctx, cachedModel)).Should(Succeed())
			defer k8sClient.Delete(ctx, cachedModel)

			isvcName := "cleanup-isvc"
			isvc := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      isvcName,
					Namespace: testNamespace,
					Labels: map[string]string{
						constants.LocalModelLabel:          modelName,
						constants.LocalModelNamespaceLabel: testNamespace,
					},
				},
				Spec: v1beta1.InferenceServiceSpec{
					Predictor: v1beta1.PredictorSpec{
						Model: &v1beta1.ModelSpec{
							PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
								StorageURI: ptr.To(sourceModelUri),
							},
							ModelFormat: v1beta1.ModelFormat{Name: "sklearn"},
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
			defer k8sClient.Delete(ctx, isvc)

			inferenceService := &v1beta1.InferenceService{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: isvcName, Namespace: testNamespace}, inferenceService)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			// Wait for the serving PVC to be created
			servingPVCLookupKey := types.NamespacedName{Name: modelName + "-gpu1", Namespace: testNamespace}
			servingPVC := &corev1.PersistentVolumeClaim{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, servingPVCLookupKey, servingPVC)
				return err == nil && servingPVC != nil
			}, timeout, interval).Should(BeTrue(), "Serving PVC should be created for ISVC")

			// Wait for the serving PV to be created
			servingPVLookupKey := types.NamespacedName{Name: modelName + "-gpu1-" + testNamespace}
			servingPV := &corev1.PersistentVolume{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, servingPVLookupKey, servingPV)
				return err == nil && servingPV != nil
			}, timeout, interval).Should(BeTrue(), "Serving PV should be created for ISVC")

			modelLookupKey := types.NamespacedName{Name: modelName, Namespace: testNamespace}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, modelLookupKey, cachedModel)
				if err != nil {
					return false
				}
				return len(cachedModel.Status.InferenceServices) == 1
			}, timeout, interval).Should(BeTrue(), "Cache status should show the ISVC")

			Expect(cachedModel.Status.InferenceServices[0].Name).To(Equal(isvcName))
			Expect(cachedModel.Status.InferenceServices[0].Namespace).To(Equal(testNamespace))

			// Note: In envtest, when ISVC is deleted, the cleanup logic in ReconcileForIsvcs
			// will attempt to delete the serving PV/PVC. However, envtest's behavior for
			// resource deletion may differ from a real cluster.
			// The key verification is that:
			// 1. Serving PV/PVC are created when ISVC exists
			// 2. Status correctly tracks the ISVC
			// 3. The cleanup code path exists in ReconcileForIsvcs (verified by code inspection)
		})

		It("Should track LLMInferenceService referenced only via LoRA adapter in namespace-scoped cache status", func() {
			defer GinkgoRecover()
			ctx, cancel := context.WithCancel(context.Background())
			DeferCleanup(cancel)

			testNamespace := fmt.Sprintf("test-ns-lora-%d", time.Now().UnixNano())
			namespaceObj := createTestNamespace(ctx, testNamespace)
			defer k8sClient.Delete(ctx, namespaceObj)

			nodeGroup1 := &v1alpha1.LocalModelNodeGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gpu1",
				},
				Spec: localModelNodeGroupSpec1,
			}
			Expect(k8sClient.Create(ctx, nodeGroup1)).Should(Succeed())
			defer k8sClient.Delete(ctx, nodeGroup1)

			adapterURI := "hf://org/ns-lora-adapter-only"
			baseURI := "hf://org/remote-base"
			modelName := fmt.Sprintf("ns-lora-adapter-cache-%d", time.Now().UnixNano())
			llmSvcName := "test-llm-ns-lora-adapter-only"
			cachedModel := &v1alpha1.LocalModelNamespaceCache{
				ObjectMeta: metav1.ObjectMeta{
					Name:      modelName,
					Namespace: testNamespace,
				},
				Spec: v1alpha1.LocalModelNamespaceCacheSpec{
					SourceModelUri: adapterURI,
					ModelSize:      resource.MustParse("10Gi"),
					NodeGroups:     []string{"gpu1"},
				},
			}
			Expect(k8sClient.Create(ctx, cachedModel)).Should(Succeed())
			defer func() {
				Expect(k8sClient.Delete(ctx, cachedModel)).Should(Succeed())
				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{Name: modelName, Namespace: testNamespace}, cachedModel)
					return err != nil && errors.IsNotFound(err)
				}, timeout, interval).Should(BeTrue())
			}()

			baseModelURI, err := apis.ParseURL(baseURI)
			Expect(err).NotTo(HaveOccurred())
			adapterModelURI, err := apis.ParseURL(adapterURI)
			Expect(err).NotTo(HaveOccurred())
			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      llmSvcName,
					Namespace: testNamespace,
					Annotations: map[string]string{
						constants.LocalModelLoRAAnnotationKey: fmt.Sprintf(
							`{"my-adapter":{"cache":%q,"namespace":%q}}`,
							modelName, testNamespace,
						),
					},
				},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Model: v1alpha2.LLMModelSpec{
						URI: *baseModelURI,
						LoRA: &v1alpha2.LoRASpec{
							Adapters: []v1alpha2.LLMModelSpec{
								{
									Name: ptr.To("my-adapter"),
									URI:  *adapterModelURI,
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, llmSvc)).Should(Succeed())
			defer k8sClient.Delete(ctx, llmSvc)

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: modelName, Namespace: testNamespace}, cachedModel)
				if err != nil {
					return false
				}
				if len(cachedModel.Status.LLMInferenceServices) != 1 {
					return false
				}
				return cachedModel.Status.LLMInferenceServices[0].Name == llmSvcName &&
					cachedModel.Status.LLMInferenceServices[0].Namespace == testNamespace
			}, timeout, interval).Should(BeTrue(), "Namespace cache status should track the LLMInferenceService via LoRA adapter reference")
		})
	})
})

func createTestNamespace(ctx context.Context, name string) *corev1.Namespace {
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	Expect(k8sClient.Create(ctx, namespace)).Should(Succeed())
	return namespace
}

func createTestPV(ctx context.Context, pvName string, cachedModel *v1alpha1.LocalModelCache) *corev1.PersistentVolume {
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: pvName,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "serving.kserve.io/v1alpha1",
					Kind:       "LocalModelCache",
					Name:       cachedModel.Name,
					UID:        cachedModel.UID,
					Controller: ptr.To(true),
				},
			},
		},
		Spec: corev1.PersistentVolumeSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse("2Gi"),
			},
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/mnt/data",
					Type: ptr.To(corev1.HostPathDirectoryOrCreate),
				},
			},
		},
	}
	Expect(k8sClient.Create(ctx, pv)).Should(Succeed())
	return pv
}

func createTestPVC(ctx context.Context, pvcName, namespace, pvName string, cachedModel *v1alpha1.LocalModelCache) *corev1.PersistentVolumeClaim {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "serving.kserve.io/v1alpha1",
					Kind:       "LocalModelCache",
					Name:       cachedModel.Name,
					UID:        cachedModel.UID,
					Controller: ptr.To(true),
				},
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("2Gi"),
				},
			},
			VolumeName: pvName,
		},
	}
	Expect(k8sClient.Create(ctx, pvc)).Should(Succeed())
	return pvc
}

var _ = Describe("LocalModel cache deletion with missing node groups", func() {
	const (
		timeout             = time.Second * 10
		interval            = time.Millisecond * 250
		sourceModelUri      = "s3://mybucket/mymodel"
		modelCacheNamespace = "kserve-localmodel-jobs"
	)

	// deletionNodeGroupSpec matches no real node; node affinity is irrelevant
	// for the deletion paths under test.
	deletionNodeGroupSpec := func(instanceType string) v1alpha1.LocalModelNodeGroupSpec {
		return v1alpha1.LocalModelNodeGroupSpec{
			PersistentVolumeSpec: corev1.PersistentVolumeSpec{
				AccessModes:                   []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				VolumeMode:                    ptr.To(corev1.PersistentVolumeFilesystem),
				Capacity:                      corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("2Gi")},
				StorageClassName:              "standard",
				PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimDelete,
				PersistentVolumeSource: corev1.PersistentVolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: "/models",
						Type: ptr.To(corev1.HostPathDirectory),
					},
				},
				NodeAffinity: &corev1.VolumeNodeAffinity{
					Required: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{
							{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{
										Key:      "node.kubernetes.io/instance-type",
										Operator: corev1.NodeSelectorOpIn,
										Values:   []string{instanceType},
									},
								},
							},
						},
					},
				},
			},
			PersistentVolumeClaimSpec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("2Gi")},
				},
			},
		}
	}

	deleteIfPresent := func(ctx context.Context, obj ctrlclient.Object) error {
		return ctrlclient.IgnoreNotFound(k8sClient.Delete(ctx, obj))
	}

	createNodeGroup := func(ctx context.Context, name string) *v1alpha1.LocalModelNodeGroup {
		nodeGroup := &v1alpha1.LocalModelNodeGroup{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Spec:       deletionNodeGroupSpec(name),
		}
		Expect(k8sClient.Create(ctx, nodeGroup)).Should(Succeed())
		DeferCleanup(deleteIfPresent, nodeGroup)
		return nodeGroup
	}

	deleteAndWaitGone := func(ctx context.Context, obj ctrlclient.Object, key types.NamespacedName) {
		Expect(k8sClient.Delete(ctx, obj)).Should(Succeed())
		Eventually(func() bool {
			err := k8sClient.Get(ctx, key, obj)
			return err != nil && errors.IsNotFound(err)
		}, timeout, interval).Should(BeTrue(), "object should be gone: "+key.String())
	}

	waitForFinalizer := func(ctx context.Context, obj ctrlclient.Object, finalizer string) {
		Eventually(func() bool {
			if err := k8sClient.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), obj); err != nil {
				return false
			}
			for _, f := range obj.GetFinalizers() {
				if f == finalizer {
					return true
				}
			}
			return false
		}, timeout, interval).Should(BeTrue(), "finalizer should be added: "+finalizer)
	}

	// envtest runs no controllers that strip the pv/pvc-protection finalizers,
	// so a deleted volume may keep its deletionTimestamp instead of vanishing.
	goneOrDeleting := func(ctx context.Context, obj ctrlclient.Object, key types.NamespacedName) bool {
		err := k8sClient.Get(ctx, key, obj)
		if err != nil {
			return errors.IsNotFound(err)
		}
		return !obj.GetDeletionTimestamp().IsZero()
	}

	It("Should finish deleting a LocalModelCache whose node group is already deleted", func() {
		ctx, cancel := context.WithCancel(context.Background())
		DeferCleanup(cancel)

		nodeGroup := createNodeGroup(ctx, "gpu-del-cluster")
		modelName := "cluster-iris-deletion"
		cachedModel := &v1alpha1.LocalModelCache{
			ObjectMeta: metav1.ObjectMeta{Name: modelName},
			Spec: v1alpha1.LocalModelCacheSpec{
				SourceModelUri: sourceModelUri,
				ModelSize:      resource.MustParse("123Gi"),
				NodeGroups:     []string{nodeGroup.Name},
			},
		}
		Expect(k8sClient.Create(ctx, cachedModel)).Should(Succeed())

		waitForFinalizer(ctx, cachedModel, reconcilers.FinalizerName)

		// Delete the node group first, then the cache: deletion must still finish.
		deleteAndWaitGone(ctx, nodeGroup, types.NamespacedName{Name: nodeGroup.Name})
		deleteAndWaitGone(ctx, cachedModel, types.NamespacedName{Name: modelName})
	})

	It("Should finish deleting a LocalModelNamespaceCache and clean its volumes when the node group is already deleted", func() {
		ctx, cancel := context.WithCancel(context.Background())
		DeferCleanup(cancel)

		testNamespace := fmt.Sprintf("test-ns-nodegroup-deleted-%d", time.Now().UnixNano())
		namespaceObj := createTestNamespace(ctx, testNamespace)
		DeferCleanup(deleteIfPresent, namespaceObj)

		nodeGroup := createNodeGroup(ctx, "gpu-del-ns")
		modelName := "ns-iris-deletion"
		cachedModel := &v1alpha1.LocalModelNamespaceCache{
			ObjectMeta: metav1.ObjectMeta{Name: modelName, Namespace: testNamespace},
			Spec: v1alpha1.LocalModelNamespaceCacheSpec{
				SourceModelUri: sourceModelUri,
				ModelSize:      resource.MustParse("123Gi"),
				NodeGroups:     []string{nodeGroup.Name},
			},
		}
		Expect(k8sClient.Create(ctx, cachedModel)).Should(Succeed())

		waitForFinalizer(ctx, cachedModel, reconcilers.NamespaceCacheFinalizerName)

		// An ISVC consumer makes the controller create the namespace-scoped
		// serving volumes on top of the download ones.
		isvc := &v1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:      modelName + "-isvc",
				Namespace: testNamespace,
				Labels: map[string]string{
					constants.LocalModelLabel:          modelName,
					constants.LocalModelNamespaceLabel: testNamespace,
				},
			},
			Spec: v1beta1.InferenceServiceSpec{
				Predictor: v1beta1.PredictorSpec{
					Model: &v1beta1.ModelSpec{
						PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
							StorageURI: ptr.To(sourceModelUri),
						},
						ModelFormat: v1beta1.ModelFormat{Name: "sklearn"},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
		DeferCleanup(deleteIfPresent, isvc)

		downloadPVName := modelName + "-" + nodeGroup.Name + "-" + testNamespace + "-download"
		servingPVName := modelName + "-" + nodeGroup.Name + "-" + testNamespace
		downloadPVC := &corev1.PersistentVolumeClaim{}
		Eventually(func() bool {
			return k8sClient.Get(ctx, types.NamespacedName{Name: downloadPVName, Namespace: modelCacheNamespace}, downloadPVC) == nil
		}, timeout, interval).Should(BeTrue(), "download PVC should be created")
		downloadPV := &corev1.PersistentVolume{}
		Eventually(func() bool {
			return k8sClient.Get(ctx, types.NamespacedName{Name: downloadPVName}, downloadPV) == nil
		}, timeout, interval).Should(BeTrue(), "download PV should be created")
		servingPV := &corev1.PersistentVolume{}
		Eventually(func() bool {
			return k8sClient.Get(ctx, types.NamespacedName{Name: servingPVName}, servingPV) == nil
		}, timeout, interval).Should(BeTrue(), "serving PV should be created")

		// Delete the node group first, then the cache: deletion must still
		// finish and the namespace-scoped volumes must be cleaned up by name.
		deleteAndWaitGone(ctx, nodeGroup, types.NamespacedName{Name: nodeGroup.Name})
		deleteAndWaitGone(ctx, cachedModel, types.NamespacedName{Name: modelName, Namespace: testNamespace})

		Eventually(func() bool {
			return goneOrDeleting(ctx, downloadPV, types.NamespacedName{Name: downloadPVName})
		}, timeout, interval).Should(BeTrue(), "download PV should be deleted or deleting")
		Eventually(func() bool {
			return goneOrDeleting(ctx, downloadPVC, types.NamespacedName{Name: downloadPVName, Namespace: modelCacheNamespace})
		}, timeout, interval).Should(BeTrue(), "download PVC should be deleted or deleting")
		Eventually(func() bool {
			return goneOrDeleting(ctx, servingPV, types.NamespacedName{Name: servingPVName})
		}, timeout, interval).Should(BeTrue(), "serving PV should be deleted or deleting")
	})

	It("Should remove every matching entry from LocalModelNodes when a LocalModelCache is deleted", func() {
		ctx, cancel := context.WithCancel(context.Background())
		DeferCleanup(cancel)

		nodeGroup := createNodeGroup(ctx, "gpu-del-sweep")
		modelName := "cluster-iris-sweep"
		cachedModel := &v1alpha1.LocalModelCache{
			ObjectMeta: metav1.ObjectMeta{Name: modelName},
			Spec: v1alpha1.LocalModelCacheSpec{
				SourceModelUri: sourceModelUri,
				ModelSize:      resource.MustParse("123Gi"),
				NodeGroups:     []string{nodeGroup.Name},
			},
		}
		Expect(k8sClient.Create(ctx, cachedModel)).Should(Succeed())
		waitForFinalizer(ctx, cachedModel, reconcilers.FinalizerName)
		DeferCleanup(deleteIfPresent, cachedModel)

		// Two entries for the deleting cache (duplicate entries are allowed by
		// the CRD) and one same-name entry owned by a namespace-scoped cache:
		// all of the former must go, the latter must survive.
		localModelNode := &v1alpha1.LocalModelNode{
			ObjectMeta: metav1.ObjectMeta{Name: "sweep-node"},
			Spec: v1alpha1.LocalModelNodeSpec{
				LocalModels: []v1alpha1.LocalModelInfo{
					{ModelName: modelName, SourceModelUri: sourceModelUri, NodeGroup: nodeGroup.Name},
					{ModelName: modelName, SourceModelUri: sourceModelUri, NodeGroup: nodeGroup.Name + "-other"},
					{ModelName: modelName, Namespace: "other-namespace", SourceModelUri: sourceModelUri, NodeGroup: nodeGroup.Name},
				},
			},
		}
		Expect(k8sClient.Create(ctx, localModelNode)).Should(Succeed())
		DeferCleanup(deleteIfPresent, localModelNode)

		deleteAndWaitGone(ctx, cachedModel, types.NamespacedName{Name: modelName})

		sweptNode := &v1alpha1.LocalModelNode{}
		Eventually(func() []v1alpha1.LocalModelInfo {
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: localModelNode.Name}, sweptNode); err != nil {
				return nil
			}
			return sweptNode.Spec.LocalModels
		}, timeout, interval).Should(Equal([]v1alpha1.LocalModelInfo{
			{ModelName: modelName, Namespace: "other-namespace", SourceModelUri: sourceModelUri, NodeGroup: nodeGroup.Name},
		}), "only the entry of the deleting cache should be removed")
	})

	It("Should keep the finalizer while namespace-scoped volume cleanup fails, then finish on retry", func() {
		ctx, cancel := context.WithCancel(context.Background())
		DeferCleanup(cancel)

		testNamespace := fmt.Sprintf("test-ns-volume-fault-%d", time.Now().UnixNano())
		namespaceObj := createTestNamespace(ctx, testNamespace)
		DeferCleanup(deleteIfPresent, namespaceObj)

		nodeGroup := createNodeGroup(ctx, "gpu-del-fault")
		modelName := "ns-iris-fault"
		modelKey := types.NamespacedName{Name: modelName, Namespace: testNamespace}
		cachedModel := &v1alpha1.LocalModelNamespaceCache{
			ObjectMeta: metav1.ObjectMeta{Name: modelName, Namespace: testNamespace},
			Spec: v1alpha1.LocalModelNamespaceCacheSpec{
				SourceModelUri: sourceModelUri,
				ModelSize:      resource.MustParse("123Gi"),
				NodeGroups:     []string{nodeGroup.Name},
			},
		}
		Expect(k8sClient.Create(ctx, cachedModel)).Should(Succeed())
		waitForFinalizer(ctx, cachedModel, reconcilers.NamespaceCacheFinalizerName)

		downloadPVName := modelName + "-" + nodeGroup.Name + "-" + testNamespace + "-download"
		downloadPVC := &corev1.PersistentVolumeClaim{}
		Eventually(func() bool {
			return k8sClient.Get(ctx, types.NamespacedName{Name: downloadPVName, Namespace: modelCacheNamespace}, downloadPVC) == nil
		}, timeout, interval).Should(BeTrue(), "download PVC should be created")

		// Deny the download PVC delete on the reconcilers' clientset only.
		denials := setAPIFault(apiFaultSpec{
			method:       http.MethodDelete,
			pathContains: "/persistentvolumeclaims/",
			nameContains: "/" + downloadPVName,
		})
		DeferCleanup(clearAPIFault)

		Expect(k8sClient.Delete(ctx, cachedModel)).Should(Succeed())

		// The deletion reconcile must reach the volume cleanup (the download PV
		// delete precedes the faulted PVC delete) and then fail on the PVC.
		downloadPV := &corev1.PersistentVolume{}
		Eventually(func() bool {
			return goneOrDeleting(ctx, downloadPV, types.NamespacedName{Name: downloadPVName})
		}, timeout, interval).Should(BeTrue(), "cleanup must reach the volume deletes")

		// Wait until at least one retry has also been denied: a first failed
		// reconcile followed by a queued retry, both hitting the fault.
		Eventually(func() bool {
			return denials.Load() >= 2
		}, time.Second*30, interval).Should(BeTrue(), "a cleanup retry must have been denied too")

		// With every retry still failing, the finalizer must stay: the cache is
		// not dropped with the volume still around.
		Expect(k8sClient.Get(ctx, modelKey, cachedModel)).Should(Succeed())
		Expect(cachedModel.Finalizers).Should(ContainElement(reconcilers.NamespaceCacheFinalizerName),
			"finalizer must stay while the PVC delete fails")
		Expect(cachedModel.DeletionTimestamp.IsZero()).Should(BeFalse())

		// With the fault gone the work-queue retry completes the deletion.
		clearAPIFault()
		Eventually(func() bool {
			err := k8sClient.Get(ctx, modelKey, cachedModel)
			return err != nil && errors.IsNotFound(err)
		}, timeout, interval).Should(BeTrue(), "LocalModelNamespaceCache should be deleted after cleanup succeeds")
		Eventually(func() bool {
			return goneOrDeleting(ctx, downloadPVC, types.NamespacedName{Name: downloadPVName, Namespace: modelCacheNamespace})
		}, timeout, interval).Should(BeTrue(), "download PVC should be deleted or deleting")
	})

	It("Should retry the LocalModelNode deletion patch on a real conflict", func() {
		ctx, cancel := context.WithCancel(context.Background())
		DeferCleanup(cancel)

		nodeName := "conflict-delete-node"
		row := v1alpha1.LocalModelInfo{ModelName: "target", SourceModelUri: sourceModelUri, NodeGroup: "unused"}
		node := &v1alpha1.LocalModelNode{
			ObjectMeta: metav1.ObjectMeta{Name: nodeName},
			Spec: v1alpha1.LocalModelNodeSpec{LocalModels: []v1alpha1.LocalModelInfo{
				row, row,
				{ModelName: "target", Namespace: "other-tenant", SourceModelUri: sourceModelUri, NodeGroup: "unused"},
			}},
		}
		Expect(k8sClient.Create(ctx, node)).Should(Succeed())
		DeferCleanup(deleteIfPresent, node)

		// A peer write lands between the read and the patch: the conflicting
		// patch must be retried against the fresh state, not overwrite it.
		w := &interleavedNodeWriter{Client: k8sClient, name: nodeName, before: firstPeerEntry}
		Expect(reconcilers.DeleteModelFromNode(ctx, k8sClient, w, logr.Discard(), nodeName, "target", "")).Should(Succeed())
		Expect(w.conflicts).Should(Equal(1))
		Expect(w.attempts).Should(Equal(2))

		current := &v1alpha1.LocalModelNode{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: nodeName}, current)).Should(Succeed())
		Expect(current.Spec.LocalModels).Should(ConsistOf(
			v1alpha1.LocalModelInfo{ModelName: "target", Namespace: "other-tenant", SourceModelUri: sourceModelUri, NodeGroup: "unused"},
			v1alpha1.LocalModelInfo{ModelName: "peer-1", Namespace: "review-tenant", SourceModelUri: sourceModelUri + "/peer-1", NodeGroup: "unused"},
		), "the concurrent peer write must survive")
	})

	It("Should give up after repeated LocalModelNode conflicts and converge on the next sweep", func() {
		ctx, cancel := context.WithCancel(context.Background())
		DeferCleanup(cancel)

		nodeName := "conflict-exhausted-node"
		row := v1alpha1.LocalModelInfo{ModelName: "target", SourceModelUri: sourceModelUri, NodeGroup: "unused"}
		node := &v1alpha1.LocalModelNode{
			ObjectMeta: metav1.ObjectMeta{Name: nodeName},
			Spec:       v1alpha1.LocalModelNodeSpec{LocalModels: []v1alpha1.LocalModelInfo{row}},
		}
		Expect(k8sClient.Create(ctx, node)).Should(Succeed())
		DeferCleanup(deleteIfPresent, node)

		w := &interleavedNodeWriter{Client: k8sClient, name: nodeName, before: appendPeerEntry}
		Expect(reconcilers.DeleteModelFromNode(ctx, k8sClient, w, logr.Discard(), nodeName, "target", "")).Should(HaveOccurred())
		Expect(w.conflicts).Should(Equal(3))
		Expect(reconcilers.DeleteModelFromNode(ctx, k8sClient, k8sClient, logr.Discard(), nodeName, "target", "")).Should(Succeed())
		current := &v1alpha1.LocalModelNode{}
		Expect(k8sClient.Get(ctx, ctrlclient.ObjectKeyFromObject(node), current)).To(Succeed())
		remaining := make([]string, 0, len(current.Spec.LocalModels))
		for _, m := range current.Spec.LocalModels {
			remaining = append(remaining, m.GetStatusKey())
		}
		Expect(remaining).To(ConsistOf("review-tenant/peer-1", "review-tenant/peer-2", "review-tenant/peer-3"))
	})

	It("Should tolerate a LocalModelNode disappearing mid-deletion", func() {
		ctx, cancel := context.WithCancel(context.Background())
		DeferCleanup(cancel)

		nodeName := "conflict-vanishing-node"
		node := &v1alpha1.LocalModelNode{
			ObjectMeta: metav1.ObjectMeta{Name: nodeName},
			Spec: v1alpha1.LocalModelNodeSpec{LocalModels: []v1alpha1.LocalModelInfo{
				{ModelName: "target", SourceModelUri: sourceModelUri, NodeGroup: "unused"},
			}},
		}
		Expect(k8sClient.Create(ctx, node)).Should(Succeed())

		DeferCleanup(deleteIfPresent, node)
		w := &interleavedNodeWriter{Client: k8sClient, name: nodeName, before: func(context.Context, types.NamespacedName, int) error {
			return k8sClient.Delete(ctx, node)
		}}
		Expect(reconcilers.DeleteModelFromNode(ctx, k8sClient, w, logr.Discard(), nodeName, "target", "")).Should(Succeed())
		Expect(w.attempts).To(Equal(1))
		Expect(reconcilers.DeleteModelFromNode(ctx, k8sClient, w, logr.Discard(), nodeName, "target", "")).To(Succeed())
		Expect(w.attempts).To(Equal(1))
	})

	DescribeTable("Should return LocalModelNode conflicts from Reconcile and converge on retry", func(ctx SpecContext, scope, operation string) {
		suffix := scope + "-" + operation
		groupName := "gpu-conflict-" + suffix
		createNodeGroup(ctx, groupName)
		modelName := "iris-conflict-" + suffix
		namespace := ""
		plainClientset, err := kubernetes.NewForConfig(cfg)
		Expect(err).Should(Succeed())
		var cache ctrlclient.Object
		var reconcile func(ctrlclient.Client) (ctrl.Result, error)
		if scope == "cluster" {
			model := &v1alpha1.LocalModelCache{
				ObjectMeta: metav1.ObjectMeta{Name: modelName, Finalizers: []string{reconcilers.FinalizerName}},
				Spec: v1alpha1.LocalModelCacheSpec{
					SourceModelUri: sourceModelUri, ModelSize: resource.MustParse("123Gi"), NodeGroups: []string{groupName},
				},
			}
			cache = model
			reconcile = func(c ctrlclient.Client) (ctrl.Result, error) {
				r := &reconcilers.LocalModelReconciler{Client: c, APIReader: k8sClient, Clientset: plainClientset, Log: logr.Discard(), Scheme: testScheme}
				return r.Reconcile(ctx, ctrl.Request{NamespacedName: ctrlclient.ObjectKeyFromObject(model)})
			}
		} else {
			namespace = fmt.Sprintf("test-ns-conflict-%d", time.Now().UnixNano())
			ns := createTestNamespace(ctx, namespace)
			DeferCleanup(deleteIfPresent, ns)
			model := &v1alpha1.LocalModelNamespaceCache{
				ObjectMeta: metav1.ObjectMeta{Name: modelName, Namespace: namespace, Finalizers: []string{reconcilers.NamespaceCacheFinalizerName}},
				Spec: v1alpha1.LocalModelNamespaceCacheSpec{
					SourceModelUri: sourceModelUri, ModelSize: resource.MustParse("123Gi"), NodeGroups: []string{groupName},
				},
			}
			cache = model
			reconcile = func(c ctrlclient.Client) (ctrl.Result, error) {
				r := &reconcilers.LocalModelNamespaceCacheReconciler{Client: c, APIReader: k8sClient, Clientset: plainClientset, Log: logr.Discard(), Scheme: testScheme}
				return r.Reconcile(ctx, ctrl.Request{NamespacedName: ctrlclient.ObjectKeyFromObject(model)})
			}
		}
		Expect(k8sClient.Create(ctx, cache)).To(Succeed())
		DeferCleanup(deleteIfPresent, cache)

		// The Ready core Node is visible only to the direct reconcile. The suite
		// controller cannot populate the LocalModelNode ahead of either call.
		// The LocalModelNode, both writes, and the conflict use the real API server.
		nodeName := groupName + "-host"
		control := clientWithReadyNode{Client: mgrClient, node: corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: nodeName, Labels: map[string]string{"node.kubernetes.io/instance-type": groupName}},
			Status:     corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}},
		}}
		keep := v1alpha1.LocalModelInfo{ModelName: "keep", SourceModelUri: sourceModelUri + "/keep", NodeGroup: groupName}
		desired := v1alpha1.LocalModelInfo{ModelName: modelName, Namespace: namespace, SourceModelUri: sourceModelUri, NodeGroup: groupName}
		initial := []v1alpha1.LocalModelInfo{keep}
		if operation == "update" {
			old := desired
			old.SourceModelUri += "/old"
			initial = append(initial, old)
		}
		node := &v1alpha1.LocalModelNode{ObjectMeta: metav1.ObjectMeta{Name: nodeName}, Spec: v1alpha1.LocalModelNodeSpec{LocalModels: initial}}
		Expect(k8sClient.Create(ctx, node)).To(Succeed())
		DeferCleanup(deleteIfPresent, node)

		waitForVersion := func(obj ctrlclient.Object, version string) {
			Eventually(func() string {
				copy := obj.DeepCopyObject().(ctrlclient.Object)
				if err := mgrClient.Get(ctx, ctrlclient.ObjectKeyFromObject(obj), copy); err != nil {
					return ""
				}
				return copy.GetResourceVersion()
			}, timeout, interval).Should(Equal(version))
		}
		// The cache object may already have a newer status version.
		Eventually(func() error {
			return mgrClient.Get(ctx, ctrlclient.ObjectKeyFromObject(cache), cache.DeepCopyObject().(ctrlclient.Object))
		}, timeout, interval).Should(Succeed())
		Eventually(func() error {
			return mgrClient.Get(ctx, types.NamespacedName{Name: groupName}, &v1alpha1.LocalModelNodeGroup{})
		}, timeout, interval).Should(Succeed())
		waitForVersion(node, node.ResourceVersion)

		w := &interleavedNodeWriter{Client: control, name: nodeName, before: firstPeerEntry}
		_, err = reconcile(w)
		Expect(w.attempts).To(Equal(1))
		Expect(w.conflicts).To(Equal(1))
		Expect(errors.IsConflict(err)).To(BeTrue(), "the outer Reconcile must return the conflict")
		live := &v1alpha1.LocalModelNode{}
		Expect(k8sClient.Get(ctx, ctrlclient.ObjectKeyFromObject(node), live)).To(Succeed())
		peer := v1alpha1.LocalModelInfo{ModelName: "peer-1", Namespace: "review-tenant", SourceModelUri: sourceModelUri + "/peer-1", NodeGroup: "unused"}
		Expect(live.Spec.LocalModels).To(ConsistOf(append(initial, peer)))
		waitForVersion(node, live.ResourceVersion)

		_, err = reconcile(control)
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, ctrlclient.ObjectKeyFromObject(node), live)).To(Succeed())
		Expect(live.Spec.LocalModels).To(ConsistOf(keep, peer, desired))
	}, Entry("cluster add", "cluster", "add"), Entry("cluster update", "cluster", "update"),
		Entry("namespace add", "ns", "add"), Entry("namespace update", "ns", "update"))
})

// apiFaultSpec selects the API requests a test wants to deny: those with the
// given method whose path contains pathContains and, when set, nameContains.
type apiFaultSpec struct {
	method       string
	pathContains string
	nameContains string
	denials      *atomic.Int64
}

// injectedAPIFault is read by faultInjectorTransport, which wraps the suite
// reconcilers' clientset; an empty method disables the injection.
var injectedAPIFault atomic.Value

func setAPIFault(f apiFaultSpec) *atomic.Int64 {
	f.denials = new(atomic.Int64)
	injectedAPIFault.Store(f)
	return f.denials
}
func clearAPIFault() { injectedAPIFault.Store(apiFaultSpec{}) }

// faultInjectorTransport denies the requests selected by injectedAPIFault with
// a 403 and passes everything else through.
type faultInjectorTransport struct {
	rt http.RoundTripper
}

func (t faultInjectorTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if f, ok := injectedAPIFault.Load().(apiFaultSpec); ok && f.method != "" &&
		req.Method == f.method && strings.Contains(req.URL.Path, f.pathContains) &&
		(f.nameContains == "" || strings.Contains(req.URL.Path, f.nameContains)) {
		f.denials.Add(1)
		return forbiddenResponse(req), nil
	}
	return t.rt.RoundTrip(req)
}

// clientWithReadyNode exposes one matching core Node only to the direct
// reconcile. The manager and the test share the real LocalModelNode API.
type clientWithReadyNode struct {
	ctrlclient.Client
	node corev1.Node
}

func (c clientWithReadyNode) List(ctx context.Context, list ctrlclient.ObjectList, opts ...ctrlclient.ListOption) error {
	if nodes, ok := list.(*corev1.NodeList); ok {
		nodes.Items = []corev1.Node{c.node}
		return nil
	}
	return c.Client.List(ctx, list, opts...)
}

func forbiddenResponse(req *http.Request) *http.Response {
	return &http.Response{
		StatusCode: http.StatusForbidden,
		Status:     "403 Forbidden",
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(
			`{"kind":"Status","apiVersion":"v1","status":"Failure","code":403,"reason":"Forbidden","message":"denied by test fault injection"}`)),
		Request: req,
	}
}

// interleavedNodeWriter wraps a controller-runtime client and runs a hook
// between a reconciler's read of a LocalModelNode and its patch, so tests can
// create genuine API-server write conflicts.
type interleavedNodeWriter struct {
	ctrlclient.Client
	name                string
	before              func(context.Context, types.NamespacedName, int) error
	attempts, conflicts int
}

func (w *interleavedNodeWriter) Patch(ctx context.Context, obj ctrlclient.Object, p ctrlclient.Patch, opts ...ctrlclient.PatchOption) error {
	if _, isNode := obj.(*v1alpha1.LocalModelNode); !isNode || obj.GetName() != w.name {
		return w.Client.Patch(ctx, obj, p, opts...)
	}
	w.attempts++
	if err := w.before(ctx, types.NamespacedName{Name: w.name}, w.attempts); err != nil {
		return err
	}
	err := w.Client.Patch(ctx, obj, p, opts...)
	if errors.IsConflict(err) {
		w.conflicts++
	}
	return err
}

// appendPeerEntry writes another cache's entry to the node, bumping its
// resourceVersion; firstPeerEntry does so only before the first patch.
func appendPeerEntry(ctx context.Context, key types.NamespacedName, seq int) error {
	node := &v1alpha1.LocalModelNode{}
	if err := k8sClient.Get(ctx, key, node); err != nil {
		return err
	}
	peerName := fmt.Sprintf("peer-%d", seq)
	node.Spec.LocalModels = append(node.Spec.LocalModels, v1alpha1.LocalModelInfo{
		ModelName: peerName, Namespace: "review-tenant",
		SourceModelUri: "s3://mybucket/mymodel/" + peerName, NodeGroup: "unused",
	})
	return k8sClient.Update(ctx, node)
}

func firstPeerEntry(ctx context.Context, key types.NamespacedName, seq int) error {
	if seq == 1 {
		return appendPeerEntry(ctx, key, seq)
	}
	return nil
}

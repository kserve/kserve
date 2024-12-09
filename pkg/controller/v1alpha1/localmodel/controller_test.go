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
	"time"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
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
			NodeGroup:      "gpu",
		}
		clusterStorageContainerSpec = v1alpha1.StorageContainerSpec{
			SupportedUriFormats: []v1alpha1.SupportedUriFormat{{Prefix: "s3://"}},
			Container: v1.Container{
				Name:  "name",
				Image: "image",
				Args: []string{
					"srcURI",
					constants.DefaultModelLocalMountPath,
				},
				TerminationMessagePolicy: v1.TerminationMessageFallbackToLogsOnError,
				VolumeMounts:             []v1.VolumeMount{},
			},
		}
		localModelNodeGroupSpec = v1alpha1.LocalModelNodeGroupSpec{
			PersistentVolumeSpec: v1.PersistentVolumeSpec{
				AccessModes:                   []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
				VolumeMode:                    ptr.To(v1.PersistentVolumeFilesystem),
				Capacity:                      v1.ResourceList{v1.ResourceStorage: resource.MustParse("2Gi")},
				StorageClassName:              "standard",
				PersistentVolumeReclaimPolicy: v1.PersistentVolumeReclaimDelete,
				PersistentVolumeSource: v1.PersistentVolumeSource{
					HostPath: &v1.HostPathVolumeSource{
						Path: "/models",
						Type: ptr.To(v1.HostPathDirectory),
					},
				},
				NodeAffinity: &v1.VolumeNodeAffinity{
					Required: &v1.NodeSelector{
						NodeSelectorTerms: []v1.NodeSelectorTerm{
							{
								MatchExpressions: []v1.NodeSelectorRequirement{
									{
										Key:      "node.kubernetes.io/instance-type",
										Operator: v1.NodeSelectorOpIn,
										Values:   []string{"gpu"},
									},
								},
							},
						},
					},
				},
			},
			PersistentVolumeClaimSpec: v1.PersistentVolumeClaimSpec{
				AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
				Resources:   v1.VolumeResourceRequirements{Requests: v1.ResourceList{v1.ResourceStorage: resource.MustParse("2Gi")}},
			},
		}
		configs = map[string]string{
			"localModel": `{
        		"jobNamespace": "kserve-localmodel-jobs",
                "defaultJobImage": "kserve/storage-initializer:latest"
            }`,
		}
	)
	Context("When creating a local model", func() {
		It("Should create pv, pvc, localmodelnode, and update status from localmodelnode", func() {
			var configMap = &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KServeNamespace,
				},
				Data: configs,
			}
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			clusterStorageContainer := &v1alpha1.ClusterStorageContainer{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
				Spec: clusterStorageContainerSpec,
			}
			Expect(k8sClient.Create(ctx, clusterStorageContainer)).Should(Succeed())
			defer k8sClient.Delete(ctx, clusterStorageContainer)

			nodeGroup := &v1alpha1.LocalModelNodeGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gpu",
				},
				Spec: localModelNodeGroupSpec,
			}
			Expect(k8sClient.Create(ctx, nodeGroup)).Should(Succeed())
			defer k8sClient.Delete(ctx, nodeGroup)

			cachedModel := &v1alpha1.LocalModelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name: "iris",
				},
				Spec: localModelSpec,
			}
			Expect(k8sClient.Create(ctx, cachedModel)).Should(Succeed())

			modelLookupKey := types.NamespacedName{Name: "iris"}
			pvLookupKey := types.NamespacedName{Name: "iris-download"}
			pvcLookupKey := types.NamespacedName{Name: "iris", Namespace: modelCacheNamespace}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, modelLookupKey, cachedModel)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			Eventually(func() bool {
				err := k8sClient.Get(ctx, modelLookupKey, cachedModel)
				return err == nil && cachedModel.Status.ModelCopies != nil
			}, timeout, interval).Should(BeTrue())

			persistentVolume := &v1.PersistentVolume{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, pvLookupKey, persistentVolume)
				return err == nil && persistentVolume != nil
			}, timeout, interval).Should(BeTrue())

			persistentVolumeClaim := &v1.PersistentVolumeClaim{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, pvcLookupKey, persistentVolumeClaim)
				return err == nil && persistentVolumeClaim != nil
			}, timeout, interval).Should(BeTrue())

			nodeName := "node-1"
			node1 := &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: nodeName,
					Labels: map[string]string{
						"node.kubernetes.io/instance-type": "gpu",
					},
				},
				Status: v1.NodeStatus{
					Conditions: []v1.NodeCondition{
						{
							Type:   v1.NodeReady,
							Status: v1.ConditionTrue,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, node1)).Should(Succeed())
			defer k8sClient.Delete(ctx, node1)

			localModelNode := &v1alpha1.LocalModelNode{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: nodeName}, localModelNode)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			Expect(localModelNode.Spec.LocalModels).Should(ContainElement(v1alpha1.LocalModelInfo{ModelName: cachedModel.Name, SourceModelUri: sourceModelUri}))

			// Todo: Test agent download
			// Update the LocalModelNode status to be successful
			localModelNode.Status.ModelStatus = map[string]v1alpha1.ModelStatus{cachedModel.Name: v1alpha1.ModelDownloaded}
			Expect(k8sClient.Status().Update(ctx, localModelNode)).Should(Succeed())

			Eventually(func() bool {
				err := k8sClient.Get(ctx, modelLookupKey, cachedModel)
				if err != nil {
					return false
				}
				if !(cachedModel.Status.ModelCopies.Available == 1 && cachedModel.Status.ModelCopies.Total == 1 && cachedModel.Status.ModelCopies.Failed == 0) {
					return false
				}
				if cachedModel.Status.NodeStatus[nodeName] != v1alpha1.NodeDownloaded {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue(), "Node status should be downloaded")

			// Now let's test deletion
			k8sClient.Delete(ctx, cachedModel)

			newLocalModel := &v1alpha1.LocalModelCache{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, modelLookupKey, newLocalModel)
				return err == nil
			}, timeout, interval).Should(BeFalse(), "Should not get the local model after deletion")
		})

		It("Should create pvs and pvcs for inference services", func() {
			var configMap = &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KServeNamespace,
				},
				Data: configs,
			}
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			clusterStorageContainer := &v1alpha1.ClusterStorageContainer{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
				Spec: clusterStorageContainerSpec,
			}
			Expect(k8sClient.Create(ctx, clusterStorageContainer)).Should(Succeed())
			defer k8sClient.Delete(ctx, clusterStorageContainer)

			nodeGroup := &v1alpha1.LocalModelNodeGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gpu",
				},
				Spec: localModelNodeGroupSpec,
			}
			Expect(k8sClient.Create(ctx, nodeGroup)).Should(Succeed())
			defer k8sClient.Delete(ctx, nodeGroup)

			modelName := "iris2"
			isvcNamespace := "default"
			isvcName := "foo"
			cachedModel := &v1alpha1.LocalModelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name: modelName,
				},
				Spec: localModelSpec,
			}
			Expect(k8sClient.Create(ctx, cachedModel)).Should(Succeed())
			defer k8sClient.Delete(ctx, cachedModel)

			isvc := &v1beta1.InferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
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

			// Mutating webhook adds a local model label
			cachedModelList := &v1alpha1.LocalModelCacheList{}
			cachedModelList.Items = []v1alpha1.LocalModelCache{*cachedModel}
			isvc.DefaultInferenceService(nil, nil, nil, cachedModelList)

			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
			inferenceService := &v1beta1.InferenceService{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: isvcName, Namespace: isvcNamespace}, inferenceService)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			// Expects a pv and a pvc are created in the isvcNamespace
			pvLookupKey := types.NamespacedName{Name: modelName + "-" + isvcNamespace}
			pvcLookupKey := types.NamespacedName{Name: modelName, Namespace: isvcNamespace}

			persistentVolume := &v1.PersistentVolume{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, pvLookupKey, persistentVolume)
				return err == nil && persistentVolume != nil
			}, timeout, interval).Should(BeTrue())

			persistentVolumeClaim := &v1.PersistentVolumeClaim{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, pvcLookupKey, persistentVolumeClaim)
				return err == nil && persistentVolumeClaim != nil
			}, timeout, interval).Should(BeTrue())

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: modelName}, cachedModel)
				if err != nil {
					return false
				}
				if len(cachedModel.Status.InferenceServices) != 1 {
					return false
				}
				isvcNamespacedName := cachedModel.Status.InferenceServices[0]
				if isvcNamespacedName.Name == isvcName && isvcNamespacedName.Namespace == isvcNamespace {
					return true
				}
				return false
			}, timeout, interval).Should(BeTrue(), "Node status should have the isvc")

			// Next we delete the isvc and make sure the pv and pvc are deleted
			Expect(k8sClient.Delete(ctx, isvc)).Should(Succeed())

			newPersistentVolume := &v1.PersistentVolume{}
			newPersistentVolumeClaim := &v1.PersistentVolumeClaim{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, pvLookupKey, newPersistentVolume)
				if err != nil {
					return false
				}
				if newPersistentVolume.DeletionTimestamp != nil {
					return true
				}
				return false
			}, timeout, interval).Should(BeTrue())

			Eventually(func() bool {
				err := k8sClient.Get(ctx, pvcLookupKey, newPersistentVolumeClaim)
				if err != nil {
					return false
				}
				if newPersistentVolumeClaim.DeletionTimestamp != nil {
					return true
				}
				return false
			}, timeout, interval).Should(BeTrue())
		})
	})
	Context("When creating multiple localModels", func() {
		// With two nodes and two local models, each node should have both local models
		It("Should create localModelNode correctly", func() {
			var configMap = &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KServeNamespace,
				},
				Data: configs,
			}
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			clusterStorageContainer := &v1alpha1.ClusterStorageContainer{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
				Spec: clusterStorageContainerSpec,
			}
			Expect(k8sClient.Create(ctx, clusterStorageContainer)).Should(Succeed())
			defer k8sClient.Delete(ctx, clusterStorageContainer)

			nodeGroup := &v1alpha1.LocalModelNodeGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gpu",
				},
				Spec: localModelNodeGroupSpec,
			}
			Expect(k8sClient.Create(ctx, nodeGroup)).Should(Succeed())
			defer k8sClient.Delete(ctx, nodeGroup)

			node1 := &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
					Labels: map[string]string{
						"node.kubernetes.io/instance-type": "gpu",
					},
				},
				Status: v1.NodeStatus{
					Conditions: []v1.NodeCondition{
						{
							Type:   v1.NodeReady,
							Status: v1.ConditionTrue,
						},
					},
				},
			}

			node2 := &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node2",
					Labels: map[string]string{
						"node.kubernetes.io/instance-type": "gpu",
					},
				},
				Status: v1.NodeStatus{
					Conditions: []v1.NodeCondition{
						{
							Type:   v1.NodeReady,
							Status: v1.ConditionTrue,
						},
					},
				},
			}

			nodes := []*v1.Node{node1, node2}
			for _, node := range nodes {
				Expect(k8sClient.Create(ctx, node)).Should(Succeed())
				defer k8sClient.Delete(ctx, node)
			}

			cachedModel1 := &v1alpha1.LocalModelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name: "iris1",
				},
				Spec: localModelSpec,
			}
			Expect(k8sClient.Create(ctx, cachedModel1)).Should(Succeed())
			cachedModel2 := &v1alpha1.LocalModelCache{
				ObjectMeta: metav1.ObjectMeta{
					Name: "iris2",
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
		})
	})
})

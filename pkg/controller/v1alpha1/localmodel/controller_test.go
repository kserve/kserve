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
	"github.com/kserve/kserve/pkg/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	crconfig "sigs.k8s.io/controller-runtime/pkg/config"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
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
		localModelNodeGroupSpec1 = v1alpha1.LocalModelNodeGroupSpec{
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
										Values:   []string{"gpu1"},
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
		localModelNodeGroupSpec2 = v1alpha1.LocalModelNodeGroupSpec{
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
										Values:   []string{"gpu2"},
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
		var (
			configMap               *v1.ConfigMap
			clusterStorageContainer *v1alpha1.ClusterStorageContainer
		)
		BeforeEach(func() {
			configMap, clusterStorageContainer = genericSetup(configs, clusterStorageContainerSpec)
			initializeManager(ctx, cfg)
		})

		AfterEach(func() {
			defer k8sClient.Delete(ctx, configMap)
			defer k8sClient.Delete(ctx, clusterStorageContainer)
		})

		It("Should create pv, pvc, localmodelnode, and update status from localmodelnode", func() {
			defer GinkgoRecover()
			configMap := &v1.ConfigMap{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace}, configMap)
			Expect(err).ToNot(HaveOccurred(), "InferenceService ConfigMap should exist")

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

			persistentVolume1 := &v1.PersistentVolume{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, pvLookupKey1, persistentVolume1)
				return err == nil && persistentVolume1 != nil
			}, timeout, interval).Should(BeTrue())
			persistentVolume2 := &v1.PersistentVolume{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, pvLookupKey2, persistentVolume2)
				return err == nil && persistentVolume2 != nil
			}, timeout, interval).Should(BeTrue())

			persistentVolumeClaim1 := &v1.PersistentVolumeClaim{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, pvcLookupKey1, persistentVolumeClaim1)
				return err == nil && persistentVolumeClaim1 != nil
			}, timeout, interval).Should(BeTrue())
			persistentVolumeClaim2 := &v1.PersistentVolumeClaim{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, pvcLookupKey2, persistentVolumeClaim2)
				return err == nil && persistentVolumeClaim2 != nil
			}, timeout, interval).Should(BeTrue())

			nodeName1 := "node-1"
			nodeName2 := "node-2"
			node1 := &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: nodeName1,
					Labels: map[string]string{
						"node.kubernetes.io/instance-type": "gpu1",
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
					Name: nodeName2,
					Labels: map[string]string{
						"node.kubernetes.io/instance-type": "gpu2",
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
			Expect(k8sClient.Create(ctx, node2)).Should(Succeed())
			defer k8sClient.Delete(ctx, node2)

			localModelNode1 := &v1alpha1.LocalModelNode{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: nodeName1}, localModelNode1)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			Expect(localModelNode1.Spec.LocalModels).Should(ContainElement(v1alpha1.LocalModelInfo{ModelName: cachedModel.Name, SourceModelUri: sourceModelUri}))
			localModelNode2 := &v1alpha1.LocalModelNode{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: nodeName2}, localModelNode2)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			Expect(localModelNode2.Spec.LocalModels).Should(ContainElement(v1alpha1.LocalModelInfo{ModelName: cachedModel.Name, SourceModelUri: sourceModelUri}))

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
				if !(cachedModel.Status.ModelCopies.Available == 2 && cachedModel.Status.ModelCopies.Total == 2 && cachedModel.Status.ModelCopies.Failed == 0) {
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

		It("Should create pvs and pvcs for inference services", func() {
			defer GinkgoRecover()
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
			defer k8sClient.Delete(ctx, cachedModel)
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
			isvc1.DefaultInferenceService(nil, nil, nil, cachedModelList)
			isvc2.DefaultInferenceService(nil, nil, nil, cachedModelList)

			Expect(k8sClient.Create(ctx, isvc1)).Should(Succeed())
			inferenceService1 := &v1beta1.InferenceService{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: isvcName1, Namespace: isvcNamespace}, inferenceService1)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())
			Expect(k8sClient.Create(ctx, isvc2)).Should(Succeed())
			inferenceService2 := &v1beta1.InferenceService{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: isvcName2, Namespace: isvcNamespace}, inferenceService2)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			// Expects a pv and a pvc are created in the isvcNamespace
			pvLookupKey1 := types.NamespacedName{Name: modelName + "-" + nodeGroup1.Name + "-" + isvcNamespace}
			pvLookupKey2 := types.NamespacedName{Name: modelName + "-" + nodeGroup2.Name + "-" + isvcNamespace}
			pvcLookupKey1 := types.NamespacedName{Name: modelName + "-" + nodeGroup1.Name, Namespace: isvcNamespace}
			pvcLookupKey2 := types.NamespacedName{Name: modelName + "-" + nodeGroup2.Name, Namespace: isvcNamespace}

			persistentVolume1 := &v1.PersistentVolume{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, pvLookupKey1, persistentVolume1)
				return err == nil && persistentVolume1 != nil
			}, timeout, interval).Should(BeTrue())
			persistentVolume2 := &v1.PersistentVolume{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, pvLookupKey2, persistentVolume2)
				return err == nil && persistentVolume2 != nil
			}, timeout, interval).Should(BeTrue())

			persistentVolumeClaim1 := &v1.PersistentVolumeClaim{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, pvcLookupKey1, persistentVolumeClaim1)
				return err == nil && persistentVolumeClaim1 != nil
			}, timeout, interval).Should(BeTrue())
			persistentVolumeClaim2 := &v1.PersistentVolumeClaim{}
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
				isvcNamespacedName1 := cachedModel.Status.InferenceServices[0]
				isvcNamespacedName2 := cachedModel.Status.InferenceServices[0]
				if isvcNamespacedName1.Name == isvcName1 && isvcNamespacedName1.Namespace == isvcNamespace {
					return true
				}
				if isvcNamespacedName2.Name == isvcName2 && isvcNamespacedName2.Namespace == isvcNamespace {
					return true
				}
				return false
			}, timeout, interval).Should(BeTrue(), "Node status should have the isvc")

			// Next we delete the isvc and make sure the pv and pvc are deleted
			Expect(k8sClient.Delete(ctx, isvc1)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, isvc2)).Should(Succeed())

			newPersistentVolume1 := &v1.PersistentVolume{}
			newPersistentVolume2 := &v1.PersistentVolume{}
			newPersistentVolumeClaim1 := &v1.PersistentVolumeClaim{}
			newPersistentVolumeClaim2 := &v1.PersistentVolumeClaim{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, pvLookupKey1, newPersistentVolume1)
				if err != nil {
					return false
				}
				if newPersistentVolume1.DeletionTimestamp != nil {
					return true
				}
				return false
			}, timeout, interval).Should(BeTrue())
			Eventually(func() bool {
				err := k8sClient.Get(ctx, pvLookupKey2, newPersistentVolume2)
				if err != nil {
					return false
				}
				if newPersistentVolume2.DeletionTimestamp != nil {
					return true
				}
				return false
			}, timeout, interval).Should(BeTrue())

			Eventually(func() bool {
				err := k8sClient.Get(ctx, pvcLookupKey1, newPersistentVolumeClaim1)
				if err != nil {
					return false
				}
				if newPersistentVolumeClaim1.DeletionTimestamp != nil {
					return true
				}
				return false
			}, timeout, interval).Should(BeTrue())
			Eventually(func() bool {
				err := k8sClient.Get(ctx, pvcLookupKey2, newPersistentVolumeClaim2)
				if err != nil {
					return false
				}
				if newPersistentVolumeClaim2.DeletionTimestamp != nil {
					return true
				}
				return false
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("When DisableVolumeManagement is set to true", func() {
		var (
			configMap               *v1.ConfigMap
			clusterStorageContainer *v1alpha1.ClusterStorageContainer
		)
		BeforeEach(func() {
			configs = map[string]string{
				"localModel": `{
					"jobNamespace": "kserve-localmodel-jobs",
					"defaultJobImage": "kserve/storage-initializer:latest",
					"disableVolumeManagement": true
				}`,
			}
			configMap, clusterStorageContainer = genericSetup(configs, clusterStorageContainerSpec)
			initializeManager(ctx, cfg)
		})

		AfterEach(func() {
			defer k8sClient.Delete(ctx, configMap)
			defer k8sClient.Delete(ctx, clusterStorageContainer)
		})

		It("Should NOT create/delete pvs and pvcs if localmodel config value DisableVolumeManagement is true", func() {
			defer GinkgoRecover()
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

			pv := createTestPV(ctx, pvName, testNamespace, cachedModel)
			defer k8sClient.Delete(ctx, pv)

			pvc := createTestPVC(ctx, pvcName, testNamespace, pvName, cachedModel)
			defer k8sClient.Delete(ctx, pvc)

			// Expects test-pv and test-pvc to not get deleted
			pvLookupKey := types.NamespacedName{Name: pvName}
			pvcLookupKey := types.NamespacedName{Name: pvcName, Namespace: testNamespace}

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
		})
	})

	Context("When creating multiple localModels", func() {
		var (
			configMap               *v1.ConfigMap
			clusterStorageContainer *v1alpha1.ClusterStorageContainer

			configs = map[string]string{
				"localModel": `{
					"jobNamespace": "kserve-localmodel-jobs",
					"defaultJobImage": "kserve/storage-initializer:latest"
				}`,
			}
		)
		BeforeEach(func() {
			configMap, clusterStorageContainer = genericSetup(configs, clusterStorageContainerSpec)

			initializeManager(ctx, cfg)
		})

		AfterEach(func() {
			defer k8sClient.Delete(ctx, configMap)
			defer k8sClient.Delete(ctx, clusterStorageContainer)
		})

		// With two nodes and two local models, each node should have both local models
		It("Should create localModelNode correctly", func() {
			defer GinkgoRecover()
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

			node1 := &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
					Labels: map[string]string{
						"node.kubernetes.io/instance-type": "gpu1",
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
						"node.kubernetes.io/instance-type": "gpu2",
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

func ptrToBool(b bool) *bool {
	return &b
}

func createTestNamespace(ctx context.Context, name string) *v1.Namespace {
	namespace := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	Expect(k8sClient.Create(ctx, namespace)).Should(Succeed())
	return namespace
}

func createTestPV(ctx context.Context, pvName, namespace string, cachedModel *v1alpha1.LocalModelCache) *v1.PersistentVolume {
	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: pvName,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "serving.kserve.io/v1alpha1",
					Kind:       "LocalModelCache",
					Name:       cachedModel.Name,
					UID:        cachedModel.UID,
					Controller: ptrToBool(true),
				},
			},
		},
		Spec: v1.PersistentVolumeSpec{
			AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
			Capacity: v1.ResourceList{
				v1.ResourceStorage: resource.MustParse("2Gi"),
			},
			PersistentVolumeSource: v1.PersistentVolumeSource{
				HostPath: &v1.HostPathVolumeSource{
					Path: "/mnt/data",
					Type: ptr.To(v1.HostPathDirectoryOrCreate),
				},
			},
		},
	}
	Expect(k8sClient.Create(ctx, pv)).Should(Succeed())
	return pv
}

func createTestPVC(ctx context.Context, pvcName, namespace, pvName string, cachedModel *v1alpha1.LocalModelCache) *v1.PersistentVolumeClaim {
	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "serving.kserve.io/v1alpha1",
					Kind:       "LocalModelCache",
					Name:       cachedModel.Name,
					UID:        cachedModel.UID,
					Controller: ptrToBool(true),
				},
			},
		},
		Spec: v1.PersistentVolumeClaimSpec{
			AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
			Resources: v1.VolumeResourceRequirements{
				Requests: v1.ResourceList{
					v1.ResourceStorage: resource.MustParse("2Gi"),
				},
			},
			VolumeName: pvName,
		},
	}
	Expect(k8sClient.Create(ctx, pvc)).Should(Succeed())
	return pvc
}

func genericSetup(configs map[string]string, clusterStorageContainerSpec v1alpha1.StorageContainerSpec) (*v1.ConfigMap,
	*v1alpha1.ClusterStorageContainer) {
	var configMap = &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.InferenceServiceConfigMapName,
			Namespace: constants.KServeNamespace,
		},
		Data: configs,
	}
	Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())

	clusterStorageContainer := &v1alpha1.ClusterStorageContainer{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
		Spec: clusterStorageContainerSpec,
	}
	Expect(k8sClient.Create(ctx, clusterStorageContainer)).Should(Succeed())

	return configMap, clusterStorageContainer
}

func initializeManager(ctx context.Context, cfg *rest.Config) {
	clientset, err := kubernetes.NewForConfig(cfg)
	Expect(err).ToNot(HaveOccurred())
	Expect(clientset).ToNot(BeNil())
	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
		Metrics: metricsserver.Options{
			BindAddress: "0",
		},
		Controller: crconfig.Controller{
			SkipNameValidation: utils.ToPointer(true),
		},
	})
	Expect(err).ToNot(HaveOccurred())
	err = (&LocalModelReconciler{
		Client:    k8sManager.GetClient(),
		Clientset: clientset,
		Scheme:    scheme.Scheme,
		Log:       ctrl.Log.WithName("v1alpha1LocalModelController"),
	}).SetupWithManager(k8sManager)

	Expect(err).ToNot(HaveOccurred())

	go func() {
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred())
	}()
}

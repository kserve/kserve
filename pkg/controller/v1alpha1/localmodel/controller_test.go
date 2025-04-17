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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
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
			Container: corev1.Container{
				Name:  "name",
				Image: "image",
				Args: []string{
					"srcURI",
					constants.DefaultModelLocalMountPath,
				},
				TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
				VolumeMounts:             []corev1.VolumeMount{},
			},
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
		configs = map[string]string{
			"localModel": `{
        		"jobNamespace": "kserve-localmodel-jobs",
                "defaultJobImage": "kserve/storage-initializer:latest"
            }`,
		}
	)

	Context("When creating a local model", func() {
		var (
			configMap               *corev1.ConfigMap
			clusterStorageContainer *v1alpha1.ClusterStorageContainer
		)
		BeforeEach(func() {
			ctx, cancel := context.WithCancel(context.Background())
			configMap, clusterStorageContainer = genericSetup(ctx, configs, clusterStorageContainerSpec)
			initializeManager(ctx, cfg)
			DeferCleanup(func() {
				k8sClient.Delete(ctx, clusterStorageContainer)
				k8sClient.Delete(ctx, configMap)

				By("canceling the context")
				cancel()
			})
		})

		It("Should create pv, pvc, localmodelnode, and update status from localmodelnode", func() {
			defer GinkgoRecover()
			ctx, cancel := context.WithCancel(context.Background())
			DeferCleanup(cancel)
			configMap := &corev1.ConfigMap{}
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
			isvc1.DefaultInferenceService(nil, nil, nil, cachedModelList)
			isvc2.DefaultInferenceService(nil, nil, nil, cachedModelList)

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
	})

	Context("When DisableVolumeManagement is set to true", func() {
		var (
			configMap               *corev1.ConfigMap
			clusterStorageContainer *v1alpha1.ClusterStorageContainer
		)

		BeforeEach(func() {
			ctx, cancel := context.WithCancel(context.Background())
			configs = map[string]string{
				"localModel": `{
					"jobNamespace": "kserve-localmodel-jobs",
					"defaultJobImage": "kserve/storage-initializer:latest",
					"disableVolumeManagement": true
				}`,
			}
			configMap, clusterStorageContainer = genericSetup(ctx, configs, clusterStorageContainerSpec)
			initializeManager(ctx, cfg)
			DeferCleanup(func() {
				k8sClient.Delete(ctx, clusterStorageContainer)
				k8sClient.Delete(ctx, configMap)

				By("canceling the context")
				cancel()
			})
		})

		It("Should NOT create/delete pvs and pvcs if localmodel config value DisableVolumeManagement is true", func() {
			defer GinkgoRecover()
			ctx, cancel := context.WithCancel(context.Background())
			DeferCleanup(cancel)
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
		var (
			configMap               *corev1.ConfigMap
			clusterStorageContainer *v1alpha1.ClusterStorageContainer

			configs = map[string]string{
				"localModel": `{
					"jobNamespace": "kserve-localmodel-jobs",
					"defaultJobImage": "kserve/storage-initializer:latest"
				}`,
			}
		)
		BeforeEach(func() {
			ctx, cancel := context.WithCancel(context.TODO())
			configMap, clusterStorageContainer = genericSetup(ctx, configs, clusterStorageContainerSpec)

			initializeManager(ctx, cfg)
			DeferCleanup(func() {
				k8sClient.Delete(ctx, clusterStorageContainer)
				k8sClient.Delete(ctx, configMap)

				By("canceling the context")
				cancel()
			})
		})

		// With two nodes and two local models, each node should have both local models
		It("Should create localModelNode correctly", func() {
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

func genericSetup(ctx context.Context, configs map[string]string, clusterStorageContainerSpec v1alpha1.StorageContainerSpec) (*corev1.ConfigMap,
	*v1alpha1.ClusterStorageContainer,
) {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.InferenceServiceConfigMapName,
			Namespace: constants.KServeNamespace,
		},
		Data: configs,
	}
	Expect(k8sClient.Create(ctx, configMap)).NotTo(HaveOccurred())

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
			SkipNameValidation: ptr.To(true),
		},
	})
	Expect(err).ToNot(HaveOccurred())

	k8sClient = k8sManager.GetClient()
	Expect(k8sClient).ToNot(BeNil())
	//nolint: contextcheck
	err = (&LocalModelReconciler{
		Client:    k8sClient,
		Clientset: clientset,
		Scheme:    scheme.Scheme,
		Log:       ctrl.Log.WithName("v1alpha1LocalModelController"),
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	go func() {
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred())
	}()
	// Wait for cache to start
	// Ping the ConfigMap to ensure the cache is started
	Eventually(func() bool {
		return k8sClient.Get(ctx, types.NamespacedName{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace}, &corev1.ConfigMap{}) == nil
	}).Should(BeTrue())
}

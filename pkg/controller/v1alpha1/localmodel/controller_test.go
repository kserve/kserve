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
	batchv1 "k8s.io/api/batch/v1"
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
		localModelSpec = v1alpha1.ClusterLocalModelSpec{
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
		It("Should create pv, pvc, and download jobs", func() {
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

			cachedModel := &v1alpha1.ClusterLocalModel{
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
			nodes := &v1.NodeList{}
			Eventually(func() bool {
				err := k8sClient.List(ctx, nodes)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			Expect(len(nodes.Items)).Should(Equal(1))

			// Now that we have a node and a local model CR ready. It should create download jobs
			jobs := &batchv1.JobList{}
			Eventually(func() bool {
				err := k8sClient.List(ctx, jobs)
				return err == nil && len(jobs.Items) > 0
			}, timeout, interval).Should(BeTrue())
			Eventually(func() bool {
				err := k8sClient.Get(ctx, modelLookupKey, cachedModel)
				if err != nil {
					return false
				}
				if cachedModel.Status.ModelCopies == nil {
					return false
				}
				if cachedModel.Status.NodeStatus[nodeName] != v1alpha1.NodeDownloadPending {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue(), "should create a job to download the model")

			// Now let's update the job status to be successful
			job := jobs.Items[0]
			job.Status.Succeeded = 1
			Expect(k8sClient.Status().Update(ctx, &job)).Should(Succeed())
			Eventually(func() bool {
				err := k8sClient.Get(ctx, modelLookupKey, cachedModel)
				if err != nil {
					return false
				}
				if cachedModel.Status.ModelCopies == nil {
					return false
				}
				if cachedModel.Status.NodeStatus[nodeName] != v1alpha1.NodeDownloaded {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue(), "Node status should be downloaded once the job succeeded")

			// Now let's test deletion
			k8sClient.Delete(ctx, cachedModel)

			Eventually(func() bool {
				err := k8sClient.Get(ctx, modelLookupKey, cachedModel)
				if err != nil {
					return false
				}
				if cachedModel.Status.ModelCopies == nil {
					return false
				}
				if cachedModel.Status.NodeStatus[nodeName] != v1alpha1.NodeDeleting {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue(), "Node status should be deleting")

			// Deletion job should be created
			deletionJob := &batchv1.Job{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "iris-node-1-delete", Namespace: modelCacheNamespace}, deletionJob)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			// Let's we update deletion job status to be successful and check if the cr is deleted.
			deletionJob.Status.Succeeded = 1
			Expect(k8sClient.Status().Update(ctx, deletionJob)).Should(Succeed())

			newLocalModel := &v1alpha1.ClusterLocalModel{}
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
			cachedModel := &v1alpha1.ClusterLocalModel{
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
			cachedModelList := &v1alpha1.ClusterLocalModelList{}
			cachedModelList.Items = []v1alpha1.ClusterLocalModel{*cachedModel}
			isvc.DefaultInferenceService(nil, nil, cachedModelList)

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
})

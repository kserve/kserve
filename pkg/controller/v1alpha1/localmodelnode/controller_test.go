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

package localmodelnode

import (
	"context"
	"time"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
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
		modelName          = "iris"
		localModelNodeSpec = v1alpha1.LocalModelNodeSpec{
			LocalModels: []v1alpha1.LocalModelInfo{
				{
					SourceModelUri: sourceModelUri,
					ModelName:      modelName,
				},
			},
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
		It("Should create download jobs and update model status from jobs", func() {
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

			nodeName = "worker"
			localModelNode := &v1alpha1.LocalModelNode{
				ObjectMeta: metav1.ObjectMeta{
					Name: nodeName,
				},
				Spec: localModelNodeSpec,
			}
			Expect(k8sClient.Create(ctx, localModelNode)).Should(Succeed())
			defer k8sClient.Delete(ctx, localModelNode)

			// Wait for the download job to be created
			job := &batchv1.Job{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: modelName + "-" + nodeName, Namespace: modelCacheNamespace}, job)
				return err == nil
			}, timeout, interval).Should(BeTrue(), "Download job should be created")

			// Now let's update the job status to be successful
			job.Status.Succeeded = 1
			Expect(k8sClient.Status().Update(ctx, job)).Should(Succeed())

			// LocalModelNode status should be updated
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: nodeName}, localModelNode)
				if err != nil {
					return false
				}
				if modelStatus, ok := localModelNode.Status.ModelStatus[modelName]; ok {
					return modelStatus == v1alpha1.ModelDownloaded
				}
				return true
			}, timeout, interval).Should(BeTrue(), "LocaModelNode status should be downloaded")
			Expect(localModelNode.Spec).Should(Equal(localModelNodeSpec), "spec should not be changed")
		})
	})
})

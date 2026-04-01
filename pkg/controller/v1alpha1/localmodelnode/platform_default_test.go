//go:build !distro

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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
)

var _ = Describe("LocalModelNode default platform", func() {
	const (
		timeout        = time.Second * 10
		interval       = time.Millisecond * 250
		sourceModelUri = "s3://mybucket/mymodel"
	)
	var (
		modelName = "iris"
		configs   = map[string]string{
			"localModel": `{
				"jobNamespace": "kserve-localmodel-jobs",
				"defaultJobImage": "kserve/storage-initializer:latest"
			}`,
			"storageInitializer": `{
				"image": "kserve/storage-initializer:latest",
				"cpuRequest": "100m",
				"cpuLimit": "1",
				"memoryRequest": "200Mi",
				"memoryLimit": "1Gi"
			}`,
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
		localModelNodeGroupSpec = v1alpha1.LocalModelNodeGroupSpec{
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
										Values:   []string{"gpu"},
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

	Context("When using default platform hooks", func() {
		It("Should use storageKey hash as the download job SubPath for storage deduplication", func() {
			ctx, cancel := context.WithCancel(context.Background())
			DeferCleanup(cancel)
			fsMock.clear()

			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KServeNamespace,
				},
				Data: configs,
			}
			Expect(k8sClient.Create(ctx, configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(ctx, configMap)

			clusterStorageContainer := &v1alpha1.ClusterStorageContainer{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-default-subpath",
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

			nodeName = "worker-default-subpath"
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: nodeName,
					Labels: map[string]string{
						"node.kubernetes.io/instance-type": "gpu",
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

			localModelNode := &v1alpha1.LocalModelNode{
				ObjectMeta: metav1.ObjectMeta{
					Name: nodeName,
				},
				Spec: v1alpha1.LocalModelNodeSpec{
					LocalModels: []v1alpha1.LocalModelInfo{
						{
							SourceModelUri: sourceModelUri,
							ModelName:      modelName,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, localModelNode)).Should(Succeed())
			defer k8sClient.Delete(ctx, localModelNode)

			// Wait for the download job to be created
			jobs := &batchv1.JobList{}
			labelSelector := map[string]string{
				"model": modelName,
				"node":  nodeName,
			}
			Eventually(func() bool {
				err := k8sClient.List(ctx, jobs, client.InNamespace("kserve-localmodel-jobs"), client.MatchingLabels(labelSelector))
				return err == nil && len(jobs.Items) == 1
			}, timeout, interval).Should(BeTrue(), "Download job should be created")

			// Verify the download job SubPath uses storageKey (hash), not modelName
			storageKey := v1alpha1.GetStorageKey(sourceModelUri)
			job := &jobs.Items[0]
			Expect(job.Spec.Template.Spec.Containers).To(HaveLen(1))
			container := job.Spec.Template.Spec.Containers[0]
			Expect(container.VolumeMounts).To(HaveLen(1))
			Expect(container.VolumeMounts[0].SubPath).To(Equal("models/"+storageKey),
				"Download job SubPath must use storageKey hash, not modelName, for storage deduplication. "+
					"storageKey=%s, modelName=%s", storageKey, modelName)
			Expect(container.Args).To(Equal([]string{sourceModelUri, MountPath}),
				"Download destination should be MountPath on default platform")
		})
	})
})

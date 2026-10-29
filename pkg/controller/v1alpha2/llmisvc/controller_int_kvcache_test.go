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

package llmisvc_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
)

var _ = Describe("LLMInferenceService Controller - KV Cache secondary tiers", func() {
	Context("Single node", func() {
		It("should inject an emptyDir volume for a secondary emptyDir tier", func(ctx SpecContext) {
			// given
			svcName := "test-llm-kvcache-emptydir"
			testNs := NewTestNamespace(ctx, envTest)

			modelURL, err := apis.ParseURL("pvc://facebook-models/opt-125m")
			Expect(err).ToNot(HaveOccurred())

			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      svcName,
					Namespace: testNs.Name,
				},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Model: v1alpha2.LLMModelSpec{
						Name: ptr.To("foo"),
						URI:  *modelURL,
					},
					WorkloadSpec: v1alpha2.WorkloadSpec{
						KVCacheOffloading: &v1alpha2.KVCacheOffloadingSpec{
							CPU: resource.MustParse("10Gi"),
							Secondary: []v1alpha2.SecondaryTierSpec{
								{FileSystem: &v1alpha2.FileSystemTierSpec{
									EmptyDir: &v1alpha2.EmptyDirTierSpec{
										Size: resource.MustParse("100Gi"),
									},
								}},
							},
						},
					},
					Router: &v1alpha2.RouterSpec{
						Route:     &v1alpha2.GatewayRoutesSpec{},
						Gateway:   &v1alpha2.GatewaySpec{},
						Scheduler: &v1alpha2.SchedulerSpec{},
					},
					Prefill: &v1alpha2.WorkloadSpec{},
				},
			}

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// then
			deployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve",
					Namespace: testNs.Name,
				}, deployment)
			}).WithContext(ctx).Should(Succeed())

			sizeLimit := resource.MustParse("100Gi")
			Expect(deployment.Spec.Template.Spec.Volumes).To(ContainElement(And(
				HaveField("Name", "kv-cache-secondary-0"),
				HaveField("VolumeSource.EmptyDir.SizeLimit", &sizeLimit),
			)))

			mainContainer := findContainer(deployment, "main")
			Expect(mainContainer).ToNot(BeNil())
			Expect(mainContainer.VolumeMounts).To(ContainElement(And(
				HaveField("Name", "kv-cache-secondary-0"),
				HaveField("MountPath", "/mnt/kv-cache-0"),
			)))
		})

		It("should remove the reserved pvc-evictor container when KV-cache offloading is not configured", func(ctx SpecContext) {
			svcName := "test-llm-kvcache-no-offloading"
			testNs := NewTestNamespace(ctx, envTest)

			modelURL, err := apis.ParseURL("pvc://facebook-models/opt-125m")
			Expect(err).ToNot(HaveOccurred())

			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      svcName,
					Namespace: testNs.Name,
				},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Model: v1alpha2.LLMModelSpec{
						Name: ptr.To("foo"),
						URI:  *modelURL,
					},
					WorkloadSpec: v1alpha2.WorkloadSpec{
						Template: &corev1.PodSpec{
							Containers: []corev1.Container{
								{Name: "main"},
								{Name: "pvc-evictor", Image: "example.com/evictor:v1.2.3"},
							},
						},
					},
					Router: &v1alpha2.RouterSpec{
						Route:     &v1alpha2.GatewayRoutesSpec{},
						Gateway:   &v1alpha2.GatewaySpec{},
						Scheduler: &v1alpha2.SchedulerSpec{},
					},
					Prefill: &v1alpha2.WorkloadSpec{},
				},
			}

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			deployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve",
					Namespace: testNs.Name,
				}, deployment)
			}).WithContext(ctx).Should(Succeed())

			Expect(findContainer(deployment, "pvc-evictor")).To(BeNil())
		})

		It("should inject an ephemeral PVC volume for a secondary pvc.spec tier", func(ctx SpecContext) {
			// given
			svcName := "test-llm-kvcache-pvc-spec"
			testNs := NewTestNamespace(ctx, envTest)

			modelURL, err := apis.ParseURL("pvc://facebook-models/opt-125m")
			Expect(err).ToNot(HaveOccurred())

			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      svcName,
					Namespace: testNs.Name,
				},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Model: v1alpha2.LLMModelSpec{
						Name: ptr.To("foo"),
						URI:  *modelURL,
					},
					WorkloadSpec: v1alpha2.WorkloadSpec{
						Template: &corev1.PodSpec{
							Containers: []corev1.Container{
								{Name: "main"},
								{
									Name:            "pvc-evictor",
									Image:           "example.com/evictor:v1.2.3",
									ImagePullPolicy: corev1.PullIfNotPresent,
									Resources: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceCPU: resource.MustParse("123m"),
										},
									},
								},
							},
						},
						KVCacheOffloading: &v1alpha2.KVCacheOffloadingSpec{
							CPU: resource.MustParse("10Gi"),
							Secondary: []v1alpha2.SecondaryTierSpec{
								{FileSystem: &v1alpha2.FileSystemTierSpec{
									PVC: &v1alpha2.PVCTierSpec{
										Spec: &corev1.PersistentVolumeClaimSpec{
											StorageClassName: ptr.To("fast-nvme"),
											AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
											Resources: corev1.VolumeResourceRequirements{
												Requests: corev1.ResourceList{
													corev1.ResourceStorage: resource.MustParse("200Gi"),
												},
											},
										},
									},
								}},
							},
						},
					},
					Router: &v1alpha2.RouterSpec{
						Route:     &v1alpha2.GatewayRoutesSpec{},
						Gateway:   &v1alpha2.GatewaySpec{},
						Scheduler: &v1alpha2.SchedulerSpec{},
					},
					Prefill: &v1alpha2.WorkloadSpec{},
				},
			}

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// then
			deployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve",
					Namespace: testNs.Name,
				}, deployment)
			}).WithContext(ctx).Should(Succeed())

			Expect(deployment.Spec.Template.Spec.Volumes).To(ContainElement(And(
				HaveField("Name", "kv-cache-secondary-0"),
				HaveField("VolumeSource.Ephemeral.VolumeClaimTemplate.Spec.StorageClassName", ptr.To("fast-nvme")),
				HaveField("VolumeSource.Ephemeral.VolumeClaimTemplate.Spec.AccessModes",
					ContainElement(corev1.ReadWriteOnce)),
			)))

			mainContainer := findContainer(deployment, "main")
			Expect(mainContainer).ToNot(BeNil())
			Expect(mainContainer.VolumeMounts).To(ContainElement(And(
				HaveField("Name", "kv-cache-secondary-0"),
				HaveField("MountPath", "/mnt/kv-cache-0"),
			)))

			evictorContainer := findContainer(deployment, "pvc-evictor-kv-cache-secondary-0")
			Expect(evictorContainer).ToNot(BeNil())
			Expect(evictorContainer.Image).To(Equal("example.com/evictor:v1.2.3"))
			Expect(evictorContainer.ImagePullPolicy).To(Equal(corev1.PullIfNotPresent))
			Expect(evictorContainer.Resources.Requests.Cpu().String()).To(Equal("123m"))
			Expect(findContainer(deployment, "pvc-evictor")).To(BeNil())
		})

		It("should inject a pre-existing PVC volume for a secondary pvc.ref tier", func(ctx SpecContext) {
			// given
			svcName := "test-llm-kvcache-pvc-ref"
			testNs := NewTestNamespace(ctx, envTest)
			pvcName := "shared-kv-cache-pvc"

			pvc := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{Name: pvcName, Namespace: testNs.Name},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse("1Gi"),
						},
					},
				},
			}
			Expect(envTest.Client.Create(ctx, pvc)).To(Succeed())
			defer func() {
				_ = envTest.Delete(ctx, pvc)
			}()

			modelURL, err := apis.ParseURL("pvc://facebook-models/opt-125m")
			Expect(err).ToNot(HaveOccurred())

			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      svcName,
					Namespace: testNs.Name,
				},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Model: v1alpha2.LLMModelSpec{
						Name: ptr.To("foo"),
						URI:  *modelURL,
					},
					WorkloadSpec: v1alpha2.WorkloadSpec{
						KVCacheOffloading: &v1alpha2.KVCacheOffloadingSpec{
							CPU: resource.MustParse("10Gi"),
							Secondary: []v1alpha2.SecondaryTierSpec{
								{FileSystem: &v1alpha2.FileSystemTierSpec{
									PVC: &v1alpha2.PVCTierSpec{
										Ref: &v1alpha2.PVCRefTierSpec{
											Name: "shared-kv-cache-pvc",
											Path: "kv/",
										},
									},
								}},
							},
						},
					},
					Router: &v1alpha2.RouterSpec{
						Route:     &v1alpha2.GatewayRoutesSpec{},
						Gateway:   &v1alpha2.GatewaySpec{},
						Scheduler: &v1alpha2.SchedulerSpec{},
					},
					Prefill: &v1alpha2.WorkloadSpec{},
				},
			}

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// then
			deployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve",
					Namespace: testNs.Name,
				}, deployment)
			}).WithContext(ctx).Should(Succeed())

			Expect(deployment.Spec.Template.Spec.Volumes).To(ContainElement(And(
				HaveField("Name", "kv-cache-secondary-0"),
				HaveField("VolumeSource.PersistentVolumeClaim.ClaimName", "shared-kv-cache-pvc"),
			)))

			mainContainer := findContainer(deployment, "main")
			Expect(mainContainer).ToNot(BeNil())
			Expect(mainContainer.VolumeMounts).To(ContainElement(And(
				HaveField("Name", "kv-cache-secondary-0"),
				HaveField("MountPath", "/mnt/kv-cache-0"),
				HaveField("SubPath", "kv/"),
			)))
		})

		It("should create and clean up a shared evictor Deployment using the container template", func(ctx SpecContext) {
			svcName := "test-llm-kvcache-pvc-ref-evictor"
			testNs := NewTestNamespace(ctx, envTest)
			pvcName := "shared-kv-cache-evictor-pvc"

			pvc := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{Name: pvcName, Namespace: testNs.Name},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse("1Gi"),
						},
					},
				},
			}
			Expect(envTest.Client.Create(ctx, pvc)).To(Succeed())
			defer func() {
				_ = envTest.Delete(ctx, pvc)
			}()

			modelURL, err := apis.ParseURL("pvc://facebook-models/opt-125m")
			Expect(err).ToNot(HaveOccurred())
			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{Name: svcName, Namespace: testNs.Name},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Model: v1alpha2.LLMModelSpec{Name: ptr.To("foo"), URI: *modelURL},
					WorkloadSpec: v1alpha2.WorkloadSpec{
						Template: &corev1.PodSpec{
							SecurityContext: &corev1.PodSecurityContext{FSGroup: ptr.To(int64(1000))},
							Containers: []corev1.Container{
								{Name: "main"},
								{
									Name:            "pvc-evictor",
									Image:           "example.com/evictor:v1.2.3",
									ImagePullPolicy: corev1.PullIfNotPresent,
									Resources: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceCPU: resource.MustParse("321m"),
										},
									},
								},
							},
						},
						KVCacheOffloading: &v1alpha2.KVCacheOffloadingSpec{
							CPU: resource.MustParse("10Gi"),
							Secondary: []v1alpha2.SecondaryTierSpec{{
								FileSystem: &v1alpha2.FileSystemTierSpec{
									PVC: &v1alpha2.PVCTierSpec{
										Ref: &v1alpha2.PVCRefTierSpec{Name: pvcName, Path: "kv/"},
									},
								},
							}},
						},
					},
					Router:  &v1alpha2.RouterSpec{Route: &v1alpha2.GatewayRoutesSpec{}, Gateway: &v1alpha2.GatewaySpec{}, Scheduler: &v1alpha2.SchedulerSpec{}},
					Prefill: &v1alpha2.WorkloadSpec{},
				},
			}
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() { testNs.DeleteAndWait(ctx, llmSvc) }()

			deployments := &appsv1.DeploymentList{}
			Eventually(func(g Gomega, ctx context.Context) {
				g.Expect(envTest.List(ctx, deployments,
					client.InNamespace(testNs.Name),
					client.MatchingLabels{"serving.kserve.io/pvc-evictor": "true"},
				)).To(Succeed())
				g.Expect(deployments.Items).To(HaveLen(1))
			}).WithContext(ctx).Should(Succeed())

			deployment := deployments.Items[0]
			Expect(deployment.Name).To(HavePrefix("kserve-llm-isvc-kv-pvc-evictor-"))
			Expect(deployment.Spec.Template.Spec.ServiceAccountName).To(Equal("kserve-llm-isvc-kv-pvc-evictor"))
			Expect(deployment.Spec.Template.Spec.Containers[0].Image).To(Equal("example.com/evictor:v1.2.3"))
			Expect(deployment.Spec.Template.Spec.Containers[0].ImagePullPolicy).To(Equal(corev1.PullIfNotPresent))
			Expect(deployment.Spec.Template.Spec.Containers[0].Resources.Requests.Cpu().String()).To(Equal("321m"))
			Expect(deployment.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim.ClaimName).To(Equal(pvcName))
			Expect(deployment.Spec.Template.Spec.SecurityContext.FSGroup).To(Equal(ptr.To(int64(1000))))

			mainDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{Name: svcName + "-kserve", Namespace: testNs.Name}, mainDeployment)
			}).WithContext(ctx).Should(Succeed())
			Expect(findContainer(mainDeployment, "pvc-evictor")).To(BeNil())

			testNs.DeleteAndWait(ctx, llmSvc)
			Eventually(func(g Gomega, ctx context.Context) error {
				deleting := &appsv1.Deployment{}
				if err := envTest.Get(ctx, client.ObjectKeyFromObject(&deployment), deleting); err != nil {
					return err
				}
				if deleting.DeletionTimestamp.IsZero() {
					return errors.New("shared evictor Deployment is not deleting")
				}
				controllerutil.RemoveFinalizer(deleting, metav1.FinalizerDeleteDependents)
				return envTest.Update(ctx, deleting)
			}).WithContext(ctx).Should(Succeed())
			Eventually(func(g Gomega, ctx context.Context) {
				g.Expect(envTest.List(ctx, deployments,
					client.InNamespace(testNs.Name),
					client.MatchingLabels{"serving.kserve.io/pvc-evictor": "true"},
				)).To(Succeed())
				g.Expect(deployments.Items).To(BeEmpty())
			}).WithContext(ctx).Should(Succeed())
		})

		It("should delete the old RWO evictor before creating a replacement", func(ctx SpecContext) {
			svcName := "test-llm-kvcache-pvc-ref-migration"
			testNs := NewTestNamespace(ctx, envTest)
			pvcName := "shared-kv-cache-migration-pvc"

			pvc := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{Name: pvcName, Namespace: testNs.Name},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
					},
				},
			}
			Expect(envTest.Client.Create(ctx, pvc)).To(Succeed())
			defer func() { _ = envTest.Delete(ctx, pvc) }()

			modelURL, err := apis.ParseURL("pvc://facebook-models/opt-125m")
			Expect(err).ToNot(HaveOccurred())
			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{Name: svcName, Namespace: testNs.Name},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Model: v1alpha2.LLMModelSpec{Name: ptr.To("foo"), URI: *modelURL},
					WorkloadSpec: v1alpha2.WorkloadSpec{KVCacheOffloading: &v1alpha2.KVCacheOffloadingSpec{
						CPU: resource.MustParse("10Gi"),
						Secondary: []v1alpha2.SecondaryTierSpec{{FileSystem: &v1alpha2.FileSystemTierSpec{
							PVC: &v1alpha2.PVCTierSpec{Ref: &v1alpha2.PVCRefTierSpec{Name: pvcName, Path: "old-cache"}},
						}}},
					}},
					Router:  &v1alpha2.RouterSpec{Route: &v1alpha2.GatewayRoutesSpec{}, Gateway: &v1alpha2.GatewaySpec{}, Scheduler: &v1alpha2.SchedulerSpec{}},
					Prefill: &v1alpha2.WorkloadSpec{},
				},
			}
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() { testNs.DeleteAndWait(ctx, llmSvc) }()

			oldDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				deployments := &appsv1.DeploymentList{}
				if err := envTest.List(ctx, deployments, client.InNamespace(testNs.Name), client.MatchingLabels{"serving.kserve.io/pvc-evictor": "true"}); err != nil {
					return err
				}
				if len(deployments.Items) != 1 {
					return fmt.Errorf("evictor Deployment count = %d, want 1", len(deployments.Items))
				}
				*oldDeployment = deployments.Items[0]
				return nil
			}).WithContext(ctx).Should(Succeed())

			controller := true
			blockOwnerDeletion := true
			blocker := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "old-evictor-pod",
					Namespace: testNs.Name,
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion:         "apps/v1",
						Kind:               "Deployment",
						Name:               oldDeployment.Name,
						UID:                oldDeployment.UID,
						Controller:         &controller,
						BlockOwnerDeletion: &blockOwnerDeletion,
					}},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyAlways,
					Containers: []corev1.Container{{
						Name: "pvc-evictor", Image: "example.com/evictor:test",
						VolumeMounts: []corev1.VolumeMount{{Name: "cache", MountPath: "/cache"}},
					}},
					Volumes: []corev1.Volume{{Name: "cache", VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: pvcName},
					}}},
				},
			}
			Expect(envTest.Create(ctx, blocker)).To(Succeed())

			current := &v1alpha2.LLMInferenceService{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{Name: svcName, Namespace: testNs.Name}, current)
			}).WithContext(ctx).Should(Succeed())
			current.Spec.KVCacheOffloading.Secondary[0].FileSystem.PVC.Ref.Path = "new-cache"
			Expect(envTest.Update(ctx, current)).To(Succeed())

			Eventually(func(g Gomega, ctx context.Context) error {
				updated := &appsv1.Deployment{}
				if err := envTest.Get(ctx, client.ObjectKeyFromObject(oldDeployment), updated); err != nil {
					return err
				}
				if updated.DeletionTimestamp.IsZero() {
					return errors.New("old evictor Deployment is not deleting")
				}
				return nil
			}).WithContext(ctx).Should(Succeed())

			Consistently(func(g Gomega, ctx context.Context) {
				deployments := &appsv1.DeploymentList{}
				g.Expect(envTest.List(ctx, deployments, client.InNamespace(testNs.Name), client.MatchingLabels{"serving.kserve.io/pvc-evictor": "true"})).To(Succeed())
				g.Expect(deployments.Items).To(HaveLen(1))
				g.Expect(deployments.Items[0].Name).To(Equal(oldDeployment.Name))
			}).WithContext(ctx).WithTimeout(2 * time.Second).Should(Succeed())

			Expect(envTest.Delete(ctx, blocker)).To(Succeed())
			Eventually(func(g Gomega, ctx context.Context) error {
				old := &appsv1.Deployment{}
				if err := envTest.Get(ctx, client.ObjectKeyFromObject(oldDeployment), old); apierrors.IsNotFound(err) {
					return nil
				} else if err != nil {
					return err
				}
				old.Finalizers = nil
				return envTest.Update(ctx, old)
			}).WithContext(ctx).Should(Succeed())

			Eventually(func(g Gomega, ctx context.Context) error {
				deployments := &appsv1.DeploymentList{}
				if err := envTest.List(ctx, deployments, client.InNamespace(testNs.Name), client.MatchingLabels{"serving.kserve.io/pvc-evictor": "true"}); err != nil {
					return err
				}
				if len(deployments.Items) != 1 || deployments.Items[0].Name == oldDeployment.Name {
					return fmt.Errorf("replacement evictor Deployment is not ready to create: %#v", deployments.Items)
				}
				return nil
			}).WithContext(ctx).Should(Succeed())
		})
	})
})

func findContainer(deployment *appsv1.Deployment, name string) *corev1.Container {
	for i := range deployment.Spec.Template.Spec.Containers {
		if deployment.Spec.Template.Spec.Containers[i].Name == name {
			return &deployment.Spec.Template.Spec.Containers[i]
		}
	}
	return nil
}

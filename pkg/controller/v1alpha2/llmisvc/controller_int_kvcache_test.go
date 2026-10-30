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
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/yaml"

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
			Expect(findContainer(deployment, "pvc-evictor")).To(BeNil())
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
					// The prefill presets do not declare the container, so only a spec
					// that does reaches the prefill take.
					Prefill: &v1alpha2.WorkloadSpec{
						Template: &corev1.PodSpec{
							Containers: []corev1.Container{
								{Name: "main"},
								{Name: "pvc-evictor", Image: "example.com/evictor:v1.2.3"},
							},
						},
					},
				},
			}

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			for _, name := range []string{svcName + "-kserve", svcName + "-kserve-prefill"} {
				deployment := &appsv1.Deployment{}
				Eventually(func(g Gomega, ctx context.Context) error {
					return envTest.Get(ctx, types.NamespacedName{
						Name:      name,
						Namespace: testNs.Name,
					}, deployment)
				}).WithContext(ctx).Should(Succeed())

				Expect(findContainer(deployment, "pvc-evictor")).To(BeNil(), "Deployment %s", name)
			}
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

			Expect(retry.RetryOnConflict(retry.DefaultRetry, func() error {
				current := &v1alpha2.LLMInferenceService{}
				if err := envTest.Get(ctx, types.NamespacedName{Name: svcName, Namespace: testNs.Name}, current); err != nil {
					return err
				}
				current.Spec.KVCacheOffloading.Secondary[0].FileSystem.PVC.Ref.Path = "new-cache"
				return envTest.Update(ctx, current)
			})).To(Succeed())

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

		// A service keeps resolving to the preset it was created against after a
		// controller upgrade, so a pvc.ref tier on a preset released without the
		// pvc-evictor container has to render as that release did: no shared
		// evictor, and no consumer label, which would restart the workload.
		Context("pvc.ref tier and the preset's pvc-evictor container", func() {
			const pvcName = "shared-kv-cache-pvc"
			evictorLabels := client.MatchingLabels{"serving.kserve.io/pvc-evictor": "true"}

			createPVCRefService := func(ctx context.Context, testNs *TestNamespace, svcName string) *v1alpha2.LLMInferenceService {
				pvc := &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{Name: pvcName, Namespace: testNs.Name},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
						},
					},
				}
				Expect(envTest.Client.Create(ctx, pvc)).To(Succeed())
				DeferCleanup(func(ctx context.Context) { _ = envTest.Delete(ctx, pvc) })

				llmSvc := LLMInferenceService(svcName,
					InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
					WithModelURI("pvc://facebook-models/opt-125m"),
					WithModelName("foo"),
				)
				llmSvc.Spec.KVCacheOffloading = &v1alpha2.KVCacheOffloadingSpec{
					CPU: resource.MustParse("10Gi"),
					Secondary: []v1alpha2.SecondaryTierSpec{{FileSystem: &v1alpha2.FileSystemTierSpec{
						PVC: &v1alpha2.PVCTierSpec{Ref: &v1alpha2.PVCRefTierSpec{Name: pvcName, Path: "kv/"}},
					}}},
				}
				Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
				DeferCleanup(func(ctx context.Context) { testNs.DeleteAndWait(ctx, llmSvc) })
				return llmSvc
			}

			It("should not create a PVC evictor on a preset released without the container", func(ctx SpecContext) {
				// given
				testNs := NewTestNamespace(ctx, envTest)
				// Shadows the shipped preset of the same name for this namespace only.
				preset := releasedLLMTemplatePreset()
				preset.Namespace = testNs.Name
				Expect(envTest.Create(ctx, preset)).To(Succeed())

				// when
				llmSvc := createPVCRefService(ctx, testNs, "test-llm-kvcache-pvc-ref-released-preset")

				// then
				workloadKey := types.NamespacedName{Name: llmSvc.Name + "-kserve", Namespace: testNs.Name}
				deployment := &appsv1.Deployment{}
				// status.workloads is written after the evictor pass, so seeing it
				// means that pass has run at least once.
				Eventually(func(g Gomega, ctx context.Context) {
					current := &v1alpha2.LLMInferenceService{}
					g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
					g.Expect(current.Status.Workloads).ToNot(BeNil())
					g.Expect(envTest.Get(ctx, workloadKey, deployment)).To(Succeed())
				}).WithContext(ctx).Should(Succeed())

				main := findContainer(deployment, "main")
				Expect(main).ToNot(BeNil())
				Expect(main.Env).ToNot(ContainElement(HaveField("Name", "KSERVE_KV_TRANSFER_ARGS")),
					"the released preset renders the argument itself; a slot means the shipped preset was used")
				Expect(main.VolumeMounts).To(ContainElement(And(
					HaveField("Name", "kv-cache-secondary-0"),
					HaveField("MountPath", "/mnt/kv-cache-0"),
					HaveField("SubPath", "kv/"),
				)))
				Expect(pvcEvictorConsumerLabels(deployment.Spec.Template.Labels)).To(BeEmpty())
				evictors := &appsv1.DeploymentList{}
				Expect(envTest.List(ctx, evictors, client.InNamespace(testNs.Name), evictorLabels)).To(Succeed())
				Expect(evictors.Items).To(BeEmpty())
				serviceAccount := &corev1.ServiceAccount{}
				err := envTest.Get(ctx, types.NamespacedName{Name: "kserve-llm-isvc-kv-pvc-evictor", Namespace: testNs.Name}, serviceAccount)
				Expect(apierrors.IsNotFound(err)).To(BeTrue(), "PVC evictor ServiceAccount: %v", err)

				var settledResourceVersion string
				Eventually(func(g Gomega, ctx context.Context) {
					g.Expect(envTest.Get(ctx, workloadKey, deployment)).To(Succeed())
					previous := settledResourceVersion
					settledResourceVersion = deployment.ResourceVersion
					g.Expect(settledResourceVersion).To(Equal(previous), "workload Deployment is still being written")
				}).WithContext(ctx).Should(Succeed())
				settledGeneration := deployment.Generation

				// An empty baseRef bumps the service generation without changing the
				// merged spec, so observedGeneration catching up proves a reconcile
				// ran against the same workload.
				trigger := LLMInferenceServiceConfig("empty-override", InNamespace[*v1alpha2.LLMInferenceServiceConfig](testNs.Name))
				Expect(envTest.Create(ctx, trigger)).To(Succeed())
				current := &v1alpha2.LLMInferenceService{}
				Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
				patch := client.MergeFrom(current.DeepCopy())
				current.Spec.BaseRefs = append(current.Spec.BaseRefs, corev1.LocalObjectReference{Name: trigger.Name})
				Expect(envTest.Patch(ctx, current, patch)).To(Succeed())
				Eventually(func(g Gomega, ctx context.Context) {
					reconciled := &v1alpha2.LLMInferenceService{}
					g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), reconciled)).To(Succeed())
					g.Expect(reconciled.Generation).To(BeNumerically(">", llmSvc.Generation))
					g.Expect(reconciled.Status.ObservedGeneration).To(Equal(reconciled.Generation))
				}).WithContext(ctx).Should(Succeed())

				Consistently(func(g Gomega, ctx context.Context) {
					current := &appsv1.Deployment{}
					g.Expect(envTest.Get(ctx, workloadKey, current)).To(Succeed())
					g.Expect(current.Generation).To(Equal(settledGeneration))
					g.Expect(pvcEvictorConsumerLabels(current.Spec.Template.Labels)).To(BeEmpty())
					evictors := &appsv1.DeploymentList{}
					g.Expect(envTest.List(ctx, evictors, client.InNamespace(testNs.Name), evictorLabels)).To(Succeed())
					g.Expect(evictors.Items).To(BeEmpty())
				}).WithContext(ctx).WithTimeout(2 * time.Second).Should(Succeed())
			})

			It("should create a PVC evictor on the shipped preset, which declares the container", func(ctx SpecContext) {
				// given
				testNs := NewTestNamespace(ctx, envTest)

				// when
				llmSvc := createPVCRefService(ctx, testNs, "test-llm-kvcache-pvc-ref-shipped-preset")

				// then
				Eventually(func(g Gomega, ctx context.Context) {
					deployment := &appsv1.Deployment{}
					g.Expect(envTest.Get(ctx, types.NamespacedName{Name: llmSvc.Name + "-kserve", Namespace: testNs.Name}, deployment)).To(Succeed())
					g.Expect(pvcEvictorConsumerLabels(deployment.Spec.Template.Labels)).To(HaveLen(1))
					evictors := &appsv1.DeploymentList{}
					g.Expect(envTest.List(ctx, evictors, client.InNamespace(testNs.Name), evictorLabels)).To(Succeed())
					g.Expect(evictors.Items).To(HaveLen(1))
					g.Expect(envTest.Get(ctx, types.NamespacedName{Name: "kserve-llm-isvc-kv-pvc-evictor", Namespace: testNs.Name}, &corev1.ServiceAccount{})).To(Succeed())
				}).WithContext(ctx).Should(Succeed())
			})
		})
	})
})

// releasedLLMTemplatePreset loads kserve-config-llm-template as v0.21.0 shipped it,
// before the pvc-evictor container and the KSERVE_KV_TRANSFER_ARGS slot existed.
func releasedLLMTemplatePreset() *v1alpha2.LLMInferenceServiceConfig {
	GinkgoHelper()
	data, err := os.ReadFile(filepath.Join("testdata", "config-llm-template-v0.21.0.yaml"))
	Expect(err).ToNot(HaveOccurred())
	preset := &v1alpha2.LLMInferenceServiceConfig{}
	Expect(yaml.Unmarshal(data, preset)).To(Succeed())
	return preset
}

func pvcEvictorConsumerLabels(labels map[string]string) map[string]string {
	consumers := map[string]string{}
	for key, value := range labels {
		if strings.HasPrefix(key, "serving.kserve.io/pvc-evictor-consumer-") {
			consumers[key] = value
		}
	}
	return consumers
}

func findContainer(deployment *appsv1.Deployment, name string) *corev1.Container {
	for i := range deployment.Spec.Template.Spec.Containers {
		if deployment.Spec.Template.Spec.Containers[i].Name == name {
			return &deployment.Spec.Template.Spec.Containers[i]
		}
	}
	return nil
}

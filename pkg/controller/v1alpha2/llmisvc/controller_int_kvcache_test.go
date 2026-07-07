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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/apis"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
)

var _ = Describe("LLMInferenceService Controller - KV Cache secondary tiers", func() {
	Context("Single node", func() {
		It("should inject an emptyDir volume for a secondary emptyDir tier", func(ctx SpecContext) {
			// given
			svcName := "test-llm-kvcache-emptydir"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

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

		It("should inject an ephemeral PVC volume for a secondary pvc.spec tier", func(ctx SpecContext) {
			// given
			svcName := "test-llm-kvcache-pvc-spec"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

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
		})

		It("should inject a pre-existing PVC volume for a secondary pvc.ref tier", func(ctx SpecContext) {
			// given
			svcName := "test-llm-kvcache-pvc-ref"
			testNs := NewTestNamespace(ctx, envTest, WithIstioShadowService(svcName))

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

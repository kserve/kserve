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

package inferenceservice

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/proto"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/credentials/s3"
)

var _ = Describe("Canary deployment controller", func() {
	configs := getRawKubeTestConfigs()

	makeCanaryISVC := func(name, namespace, stableURI string, minReplicas int32, canaries []v1beta1.CanarySpec) *v1beta1.InferenceService {
		return &v1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:        name,
				Namespace:   namespace,
				Annotations: getDefaultAnnotations(constants.AutoscalerClassNone),
			},
			Spec: v1beta1.InferenceServiceSpec{
				Predictor: v1beta1.PredictorSpec{
					ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
						MinReplicas: ptr.To(minReplicas),
					},
					Model: &v1beta1.ModelSpec{
						ModelFormat: v1beta1.ModelFormat{Name: "tensorflow"},
						PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
							StorageURI:     proto.String(stableURI),
							RuntimeVersion: proto.String("0.14.0"),
						},
					},
				},
				Canary: canaries,
			},
		}
	}

	makeCanary := func(name string, traffic int32, uri string) v1beta1.CanarySpec {
		return v1beta1.CanarySpec{
			TrafficPercent: traffic,
			Predictor: v1beta1.PredictorSpec{
				Name: name,
				Model: &v1beta1.ModelSpec{
					ModelFormat: v1beta1.ModelFormat{Name: "tensorflow"},
					PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
						StorageURI:     proto.String(uri),
						RuntimeVersion: proto.String("0.14.0"),
					},
				},
			},
		}
	}

	// markCanaryAvailable simulates the Deployment controller by setting
	// AvailableReplicas on the canary Deployment so that the KServe controller
	// marks the canary as Ready and reduces the stable minReplicas accordingly.
	markCanaryAvailable := func(ctx context.Context, key types.NamespacedName) {
		Eventually(func() error {
			deploy := &appsv1.Deployment{}
			if err := k8sClient.Get(ctx, key, deploy); err != nil {
				return err
			}
			replicas := int32(1)
			if deploy.Spec.Replicas != nil {
				replicas = *deploy.Spec.Replicas
			}
			updated := deploy.DeepCopy()
			updated.Status.AvailableReplicas = replicas
			updated.Status.ReadyReplicas = replicas
			updated.Status.Replicas = replicas
			updated.Status.UpdatedReplicas = replicas
			updated.Status.Conditions = []appsv1.DeploymentCondition{
				{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue},
				{Type: appsv1.DeploymentProgressing, Status: corev1.ConditionTrue, Reason: "NewReplicaSetAvailable"},
			}
			return k8sClient.Status().Update(ctx, updated)
		}, timeout, interval).Should(Succeed())
	}

	Context("When creating an InferenceService with a canary", func() {
		It("Should mount each predictor's own PVC from storageUris", func(ctx SpecContext) {
			configMap := createInferenceServiceConfigMap(configs)
			Expect(k8sClient.Create(ctx, configMap)).To(Succeed())
			defer k8sClient.Delete(ctx, configMap)

			namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "canary-storage-"}}
			Expect(k8sClient.Create(ctx, namespace)).To(Succeed())
			defer k8sClient.Delete(ctx, namespace)

			servingRuntime := getServingRuntime("tf-canary-storage", namespace.Name)
			Expect(k8sClient.Create(ctx, &servingRuntime)).To(Succeed())
			defer k8sClient.Delete(ctx, &servingRuntime)

			isvc := makeCanaryISVC("canary-storage-test", namespace.Name, "", 5,
				[]v1beta1.CanarySpec{makeCanary("v2", 20, "")})
			isvc.Spec.Predictor.Model.StorageURI = nil
			isvc.Spec.Predictor.StorageUris = []v1beta1.StorageUri{{Uri: "pvc://iris-models-v1", MountPath: "/mnt/models"}}
			isvc.Spec.Canary[0].Predictor.Model.StorageURI = nil
			isvc.Spec.Canary[0].Predictor.StorageUris = []v1beta1.StorageUri{{Uri: "pvc://iris-models-v2", MountPath: "/mnt/models"}}
			isvc.DefaultInferenceService(nil, nil, &v1beta1.SecurityConfig{AutoMountServiceAccountToken: false}, nil, nil)
			Expect(k8sClient.Create(ctx, isvc)).To(Succeed())
			defer k8sClient.Delete(ctx, isvc)

			for _, predictor := range []struct {
				name  string
				claim string
			}{
				{name: "", claim: "iris-models-v1"},
				{name: "v2", claim: "iris-models-v2"},
			} {
				key := types.NamespacedName{Name: constants.PredictorServiceName(isvc.Name, predictor.name), Namespace: namespace.Name}
				deployment := &appsv1.Deployment{}
				Eventually(func() error {
					return k8sClient.Get(ctx, key, deployment)
				}, timeout, interval).Should(Succeed())
				podSpec := deployment.Spec.Template.Spec
				Expect(podSpec.Volumes).To(HaveLen(1), "predictor %q should mount %s", predictor.name, predictor.claim)
				Expect(podSpec.Volumes[0].PersistentVolumeClaim).To(Equal(&corev1.PersistentVolumeClaimVolumeSource{ClaimName: predictor.claim}))
				Expect(podSpec.Containers).To(HaveLen(1))
				Expect(podSpec.Containers[0].VolumeMounts).To(ContainElement(corev1.VolumeMount{
					Name:      podSpec.Volumes[0].Name,
					MountPath: "/mnt/models",
					ReadOnly:  true,
				}))
			}
		})

		It("Should create separate Deployments for stable and canary", func() {
			configMap := createInferenceServiceConfigMap(configs)
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			servingRuntime := getServingRuntime("tf-canary-create", "default")
			Expect(k8sClient.Create(context.TODO(), &servingRuntime)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), &servingRuntime)

			serviceName := "canary-create-test"
			ctx := context.Background()

			isvc := makeCanaryISVC(serviceName, "default", "s3://test/model-v1", 4,
				[]v1beta1.CanarySpec{makeCanary("v2", 25, "s3://test/model-v2")})
			isvc.DefaultInferenceService(nil, nil, &v1beta1.SecurityConfig{AutoMountServiceAccountToken: false}, nil, nil)
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
			defer k8sClient.Delete(ctx, isvc)

			stableKey := types.NamespacedName{Name: constants.PredictorServiceName(serviceName), Namespace: "default"}
			canaryKey := types.NamespacedName{Name: constants.PredictorServiceName(serviceName, "v2"), Namespace: "default"}

			// Mark canary as available so the controller reduces stable minReplicas.
			markCanaryAvailable(ctx, canaryKey)

			// Stable gets 3 replicas (4 - 1), canary gets 1 (25% of 4)
			Eventually(func() int32 {
				deploy := &appsv1.Deployment{}
				if err := k8sClient.Get(ctx, stableKey, deploy); err != nil {
					return -1
				}
				return *deploy.Spec.Replicas
			}, timeout, interval).Should(Equal(int32(3)))

			Eventually(func() int32 {
				deploy := &appsv1.Deployment{}
				if err := k8sClient.Get(ctx, canaryKey, deploy); err != nil {
					return -1
				}
				return *deploy.Spec.Replicas
			}, timeout, interval).Should(Equal(int32(1)))

			// Verify canary has correct storageUri annotation
			Eventually(func() string {
				deploy := &appsv1.Deployment{}
				if err := k8sClient.Get(ctx, canaryKey, deploy); err != nil {
					return ""
				}
				return deploy.Spec.Template.Annotations[constants.StorageInitializerSourceUriInternalAnnotationKey]
			}, timeout, interval).Should(Equal("s3://test/model-v2"))

			stableDeploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, stableKey, stableDeploy)).Should(Succeed())
			Expect(stableDeploy.Spec.Template.Annotations[constants.StorageInitializerSourceUriInternalAnnotationKey]).To(Equal("s3://test/model-v1"))
		})

		It("Should initialize canary storageUris with its own downloader and credentials", func(ctx SpecContext) {
			configMap := createInferenceServiceConfigMap(configs)
			Expect(k8sClient.Create(ctx, configMap)).To(Succeed())
			defer k8sClient.Delete(ctx, configMap)

			namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "canary-cloud-"}}
			Expect(k8sClient.Create(ctx, namespace)).To(Succeed())
			defer k8sClient.Delete(ctx, namespace)

			servingRuntime := getServingRuntime("tf-canary-cloud-storage", namespace.Name)
			Expect(k8sClient.Create(ctx, &servingRuntime)).To(Succeed())
			defer k8sClient.Delete(ctx, &servingRuntime)

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "canary-storage-secret", Namespace: namespace.Name},
				Data: map[string][]byte{
					s3.AWSAccessKeyIdName:     []byte("unused"),
					s3.AWSSecretAccessKeyName: []byte("unused"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())
			defer k8sClient.Delete(ctx, secret)

			serviceAccount := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{Name: "canary-storage", Namespace: namespace.Name},
				Secrets:    []corev1.ObjectReference{{Name: secret.Name}},
			}
			Expect(k8sClient.Create(ctx, serviceAccount)).To(Succeed())
			defer k8sClient.Delete(ctx, serviceAccount)

			storageContainer := &v1alpha1.ClusterStorageContainer{
				ObjectMeta: metav1.ObjectMeta{Name: "canary-cloud-downloader"},
				Spec: v1alpha1.StorageContainerSpec{
					Container:                  corev1.Container{Name: constants.StorageInitializerContainerName, Image: "example.com/model-downloader:canary"},
					SupportedUriFormats:        []v1alpha1.SupportedUriFormat{{Prefix: "s3://"}},
					WorkloadType:               v1alpha1.InitContainer,
					SupportsMultiModelDownload: ptr.To(true),
				},
			}
			Expect(k8sClient.Create(ctx, storageContainer)).To(Succeed())
			defer k8sClient.Delete(ctx, storageContainer)

			isvc := makeCanaryISVC("cloud-storage", namespace.Name, "", 5,
				[]v1beta1.CanarySpec{makeCanary("v2", 20, "")})
			isvc.Spec.Predictor.Model.StorageURI = nil
			isvc.Spec.Predictor.StorageUris = []v1beta1.StorageUri{{Uri: "pvc://iris-models-v1", MountPath: "/mnt/models"}}
			canary := &isvc.Spec.Canary[0].Predictor
			canary.Model.StorageURI = nil
			canary.StorageUris = []v1beta1.StorageUri{
				{Uri: "s3://models/canary", MountPath: "/mnt/models"},
				{Uri: "s3://adapters/canary", MountPath: "/mnt/adapters"},
			}
			canary.StorageContainerName = &storageContainer.Name
			canary.ServiceAccountName = serviceAccount.Name
			isvc.DefaultInferenceService(nil, nil, &v1beta1.SecurityConfig{AutoMountServiceAccountToken: false}, nil, nil)
			Expect(k8sClient.Create(ctx, isvc)).To(Succeed())
			defer k8sClient.Delete(ctx, isvc)

			deployment := &appsv1.Deployment{}
			key := types.NamespacedName{Name: constants.PredictorServiceName(isvc.Name, "v2"), Namespace: namespace.Name}
			Eventually(func() error {
				return k8sClient.Get(ctx, key, deployment)
			}, timeout, interval).Should(Succeed())
			podSpec := deployment.Spec.Template.Spec
			Expect(podSpec.ServiceAccountName).To(Equal(serviceAccount.Name))
			Expect(podSpec.InitContainers).To(HaveLen(1))
			initializer := podSpec.InitContainers[0]
			Expect(initializer.Image).To(Equal("example.com/model-downloader:canary"))
			Expect(initializer.Args).To(Equal([]string{"s3://models/canary", "/mnt/models", "s3://adapters/canary", "/mnt/adapters"}))
			Expect(initializer.Env).To(ContainElement(corev1.EnvVar{
				Name: s3.AWSAccessKeyId,
				ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: secret.Name},
					Key:                  s3.AWSAccessKeyIdName,
				}},
			}))
			Expect(podSpec.Volumes).To(HaveLen(1))
			Expect(podSpec.Volumes[0].EmptyDir).NotTo(BeNil())
			Expect(initializer.VolumeMounts).To(ContainElement(corev1.VolumeMount{
				Name:      podSpec.Volumes[0].Name,
				MountPath: "/mnt",
			}))
			Expect(podSpec.Containers).To(HaveLen(1))
			Expect(podSpec.Containers[0].VolumeMounts).To(ContainElement(corev1.VolumeMount{
				Name:      podSpec.Volumes[0].Name,
				MountPath: "/mnt",
				ReadOnly:  true,
			}))
		})
	})

	Context("When bumping canary traffic", func() {
		It("Should adjust replica counts", func() {
			configMap := createInferenceServiceConfigMap(configs)
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			servingRuntime := getServingRuntime("tf-canary-traffic", "default")
			Expect(k8sClient.Create(context.TODO(), &servingRuntime)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), &servingRuntime)

			serviceName := "canary-traffic-test"
			serviceKey := types.NamespacedName{Name: serviceName, Namespace: "default"}
			ctx := context.Background()

			isvc := makeCanaryISVC(serviceName, "default", "s3://test/model-v1", 4,
				[]v1beta1.CanarySpec{makeCanary("v2", 25, "s3://test/model-v2")})
			isvc.DefaultInferenceService(nil, nil, &v1beta1.SecurityConfig{AutoMountServiceAccountToken: false}, nil, nil)
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
			defer k8sClient.Delete(ctx, isvc)

			stableKey := types.NamespacedName{Name: constants.PredictorServiceName(serviceName), Namespace: "default"}
			canaryKey := types.NamespacedName{Name: constants.PredictorServiceName(serviceName, "v2"), Namespace: "default"}

			// Wait for initial deployment
			Eventually(func() error {
				return k8sClient.Get(ctx, canaryKey, &appsv1.Deployment{})
			}, timeout, interval).Should(Succeed())

			// Mark canary as available so the controller reduces stable minReplicas.
			markCanaryAvailable(ctx, canaryKey)

			// Bump traffic to 50%
			Expect(retry.RetryOnConflict(retry.DefaultRetry, func() error {
				updatedISVC := &v1beta1.InferenceService{}
				if err := k8sClient.Get(ctx, serviceKey, updatedISVC); err != nil {
					return err
				}
				updatedISVC.Spec.Canary[0].TrafficPercent = 50
				return k8sClient.Update(ctx, updatedISVC)
			})).Should(Succeed())

			// Verify replica adjustment: 50% of 4 = 2 canary, 2 stable
			Eventually(func() int32 {
				deploy := &appsv1.Deployment{}
				if err := k8sClient.Get(ctx, canaryKey, deploy); err != nil {
					return -1
				}
				return *deploy.Spec.Replicas
			}, timeout, interval).Should(Equal(int32(2)))

			Eventually(func() int32 {
				deploy := &appsv1.Deployment{}
				if err := k8sClient.Get(ctx, stableKey, deploy); err != nil {
					return -1
				}
				return *deploy.Spec.Replicas
			}, timeout, interval).Should(Equal(int32(2)))
		})
	})

	Context("When promoting a canary", func() {
		It("Should not trigger a redeployment of the canary pods", func() {
			configMap := createInferenceServiceConfigMap(configs)
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			servingRuntime := getServingRuntime("tf-canary-promote", "default")
			Expect(k8sClient.Create(context.TODO(), &servingRuntime)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), &servingRuntime)

			serviceName := "canary-promote-test"
			serviceKey := types.NamespacedName{Name: serviceName, Namespace: "default"}
			ctx := context.Background()

			isvc := makeCanaryISVC(serviceName, "default", "s3://test/model-v1", 4,
				[]v1beta1.CanarySpec{makeCanary("v2", 25, "s3://test/model-v2")})
			isvc.DefaultInferenceService(nil, nil, &v1beta1.SecurityConfig{AutoMountServiceAccountToken: false}, nil, nil)
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
			defer k8sClient.Delete(ctx, isvc)

			canaryKey := types.NamespacedName{Name: constants.PredictorServiceName(serviceName, "v2"), Namespace: "default"}

			// Wait for canary deployment and record its pod template annotations
			var prePromotionAnnotations map[string]string
			Eventually(func() error {
				deploy := &appsv1.Deployment{}
				if err := k8sClient.Get(ctx, canaryKey, deploy); err != nil {
					return err
				}
				prePromotionAnnotations = deploy.Spec.Template.Annotations
				return nil
			}, timeout, interval).Should(Succeed())

			// Promote: set stable name to v2, update model, remove canary
			Expect(retry.RetryOnConflict(retry.DefaultRetry, func() error {
				updatedISVC := &v1beta1.InferenceService{}
				if err := k8sClient.Get(ctx, serviceKey, updatedISVC); err != nil {
					return err
				}
				updatedISVC.Spec.Predictor.Name = "v2"
				updatedISVC.Spec.Predictor.Model.StorageURI = proto.String("s3://test/model-v2")
				updatedISVC.Spec.Canary = nil
				return k8sClient.Update(ctx, updatedISVC)
			})).Should(Succeed())

			// The canary deployment (now stable) should keep the same pod template
			// — no annotation changes means no rollout triggered
			Consistently(func() map[string]string {
				deploy := &appsv1.Deployment{}
				if err := k8sClient.Get(ctx, canaryKey, deploy); err != nil {
					return nil
				}
				return deploy.Spec.Template.Annotations
			}, fastTimeout, interval).Should(Equal(prePromotionAnnotations))

			// Old stable deployment should be cleaned up
			oldStableKey := types.NamespacedName{Name: constants.PredictorServiceName(serviceName), Namespace: "default"}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, oldStableKey, &appsv1.Deployment{})
				return apierr.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("When removing a canary", func() {
		It("Should delete the canary Deployment and Service and restore stable replicas", func() {
			configMap := createInferenceServiceConfigMap(configs)
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			servingRuntime := getServingRuntime("tf-canary-remove", "default")
			Expect(k8sClient.Create(context.TODO(), &servingRuntime)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), &servingRuntime)

			serviceName := "canary-remove-test"
			serviceKey := types.NamespacedName{Name: serviceName, Namespace: "default"}
			ctx := context.Background()

			isvc := makeCanaryISVC(serviceName, "default", "s3://test/model-v1", 4,
				[]v1beta1.CanarySpec{makeCanary("v2", 25, "s3://test/model-v2")})
			isvc.DefaultInferenceService(nil, nil, &v1beta1.SecurityConfig{AutoMountServiceAccountToken: false}, nil, nil)
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
			defer k8sClient.Delete(ctx, isvc)

			stableKey := types.NamespacedName{Name: constants.PredictorServiceName(serviceName), Namespace: "default"}
			canaryKey := types.NamespacedName{Name: constants.PredictorServiceName(serviceName, "v2"), Namespace: "default"}

			// Wait for canary deployment
			Eventually(func() error {
				return k8sClient.Get(ctx, canaryKey, &appsv1.Deployment{})
			}, timeout, interval).Should(Succeed())

			// Remove canary
			Expect(retry.RetryOnConflict(retry.DefaultRetry, func() error {
				updatedISVC := &v1beta1.InferenceService{}
				if err := k8sClient.Get(ctx, serviceKey, updatedISVC); err != nil {
					return err
				}
				updatedISVC.Spec.Canary = nil
				return k8sClient.Update(ctx, updatedISVC)
			})).Should(Succeed())

			// Canary deployment should be deleted
			Eventually(func() bool {
				err := k8sClient.Get(ctx, canaryKey, &appsv1.Deployment{})
				return apierr.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			// Canary service should be deleted
			Eventually(func() bool {
				err := k8sClient.Get(ctx, canaryKey, &corev1.Service{})
				return apierr.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			// Stable replicas should be restored to 4
			Eventually(func() int32 {
				deploy := &appsv1.Deployment{}
				if err := k8sClient.Get(ctx, stableKey, deploy); err != nil {
					return -1
				}
				return *deploy.Spec.Replicas
			}, timeout, interval).Should(Equal(int32(4)))
		})
	})

	Context("When creating multiple canaries", func() {
		It("Should create separate Deployments with correct replica split", func() {
			configMap := createInferenceServiceConfigMap(configs)
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			servingRuntime := getServingRuntime("tf-canary-multi", "default")
			Expect(k8sClient.Create(context.TODO(), &servingRuntime)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), &servingRuntime)

			serviceName := "canary-multi-test"
			ctx := context.Background()

			isvc := makeCanaryISVC(serviceName, "default", "s3://test/model-v1", 4,
				[]v1beta1.CanarySpec{
					makeCanary("v2", 25, "s3://test/model-v2"),
					makeCanary("v3", 25, "s3://test/model-v3"),
				})
			isvc.DefaultInferenceService(nil, nil, &v1beta1.SecurityConfig{AutoMountServiceAccountToken: false}, nil, nil)
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
			defer k8sClient.Delete(ctx, isvc)

			stableKey := types.NamespacedName{Name: constants.PredictorServiceName(serviceName), Namespace: "default"}
			canaryV2Key := types.NamespacedName{Name: constants.PredictorServiceName(serviceName, "v2"), Namespace: "default"}
			canaryV3Key := types.NamespacedName{Name: constants.PredictorServiceName(serviceName, "v3"), Namespace: "default"}

			// Mark both canaries as available so the controller reduces stable minReplicas.
			markCanaryAvailable(ctx, canaryV2Key)
			markCanaryAvailable(ctx, canaryV3Key)

			// 4 total: 2 stable, 1 v2 (25%), 1 v3 (25%)
			Eventually(func() int32 {
				deploy := &appsv1.Deployment{}
				if err := k8sClient.Get(ctx, stableKey, deploy); err != nil {
					return -1
				}
				return *deploy.Spec.Replicas
			}, timeout, interval).Should(Equal(int32(2)))

			Eventually(func() int32 {
				deploy := &appsv1.Deployment{}
				if err := k8sClient.Get(ctx, canaryV2Key, deploy); err != nil {
					return -1
				}
				return *deploy.Spec.Replicas
			}, timeout, interval).Should(Equal(int32(1)))

			Eventually(func() int32 {
				deploy := &appsv1.Deployment{}
				if err := k8sClient.Get(ctx, canaryV3Key, deploy); err != nil {
					return -1
				}
				return *deploy.Spec.Replicas
			}, timeout, interval).Should(Equal(int32(1)))
		})
	})

	Context("When the canary uses an HPA autoscaler", func() {
		It("Should set an owner reference on the canary HPA so it is garbage-collected with the ISVC", func() {
			configMap := createInferenceServiceConfigMap(configs)
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			servingRuntime := getServingRuntime("tf-canary-hpa", "default")
			Expect(k8sClient.Create(context.TODO(), &servingRuntime)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), &servingRuntime)

			serviceName := "canary-hpa-test"
			ctx := context.Background()

			isvc := makeCanaryISVC(serviceName, "default", "s3://test/model-v1", 4,
				[]v1beta1.CanarySpec{makeCanary("v2", 25, "s3://test/model-v2")})
			// Use the HPA autoscaler class so an HPA is actually created for the canary.
			isvc.Annotations = getDefaultAnnotations(constants.AutoscalerClassHPA)
			isvc.DefaultInferenceService(nil, nil, &v1beta1.SecurityConfig{AutoMountServiceAccountToken: false}, nil, nil)
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
			defer k8sClient.Delete(ctx, isvc)

			canaryHPAKey := types.NamespacedName{Name: constants.PredictorServiceName(serviceName, "v2"), Namespace: "default"}

			// Wait for the canary HPA to be created.
			actualHPA := &autoscalingv2.HorizontalPodAutoscaler{}
			Eventually(func() error {
				return k8sClient.Get(ctx, canaryHPAKey, actualHPA)
			}, timeout, interval).Should(Succeed())

			// The canary HPA must be owned by the InferenceService; otherwise it is
			// orphaned and never garbage-collected when the ISVC is deleted.
			controllerRef := metav1.GetControllerOf(actualHPA)
			Expect(controllerRef).NotTo(BeNil(), "canary HPA has no controller owner reference (orphaned)")
			Expect(controllerRef.Kind).To(Equal("InferenceService"))
			Expect(controllerRef.Name).To(Equal(serviceName))
		})
	})

	Context("When force stopping an InferenceService with canary", func() {
		It("Should delete both stable and canary Deployments", func() {
			configMap := createInferenceServiceConfigMap(configs)
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			servingRuntime := getServingRuntime("tf-canary-stop", "default")
			Expect(k8sClient.Create(context.TODO(), &servingRuntime)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), &servingRuntime)

			serviceName := "canary-stop-test"
			serviceKey := types.NamespacedName{Name: serviceName, Namespace: "default"}
			ctx := context.Background()

			isvc := makeCanaryISVC(serviceName, "default", "s3://test/model-v1", 4,
				[]v1beta1.CanarySpec{makeCanary("v2", 25, "s3://test/model-v2")})
			isvc.DefaultInferenceService(nil, nil, &v1beta1.SecurityConfig{AutoMountServiceAccountToken: false}, nil, nil)
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
			defer k8sClient.Delete(ctx, isvc)

			stableKey := types.NamespacedName{Name: constants.PredictorServiceName(serviceName), Namespace: "default"}
			canaryKey := types.NamespacedName{Name: constants.PredictorServiceName(serviceName, "v2"), Namespace: "default"}

			// Wait for both deployments
			Eventually(func() error {
				return k8sClient.Get(ctx, stableKey, &appsv1.Deployment{})
			}, timeout, interval).Should(Succeed())
			Eventually(func() error {
				return k8sClient.Get(ctx, canaryKey, &appsv1.Deployment{})
			}, timeout, interval).Should(Succeed())

			// Apply force stop
			Expect(retry.RetryOnConflict(retry.DefaultRetry, func() error {
				updatedISVC := &v1beta1.InferenceService{}
				if err := k8sClient.Get(ctx, serviceKey, updatedISVC); err != nil {
					return err
				}
				if updatedISVC.Annotations == nil {
					updatedISVC.Annotations = make(map[string]string)
				}
				updatedISVC.Annotations[constants.StopAnnotationKey] = "true"
				return k8sClient.Update(ctx, updatedISVC)
			})).Should(Succeed())

			// Both deployments should be deleted
			Eventually(func() bool {
				err := k8sClient.Get(ctx, stableKey, &appsv1.Deployment{})
				return apierr.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			Eventually(func() bool {
				err := k8sClient.Get(ctx, canaryKey, &appsv1.Deployment{})
				return apierr.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("When removing a canary with orphaned HPAs", func() {
		It("Should clean up HPAs that no longer match expected deployment names", func() {
			configMap := createInferenceServiceConfigMap(configs)
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			servingRuntime := getServingRuntime("tf-canary-hpa-orphan", "default")
			Expect(k8sClient.Create(context.TODO(), &servingRuntime)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), &servingRuntime)

			serviceName := "canary-hpa-orphan-test"
			serviceKey := types.NamespacedName{Name: serviceName, Namespace: "default"}
			ctx := context.Background()

			isvc := makeCanaryISVC(serviceName, "default", "s3://test/model-v1", 4,
				[]v1beta1.CanarySpec{makeCanary("v2", 25, "s3://test/model-v2")})
			isvc.DefaultInferenceService(nil, nil, &v1beta1.SecurityConfig{AutoMountServiceAccountToken: false}, nil, nil)
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
			defer k8sClient.Delete(ctx, isvc)

			stableKey := types.NamespacedName{Name: constants.PredictorServiceName(serviceName), Namespace: "default"}
			canaryKey := types.NamespacedName{Name: constants.PredictorServiceName(serviceName, "v2"), Namespace: "default"}

			// Wait for both deployments
			Eventually(func() error {
				return k8sClient.Get(ctx, stableKey, &appsv1.Deployment{})
			}, timeout, interval).Should(Succeed())
			Eventually(func() error {
				return k8sClient.Get(ctx, canaryKey, &appsv1.Deployment{})
			}, timeout, interval).Should(Succeed())

			// Create an orphaned HPA with predictor labels but a stale name
			orphanHPAName := serviceName + "-old-predictor"
			orphanHPA := &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      orphanHPAName,
					Namespace: "default",
					Labels: map[string]string{
						constants.InferenceServicePodLabelKey: serviceName,
						constants.KServiceComponentLabel:      string(v1beta1.PredictorComponent),
					},
				},
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       orphanHPAName,
					},
					MaxReplicas: 3,
				},
			}
			Expect(k8sClient.Create(ctx, orphanHPA)).Should(Succeed())

			// Verify the orphan HPA exists
			orphanHPAKey := types.NamespacedName{Name: orphanHPAName, Namespace: "default"}
			Expect(k8sClient.Get(ctx, orphanHPAKey, &autoscalingv2.HorizontalPodAutoscaler{})).Should(Succeed())

			// Remove canary to trigger reconcile with cleanup
			Expect(retry.RetryOnConflict(retry.DefaultRetry, func() error {
				updatedISVC := &v1beta1.InferenceService{}
				if err := k8sClient.Get(ctx, serviceKey, updatedISVC); err != nil {
					return err
				}
				updatedISVC.Spec.Canary = nil
				return k8sClient.Update(ctx, updatedISVC)
			})).Should(Succeed())

			// Orphaned HPA should be deleted by the reconciler
			Eventually(func() bool {
				err := k8sClient.Get(ctx, orphanHPAKey, &autoscalingv2.HorizontalPodAutoscaler{})
				return apierr.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			// Stable deployment should still exist
			Eventually(func() error {
				return k8sClient.Get(ctx, stableKey, &appsv1.Deployment{})
			}, timeout, interval).Should(Succeed())
		})
	})

	Context("When promoting a canary with orphaned HPAs", func() {
		It("Should clean up HPAs from the old stable deployment name", func() {
			configMap := createInferenceServiceConfigMap(configs)
			Expect(k8sClient.Create(context.TODO(), configMap)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), configMap)

			servingRuntime := getServingRuntime("tf-canary-hpa-promote", "default")
			Expect(k8sClient.Create(context.TODO(), &servingRuntime)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), &servingRuntime)

			serviceName := "canary-hpa-promote-test"
			serviceKey := types.NamespacedName{Name: serviceName, Namespace: "default"}
			ctx := context.Background()

			isvc := makeCanaryISVC(serviceName, "default", "s3://test/model-v1", 4,
				[]v1beta1.CanarySpec{makeCanary("v2", 25, "s3://test/model-v2")})
			isvc.DefaultInferenceService(nil, nil, &v1beta1.SecurityConfig{AutoMountServiceAccountToken: false}, nil, nil)
			Expect(k8sClient.Create(ctx, isvc)).Should(Succeed())
			defer k8sClient.Delete(ctx, isvc)

			stableKey := types.NamespacedName{Name: constants.PredictorServiceName(serviceName), Namespace: "default"}
			canaryKey := types.NamespacedName{Name: constants.PredictorServiceName(serviceName, "v2"), Namespace: "default"}

			// Wait for both deployments
			Eventually(func() error {
				return k8sClient.Get(ctx, stableKey, &appsv1.Deployment{})
			}, timeout, interval).Should(Succeed())
			Eventually(func() error {
				return k8sClient.Get(ctx, canaryKey, &appsv1.Deployment{})
			}, timeout, interval).Should(Succeed())

			// Create an HPA matching the old stable name (simulates HPA created before promotion)
			oldStableName := constants.PredictorServiceName(serviceName)
			oldStableHPA := &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      oldStableName,
					Namespace: "default",
					Labels: map[string]string{
						constants.InferenceServicePodLabelKey: serviceName,
						constants.KServiceComponentLabel:      string(v1beta1.PredictorComponent),
					},
				},
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       oldStableName,
					},
					MaxReplicas: 3,
				},
			}
			Expect(k8sClient.Create(ctx, oldStableHPA)).Should(Succeed())

			oldStableHPAKey := types.NamespacedName{Name: oldStableName, Namespace: "default"}
			Expect(k8sClient.Get(ctx, oldStableHPAKey, &autoscalingv2.HorizontalPodAutoscaler{})).Should(Succeed())

			// Promote: set stable name to v2, update model, remove canary
			Expect(retry.RetryOnConflict(retry.DefaultRetry, func() error {
				updatedISVC := &v1beta1.InferenceService{}
				if err := k8sClient.Get(ctx, serviceKey, updatedISVC); err != nil {
					return err
				}
				updatedISVC.Spec.Predictor.Name = "v2"
				updatedISVC.Spec.Predictor.Model.StorageURI = proto.String("s3://test/model-v2")
				updatedISVC.Spec.Canary = nil
				return k8sClient.Update(ctx, updatedISVC)
			})).Should(Succeed())

			// Old stable HPA should be cleaned up (name no longer in expectedNames)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, oldStableHPAKey, &autoscalingv2.HorizontalPodAutoscaler{})
				return apierr.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			// The promoted deployment should still exist
			Eventually(func() error {
				return k8sClient.Get(ctx, canaryKey, &appsv1.Deployment{})
			}, timeout, interval).Should(Succeed())

			// Old stable deployment should be cleaned up
			Eventually(func() bool {
				err := k8sClient.Get(ctx, stableKey, &appsv1.Deployment{})
				return apierr.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})
	})
})

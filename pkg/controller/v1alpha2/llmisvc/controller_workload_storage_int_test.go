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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/apis"
	"knative.dev/pkg/kmeta"
	"sigs.k8s.io/controller-runtime/pkg/client"
	lwsapi "sigs.k8s.io/lws/api/leaderworkerset/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"
	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
	"github.com/kserve/kserve/pkg/credentials"
	"github.com/kserve/kserve/pkg/credentials/hf"
	"github.com/kserve/kserve/pkg/credentials/s3"
	"github.com/kserve/kserve/pkg/utils"
)

var (
	isvcConfigPatchStorageInit = `{
			"memoryRequest": "100Mi",
			"memoryLimit": "1Gi",
			"cpuRequest": "100m",
			"cpuLimit": "1",
			"cpuModelcar": "10m",
			"memoryModelcar": "15Mi",
			"enableModelcar": true,
			"caBundleConfigMapName": "global-s3-custom-certs",
			"caBundleVolumeMountPath": "/path/to/globalcerts"
		}`
	isvcConfigPatchCredentials = `{
       		"s3": {
           		"s3AccessKeyIDName": "AWS_ACCESS_KEY_ID",
           		"s3SecretAccessKeyName": "AWS_SECRET_ACCESS_KEY",
           		"s3CABundleConfigMap": "local-s3-custom-certs",
           		"s3CABundle": "/path/to/localcerts.crt"
       		}
    	}` // #nosec G101
)

var _ = Describe("LLMInferenceService Controller - Storage configuration", func() {
	Context("Single node", func() {
		It("should configure direct PVC mount when model uri starts with pvc://", func(ctx SpecContext) {
			// given
			svcName := "test-llm-storage-pvc"
			nsName := kmeta.ChildName(svcName, "-test")
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: nsName,
				},
			}
			Expect(envTest.Client.Create(ctx, namespace)).To(Succeed())
			Expect(envTest.Client.Create(ctx, IstioShadowService(svcName, nsName))).To(Succeed())
			defer func() {
				envTest.DeleteAll(namespace)
			}()

			modelURL, err := apis.ParseURL("pvc://facebook-models/opt-125m")
			Expect(err).ToNot(HaveOccurred())

			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      svcName,
					Namespace: nsName,
				},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Model: v1alpha2.LLMModelSpec{
						Name: ptr.To("foo"),
						URI:  *modelURL,
					},
					WorkloadSpec: v1alpha2.WorkloadSpec{},
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
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// then
			expectedMainDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve",
					Namespace: nsName,
				}, expectedMainDeployment)
			}).WithContext(ctx).Should(Succeed())

			expectedPrefillDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-prefill",
					Namespace: nsName,
				}, expectedPrefillDeployment)
			}).WithContext(ctx).Should(Succeed())

			validatePvcStorageIsConfigured(expectedMainDeployment)
			validatePvcStorageIsConfigured(expectedPrefillDeployment)
		})

		It("should configure a modelcar when model uri starts with oci://", func(ctx SpecContext) {
			// given
			svcName := "test-llm-storage-oci"
			nsName := kmeta.ChildName(svcName, "-test")
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: nsName,
				},
			}
			Expect(envTest.Client.Create(ctx, namespace)).To(Succeed())
			Expect(envTest.Client.Create(ctx, IstioShadowService(svcName, nsName))).To(Succeed())
			defer func() {
				envTest.DeleteAll(namespace)
			}()

			modelURL, err := apis.ParseURL("oci://registry.io/user-id/repo-id:tag")
			Expect(err).ToNot(HaveOccurred())

			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      svcName,
					Namespace: nsName,
				},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Model: v1alpha2.LLMModelSpec{
						Name: ptr.To("foo"),
						URI:  *modelURL,
					},
					WorkloadSpec: v1alpha2.WorkloadSpec{},
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
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// then
			expectedMainDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve",
					Namespace: nsName,
				}, expectedMainDeployment)
			}).WithContext(ctx).Should(Succeed())

			expectedPrefillDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-prefill",
					Namespace: nsName,
				}, expectedPrefillDeployment)
			}).WithContext(ctx).Should(Succeed())

			validateOciStorageIsConfigured(expectedMainDeployment)
			validateOciStorageIsConfigured(expectedPrefillDeployment)
		})

		It("should use storage-initializer to download model when uri starts with hf://", func(ctx SpecContext) {
			// given
			svcName := "test-llm-storage-hf"
			nsName := kmeta.ChildName(svcName, "-test")
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: nsName,
				},
			}
			Expect(envTest.Client.Create(ctx, namespace)).To(Succeed())
			defer func() {
				envTest.DeleteAll(namespace)
			}()

			modelURL, err := apis.ParseURL("hf://user-id/repo-id:tag")
			Expect(err).ToNot(HaveOccurred())

			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      svcName,
					Namespace: nsName,
				},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Model: v1alpha2.LLMModelSpec{
						Name: ptr.To("foo"),
						URI:  *modelURL,
					},
					WorkloadSpec: v1alpha2.WorkloadSpec{},
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
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// then
			expectedMainDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve",
					Namespace: nsName,
				}, expectedMainDeployment)
			}).WithContext(ctx).Should(Succeed())

			expectedPrefillDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-prefill",
					Namespace: nsName,
				}, expectedPrefillDeployment)
			}).WithContext(ctx).Should(Succeed())

			validateStorageInitializerIsConfigured(expectedMainDeployment, "hf://user-id/repo-id:tag")
			validateStorageInitializerIsConfigured(expectedPrefillDeployment, "hf://user-id/repo-id:tag")
		})

		It("should use storage-initializer and set proper env variables when uri starts with hf:// and credentials are configured", func(ctx SpecContext) {
			// setup test dependencies
			svcName := "test-llm-storage-hf-with-credentials"
			nsName := kmeta.ChildName(svcName, "-test")
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: nsName,
				},
			}
			Expect(envTest.Client.Create(ctx, namespace)).To(Succeed())
			defer func() {
				envTest.DeleteAll(namespace)
			}()

			secretName := kmeta.ChildName(svcName, "-secret")
			hfTokenValue := "test-token"
			credentialSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: nsName,
				},
				StringData: map[string]string{
					hf.HFTokenKey: hfTokenValue,
				},
			}
			Expect(envTest.Client.Create(ctx, credentialSecret)).To(Succeed())

			serviceAccountName := kmeta.ChildName(svcName, "-sa")
			serviceAccount := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceAccountName,
					Namespace: nsName,
				},
				Secrets: []corev1.ObjectReference{
					{
						Name:      secretName,
						Namespace: nsName,
					},
				},
			}
			Expect(envTest.Client.Create(ctx, serviceAccount)).To(Succeed())

			modelURL, err := apis.ParseURL("hf://user-id/repo-id:tag")
			Expect(err).ToNot(HaveOccurred())

			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      svcName,
					Namespace: nsName,
				},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Model: v1alpha2.LLMModelSpec{
						Name: ptr.To("foo"),
						URI:  *modelURL,
					},
					WorkloadSpec: v1alpha2.WorkloadSpec{
						Template: &corev1.PodSpec{
							ServiceAccountName: serviceAccountName,
						},
					},
					Router: &v1alpha2.RouterSpec{
						Route:     &v1alpha2.GatewayRoutesSpec{},
						Gateway:   &v1alpha2.GatewaySpec{},
						Scheduler: &v1alpha2.SchedulerSpec{},
					},
					Prefill: &v1alpha2.WorkloadSpec{
						Template: &corev1.PodSpec{
							ServiceAccountName: serviceAccountName,
						},
					},
				},
			}

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// retrieve the created deployments
			expectedMainDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve",
					Namespace: nsName,
				}, expectedMainDeployment)
			}).WithContext(ctx).Should(Succeed())

			expectedPrefillDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-prefill",
					Namespace: nsName,
				}, expectedPrefillDeployment)
			}).WithContext(ctx).Should(Succeed())

			// validate the storage initializer configuration in the deployments
			validateStorageInitializerIsConfigured(expectedMainDeployment, "hf://user-id/repo-id:tag")
			validateStorageInitializerIsConfigured(expectedPrefillDeployment, "hf://user-id/repo-id:tag")

			// validate the storage initializer credentials are properly set
			expectedEnvVars := []corev1.EnvVar{
				{
					Name: hf.HFTokenKey,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: secretName,
							},
							Key: "HF_TOKEN",
						},
					},
				},
			}
			validateStorageInitializerCredentials(expectedMainDeployment, expectedEnvVars)
			validateStorageInitializerCredentials(expectedPrefillDeployment, expectedEnvVars)
		})

		It("should use storage-initializer to download model when uri starts with s3://", func(ctx SpecContext) {
			// create the global s3 ca bundle configmap
			globalS3CaBundleconfigMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "global-s3-custom-certs",
					Namespace: constants.KServeNamespace,
				},
				Data: map[string]string{
					"cabundle.crt": "global-test-cert",
				},
			}
			Expect(envTest.Client.Create(ctx, globalS3CaBundleconfigMap)).To(Succeed())
			defer func() {
				Expect(envTest.Client.Delete(ctx, globalS3CaBundleconfigMap)).To(Succeed())
			}()

			// patch the infernceservice-config configmap
			patchInferenceServiceConfig(ctx, "storageInitializer", isvcConfigPatchStorageInit)
			defer restoreInferenceServiceConfig(ctx)

			// setup test dependencies
			svcName := "test-llm-storage-s3"
			nsName := kmeta.ChildName(svcName, "-test")
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: nsName,
				},
			}
			Expect(envTest.Client.Create(ctx, namespace)).To(Succeed())
			defer func() {
				envTest.DeleteAll(namespace)
			}()

			modelURL, err := apis.ParseURL("s3://user-id/repo-id:tag")
			Expect(err).ToNot(HaveOccurred())

			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      svcName,
					Namespace: nsName,
				},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Model: v1alpha2.LLMModelSpec{
						Name: ptr.To("foo"),
						URI:  *modelURL,
					},
					WorkloadSpec: v1alpha2.WorkloadSpec{},
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
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// Ensure the global-ca-bundle config map is created in the llmisvc's namespace
			generatedGlobalCaBundleConfigMap := &corev1.ConfigMap{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      constants.DefaultGlobalCaBundleConfigMapName,
					Namespace: nsName,
				}, generatedGlobalCaBundleConfigMap)
			}).WithContext(ctx).Should(Succeed())

			// retrieve the created deployments
			expectedMainDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve",
					Namespace: nsName,
				}, expectedMainDeployment)
			}).WithContext(ctx).Should(Succeed())

			expectedPrefillDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-prefill",
					Namespace: nsName,
				}, expectedPrefillDeployment)
			}).WithContext(ctx).Should(Succeed())

			// validate the storage initializer configuration in the deployments
			validateStorageInitializerIsConfigured(expectedMainDeployment, "s3://user-id/repo-id:tag")
			validateStorageInitializerIsConfigured(expectedPrefillDeployment, "s3://user-id/repo-id:tag")

			// validate the storage initializer credentials are properly set
			expectedEnvVars := []corev1.EnvVar{
				{
					Name:  constants.CaBundleConfigMapNameEnvVarKey,
					Value: constants.DefaultGlobalCaBundleConfigMapName,
				},
				{
					Name:  constants.CaBundleVolumeMountPathEnvVarKey,
					Value: "/path/to/globalcerts",
				},
			}
			validateStorageInitializerCredentials(expectedMainDeployment, expectedEnvVars)
			validateStorageInitializerCredentials(expectedPrefillDeployment, expectedEnvVars)

			var defaultMode int32 = 420
			expectedCaBundleVolume := []corev1.Volume{
				{
					Name: CaBundleVolumeName,
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: constants.DefaultGlobalCaBundleConfigMapName,
							},
							DefaultMode: &defaultMode,
						},
					},
				},
			}
			validateStorageInitializerVolumes(expectedMainDeployment, expectedCaBundleVolume)
			validateStorageInitializerVolumes(expectedPrefillDeployment, expectedCaBundleVolume)

			expectedCaBundleVolumeMount := []corev1.VolumeMount{
				{
					Name:      CaBundleVolumeName,
					MountPath: "/path/to/globalcerts",
					ReadOnly:  true,
				},
			}
			validateStorageInitializerVolumeMounts(expectedMainDeployment, expectedCaBundleVolumeMount)
			validateStorageInitializerVolumeMounts(expectedPrefillDeployment, expectedCaBundleVolumeMount)
		})

		It("should use storage-initializer to download model when uri starts with s3:// and s3 config is configured", func(ctx SpecContext) {
			// patch the infernceservice-config configmap
			patchInferenceServiceConfig(ctx, "credentials", isvcConfigPatchCredentials)
			defer restoreInferenceServiceConfig(ctx)

			// setup test dependencies
			svcName := "test-llm-storage-s3-config"
			nsName := kmeta.ChildName(svcName, "-test")
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: nsName,
				},
			}
			Expect(envTest.Client.Create(ctx, namespace)).To(Succeed())
			defer func() {
				envTest.DeleteAll(namespace)
			}()

			localCaBundleConfigMapName := "local-s3-custom-certs"
			localCaBundleconfigMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      localCaBundleConfigMapName,
					Namespace: nsName,
				},
				Data: map[string]string{
					"localcerts.crt": "test-cert",
				},
			}
			Expect(envTest.Client.Create(ctx, localCaBundleconfigMap)).To(Succeed())

			secretName := kmeta.ChildName(svcName, "-secret")
			s3Endpoint := "s3-config-credentials-test.kserve:9000"
			s3UseHttps := "0"
			s3Region := "us-south"
			s3Anon := "false"
			credentialSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: nsName,
					Annotations: map[string]string{
						s3.InferenceServiceS3SecretEndpointAnnotation: s3Endpoint,
						s3.InferenceServiceS3SecretHttpsAnnotation:    s3UseHttps,
						s3.InferenceServiceS3SecretRegionAnnotation:   s3Region,
						s3.InferenceServiceS3UseAnonymousCredential:   s3Anon,
					},
				},
				StringData: map[string]string{
					s3.AWSAccessKeyId:     "test-id",
					s3.AWSSecretAccessKey: "test-secret",
				},
			}
			Expect(envTest.Client.Create(ctx, credentialSecret)).To(Succeed())

			serviceAccountName := kmeta.ChildName(svcName, "-sa")
			serviceAccount := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceAccountName,
					Namespace: nsName,
				},
				Secrets: []corev1.ObjectReference{
					{
						Name:      secretName,
						Namespace: nsName,
					},
				},
			}
			Expect(envTest.Client.Create(ctx, serviceAccount)).To(Succeed())

			modelURL, err := apis.ParseURL("s3://user-id/repo-id:tag")
			Expect(err).ToNot(HaveOccurred())

			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      svcName,
					Namespace: nsName,
				},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Model: v1alpha2.LLMModelSpec{
						Name: ptr.To("foo"),
						URI:  *modelURL,
					},
					WorkloadSpec: v1alpha2.WorkloadSpec{
						Template: &corev1.PodSpec{
							ServiceAccountName: serviceAccountName,
						},
					},
					Router: &v1alpha2.RouterSpec{
						Route:     &v1alpha2.GatewayRoutesSpec{},
						Gateway:   &v1alpha2.GatewaySpec{},
						Scheduler: &v1alpha2.SchedulerSpec{},
					},
					Prefill: &v1alpha2.WorkloadSpec{
						Template: &corev1.PodSpec{
							ServiceAccountName: serviceAccountName,
						},
					},
				},
			}
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// retrieve the created deployments
			expectedMainDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve",
					Namespace: nsName,
				}, expectedMainDeployment)
			}).WithContext(ctx).Should(Succeed())

			expectedPrefillDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-prefill",
					Namespace: nsName,
				}, expectedPrefillDeployment)
			}).WithContext(ctx).Should(Succeed())

			// validate the storage initializer configuration in the deployments
			validateStorageInitializerIsConfigured(expectedMainDeployment, "s3://user-id/repo-id:tag")
			validateStorageInitializerIsConfigured(expectedPrefillDeployment, "s3://user-id/repo-id:tag")

			// validate the storage initializer credentials are properly set
			expectedEnvVars := createExpectedS3ConfigEnvVars(
				secretName,
				s3UseHttps,
				s3Endpoint,
				s3Anon,
				"http://"+s3Endpoint,
				s3Region,
				localCaBundleConfigMapName,
				"/path/to",
				"/path/to/localcerts.crt",
			)
			validateStorageInitializerCredentials(expectedMainDeployment, expectedEnvVars)
			validateStorageInitializerCredentials(expectedPrefillDeployment, expectedEnvVars)

			var defaultMode int32 = 420
			expectedCaBundleVolume := []corev1.Volume{
				{
					Name: CaBundleVolumeName,
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: localCaBundleConfigMapName,
							},
							DefaultMode: &defaultMode,
						},
					},
				},
			}
			validateStorageInitializerVolumes(expectedMainDeployment, expectedCaBundleVolume)
			validateStorageInitializerVolumes(expectedPrefillDeployment, expectedCaBundleVolume)

			expectedCaBundleVolumeMount := []corev1.VolumeMount{
				{
					Name:      CaBundleVolumeName,
					MountPath: "/path/to",
					ReadOnly:  true,
				},
			}
			validateStorageInitializerVolumeMounts(expectedMainDeployment, expectedCaBundleVolumeMount)
			validateStorageInitializerVolumeMounts(expectedPrefillDeployment, expectedCaBundleVolumeMount)
		})

		It("should use storage-initializer and set proper env variables when uri starts with s3:// and credentials are configured", func(ctx SpecContext) {
			// create the global s3 ca bundle configmap
			globalS3CaBundleconfigMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "global-s3-custom-certs",
					Namespace: constants.KServeNamespace,
				},
				Data: map[string]string{
					"cabundle.crt": "global-test-cert",
				},
			}
			Expect(envTest.Client.Create(ctx, globalS3CaBundleconfigMap)).To(Succeed())
			defer func() {
				Expect(envTest.Client.Delete(ctx, globalS3CaBundleconfigMap)).To(Succeed())
			}()

			// patch the infernceservice-config configmap
			patchInferenceServiceConfig(ctx, "storageInitializer", isvcConfigPatchStorageInit)
			defer restoreInferenceServiceConfig(ctx)

			// setup test dependencies
			svcName := "test-llm-storage-s3-with-credentials"
			nsName := kmeta.ChildName(svcName, "-test")
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: nsName,
				},
			}
			Expect(envTest.Client.Create(ctx, namespace)).To(Succeed())
			defer func() {
				envTest.DeleteAll(namespace)
			}()

			s3CaBundleConfigMapName := "s3-custom-certs"
			s3CaBundleconfigMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      s3CaBundleConfigMapName,
					Namespace: nsName,
				},
				Data: map[string]string{
					"s3.crt": "test-cert",
				},
			}
			Expect(envTest.Client.Create(ctx, s3CaBundleconfigMap)).To(Succeed())

			secretName := kmeta.ChildName(svcName, "-secret")
			s3Endpoint := "s3-credentials-test.kserve:9000"
			s3UseHttps := "0"
			s3Region := "us-south"
			s3Anon := "false"
			credentialSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: nsName,
					Annotations: map[string]string{
						s3.InferenceServiceS3SecretEndpointAnnotation:    s3Endpoint,
						s3.InferenceServiceS3SecretHttpsAnnotation:       s3UseHttps,
						s3.InferenceServiceS3SecretRegionAnnotation:      s3Region,
						s3.InferenceServiceS3UseAnonymousCredential:      s3Anon,
						s3.InferenceServiceS3CABundleConfigMapAnnotation: s3CaBundleConfigMapName,
						s3.InferenceServiceS3CABundleAnnotation:          "/path/to/s3.crt",
					},
				},
				StringData: map[string]string{
					s3.AWSAccessKeyId:     "test-id",
					s3.AWSSecretAccessKey: "test-secret",
				},
			}
			Expect(envTest.Client.Create(ctx, credentialSecret)).To(Succeed())

			serviceAccountName := kmeta.ChildName(svcName, "-sa")
			serviceAccount := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceAccountName,
					Namespace: nsName,
				},
				Secrets: []corev1.ObjectReference{
					{
						Name:      secretName,
						Namespace: nsName,
					},
				},
			}
			Expect(envTest.Client.Create(ctx, serviceAccount)).To(Succeed())

			modelURL, err := apis.ParseURL("s3://bucket/model")
			Expect(err).ToNot(HaveOccurred())

			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      svcName,
					Namespace: nsName,
				},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Model: v1alpha2.LLMModelSpec{
						Name: ptr.To("foo"),
						URI:  *modelURL,
					},
					WorkloadSpec: v1alpha2.WorkloadSpec{
						Template: &corev1.PodSpec{
							ServiceAccountName: serviceAccountName,
						},
					},
					Router: &v1alpha2.RouterSpec{
						Route:     &v1alpha2.GatewayRoutesSpec{},
						Gateway:   &v1alpha2.GatewaySpec{},
						Scheduler: &v1alpha2.SchedulerSpec{},
					},
					Prefill: &v1alpha2.WorkloadSpec{
						Template: &corev1.PodSpec{
							ServiceAccountName: serviceAccountName,
						},
					},
				},
			}
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// retrieve the created deployments
			expectedMainDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve",
					Namespace: nsName,
				}, expectedMainDeployment)
			}).WithContext(ctx).Should(Succeed())

			expectedPrefillDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-prefill",
					Namespace: nsName,
				}, expectedPrefillDeployment)
			}).WithContext(ctx).Should(Succeed())

			// validate the storage initializer configuration in the deployments
			validateStorageInitializerIsConfigured(expectedMainDeployment, "s3://bucket/model")
			validateStorageInitializerIsConfigured(expectedPrefillDeployment, "s3://bucket/model")

			// validate the storage initializer credentials are properly set
			expectedEnvVars := createExpectedS3ConfigEnvVars(
				credentialSecret.Name,
				s3UseHttps,
				s3Endpoint,
				s3Anon,
				"http://"+s3Endpoint,
				s3Region,
				s3CaBundleConfigMapName,
				"/path/to",
				"/path/to/s3.crt",
			)
			validateStorageInitializerCredentials(expectedMainDeployment, expectedEnvVars)
			validateStorageInitializerCredentials(expectedPrefillDeployment, expectedEnvVars)

			var defaultMode int32 = 420
			expectedCaBundleVolume := []corev1.Volume{
				{
					Name: CaBundleVolumeName,
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: s3CaBundleConfigMapName,
							},
							DefaultMode: &defaultMode,
						},
					},
				},
			}
			validateStorageInitializerVolumes(expectedMainDeployment, expectedCaBundleVolume)
			validateStorageInitializerVolumes(expectedPrefillDeployment, expectedCaBundleVolume)

			expectedCaBundleVolumeMount := []corev1.VolumeMount{
				{
					Name:      CaBundleVolumeName,
					MountPath: "/path/to",
					ReadOnly:  true,
				},
			}
			validateStorageInitializerVolumeMounts(expectedMainDeployment, expectedCaBundleVolumeMount)
			validateStorageInitializerVolumeMounts(expectedPrefillDeployment, expectedCaBundleVolumeMount)
		})

		It("should use storage-initializer and set proper env variables when uri starts with s3:// and credentials are configured for IAM", func(ctx SpecContext) {
			// create the global s3 ca bundle configmap
			globalS3CaBundleconfigMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "global-s3-custom-certs",
					Namespace: constants.KServeNamespace,
				},
				Data: map[string]string{
					"cabundle.crt": "global-test-cert",
				},
			}
			Expect(envTest.Client.Create(ctx, globalS3CaBundleconfigMap)).To(Succeed())
			defer func() {
				Expect(envTest.Client.Delete(ctx, globalS3CaBundleconfigMap)).To(Succeed())
			}()

			// patch the infernceservice-config configmap
			patchInferenceServiceConfig(ctx, "storageInitializer", isvcConfigPatchStorageInit)
			defer restoreInferenceServiceConfig(ctx)

			// setup test dependencies
			svcName := "test-llm-storage-s3-with-iam-credentials"
			nsName := kmeta.ChildName(svcName, "-test")
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: nsName,
				},
			}
			Expect(envTest.Client.Create(ctx, namespace)).To(Succeed())
			defer func() {
				envTest.DeleteAll(namespace)
			}()

			s3CaBundleConfigMapName := "s3-custom-certs"
			s3CaBundleconfigMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      s3CaBundleConfigMapName,
					Namespace: nsName,
				},
				Data: map[string]string{
					"s3.crt": "test-cert",
				},
			}
			Expect(envTest.Client.Create(ctx, s3CaBundleconfigMap)).To(Succeed())

			serviceAccountName := kmeta.ChildName(svcName, "-sa")
			s3IamRole := "arn:aws:iam::123456789012:role/s3access"
			s3Endpoint := "s3-credentials-test.kserve:9000"
			s3UseHttps := "0"
			s3Region := "us-south"
			s3Anon := "false"
			serviceAccount := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceAccountName,
					Namespace: nsName,
					Annotations: map[string]string{
						credentials.AwsIrsaAnnotationKey:                 s3IamRole,
						s3.InferenceServiceS3SecretEndpointAnnotation:    s3Endpoint,
						s3.InferenceServiceS3SecretHttpsAnnotation:       s3UseHttps,
						s3.InferenceServiceS3SecretRegionAnnotation:      s3Region,
						s3.InferenceServiceS3UseAnonymousCredential:      s3Anon,
						s3.InferenceServiceS3CABundleConfigMapAnnotation: s3CaBundleConfigMapName,
						s3.InferenceServiceS3CABundleAnnotation:          "/path/to/s3.crt",
					},
				},
			}
			Expect(envTest.Client.Create(ctx, serviceAccount)).To(Succeed())

			modelURL, err := apis.ParseURL("s3://bucket/model")
			Expect(err).ToNot(HaveOccurred())

			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      svcName,
					Namespace: nsName,
				},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Model: v1alpha2.LLMModelSpec{
						Name: ptr.To("foo"),
						URI:  *modelURL,
					},
					WorkloadSpec: v1alpha2.WorkloadSpec{
						Template: &corev1.PodSpec{
							ServiceAccountName: serviceAccountName,
						},
					},
					Router: &v1alpha2.RouterSpec{
						Route:     &v1alpha2.GatewayRoutesSpec{},
						Gateway:   &v1alpha2.GatewaySpec{},
						Scheduler: &v1alpha2.SchedulerSpec{},
					},
					Prefill: &v1alpha2.WorkloadSpec{
						Template: &corev1.PodSpec{
							ServiceAccountName: serviceAccountName,
						},
					},
				},
			}
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// retrieve the created deployments
			expectedMainDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve",
					Namespace: nsName,
				}, expectedMainDeployment)
			}).WithContext(ctx).Should(Succeed())

			expectedPrefillDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-prefill",
					Namespace: nsName,
				}, expectedPrefillDeployment)
			}).WithContext(ctx).Should(Succeed())

			// validate the storage initializer configuration in the deployments
			validateStorageInitializerIsConfigured(expectedMainDeployment, "s3://bucket/model")
			validateStorageInitializerIsConfigured(expectedPrefillDeployment, "s3://bucket/model")

			// validate the storage initializer credentials are properly set
			expectedEnvVars := createExpectedS3ConfigEnvVars(
				"",
				s3UseHttps,
				s3Endpoint,
				s3Anon,
				"http://"+s3Endpoint,
				s3Region,
				s3CaBundleConfigMapName,
				"/path/to",
				"/path/to/s3.crt",
			)
			validateStorageInitializerCredentials(expectedMainDeployment, expectedEnvVars)
			validateStorageInitializerCredentials(expectedPrefillDeployment, expectedEnvVars)

			var defaultMode int32 = 420
			expectedCaBundleVolume := []corev1.Volume{
				{
					Name: CaBundleVolumeName,
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: s3CaBundleConfigMapName,
							},
							DefaultMode: &defaultMode,
						},
					},
				},
			}
			validateStorageInitializerVolumes(expectedMainDeployment, expectedCaBundleVolume)
			validateStorageInitializerVolumes(expectedPrefillDeployment, expectedCaBundleVolume)

			expectedCaBundleVolumeMount := []corev1.VolumeMount{
				{
					Name:      CaBundleVolumeName,
					MountPath: "/path/to",
					ReadOnly:  true,
				},
			}
			validateStorageInitializerVolumeMounts(expectedMainDeployment, expectedCaBundleVolumeMount)
			validateStorageInitializerVolumeMounts(expectedPrefillDeployment, expectedCaBundleVolumeMount)

			// validate the role-arn annotation was properly propagated to the created service account
			expectedServiceAccount := &corev1.ServiceAccount{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      serviceAccountName,
					Namespace: nsName,
				}, expectedServiceAccount)
			}).WithContext(ctx).Should(Succeed())
			Expect(expectedServiceAccount.Annotations[credentials.AwsIrsaAnnotationKey]).To(BeEquivalentTo(s3IamRole))
		})

		It("should NOT create storage-initializer when explicitly disabled for s3:// URI", func(ctx SpecContext) {
			// given
			svcName := "test-llm-storage-s3-disabled"
			nsName := kmeta.ChildName(svcName, "-test")
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: nsName,
				},
			}
			Expect(envTest.Client.Create(ctx, namespace)).To(Succeed())
			Expect(envTest.Client.Create(ctx, IstioShadowService(svcName, nsName))).To(Succeed())
			defer func() {
				envTest.DeleteAll(namespace)
			}()

			modelURL, err := apis.ParseURL("s3://user-id/repo-id:tag")
			Expect(err).ToNot(HaveOccurred())

			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      svcName,
					Namespace: nsName,
				},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Model: v1alpha2.LLMModelSpec{
						Name: ptr.To("foo"),
						URI:  *modelURL,
					},
					StorageInitializer: &v1alpha2.StorageInitializerSpec{
						Enabled: ptr.To(false),
					},
					WorkloadSpec: v1alpha2.WorkloadSpec{},
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
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// then
			expectedMainDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve",
					Namespace: nsName,
				}, expectedMainDeployment)
			}).WithContext(ctx).Should(Succeed())

			expectedPrefillDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-prefill",
					Namespace: nsName,
				}, expectedPrefillDeployment)
			}).WithContext(ctx).Should(Succeed())

			// Validate NO storage-initializer was created
			validateNoStorageInitializer(expectedMainDeployment)
			validateNoStorageInitializer(expectedPrefillDeployment)
		})

		It("should NOT create storage-initializer when explicitly disabled for hf:// URI", func(ctx SpecContext) {
			// given
			svcName := "test-llm-storage-hf-disabled"
			nsName := kmeta.ChildName(svcName, "-test")
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: nsName,
				},
			}
			Expect(envTest.Client.Create(ctx, namespace)).To(Succeed())
			Expect(envTest.Client.Create(ctx, IstioShadowService(svcName, nsName))).To(Succeed())
			defer func() {
				envTest.DeleteAll(namespace)
			}()

			modelURL, err := apis.ParseURL("hf://meta-llama/Llama-3.2-1B-Instruct")
			Expect(err).ToNot(HaveOccurred())

			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      svcName,
					Namespace: nsName,
				},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Model: v1alpha2.LLMModelSpec{
						Name: ptr.To("foo"),
						URI:  *modelURL,
					},
					StorageInitializer: &v1alpha2.StorageInitializerSpec{
						Enabled: ptr.To(false),
					},
					WorkloadSpec: v1alpha2.WorkloadSpec{},
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
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// then
			expectedMainDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve",
					Namespace: nsName,
				}, expectedMainDeployment)
			}).WithContext(ctx).Should(Succeed())

			expectedPrefillDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-prefill",
					Namespace: nsName,
				}, expectedPrefillDeployment)
			}).WithContext(ctx).Should(Succeed())

			// Validate NO storage-initializer was created
			validateNoStorageInitializer(expectedMainDeployment)
			validateNoStorageInitializer(expectedPrefillDeployment)
		})

		It("should NOT configure PVC storage when storage-initializer is disabled", func(ctx SpecContext) {
			// given
			svcName := "test-llm-storage-pvc-si-disabled"
			nsName := kmeta.ChildName(svcName, "-test")
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: nsName,
				},
			}
			Expect(envTest.Client.Create(ctx, namespace)).To(Succeed())
			Expect(envTest.Client.Create(ctx, IstioShadowService(svcName, nsName))).To(Succeed())
			defer func() {
				envTest.DeleteAll(namespace)
			}()

			modelURL, err := apis.ParseURL("pvc://facebook-models/opt-125m")
			Expect(err).ToNot(HaveOccurred())

			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      svcName,
					Namespace: nsName,
				},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Model: v1alpha2.LLMModelSpec{
						Name: ptr.To("foo"),
						URI:  *modelURL,
					},
					StorageInitializer: &v1alpha2.StorageInitializerSpec{
						Enabled: ptr.To(false),
					},
					WorkloadSpec: v1alpha2.WorkloadSpec{},
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
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// then
			expectedMainDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve",
					Namespace: nsName,
				}, expectedMainDeployment)
			}).WithContext(ctx).Should(Succeed())

			expectedPrefillDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-prefill",
					Namespace: nsName,
				}, expectedPrefillDeployment)
			}).WithContext(ctx).Should(Succeed())

			// Validate NO PVC storage is configured when storage-initializer is disabled
			validateNoPvcStorage(expectedMainDeployment)
			validateNoPvcStorage(expectedPrefillDeployment)
			// Validate NO storage-initializer was created
			validateNoStorageInitializer(expectedMainDeployment)
			validateNoStorageInitializer(expectedPrefillDeployment)
		})

		It("should NOT configure OCI storage when storage-initializer is disabled", func(ctx SpecContext) {
			// given
			svcName := "test-llm-storage-oci-si-disabled"
			nsName := kmeta.ChildName(svcName, "-test")
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: nsName,
				},
			}
			Expect(envTest.Client.Create(ctx, namespace)).To(Succeed())
			Expect(envTest.Client.Create(ctx, IstioShadowService(svcName, nsName))).To(Succeed())
			defer func() {
				envTest.DeleteAll(namespace)
			}()

			modelURL, err := apis.ParseURL("oci://registry.io/user-id/repo-id:tag")
			Expect(err).ToNot(HaveOccurred())

			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      svcName,
					Namespace: nsName,
				},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Model: v1alpha2.LLMModelSpec{
						Name: ptr.To("foo"),
						URI:  *modelURL,
					},
					StorageInitializer: &v1alpha2.StorageInitializerSpec{
						Enabled: ptr.To(false),
					},
					WorkloadSpec: v1alpha2.WorkloadSpec{},
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
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// then
			expectedMainDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve",
					Namespace: nsName,
				}, expectedMainDeployment)
			}).WithContext(ctx).Should(Succeed())

			expectedPrefillDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-prefill",
					Namespace: nsName,
				}, expectedPrefillDeployment)
			}).WithContext(ctx).Should(Succeed())

			// Validate NO OCI storage is configured when storage-initializer is disabled
			validateNoOciStorage(expectedMainDeployment)
			validateNoOciStorage(expectedPrefillDeployment)
			// Validate NO storage-initializer was created
			validateNoStorageInitializer(expectedMainDeployment)
			validateNoStorageInitializer(expectedPrefillDeployment)
		})
	})

	Context("Multi node", func() {
		It("should configure direct PVC mount when model uri starts with pvc://", func(ctx SpecContext) {
			// given
			svcName := "test-llm-storage-pvc-mn"
			nsName := kmeta.ChildName(svcName, "-test")
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: nsName,
				},
			}
			Expect(envTest.Client.Create(ctx, namespace)).To(Succeed())
			Expect(envTest.Client.Create(ctx, IstioShadowService(svcName, nsName))).To(Succeed())
			defer func() {
				envTest.DeleteAll(namespace)
			}()

			modelURL, err := apis.ParseURL("pvc://facebook-models/opt-125m")
			Expect(err).ToNot(HaveOccurred())

			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      svcName,
					Namespace: nsName,
				},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Model: v1alpha2.LLMModelSpec{
						Name: ptr.To("foo"),
						URI:  *modelURL,
					},
					WorkloadSpec: v1alpha2.WorkloadSpec{
						Worker: &corev1.PodSpec{Containers: []corev1.Container{}},
						Parallelism: &v1alpha2.ParallelismSpec{
							Data:      ptr.To[int32](1),
							DataLocal: ptr.To[int32](1),
						},
					},
					Router: &v1alpha2.RouterSpec{
						Route:     &v1alpha2.GatewayRoutesSpec{},
						Gateway:   &v1alpha2.GatewaySpec{},
						Scheduler: &v1alpha2.SchedulerSpec{},
					},
					Prefill: &v1alpha2.WorkloadSpec{
						Worker: &corev1.PodSpec{Containers: []corev1.Container{}},
						Parallelism: &v1alpha2.ParallelismSpec{
							Data:      ptr.To[int32](1),
							DataLocal: ptr.To[int32](1),
						},
					},
				},
			}

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// then
			expectedMainLWS := &lwsapi.LeaderWorkerSet{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-mn",
					Namespace: nsName,
				}, expectedMainLWS)
			}).WithContext(ctx).Should(Succeed())

			expectedPrefillLWS := &lwsapi.LeaderWorkerSet{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-mn-prefill",
					Namespace: nsName,
				}, expectedPrefillLWS)
			}).WithContext(ctx).Should(Succeed())

			validatePvcStorageIsConfiguredForLWS(expectedMainLWS)
			validatePvcStorageIsConfiguredForLWS(expectedPrefillLWS)
		})

		It("should configure a modelcar when model uri starts with oci://", func(ctx SpecContext) {
			// given
			svcName := "test-llm-storage-oci-mn"
			nsName := kmeta.ChildName(svcName, "-test")
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: nsName,
				},
			}
			Expect(envTest.Client.Create(ctx, namespace)).To(Succeed())
			Expect(envTest.Client.Create(ctx, IstioShadowService(svcName, nsName))).To(Succeed())
			defer func() {
				envTest.DeleteAll(namespace)
			}()

			modelURL, err := apis.ParseURL("oci://registry.io/user-id/repo-id:tag")
			Expect(err).ToNot(HaveOccurred())

			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      svcName,
					Namespace: nsName,
				},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Model: v1alpha2.LLMModelSpec{
						Name: ptr.To("foo"),
						URI:  *modelURL,
					},
					WorkloadSpec: v1alpha2.WorkloadSpec{
						Worker: &corev1.PodSpec{
							Containers: []corev1.Container{},
						},
						Parallelism: &v1alpha2.ParallelismSpec{
							Data:      ptr.To[int32](1),
							DataLocal: ptr.To[int32](1),
						},
					},
					Router: &v1alpha2.RouterSpec{
						Route:     &v1alpha2.GatewayRoutesSpec{},
						Gateway:   &v1alpha2.GatewaySpec{},
						Scheduler: &v1alpha2.SchedulerSpec{},
					},
					Prefill: &v1alpha2.WorkloadSpec{
						Worker: &corev1.PodSpec{Containers: []corev1.Container{}},
						Parallelism: &v1alpha2.ParallelismSpec{
							Data:      ptr.To[int32](1),
							DataLocal: ptr.To[int32](1),
						},
					},
				},
			}

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// then
			expectedMainLWS := &lwsapi.LeaderWorkerSet{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-mn",
					Namespace: nsName,
				}, expectedMainLWS)
			}).WithContext(ctx).Should(Succeed())

			expectedPrefillLWS := &lwsapi.LeaderWorkerSet{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-mn-prefill",
					Namespace: nsName,
				}, expectedPrefillLWS)
			}).WithContext(ctx).Should(Succeed())

			validateOciStorageIsConfiguredForLWS(expectedMainLWS)
			validateOciStorageIsConfiguredForLWS(expectedPrefillLWS)
		})

		It("should use storage-initializer to download model when uri starts with hf://", func(ctx SpecContext) {
			// given
			svcName := "test-llm-storage-hf-mn"
			nsName := kmeta.ChildName(svcName, "-test")
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: nsName,
				},
			}
			Expect(envTest.Client.Create(ctx, namespace)).To(Succeed())
			Expect(envTest.Client.Create(ctx, IstioShadowService(svcName, nsName))).To(Succeed())
			defer func() {
				envTest.DeleteAll(namespace)
			}()

			modelURL, err := apis.ParseURL("hf://user-id/repo-id:tag")
			Expect(err).ToNot(HaveOccurred())

			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      svcName,
					Namespace: nsName,
				},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Model: v1alpha2.LLMModelSpec{
						Name: ptr.To("foo"),
						URI:  *modelURL,
					},
					WorkloadSpec: v1alpha2.WorkloadSpec{
						Worker: &corev1.PodSpec{
							Containers: []corev1.Container{},
						},
						Parallelism: &v1alpha2.ParallelismSpec{
							Data:      ptr.To[int32](1),
							DataLocal: ptr.To[int32](1),
						},
					},
					Router: &v1alpha2.RouterSpec{
						Route:     &v1alpha2.GatewayRoutesSpec{},
						Gateway:   &v1alpha2.GatewaySpec{},
						Scheduler: &v1alpha2.SchedulerSpec{},
					},
					Prefill: &v1alpha2.WorkloadSpec{
						Worker: &corev1.PodSpec{Containers: []corev1.Container{}},
						Parallelism: &v1alpha2.ParallelismSpec{
							Data:      ptr.To[int32](1),
							DataLocal: ptr.To[int32](1),
						},
					},
				},
			}

			// when
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// then
			expectedMainLWS := &lwsapi.LeaderWorkerSet{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-mn",
					Namespace: nsName,
				}, expectedMainLWS)
			}).WithContext(ctx).Should(Succeed())

			expectedPrefillLWS := &lwsapi.LeaderWorkerSet{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-mn-prefill",
					Namespace: nsName,
				}, expectedPrefillLWS)
			}).WithContext(ctx).Should(Succeed())

			validateStorageInitializerIsConfiguredForLWS(expectedMainLWS, "hf://user-id/repo-id:tag")
			validateStorageInitializerIsConfiguredForLWS(expectedPrefillLWS, "hf://user-id/repo-id:tag")
		})

		It("multi node should use storage-initializer and set proper env variables when uri starts with hf:// and credentials are configured", func(ctx SpecContext) {
			// setup test dependencies
			svcName := "test-llm-storage-hf-mn-with-credentials"
			nsName := kmeta.ChildName(svcName, "-test")
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: nsName,
				},
			}
			Expect(envTest.Client.Create(ctx, namespace)).To(Succeed())
			Expect(envTest.Client.Create(ctx, IstioShadowService(svcName, nsName))).To(Succeed())
			defer func() {
				envTest.DeleteAll(namespace)
			}()

			secretName := kmeta.ChildName(svcName, "-secret")
			hfTokenValue := "test-token"
			credentialSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: nsName,
				},
				StringData: map[string]string{
					hf.HFTokenKey: hfTokenValue,
				},
			}
			Expect(envTest.Client.Create(ctx, credentialSecret)).To(Succeed())

			serviceAccountName := kmeta.ChildName(svcName, "-sa")
			serviceAccount := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceAccountName,
					Namespace: nsName,
				},
				Secrets: []corev1.ObjectReference{
					{
						Name:      secretName,
						Namespace: nsName,
					},
				},
			}
			Expect(envTest.Client.Create(ctx, serviceAccount)).To(Succeed())

			modelURL, err := apis.ParseURL("hf://user-id/repo-id:tag")
			Expect(err).ToNot(HaveOccurred())

			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      svcName,
					Namespace: nsName,
				},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Model: v1alpha2.LLMModelSpec{
						Name: ptr.To("foo"),
						URI:  *modelURL,
					},
					WorkloadSpec: v1alpha2.WorkloadSpec{
						Template: &corev1.PodSpec{
							ServiceAccountName: serviceAccountName,
							Containers:         []corev1.Container{},
						},
						Worker: &corev1.PodSpec{
							ServiceAccountName: serviceAccountName,
							Containers:         []corev1.Container{},
						},
						Parallelism: &v1alpha2.ParallelismSpec{
							Data:      ptr.To[int32](1),
							DataLocal: ptr.To[int32](1),
						},
					},
					Router: &v1alpha2.RouterSpec{
						Route:     &v1alpha2.GatewayRoutesSpec{},
						Gateway:   &v1alpha2.GatewaySpec{},
						Scheduler: &v1alpha2.SchedulerSpec{},
					},
					Prefill: &v1alpha2.WorkloadSpec{
						Template: &corev1.PodSpec{
							ServiceAccountName: serviceAccountName,
							Containers:         []corev1.Container{},
						},
						Worker: &corev1.PodSpec{
							ServiceAccountName: serviceAccountName,
							Containers:         []corev1.Container{},
						},
						Parallelism: &v1alpha2.ParallelismSpec{
							Data:      ptr.To[int32](1),
							DataLocal: ptr.To[int32](1),
						},
					},
				},
			}

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// retrieve the created LeaderWorkerSets
			expectedMainLWS := &lwsapi.LeaderWorkerSet{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-mn",
					Namespace: nsName,
				}, expectedMainLWS)
			}).WithContext(ctx).Should(Succeed())

			expectedPrefillLWS := &lwsapi.LeaderWorkerSet{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-mn-prefill",
					Namespace: nsName,
				}, expectedPrefillLWS)
			}).WithContext(ctx).Should(Succeed())

			// validate the storage initializer configuration in the leader worker sets
			validateStorageInitializerIsConfiguredForLWS(expectedMainLWS, "hf://user-id/repo-id:tag")
			validateStorageInitializerIsConfiguredForLWS(expectedPrefillLWS, "hf://user-id/repo-id:tag")

			// validate the storage initializer credentials in the leader worker sets
			expectedEnvVars := []corev1.EnvVar{
				{
					Name: hf.HFTokenKey,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: secretName,
							},
							Key: "HF_TOKEN",
						},
					},
				},
			}
			validateStorageInitializerCredentialsForLWS(expectedMainLWS, expectedEnvVars)
			validateStorageInitializerCredentialsForLWS(expectedPrefillLWS, expectedEnvVars)
		})

		It("should use storage-initializer to download model when uri starts with s3://", func(ctx SpecContext) {
			// create the global s3 ca bundle configmap
			globalS3CaBundleconfigMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "global-s3-custom-certs",
					Namespace: constants.KServeNamespace,
				},
				Data: map[string]string{
					"cabundle.crt": "global-test-cert",
				},
			}
			Expect(envTest.Client.Create(ctx, globalS3CaBundleconfigMap)).To(Succeed())
			defer func() {
				Expect(envTest.Client.Delete(ctx, globalS3CaBundleconfigMap)).To(Succeed())
			}()

			// patch the infernceservice-config configmap
			patchInferenceServiceConfig(ctx, "storageInitializer", isvcConfigPatchStorageInit)
			defer restoreInferenceServiceConfig(ctx)

			// setup test dependencies
			svcName := "test-llm-storage-s3-mn"
			nsName := kmeta.ChildName(svcName, "-test")
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: nsName,
				},
			}
			Expect(envTest.Client.Create(ctx, namespace)).To(Succeed())
			defer func() {
				envTest.DeleteAll(namespace)
			}()

			Expect(envTest.Client.Create(ctx, IstioShadowService(svcName, nsName))).To(Succeed())

			modelURL, err := apis.ParseURL("s3://user-id/repo-id:tag")
			Expect(err).ToNot(HaveOccurred())

			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      svcName,
					Namespace: nsName,
				},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Model: v1alpha2.LLMModelSpec{
						Name: ptr.To("foo"),
						URI:  *modelURL,
					},
					WorkloadSpec: v1alpha2.WorkloadSpec{
						Worker: &corev1.PodSpec{
							Containers: []corev1.Container{},
						},
						Parallelism: &v1alpha2.ParallelismSpec{
							Data:      ptr.To[int32](1),
							DataLocal: ptr.To[int32](1),
						},
					},
					Router: &v1alpha2.RouterSpec{
						Route:     &v1alpha2.GatewayRoutesSpec{},
						Gateway:   &v1alpha2.GatewaySpec{},
						Scheduler: &v1alpha2.SchedulerSpec{},
					},
					Prefill: &v1alpha2.WorkloadSpec{
						Worker: &corev1.PodSpec{Containers: []corev1.Container{}},
						Parallelism: &v1alpha2.ParallelismSpec{
							Data:      ptr.To[int32](1),
							DataLocal: ptr.To[int32](1),
						},
					},
				},
			}
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// Ensure the global-ca-bundle config map is created in the llmisvc's namespace
			generatedGlobalCaBundleConfigMap := &corev1.ConfigMap{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      constants.DefaultGlobalCaBundleConfigMapName,
					Namespace: nsName,
				}, generatedGlobalCaBundleConfigMap)
			}).WithContext(ctx).Should(Succeed())

			// retrieve the created leader worker sets
			expectedMainLWS := &lwsapi.LeaderWorkerSet{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-mn",
					Namespace: nsName,
				}, expectedMainLWS)
			}).WithContext(ctx).Should(Succeed())

			expectedPrefillLWS := &lwsapi.LeaderWorkerSet{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-mn-prefill",
					Namespace: nsName,
				}, expectedPrefillLWS)
			}).WithContext(ctx).Should(Succeed())

			// validate the storage initializer configuration in the leader worker sets
			validateStorageInitializerIsConfiguredForLWS(expectedMainLWS, "s3://user-id/repo-id:tag")
			validateStorageInitializerIsConfiguredForLWS(expectedPrefillLWS, "s3://user-id/repo-id:tag")

			// validate the storage initializer credentials are properly set
			expectedEnvVars := []corev1.EnvVar{
				{
					Name:  constants.CaBundleConfigMapNameEnvVarKey,
					Value: constants.DefaultGlobalCaBundleConfigMapName,
				},
				{
					Name:  constants.CaBundleVolumeMountPathEnvVarKey,
					Value: "/path/to/globalcerts",
				},
			}
			validateStorageInitializerCredentialsForLWS(expectedMainLWS, expectedEnvVars)
			validateStorageInitializerCredentialsForLWS(expectedPrefillLWS, expectedEnvVars)

			expectedCaBundleVolume := []corev1.Volume{
				{
					Name: CaBundleVolumeName,
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: constants.DefaultGlobalCaBundleConfigMapName,
							},
						},
					},
				},
			}
			validateStorageInitializerVolumesForLWS(expectedMainLWS, expectedCaBundleVolume)
			validateStorageInitializerVolumesForLWS(expectedPrefillLWS, expectedCaBundleVolume)

			expectedCaBundleVolumeMount := []corev1.VolumeMount{
				{
					Name:      CaBundleVolumeName,
					MountPath: "/path/to/globalcerts",
					ReadOnly:  true,
				},
			}
			validateStorageInitializerVolumeMountsForLWS(expectedMainLWS, expectedCaBundleVolumeMount)
			validateStorageInitializerVolumeMountsForLWS(expectedPrefillLWS, expectedCaBundleVolumeMount)
		})

		It("should use storage-initializer to download model when uri starts with s3:// and s3 config is configured", func(ctx SpecContext) {
			// patch the infernceservice-config configmap
			patchInferenceServiceConfig(ctx, "credentials", isvcConfigPatchCredentials)
			defer restoreInferenceServiceConfig(ctx)

			// setup test dependencies
			svcName := "test-llm-storage-s3-config-mn"
			nsName := kmeta.ChildName(svcName, "-test")
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: nsName,
				},
			}
			Expect(envTest.Client.Create(ctx, namespace)).To(Succeed())
			defer func() {
				envTest.DeleteAll(namespace)
			}()

			Expect(envTest.Client.Create(ctx, IstioShadowService(svcName, nsName))).To(Succeed())

			localCaBundleConfigMapName := "local-s3-custom-certs"
			localCaBundleconfigMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      localCaBundleConfigMapName,
					Namespace: nsName,
				},
				Data: map[string]string{
					"localcerts.crt": "test-cert",
				},
			}
			Expect(envTest.Client.Create(ctx, localCaBundleconfigMap)).To(Succeed())

			secretName := kmeta.ChildName(svcName, "-secret")
			s3Endpoint := "s3-config-credentials-test.kserve:9000"
			s3UseHttps := "0"
			s3Region := "us-south"
			s3Anon := "false"
			credentialSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: nsName,
					Annotations: map[string]string{
						s3.InferenceServiceS3SecretEndpointAnnotation: s3Endpoint,
						s3.InferenceServiceS3SecretHttpsAnnotation:    s3UseHttps,
						s3.InferenceServiceS3SecretRegionAnnotation:   s3Region,
						s3.InferenceServiceS3UseAnonymousCredential:   s3Anon,
					},
				},
				StringData: map[string]string{
					s3.AWSAccessKeyId:     "test-id",
					s3.AWSSecretAccessKey: "test-secret",
				},
			}
			Expect(envTest.Client.Create(ctx, credentialSecret)).To(Succeed())

			serviceAccountName := kmeta.ChildName(svcName, "-sa")
			serviceAccount := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceAccountName,
					Namespace: nsName,
				},
				Secrets: []corev1.ObjectReference{
					{
						Name:      secretName,
						Namespace: nsName,
					},
				},
			}
			Expect(envTest.Client.Create(ctx, serviceAccount)).To(Succeed())

			modelURL, err := apis.ParseURL("s3://user-id/repo-id:tag")
			Expect(err).ToNot(HaveOccurred())

			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      svcName,
					Namespace: nsName,
				},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Model: v1alpha2.LLMModelSpec{
						Name: ptr.To("foo"),
						URI:  *modelURL,
					},
					WorkloadSpec: v1alpha2.WorkloadSpec{
						Template: &corev1.PodSpec{
							ServiceAccountName: serviceAccountName,
							Containers:         []corev1.Container{},
						},
						Worker: &corev1.PodSpec{
							ServiceAccountName: serviceAccountName,
							Containers:         []corev1.Container{},
						},
						Parallelism: &v1alpha2.ParallelismSpec{
							Data:      ptr.To[int32](1),
							DataLocal: ptr.To[int32](1),
						},
					},
					Router: &v1alpha2.RouterSpec{
						Route:     &v1alpha2.GatewayRoutesSpec{},
						Gateway:   &v1alpha2.GatewaySpec{},
						Scheduler: &v1alpha2.SchedulerSpec{},
					},
					Prefill: &v1alpha2.WorkloadSpec{
						Template: &corev1.PodSpec{
							ServiceAccountName: serviceAccountName,
							Containers:         []corev1.Container{},
						},
						Worker: &corev1.PodSpec{
							ServiceAccountName: serviceAccountName,
							Containers:         []corev1.Container{},
						},
						Parallelism: &v1alpha2.ParallelismSpec{
							Data:      ptr.To[int32](1),
							DataLocal: ptr.To[int32](1),
						},
					},
				},
			}
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// retrieve the created leader worker sets
			expectedMainLWS := &lwsapi.LeaderWorkerSet{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-mn",
					Namespace: nsName,
				}, expectedMainLWS)
			}).WithContext(ctx).Should(Succeed())

			expectedPrefillLWS := &lwsapi.LeaderWorkerSet{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-mn-prefill",
					Namespace: nsName,
				}, expectedPrefillLWS)
			}).WithContext(ctx).Should(Succeed())

			// validate the storage initializer configuration in the leader worker sets
			validateStorageInitializerIsConfiguredForLWS(expectedMainLWS, "s3://user-id/repo-id:tag")
			validateStorageInitializerIsConfiguredForLWS(expectedPrefillLWS, "s3://user-id/repo-id:tag")

			// validate the storage initializer credentials are properly set
			expectedEnvVars := createExpectedS3ConfigEnvVars(
				secretName,
				s3UseHttps,
				s3Endpoint,
				s3Anon,
				"http://"+s3Endpoint,
				s3Region,
				localCaBundleConfigMapName,
				"/path/to",
				"/path/to/localcerts.crt",
			)
			validateStorageInitializerCredentialsForLWS(expectedMainLWS, expectedEnvVars)
			validateStorageInitializerCredentialsForLWS(expectedPrefillLWS, expectedEnvVars)

			expectedCaBundleVolume := []corev1.Volume{
				{
					Name: CaBundleVolumeName,
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: localCaBundleConfigMapName,
							},
						},
					},
				},
			}
			validateStorageInitializerVolumesForLWS(expectedMainLWS, expectedCaBundleVolume)
			validateStorageInitializerVolumesForLWS(expectedPrefillLWS, expectedCaBundleVolume)

			expectedCaBundleVolumeMount := []corev1.VolumeMount{
				{
					Name:      CaBundleVolumeName,
					MountPath: "/path/to",
					ReadOnly:  true,
				},
			}
			validateStorageInitializerVolumeMountsForLWS(expectedMainLWS, expectedCaBundleVolumeMount)
			validateStorageInitializerVolumeMountsForLWS(expectedPrefillLWS, expectedCaBundleVolumeMount)
		})

		It("should use storage-initializer and set proper env variables when uri starts with s3:// and credentials are configured", func(ctx SpecContext) {
			// create the global s3 ca bundle configmap
			globalS3CaBundleconfigMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "global-s3-custom-certs",
					Namespace: constants.KServeNamespace,
				},
				Data: map[string]string{
					"cabundle.crt": "global-test-cert",
				},
			}
			Expect(envTest.Client.Create(ctx, globalS3CaBundleconfigMap)).To(Succeed())
			defer func() {
				Expect(envTest.Client.Delete(ctx, globalS3CaBundleconfigMap)).To(Succeed())
			}()

			// patch the infernceservice-config configmap
			patchInferenceServiceConfig(ctx, "storageInitializer", isvcConfigPatchStorageInit)
			defer restoreInferenceServiceConfig(ctx)

			// setup test dependencies
			svcName := "test-llm-storage-s3-mn-with-credentials"
			nsName := kmeta.ChildName(svcName, "-test")
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: nsName,
				},
			}
			Expect(envTest.Client.Create(ctx, namespace)).To(Succeed())
			defer func() {
				envTest.DeleteAll(namespace)
			}()

			Expect(envTest.Client.Create(ctx, IstioShadowService(svcName, nsName))).To(Succeed())

			s3CaBundleConfigMapName := "s3-custom-certs"
			s3CaBundleconfigMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      s3CaBundleConfigMapName,
					Namespace: nsName,
				},
				Data: map[string]string{
					"s3.crt": "test-cert",
				},
			}
			Expect(envTest.Client.Create(ctx, s3CaBundleconfigMap)).To(Succeed())

			secretName := kmeta.ChildName(svcName, "-secret")
			s3Endpoint := "s3-credentials-test.kserve:9000"
			s3UseHttps := "0"
			s3Region := "us-south"
			s3Anon := "false"
			credentialSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: nsName,
					Annotations: map[string]string{
						s3.InferenceServiceS3SecretEndpointAnnotation:    s3Endpoint,
						s3.InferenceServiceS3SecretHttpsAnnotation:       s3UseHttps,
						s3.InferenceServiceS3SecretRegionAnnotation:      s3Region,
						s3.InferenceServiceS3UseAnonymousCredential:      s3Anon,
						s3.InferenceServiceS3CABundleConfigMapAnnotation: s3CaBundleConfigMapName,
						s3.InferenceServiceS3CABundleAnnotation:          "/path/to/s3.crt",
					},
				},
				StringData: map[string]string{
					s3.AWSAccessKeyId:     "test-id",
					s3.AWSSecretAccessKey: "test-secret",
				},
			}
			Expect(envTest.Client.Create(ctx, credentialSecret)).To(Succeed())

			serviceAccountName := kmeta.ChildName(svcName, "-sa")
			serviceAccount := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceAccountName,
					Namespace: nsName,
				},
				Secrets: []corev1.ObjectReference{
					{
						Name:      secretName,
						Namespace: nsName,
					},
				},
			}
			Expect(envTest.Client.Create(ctx, serviceAccount)).To(Succeed())

			modelURL, err := apis.ParseURL("s3://bucket/model")
			Expect(err).ToNot(HaveOccurred())

			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      svcName,
					Namespace: nsName,
				},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Model: v1alpha2.LLMModelSpec{
						Name: ptr.To("foo"),
						URI:  *modelURL,
					},
					WorkloadSpec: v1alpha2.WorkloadSpec{
						Template: &corev1.PodSpec{
							ServiceAccountName: serviceAccountName,
							Containers:         []corev1.Container{},
						},
						Worker: &corev1.PodSpec{
							ServiceAccountName: serviceAccountName,
							Containers:         []corev1.Container{},
						},
						Parallelism: &v1alpha2.ParallelismSpec{
							Data:      ptr.To[int32](1),
							DataLocal: ptr.To[int32](1),
						},
					},
					Router: &v1alpha2.RouterSpec{
						Route:     &v1alpha2.GatewayRoutesSpec{},
						Gateway:   &v1alpha2.GatewaySpec{},
						Scheduler: &v1alpha2.SchedulerSpec{},
					},
					Prefill: &v1alpha2.WorkloadSpec{
						Template: &corev1.PodSpec{
							ServiceAccountName: serviceAccountName,
							Containers:         []corev1.Container{},
						},
						Worker: &corev1.PodSpec{
							ServiceAccountName: serviceAccountName,
							Containers:         []corev1.Container{},
						},
						Parallelism: &v1alpha2.ParallelismSpec{
							Data:      ptr.To[int32](1),
							DataLocal: ptr.To[int32](1),
						},
					},
				},
			}

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// retrieve the created leader worker sets
			expectedMainLWS := &lwsapi.LeaderWorkerSet{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-mn",
					Namespace: nsName,
				}, expectedMainLWS)
			}).WithContext(ctx).Should(Succeed())

			expectedPrefillLWS := &lwsapi.LeaderWorkerSet{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-mn-prefill",
					Namespace: nsName,
				}, expectedPrefillLWS)
			}).WithContext(ctx).Should(Succeed())

			// validate the storage initializer configuration in the leader worker sets
			validateStorageInitializerIsConfiguredForLWS(expectedMainLWS, "s3://bucket/model")
			validateStorageInitializerIsConfiguredForLWS(expectedPrefillLWS, "s3://bucket/model")

			// validate the storage initializer credentials are properly set
			expectedEnvVars := createExpectedS3ConfigEnvVars(
				credentialSecret.Name,
				s3UseHttps,
				s3Endpoint,
				s3Anon,
				"http://"+s3Endpoint,
				s3Region,
				s3CaBundleConfigMapName,
				"/path/to",
				"/path/to/s3.crt",
			)
			validateStorageInitializerCredentialsForLWS(expectedMainLWS, expectedEnvVars)
			validateStorageInitializerCredentialsForLWS(expectedPrefillLWS, expectedEnvVars)

			expectedCaBundleVolume := []corev1.Volume{
				{
					Name: CaBundleVolumeName,
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: s3CaBundleConfigMapName,
							},
						},
					},
				},
			}
			validateStorageInitializerVolumesForLWS(expectedMainLWS, expectedCaBundleVolume)
			validateStorageInitializerVolumesForLWS(expectedPrefillLWS, expectedCaBundleVolume)

			expectedCaBundleVolumeMount := []corev1.VolumeMount{
				{
					Name:      CaBundleVolumeName,
					MountPath: "/path/to",
					ReadOnly:  true,
				},
			}
			validateStorageInitializerVolumeMountsForLWS(expectedMainLWS, expectedCaBundleVolumeMount)
			validateStorageInitializerVolumeMountsForLWS(expectedPrefillLWS, expectedCaBundleVolumeMount)
		})

		It("should use storage-initializer and set proper env variables when uri starts with s3:// and credentials are configured for IAM", func(ctx SpecContext) {
			// create the global s3 ca bundle configmap
			globalS3CaBundleconfigMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "global-s3-custom-certs",
					Namespace: constants.KServeNamespace,
				},
				Data: map[string]string{
					"cabundle.crt": "global-test-cert",
				},
			}
			Expect(envTest.Client.Create(ctx, globalS3CaBundleconfigMap)).To(Succeed())
			defer func() {
				Expect(envTest.Client.Delete(ctx, globalS3CaBundleconfigMap)).To(Succeed())
			}()

			// patch the infernceservice-config configmap
			patchInferenceServiceConfig(ctx, "storageInitializer", isvcConfigPatchStorageInit)
			defer restoreInferenceServiceConfig(ctx)

			// setup test dependencies
			svcName := "test-llm-storage-s3-mn-with-iam-credentials"
			nsName := kmeta.ChildName(svcName, "-test")
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: nsName,
				},
			}
			Expect(envTest.Client.Create(ctx, namespace)).To(Succeed())
			defer func() {
				envTest.DeleteAll(namespace)
			}()

			Expect(envTest.Client.Create(ctx, IstioShadowService(svcName, nsName))).To(Succeed())

			s3CaBundleConfigMapName := "s3-custom-certs"
			s3CaBundleconfigMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      s3CaBundleConfigMapName,
					Namespace: nsName,
				},
				Data: map[string]string{
					"s3.crt": "test-cert",
				},
			}
			Expect(envTest.Client.Create(ctx, s3CaBundleconfigMap)).To(Succeed())

			serviceAccountName := kmeta.ChildName(svcName, "-sa")
			s3IamRole := "arn:aws:iam::123456789012:role/s3access"
			s3Endpoint := "s3-credentials-test.kserve:9000"
			s3UseHttps := "0"
			s3Region := "us-south"
			s3Anon := "false"
			serviceAccount := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceAccountName,
					Namespace: nsName,
					Annotations: map[string]string{
						credentials.AwsIrsaAnnotationKey:                 s3IamRole,
						s3.InferenceServiceS3SecretEndpointAnnotation:    s3Endpoint,
						s3.InferenceServiceS3SecretHttpsAnnotation:       s3UseHttps,
						s3.InferenceServiceS3SecretRegionAnnotation:      s3Region,
						s3.InferenceServiceS3UseAnonymousCredential:      s3Anon,
						s3.InferenceServiceS3CABundleConfigMapAnnotation: s3CaBundleConfigMapName,
						s3.InferenceServiceS3CABundleAnnotation:          "/path/to/s3.crt",
					},
				},
			}
			Expect(envTest.Client.Create(ctx, serviceAccount)).To(Succeed())

			modelURL, err := apis.ParseURL("s3://bucket/model")
			Expect(err).ToNot(HaveOccurred())

			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      svcName,
					Namespace: nsName,
				},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					Model: v1alpha2.LLMModelSpec{
						Name: ptr.To("foo"),
						URI:  *modelURL,
					},
					WorkloadSpec: v1alpha2.WorkloadSpec{
						Template: &corev1.PodSpec{
							ServiceAccountName: serviceAccountName,
							Containers:         []corev1.Container{},
						},
						Worker: &corev1.PodSpec{
							ServiceAccountName: serviceAccountName,
							Containers:         []corev1.Container{},
						},
						Parallelism: &v1alpha2.ParallelismSpec{
							Data:      ptr.To[int32](1),
							DataLocal: ptr.To[int32](1),
						},
					},
					Router: &v1alpha2.RouterSpec{
						Route:     &v1alpha2.GatewayRoutesSpec{},
						Gateway:   &v1alpha2.GatewaySpec{},
						Scheduler: &v1alpha2.SchedulerSpec{},
					},
					Prefill: &v1alpha2.WorkloadSpec{
						Template: &corev1.PodSpec{
							ServiceAccountName: serviceAccountName,
							Containers:         []corev1.Container{},
						},
						Worker: &corev1.PodSpec{
							ServiceAccountName: serviceAccountName,
							Containers:         []corev1.Container{},
						},
						Parallelism: &v1alpha2.ParallelismSpec{
							Data:      ptr.To[int32](1),
							DataLocal: ptr.To[int32](1),
						},
					},
				},
			}

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				Expect(envTest.Delete(ctx, llmSvc)).To(Succeed())
			}()

			// retrieve the created leader worker sets
			expectedMainLWS := &lwsapi.LeaderWorkerSet{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-mn",
					Namespace: nsName,
				}, expectedMainLWS)
			}).WithContext(ctx).Should(Succeed())

			expectedPrefillLWS := &lwsapi.LeaderWorkerSet{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-kserve-mn-prefill",
					Namespace: nsName,
				}, expectedPrefillLWS)
			}).WithContext(ctx).Should(Succeed())

			// validate the storage initializer configuration in the leader worker sets
			validateStorageInitializerIsConfiguredForLWS(expectedMainLWS, "s3://bucket/model")
			validateStorageInitializerIsConfiguredForLWS(expectedPrefillLWS, "s3://bucket/model")

			// validate the storage initializer credentials are properly set
			expectedEnvVars := createExpectedS3ConfigEnvVars(
				"",
				s3UseHttps,
				s3Endpoint,
				s3Anon,
				"http://"+s3Endpoint,
				s3Region,
				s3CaBundleConfigMapName,
				"/path/to",
				"/path/to/s3.crt",
			)
			validateStorageInitializerCredentialsForLWS(expectedMainLWS, expectedEnvVars)
			validateStorageInitializerCredentialsForLWS(expectedPrefillLWS, expectedEnvVars)

			expectedCaBundleVolume := []corev1.Volume{
				{
					Name: CaBundleVolumeName,
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: s3CaBundleConfigMapName,
							},
						},
					},
				},
			}
			validateStorageInitializerVolumesForLWS(expectedMainLWS, expectedCaBundleVolume)
			validateStorageInitializerVolumesForLWS(expectedPrefillLWS, expectedCaBundleVolume)

			expectedCaBundleVolumeMount := []corev1.VolumeMount{
				{
					Name:      CaBundleVolumeName,
					MountPath: "/path/to",
					ReadOnly:  true,
				},
			}
			validateStorageInitializerVolumeMountsForLWS(expectedMainLWS, expectedCaBundleVolumeMount)
			validateStorageInitializerVolumeMountsForLWS(expectedPrefillLWS, expectedCaBundleVolumeMount)

			// validate the role-arn annotation was properly propagated to the created service account
			expectedMainServiceAccount := &corev1.ServiceAccount{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-sa",
					Namespace: nsName,
				}, expectedMainServiceAccount)
			}).WithContext(ctx).Should(Succeed())

			expectedPrefillServiceAccount := &corev1.ServiceAccount{}
			Eventually(func(g Gomega, ctx context.Context) error {
				return envTest.Get(ctx, types.NamespacedName{
					Name:      svcName + "-sa",
					Namespace: nsName,
				}, expectedPrefillServiceAccount)
			}).WithContext(ctx).Should(Succeed())

			Expect(expectedMainServiceAccount.Annotations[credentials.AwsIrsaAnnotationKey]).To(BeEquivalentTo(s3IamRole))
			Expect(expectedPrefillServiceAccount.Annotations[credentials.AwsIrsaAnnotationKey]).To(BeEquivalentTo(s3IamRole))
		})
	})
})

func validatePvcStorageIsConfigured(deployment *appsv1.Deployment) {
	validatePvcStorageForPodSpec(&deployment.Spec.Template.Spec)
}

func validateOciStorageIsConfigured(deployment *appsv1.Deployment) {
	validateOciStorageForPodSpec(&deployment.Spec.Template.Spec)
}

func validateStorageInitializerIsConfigured(deployment *appsv1.Deployment, storageUri string) {
	validateStorageInitializerForPodSpec(&deployment.Spec.Template.Spec, storageUri)
}

func validateStorageInitializerCredentials(deployment *appsv1.Deployment, envVars []corev1.EnvVar) {
	validatePodSpecEnvVars(&deployment.Spec.Template.Spec, envVars)
}

func validateStorageInitializerVolumes(deployment *appsv1.Deployment, volumes []corev1.Volume) {
	validatePodSpecVolumes(&deployment.Spec.Template.Spec, volumes)
}

func validateStorageInitializerVolumeMounts(deployment *appsv1.Deployment, volumeMounts []corev1.VolumeMount) {
	validatePodSpecVolumeMounts(&deployment.Spec.Template.Spec, volumeMounts)
}

func validatePvcStorageIsConfiguredForLWS(lws *lwsapi.LeaderWorkerSet) {
	workerSpec := lws.Spec.LeaderWorkerTemplate.WorkerTemplate.Spec
	validatePvcStorageForPodSpec(&workerSpec)
}

func validatePvcStorageForPodSpec(podSpec *corev1.PodSpec) {
	mainContainer := utils.GetContainerWithName(podSpec, "main")
	Expect(mainContainer).ToNot(BeNil())

	Expect(podSpec.Volumes).To(ContainElement(And(
		HaveField("Name", constants.PvcSourceMountName),
		HaveField("VolumeSource.PersistentVolumeClaim.ClaimName", "facebook-models"),
	)))

	Expect(mainContainer.VolumeMounts).To(ContainElement(And(
		HaveField("Name", constants.PvcSourceMountName),
		HaveField("MountPath", constants.DefaultModelLocalMountPath),
		HaveField("ReadOnly", BeTrue()),
		HaveField("SubPath", "opt-125m"),
	)))
}

func validateOciStorageIsConfiguredForLWS(lws *lwsapi.LeaderWorkerSet) {
	workerSpec := lws.Spec.LeaderWorkerTemplate.WorkerTemplate.Spec
	validateOciStorageForPodSpec(&workerSpec)
}

func validateOciStorageForPodSpec(podSpec *corev1.PodSpec) {
	// Check the main container and modelcar container are present.
	mainContainer := utils.GetContainerWithName(podSpec, "main")
	Expect(mainContainer).ToNot(BeNil())
	modelcarContainer := utils.GetContainerWithName(podSpec, constants.ModelcarContainerName)
	Expect(modelcarContainer).ToNot(BeNil())

	// Check container are sharing resources.
	Expect(podSpec.ShareProcessNamespace).To(Not(BeNil()))
	Expect(*podSpec.ShareProcessNamespace).To(BeTrue())

	// Check the model server has an envvar indicating that the model may not be mounted immediately.
	Expect(mainContainer.Env).To(ContainElement(And(
		HaveField("Name", constants.ModelInitModeEnvVarKey),
		HaveField("Value", "async"),
	)))

	// Check OCI init container for the pre-fetch
	Expect(podSpec.InitContainers).To(ContainElement(And(
		HaveField("Name", constants.ModelcarInitContainerName),
		HaveField("Resources.Limits", And(HaveKey(corev1.ResourceCPU), HaveKey(corev1.ResourceMemory))),
		HaveField("Resources.Requests", And(HaveKey(corev1.ResourceCPU), HaveKey(corev1.ResourceMemory))),
	)))

	// Basic check of empty dir volume is configured (shared mount between the containers)
	Expect(podSpec.Volumes).To(ContainElement(HaveField("Name", constants.StorageInitializerVolumeName)))

	// Check that the empty-dir volume is mounted to the modelcar and main container (shared storage)
	Expect(mainContainer.VolumeMounts).To(ContainElement(And(
		HaveField("Name", constants.StorageInitializerVolumeName),
		HaveField("MountPath", "/mnt"),
	)))
	Expect(modelcarContainer.VolumeMounts).To(ContainElement(And(
		HaveField("Name", constants.StorageInitializerVolumeName),
		HaveField("MountPath", "/mnt"),
		HaveField("ReadOnly", false),
	)))
}

func validateStorageInitializerIsConfiguredForLWS(lws *lwsapi.LeaderWorkerSet, storageUri string) {
	workerSpec := lws.Spec.LeaderWorkerTemplate.WorkerTemplate.Spec
	validateStorageInitializerForPodSpec(&workerSpec, storageUri)
}

func validateStorageInitializerCredentialsForLWS(lws *lwsapi.LeaderWorkerSet, envVars []corev1.EnvVar) {
	leaderSpec := lws.Spec.LeaderWorkerTemplate.LeaderTemplate.Spec
	workerSpec := lws.Spec.LeaderWorkerTemplate.WorkerTemplate.Spec
	validatePodSpecEnvVars(&leaderSpec, envVars)
	validatePodSpecEnvVars(&workerSpec, envVars)
}

func validateStorageInitializerVolumesForLWS(lws *lwsapi.LeaderWorkerSet, volumes []corev1.Volume) {
	leaderSpec := lws.Spec.LeaderWorkerTemplate.LeaderTemplate.Spec
	workerSpec := lws.Spec.LeaderWorkerTemplate.WorkerTemplate.Spec
	validatePodSpecVolumes(&leaderSpec, volumes)
	validatePodSpecVolumes(&workerSpec, volumes)
}

func validateStorageInitializerVolumeMountsForLWS(lws *lwsapi.LeaderWorkerSet, volumeMounts []corev1.VolumeMount) {
	leaderSpec := lws.Spec.LeaderWorkerTemplate.LeaderTemplate.Spec
	workerSpec := lws.Spec.LeaderWorkerTemplate.WorkerTemplate.Spec
	validatePodSpecVolumeMounts(&leaderSpec, volumeMounts)
	validatePodSpecVolumeMounts(&workerSpec, volumeMounts)
}

func validateStorageInitializerForPodSpec(podSpec *corev1.PodSpec, storageUri string) {
	// Check the volume to store the model exists
	Expect(podSpec.Volumes).To(ContainElement(And(
		HaveField("Name", constants.StorageInitializerVolumeName),
		HaveField("EmptyDir", Not(BeNil())),
	)))

	// Check the storage-initializer container is present.
	Expect(podSpec.InitContainers).To(ContainElement(And(
		HaveField("Name", constants.StorageInitializerContainerName),
		HaveField("Args", ContainElements(storageUri, constants.DefaultModelLocalMountPath)),
		HaveField("VolumeMounts", ContainElement(And(
			HaveField("Name", constants.StorageInitializerVolumeName),
			HaveField("MountPath", constants.DefaultModelLocalMountPath),
		))),
	)))

	// Check the main container has the model mounted
	mainContainer := utils.GetContainerWithName(podSpec, "main")
	Expect(mainContainer).ToNot(BeNil())
	Expect(mainContainer.VolumeMounts).To(ContainElement(And(
		HaveField("Name", constants.StorageInitializerVolumeName),
		HaveField("MountPath", constants.DefaultModelLocalMountPath),
		HaveField("ReadOnly", BeTrue()),
	)))
}

func validatePodSpecEnvVars(podSpec *corev1.PodSpec, envVars []corev1.EnvVar) {
	initContainer := utils.GetInitContainerWithName(podSpec, constants.StorageInitializerContainerName)
	Expect(initContainer).NotTo(BeNil())
	Expect(initContainer.Env).To(ContainElements(envVars))
}

func validatePodSpecVolumes(podSpec *corev1.PodSpec, volumes []corev1.Volume) {
	Expect(podSpec).NotTo(BeNil())
	Expect(podSpec.Volumes).To(ContainElements(volumes))
}

func validatePodSpecVolumeMounts(podSpec *corev1.PodSpec, volumeMounts []corev1.VolumeMount) {
	initContainer := utils.GetInitContainerWithName(podSpec, constants.StorageInitializerContainerName)
	Expect(initContainer).NotTo(BeNil())
	Expect(initContainer.VolumeMounts).To(ContainElements(volumeMounts))
}

func validateNoStorageInitializer(deployment *appsv1.Deployment) {
	podSpec := &deployment.Spec.Template.Spec

	// Check that storage-initializer container does NOT exist
	initContainer := utils.GetInitContainerWithName(podSpec, constants.StorageInitializerContainerName)
	Expect(initContainer).To(BeNil())

	// Check that storage-initializer volume does NOT exist
	for _, vol := range podSpec.Volumes {
		Expect(vol.Name).NotTo(Equal(constants.StorageInitializerVolumeName))
	}
}

func validateNoPvcStorage(deployment *appsv1.Deployment) {
	podSpec := &deployment.Spec.Template.Spec
	mainContainer := utils.GetContainerWithName(podSpec, "main")
	Expect(mainContainer).ToNot(BeNil())

	// Check that PVC volume does NOT exist
	for _, vol := range podSpec.Volumes {
		Expect(vol.Name).NotTo(Equal(constants.PvcSourceMountName))
	}

	// Check that PVC volume mount does NOT exist in main container
	for _, mount := range mainContainer.VolumeMounts {
		Expect(mount.Name).NotTo(Equal(constants.PvcSourceMountName))
	}
}

func validateNoOciStorage(deployment *appsv1.Deployment) {
	podSpec := &deployment.Spec.Template.Spec

	// Check that modelcar container does NOT exist
	modelcarContainer := utils.GetContainerWithName(podSpec, constants.ModelcarContainerName)
	Expect(modelcarContainer).To(BeNil())

	// Check that modelcar init container does NOT exist
	modelcarInitContainer := utils.GetInitContainerWithName(podSpec, constants.ModelcarInitContainerName)
	Expect(modelcarInitContainer).To(BeNil())

	// Check that main container does NOT have async model init mode env var
	mainContainer := utils.GetContainerWithName(podSpec, "main")
	if mainContainer != nil {
		for _, env := range mainContainer.Env {
			if env.Name == constants.ModelInitModeEnvVarKey {
				Expect(env.Value).NotTo(Equal("async"))
			}
		}
	}
}

func patchInferenceServiceConfig(ctx context.Context, patchKey string, patchValue string) {
	isvcConfigMap := &corev1.ConfigMap{}
	Expect(envTest.Client.Get(ctx, types.NamespacedName{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace}, isvcConfigMap)).To(Succeed())
	patchedIsvcConfigMap := client.MergeFrom(isvcConfigMap.DeepCopy())
	isvcConfigMap.Data[patchKey] = patchValue
	Expect(envTest.Client.Patch(ctx, isvcConfigMap, patchedIsvcConfigMap)).To(Succeed())
}

func restoreInferenceServiceConfig(ctx context.Context) {
	isvcConfigMap := &corev1.ConfigMap{}
	Expect(envTest.Client.Get(ctx, types.NamespacedName{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace}, isvcConfigMap)).To(Succeed())
	restoredIsvcConfigMap := client.MergeFrom(isvcConfigMap.DeepCopy())
	isvcConfigMap.Data = InferenceServiceCfgMap(constants.KServeNamespace).Data
	Expect(envTest.Client.Patch(ctx, isvcConfigMap, restoredIsvcConfigMap)).To(Succeed())
}

//nolint:unparam // keeping params for test flexibility
func createExpectedS3ConfigEnvVars(
	credentialSecretName string,
	s3UseHttps string,
	s3Endpoint string,
	s3Anon string,
	s3EndpointUrl string,
	s3Region string,
	s3CaBundleConfigMapName string,
	s3CABundleDirPath string,
	s3CABundleFilePath string,
) []corev1.EnvVar {
	envVars := []corev1.EnvVar{}
	if credentialSecretName != "" {
		envVars = append(
			envVars,
			[]corev1.EnvVar{
				{
					Name: s3.AWSAccessKeyId,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: credentialSecretName,
							},
							Key: s3.AWSAccessKeyId,
						},
					},
				},
				{
					Name: s3.AWSSecretAccessKey,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: credentialSecretName,
							},
							Key: s3.AWSSecretAccessKey,
						},
					},
				},
			}...,
		)
	}
	if s3UseHttps != "" {
		envVars = append(
			envVars,
			corev1.EnvVar{
				Name:  s3.S3UseHttps,
				Value: s3UseHttps,
			},
		)
	}
	if s3Endpoint != "" {
		envVars = append(
			envVars,
			corev1.EnvVar{
				Name:  s3.S3Endpoint,
				Value: s3Endpoint,
			},
		)
	}
	if s3Anon != "" {
		envVars = append(
			envVars,
			corev1.EnvVar{
				Name:  s3.AWSAnonymousCredential,
				Value: s3Anon,
			},
		)
	}
	if s3EndpointUrl != "" {
		envVars = append(
			envVars,
			corev1.EnvVar{
				Name:  s3.AWSEndpointUrl,
				Value: s3EndpointUrl,
			},
		)
	}
	if s3Region != "" {
		envVars = append(
			envVars,
			corev1.EnvVar{
				Name:  s3.AWSRegion,
				Value: s3Region,
			},
		)
	}
	if s3CaBundleConfigMapName != "" {
		envVars = append(
			envVars,
			[]corev1.EnvVar{
				{
					Name:  s3.AWSCABundleConfigMap,
					Value: s3CaBundleConfigMapName,
				},
				{
					Name:  constants.CaBundleConfigMapNameEnvVarKey,
					Value: s3CaBundleConfigMapName,
				},
			}...,
		)
	}
	if s3CABundleDirPath != "" {
		envVars = append(
			envVars,
			corev1.EnvVar{
				Name:  constants.CaBundleVolumeMountPathEnvVarKey,
				Value: s3CABundleDirPath,
			},
		)
	}
	if s3CABundleFilePath != "" {
		envVars = append(
			envVars,
			corev1.EnvVar{
				Name:  s3.AWSCABundle,
				Value: s3CABundleFilePath,
			},
		)
	}
	return envVars
}

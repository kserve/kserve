/*
Copyright 2026 The KServe Authors.

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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
)

// A predictor model without a storageUri is treated as a multi-model server: the
// controller annotates the pod for agent injection, and the agent injector then
// mounts an emptyDir at the model dir. When the model container already mounts a
// volume there, that second mount makes the pod invalid.
var _ = Describe("Multi-model serving for a predictor model without a storageUri", func() {
	newISVC := func(name, namespace string, modelDirMount bool) *v1beta1.InferenceService {
		isvc := &v1beta1.InferenceService{
			ObjectMeta: metav1.ObjectMeta{
				Name:        name,
				Namespace:   namespace,
				Annotations: getDefaultAnnotations(constants.AutoscalerClassNone),
			},
			Spec: v1beta1.InferenceServiceSpec{
				Predictor: v1beta1.PredictorSpec{
					ComponentExtensionSpec: v1beta1.ComponentExtensionSpec{
						MinReplicas: ptr.To(int32(1)),
					},
					Model: &v1beta1.ModelSpec{
						ModelFormat: v1beta1.ModelFormat{Name: "tensorflow"},
						PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
							RuntimeVersion: ptr.To("1.14.0"),
							Container: corev1.Container{
								Name:      constants.InferenceServiceContainerName,
								Resources: defaultResource,
							},
						},
					},
				},
			},
		}
		if modelDirMount {
			isvc.Spec.Predictor.Volumes = []corev1.Volume{{
				Name:         "model-vol",
				VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
			}}
			isvc.Spec.Predictor.Model.VolumeMounts = []corev1.VolumeMount{{
				Name:      "model-vol",
				MountPath: constants.ModelDir,
			}}
		}
		isvc.DefaultInferenceService(nil, nil, &v1beta1.SecurityConfig{AutoMountServiceAccountToken: false}, nil, nil)
		return isvc
	}

	// setup creates the config, a namespace and a serving runtime for one test, with
	// cleanup registered through DeferCleanup.
	setup := func(ctx context.Context, namespace string) {
		configMap := createInferenceServiceConfigMap(getRawKubeTestConfigs())
		Expect(k8sClient.Create(ctx, configMap)).To(Succeed())
		DeferCleanup(k8sClient.Delete, context.Background(), configMap)

		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		DeferCleanup(k8sClient.Delete, context.Background(), ns)

		servingRuntime := getServingRuntime("tf-serving-mms", namespace)
		Expect(k8sClient.Create(ctx, &servingRuntime)).To(Succeed())
		DeferCleanup(k8sClient.Delete, context.Background(), &servingRuntime)
	}

	predictorDeployment := func(ctx context.Context, isvc *v1beta1.InferenceService) *appsv1.Deployment {
		key := types.NamespacedName{Name: constants.PredictorServiceName(isvc.Name), Namespace: isvc.Namespace}
		deploy := &appsv1.Deployment{}
		Eventually(func() error { return k8sClient.Get(ctx, key, deploy) }, timeout, interval).Should(Succeed())
		return deploy
	}

	modelConfigKey := func(isvc *v1beta1.InferenceService) types.NamespacedName {
		return types.NamespacedName{Name: constants.ModelConfigName(isvc.Name, 0), Namespace: isvc.Namespace}
	}

	It("Should enable multi-model serving when the model container does not mount the model dir", func(ctx SpecContext) {
		setup(ctx, "mms-enabled")

		isvc := newISVC("mms-enabled", "mms-enabled", false)
		Expect(k8sClient.Create(ctx, isvc)).To(Succeed())
		DeferCleanup(k8sClient.Delete, context.Background(), isvc)

		deploy := predictorDeployment(ctx, isvc)
		Expect(deploy.Spec.Template.Annotations).To(HaveKeyWithValue(constants.AgentShouldInjectAnnotationKey, "true"))

		Eventually(func() error {
			return k8sClient.Get(ctx, modelConfigKey(isvc), &corev1.ConfigMap{})
		}, timeout, interval).Should(Succeed())
	})

	It("Should not enable multi-model serving when the model container already mounts the model dir", func(ctx SpecContext) {
		setup(ctx, "mms-model-dir-mount")

		isvc := newISVC("mms-model-dir-mount", "mms-model-dir-mount", true)
		Expect(k8sClient.Create(ctx, isvc)).To(Succeed())
		DeferCleanup(k8sClient.Delete, context.Background(), isvc)

		deploy := predictorDeployment(ctx, isvc)
		Expect(deploy.Spec.Template.Annotations).NotTo(HaveKey(constants.AgentShouldInjectAnnotationKey))

		container := findContainer(deploy.Spec.Template.Spec)
		Expect(container).NotTo(BeNil())
		Expect(container.VolumeMounts).To(ConsistOf(corev1.VolumeMount{Name: "model-vol", MountPath: constants.ModelDir}))

		Consistently(func() bool {
			err := k8sClient.Get(ctx, modelConfigKey(isvc), &corev1.ConfigMap{})
			return apierr.IsNotFound(err)
		}, fastTimeout, interval).Should(BeTrue())
	})
})

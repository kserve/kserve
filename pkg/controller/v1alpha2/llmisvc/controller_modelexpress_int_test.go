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

package llmisvc_test

import (
	"context"
	"slices"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/apis"
	"knative.dev/pkg/kmeta"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
	"github.com/kserve/kserve/pkg/credentials/s3"
)

const mxAddress = "https://modelexpress.mx.svc:8001"

func modelExpressService(name, ns, uri string, annotations map[string]string) *v1alpha2.LLMInferenceService {
	modelURL, err := apis.ParseURL(uri)
	Expect(err).ToNot(HaveOccurred())
	return &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Annotations: annotations},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Model:        v1alpha2.LLMModelSpec{Name: ptr.To("foo"), URI: *modelURL},
			WorkloadSpec: v1alpha2.WorkloadSpec{},
			Router: &v1alpha2.RouterSpec{
				Route:     &v1alpha2.GatewayRoutesSpec{},
				Gateway:   &v1alpha2.GatewaySpec{},
				Scheduler: &v1alpha2.SchedulerSpec{},
			},
			Prefill: &v1alpha2.WorkloadSpec{},
		},
	}
}

func getDeployment(ctx context.Context, name, ns string) *appsv1.Deployment {
	d := &appsv1.Deployment{}
	Eventually(func(ctx context.Context) error {
		return envTest.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, d)
	}).WithContext(ctx).Should(Succeed())
	return d
}

func mainContainer(d *appsv1.Deployment) *corev1.Container {
	i := slices.IndexFunc(d.Spec.Template.Spec.Containers, func(c corev1.Container) bool { return c.Name == "main" })
	Expect(i).To(BeNumerically(">=", 0), "main container in %s", d.Name)
	return &d.Spec.Template.Spec.Containers[i]
}

func envNamed(c *corev1.Container, name string) *corev1.EnvVar {
	i := slices.IndexFunc(c.Env, func(e corev1.EnvVar) bool { return e.Name == name })
	if i < 0 {
		return nil
	}
	return &c.Env[i]
}

func expectModelExpressCondition(ctx context.Context, llmSvc *v1alpha2.LLMInferenceService, status corev1.ConditionStatus, reason string) {
	Eventually(func(g Gomega, ctx context.Context) {
		current := &v1alpha2.LLMInferenceService{}
		g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
		cond := current.Status.GetCondition(v1alpha2.ModelExpressReady)
		g.Expect(cond).ToNot(BeNil())
		g.Expect(cond.Status).To(Equal(status))
		g.Expect(cond.Reason).To(Equal(reason))
	}).WithContext(ctx).Should(Succeed())
}

var _ = Describe("LLMInferenceService Controller - ModelExpress", func() {
	It("renders native s3:// engine pods that stream weights through ModelExpress", func(ctx SpecContext) {
		svcName := "test-mx-native-s3"
		testNs := NewTestNamespace(ctx, envTest)

		secretName := kmeta.ChildName(svcName, "-secret")
		Expect(envTest.Client.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: testNs.Name,
				Annotations: map[string]string{
					s3.InferenceServiceS3SecretEndpointAnnotation: "minio.minio.svc:9000",
					s3.InferenceServiceS3SecretHttpsAnnotation:    "0",
				},
			},
			StringData: map[string]string{s3.AWSAccessKeyId: "id", s3.AWSSecretAccessKey: "secret"},
		})).To(Succeed())
		Expect(retry.RetryOnConflict(retry.DefaultRetry, func() error {
			defaultSA := &corev1.ServiceAccount{}
			if err := envTest.Get(ctx, types.NamespacedName{Name: "default", Namespace: testNs.Name}, defaultSA); err != nil {
				return err
			}
			defaultSA.Secrets = []corev1.ObjectReference{{Name: secretName, Namespace: testNs.Name}}
			return envTest.Update(ctx, defaultSA)
		})).To(Succeed())

		llmSvc := modelExpressService(svcName, testNs.Name, "s3://models/llama", map[string]string{
			constants.ModelExpressModeAnnotationKey:          "native",
			constants.ModelExpressAddressAnnotationKey:       mxAddress,
			constants.ModelExpressTokenAudienceAnnotationKey: "modelexpress",
		})
		Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
		defer func() { testNs.DeleteAndWait(ctx, llmSvc) }()

		expectModelExpressCondition(ctx, llmSvc, corev1.ConditionTrue, "")

		serviceAccountName := kmeta.ChildName(svcName, "-kserve")
		Eventually(func(ctx context.Context) error {
			return envTest.Get(ctx, types.NamespacedName{Name: serviceAccountName, Namespace: testNs.Name}, &corev1.ServiceAccount{})
		}).WithContext(ctx).Should(Succeed(), "ModelExpress needs a predictable identity without a routing sidecar")

		for _, name := range []string{svcName + "-kserve", svcName + "-kserve-prefill"} {
			d := getDeployment(ctx, name, testNs.Name)
			podSpec := d.Spec.Template.Spec
			Expect(podSpec.ServiceAccountName).To(Equal(serviceAccountName), name)

			initIdx := slices.IndexFunc(podSpec.InitContainers, func(c corev1.Container) bool {
				return c.Name == constants.StorageInitializerContainerName
			})
			Expect(initIdx).To(BeNumerically(">=", 0), name)
			initContainer := &podSpec.InitContainers[initIdx]
			Expect(initContainer.Args).To(Equal([]string{"s3://models/llama", constants.DefaultModelLocalMountPath}), name)
			Expect(envNamed(initContainer, "STORAGE_IGNORE_PATTERNS")).ToNot(BeNil(), name)

			c := mainContainer(d)
			Expect(c.Args).To(ContainElements("--load-format", "modelexpress"), name)
			Expect(envNamed(c, "KSERVE_MODEL_ARGS")).To(BeNil(), "%s: s3:// serves /mnt/models", name)
			Expect(envNamed(c, "MX_MODEL_URI")).To(HaveField("Value", "s3://models/llama"), name)
			Expect(envNamed(c, "MX_SERVER_ADDRESS")).To(HaveField("Value", mxAddress), name)
			Expect(envNamed(c, "MODEL_EXPRESS_URL")).To(HaveField("Value", mxAddress), name)
			Expect(envNamed(c, "MX_MODEL_REVISION").Value).To(HavePrefix("uri-"), name)
			Expect(envNamed(c, s3.AWSAccessKeyId)).ToNot(BeNil(), name)
			Expect(envNamed(c, s3.AWSEndpointUrl)).To(HaveField("Value", "http://minio.minio.svc:9000"), name)
			Expect(envNamed(c, "RUNAI_STREAMER_S3_USE_VIRTUAL_ADDRESSING")).To(HaveField("Value", "0"), name)
			Expect(envNamed(c, "MX_AUTH_TOKEN_PATH")).To(HaveField("Value", "/var/run/secrets/modelexpress/token"), name)
			Expect(podSpec.Volumes).To(ContainElement(HaveField("Name", "modelexpress-token")), name)
		}
	})

	It("renders native hf:// engine pods that load through the ModelExpress server cache", func(ctx SpecContext) {
		svcName := "test-mx-native-hf"
		testNs := NewTestNamespace(ctx, envTest)

		llmSvc := modelExpressService(svcName, testNs.Name, "hf://meta-llama/Llama-3.3-70B-Instruct:main", map[string]string{
			constants.ModelExpressModeAnnotationKey:    "native",
			constants.ModelExpressAddressAnnotationKey: mxAddress,
		})
		Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
		defer func() { testNs.DeleteAndWait(ctx, llmSvc) }()

		expectModelExpressCondition(ctx, llmSvc, corev1.ConditionTrue, "")

		for _, name := range []string{svcName + "-kserve", svcName + "-kserve-prefill"} {
			d := getDeployment(ctx, name, testNs.Name)
			Expect(d.Spec.Template.Spec.InitContainers).ToNot(ContainElement(HaveField("Name", constants.StorageInitializerContainerName)),
				"%s: native hf:// downloads nothing up front", name)

			c := mainContainer(d)
			Expect(envNamed(c, "KSERVE_MODEL_ARGS")).To(HaveField("Value", "meta-llama/Llama-3.3-70B-Instruct --revision main"), name)
			Expect(envNamed(c, "MODEL_EXPRESS_NO_SHARED_STORAGE")).To(HaveField("Value", "1"), name)
			Expect(envNamed(c, "HF_HUB_OFFLINE")).To(HaveField("Value", "1"), name)
			hubCache := envNamed(c, "HF_HUB_CACHE")
			Expect(hubCache).ToNot(BeNil(), name)
			Expect(envNamed(c, "MODEL_EXPRESS_CACHE_DIRECTORY")).To(HaveField("Value", hubCache.Value), name)
			Expect(envNamed(c, "MX_AUTH_TOKEN_PATH")).To(BeNil(), "%s: no audience, no token", name)
		}
	})

	It("does not render a native workload it cannot point at a server", func(ctx SpecContext) {
		svcName := "test-mx-native-unresolved"
		testNs := NewTestNamespace(ctx, envTest)

		llmSvc := modelExpressService(svcName, testNs.Name, "s3://models/llama", map[string]string{
			constants.ModelExpressModeAnnotationKey: "native",
		})
		Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
		defer func() { testNs.DeleteAndWait(ctx, llmSvc) }()

		expectModelExpressCondition(ctx, llmSvc, corev1.ConditionFalse, "ServerNotResolved")
		Consistently(func(ctx context.Context) bool {
			err := envTest.Get(ctx, types.NamespacedName{Name: svcName + "-kserve", Namespace: testNs.Name}, &appsv1.Deployment{})
			return apierrors.IsNotFound(err)
		}).WithContext(ctx).WithTimeout(2 * time.Second).Should(BeTrue())
	})

	It("stops a native service even when its server cannot be resolved", func(ctx SpecContext) {
		svcName := "test-mx-native-stopped"
		testNs := NewTestNamespace(ctx, envTest)

		llmSvc := modelExpressService(svcName, testNs.Name, "s3://models/llama", map[string]string{
			constants.ModelExpressModeAnnotationKey: "native",
		})
		Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
		defer func() { testNs.DeleteAndWait(ctx, llmSvc) }()
		expectModelExpressCondition(ctx, llmSvc, corev1.ConditionFalse, "ServerNotResolved")

		Expect(retry.RetryOnConflict(retry.DefaultRetry, func() error {
			current := &v1alpha2.LLMInferenceService{}
			if err := envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current); err != nil {
				return err
			}
			current.Annotations[constants.StopAnnotationKey] = "true"
			return envTest.Update(ctx, current)
		})).To(Succeed())

		Eventually(func(g Gomega, ctx context.Context) {
			current := &v1alpha2.LLMInferenceService{}
			g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
			g.Expect(current.Status.GetCondition(v1alpha2.ModelExpressReady)).To(BeNil())
			main := current.Status.GetCondition(v1alpha2.MainWorkloadReady)
			g.Expect(main).ToNot(BeNil())
			g.Expect(main.Reason).To(Equal("Stopped"))
		}).WithContext(ctx).Should(Succeed())
	})

	It("renders a layered workload without ModelExpress when no server is configured", func(ctx SpecContext) {
		svcName := "test-mx-layered-unresolved"
		testNs := NewTestNamespace(ctx, envTest)

		llmSvc := modelExpressService(svcName, testNs.Name, "pvc://models/llama", map[string]string{
			constants.ModelExpressModeAnnotationKey: "layered",
		})
		Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
		defer func() { testNs.DeleteAndWait(ctx, llmSvc) }()

		expectModelExpressCondition(ctx, llmSvc, corev1.ConditionFalse, "ServerNotResolved")
		c := mainContainer(getDeployment(ctx, svcName+"-kserve", testNs.Name))
		Expect(c.Args).ToNot(ContainElement("--load-format"))
		Expect(envNamed(c, "MX_SERVER_ADDRESS")).To(BeNil())
	})

	It("leaves engine pods of services without ModelExpress free of ModelExpress configuration", func(ctx SpecContext) {
		svcName := "test-mx-disabled"
		testNs := NewTestNamespace(ctx, envTest)

		llmSvc := modelExpressService(svcName, testNs.Name, "hf://org/model", nil)
		Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
		defer func() { testNs.DeleteAndWait(ctx, llmSvc) }()

		for _, name := range []string{svcName + "-kserve", svcName + "-kserve-prefill"} {
			d := getDeployment(ctx, name, testNs.Name)
			c := mainContainer(d)
			Expect(envNamed(c, "KSERVE_MODEL_ARGS")).To(BeNil(), "%s: the empty slot is removed", name)
			Expect(envNamed(c, "MX_SERVER_ADDRESS")).To(BeNil(), name)
			Expect(c.Args).ToNot(ContainElement("--load-format"), name)
			Expect(d.Spec.Template.Spec.InitContainers).To(ContainElement(HaveField("Name", constants.StorageInitializerContainerName)), name)
		}
		current := &v1alpha2.LLMInferenceService{}
		Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
		Expect(current.Status.GetCondition(v1alpha2.ModelExpressReady)).To(BeNil())
	})
})

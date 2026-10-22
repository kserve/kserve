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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"knative.dev/pkg/kmeta"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"
	. "github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc/fixture"
	"github.com/kserve/kserve/pkg/credentials"
	. "github.com/kserve/kserve/pkg/testing"
)

var _ = Describe("LLMInferenceService Controller — Standalone Tokenizer", func() {
	Context("When standalone tokenizer should be deployed", func() {
		It("should create tokenizer Deployment and Service from well-known config", func(ctx SpecContext) {
			svcName := "test-llm-tokenizer-explicit"
			testNs := NewTestNamespace(ctx, envTest)

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithManagedTokenizer(),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			tokenizerName := kmeta.ChildName(svcName, "-tokenizer")

			// Verify tokenizer Deployment is created with correct properties
			Eventually(func(g Gomega, ctx context.Context) {
				dep := &appsv1.Deployment{}
				g.Expect(envTest.Get(ctx, types.NamespacedName{
					Name:      tokenizerName,
					Namespace: testNs.Name,
				}, dep)).To(Succeed())

				g.Expect(dep).To(BeOwnedBy(llmSvc))

				// Well-known config should provide the main container
				containerNames := containerNameList(dep)
				g.Expect(containerNames).To(ContainElement("main"))
			}).WithContext(ctx).Should(Succeed())

			// Verify tokenizer Service is created with correct port
			Eventually(func(g Gomega, ctx context.Context) {
				svc := &corev1.Service{}
				g.Expect(envTest.Get(ctx, types.NamespacedName{
					Name:      tokenizerName,
					Namespace: testNs.Name,
				}, svc)).To(Succeed())

				g.Expect(svc.Spec.Ports).To(HaveLen(1))
				g.Expect(svc.Spec.Ports[0].Name).To(Equal("render-http"))
				g.Expect(svc.Spec.Ports[0].Port).To(Equal(int32(8000)))
				g.Expect(svc.Spec.Selector).To(Equal(llmisvc.TokenizerLabels(llmSvc)))
			}).WithContext(ctx).Should(Succeed())
		})

		It("should run tokenizer pod with the main workload ServiceAccount when the tokenizer sets none", func(ctx SpecContext) {
			svcName := "test-llm-tokenizer-sa-cascade"
			testNs := NewTestNamespace(ctx, envTest)

			mainSAName := svcName + "-main-sa"
			roleARN := "arn:aws:iam::123456789012:role/kserve-s3"
			mainSA := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      mainSAName,
					Namespace: testNs.Name,
					Annotations: map[string]string{
						credentials.AwsIrsaAnnotationKey: roleARN,
					},
				},
			}
			Expect(envTest.Create(ctx, mainSA)).To(Succeed())

			// The main workload template sets the ServiceAccount; the scheduler
			// and tokenizer templates set none. The tokenizer must not fall
			// back to the generated scheduler (EPP) ServiceAccount: its token
			// subject differs from the main workload subject, so a trust
			// policy scoped to the main ServiceAccount would reject the STS
			// token exchange.
			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithManagedTokenizer(),
				WithTemplate(&corev1.PodSpec{
					ServiceAccountName: mainSAName,
				}),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			tokenizerName := kmeta.ChildName(svcName, "-tokenizer")

			Eventually(func(g Gomega, ctx context.Context) {
				dep := &appsv1.Deployment{}
				g.Expect(envTest.Get(ctx, types.NamespacedName{
					Name:      tokenizerName,
					Namespace: testNs.Name,
				}, dep)).To(Succeed())
				g.Expect(dep.Spec.Template.Spec.ServiceAccountName).To(Equal(mainSAName),
					"tokenizer pod must run with the main workload ServiceAccount when the tokenizer sets none")
			}).WithContext(ctx).Should(Succeed())
		})

		It("should keep the tokenizer template ServiceAccount when explicitly set", func(ctx SpecContext) {
			svcName := "test-llm-tokenizer-explicit-sa"
			testNs := NewTestNamespace(ctx, envTest)

			tokenizerSAName := svcName + "-tokenizer-sa"
			tokenizerSA := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tokenizerSAName,
					Namespace: testNs.Name,
				},
			}
			Expect(envTest.Create(ctx, tokenizerSA)).To(Succeed())

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithManagedTokenizer(),
			)
			llmSvc.Spec.Router.Scheduler.Tokenizer.Template = &corev1.PodSpec{
				ServiceAccountName: tokenizerSAName,
				Containers:         []corev1.Container{{Name: "main"}},
			}

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			tokenizerName := kmeta.ChildName(svcName, "-tokenizer")

			// The explicitly configured tokenizer ServiceAccount must win over
			// every fallback, and the credential lookup must use the same
			// ServiceAccount the pod runs with.
			Eventually(func(g Gomega, ctx context.Context) {
				dep := &appsv1.Deployment{}
				g.Expect(envTest.Get(ctx, types.NamespacedName{
					Name:      tokenizerName,
					Namespace: testNs.Name,
				}, dep)).To(Succeed())
				g.Expect(dep.Spec.Template.Spec.ServiceAccountName).To(Equal(tokenizerSAName),
					"tokenizer pod must keep the explicitly configured ServiceAccount")
			}).WithContext(ctx).Should(Succeed())
		})

		It("should run tokenizer pod with the main worker ServiceAccount in a worker-only configuration", func(ctx SpecContext) {
			svcName := "test-llm-tokenizer-worker-sa"
			testNs := NewTestNamespace(ctx, envTest)

			workerSAName := svcName + "-worker-sa"
			workerSA := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      workerSAName,
					Namespace: testNs.Name,
					Annotations: map[string]string{
						credentials.AwsIrsaAnnotationKey: "arn:aws:iam::123456789012:role/kserve-s3",
					},
				},
			}
			Expect(envTest.Create(ctx, workerSA)).To(Succeed())

			// Worker-only configuration: the main workload sets its
			// ServiceAccount through spec.worker; the leader template sets
			// none. The tokenizer resolution must use the same precedence as
			// the main workload.
			workerSpec := SimpleWorkerPodSpec()
			workerSpec.ServiceAccountName = workerSAName
			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithManagedTokenizer(),
				WithParallelism(ParallelismSpec(WithDataParallelism(2), WithDataLocalParallelism(1))),
				WithTemplate(SimpleWorkerPodSpec()),
				WithWorker(workerSpec),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			tokenizerName := kmeta.ChildName(svcName, "-tokenizer")

			Eventually(func(g Gomega, ctx context.Context) {
				dep := &appsv1.Deployment{}
				g.Expect(envTest.Get(ctx, types.NamespacedName{
					Name:      tokenizerName,
					Namespace: testNs.Name,
				}, dep)).To(Succeed())
				g.Expect(dep.Spec.Template.Spec.ServiceAccountName).To(Equal(workerSAName),
					"tokenizer pod must run with the main worker ServiceAccount in a worker-only configuration")
			}).WithContext(ctx).Should(Succeed())
		})

		It("should deploy tokenizer when token-producer plugin is in inline config", func(ctx SpecContext) {
			svcName := "test-llm-tok-3plugin"
			testNs := NewTestNamespace(ctx, envTest)

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithSchedulerConfigInline(`
apiVersion: inference.networking.x-k8s.io/v1alpha1
kind: EndpointPickerConfig
plugins:
- type: single-profile-handler
- type: token-producer
- type: precise-prefix-cache-producer
  parameters:
    tokenProcessorConfig:
      blockSize: 64
    kvEventsConfig:
      topicFilter: "kv@"
      discoverPods: true
      podDiscoveryConfig:
        socketPort: 5557
    indexerConfig:
      kvBlockIndexConfig:
        enableMetrics: true
- type: prefix-cache-scorer
  parameters:
    prefixMatchInfoProducerName: precise-prefix-cache-producer
- type: kv-cache-utilization-scorer
- type: queue-scorer
- type: max-score-picker
schedulingProfiles:
- name: default
  plugins:
  - pluginRef: prefix-cache-scorer
    weight: 3
  - pluginRef: kv-cache-utilization-scorer
    weight: 2
  - pluginRef: queue-scorer
    weight: 2
  - pluginRef: max-score-picker
`),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			tokenizerName := kmeta.ChildName(svcName, "-tokenizer")

			// Verify tokenizer Deployment is auto-deployed from plugin detection
			Eventually(func(g Gomega, ctx context.Context) {
				dep := &appsv1.Deployment{}
				g.Expect(envTest.Get(ctx, types.NamespacedName{
					Name:      tokenizerName,
					Namespace: testNs.Name,
				}, dep)).To(Succeed())

				g.Expect(dep).To(BeOwnedBy(llmSvc))
				containerNames := containerNameList(dep)
				g.Expect(containerNames).To(ContainElement("main"))
			}).WithContext(ctx).Should(Succeed())

			// Verify token-producer gets wired with tokenizer URL
			expectedDeployment := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) error {
				if err := envTest.Get(ctx, types.NamespacedName{
					Name:      kmeta.ChildName(svcName, "-kserve-router-scheduler"),
					Namespace: testNs.Name,
				}, expectedDeployment); err != nil {
					return err
				}

				configText, found := getSchedulerConfigText(expectedDeployment)
				g.Expect(found).To(BeTrue(), "Expected --config-text in scheduler deployment")
				g.Expect(configText).To(ContainSubstring("token-producer"))
				g.Expect(configText).To(ContainSubstring(kmeta.ChildName(svcName, "-tokenizer")))
				return nil
			}).WithContext(ctx).Should(Succeed())
		})

		It("should report TokenizerReady status condition", func(ctx SpecContext) {
			svcName := "test-llm-tok-status"
			testNs := NewTestNamespace(ctx, envTest)

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithManagedTokenizer(),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// Wait for tokenizer deployment to exist, then simulate readiness
			ensureTokenizerDeploymentReady(ctx, envTest.Client, llmSvc)
			ensureRouterManagedResourcesAreReady(ctx, envTest.Client, llmSvc)

			Eventually(LLMInferenceServiceIsReady(llmSvc, func(g Gomega, current *v1alpha2.LLMInferenceService) {
				g.Expect(current.Status).To(HaveCondition(string(v1alpha2.TokenizerReady), "True"))
			})).WithContext(ctx).Should(Succeed())
		})

		It("should inherit image pull secrets from the main workload template", func(ctx SpecContext) {
			svcName := "test-llm-tok-pull-secrets"
			testNs := NewTestNamespace(ctx, envTest)
			pullSecrets := []corev1.LocalObjectReference{{Name: "registry-pull"}}

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithTemplate(&corev1.PodSpec{ImagePullSecrets: pullSecrets}),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithManagedTokenizer(),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			Eventually(func(g Gomega, ctx context.Context) {
				dep := &appsv1.Deployment{}
				g.Expect(envTest.Get(ctx, types.NamespacedName{
					Name:      kmeta.ChildName(svcName, "-tokenizer"),
					Namespace: testNs.Name,
				}, dep)).To(Succeed())

				g.Expect(dep.Spec.Template.Spec.ImagePullSecrets).To(Equal(pullSecrets))
				g.Expect(dep.Spec.Template.Spec.ServiceAccountName).To(BeEmpty())
			}).WithContext(ctx).Should(Succeed())

			current := &v1alpha2.LLMInferenceService{}
			Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
			Expect(current.Spec.Router.Scheduler.Tokenizer.Template).To(BeNil())
		})

		It("should update the tokenizer Deployment when its ServiceAccount image pull secrets change", func(ctx SpecContext) {
			svcName := "test-llm-tok-sa-pull-secrets"
			testNs := NewTestNamespace(ctx, envTest,
				WithDefaultServiceAccountImagePullSecrets(corev1.LocalObjectReference{Name: "default-pull"}),
			)

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithTemplate(&corev1.PodSpec{ImagePullSecrets: []corev1.LocalObjectReference{{Name: "registry-pull"}}}),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithManagedTokenizer(),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			tokenizerKey := types.NamespacedName{
				Name:      kmeta.ChildName(svcName, "-tokenizer"),
				Namespace: testNs.Name,
			}
			dep := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) {
				g.Expect(envTest.Get(ctx, tokenizerKey, dep)).To(Succeed())
				g.Expect(dep.Spec.Template.Spec.ImagePullSecrets).To(Equal([]corev1.LocalObjectReference{
					{Name: "registry-pull"}, {Name: "default-pull"},
				}))
			}).WithContext(ctx).Should(Succeed())

			// Waits for the Deployment to stop changing so the ServiceAccount update is the only reconcile trigger.
			Eventually(func(g Gomega, ctx context.Context) {
				resourceVersion := dep.ResourceVersion
				g.Consistently(func(g Gomega, ctx context.Context) {
					g.Expect(envTest.Get(ctx, tokenizerKey, dep)).To(Succeed())
					g.Expect(dep.ResourceVersion).To(Equal(resourceVersion))
				}).WithContext(ctx).WithTimeout(5 * time.Second).Should(Succeed())
			}).WithContext(ctx).WithTimeout(time.Minute).Should(Succeed())

			Expect(retry.RetryOnConflict(retry.DefaultRetry, func() error {
				sa := &corev1.ServiceAccount{}
				if err := envTest.Get(ctx, types.NamespacedName{Name: "default", Namespace: testNs.Name}, sa); err != nil {
					return err
				}
				sa.ImagePullSecrets = []corev1.LocalObjectReference{{Name: "rotated-pull"}}
				return envTest.Update(ctx, sa)
			})).To(Succeed())

			Eventually(func(g Gomega, ctx context.Context) {
				dep := &appsv1.Deployment{}
				g.Expect(envTest.Get(ctx, tokenizerKey, dep)).To(Succeed())
				g.Expect(dep.Spec.Template.Spec.ImagePullSecrets).To(Equal([]corev1.LocalObjectReference{
					{Name: "registry-pull"}, {Name: "rotated-pull"},
				}))
			}).WithContext(ctx).Should(Succeed())
		})

		It("should keep the tokenizer Deployment unchanged when no template sets image pull secrets", func(ctx SpecContext) {
			svcName := "test-llm-tok-no-pull-secrets"
			testNs := NewTestNamespace(ctx, envTest,
				WithDefaultServiceAccountImagePullSecrets(corev1.LocalObjectReference{Name: "default-pull"}),
			)

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithManagedTokenizer(),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			tokenizerKey := types.NamespacedName{
				Name:      kmeta.ChildName(svcName, "-tokenizer"),
				Namespace: testNs.Name,
			}
			dep := &appsv1.Deployment{}
			Eventually(func(g Gomega, ctx context.Context) {
				g.Expect(envTest.Get(ctx, tokenizerKey, dep)).To(Succeed())
				g.Expect(dep.Spec.Template.Spec.ImagePullSecrets).To(BeEmpty())
			}).WithContext(ctx).Should(Succeed())
			generation := dep.Generation

			Expect(retry.RetryOnConflict(retry.DefaultRetry, func() error {
				current := &v1alpha2.LLMInferenceService{}
				if err := envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current); err != nil {
					return err
				}
				if current.Annotations == nil {
					current.Annotations = map[string]string{}
				}
				current.Annotations["test.kserve.io/reconcile"] = "1"
				return envTest.Update(ctx, current)
			})).To(Succeed())

			Consistently(func(g Gomega, ctx context.Context) {
				current := &appsv1.Deployment{}
				g.Expect(envTest.Get(ctx, tokenizerKey, current)).To(Succeed())
				g.Expect(current.Generation).To(Equal(generation))
				g.Expect(current.Spec.Template.Spec.ImagePullSecrets).To(BeEmpty())
			}).WithContext(ctx).WithTimeout(3 * time.Second).Should(Succeed())
		})
	})

	Context("When tokenizer is not configured", func() {
		It("should not create tokenizer resources", func(ctx SpecContext) {
			svcName := "test-llm-no-tokenizer"
			testNs := NewTestNamespace(ctx, envTest)

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// Wait for the scheduler to be created (proves reconciler ran)
			Eventually(func(g Gomega, ctx context.Context) {
				dep := &appsv1.Deployment{}
				g.Expect(envTest.Get(ctx, types.NamespacedName{
					Name:      kmeta.ChildName(svcName, "-kserve-router-scheduler"),
					Namespace: testNs.Name,
				}, dep)).To(Succeed())
			}).WithContext(ctx).Should(Succeed())

			// Tokenizer Deployment should not exist
			tokenizerName := kmeta.ChildName(svcName, "-tokenizer")
			Consistently(func(g Gomega, ctx context.Context) {
				dep := &appsv1.Deployment{}
				err := envTest.Get(ctx, types.NamespacedName{
					Name:      tokenizerName,
					Namespace: testNs.Name,
				}, dep)
				g.Expect(errors.IsNotFound(err)).To(BeTrue(), "tokenizer Deployment should not exist")
			}).WithContext(ctx).Should(Succeed())
		})

		// Uses oci:// because, unlike hf://, it requires a "main" container to
		// already exist in the pod spec -- the case that must not break the scheduler.
		It("should reconcile the scheduler successfully for an OCI modelcar model URI", func(ctx SpecContext) {
			svcName := "test-llm-no-tokenizer-oci"
			testNs := NewTestNamespace(ctx, envTest)

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("oci://registry.io/user-id/repo-id:tag"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			// Scheduler Deployment must still get created.
			Eventually(func(g Gomega, ctx context.Context) {
				dep := &appsv1.Deployment{}
				g.Expect(envTest.Get(ctx, types.NamespacedName{
					Name:      kmeta.ChildName(svcName, "-kserve-router-scheduler"),
					Namespace: testNs.Name,
				}, dep)).To(Succeed())
			}).WithContext(ctx).Should(Succeed())

			// Tokenizer Deployment should not exist
			tokenizerName := kmeta.ChildName(svcName, "-tokenizer")
			Consistently(func(g Gomega, ctx context.Context) {
				dep := &appsv1.Deployment{}
				err := envTest.Get(ctx, types.NamespacedName{
					Name:      tokenizerName,
					Namespace: testNs.Name,
				}, dep)
				g.Expect(errors.IsNotFound(err)).To(BeTrue(), "tokenizer Deployment should not exist")
			}).WithContext(ctx).Should(Succeed())

			// Must never fail with reason SchedulerReconcileError (may still be False
			// for other reasons, e.g. not yet Available).
			Consistently(func(g Gomega, ctx context.Context) {
				updated := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, types.NamespacedName{
					Name:      svcName,
					Namespace: testNs.Name,
				}, updated)).To(Succeed())

				cond := updated.Status.GetCondition(v1alpha2.SchedulerWorkloadReady)
				if cond != nil {
					g.Expect(cond.Reason).ToNot(Equal("SchedulerReconcileError"),
						"scheduler reconcile must not fail when no tokenizer is needed: %+v", cond)
				}
			}).WithContext(ctx).Should(Succeed())
		})
	})

	Context("Legacy migration — precise-prefix-cache-scorer triggers tokenizer", func() {
		It("should create tokenizer Deployment when precise-prefix-cache-scorer is in inline config", func(ctx SpecContext) {
			svcName := "test-llm-tok-legacy"
			testNs := NewTestNamespace(ctx, envTest)

			legacyConfig := `
apiVersion: inference.networking.x-k8s.io/v1alpha1
kind: EndpointPickerConfig
plugins:
- type: single-profile-handler
- type: precise-prefix-cache-scorer
  parameters:
    indexerConfig:
      tokenProcessorConfig:
        blockSize: 16
- type: queue-scorer
- type: max-score-picker
schedulingProfiles:
- name: default
  plugins:
  - pluginRef: queue-scorer
    weight: 2
  - pluginRef: precise-prefix-cache-scorer
    weight: 3
  - pluginRef: max-score-picker
`
			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithSchedulerConfigInline(legacyConfig),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			tokenizerName := kmeta.ChildName(svcName, "-tokenizer")

			// Tokenizer Deployment should be auto-provisioned
			Eventually(func(g Gomega, ctx context.Context) {
				dep := &appsv1.Deployment{}
				g.Expect(envTest.Get(ctx, types.NamespacedName{
					Name:      tokenizerName,
					Namespace: testNs.Name,
				}, dep)).To(Succeed())

				g.Expect(dep).To(BeOwnedBy(llmSvc))
				containerNames := containerNameList(dep)
				g.Expect(containerNames).To(ContainElement("main"))
			}).WithContext(ctx).Should(Succeed())

			// Tokenizer Service should also exist
			Eventually(func(g Gomega, ctx context.Context) {
				svc := &corev1.Service{}
				g.Expect(envTest.Get(ctx, types.NamespacedName{
					Name:      tokenizerName,
					Namespace: testNs.Name,
				}, svc)).To(Succeed())
				g.Expect(svc.Spec.Ports[0].Name).To(Equal("render-http"))
			}).WithContext(ctx).Should(Succeed())
		})
	})

	Context("Tokenizer cleanup on spec change", func() {
		It("should delete tokenizer resources when tokenizer field is removed from spec", func(ctx SpecContext) {
			svcName := "test-llm-tok-cleanup"
			testNs := NewTestNamespace(ctx, envTest)

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithManagedTokenizer(),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			tokenizerName := kmeta.ChildName(svcName, "-tokenizer")

			// Wait for tokenizer Deployment to exist and report TokenizerReady
			Eventually(func(g Gomega, ctx context.Context) {
				dep := &appsv1.Deployment{}
				g.Expect(envTest.Get(ctx, types.NamespacedName{
					Name:      tokenizerName,
					Namespace: testNs.Name,
				}, dep)).To(Succeed())
			}).WithContext(ctx).Should(Succeed())
			ensureTokenizerDeploymentReady(ctx, envTest.Client, llmSvc)
			Eventually(func(g Gomega, ctx context.Context) {
				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
				g.Expect(current.Status).To(HaveCondition(string(v1alpha2.TokenizerReady), "True"))
			}).WithContext(ctx).Should(Succeed())

			// Remove the tokenizer from the spec
			Expect(retry.RetryOnConflict(retry.DefaultRetry, func() error {
				current := &v1alpha2.LLMInferenceService{}
				Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
				current.Spec.Router.Scheduler.Tokenizer = nil
				return envTest.Update(ctx, current)
			})).To(Succeed())

			// Tokenizer Deployment should be deleted
			Eventually(func(g Gomega, ctx context.Context) {
				dep := &appsv1.Deployment{}
				err := envTest.Get(ctx, types.NamespacedName{
					Name:      tokenizerName,
					Namespace: testNs.Name,
				}, dep)
				g.Expect(errors.IsNotFound(err)).To(BeTrue(), "tokenizer Deployment should be deleted")
			}).WithContext(ctx).Should(Succeed())

			// Tokenizer Service should also be deleted
			Eventually(func(g Gomega, ctx context.Context) {
				svc := &corev1.Service{}
				err := envTest.Get(ctx, types.NamespacedName{
					Name:      tokenizerName,
					Namespace: testNs.Name,
				}, svc)
				g.Expect(errors.IsNotFound(err)).To(BeTrue(), "tokenizer Service should be deleted")
			}).WithContext(ctx).Should(Succeed())

			// TokenizerReady must be cleared so a stale False cannot block RouterReady
			Eventually(func(g Gomega, ctx context.Context) {
				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
				g.Expect(current.Status.GetCondition(v1alpha2.TokenizerReady)).To(BeNil(),
					"TokenizerReady should be cleared after tokenizer is disabled")
			}).WithContext(ctx).Should(Succeed())
		})

		It("should delete tokenizer resources when token-producer plugin is removed from inline config", func(ctx SpecContext) {
			svcName := "test-llm-tok-cleanup-plugin"
			testNs := NewTestNamespace(ctx, envTest)

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithSchedulerConfigInline(`
apiVersion: inference.networking.x-k8s.io/v1alpha1
kind: EndpointPickerConfig
plugins:
- type: single-profile-handler
- type: token-producer
- type: precise-prefix-cache-producer
- type: prefix-cache-scorer
  parameters:
    prefixMatchInfoProducerName: precise-prefix-cache-producer
- type: queue-scorer
- type: max-score-picker
schedulingProfiles:
- name: default
  plugins:
  - pluginRef: prefix-cache-scorer
    weight: 3
  - pluginRef: queue-scorer
    weight: 2
  - pluginRef: max-score-picker
`),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			tokenizerName := kmeta.ChildName(svcName, "-tokenizer")

			// Wait for tokenizer Deployment to exist (triggered by token-producer in config)
			// and report TokenizerReady
			Eventually(func(g Gomega, ctx context.Context) {
				dep := &appsv1.Deployment{}
				g.Expect(envTest.Get(ctx, types.NamespacedName{
					Name:      tokenizerName,
					Namespace: testNs.Name,
				}, dep)).To(Succeed())
			}).WithContext(ctx).Should(Succeed())
			ensureTokenizerDeploymentReady(ctx, envTest.Client, llmSvc)
			Eventually(func(g Gomega, ctx context.Context) {
				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
				g.Expect(current.Status).To(HaveCondition(string(v1alpha2.TokenizerReady), "True"))
			}).WithContext(ctx).Should(Succeed())

			// Remove token-producer from inline config (switch to simple config)
			Expect(retry.RetryOnConflict(retry.DefaultRetry, func() error {
				current := &v1alpha2.LLMInferenceService{}
				Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
				current.Spec.Router.Scheduler.Config = nil
				return envTest.Update(ctx, current)
			})).To(Succeed())

			// Tokenizer Deployment should be deleted
			Eventually(func(g Gomega, ctx context.Context) {
				dep := &appsv1.Deployment{}
				err := envTest.Get(ctx, types.NamespacedName{
					Name:      tokenizerName,
					Namespace: testNs.Name,
				}, dep)
				g.Expect(errors.IsNotFound(err)).To(BeTrue(), "tokenizer Deployment should be deleted after removing token-producer plugin")
			}).WithContext(ctx).Should(Succeed())

			// Tokenizer Service should also be deleted
			Eventually(func(g Gomega, ctx context.Context) {
				svc := &corev1.Service{}
				err := envTest.Get(ctx, types.NamespacedName{
					Name:      tokenizerName,
					Namespace: testNs.Name,
				}, svc)
				g.Expect(errors.IsNotFound(err)).To(BeTrue(), "tokenizer Service should be deleted after removing token-producer plugin")
			}).WithContext(ctx).Should(Succeed())

			// TokenizerReady must be cleared so a stale False cannot block RouterReady
			Eventually(func(g Gomega, ctx context.Context) {
				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
				g.Expect(current.Status.GetCondition(v1alpha2.TokenizerReady)).To(BeNil(),
					"TokenizerReady should be cleared after token-producer plugin is removed")
			}).WithContext(ctx).Should(Succeed())
		})
	})

	Context("ForceStop handling", func() {
		It("should set TokenizerReady to False with reason Stopped when force-stop is applied", func(ctx SpecContext) {
			svcName := "test-llm-tok-forcestop"
			testNs := NewTestNamespace(ctx, envTest)

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithManagedScheduler(),
				WithManagedTokenizer(),
			)

			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
			}()

			tokenizerName := kmeta.ChildName(svcName, "-tokenizer")

			// Wait for tokenizer to be deployed and ready
			Eventually(func(g Gomega, ctx context.Context) {
				dep := &appsv1.Deployment{}
				g.Expect(envTest.Get(ctx, types.NamespacedName{
					Name:      tokenizerName,
					Namespace: testNs.Name,
				}, dep)).To(Succeed())
			}).WithContext(ctx).Should(Succeed())
			ensureTokenizerDeploymentReady(ctx, envTest.Client, llmSvc)
			Eventually(func(g Gomega, ctx context.Context) {
				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
				g.Expect(current.Status).To(HaveCondition(string(v1alpha2.TokenizerReady), "True"))
			}).WithContext(ctx).Should(Succeed())

			// Apply force-stop annotation
			Expect(retry.RetryOnConflict(retry.DefaultRetry, func() error {
				current := &v1alpha2.LLMInferenceService{}
				if err := envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current); err != nil {
					return err
				}
				if current.Annotations == nil {
					current.Annotations = make(map[string]string)
				}
				current.Annotations[constants.StopAnnotationKey] = "true"
				return envTest.Update(ctx, current)
			})).To(Succeed())

			// TokenizerReady should be False with reason "Stopped"
			Eventually(func(g Gomega, ctx context.Context) {
				current := &v1alpha2.LLMInferenceService{}
				g.Expect(envTest.Get(ctx, client.ObjectKeyFromObject(llmSvc), current)).To(Succeed())
				cond := current.Status.GetCondition(v1alpha2.TokenizerReady)
				g.Expect(cond).ToNot(BeNil(), "TokenizerReady condition should exist when force-stopped")
				g.Expect(string(cond.Status)).To(Equal("False"))
				g.Expect(cond.Reason).To(Equal("Stopped"))
			}).WithContext(ctx).Should(Succeed())

			// Tokenizer Deployment should be deleted
			Eventually(func(g Gomega, ctx context.Context) {
				dep := &appsv1.Deployment{}
				err := envTest.Get(ctx, types.NamespacedName{
					Name:      tokenizerName,
					Namespace: testNs.Name,
				}, dep)
				g.Expect(errors.IsNotFound(err)).To(BeTrue(), "tokenizer Deployment should be deleted when force-stopped")
			}).WithContext(ctx).Should(Succeed())
		})
	})

	Context("External InferencePool ref", func() {
		It("should not create tokenizer when scheduler has external pool ref", func(ctx SpecContext) {
			svcName := "test-llm-tok-extpool"
			testNs := NewTestNamespace(ctx, envTest)
			infPool := InferencePool("external-pool",
				WithSelector("app", "workload"),
				WithEndpointPickerRef("", "Service", "external-epp", 9002),
			)
			infPool.Namespace = testNs.Name

			llmSvc := LLMInferenceService(svcName,
				InNamespace[*v1alpha2.LLMInferenceService](testNs.Name),
				WithModelURI("hf://facebook/opt-125m"),
				WithManagedRoute(),
				WithManagedGateway(),
				WithInferencePoolRef("external-pool"),
				WithManagedTokenizer(),
			)

			Expect(envTest.Create(ctx, infPool)).To(Succeed())
			Expect(envTest.Create(ctx, llmSvc)).To(Succeed())
			defer func() {
				testNs.DeleteAndWait(ctx, llmSvc)
				testNs.DeleteAndWait(ctx, infPool)
			}()

			// Wait for reconciliation to process (workload should exist)
			Eventually(func(g Gomega, ctx context.Context) {
				dep := &appsv1.Deployment{}
				g.Expect(envTest.Get(ctx, types.NamespacedName{
					Name:      kmeta.ChildName(svcName, "-kserve"),
					Namespace: testNs.Name,
				}, dep)).To(Succeed())
			}).WithContext(ctx).Should(Succeed())

			// Tokenizer should not exist with external pool ref
			tokenizerName := kmeta.ChildName(svcName, "-tokenizer")
			Consistently(func(g Gomega, ctx context.Context) {
				dep := &appsv1.Deployment{}
				err := envTest.Get(ctx, types.NamespacedName{
					Name:      tokenizerName,
					Namespace: testNs.Name,
				}, dep)
				g.Expect(errors.IsNotFound(err)).To(BeTrue(), "tokenizer Deployment should not exist with external pool ref")
			}).WithContext(ctx).Should(Succeed())
		})
	})
})

func containerNameList(dep *appsv1.Deployment) []string {
	names := make([]string, len(dep.Spec.Template.Spec.Containers))
	for i, c := range dep.Spec.Template.Spec.Containers {
		names[i] = c.Name
	}
	return names
}

func ensureTokenizerDeploymentReady(ctx context.Context, c client.Client, llmSvc *v1alpha2.LLMInferenceService) {
	if envTest.UsingExistingCluster() {
		return
	}

	Eventually(func(g Gomega, ctx context.Context) {
		deployments := &appsv1.DeploymentList{}
		g.Expect(c.List(ctx, deployments, &client.ListOptions{
			Namespace:     llmSvc.Namespace,
			LabelSelector: labels.SelectorFromSet(llmisvc.TokenizerLabels(llmSvc)),
		})).To(Succeed())
		g.Expect(deployments.Items).NotTo(BeEmpty())

		for _, d := range deployments.Items {
			dep := d.DeepCopy()
			dep.Status.Replicas = 1
			dep.Status.ReadyReplicas = 1
			dep.Status.AvailableReplicas = 1
			dep.Status.Conditions = []appsv1.DeploymentCondition{
				{
					Type:   appsv1.DeploymentAvailable,
					Status: corev1.ConditionTrue,
				},
			}
			g.Expect(c.Status().Update(ctx, dep)).To(Succeed())
		}
	}).WithContext(ctx).Should(Succeed())
}

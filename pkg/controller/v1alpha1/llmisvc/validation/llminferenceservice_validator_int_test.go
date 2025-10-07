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

package validation_test

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/controller/v1alpha1/llmisvc/fixture"
)

var _ = Describe("LLMInferenceService webhook validation", func() {
	var (
		ns        *corev1.Namespace
		nsName    string
		gateway   *gwapiv1.Gateway
		httpRoute *gwapiv1.HTTPRoute
	)

	BeforeEach(func(ctx SpecContext) {
		nsName = fmt.Sprintf("test-llmisvc-validation-%d", time.Now().UnixNano())

		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: nsName,
			},
		}
		Expect(envTest.Client.Create(ctx, ns)).To(Succeed())

		gateway = fixture.Gateway("test-gateway",
			fixture.InNamespace[*gwapiv1.Gateway](nsName),
			fixture.WithClassName("test-gateway-class"),
			fixture.WithListener(gwapiv1.HTTPProtocolType),
		)
		Expect(envTest.Client.Create(ctx, gateway)).To(Succeed())

		httpRoute = fixture.HTTPRoute("test-route",
			fixture.InNamespace[*gwapiv1.HTTPRoute](nsName),
			fixture.WithParentRef(fixture.GatewayRef("test-gateway")),
			fixture.WithPath("/test"),
		)
		Expect(envTest.Client.Create(ctx, httpRoute)).To(Succeed())

		DeferCleanup(func() {
			httpRoute := httpRoute
			gateway := gateway
			ns := ns
			envTest.DeleteAll(httpRoute, gateway, ns)
		})
	})

	Context("cross-field constraints validation", func() {
		It("should reject LLMInferenceService with both refs and spec in HTTPRoute", func(ctx SpecContext) {
			// given
			llmSvc := fixture.LLMInferenceService("test-both-refs-and-spec",
				fixture.InNamespace[*v1alpha1.LLMInferenceService](nsName),
				fixture.WithModelURI("hf://facebook/opt-125m"),
				fixture.WithHTTPRouteRefs(fixture.HTTPRouteRef("test-route")),
				fixture.WithHTTPRouteSpec(&fixture.HTTPRoute("test-route",
					fixture.WithHTTPRule(
						fixture.Matches(fixture.PathPrefixMatch("/test")),
						fixture.BackendRefs(fixture.ServiceRef("test-service", 80, 1)),
					),
				).Spec),
			)

			// when
			errValidation := envTest.Client.Create(ctx, llmSvc)

			// then
			Expect(errValidation).To(HaveOccurred())
			Expect(errValidation.Error()).To(ContainSubstring("unsupported configuration"))
		})

		It("should reject LLMInferenceService with user-defined routes and managed gateway", func(ctx SpecContext) {
			// given
			llmSvc := fixture.LLMInferenceService("test-refs-with-managed-gateway",
				fixture.InNamespace[*v1alpha1.LLMInferenceService](nsName),
				fixture.WithModelURI("hf://facebook/opt-125m"),
				fixture.WithHTTPRouteRefs(fixture.HTTPRouteRef("test-route")),
				fixture.WithManagedGateway(),
			)

			// when
			errValidation := envTest.Client.Create(ctx, llmSvc)

			// then
			Expect(errValidation).To(HaveOccurred())
			Expect(errValidation.Error()).To(ContainSubstring("cannot be used with a managed gateway"))
		})

		It("should reject LLMInferenceService with managed route and user-defined gateway refs", func(ctx SpecContext) {
			// given
			llmSvc := fixture.LLMInferenceService("test-spec-with-gateway-refs",
				fixture.InNamespace[*v1alpha1.LLMInferenceService](nsName),
				fixture.WithModelURI("hf://facebook/opt-125m"),
				fixture.WithGatewayRefs(fixture.LLMGatewayRef("test-gateway", nsName)),
				fixture.WithManagedRoute(),
			)

			// when
			errValidation := envTest.Client.Create(ctx, llmSvc)

			// then
			Expect(errValidation).To(HaveOccurred())
			Expect(errValidation.Error()).To(ContainSubstring("cannot be used with managed route"))
		})

		It("should reject LLMInferenceService with managed route spec and user-defined gateway refs", func(ctx SpecContext) {
			// given
			llmSvc := fixture.LLMInferenceService("test-spec-with-gateway-refs",
				fixture.InNamespace[*v1alpha1.LLMInferenceService](nsName),
				fixture.WithModelURI("hf://facebook/opt-125m"),
				fixture.WithGatewayRefs(fixture.LLMGatewayRef("test-gateway", nsName)),
				fixture.WithHTTPRouteSpec(&fixture.HTTPRoute("test-route",
					fixture.WithHTTPRule(
						fixture.Matches(fixture.PathPrefixMatch("/test")),
						fixture.BackendRefs(fixture.ServiceRef("custom-backend", 8080, 1)),
						fixture.Timeouts("30s", "60s"),
					),
				).Spec),
			)

			// when
			errValidation := envTest.Client.Create(ctx, llmSvc)

			// then
			Expect(errValidation).To(HaveOccurred())
			Expect(errValidation.Error()).To(ContainSubstring("unsupported configuration"))
		})
	})

	Context("parallelism constraints validation", func() {
		It("should reject LLMInferenceService with both pipeline and data parallelism", func(ctx SpecContext) {
			// given
			llmSvc := fixture.LLMInferenceService("test-both-pipeline-and-data",
				fixture.InNamespace[*v1alpha1.LLMInferenceService](nsName),
				fixture.WithModelURI("hf://facebook/opt-125m"),
				fixture.WithParallelism(fixture.ParallelismSpec(
					fixture.WithPipelineParallelism(2),
					fixture.WithDataParallelism(4),
					fixture.WithDataLocalParallelism(2),
				)),
			)

			// when
			errValidation := envTest.Client.Create(ctx, llmSvc)

			// then
			Expect(errValidation).To(HaveOccurred())
			Expect(errValidation.Error()).To(ContainSubstring("cannot set both pipeline parallelism and data parallelism"))
		})

		It("should reject LLMInferenceService with data parallelism but missing dataLocal", func(ctx SpecContext) {
			// given
			llmSvc := fixture.LLMInferenceService("test-data-without-datalocal",
				fixture.InNamespace[*v1alpha1.LLMInferenceService](nsName),
				fixture.WithModelURI("hf://facebook/opt-125m"),
				fixture.WithParallelism(fixture.ParallelismSpec(
					fixture.WithDataParallelism(4),
				)),
			)

			// when
			errValidation := envTest.Client.Create(ctx, llmSvc)

			// then
			Expect(errValidation).To(HaveOccurred())
			Expect(errValidation.Error()).To(ContainSubstring("dataLocal must be set when data is set"))
		})

		It("should reject LLMInferenceService with dataLocal parallelism but missing data", func(ctx SpecContext) {
			// given
			llmSvc := fixture.LLMInferenceService("test-datalocal-without-data",
				fixture.InNamespace[*v1alpha1.LLMInferenceService](nsName),
				fixture.WithModelURI("hf://facebook/opt-125m"),
				fixture.WithParallelism(fixture.ParallelismSpec(
					fixture.WithDataLocalParallelism(2),
				)),
			)

			// when
			errValidation := envTest.Client.Create(ctx, llmSvc)

			// then
			Expect(errValidation).To(HaveOccurred())
			Expect(errValidation.Error()).To(ContainSubstring("data must be set when dataLocal is set"))
		})

		It("should reject LLMInferenceService with worker but no parallelism configuration", func(ctx SpecContext) {
			// given
			llmSvc := fixture.LLMInferenceService("test-worker-no-parallelism",
				fixture.InNamespace[*v1alpha1.LLMInferenceService](nsName),
				fixture.WithModelURI("hf://facebook/opt-125m"),
				fixture.WithWorker(fixture.SimpleWorkerPodSpec()),
			)

			// when
			errValidation := envTest.Client.Create(ctx, llmSvc)

			// then
			Expect(errValidation).To(HaveOccurred())
			Expect(errValidation.Error()).To(ContainSubstring("when worker is specified, parallelism must be configured"))
		})

		It("should reject LLMInferenceService with prefill having both pipeline and data parallelism", func(ctx SpecContext) {
			// given
			llmSvc := fixture.LLMInferenceService("test-prefill-both-parallelism",
				fixture.InNamespace[*v1alpha1.LLMInferenceService](nsName),
				fixture.WithModelURI("hf://facebook/opt-125m"),
				fixture.WithPrefillParallelism(fixture.ParallelismSpec(
					fixture.WithPipelineParallelism(2),
					fixture.WithDataParallelism(4),
					fixture.WithDataLocalParallelism(2),
				)),
			)

			// when
			errValidation := envTest.Client.Create(ctx, llmSvc)

			// then
			Expect(errValidation).To(HaveOccurred())
			Expect(errValidation.Error()).To(ContainSubstring("cannot set both pipeline parallelism and data parallelism"))
		})

		It("should reject LLMInferenceService with prefill worker but no parallelism configuration", func(ctx SpecContext) {
			// given
			llmSvc := fixture.LLMInferenceService("test-prefill-worker-no-parallelism",
				fixture.InNamespace[*v1alpha1.LLMInferenceService](nsName),
				fixture.WithModelURI("hf://facebook/opt-125m"),
				fixture.WithPrefillWorker(fixture.SimpleWorkerPodSpec()),
			)

			// when
			errValidation := envTest.Client.Create(ctx, llmSvc)

			// then
			Expect(errValidation).To(HaveOccurred())
			Expect(errValidation.Error()).To(ContainSubstring("when worker is specified, parallelism must be configured"))
		})

		It("should accept LLMInferenceService with valid pipeline parallelism", func(ctx SpecContext) {
			// given
			llmSvc := fixture.LLMInferenceService("test-valid-pipeline",
				fixture.InNamespace[*v1alpha1.LLMInferenceService](nsName),
				fixture.WithModelURI("hf://facebook/opt-125m"),
				fixture.WithParallelism(fixture.ParallelismSpec(
					fixture.WithPipelineParallelism(2),
				)),
				fixture.WithWorker(fixture.SimpleWorkerPodSpec()),
			)

			// then
			Expect(envTest.Client.Create(ctx, llmSvc)).To(Succeed())
		})

		It("should accept LLMInferenceService with valid data parallelism", func(ctx SpecContext) {
			// given
			llmSvc := fixture.LLMInferenceService("test-valid-data",
				fixture.InNamespace[*v1alpha1.LLMInferenceService](nsName),
				fixture.WithModelURI("hf://facebook/opt-125m"),
				fixture.WithParallelism(fixture.ParallelismSpec(
					fixture.WithDataParallelism(4),
					fixture.WithDataLocalParallelism(2),
				)),
				fixture.WithWorker(fixture.SimpleWorkerPodSpec()),
			)

			// then
			Expect(envTest.Client.Create(ctx, llmSvc)).To(Succeed())
		})

		It("should accept LLMInferenceService with valid prefill parallelism configuration", func(ctx SpecContext) {
			// given
			llmSvc := fixture.LLMInferenceService("test-valid-prefill",
				fixture.InNamespace[*v1alpha1.LLMInferenceService](nsName),
				fixture.WithModelURI("hf://facebook/opt-125m"),
				fixture.WithPrefillParallelism(fixture.ParallelismSpec(
					fixture.WithPipelineParallelism(2),
				)),
				fixture.WithPrefillWorker(fixture.SimpleWorkerPodSpec()),
			)

			// then
			Expect(envTest.Client.Create(ctx, llmSvc)).To(Succeed())
		})

		It("should reject LLMInferenceService update with different decode parallelism 'size'", func(ctx SpecContext) {
			name := "test-update-decode-parallelism-different-size"
			// given
			llmSvc := fixture.LLMInferenceService(name,
				fixture.InNamespace[*v1alpha1.LLMInferenceService](nsName),
				fixture.WithModelURI("hf://facebook/opt-125m"),
				fixture.WithParallelism(fixture.ParallelismSpec(
					fixture.WithDataParallelism(1),
					fixture.WithDataLocalParallelism(8),
				)),
				fixture.WithWorker(fixture.SimpleWorkerPodSpec()),
			)

			// Consistency check
			Expect(llmSvc.Spec.Parallelism.GetSize()).To(Equal(ptr.To(int32(1))))
			Expect(envTest.Client.Create(ctx, llmSvc)).To(Succeed())

			updated := &v1alpha1.LLMInferenceService{}
			Expect(envTest.Client.Get(ctx, types.NamespacedName{Namespace: llmSvc.GetNamespace(), Name: llmSvc.GetName()}, updated)).To(Succeed())

			updated.Spec.Parallelism.Data = ptr.To[int32](8)
			updated.Spec.Parallelism.DataLocal = ptr.To[int32](1)

			// Consistency check
			Expect(updated.Spec.Parallelism.GetSize()).To(Equal(ptr.To(int32(8))))

			// then
			Expect(envTest.Client.Update(ctx, updated)).To(HaveOccurred())
		})

		It("should reject LLMInferenceService update with different prefill parallelism 'size'", func(ctx SpecContext) {
			name := "test-update-prefill-parallelism-different-size"
			// given
			llmSvc := fixture.LLMInferenceService(name,
				fixture.InNamespace[*v1alpha1.LLMInferenceService](nsName),
				fixture.WithModelURI("hf://facebook/opt-125m"),
				fixture.WithPrefillParallelism(fixture.ParallelismSpec(
					fixture.WithDataParallelism(1),
					fixture.WithDataLocalParallelism(8),
				)),
				fixture.WithPrefillWorker(fixture.SimpleWorkerPodSpec()),
			)

			// Consistency check
			Expect(llmSvc.Spec.Prefill.Parallelism.GetSize()).To(Equal(ptr.To(int32(1))))
			Expect(envTest.Client.Create(ctx, llmSvc)).To(Succeed())

			updated := &v1alpha1.LLMInferenceService{}
			Expect(envTest.Client.Get(ctx, types.NamespacedName{Namespace: llmSvc.GetNamespace(), Name: llmSvc.GetName()}, updated)).To(Succeed())

			updated.Spec.Prefill.Parallelism.Data = ptr.To[int32](9)
			updated.Spec.Prefill.Parallelism.DataLocal = ptr.To[int32](1)

			// Consistency check
			Expect(updated.Spec.Prefill.Parallelism.GetSize()).To(Equal(ptr.To[int32](9)))

			// then
			Expect(envTest.Client.Update(ctx, updated)).To(HaveOccurred())
		})

		It("should accept LLMInferenceService without parallelism configuration", func(ctx SpecContext) {
			// given
			llmSvc := fixture.LLMInferenceService("test-no-parallelism",
				fixture.InNamespace[*v1alpha1.LLMInferenceService](nsName),
				fixture.WithModelURI("hf://facebook/opt-125m"),
			)

			// then
			Expect(envTest.Client.Create(ctx, llmSvc)).To(Succeed())
		})
	})
})

var _ = Describe("LLMInferenceService API validation", func() {
	var (
		ns     *corev1.Namespace
		nsName string
	)
	BeforeEach(func(ctx SpecContext) {
		nsName = fmt.Sprintf("test-llmisvc-api-validation-%d", time.Now().UnixNano())

		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: nsName,
			},
		}
		Expect(envTest.Client.Create(ctx, ns)).To(Succeed())

		DeferCleanup(func() {
			ns := ns
			envTest.DeleteAll(ns)
		})
	})
	Context("Integer value validation", func() {
		It("should reject LLMInferenceService with negative workload replicas", func(ctx SpecContext) {
			// given
			llmSvc := fixture.LLMInferenceService("test-negative-replicas",
				fixture.InNamespace[*v1alpha1.LLMInferenceService](nsName),
				fixture.WithModelURI("hf://facebook/opt-125m"),
				fixture.WithReplicas(-1),
			)

			// when
			errValidation := envTest.Client.Create(ctx, llmSvc)

			// then
			Expect(errValidation).To(HaveOccurred(), "Expected the Create call to fail due to a validation error, but it succeeded")
			Expect(errValidation.Error()).To(ContainSubstring("spec.replicas in body should be greater than or equal to 0"))
		})

		It("should reject LLMInferenceService with negative tensor parallelism", func(ctx SpecContext) {
			// given
			llmSvc := fixture.LLMInferenceService("test-negative-int-parallelism",
				fixture.InNamespace[*v1alpha1.LLMInferenceService](nsName),
				fixture.WithModelURI("hf://facebook/opt-125m"),
				fixture.WithParallelism(fixture.ParallelismSpec(
					fixture.WithTensorParallelism(-1),
				)),
			)

			// when
			errValidation := envTest.Client.Create(ctx, llmSvc)

			// then
			Expect(errValidation).To(HaveOccurred(), "Expected the Create call to fail due to a validation error, but it succeeded")
			Expect(errValidation.Error()).To(ContainSubstring("spec.parallelism.tensor in body should be greater than or equal to 1"))
		})

		It("should reject LLMInferenceService with negative pipeline parallelism", func(ctx SpecContext) {
			// given
			llmSvc := fixture.LLMInferenceService("test-negative-int-pipeline",
				fixture.InNamespace[*v1alpha1.LLMInferenceService](nsName),
				fixture.WithModelURI("hf://facebook/opt-125m"),
				fixture.WithParallelism(fixture.ParallelismSpec(
					fixture.WithPipelineParallelism(-1),
				)),
			)

			// when
			errValidation := envTest.Client.Create(ctx, llmSvc)

			// then
			Expect(errValidation).To(HaveOccurred(), "Expected the Create call to fail due to a validation error, but it succeeded")
			Expect(errValidation.Error()).To(ContainSubstring("spec.parallelism.pipeline in body should be greater than or equal to 1"))
		})

		It("should reject LLMInferenceService with negative data parallelism", func(ctx SpecContext) {
			// given
			llmSvc := fixture.LLMInferenceService("test-negative-data",
				fixture.InNamespace[*v1alpha1.LLMInferenceService](nsName),
				fixture.WithModelURI("hf://facebook/opt-125m"),
				fixture.WithParallelism(fixture.ParallelismSpec(
					fixture.WithDataParallelism(-1),
					fixture.WithDataLocalParallelism(1),
				)),
			)

			// when
			errValidation := envTest.Client.Create(ctx, llmSvc)

			// then
			Expect(errValidation).To(HaveOccurred(), "Expected the Create call to fail due to a validation error, but it succeeded")
			Expect(errValidation.Error()).To(ContainSubstring("spec.parallelism.data in body should be greater than or equal to 1"))
		})

		It("should reject LLMInferenceService with zero dataLocal parallelism", func(ctx SpecContext) {
			// given
			llmSvc := fixture.LLMInferenceService("test-negative-datalocal",
				fixture.InNamespace[*v1alpha1.LLMInferenceService](nsName),
				fixture.WithModelURI("hf://facebook/opt-125m"),
				fixture.WithParallelism(fixture.ParallelismSpec(
					fixture.WithDataParallelism(4),
					fixture.WithDataLocalParallelism(0),
				)),
			)

			// when
			errValidation := envTest.Client.Create(ctx, llmSvc)

			// then
			Expect(errValidation).To(HaveOccurred(), "Expected the Create call to fail due to a validation error, but it succeeded")
			Expect(errValidation.Error()).To(ContainSubstring("spec.parallelism.dataLocal in body should be greater than or equal to 1"))
		})

		It("should reject LLMInferenceService with zero data parallelism RPC Port", func(ctx SpecContext) {
			// given
			llmSvc := fixture.LLMInferenceService("test-zero-data-rpc-port",
				fixture.InNamespace[*v1alpha1.LLMInferenceService](nsName),
				fixture.WithModelURI("hf://facebook/opt-125m"),
				fixture.WithParallelism(fixture.ParallelismSpec(
					fixture.WithDataRPCPort(0),
				)),
			)

			// when
			errValidation := envTest.Client.Create(ctx, llmSvc)

			// then
			Expect(errValidation).To(HaveOccurred(), "Expected the Create call to fail due to a validation error, but it succeeded")
			Expect(errValidation.Error()).To(ContainSubstring("spec.parallelism.dataRPCPort in body should be greater than or equal to 1"))
		})

		It("should reject LLMInferenceService with too large data parallelism RPC Port", func(ctx SpecContext) {
			// given
			llmSvc := fixture.LLMInferenceService("test-max-data-rpc-port-exceeded",
				fixture.InNamespace[*v1alpha1.LLMInferenceService](nsName),
				fixture.WithModelURI("hf://facebook/opt-125m"),
				fixture.WithParallelism(fixture.ParallelismSpec(
					fixture.WithDataRPCPort(99999),
				)),
			)

			// when
			errValidation := envTest.Client.Create(ctx, llmSvc)

			// then
			Expect(errValidation).To(HaveOccurred(), "Expected the Create call to fail due to a validation error, but it succeeded")
			Expect(errValidation.Error()).To(ContainSubstring("spec.parallelism.dataRPCPort in body should be less than or equal to 65535"))
		})
	})
})

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

package crdvalidation_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/apis"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

var _ = Describe("LLMInferenceService storageInitializer CEL", func() {
	It("should reject storageContainerName when enabled is false", func(ctx SpecContext) {
		modelURL, err := apis.ParseURL("custom://models/llama")
		Expect(err).NotTo(HaveOccurred())

		svc := &v1alpha2.LLMInferenceService{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "storage-disabled-named-",
				Namespace:    "default",
			},
			Spec: v1alpha2.LLMInferenceServiceSpec{
				Model: v1alpha2.LLMModelSpec{URI: *modelURL},
				StorageInitializer: &v1alpha2.StorageInitializerSpec{
					Enabled:              ptr.To(false),
					StorageContainerName: ptr.To("my-csc"),
				},
			},
		}

		err = envTest.Create(ctx, svc)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("storageContainerName cannot be set when enabled is false"))
	})

	It("should accept storageContainerName when enabled is omitted", func(ctx SpecContext) {
		modelURL, err := apis.ParseURL("custom://models/llama")
		Expect(err).NotTo(HaveOccurred())

		svc := &v1alpha2.LLMInferenceService{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "storage-named-",
				Namespace:    "default",
			},
			Spec: v1alpha2.LLMInferenceServiceSpec{
				Model: v1alpha2.LLMModelSpec{URI: *modelURL},
				StorageInitializer: &v1alpha2.StorageInitializerSpec{
					StorageContainerName: ptr.To("my-csc"),
				},
			},
		}

		Expect(envTest.Client.Create(ctx, svc)).To(Succeed())
		Expect(envTest.Client.Delete(ctx, svc)).To(Succeed())
	})
})

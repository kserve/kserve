/*
Copyright 2018 The Kubernetes Authors.

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

package admission

import (
	"errors"
	"net/http"

	"github.com/mattbaird/jsonpatch"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission/types"
)

var _ = Describe("admission webhook response", func() {
	Describe("ErrorResponse", func() {
		It("should return the response with an error", func() {
			err := errors.New("this is an error")
			expected := types.Response{
				Response: &admissionv1beta1.AdmissionResponse{
					Allowed: false,
					Result: &metav1.Status{
						Code:    http.StatusBadRequest,
						Message: err.Error(),
					},
				},
			}
			resp := ErrorResponse(http.StatusBadRequest, err)
			Expect(resp).To(Equal(expected))
		})
	})

	Describe("ValidationResponse", func() {
		It("should return the response with an admission decision", func() {
			expected := types.Response{
				Response: &admissionv1beta1.AdmissionResponse{
					Allowed: true,
					Result: &metav1.Status{
						Reason: metav1.StatusReason("allow to admit"),
					},
				},
			}
			resp := ValidationResponse(true, "allow to admit")
			Expect(resp).To(Equal(expected))
		})
	})

	Describe("PatchResponse", func() {
		It("should return the response with patches", func() {
			expected := types.Response{
				Patches: []jsonpatch.JsonPatchOperation{},
				Response: &admissionv1beta1.AdmissionResponse{
					Allowed:   true,
					PatchType: func() *admissionv1beta1.PatchType { pt := admissionv1beta1.PatchTypeJSONPatch; return &pt }(),
				},
			}
			resp := PatchResponse(&corev1.Pod{}, &corev1.Pod{})
			Expect(resp).To(Equal(expected))
		})
	})
})

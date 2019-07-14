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
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission/types"
)

var _ = Describe("admission webhook decoder", func() {
	var decoder types.Decoder
	BeforeEach(func(done Done) {
		var err error
		decoder, err = NewDecoder(scheme.Scheme)
		Expect(err).NotTo(HaveOccurred())
		Expect(decoder).NotTo(BeNil())
		close(done)
	})

	Describe("NewDecoder", func() {
		It("should return a decoder without an error", func() {
			decoder, err := NewDecoder(scheme.Scheme)
			Expect(err).NotTo(HaveOccurred())
			Expect(decoder).NotTo(BeNil())
		})
	})

	Describe("Decode", func() {
		req := types.Request{
			AdmissionRequest: &admissionv1beta1.AdmissionRequest{
				Object: runtime.RawExtension{
					Raw: []byte(`{
    "apiVersion": "v1",
    "kind": "Pod",
    "metadata": {
        "name": "foo",
        "namespace": "default"
    },
    "spec": {
        "containers": [
            {
                "image": "bar",
                "name": "bar"
            }
        ]
    }
}`),
				},
			},
		}

		It("should be able to decode", func() {
			err := decoder.Decode(req, &corev1.Pod{})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return an error if the GVK mismatch", func() {
			err := decoder.Decode(req, &corev1.Node{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring("unable to decode"))
		})
	})
})

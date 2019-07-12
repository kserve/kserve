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
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"

	"github.com/mattbaird/jsonpatch"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	admissionregistrationv1beta1 "k8s.io/api/admissionregistration/v1beta1"
	atypes "sigs.k8s.io/controller-runtime/pkg/webhook/admission/types"
	"sigs.k8s.io/controller-runtime/pkg/webhook/types"
)

var _ = Describe("admission webhook", func() {
	var w *httptest.ResponseRecorder
	BeforeEach(func(done Done) {
		w = &httptest.ResponseRecorder{
			Body: bytes.NewBuffer(nil),
		}
		close(done)
	})
	Describe("validating webhook", func() {
		var alwaysAllow, alwaysDeny *fakeHandler
		var req *http.Request
		var wh *Webhook
		BeforeEach(func(done Done) {
			alwaysAllow = &fakeHandler{
				fn: func(ctx context.Context, req atypes.Request) atypes.Response {
					return atypes.Response{
						Response: &admissionv1beta1.AdmissionResponse{
							Allowed: true,
						},
					}
				},
			}
			alwaysDeny = &fakeHandler{
				fn: func(ctx context.Context, req atypes.Request) atypes.Response {
					return atypes.Response{
						Response: &admissionv1beta1.AdmissionResponse{
							Allowed: false,
						},
					}
				},
			}
			req = &http.Request{
				Header: http.Header{"Content-Type": []string{"application/json"}},
				Body:   nopCloser{Reader: bytes.NewBufferString(`{"request":{}}`)},
			}
			close(done)
		})

		Context("multiple handlers can be invoked", func() {
			BeforeEach(func(done Done) {
				wh = &Webhook{
					Type:     types.WebhookTypeValidating,
					Handlers: []Handler{alwaysAllow, alwaysDeny},
				}
				close(done)
			})

			It("should deny the request", func() {
				expected := []byte(`{"response":{"uid":"","allowed":false,"status":{"metadata":{},"code":200}}}
`)
				wh.ServeHTTP(w, req)
				Expect(w.Body.Bytes()).To(Equal(expected))
				Expect(alwaysAllow.invoked).To(BeTrue())
				Expect(alwaysDeny.invoked).To(BeTrue())
			})
		})

		Context("validating webhook should return if one of the handler denies", func() {
			BeforeEach(func(done Done) {
				wh = &Webhook{
					Type:     types.WebhookTypeValidating,
					Handlers: []Handler{alwaysDeny, alwaysAllow},
				}
				close(done)
			})

			It("should deny the request", func() {
				expected := []byte(`{"response":{"uid":"","allowed":false,"status":{"metadata":{},"code":200}}}
`)
				wh.ServeHTTP(w, req)
				Expect(w.Body.Bytes()).To(Equal(expected))
				Expect(alwaysDeny.invoked).To(BeTrue())
				Expect(alwaysAllow.invoked).To(BeFalse())
			})
		})
	})

	Describe("mutating webhook", func() {
		Context("multiple patch handlers", func() {
			req := &http.Request{
				Header: http.Header{"Content-Type": []string{"application/json"}},
				Body:   nopCloser{Reader: bytes.NewBufferString(`{"request":{}}`)},
			}
			patcher1 := &fakeHandler{
				fn: func(ctx context.Context, req atypes.Request) atypes.Response {
					return atypes.Response{
						Patches: []jsonpatch.JsonPatchOperation{
							{
								Operation: "add",
								Path:      "/metadata/annotation/new-key",
								Value:     "new-value",
							},
							{
								Operation: "replace",
								Path:      "/spec/replicas",
								Value:     "2",
							},
						},
						Response: &admissionv1beta1.AdmissionResponse{
							Allowed:   true,
							PatchType: func() *admissionv1beta1.PatchType { pt := admissionv1beta1.PatchTypeJSONPatch; return &pt }(),
						},
					}
				},
			}
			patcher2 := &fakeHandler{
				fn: func(ctx context.Context, req atypes.Request) atypes.Response {
					return atypes.Response{
						Patches: []jsonpatch.JsonPatchOperation{
							{
								Operation: "add",
								Path:      "/metadata/annotation/hello",
								Value:     "world",
							},
						},
						Response: &admissionv1beta1.AdmissionResponse{
							Allowed:   true,
							PatchType: func() *admissionv1beta1.PatchType { pt := admissionv1beta1.PatchTypeJSONPatch; return &pt }(),
						},
					}
				},
			}
			wh := &Webhook{
				Type:     types.WebhookTypeMutating,
				Handlers: []Handler{patcher1, patcher2},
			}
			expected := []byte(
				`{"response":{"uid":"","allowed":true,"status":{"metadata":{},"code":200},` +
					`"patch":"W3sib3AiOiJhZGQiLCJwYXRoIjoiL21ldGFkYXRhL2Fubm90YXRpb2` +
					`4vbmV3LWtleSIsInZhbHVlIjoibmV3LXZhbHVlIn0seyJvcCI6InJlcGxhY2UiLCJwYXRoIjoiL3NwZWMvcmVwbGljYXMiLC` +
					`J2YWx1ZSI6IjIifSx7Im9wIjoiYWRkIiwicGF0aCI6Ii9tZXRhZGF0YS9hbm5vdGF0aW9uL2hlbGxvIiwidmFsdWUiOiJ3b3JsZCJ9XQ==",` +
					`"patchType":"JSONPatch"}}
`)
			patches := []jsonpatch.JsonPatchOperation{
				{
					Operation: "add",
					Path:      "/metadata/annotation/new-key",
					Value:     "new-value",
				},
				{
					Operation: "replace",
					Path:      "/spec/replicas",
					Value:     "2",
				},
				{
					Operation: "add",
					Path:      "/metadata/annotation/hello",
					Value:     "world",
				},
			}
			j, _ := json.Marshal(patches)
			base64encoded := base64.StdEncoding.EncodeToString(j)
			It("should aggregates patches from multiple handlers", func() {
				wh.ServeHTTP(w, req)
				Expect(w.Body.Bytes()).To(Equal(expected))
				Expect(w.Body.String()).To(ContainSubstring(base64encoded))
				Expect(patcher1.invoked).To(BeTrue())
				Expect(patcher2.invoked).To(BeTrue())
			})
		})

		Context("patch handler denies the request", func() {
			req := &http.Request{
				Header: http.Header{"Content-Type": []string{"application/json"}},
				Body:   nopCloser{Reader: bytes.NewBufferString(`{"request":{}}`)},
			}
			errPatcher := &fakeHandler{
				fn: func(ctx context.Context, req atypes.Request) atypes.Response {
					return atypes.Response{
						Response: &admissionv1beta1.AdmissionResponse{
							Allowed: false,
						},
					}
				},
			}
			wh := &Webhook{
				Type:     types.WebhookTypeMutating,
				Handlers: []Handler{errPatcher},
			}
			expected := []byte(`{"response":{"uid":"","allowed":false,"status":{"metadata":{},"code":200}}}
`)
			It("should deny the request", func() {
				wh.ServeHTTP(w, req)
				Expect(w.Body.Bytes()).To(Equal(expected))
				Expect(errPatcher.invoked).To(BeTrue())
			})
		})
	})

	Describe("webhook validation", func() {
		Context("valid mutating webhook", func() {
			wh := &Webhook{
				Type:     types.WebhookTypeMutating,
				Handlers: []Handler{&fakeHandler{}},
				Rules: []admissionregistrationv1beta1.RuleWithOperations{
					{
						Operations: []admissionregistrationv1beta1.OperationType{},
						Rule: admissionregistrationv1beta1.Rule{
							APIGroups:   []string{"apps"},
							APIVersions: []string{"v1"},
							Resources:   []string{"deployments"}},
					},
				},
			}
			It("should pass validation", func() {
				err := wh.Validate()
				Expect(err).NotTo(HaveOccurred())
				Expect(wh.Name).To(Equal("mutatedeployments.example.com"))
				Expect(wh.Path).To(Equal("/mutate-deployments"))
			})
		})

		Context("valid validating webhook", func() {
			wh := &Webhook{
				Type:     types.WebhookTypeValidating,
				Handlers: []Handler{&fakeHandler{}},
				Rules: []admissionregistrationv1beta1.RuleWithOperations{
					{
						Operations: []admissionregistrationv1beta1.OperationType{},
						Rule: admissionregistrationv1beta1.Rule{
							APIGroups:   []string{"apps"},
							APIVersions: []string{"v1"},
							Resources:   []string{"deployments"}},
					},
				},
			}
			It("should pass validation", func() {
				err := wh.Validate()
				Expect(err).NotTo(HaveOccurred())
				Expect(wh.Name).To(Equal("validatedeployments.example.com"))
				Expect(wh.Path).To(Equal("/validate-deployments"))
			})
		})

		Context("missing webhook type", func() {
			wh := &Webhook{
				Handlers: []Handler{&fakeHandler{}},
				Rules: []admissionregistrationv1beta1.RuleWithOperations{
					{
						Operations: []admissionregistrationv1beta1.OperationType{},
						Rule: admissionregistrationv1beta1.Rule{
							APIGroups:   []string{"apps"},
							APIVersions: []string{"v1"},
							Resources:   []string{"deployments"}},
					},
				},
			}
			It("should fail validation", func() {
				err := wh.Validate()
				Expect(err.Error()).To(ContainSubstring("only WebhookTypeMutating and WebhookTypeValidating are supported"))
			})
		})

		Context("missing Rules", func() {
			wh := &Webhook{
				Type:     types.WebhookTypeValidating,
				Handlers: []Handler{&fakeHandler{}},
			}
			It("should fail validation", func() {
				err := wh.Validate()
				Expect(err).To(Equal(errors.New("field Rules should not be empty")))

			})
		})

		Context("missing Handlers", func() {
			wh := &Webhook{
				Type:     types.WebhookTypeValidating,
				Handlers: []Handler{},
				Rules: []admissionregistrationv1beta1.RuleWithOperations{
					{
						Operations: []admissionregistrationv1beta1.OperationType{},
						Rule: admissionregistrationv1beta1.Rule{
							APIGroups:   []string{"apps"},
							APIVersions: []string{"v1"},
							Resources:   []string{"deployments"}},
					},
				},
			}
			It("should fail validation", func() {
				err := wh.Validate()
				Expect(err).To(Equal(errors.New("field Handler should not be empty")))
			})
		})

	})
})

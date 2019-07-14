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

package cert

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	admissionregistrationv1beta1 "k8s.io/api/admissionregistration/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/internal/cert/generator"
	"sigs.k8s.io/controller-runtime/pkg/webhook/internal/cert/writer"
)

var _ = Describe("provisioner", func() {
	Context("Invalid Provisioner", func() {
		It("should return error", func() {
			p := Provisioner{}
			_, err := p.Provision(Options{})
			Expect(err).To(MatchError("CertWriter need to be set"))
		})
	})

	Context("No objects in the options", func() {
		It("should return no error", func() {
			fcw := &fakeCertWriter{}
			p := Provisioner{CertWriter: fcw}
			changed, err := p.Provision(Options{})
			Expect(err).NotTo(HaveOccurred())
			Expect(changed).To(BeFalse())
			Expect(fcw.invokedEnsureCert).To(BeFalse())
			Expect(fcw.invokedInject).To(BeFalse())
		})
	})

	Context("WebhookClientConfig is missing in the options", func() {
		It("should return error", func() {
			p := Provisioner{CertWriter: &fakeCertWriter{}}
			_, err := p.Provision(Options{
				Objects: []runtime.Object{
					&corev1.Pod{},
				},
			})
			Expect(err).To(MatchError("clientConfig should not be empty"))
		})
	})

	Context("object is not support for injecting webhookClientConfig", func() {
		It("should return no error", func() {
			p := Provisioner{CertWriter: &fakeCertWriter{}}
			_, err := p.Provision(Options{
				ClientConfig: &admissionregistrationv1beta1.WebhookClientConfig{
					Service: &admissionregistrationv1beta1.ServiceReference{
						Namespace: "test-svc-namespace",
						Name:      "test-service",
					},
				},
				Objects: []runtime.Object{
					&corev1.Pod{},
				},
			})
			Expect(err.Error()).To(ContainSubstring("not supported for injecting a webhookClientConfig"))
		})
	})

	Context("webhookConfig has 0 webhook", func() {
		It("should return no error", func() {
			fcw := &fakeCertWriter{}
			p := Provisioner{CertWriter: fcw}
			_, err := p.Provision(Options{
				ClientConfig: &admissionregistrationv1beta1.WebhookClientConfig{
					Service: &admissionregistrationv1beta1.ServiceReference{
						Namespace: "test-svc-namespace",
						Name:      "test-service",
					},
				},
				Objects: []runtime.Object{
					&admissionregistrationv1beta1.MutatingWebhookConfiguration{},
				},
			})
			Expect(err).To(BeNil())
			Expect(fcw.invokedEnsureCert).To(BeTrue())
			Expect(fcw.invokedInject).To(BeTrue())
		})
	})

	Context("happy path", func() {
		It("should return no error", func() {
			fcw := &fakeCertWriter{}
			mwc := &admissionregistrationv1beta1.MutatingWebhookConfiguration{
				Webhooks: []admissionregistrationv1beta1.Webhook{
					{
						Name: "foo-webhook",
					},
				},
			}
			vwc := &admissionregistrationv1beta1.ValidatingWebhookConfiguration{
				Webhooks: []admissionregistrationv1beta1.Webhook{
					{
						Name: "foo-webhook",
					},
				},
			}
			p := Provisioner{CertWriter: fcw}
			_, err := p.Provision(Options{
				ClientConfig: &admissionregistrationv1beta1.WebhookClientConfig{
					Service: &admissionregistrationv1beta1.ServiceReference{
						Namespace: "test-svc-namespace",
						Name:      "test-service",
					},
				},
				Objects: []runtime.Object{mwc, vwc},
			})
			Expect(err).To(BeNil())
			Expect(fcw.invokedEnsureCert).To(BeTrue())
			Expect(fcw.invokedInject).To(BeTrue())
		})
	})
})

var _ = Describe("dnsNameFromClientConfig", func() {
	Context("Invalid WebhookClientConfig", func() {
		It("should return error", func() {
			_, err := dnsNameFromClientConfig(nil)
			Expect(err).To(MatchError("clientConfig should not be empty"))
		})
	})

	Context("Neither Service nor URL is set", func() {
		It("should return error", func() {
			urlStr := "foo.example.com"
			cc := &admissionregistrationv1beta1.WebhookClientConfig{
				Service: &admissionregistrationv1beta1.ServiceReference{},
				URL:     &urlStr,
			}
			_, err := dnsNameFromClientConfig(cc)
			Expect(err.Error()).To(ContainSubstring("service and URL can't be set at the same time in a webhook"))
		})
	})

	Context("Both Service and URL are set", func() {
		It("should return error", func() {
			urlStr := "https://foo.example.com"
			cc := &admissionregistrationv1beta1.WebhookClientConfig{
				Service: &admissionregistrationv1beta1.ServiceReference{},
				URL:     &urlStr,
			}
			_, err := dnsNameFromClientConfig(cc)
			Expect(err.Error()).To(ContainSubstring("service and URL can't be set at the same time in a webhook"))
		})
	})

	Context("Only service is set", func() {
		It("should return a DNS name", func() {
			path := "somepath"
			cc := &admissionregistrationv1beta1.WebhookClientConfig{
				Service: &admissionregistrationv1beta1.ServiceReference{
					Namespace: "test-svc-namespace",
					Name:      "test-service",
					Path:      &path,
				},
			}
			dnsName, err := dnsNameFromClientConfig(cc)
			Expect(err).NotTo(HaveOccurred())
			Expect(dnsName).To(Equal("test-service.test-svc-namespace.svc"))
		})
	})

	Context("Only URL is set", func() {
		It("should return a DNS name", func() {
			urlStr := "https://foo.example.com/webhookendpoint"
			cc := &admissionregistrationv1beta1.WebhookClientConfig{
				URL: &urlStr,
			}
			dnsName, err := dnsNameFromClientConfig(cc)
			Expect(err).NotTo(HaveOccurred())
			Expect(dnsName).To(Equal("foo.example.com"))
		})

		It("should return a DNS name w/o port", func() {
			urlStr := "https://foo.example.com:9876/webhookendpoint"
			cc := &admissionregistrationv1beta1.WebhookClientConfig{
				URL: &urlStr,
			}
			dnsName, err := dnsNameFromClientConfig(cc)
			Expect(err).NotTo(HaveOccurred())
			Expect(dnsName).To(Equal("foo.example.com"))
		})
	})
})

type fakeCertWriter struct {
	invokedEnsureCert bool
	invokedInject     bool
}

var _ writer.CertWriter = &fakeCertWriter{}

func (f *fakeCertWriter) EnsureCert(dnsName string) (*generator.Artifacts, bool, error) {
	f.invokedEnsureCert = true
	return &generator.Artifacts{}, true, nil
}

func (f *fakeCertWriter) Inject(objs ...runtime.Object) error {
	f.invokedInject = true
	return nil
}

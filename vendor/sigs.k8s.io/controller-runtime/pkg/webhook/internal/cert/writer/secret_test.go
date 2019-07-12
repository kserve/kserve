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

package writer

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/internal/cert/generator"
	fakegenerator "sigs.k8s.io/controller-runtime/pkg/webhook/internal/cert/generator/fake"
)

var _ = Describe("secretCertWriter", func() {
	dnsName := "test-service.test-svc-namespace.svc"

	var certWriter CertWriter
	var sCertWriter *secretCertWriter
	var secret *corev1.Secret

	BeforeEach(func(done Done) {
		var err error
		certWriter, err = NewSecretCertWriter(SecretCertWriterOptions{
			Client: fake.NewFakeClient(),
			Secret: &types.NamespacedName{
				Namespace: "namespace-bar",
				Name:      "secret-foo",
			},
			CertGenerator: &fakegenerator.CertGenerator{
				DNSNameToCertArtifacts: map[string]*generator.Artifacts{
					dnsName: {
						CAKey:  []byte(`CAKeyBytes`),
						CACert: []byte(`CACertBytes`),
						Cert:   []byte(`CertBytes`),
						Key:    []byte(`KeyBytes`),
					},
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		sCertWriter = certWriter.(*secretCertWriter)
		close(done)
	})

	Context("Failed to EnsureCerts", func() {
		Describe("empty DNS name", func() {
			It("should return error", func() {
				_, _, err := certWriter.EnsureCert("")
				Expect(err).To(MatchError("dnsName should not be empty"))
			})
		})

	})

	Context("Succeeded to EnsureCerts", func() {
		BeforeEach(func(done Done) {
			//isController := true
			//blockOwnerDeletion := true
			secret = &corev1.Secret{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Secret",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "namespace-bar",
					Name:      "secret-foo",
					//OwnerReferences: []metav1.OwnerReference{
					//	{
					//		APIVersion:         "admissionregistration.k8s.io/v1beta1",
					//		Kind:               "MutatingWebhookConfiguration",
					//		Name:               "test-mwc",
					//		UID:                "123456",
					//		BlockOwnerDeletion: &blockOwnerDeletion,
					//		Controller:         &isController,
					//	},
					//},
				},
				Data: map[string][]byte{
					CAKeyName:      []byte(`CAKeyBytes`),
					CACertName:     []byte(`CACertBytes`),
					ServerKeyName:  []byte(`KeyBytes`),
					ServerCertName: []byte(`CertBytes`),
				},
			}
			close(done)
		})

		Context("certGenerator is not set", func() {
			It("should default it and return no error", func() {
				_, _, err := certWriter.EnsureCert(dnsName)
				Expect(err).NotTo(HaveOccurred())
				list := &corev1.SecretList{}
				err = sCertWriter.Client.List(nil, &client.ListOptions{
					Namespace: "namespace-bar",
				}, list)
				Expect(err).NotTo(HaveOccurred())
				Expect(list.Items).To(HaveLen(1))
			})
		})

		Context("no existing secret", func() {
			It("should create new secrets with certs", func() {
				_, changed, err := certWriter.EnsureCert(dnsName)
				Expect(err).NotTo(HaveOccurred())
				list := &corev1.SecretList{}
				err = sCertWriter.Client.List(nil, &client.ListOptions{
					Namespace: "namespace-bar",
				}, list)
				Expect(err).NotTo(HaveOccurred())
				Expect(list.Items).To(ConsistOf(*secret))
				Expect(list.Items).To(HaveLen(1))
				Expect(changed).To(BeTrue())
			})
		})

		Context("old secret exists", func() {
			var oldSecret *corev1.Secret

			Context("cert is invalid", func() {
				Describe("cert in secret is incomplete", func() {
					BeforeEach(func(done Done) {
						oldSecret = secret.DeepCopy()
						oldSecret.Data = nil
						sCertWriter.Client = fake.NewFakeClient(oldSecret)
						close(done)
					})

					It("should replace with new certs", func() {
						_, changed, err := certWriter.EnsureCert(dnsName)
						Expect(err).NotTo(HaveOccurred())
						list := &corev1.SecretList{}
						err = sCertWriter.Client.List(nil, &client.ListOptions{
							Namespace: "namespace-bar",
						}, list)
						Expect(err).NotTo(HaveOccurred())
						Expect(list.Items).To(ConsistOf(*secret))
						Expect(list.Items).To(HaveLen(1))
						Expect(changed).To(BeTrue())
					})
				})

				Describe("cert content is invalid", func() {
					BeforeEach(func(done Done) {
						oldSecret = secret.DeepCopy()
						oldSecret.Data = map[string][]byte{
							CAKeyName:      []byte(`invalidCAKeyBytes`),
							CACertName:     []byte(`invalidCACertBytes`),
							ServerKeyName:  []byte(`oldKeyBytes`),
							ServerCertName: []byte(`oldCertBytes`),
						}
						sCertWriter.Client = fake.NewFakeClient(oldSecret)
						close(done)
					})

					It("should replace with new certs", func() {
						_, changed, err := certWriter.EnsureCert(dnsName)
						Expect(err).NotTo(HaveOccurred())
						list := &corev1.SecretList{}
						err = sCertWriter.Client.List(nil, &client.ListOptions{
							Namespace: "namespace-bar",
						}, list)
						Expect(err).NotTo(HaveOccurred())
						Expect(list.Items).To(ConsistOf(*secret))
						Expect(list.Items).To(HaveLen(1))
						Expect(changed).To(BeTrue())
					})
				})
			})

			Context("cert is valid", func() {
				BeforeEach(func(done Done) {
					oldSecret.Data = map[string][]byte{
						CAKeyName:      []byte(certs2.CAKey),
						CACertName:     []byte(certs2.CACert),
						ServerKeyName:  []byte(certs2.Key),
						ServerCertName: []byte(certs2.Cert),
					}
					sCertWriter.Client = fake.NewFakeClient(oldSecret)
					close(done)
				})

				Context("when not expiring", func() {
					BeforeEach(func(done Done) {
						oldSecret = secret.DeepCopy()
						oldSecret.Data = map[string][]byte{
							CAKeyName:      []byte(certs2.CAKey),
							CACertName:     []byte(certs2.CACert),
							ServerKeyName:  []byte(certs2.Key),
							ServerCertName: []byte(certs2.Cert),
						}

						sCertWriter.Client = fake.NewFakeClient(oldSecret)
						close(done)
					})
					It("should keep the secret", func() {
						_, changed, err := certWriter.EnsureCert(dnsName)
						Expect(err).NotTo(HaveOccurred())
						list := &corev1.SecretList{}
						err = sCertWriter.Client.List(nil, &client.ListOptions{
							Namespace: "namespace-bar",
						}, list)
						Expect(err).NotTo(HaveOccurred())
						Expect(list.Items).To(HaveLen(1))
						Expect(list.Items[0]).To(Equal(*oldSecret))
						Expect(changed).To(BeFalse())
					})
				})

				Context("when expiring", func() {
					// TODO: implement this.
					BeforeEach(func(done Done) {
						oldSecret = secret.DeepCopy()
						oldSecret.Data = map[string][]byte{
							CAKeyName:  []byte(`oldCAKeyBytes`),
							CACertName: []byte(`oldCACertBytes`),
							//ServerKeyName:  []byte(expiringKeyPEM),
							//ServerCertName: []byte(expiringCertPEM),
						}
						//j, _ := json.Marshal(someNewValidSecret)
						//expectedSecret = runtime.RawExtension{Raw: j}

						sCertWriter.Client = fake.NewFakeClient(oldSecret)
						close(done)
					})

					It("should replace the expiring cert", func() {

					})
				})
			})
		})
	})
})

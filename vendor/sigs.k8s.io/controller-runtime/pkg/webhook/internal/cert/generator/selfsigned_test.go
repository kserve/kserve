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

package generator

import (
	"crypto/x509"
	"encoding/pem"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Cert Generator", func() {
	cn := "mysvc.myns.svc"
	Describe("CA doesn't exist", func() {
		It("should generate CA", func() {
			cp := SelfSignedCertGenerator{}
			certs, err := cp.Generate(cn)
			Expect(err).NotTo(HaveOccurred())

			// First, create the set of root certificates. For this example we only
			// have one. It's also possible to omit this in order to use the
			// default root set of the current operating system.
			roots := x509.NewCertPool()
			ok := roots.AppendCertsFromPEM(certs.CACert)
			Expect(ok).To(BeTrue())

			block, _ := pem.Decode(certs.Cert)
			Expect(block).NotTo(BeNil())

			cert, err := x509.ParseCertificate(block.Bytes)
			Expect(err).NotTo(HaveOccurred())

			opts := x509.VerifyOptions{
				DNSName: cn,
				Roots:   roots,
			}

			_, err = cert.Verify(opts)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("CA doesn't exist", func() {
		Context("CA is valid", func() {
			It("should reuse existing CA", func() {
				cp := SelfSignedCertGenerator{}
				certs, err := cp.Generate("foo.example.com")
				Expect(err).NotTo(HaveOccurred())

				cp = SelfSignedCertGenerator{}
				cp.SetCA(certs.CAKey, certs.CACert)
				certs, err = cp.Generate(cn)
				Expect(err).NotTo(HaveOccurred())

				Expect(certs.CAKey).To(Equal(cp.caKey))
				Expect(certs.CACert).To(Equal(cp.caCert))

				// First, create the set of root certificates. For this example we only
				// have one. It's also possible to omit this in order to use the
				// default root set of the current operating system.
				roots := x509.NewCertPool()
				ok := roots.AppendCertsFromPEM(certs.CACert)
				Expect(ok).To(BeTrue())

				block, _ := pem.Decode(certs.Cert)
				Expect(block).NotTo(BeNil())

				cert, err := x509.ParseCertificate(block.Bytes)
				Expect(err).NotTo(HaveOccurred())

				opts := x509.VerifyOptions{
					DNSName: cn,
					Roots:   roots,
				}

				_, err = cert.Verify(opts)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("CA is invalid", func() {
			It("should reuse existing CA", func() {
				cp := SelfSignedCertGenerator{}
				certs, err := cp.Generate("foo.example.com")
				Expect(err).NotTo(HaveOccurred())

				cp = SelfSignedCertGenerator{}
				cp.SetCA([]byte("invalidCAKey"), []byte("invalidCACert"))

				certs, err = cp.Generate(cn)
				Expect(err).NotTo(HaveOccurred())

				Expect(certs.CAKey).NotTo(Equal(cp.caKey))
				Expect(certs.CACert).NotTo(Equal(cp.caCert))

				// First, create the set of root certificates. For this example we only
				// have one. It's also possible to omit this in order to use the
				// default root set of the current operating system.
				roots := x509.NewCertPool()
				ok := roots.AppendCertsFromPEM(certs.CACert)
				Expect(ok).To(BeTrue())

				block, _ := pem.Decode(certs.Cert)
				Expect(block).NotTo(BeNil())

				cert, err := x509.ParseCertificate(block.Bytes)
				Expect(err).NotTo(HaveOccurred())

				opts := x509.VerifyOptions{
					DNSName: cn,
					Roots:   roots,
				}

				_, err = cert.Verify(opts)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})
})

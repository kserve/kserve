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
	"io/ioutil"
	"os"
	"path"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"sigs.k8s.io/controller-runtime/pkg/webhook/internal/cert/generator"
	fakegenerator "sigs.k8s.io/controller-runtime/pkg/webhook/internal/cert/generator/fake"
)

var _ = Describe("fsCertWriter", func() {
	dnsName := "test-service.test-svc-namespace.svc"

	var certWriter CertWriter
	var testingDir string
	BeforeEach(func(done Done) {
		var err error
		testingDir, err = ioutil.TempDir("", "testdir")
		Expect(err).NotTo(HaveOccurred())
		certWriter, err = NewFSCertWriter(FSCertWriterOptions{
			CertGenerator: &fakegenerator.CertGenerator{
				DNSNameToCertArtifacts: map[string]*generator.Artifacts{
					dnsName: {
						CACert: []byte(certs2.CACert),
						Cert:   []byte(certs2.Cert),
						Key:    []byte(certs2.Key),
					},
				},
			},
			Path: testingDir,
		})
		Expect(err).NotTo(HaveOccurred())
		close(done)
	})

	AfterEach(func() {
		os.RemoveAll(testingDir)
	})

	Context("Failed to EnsureCert", func() {
		Describe("empty DNS name", func() {
			It("should return error", func() {
				_, _, err := certWriter.EnsureCert("")
				Expect(err).To(MatchError("dnsName should not be empty"))
			})
		})
	})

	Context("Succeeded to EnsureCert", func() {
		Context("CertGenerator is not set", func() {
			It("should default it and return no error", func() {
				_, _, err := certWriter.EnsureCert(dnsName)
				Expect(err).NotTo(HaveOccurred())

			})
		})

		Context("no existing certificate files", func() {
			It("should create new certificate files", func() {
				_, _, err := certWriter.EnsureCert(dnsName)
				Expect(err).NotTo(HaveOccurred())
				caBytes, err := ioutil.ReadFile(path.Join(testingDir, CACertName))
				Expect(err).NotTo(HaveOccurred())
				Expect(caBytes).To(Equal([]byte(certs2.CACert)))
				certBytes, err := ioutil.ReadFile(path.Join(testingDir, ServerCertName))
				Expect(err).NotTo(HaveOccurred())
				Expect(certBytes).To(Equal([]byte(certs2.Cert)))
				keyBytes, err := ioutil.ReadFile(path.Join(testingDir, ServerKeyName))
				Expect(err).NotTo(HaveOccurred())
				Expect(keyBytes).To(Equal([]byte(certs2.Key)))
			})
		})

		Context("old secret exists", func() {
			Context("cert is invalid", func() {
				Describe("cert in secret is incomplete", func() {
					Context("cert file is not a symbolic link", func() {
						BeforeEach(func(done Done) {
							err := ioutil.WriteFile(path.Join(testingDir, CACertName), []byte(`oldCACertBytes`), 0600)
							Expect(err).NotTo(HaveOccurred())
							close(done)
						})

						It("should replace with new certs", func() {
							_, _, err := certWriter.EnsureCert(dnsName)
							Expect(err).NotTo(HaveOccurred())
							caBytes, err := ioutil.ReadFile(path.Join(testingDir, CACertName))
							Expect(err).NotTo(HaveOccurred())
							Expect(caBytes).To(Equal([]byte(certs2.CACert)))
							certBytes, err := ioutil.ReadFile(path.Join(testingDir, ServerCertName))
							Expect(err).NotTo(HaveOccurred())
							Expect(certBytes).To(Equal([]byte(certs2.Cert)))
							keyBytes, err := ioutil.ReadFile(path.Join(testingDir, ServerKeyName))
							Expect(err).NotTo(HaveOccurred())
							Expect(keyBytes).To(Equal([]byte(certs2.Key)))
						})
					})

					Context("cert file is a symbolic link", func() {
						BeforeEach(func(done Done) {
							dataDir := path.Join(testingDir, "..data")
							realDataDir := path.Join(testingDir, "..2018_06_01_15_04_05.12345678")
							caFileName := path.Join(testingDir, "..2018_06_01_15_04_05.12345678", CACertName)
							err := os.Mkdir(realDataDir, 0700)
							Expect(err).NotTo(HaveOccurred())
							err = ioutil.WriteFile(caFileName, []byte(`oldCACertBytes`), 0600)
							Expect(err).NotTo(HaveOccurred())
							err = os.Symlink(realDataDir, dataDir)
							Expect(err).NotTo(HaveOccurred())
							close(done)
						})

						It("should replace with new certs", func() {
							_, _, err := certWriter.EnsureCert(dnsName)
							Expect(err).NotTo(HaveOccurred())
							caBytes, err := ioutil.ReadFile(path.Join(testingDir, CACertName))
							Expect(err).NotTo(HaveOccurred())
							Expect(caBytes).To(Equal([]byte(certs2.CACert)))
							certBytes, err := ioutil.ReadFile(path.Join(testingDir, ServerCertName))
							Expect(err).NotTo(HaveOccurred())
							Expect(certBytes).To(Equal([]byte(certs2.Cert)))
							keyBytes, err := ioutil.ReadFile(path.Join(testingDir, ServerKeyName))
							Expect(err).NotTo(HaveOccurred())
							Expect(keyBytes).To(Equal([]byte(certs2.Key)))
						})
					})
				})

				Describe("cert content is invalid", func() {
					Context("cert files are not symbolic links", func() {
						BeforeEach(func(done Done) {
							ioutil.WriteFile(path.Join(testingDir, CACertName), []byte(`oldCACertBytes`), 0600)
							ioutil.WriteFile(path.Join(testingDir, ServerCertName), []byte(`oldCertBytes`), 0600)
							ioutil.WriteFile(path.Join(testingDir, ServerKeyName), []byte(`oldKeyBytes`), 0600)
							close(done)
						})

						It("should replace with new certs", func() {
							_, _, err := certWriter.EnsureCert(dnsName)
							Expect(err).NotTo(HaveOccurred())
							caBytes, err := ioutil.ReadFile(path.Join(testingDir, CACertName))
							Expect(err).NotTo(HaveOccurred())
							Expect(caBytes).To(Equal([]byte(certs2.CACert)))
							certBytes, err := ioutil.ReadFile(path.Join(testingDir, ServerCertName))
							Expect(err).NotTo(HaveOccurred())
							Expect(certBytes).To(Equal([]byte(certs2.Cert)))
							keyBytes, err := ioutil.ReadFile(path.Join(testingDir, ServerKeyName))
							Expect(err).NotTo(HaveOccurred())
							Expect(keyBytes).To(Equal([]byte(certs2.Key)))
						})
					})

					Context("cert files are symbolic links", func() {
						BeforeEach(func(done Done) {
							dataDir := path.Join(testingDir, "..data")
							realDataDir := path.Join(testingDir, "..2018_06_01_15_04_05.12345678")
							caFileName := path.Join(testingDir, "..2018_06_01_15_04_05.12345678", CACertName)
							certFileName := path.Join(testingDir, "..2018_06_01_15_04_05.12345678", ServerCertName)
							keyFileName := path.Join(testingDir, "..2018_06_01_15_04_05.12345678", ServerKeyName)
							err := os.Mkdir(realDataDir, 0700)
							Expect(err).NotTo(HaveOccurred())
							err = ioutil.WriteFile(caFileName, []byte(`oldCACertBytes`), 0600)
							Expect(err).NotTo(HaveOccurred())
							err = ioutil.WriteFile(certFileName, []byte(`oldCertBytes`), 0600)
							Expect(err).NotTo(HaveOccurred())
							err = ioutil.WriteFile(keyFileName, []byte(`oldKeyBytes`), 0600)
							Expect(err).NotTo(HaveOccurred())
							err = os.Symlink(realDataDir, dataDir)
							Expect(err).NotTo(HaveOccurred())
							close(done)
						})

						It("should replace with new certs", func() {
							_, _, err := certWriter.EnsureCert(dnsName)
							Expect(err).NotTo(HaveOccurred())
							caBytes, err := ioutil.ReadFile(path.Join(testingDir, CACertName))
							Expect(err).NotTo(HaveOccurred())
							Expect(caBytes).To(Equal([]byte(certs2.CACert)))
							certBytes, err := ioutil.ReadFile(path.Join(testingDir, ServerCertName))
							Expect(err).NotTo(HaveOccurred())
							Expect(certBytes).To(Equal([]byte(certs2.Cert)))
							keyBytes, err := ioutil.ReadFile(path.Join(testingDir, ServerKeyName))
							Expect(err).NotTo(HaveOccurred())
							Expect(keyBytes).To(Equal([]byte(certs2.Key)))
						})
					})
				})
			})
		})

		Context("cert is valid", func() {
			Context("when not expiring", func() {
				BeforeEach(func(done Done) {
					err := ioutil.WriteFile(path.Join(testingDir, CACertName), []byte(certs2.CACert), 0600)
					Expect(err).NotTo(HaveOccurred())
					err = ioutil.WriteFile(path.Join(testingDir, ServerCertName), []byte(certs2.Cert), 0600)
					Expect(err).NotTo(HaveOccurred())
					err = ioutil.WriteFile(path.Join(testingDir, ServerKeyName), []byte(certs2.Key), 0600)
					Expect(err).NotTo(HaveOccurred())
					close(done)
				})
				It("should keep the secret", func() {
					_, _, err := certWriter.EnsureCert(dnsName)
					Expect(err).NotTo(HaveOccurred())
					caBytes, err := ioutil.ReadFile(path.Join(testingDir, CACertName))
					Expect(err).NotTo(HaveOccurred())
					Expect(caBytes).To(Equal([]byte(certs2.CACert)))
					certBytes, err := ioutil.ReadFile(path.Join(testingDir, ServerCertName))
					Expect(err).NotTo(HaveOccurred())
					Expect(certBytes).To(Equal([]byte(certs2.Cert)))
					keyBytes, err := ioutil.ReadFile(path.Join(testingDir, ServerKeyName))
					Expect(err).NotTo(HaveOccurred())
					Expect(keyBytes).To(Equal([]byte(certs2.Key)))
				})
			})

			Context("when expiring", func() {
				// TODO: implement this.
				BeforeEach(func(done Done) {
					close(done)
				})

				It("should replace the expiring cert", func() {

				})
			})
		})
	})
})

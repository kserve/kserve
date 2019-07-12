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
	goerrors "errors"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/webhook/internal/cert/generator"
)

var certs1, certs2 *generator.Artifacts

func init() {
	cn1 := "example.com"
	cn2 := "test-service.test-svc-namespace.svc"
	cp1 := generator.SelfSignedCertGenerator{}
	cp2 := generator.SelfSignedCertGenerator{}
	certs1, _ = cp1.Generate(cn1)
	certs2, _ = cp2.Generate(cn2)
}

type fakeCertReadWriter struct {
	numReadCalled  int
	readCertAndErr []certAndErr

	numWriteCalled  int
	writeCertAndErr []certAndErr

	numOverwriteCalled  int
	overwriteCertAndErr []certAndErr
}

type certAndErr struct {
	cert *generator.Artifacts
	err  error
}

var _ certReadWriter = &fakeCertReadWriter{}

func (f *fakeCertReadWriter) read() (*generator.Artifacts, error) {
	defer func() { f.numReadCalled++ }()

	if len(f.readCertAndErr) <= f.numReadCalled {
		return &generator.Artifacts{}, nil
	}
	certAndErr := f.readCertAndErr[f.numReadCalled]
	return certAndErr.cert, certAndErr.err
}

func (f *fakeCertReadWriter) write() (*generator.Artifacts, error) {
	defer func() { f.numWriteCalled++ }()

	if len(f.writeCertAndErr) <= f.numWriteCalled {
		return &generator.Artifacts{}, nil
	}
	certAndErr := f.writeCertAndErr[f.numWriteCalled]
	return certAndErr.cert, certAndErr.err
}

func (f *fakeCertReadWriter) overwrite() (*generator.Artifacts, error) {
	defer func() { f.numOverwriteCalled++ }()

	if len(f.overwriteCertAndErr) <= f.numOverwriteCalled {
		return &generator.Artifacts{}, nil
	}
	certAndErr := f.overwriteCertAndErr[f.numOverwriteCalled]
	return certAndErr.cert, certAndErr.err
}

var _ = Describe("handleCommon", func() {
	var cert *generator.Artifacts
	var invalidCert *generator.Artifacts
	dnsName := "example.com"

	BeforeEach(func(done Done) {
		cert = &generator.Artifacts{
			CACert: []byte(certs1.CACert),
			Cert:   []byte(certs1.Cert),
			Key:    []byte(certs1.Key),
		}
		invalidCert = &generator.Artifacts{
			CACert: []byte(`CACertBytes`),
			Cert:   []byte(`CertBytes`),
			Key:    []byte(`KeyBytes`),
		}
		close(done)
	})

	Context("when DNS name is empty", func() {
		It("should return an error", func() {
			certrw := &fakeCertReadWriter{}
			_, _, err := handleCommon("", certrw)
			Expect(err).To(MatchError("dnsName should not be empty"))
		})
	})

	Context("when certReadWriter is nil", func() {
		It("should return an error", func() {
			_, _, err := handleCommon(dnsName, nil)
			Expect(err).To(MatchError("certReaderWriter should not be nil"))
		})
	})

	Context("cert doesn't exist", func() {
		It("should return no error on successful write", func() {
			certrw := &fakeCertReadWriter{
				readCertAndErr: []certAndErr{
					{
						err: notFoundError{errors.NewNotFound(schema.GroupResource{}, "foo")},
					},
				},
				writeCertAndErr: []certAndErr{
					{
						cert: cert,
					},
				},
			}

			certs, changed, err := handleCommon(dnsName, certrw)
			Expect(err).NotTo(HaveOccurred())
			Expect(certrw.numReadCalled).To(Equal(1))
			Expect(certrw.numWriteCalled).To(Equal(1))
			Expect(certrw.numOverwriteCalled).To(Equal(0))
			Expect(changed).To(BeTrue())
			Expect(certs).To(Equal(cert))
		})

		It("should return the error on failed write", func() {
			certrw := &fakeCertReadWriter{
				readCertAndErr: []certAndErr{
					{
						err: notFoundError{errors.NewNotFound(schema.GroupResource{}, "foo")},
					},
				},
				writeCertAndErr: []certAndErr{
					{
						err: goerrors.New("failed to write"),
					},
				},
			}

			_, _, err := handleCommon(dnsName, certrw)
			Expect(err).To(MatchError("failed to write"))
			Expect(certrw.numReadCalled).To(Equal(1))
			Expect(certrw.numWriteCalled).To(Equal(1))
			Expect(certrw.numOverwriteCalled).To(Equal(0))
		})
	})

	Context("valid cert exist", func() {
		It("should return no error on successful read", func() {
			certrw := &fakeCertReadWriter{
				readCertAndErr: []certAndErr{
					{
						cert: cert,
					},
				},
			}

			certs, changed, err := handleCommon(dnsName, certrw)
			Expect(err).NotTo(HaveOccurred())
			Expect(certrw.numReadCalled).To(Equal(1))
			Expect(certrw.numWriteCalled).To(Equal(0))
			Expect(certrw.numOverwriteCalled).To(Equal(0))
			Expect(changed).To(BeFalse())
			Expect(certs).To(Equal(cert))
		})

		It("should return the error on failed read", func() {
			certrw := &fakeCertReadWriter{
				readCertAndErr: []certAndErr{
					{
						err: goerrors.New("failed to read"),
					},
				},
			}

			_, _, err := handleCommon(dnsName, certrw)
			Expect(err).To(MatchError("failed to read"))
			Expect(certrw.numReadCalled).To(Equal(1))
			Expect(certrw.numWriteCalled).To(Equal(0))
			Expect(certrw.numOverwriteCalled).To(Equal(0))
		})
	})

	Context("invalid cert exist", func() {
		It("should replace the empty cert with a new one", func() {
			certrw := &fakeCertReadWriter{
				readCertAndErr: []certAndErr{
					{
						cert: nil,
					},
				},
				overwriteCertAndErr: []certAndErr{
					{
						cert: cert,
					},
				},
			}

			certs, changed, err := handleCommon(dnsName, certrw)
			Expect(err).NotTo(HaveOccurred())
			Expect(certrw.numReadCalled).To(Equal(1))
			Expect(certrw.numWriteCalled).To(Equal(0))
			Expect(certrw.numOverwriteCalled).To(Equal(1))
			Expect(changed).To(BeTrue())
			Expect(certs).To(Equal(cert))
		})

		It("should return no error on successful overwrite", func() {
			certrw := &fakeCertReadWriter{
				readCertAndErr: []certAndErr{
					{
						cert: invalidCert,
					},
				},
				overwriteCertAndErr: []certAndErr{
					{
						cert: cert,
					},
				},
			}

			certs, changed, err := handleCommon(dnsName, certrw)
			Expect(err).NotTo(HaveOccurred())
			Expect(certrw.numReadCalled).To(Equal(1))
			Expect(certrw.numWriteCalled).To(Equal(0))
			Expect(certrw.numOverwriteCalled).To(Equal(1))
			Expect(changed).To(BeTrue())
			Expect(certs).To(Equal(cert))
		})

		It("should return the error on failed overwrite", func() {
			certrw := &fakeCertReadWriter{
				readCertAndErr: []certAndErr{
					{
						cert: invalidCert,
					},
				},
				overwriteCertAndErr: []certAndErr{
					{
						err: goerrors.New("failed to overwrite"),
					},
				},
			}

			_, _, err := handleCommon(dnsName, certrw)
			Expect(err).To(MatchError("failed to overwrite"))
			Expect(certrw.numReadCalled).To(Equal(1))
			Expect(certrw.numOverwriteCalled).To(Equal(1))
		})
	})

	Context("racing", func() {
		It("should return the valid cert created by the racing one", func() {
			certrw := &fakeCertReadWriter{
				readCertAndErr: []certAndErr{
					{
						err: notFoundError{errors.NewNotFound(schema.GroupResource{}, "foo")},
					},
					{
						cert: cert,
					},
				},
				writeCertAndErr: []certAndErr{
					{
						err: alreadyExistError{errors.NewAlreadyExists(schema.GroupResource{}, "foo")},
					},
				},
			}

			certs, changed, err := handleCommon(dnsName, certrw)
			Expect(err).NotTo(HaveOccurred())
			Expect(certrw.numReadCalled).To(Equal(2))
			Expect(certrw.numWriteCalled).To(Equal(1))
			Expect(changed).To(BeTrue())
			Expect(certs).To(Equal(cert))
		})

		It("should return the error if failed to read the cert created by the racing one", func() {
			certrw := &fakeCertReadWriter{
				readCertAndErr: []certAndErr{
					{
						err: notFoundError{errors.NewNotFound(schema.GroupResource{}, "foo")},
					},
					{
						err: goerrors.New("failed to read"),
					},
				},
				writeCertAndErr: []certAndErr{
					{
						err: alreadyExistError{errors.NewAlreadyExists(schema.GroupResource{}, "foo")},
					},
				},
			}

			_, _, err := handleCommon(dnsName, certrw)
			Expect(err).To(MatchError("failed to read"))
			Expect(certrw.numReadCalled).To(Equal(2))
			Expect(certrw.numWriteCalled).To(Equal(1))
		})
	})
})

var _ = Describe("validate cert", func() {
	Context("invalid pair", func() {
		It("should detect it", func() {
			certs := generator.Artifacts{
				CACert: certs1.CACert,
				Cert:   certs1.Cert,
				Key:    certs2.Key,
			}
			valid := validCert(&certs, "example.com")
			Expect(valid).To(BeFalse())
		})
	})

	Context("CA not matching", func() {
		It("should detect it", func() {
			certs := generator.Artifacts{
				CACert: certs2.CACert,
				Cert:   certs1.Cert,
				Key:    certs1.Key,
			}
			valid := validCert(&certs, "example.com")
			Expect(valid).To(BeFalse())
		})
	})

	Context("DNS name not matching", func() {
		It("should detect it", func() {
			certs := generator.Artifacts{
				CACert: certs1.CACert,
				Cert:   certs1.Cert,
				Key:    certs1.Key,
			}
			valid := validCert(&certs, "foo.com")
			Expect(valid).To(BeFalse())
		})
	})
})

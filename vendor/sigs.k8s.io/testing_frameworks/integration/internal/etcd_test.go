package internal_test

import (
	"net/url"

	. "sigs.k8s.io/testing_frameworks/integration/internal"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Etcd", func() {
	It("defaults Args if they are empty", func() {
		initialArgs := []string{}
		defaultedArgs := DoEtcdArgDefaulting(initialArgs)
		Expect(defaultedArgs).To(BeEquivalentTo(EtcdDefaultArgs))
	})

	It("keeps Args as is if they are not empty", func() {
		initialArgs := []string{"--eins", "--zwei=2"}
		defaultedArgs := DoEtcdArgDefaulting(initialArgs)
		Expect(defaultedArgs).To(BeEquivalentTo([]string{
			"--eins", "--zwei=2",
		}))
	})
})

var _ = Describe("GetEtcdStartMessage()", func() {
	Context("when using a non tls URL", func() {
		It("generates valid start message", func() {
			url := url.URL{
				Scheme: "http",
				Host:   "some.insecure.host:1234",
			}
			message := GetEtcdStartMessage(url)
			Expect(message).To(Equal("serving insecure client requests on some.insecure.host"))
		})
	})
	Context("when using a tls URL", func() {
		It("generates valid start message", func() {
			url := url.URL{
				Scheme: "https",
				Host:   "some.secure.host:8443",
			}
			message := GetEtcdStartMessage(url)
			Expect(message).To(Equal("serving client requests on some.secure.host"))
		})
	})
})

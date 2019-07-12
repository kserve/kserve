package addr_test

import (
	"sigs.k8s.io/testing_frameworks/integration/addr"

	"fmt"
	"net"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("SuggestAddress", func() {
	It("returns a free port and an address to bind to", func() {
		port, host, err := addr.Suggest()

		Expect(err).NotTo(HaveOccurred())
		Expect(host).To(Equal("127.0.0.1"))
		Expect(port).NotTo(Equal(0))

		addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%d", host, port))
		Expect(err).NotTo(HaveOccurred())
		l, err := net.ListenTCP("tcp", addr)
		defer func() {
			Expect(l.Close()).To(Succeed())
		}()
		Expect(err).NotTo(HaveOccurred())
	})
})

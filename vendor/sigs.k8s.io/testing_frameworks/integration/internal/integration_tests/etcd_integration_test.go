package integration_tests

import (
	"bytes"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "sigs.k8s.io/testing_frameworks/integration"
)

var _ = Describe("Etcd", func() {
	It("sets the properties after defaulting", func() {
		etcd := &Etcd{}

		Expect(etcd.URL).To(BeZero())
		Expect(etcd.DataDir).To(BeZero())
		Expect(etcd.Path).To(BeZero())
		Expect(etcd.StartTimeout).To(BeZero())
		Expect(etcd.StopTimeout).To(BeZero())

		Expect(etcd.Start()).To(Succeed())
		defer func() {
			Expect(etcd.Stop()).To(Succeed())
		}()

		Expect(etcd.URL).NotTo(BeZero())
		Expect(etcd.DataDir).NotTo(BeZero())
		Expect(etcd.Path).NotTo(BeZero())
		Expect(etcd.StartTimeout).NotTo(BeZero())
		Expect(etcd.StopTimeout).NotTo(BeZero())
	})

	It("can inspect IO", func() {
		stderr := &bytes.Buffer{}
		etcd := &Etcd{
			Err: stderr,
		}

		Expect(etcd.Start()).To(Succeed())
		defer func() {
			Expect(etcd.Stop()).To(Succeed())
		}()

		Expect(stderr.String()).NotTo(BeEmpty())
	})

	It("can use user specified Args", func() {
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}
		etcd := &Etcd{
			Args:         []string{"--help"},
			Out:          stdout,
			Err:          stderr,
			StartTimeout: 500 * time.Millisecond,
		}

		// it will timeout, as we'll never see the "startup message" we are waiting
		// for on StdErr
		Expect(etcd.Start()).To(MatchError(ContainSubstring("timeout")))

		Expect(stdout.String()).To(ContainSubstring("member flags"))
		Expect(stderr.String()).To(ContainSubstring("usage: etcd"))
	})
})

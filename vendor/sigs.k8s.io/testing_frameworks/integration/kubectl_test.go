package integration_test

import (
	"io/ioutil"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "sigs.k8s.io/testing_frameworks/integration"
)

var _ = Describe("Kubectl", func() {
	It("runs kubectl", func() {
		k := &KubeCtl{Path: "bash"}
		args := []string{"-c", "echo 'something'"}
		stdout, stderr, err := k.Run(args...)
		Expect(err).NotTo(HaveOccurred())
		Expect(stdout).To(ContainSubstring("something"))
		bytes, err := ioutil.ReadAll(stderr)
		Expect(err).NotTo(HaveOccurred())
		Expect(bytes).To(BeEmpty())
	})

	Context("when the command returns a non-zero exit code", func() {
		It("returns an error", func() {
			k := &KubeCtl{Path: "bash"}
			args := []string{
				"-c", "echo 'this is StdErr' >&2; echo 'but this is StdOut' >&1; exit 66",
			}

			stdout, stderr, err := k.Run(args...)

			Expect(err).To(MatchError(ContainSubstring("exit status 66")))

			Expect(stdout).To(ContainSubstring("but this is StdOut"))
			Expect(stderr).To(ContainSubstring("this is StdErr"))
		})
	})
})

package internal_test

import (
	"net/url"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"sigs.k8s.io/testing_frameworks/integration"
	. "sigs.k8s.io/testing_frameworks/integration/internal"
)

var _ = Describe("Arguments", func() {
	It("templates URLs", func() {
		templates := []string{
			"plain URL: {{ .SomeURL }}",
			"method on URL: {{ .SomeURL.Hostname }}",
			"empty URL: {{ .EmptyURL }}",
			"handled empty URL: {{- if .EmptyURL }}{{ .EmptyURL }}{{ end }}",
		}
		data := struct {
			SomeURL  *url.URL
			EmptyURL *url.URL
		}{
			&url.URL{Scheme: "https", Host: "the.host.name:3456"},
			nil,
		}

		out, err := RenderTemplates(templates, data)
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(BeEquivalentTo([]string{
			"plain URL: https://the.host.name:3456",
			"method on URL: the.host.name",
			"empty URL: &lt;nil&gt;",
			"handled empty URL:",
		}))
	})

	It("templates strings", func() {
		templates := []string{
			"a string: {{ .SomeString }}",
			"empty string: {{- .EmptyString }}",
		}
		data := struct {
			SomeString  string
			EmptyString string
		}{
			"this is some random string",
			"",
		}

		out, err := RenderTemplates(templates, data)
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(BeEquivalentTo([]string{
			"a string: this is some random string",
			"empty string:",
		}))
	})

	It("has no access to unexported fields", func() {
		templates := []string{
			"this is just a string",
			"this blows up {{ .test }}",
		}
		data := struct{ test string }{"ooops private"}

		out, err := RenderTemplates(templates, data)
		Expect(out).To(BeEmpty())
		Expect(err).To(MatchError(
			ContainSubstring("is an unexported field of struct"),
		))
	})

	It("errors when field cannot be found", func() {
		templates := []string{"this does {{ .NotExist }}"}
		data := struct{ Unused string }{"unused"}

		out, err := RenderTemplates(templates, data)
		Expect(out).To(BeEmpty())
		Expect(err).To(MatchError(
			ContainSubstring("can't evaluate field"),
		))
	})

	Context("When overriding external default args", func() {
		It("does not change the internal default args for APIServer", func() {
			integration.APIServerDefaultArgs[0] = "oh no!"
			Expect(APIServerDefaultArgs).NotTo(BeEquivalentTo(integration.APIServerDefaultArgs))
		})
		It("does not change the internal default args for Etcd", func() {
			integration.EtcdDefaultArgs[0] = "oh no!"
			Expect(EtcdDefaultArgs).NotTo(BeEquivalentTo(integration.EtcdDefaultArgs))
		})
	})
})

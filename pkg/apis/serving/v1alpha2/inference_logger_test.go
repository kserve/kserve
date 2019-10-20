package v1alpha2

import (
	"github.com/onsi/gomega"
	"testing"
)

func TestInferenceLoggerValidate(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	// OK
	il0 := InferenceLogger{}
	err := validate_inference_logger(&il0)
	g.Expect(err).To(gomega.BeNil())

	url := "http://localhost"
	// OK
	il1 := InferenceLogger{Url: &url}
	err = validate_inference_logger(&il1)
	g.Expect(err).To(gomega.BeNil())

	// Invalid sample
	sampleHigh := float32(1.2)
	il2 := InferenceLogger{Url: &url, Sample: &sampleHigh}
	err = validate_inference_logger(&il2)
	g.Expect(err).NotTo(gomega.BeNil())
	g.Expect(err.Error()).To(gomega.Equal(InvalidInferenceLoggerSample))

}

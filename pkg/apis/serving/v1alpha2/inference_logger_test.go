package v1alpha2

import (
	"github.com/onsi/gomega"
	"testing"
)

func TestInferenceLoggerValidate(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	// No URL provided
	il1 := InferenceLogger{}
	err := validate_inference_logger(&il1)
	g.Expect(err).NotTo(gomega.BeNil())
	g.Expect(err.Error()).To(gomega.Equal(EmptyInferenceLoggerUrl))

	// OK
	il2 := InferenceLogger{Url: "http://localhost"}
	err = validate_inference_logger(&il2)
	g.Expect(err).To(gomega.BeNil())

	// Invalid sample
	sampleHigh := float32(1.2)
	il3 := InferenceLogger{Url: "http://localhost", Sample: &sampleHigh}
	err = validate_inference_logger(&il3)
	g.Expect(err).NotTo(gomega.BeNil())
	g.Expect(err.Error()).To(gomega.Equal(InvalidInferenceLoggerSample))

}

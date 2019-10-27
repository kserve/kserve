package v1alpha2

import (
	"github.com/onsi/gomega"
	"testing"
)

func TestInferenceLoggerValidate(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	// OK
	il0 := Logger{LogType: LogAll}
	err := validate_inference_logger(&il0)
	g.Expect(err).To(gomega.BeNil())

	url := "http://localhost"
	// OK
	il1 := Logger{Url: &url, LogType: LogAll}
	err = validate_inference_logger(&il1)
	g.Expect(err).To(gomega.BeNil())

	// Invalid logger type
	il2 := Logger{Url: &url, LogType: "a"}
	err = validate_inference_logger(&il2)
	g.Expect(err).NotTo(gomega.BeNil())
	g.Expect(err.Error()).To(gomega.Equal(InvalidLoggerType))

}

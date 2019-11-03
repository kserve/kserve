package v1alpha2

import (
	"github.com/onsi/gomega"
	"testing"
)

func TestLoggerValidate(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	// OK
	il0 := Logger{Mode: LogAll}
	err := validate_logger(&il0)
	g.Expect(err).To(gomega.BeNil())

	url := "http://localhost"
	// OK
	il1 := Logger{Url: &url, Mode: LogAll}
	err = validate_logger(&il1)
	g.Expect(err).To(gomega.BeNil())

	// Invalid logger type
	il2 := Logger{Url: &url, Mode: "a"}
	err = validate_logger(&il2)
	g.Expect(err).NotTo(gomega.BeNil())
	g.Expect(err.Error()).To(gomega.Equal(InvalidLoggerType))

}

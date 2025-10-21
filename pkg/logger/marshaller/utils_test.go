package marshaller

import (
	"testing"

	"github.com/onsi/gomega"
)

func TestGetMarshaller(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	jm, err := GetMarshaller(LogStoreFormatJson)
	g.Expect(err).To(gomega.BeNil())
	g.Expect(jm).ToNot(gomega.BeNil())

	cm, err := GetMarshaller(LogStoreFormatCSV)
	g.Expect(err).To(gomega.BeNil())
	g.Expect(cm).ToNot(gomega.BeNil())

	pm, err := GetMarshaller(LogStoreFormatParquet)
	g.Expect(err).To(gomega.BeNil())
	g.Expect(pm).ToNot(gomega.BeNil())

	_, err = GetMarshaller("bad")
	g.Expect(err).ToNot(gomega.BeNil())
}

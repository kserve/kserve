package types

import (
	"fmt"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/onsi/gomega"
	"testing"
)

func TestCreateTFDTypeTypical(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tfDType, err := NewTFDType("tensor_name", "DT_STRING")
	expectedTFDType := DtString
	g.Expect(tfDType).Should(gomega.Equal(expectedTFDType))
	g.Expect(err).Should(gomega.BeNil())
}

func TestCreateTFDTypeB64(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tfDType, err := NewTFDType("tensor_name_bytes", "DT_STRING")
	expectedTFDType := DtB64String
	g.Expect(tfDType).Should(gomega.Equal(expectedTFDType))
	g.Expect(err).Should(gomega.BeNil())
}

func TestCreateTFDTypeUnsupported(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	_, err := NewTFDType("tensor_name", "BAD TYPE")
	expectedErr := fmt.Sprintf(UnsupportedDataTypeError, "tensor_name", "BAD TYPE")
	g.Expect(err).Should(gomega.MatchError(expectedErr))
}

func TestTFDTypeSchemaTypical(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tfDType := DtString
	schema := tfDType.Schema()
	expectedSchema := openapi3.NewStringSchema()
	g.Expect(schema).Should(gomega.Equal(expectedSchema))
}

func TestTFDTypeSchemaB64(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	tfDType := DtB64String
	schema := tfDType.Schema()
	expectedSchema := &openapi3.Schema{
		Type: "object",
		Properties: map[string]*openapi3.SchemaRef{
			"b64": {
				Value: &openapi3.Schema{
					Type: "string",
				},
			},
		},
	}
	g.Expect(schema).Should(gomega.Equal(expectedSchema))
}

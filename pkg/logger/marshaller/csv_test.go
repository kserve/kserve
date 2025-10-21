package marshaller

import (
	"strings"
	"testing"

	"github.com/kserve/kserve/pkg/logger/types"
	"github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
)

const (
	CEInferenceRequest  = "org.kubeflow.serving.inference.request"
	CEInferenceResponse = "org.kubeflow.serving.inference.response"
)

func TestCSVMarshalling(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	marshaller := &CSVMarshaller{}
	bytes, err := marshaller.Marshal([]types.LogRequest{
		{
			Id:               "0123",
			Namespace:        "ns",
			InferenceService: "inference",
			Component:        "predictor",
			ReqType:          CEInferenceRequest,
		},
		{
			Id:               "1234",
			Namespace:        "ns",
			InferenceService: "inference",
			Component:        "predictor",
			ReqType:          CEInferenceRequest,
		},
	})
	g.Expect(err).ToNot(gomega.HaveOccurred())
	assert.Greater(t, len(bytes), 0, "marshalled byte array is empty")
	csvBody := string(bytes)
	lines := strings.Split(csvBody, "\n")
	assert.Equal(t, 4, len(lines))
	assert.Equal(t, "Url.Scheme,Url.Opaque,Url.Host,Url.Path,Url.RawPath,Url.OmitHost,Url.ForceQuery,Url.RawQuery,Url.Fragment,Url.RawFragment,Bytes,ContentType,ReqType,Id,SourceUri.Scheme,SourceUri.Opaque,SourceUri.Host,SourceUri.Path,SourceUri.RawPath,SourceUri.OmitHost,SourceUri.ForceQuery,SourceUri.RawQuery,SourceUri.Fragment,SourceUri.RawFragment,InferenceService,Namespace,Component,Endpoint,Metadata,Annotations,CertName,TlsSkipVerify", lines[0])
	assert.Equal(t, ",,,,,,,,,,,,org.kubeflow.serving.inference.request,0123,,,,,,,,,,,inference,ns,predictor,,,,,false", lines[1])
	assert.Equal(t, ",,,,,,,,,,,,org.kubeflow.serving.inference.request,1234,,,,,,,,,,,inference,ns,predictor,,,,,false", lines[2])
}

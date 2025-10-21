package marshaller

import (
	"log"
	"os"
	"testing"

	"github.com/kserve/kserve/pkg/logger/types"
	"github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	"github.com/xitongsys/parquet-go-source/local"
	"github.com/xitongsys/parquet-go/reader"
)

func TestParquetMarshalling(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	marshaller := ParquetMarshaller{}
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

	err = os.WriteFile("/tmp/test.parquet", bytes, 0644)
	if err != nil {
		// log.Fatalf will print the error message and exit the program
		log.Fatalf("Error writing file: %v", err)
	}

	fr, err := local.NewLocalFileReader("/tmp/test.parquet")
	g.Expect(err).ToNot(gomega.HaveOccurred())

	parquetReader, err := reader.NewParquetReader(fr, new(ParquetLogRequest), 1)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	g.Expect(parquetReader).ToNot(gomega.BeNil())
	rows := parquetReader.GetNumRows()
	g.Expect(rows).To(gomega.Equal(int64(2)))
	items := make([]ParquetLogRequest, 2)
	err = parquetReader.Read(&items)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	g.Expect(*items[0].Id).To(gomega.Equal("0123"))
	g.Expect(*items[1].Id).To(gomega.Equal("1234"))
	parquetReader.ReadStop()
	fr.Close()
}

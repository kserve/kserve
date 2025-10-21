/*
Copyright 2021 The KServe Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package marshaller

import (
	"io"
	"sync"

	"github.com/xitongsys/parquet-go-source/mem"
	"github.com/xitongsys/parquet-go/parquet"
	"github.com/xitongsys/parquet-go/writer"
)

const LogStoreFormatParquet = "parquet"

type ParquetMarshaller struct{}

func (p *ParquetMarshaller) Marshal(v []interface{}) ([]byte, error) {
	var wg sync.WaitGroup
	buffer := make([]byte, 0)
	wg.Add(1)
	var readErr error
	memFile, err := mem.NewMemFileWriter("temp.parquet", func(name string, reader io.Reader) error {
		defer wg.Done()
		_, rErr := reader.Read(buffer)
		if rErr != nil {
			readErr = rErr
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if readErr != nil {
		return nil, readErr
	}

	pw, err := writer.NewParquetWriter(memFile, v, 4)
	if err != nil {
		return nil, err
	}
	pw.RowGroupSize = 128 * 1024 * 1024
	pw.CompressionType = parquet.CompressionCodec_SNAPPY

	for _, record := range v {
		if err := pw.Write(record); err != nil {
			return nil, err
		}
	}

	if err := pw.WriteStop(); err != nil {
		return nil, err
	}

	err = memFile.Close()
	if err != nil {
		return nil, err
	}
	wg.Wait()
	return buffer, nil
}

var _ Marshaller = &ParquetMarshaller{}

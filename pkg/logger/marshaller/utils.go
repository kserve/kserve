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

import "fmt"

var registeredMarshallers = map[string]Marshaller{
	LogStoreFormatJson:    &JSONMarshaller{},
	LogStoreFormatCSV:     &CSVMarshaller{},
	LogStoreFormatParquet: &ParquetMarshaller{},
}

func RegisterMarshaller(logStoreFormat string, marshaller Marshaller) {
	registeredMarshallers[logStoreFormat] = marshaller
}

func GetMarshaller(logStoreFormat string) (Marshaller, error) {
	if m, ok := registeredMarshallers[logStoreFormat]; ok {
		return m, nil
	}
	return nil, fmt.Errorf("unknown log store format: %s", logStoreFormat)
}

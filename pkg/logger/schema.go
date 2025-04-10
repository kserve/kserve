/*
Copyright 2023 The KServe Authors.

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

package logger

import (
	"encoding/json"
	"fmt"
	"reflect"
	"time"
)

type Field struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Location string `json:"location"`
	Key      string `json:"key"`
}

type Schema struct {
	Fields []Field `json:"fields"`
	Format string  `json:"format"`
}

const (
	schemaAnnotationKey     = "annotation"
	schemaMetadataHeaderKey = "metadata"
	schemaDefaultLoggerKey  = "logger"
)

// loads the designated schema structure into a compatible go struct
func LoadSchema(data []byte) (*Schema, error) {

	var schema Schema
	if err := json.Unmarshal(data, &schema); err != nil {
		return nil, fmt.Errorf("invalid schema JSON: %v", err)
	}
	return &schema, nil
}

func GeneratePayload(logReq LogRequest) ([]byte, error) {
	s := *PayloadSchema
	fmt.Println(logReq)
	fmt.Println("---------")
	fmt.Println(s)
	fmt.Println("==========")
	ret := map[string]interface{}{}
	val := reflect.ValueOf(logReq)
	for _, f := range s.Fields {
		switch f.Location {
		// case schemaAnnotationKey:
		// 	ret[f.Name] = logReq.Annotations[f.Key]
		case schemaMetadataHeaderKey:
			ret[f.Name] = logReq.Metadata[f.Key]
		case schemaDefaultLoggerKey:
			value := val.FieldByName(f.Key)
			if value.IsValid() {
				switch f.Type {
				case "string":
					ret[f.Name] = value.String()
				case "int":
					ret[f.Name] = value.Int()
				case "bool":
					ret[f.Name] = value.Bool()
				case "time":
					ret[f.Name] = value.Interface().(time.Time).String()
				default:
					ret[f.Name] = value.Interface()
				}
			}
		}
	}
	ret["payload"] = string(*logReq.Bytes)
	out, err := json.Marshal(ret)
	if err != nil {
		return nil, err
	}
	return out, nil
}

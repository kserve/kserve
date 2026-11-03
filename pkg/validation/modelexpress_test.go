/*
Copyright 2026 The KServe Authors.

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

package validation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/kserve/kserve/pkg/constants"
)

func TestModelExpressModeFromAnnotations(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		want        ModelExpressMode
		wantErr     bool
	}{
		{name: "nil annotations disable", annotations: nil, want: ""},
		{name: "absent key disables", annotations: map[string]string{"foo": "bar"}, want: ""},
		{name: "layered", annotations: map[string]string{constants.ModelExpressModeAnnotationKey: "layered"}, want: ModelExpressModeLayered},
		{name: "native", annotations: map[string]string{constants.ModelExpressModeAnnotationKey: "native"}, want: ModelExpressModeNative},
		{name: "surrounding whitespace trimmed", annotations: map[string]string{constants.ModelExpressModeAnnotationKey: " native\n"}, want: ModelExpressModeNative},
		{name: "empty rejected", annotations: map[string]string{constants.ModelExpressModeAnnotationKey: ""}, wantErr: true},
		{name: "case sensitive", annotations: map[string]string{constants.ModelExpressModeAnnotationKey: "Native"}, wantErr: true},
		{name: "unknown rejected", annotations: map[string]string{constants.ModelExpressModeAnnotationKey: "p2p"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ModelExpressModeFromAnnotations(tt.annotations)
			if tt.wantErr {
				require.Error(t, err)
				assert.Empty(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestValidateModelExpressAddress(t *testing.T) {
	valid := []string{
		"modelexpress.mx.svc:8001",
		"http://modelexpress.mx.svc:8001",
		"https://modelexpress.mx.svc:8001",
		"10.0.0.1:8001",
		"[fd00::1]:8001",
		"localhost:1",
		"localhost:65535",
	}
	for _, addr := range valid {
		t.Run("valid "+addr, func(t *testing.T) {
			assert.NoError(t, ValidateModelExpressAddress(addr))
		})
	}

	invalid := []string{
		"",
		"modelexpress",
		"grpc://modelexpress:8001",
		"dns:///modelexpress:8001",
		"https://modelexpress",
		":8001",
		"http://:8001",
		"modelexpress:0",
		"modelexpress:65536",
		"modelexpress:port",
		"modelexpress:8001/path",
	}
	for _, addr := range invalid {
		t.Run("invalid "+addr, func(t *testing.T) {
			assert.Error(t, ValidateModelExpressAddress(addr))
		})
	}
}

func TestValidateModelExpressAnnotations(t *testing.T) {
	annotationsPath := field.NewPath("metadata").Child("annotations")
	modeField := annotationsPath.Key(constants.ModelExpressModeAnnotationKey).String()
	addressField := annotationsPath.Key(constants.ModelExpressAddressAnnotationKey).String()
	audienceField := annotationsPath.Key(constants.ModelExpressTokenAudienceAnnotationKey).String()
	revisionField := annotationsPath.Key(constants.ModelExpressRevisionAnnotationKey).String()

	tests := []struct {
		name        string
		annotations map[string]string
		wantFields  []string
		wantTypes   []field.ErrorType
	}{
		{name: "nil", annotations: nil},
		{name: "unrelated annotations", annotations: map[string]string{"foo": "bar"}},
		{
			name: "fully specified native",
			annotations: map[string]string{
				constants.ModelExpressModeAnnotationKey:          "native",
				constants.ModelExpressAddressAnnotationKey:       "https://mx.mx.svc:8001",
				constants.ModelExpressTokenAudienceAnnotationKey: "modelexpress",
				constants.ModelExpressRevisionAnnotationKey:      "2026-09-01",
			},
		},
		{
			name:        "mode alone is valid",
			annotations: map[string]string{constants.ModelExpressModeAnnotationKey: "layered"},
		},
		{
			name:        "invalid mode",
			annotations: map[string]string{constants.ModelExpressModeAnnotationKey: "turbo"},
			wantFields:  []string{modeField},
			wantTypes:   []field.ErrorType{field.ErrorTypeNotSupported},
		},
		{
			name:        "address without mode",
			annotations: map[string]string{constants.ModelExpressAddressAnnotationKey: "mx:8001"},
			wantFields:  []string{modeField},
			wantTypes:   []field.ErrorType{field.ErrorTypeRequired},
		},
		{
			name: "every option without mode reports each",
			annotations: map[string]string{
				constants.ModelExpressAddressAnnotationKey:       "mx:8001",
				constants.ModelExpressTokenAudienceAnnotationKey: "modelexpress",
				constants.ModelExpressRevisionAnnotationKey:      "r1",
			},
			wantFields: []string{modeField, modeField, modeField},
			wantTypes:  []field.ErrorType{field.ErrorTypeRequired, field.ErrorTypeRequired, field.ErrorTypeRequired},
		},
		{
			name: "grpc address rejected",
			annotations: map[string]string{
				constants.ModelExpressModeAnnotationKey:    "native",
				constants.ModelExpressAddressAnnotationKey: "grpc://mx:8001",
			},
			wantFields: []string{addressField},
			wantTypes:  []field.ErrorType{field.ErrorTypeInvalid},
		},
		{
			name: "address whitespace trimmed",
			annotations: map[string]string{
				constants.ModelExpressModeAnnotationKey:    "native",
				constants.ModelExpressAddressAnnotationKey: " mx:8001 ",
			},
		},
		{
			name: "empty audience and revision rejected",
			annotations: map[string]string{
				constants.ModelExpressModeAnnotationKey:          "layered",
				constants.ModelExpressTokenAudienceAnnotationKey: " ",
				constants.ModelExpressRevisionAnnotationKey:      "",
			},
			wantFields: []string{audienceField, revisionField},
			wantTypes:  []field.ErrorType{field.ErrorTypeInvalid, field.ErrorTypeInvalid},
		},
		{
			name: "invalid mode and address both reported",
			annotations: map[string]string{
				constants.ModelExpressModeAnnotationKey:    "turbo",
				constants.ModelExpressAddressAnnotationKey: "mx",
			},
			wantFields: []string{modeField, addressField},
			wantTypes:  []field.ErrorType{field.ErrorTypeNotSupported, field.ErrorTypeInvalid},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateModelExpressAnnotations(tt.annotations)
			require.Len(t, errs, len(tt.wantFields), "errors: %v", errs)
			for i, err := range errs {
				assert.Equal(t, tt.wantFields[i], err.Field)
				assert.Equal(t, tt.wantTypes[i], err.Type)
			}
		})
	}
}

func TestValidateModelExpressSource(t *testing.T) {
	tests := []struct {
		name     string
		mode     ModelExpressMode
		modelURI string
		loraURIs []string
		wantErr  string
	}{
		{name: "disabled accepts anything", mode: "", modelURI: "oci://registry/model:tag"},
		{name: "layered accepts pvc", mode: ModelExpressModeLayered, modelURI: "pvc://models/llama"},
		{name: "layered accepts oci", mode: ModelExpressModeLayered, modelURI: "oci://registry/model:tag"},
		{name: "layered accepts s3 with remote adapters", mode: ModelExpressModeLayered, modelURI: "s3://b/m", loraURIs: []string{"hf://org/adapter"}},
		{name: "native s3", mode: ModelExpressModeNative, modelURI: "s3://bucket/llama"},
		{name: "native s3 with pvc adapters", mode: ModelExpressModeNative, modelURI: "s3://bucket/llama", loraURIs: []string{"pvc://adapters/a", "pvc://adapters/b"}},
		{name: "native hf", mode: ModelExpressModeNative, modelURI: "hf://meta-llama/Llama-3.3-70B-Instruct"},
		{name: "native hf with remote adapters", mode: ModelExpressModeNative, modelURI: "hf://org/model", loraURIs: []string{"hf://org/a", "s3://b/a"}},
		{name: "native s3 with hf adapter", mode: ModelExpressModeNative, modelURI: "s3://bucket/llama", loraURIs: []string{"pvc://adapters/a", "hf://org/a"}, wantErr: `only pvc:// LoRA adapters, got "hf://org/a"`},
		{name: "native s3 with s3 adapter", mode: ModelExpressModeNative, modelURI: "s3://bucket/llama", loraURIs: []string{"s3://bucket/a"}, wantErr: `got "s3://bucket/a"`},
		{name: "native pvc", mode: ModelExpressModeNative, modelURI: "pvc://models/llama", wantErr: "supports s3:// and hf:// models"},
		{name: "native oci", mode: ModelExpressModeNative, modelURI: "oci://registry/model:tag", wantErr: "use layered"},
		{name: "native oci+native", mode: ModelExpressModeNative, modelURI: "oci+native://registry/model:tag", wantErr: "supports s3:// and hf:// models"},
		{name: "native gs", mode: ModelExpressModeNative, modelURI: "gs://bucket/model", wantErr: "supports s3:// and hf:// models"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateModelExpressSource(tt.mode, tt.modelURI, tt.loraURIs)
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

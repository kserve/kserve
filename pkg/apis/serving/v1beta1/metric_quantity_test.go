/*
Copyright 2025 The KServe Authors.

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

package v1beta1

import (
	"encoding/json"
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
)

func TestMetricQuantity_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name             string
		input            string
		expectedRaw      string
		expectedQuantity string
		wantErr          bool
	}{
		{
			name:             "Decimal string value",
			input:            `"0.6"`,
			expectedRaw:      "0.6",
			expectedQuantity: "600m",
			wantErr:          false,
		},
		{
			name:             "Integer string value",
			input:            `"5"`,
			expectedRaw:      "5",
			expectedQuantity: "5",
			wantErr:          false,
		},
		{
			name:             "Memory value with unit",
			input:            `"50Gi"`,
			expectedRaw:      "50Gi",
			expectedQuantity: "50Gi",
			wantErr:          false,
		},
		{
			name:             "Milli value",
			input:            `"500m"`,
			expectedRaw:      "500m",
			expectedQuantity: "500m",
			wantErr:          false,
		},
		{
			name:             "Integer without quotes",
			input:            `100`,
			expectedRaw:      "100",
			expectedQuantity: "100",
			wantErr:          false,
		},
		{
			name:             "Zero value",
			input:            `"0"`,
			expectedRaw:      "0",
			expectedQuantity: "0",
			wantErr:          false,
		},
		{
			name:             "Scientific notation",
			input:            `"1e3"`,
			expectedRaw:      "1e3",
			expectedQuantity: "1e3",
			wantErr:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mq MetricQuantity
			err := json.Unmarshal([]byte(tt.input), &mq)

			if (err != nil) != tt.wantErr {
				t.Errorf("MetricQuantity.UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if mq.GetOriginal() != tt.expectedRaw {
					t.Errorf("MetricQuantity.GetOriginal() = %v, expected %v", mq.GetOriginal(), tt.expectedRaw)
				}

				quantity := mq.GetQuantity()
				if quantity.String() != tt.expectedQuantity {
					t.Errorf("MetricQuantity.GetQuantity().String() = %v, expected %v", quantity.String(), tt.expectedQuantity)
				}
			}
		})
	}
}

func TestMetricQuantity_MarshalJSON(t *testing.T) {
	tests := []struct {
		name        string
		mq          MetricQuantity
		expectedOut string
		wantErr     bool
	}{
		{
			name: "Decimal value marshaling",
			mq: MetricQuantity{
				Original: "0.6",
				Quantity: resource.MustParse("600m"),
			},
			expectedOut: `"0.6"`,
			wantErr:     false,
		},
		{
			name: "Memory value marshaling",
			mq: MetricQuantity{
				Original: "50Gi",
				Quantity: resource.MustParse("50Gi"),
			},
			expectedOut: `"50Gi"`,
			wantErr:     false,
		},
		{
			name: "Integer value marshaling",
			mq: MetricQuantity{
				Original: "5",
				Quantity: resource.MustParse("5"),
			},
			expectedOut: `"5"`,
			wantErr:     false,
		},
		{
			name: "Empty raw fallback to quantity",
			mq: MetricQuantity{
				Original: "",
				Quantity: resource.MustParse("500m"),
			},
			expectedOut: `"500m"`,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.mq)
			if (err != nil) != tt.wantErr {
				t.Errorf("MetricQuantity.MarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && string(got) != tt.expectedOut {
				t.Errorf("MetricQuantity.MarshalJSON() = %v, expected %v", string(got), tt.expectedOut)
			}
		})
	}
}

func TestMetricQuantity_GetMethods(t *testing.T) {
	mq := &MetricQuantity{
		Original: "0.5",
		Quantity: resource.MustParse("500m"),
	}

	// Test GetOriginal
	if mq.GetOriginal() != "0.5" {
		t.Errorf("GetOriginal() = %v, expected %v", mq.GetOriginal(), "0.5")
	}

	// Test GetQuantity
	expectedQuantity := resource.MustParse("500m")
	if !mq.GetQuantity().Equal(expectedQuantity) {
		t.Errorf("GetQuantity() = %v, expected %v", mq.GetQuantity(), expectedQuantity)
	}

	// Test nil pointer
	var nilMQ *MetricQuantity
	nilQuantity := nilMQ.GetQuantity()
	if nilQuantity.String() != "0" {
		t.Errorf("nil MetricQuantity GetQuantity() should return zero quantity")
	}
}

func TestMetricQuantity_RoundTrip(t *testing.T) {
	tests := []string{
		`"0.6"`,
		`"50Gi"`,
		`"500m"`,
		`"10"`,
		`100`,
	}

	for _, input := range tests {
		t.Run("RoundTrip_"+input, func(t *testing.T) {
			// Unmarshal
			var mq MetricQuantity
			err := json.Unmarshal([]byte(input), &mq)
			if err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}

			// Marshal back
			output, err := json.Marshal(mq)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}

			// The output should be a valid quantity representation
			// (it may differ from input due to normalization by resource.Quantity)
			var checkMQ MetricQuantity
			err = json.Unmarshal(output, &checkMQ)
			if err != nil {
				t.Fatalf("Re-unmarshal failed: %v", err)
			}

			// The quantities should be equivalent
			origQuantity := mq.GetQuantity()
			finalQuantity := checkMQ.GetQuantity()
			if !origQuantity.Equal(finalQuantity) {
				t.Errorf("Round-trip quantity mismatch: original %v, final %v",
					origQuantity, finalQuantity)
			}
		})
	}
}

func TestMetricQuantity_Equal(t *testing.T) {
	mq1 := &MetricQuantity{Original: "0.5", Quantity: resource.MustParse("500m")}
	mq2 := &MetricQuantity{Original: "500m", Quantity: resource.MustParse("500m")}
	mq3 := &MetricQuantity{Original: "1", Quantity: resource.MustParse("1")}

	// Equal quantities (different raw values)
	if !mq1.GetQuantity().Equal(mq2.GetQuantity()) {
		t.Error("mq1 and mq2 should be equal (same quantity)")
	}

	// Different quantities
	if mq1.GetQuantity().Equal(mq3.GetQuantity()) {
		t.Error("mq1 and mq3 should not be equal (different quantities)")
	}
}

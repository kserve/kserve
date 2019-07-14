/*
Copyright 2018 The Knative Authors

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
package activator

import (
	"testing"

	net "github.com/knative/serving/pkg/apis/networking"
)

func TestServicePort(t *testing.T) {
	tests := []struct {
		name     string
		protocol net.ProtocolType
		port     int32
	}{{
		name:     "h2c",
		protocol: net.ProtocolH2C,
		port:     ServicePortH2C,
	}, {
		name:     "http1",
		protocol: net.ProtocolHTTP1,
		port:     ServicePortHTTP1,
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := tt.port
			got := ServicePort(tt.protocol)

			if want != got {
				t.Errorf("unexpected port. want %d, got %d", want, got)
			}
		})
	}
}

func TestRevIDString(t *testing.T) {
	if got, want := (&RevisionID{
		Namespace: "bulk",
		Name:      "goods",
	}).String(), "bulk/goods"; got != want {
		t.Errorf("RevID.String = %q, want: %q", got, want)
	}
}

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

package types

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// cert config validation (via SecurityConfig.Validate with mode=cert).
func TestCertConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cert    CertConfig
		wantErr bool
	}{
		{"valid", CertConfig{TrustBundle: "kserve/ca", SubjectRegexp: ".*@kserve"}, false},
		{"empty trustBundle", CertConfig{SubjectRegexp: ".*"}, true},
		{"empty subjectRegexp", CertConfig{TrustBundle: "kserve/ca"}, true},
		{"invalid subjectRegexp", CertConfig{TrustBundle: "kserve/ca", SubjectRegexp: "([a-z"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := SecurityConfig{Mode: ModeCert, FailurePolicy: FailurePolicyReject, Cert: tt.cert}
			err := cfg.Validate()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

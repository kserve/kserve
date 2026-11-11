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
	"errors"
	"strings"
)

// ModeKeyless verifies signatures using public Sigstore keyless signing.
const ModeKeyless Mode = "keyless"

// KeylessConfig configures the expected public Sigstore signing identity.
type KeylessConfig struct {
	// Subject is the expected certificate identity.
	Subject string `json:"subject,omitempty"`

	// Issuer is the expected OIDC issuer.
	Issuer string `json:"issuer,omitempty"`
}

// Validate requires an exact subject and OIDC issuer.
func (c KeylessConfig) Validate() error {
	if strings.TrimSpace(c.Subject) == "" {
		return errors.New("keyless.subject must be set")
	}
	if strings.TrimSpace(c.Issuer) == "" {
		return errors.New("keyless.issuer must be set")
	}
	return nil
}

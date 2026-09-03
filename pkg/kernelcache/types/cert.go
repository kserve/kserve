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
	"fmt"
	"regexp"
)

// ModeCert verifies a signature made with a cert-manager issued key by checking
// the X.509 chain against a CA bundle plus a signer identity.
const ModeCert Mode = "cert"

// DefaultTrustBundleKey is the data key read from the trust bundle when
// CertConfig.TrustBundleKey is empty.
const DefaultTrustBundleKey = "ca.crt"

// CertConfig configures the cert mode.
type CertConfig struct {
	// TrustBundle references the Secret or ConfigMap ("namespace/name") holding
	// the CA certificate(s) used to verify the signing chain. Both a Secret and
	// a ConfigMap of that name may exist; the Secret takes precedence. A bundle
	// may concatenate multiple PEM certs (e.g. root + intermediate, or several
	// CAs for rotation); all are trusted as anchors.
	TrustBundle string `json:"trustBundle"`

	// TrustBundleKey is the data key under which the CA PEM is stored in the
	// trust bundle. Defaults to "ca.crt"; set it for bundles that use another
	// name (e.g. "cabundle.pem", "ca-int.crt").
	TrustBundleKey string `json:"trustBundleKey,omitempty"`

	// SubjectRegexp matches the accepted signer identity (the leaf certificate
	// SAN). Required: without it any cert chaining to the CA would be accepted.
	SubjectRegexp string `json:"subjectRegexp"`
}

func (c CertConfig) validate() error {
	if c.TrustBundle == "" {
		return errors.New("cert.trustBundle must be set")
	}
	if c.SubjectRegexp == "" {
		return errors.New("cert.subjectRegexp must be set")
	}
	if _, err := regexp.Compile(c.SubjectRegexp); err != nil {
		return fmt.Errorf("cert.subjectRegexp is not a valid regexp: %w", err)
	}
	return nil
}

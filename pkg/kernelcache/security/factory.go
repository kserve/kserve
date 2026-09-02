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

package security

import (
	"context"
	"fmt"
)

// NewVerifier builds the Verifier for the configured mode. The config is
// defaulted and validated first. ctx bounds any construction-time I/O (such as
// loading a trust bundle); src provides key and trust material to the mode
// implementations and is unused by the disabled mode.
func NewVerifier(ctx context.Context, cfg SecurityConfig, src SecretSource) (Verifier, error) {
	cfg.Default()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	switch cfg.Mode {
	case ModeDisabled:
		return noopVerifier{}, nil
	case ModeCert:
		// Assign through a concrete error check so a nil *certVerifier is never
		// wrapped in a non-nil Verifier interface (typed-nil trap).
		cv, err := newCertVerifier(ctx, cfg.Cert, src)
		if err != nil {
			return nil, err
		}
		return cv, nil
	default:
		// Unreachable after Validate, kept as a defensive guard.
		return nil, fmt.Errorf("%w: %q", ErrUnknownMode, cfg.Mode)
	}
}

// noopVerifier performs no signature check. It backs the disabled mode.
type noopVerifier struct{}

var _ Verifier = noopVerifier{}

// Verify reports the image as not verified without contacting any registry.
// Digest resolution for the disabled mode is added together with the shared
// registry helper used by the real verifiers.
func (noopVerifier) Verify(_ context.Context, _ VerifyRequest) (VerifyResult, error) {
	return VerifyResult{
		Mode:     ModeDisabled,
		Verified: false,
		Reason:   "verification disabled",
	}, nil
}

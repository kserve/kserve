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

// NewSigner builds the Signer for the configured mode, mirroring NewVerifier.
// The config is defaulted and validated first; ctx bounds any construction-time
// I/O and src provides key material to the signing modes. Only the disabled
// mode is wired here -- the signing modes (cert, keyless) add their own cases.
func NewSigner(ctx context.Context, cfg SecurityConfig, src SecretSource) (Signer, error) {
	cfg.Default()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	switch cfg.Mode {
	case ModeDisabled:
		return noopSigner{}, nil
	default:
		// Validate accepts every known mode, so a mode reaching here is valid
		// but has no signer wired yet (cert, keyless) or never signs (kyverno).
		return nil, fmt.Errorf("signing not implemented for mode %q", cfg.Mode)
	}
}

// noopSigner signs nothing. It backs the disabled mode.
type noopSigner struct{}

var _ Signer = noopSigner{}

// Sign is a no-op that reports the disabled mode without contacting a registry.
func (noopSigner) Sign(_ context.Context, _ SignRequest) (SignResult, error) {
	return SignResult{Mode: ModeDisabled}, nil
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

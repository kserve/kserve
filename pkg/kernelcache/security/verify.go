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

import "context"

// VerifyRequest carries the inputs for a verification. It is a struct rather
// than a bare image reference so new inputs (for example, delegated-mode
// annotations) can be added without changing the Verifier interface.
type VerifyRequest struct {
	// ImageRef is the image to verify (tag or digest reference).
	ImageRef string

	// Annotations carries out-of-band metadata a mode may need. The kyverno
	// mode reads the policy-engine verification result from here; other modes
	// ignore it.
	Annotations map[string]string
}

// VerifyResult is the outcome of a completed verification attempt. Digest is
// populated whenever the reference could be resolved, even when Verified is
// false, so callers can still pin an immutable reference under a warn policy.
type VerifyResult struct {
	// Digest is the resolved sha256 digest of the image.
	Digest string

	// Verified reports whether the signature check passed.
	Verified bool

	// Mode is the mode that produced this result.
	Mode Mode

	// Reason is a human-readable explanation, used for status and logging.
	Reason string
}

// ShouldBlock reports whether a consumer must block use of the image given a
// failure policy. It centralises the allow/block decision so callers do not
// reimplement the subtle cases (in particular, disabled mode is a deliberate
// skip, not a failure, and must never block).
//
// Rules:
//   - disabled mode: never blocks (verification was intentionally skipped).
//   - verified: never blocks.
//   - not verified: blocks only under FailurePolicyReject.
//
// Operational errors (a nil, non-completed result) are handled by the caller
// separately and are outside this decision.
func (r VerifyResult) ShouldBlock(policy FailurePolicy) bool {
	if r.Mode == ModeDisabled || r.Verified {
		return false
	}
	return policy == FailurePolicyReject
}

// Verifier verifies a kernel cache image according to a configured mode.
//
// The returned error is reserved for operational failures (network, registry,
// misconfiguration) that a caller may retry. A completed check that fails is
// reported as VerifyResult{Verified: false} with a nil error, so the caller can
// apply its FailurePolicy uniformly.
type Verifier interface {
	Verify(ctx context.Context, req VerifyRequest) (VerifyResult, error)
}

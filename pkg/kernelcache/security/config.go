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

// Package security defines the contracts for verifying kernel cache OCI images.
// It exposes a mode-agnostic Verifier interface plus the configuration that
// selects a verification mode. Only the disabled mode exists here; each real
// mode (and any config it needs) is added together with its implementation.
package security

import (
	"errors"
	"fmt"
)

// Mode selects how kernel cache images are verified.
type Mode string

const (
	// ModeDisabled performs no signature verification.
	ModeDisabled Mode = "disabled"
)

// FailurePolicy decides what a consumer does when verification fails. The
// policy is applied by the caller (via VerifyResult.ShouldBlock), not by the
// Verifier, so the same result can be handled uniformly across modes.
type FailurePolicy string

const (
	// FailurePolicyReject blocks use of an image that fails verification.
	FailurePolicyReject FailurePolicy = "reject"

	// FailurePolicyWarn allows use after logging a warning. Intended for
	// migration or development, not production.
	FailurePolicyWarn FailurePolicy = "warn"
)

// SecurityConfig is the cluster-level security configuration. It mirrors the
// "security" block of the "kernelcache" section in the inferenceservice-config
// ConfigMap; the json tags are the wire contract and must match that schema.
// Mode-specific sub-configuration is added with each mode.
type SecurityConfig struct {
	// Mode selects the verification mechanism.
	Mode Mode `json:"mode,omitempty"`

	// FailurePolicy controls consumer behaviour on verification failure.
	FailurePolicy FailurePolicy `json:"failurePolicy,omitempty"`
}

// ErrUnknownMode is returned when the configured mode is not recognised.
var ErrUnknownMode = errors.New("unknown verification mode")

// Default fills empty fields with safe defaults. An unset mode defaults to
// disabled so an unconfigured feature performs no verification rather than
// failing; an unset failure policy defaults to reject.
func (c *SecurityConfig) Default() {
	if c.Mode == "" {
		c.Mode = ModeDisabled
	}
	if c.FailurePolicy == "" {
		c.FailurePolicy = FailurePolicyReject
	}
}

// Validate checks that enum-valued fields hold known values. Each mode adds its
// own case here (and any mode-specific field checks in its constructor).
func (c *SecurityConfig) Validate() error {
	switch c.Mode {
	case ModeDisabled:
	default:
		return fmt.Errorf("%w: %q", ErrUnknownMode, c.Mode)
	}

	switch c.FailurePolicy {
	case FailurePolicyReject, FailurePolicyWarn:
	default:
		return fmt.Errorf("invalid failurePolicy: %q", c.FailurePolicy)
	}

	return nil
}

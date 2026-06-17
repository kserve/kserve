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

package utils

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/version"
)

type stubVersioner struct {
	v   *version.Info
	err error
}

func (s *stubVersioner) ServerVersion() (*version.Info, error) { return s.v, s.err }

func TestCheckImageVolumeCompatibility(t *testing.T) {
	tests := []struct {
		name       string
		minor      string
		discovErr  bool
		wantStatus ImageVolumeStatus
	}{
		// Below the alpha introduction threshold.
		{"k8s 1.30: unsupported", "30", false, ImageVolumeUnsupported},
		// Alpha (1.31-1.32): gate required, subPath forbidden.
		{"k8s 1.31: subpath unsupported", "31", false, ImageVolumeSubPathUnsupported},
		{"k8s 1.32: subpath unsupported", "32", false, ImageVolumeSubPathUnsupported},
		// Beta (1.33-1.34): gate required, subPath supported.
		{"k8s 1.33: needs gate", "33", false, ImageVolumeNeedsGate},
		// Last version requiring the feature gate.
		{"k8s 1.34: needs gate", "34", false, ImageVolumeNeedsGate},
		// Boundary: first beta-defaults-on release.
		{"k8s 1.35: ok", "35", false, ImageVolumeOK},
		{"k8s 1.36: ok (stable)", "36", false, ImageVolumeOK},
		// Trailing "+" in GKE/EKS minor strings must be stripped.
		{"k8s 1.31+: subpath unsupported (plus stripped)", "31+", false, ImageVolumeSubPathUnsupported},
		{"k8s 1.35+: ok (plus stripped)", "35+", false, ImageVolumeOK},
		// Failure modes.
		{"discovery error: unknown", "", true, ImageVolumeUnknown},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var sv ServerVersioner
			if tc.discovErr {
				sv = &stubVersioner{err: errors.New("unreachable")}
			} else {
				sv = &stubVersioner{v: &version.Info{Major: "1", Minor: tc.minor}}
			}
			result := CheckImageVolumeCompatibility(context.Background(), sv)
			assert.Equal(t, tc.wantStatus, result.Status)
		})
	}
}

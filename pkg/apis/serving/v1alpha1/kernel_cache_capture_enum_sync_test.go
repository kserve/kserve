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

package v1alpha1

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// kubebuilder validation markers are plain comment literals: they cannot
// reference the Go consts in ValidCachePresets / ValidVolumeStrategies. These
// tests keep the enum comments and the canonical slices in lockstep so a value
// added to one (e.g. a new "gaudi" preset) is not silently dropped from the
// other, which previously let the CRD reject a value the webhook accepted.

// enumMarkerValues extracts the semicolon-separated values from the first
// +kubebuilder:validation:Enum marker attached to the given field name in the
// KernelCacheCaptureSpec source file.
func enumMarkerValues(t *testing.T, fieldName string) []string {
	t.Helper()

	const src = "kernel_cache_capture_types.go"
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("reading %s: %v", src, err)
	}

	enumRe := regexp.MustCompile(`\+kubebuilder:validation:Enum=(\S+)`)
	fieldRe := regexp.MustCompile(`^\s*` + regexp.QuoteMeta(fieldName) + `\s`)

	lines := strings.Split(string(data), "\n")
	var pending []string
	for _, line := range lines {
		if m := enumRe.FindStringSubmatch(line); m != nil {
			pending = strings.Split(m[1], ";")
			continue
		}
		if fieldRe.MatchString(line) {
			if pending == nil {
				t.Fatalf("no +kubebuilder:validation:Enum marker found immediately before field %q", fieldName)
			}
			return pending
		}
		// A non-comment, non-blank line resets the pending marker so we only
		// associate an Enum comment with the field it directly precedes.
		if trimmed := strings.TrimSpace(line); trimmed != "" && !strings.HasPrefix(trimmed, "//") {
			pending = nil
		}
	}

	t.Fatalf("field %q not found in %s", fieldName, src)
	return nil
}

func TestCachePresetEnumMarkerMatchesCanonical(t *testing.T) {
	got := enumMarkerValues(t, "CachePreset")
	if strings.Join(got, ";") != strings.Join(ValidCachePresets, ";") {
		t.Errorf("CachePreset Enum marker %v does not match ValidCachePresets %v; "+
			"update the +kubebuilder:validation:Enum comment and ValidCachePresets together",
			got, ValidCachePresets)
	}
}

func TestVolumeStrategyEnumMarkerMatchesCanonical(t *testing.T) {
	got := enumMarkerValues(t, "VolumeStrategy")
	if strings.Join(got, ";") != strings.Join(ValidVolumeStrategies, ";") {
		t.Errorf("VolumeStrategy Enum marker %v does not match ValidVolumeStrategies %v; "+
			"update the +kubebuilder:validation:Enum comment and ValidVolumeStrategies together",
			got, ValidVolumeStrategies)
	}
}

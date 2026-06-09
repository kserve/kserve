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

package validation

import (
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// KBSResourceIdRegexp validates KBS resource ID format: kbs:///<repo>/<type>/<tag>.
// Each segment allows alphanumeric characters, hyphens, underscores, and dots.
// Query parameters are intentionally not supported; the three-segment path is
// the standard KBS addressing scheme defined by the Confidential Containers project.
var KBSResourceIdRegexp = regexp.MustCompile(`^kbs:///[a-zA-Z0-9_.-]+/[a-zA-Z0-9_.-]+/[a-zA-Z0-9_.-]+$`)

// ValidateConfidentialSpec validates confidential model serving configuration.
// It is shared between InferenceService (v1beta1) and LLMInferenceService (v1alpha2)
// webhooks to avoid duplicating the validation logic.
func ValidateConfidentialSpec(enabled bool, resourceId *string, uri string, basePath *field.Path) (admission.Warnings, field.ErrorList) {
	var warnings admission.Warnings
	var allErrs field.ErrorList

	if !enabled {
		return warnings, allErrs
	}

	// Warn if OCI URI is used with confidential
	if strings.HasPrefix(uri, "oci://") {
		warnings = append(warnings,
			"confidential has no effect with OCI URIs; OCI image decryption is handled by the container runtime via runtimeClassName")
	}

	// Validate resourceId format if provided
	if resourceId != nil && *resourceId != "" {
		if !KBSResourceIdRegexp.MatchString(*resourceId) {
			allErrs = append(allErrs, field.Invalid(
				basePath.Child("confidential", "resourceId"),
				*resourceId,
				"must be in the format kbs:///<repo>/<type>/<tag>"))
		}
	}

	return warnings, allErrs
}

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
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/kserve/kserve/pkg/constants"
)

// LocalModelCachePolicyRequired resolves the per-workload shared-PVC local model
// cache policy (constants.LocalModelCachePolicyAnnotationKey) from a workload's
// annotations. It reports whether the workload opted into CacheRequired.
//
// Unset or empty is the FallbackToSource default. Values are compared
// case-insensitively after trimming. A malformed non-empty value fails closed:
// it is reported as an error AND treated as CacheRequired (never as a silent
// fallback to a source download); admission validation rejects it first, so the
// fail-closed read only covers workloads admitted before validation was
// installed.
func LocalModelCachePolicyRequired(annotations map[string]string) (bool, error) {
	raw, ok := annotations[constants.LocalModelCachePolicyAnnotationKey]
	if !ok {
		return false, nil
	}
	return LocalModelCachePolicyValue(raw)
}

// LocalModelCachePolicyInternalRequired resolves the policy from the internal
// annotation the workload defaulters mirror onto serving pod templates
// (constants.LocalModelCachePolicyInternalAnnotationKey). It backs the pod
// mutator's gate. Malformed non-empty values fail closed (treated as
// CacheRequired) with an error, so a crafted or corrupted annotation never
// silently disables the gate.
func LocalModelCachePolicyInternalRequired(annotations map[string]string) (bool, error) {
	raw, ok := annotations[constants.LocalModelCachePolicyInternalAnnotationKey]
	if !ok {
		return false, nil
	}
	return LocalModelCachePolicyValue(raw)
}

// LocalModelCachePolicyValue parses one local-model-cache-policy value.
// Unset, empty or whitespace-only means the FallbackToSource default. Values
// are compared case-insensitively after trimming. A malformed non-empty value
// fails closed: it is reported as an error AND treated as CacheRequired
// (never as a silent fallback to a source download); admission validation
// rejects it first, so the fail-closed read only covers workloads admitted
// before validation was installed or pod annotations tampered with.
func LocalModelCachePolicyValue(raw string) (bool, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return false, nil
	}
	if strings.EqualFold(trimmed, constants.LocalModelCachePolicyFallbackToSource) {
		return false, nil
	}
	if strings.EqualFold(trimmed, constants.LocalModelCachePolicyCacheRequired) {
		return true, nil
	}
	return true, fmt.Errorf("invalid %s value %q: must be %s or %s",
		constants.LocalModelCachePolicyAnnotationKey, raw,
		constants.LocalModelCachePolicyFallbackToSource, constants.LocalModelCachePolicyCacheRequired)
}

// ValidateLocalModelCachePolicyAnnotation rejects an unusable
// serving.kserve.io/local-model-cache-policy at admission, so a typo fails the
// create/update instead of parking the workload in fail-closed serving.
func ValidateLocalModelCachePolicyAnnotation(annotations map[string]string, path *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	raw, ok := annotations[constants.LocalModelCachePolicyAnnotationKey]
	if !ok {
		return allErrs
	}

	trimmed := strings.TrimSpace(raw)
	if trimmed == "" ||
		strings.EqualFold(trimmed, constants.LocalModelCachePolicyFallbackToSource) ||
		strings.EqualFold(trimmed, constants.LocalModelCachePolicyCacheRequired) {
		return allErrs
	}
	return append(allErrs, field.NotSupported(
		path.Key(constants.LocalModelCachePolicyAnnotationKey),
		raw,
		[]string{constants.LocalModelCachePolicyFallbackToSource, constants.LocalModelCachePolicyCacheRequired},
	))
}

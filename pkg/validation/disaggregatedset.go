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
	"strings"

	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/kserve/kserve/pkg/constants"
)

// disaggregatedSetAnnotationValues are the accepted values of the opt-in annotation,
// compared case-insensitively after trimming. This matches the convention used by the
// other boolean annotation on these resources, serving.kserve.io/stop, which is read with
// strings.EqualFold against "true". It is deliberately narrower than strconv.ParseBool,
// which would also accept "1", "t", "0" and "f"; those are not idiomatic in a Kubernetes
// annotation and would widen the contract beyond what the API documents.
const (
	disaggregatedSetAnnotationTrue  = "true"
	disaggregatedSetAnnotationFalse = "false"
)

// DisaggregatedSetEnabled reports whether the annotations opt the resource into the
// DisaggregatedSet workload backend.
//
// It is deliberately lenient: an absent, malformed or unrecognised value reads as "not
// opted in", because ValidateDisaggregatedSetAnnotation rejects those at admission. That
// keeps a bad annotation from being treated as opt-in by a controller if one ever reaches
// it, for instance on a resource created before the webhook existed.
func DisaggregatedSetEnabled(annotations map[string]string) bool {
	raw, ok := annotations[constants.LLMDisaggregatedSetAnnotationKey]
	if !ok {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(raw), disaggregatedSetAnnotationTrue)
}

// ValidateDisaggregatedSetAnnotation validates the DisaggregatedSet opt-in annotation
// on any resource that supports it.
//
// This checks only that the value is well formed. It deliberately does not reject
// feature-level combinations such as opting in alongside autoscaling, for two reasons.
//
// First, admission cannot see the feature gate: the webhook validator is stateless and
// this package cannot reach the controller's config without an import cycle. Rejecting
// on the annotation alone would change admission behaviour for services whose cluster
// has the feature switched off, where the annotation is inert.
//
// Second, admission does not see the final spec. Presets referenced by spec.baseRefs are
// merged into the spec by the reconciler after admission, via a strategic merge patch
// over the whole spec, so fields like spec.scaling can appear later. A webhook check
// would therefore be incomplete by construction.
//
// Feature-level constraints belong in the reconciler, which sees both the gate and the
// merged spec and can surface them as a status condition on every pass.
func ValidateDisaggregatedSetAnnotation(annotations map[string]string) field.ErrorList {
	var allErrs field.ErrorList

	raw, ok := annotations[constants.LLMDisaggregatedSetAnnotationKey]
	if !ok {
		return allErrs
	}

	trimmed := strings.TrimSpace(raw)
	if !strings.EqualFold(trimmed, disaggregatedSetAnnotationTrue) &&
		!strings.EqualFold(trimmed, disaggregatedSetAnnotationFalse) {
		allErrs = append(allErrs, field.NotSupported(
			field.NewPath("metadata").Child("annotations").Key(constants.LLMDisaggregatedSetAnnotationKey),
			raw,
			[]string{disaggregatedSetAnnotationTrue, disaggregatedSetAnnotationFalse},
		))
	}

	return allErrs
}

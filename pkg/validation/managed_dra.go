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
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/kserve/kserve/pkg/constants"
)

// HasManagedDRA reports whether the annotations contain the managed DRA device class key.
func HasManagedDRA(annotations map[string]string) bool {
	_, ok := annotations[constants.ManagedDRADeviceClassAnnotationKey]
	return ok
}

// ManagedDRADeviceClass returns the trimmed device class and whether it was present.
func ManagedDRADeviceClass(annotations map[string]string) (string, bool) {
	raw, ok := annotations[constants.ManagedDRADeviceClassAnnotationKey]
	if !ok {
		return "", false
	}
	return strings.TrimSpace(raw), true
}

// ManagedDRADeviceCount parses the device count annotation, defaulting to 1.
func ManagedDRADeviceCount(annotations map[string]string) (int, error) {
	raw, ok := annotations[constants.ManagedDRADeviceCountAnnotationKey]
	if !ok || strings.TrimSpace(raw) == "" {
		return 1, nil
	}
	count, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("invalid %s value %q: %w", constants.ManagedDRADeviceCountAnnotationKey, raw, err)
	}
	if count < 1 {
		return 0, fmt.Errorf("invalid %s value %q: must be >= 1", constants.ManagedDRADeviceCountAnnotationKey, raw)
	}
	return count, nil
}

// ManagedDRACelSelectors splits the CEL selector annotation by newline, returning non-empty trimmed entries.
func ManagedDRACelSelectors(annotations map[string]string) []string {
	raw, ok := annotations[constants.ManagedDRACelSelectorAnnotationKey]
	if !ok || strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, "\n")
	selectors := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			selectors = append(selectors, v)
		}
	}
	return selectors
}

// ManagedDRAContainerName returns the trimmed container name and whether it was present.
func ManagedDRAContainerName(annotations map[string]string) (string, bool) {
	raw, ok := annotations[constants.ManagedDRAContainerNameAnnotationKey]
	if !ok {
		return "", false
	}
	return strings.TrimSpace(raw), true
}

// ValidateManagedDRAAnnotations validates DRA-related annotations on any resource.
func ValidateManagedDRAAnnotations(annotations map[string]string) field.ErrorList {
	var allErrs field.ErrorList
	if len(annotations) == 0 {
		return allErrs
	}

	annotationsPath := field.NewPath("metadata").Child("annotations")

	hasDeviceClass := HasManagedDRA(annotations)
	_, hasDeviceCount := annotations[constants.ManagedDRADeviceCountAnnotationKey]
	_, hasCelSelector := annotations[constants.ManagedDRACelSelectorAnnotationKey]
	_, hasContainerName := annotations[constants.ManagedDRAContainerNameAnnotationKey]

	// Require device-class if any other DRA annotation is set.
	if !hasDeviceClass && (hasDeviceCount || hasCelSelector || hasContainerName) {
		allErrs = append(allErrs, field.Required(
			annotationsPath.Key(constants.ManagedDRADeviceClassAnnotationKey),
			fmt.Sprintf("%s is required to enable managed DRA when %s, %s, or %s is set",
				constants.ManagedDRADeviceClassAnnotationKey,
				constants.ManagedDRADeviceCountAnnotationKey,
				constants.ManagedDRACelSelectorAnnotationKey,
				constants.ManagedDRAContainerNameAnnotationKey),
		))
	}

	if trimmed, present := ManagedDRADeviceClass(annotations); present {
		raw := annotations[constants.ManagedDRADeviceClassAnnotationKey]
		if trimmed == "" {
			allErrs = append(allErrs, field.Invalid(
				annotationsPath.Key(constants.ManagedDRADeviceClassAnnotationKey),
				raw,
				"device class must not be empty",
			))
		} else if errs := validation.IsDNS1123Subdomain(trimmed); len(errs) > 0 {
			allErrs = append(allErrs, field.Invalid(
				annotationsPath.Key(constants.ManagedDRADeviceClassAnnotationKey),
				raw,
				"device class must be a DNS subdomain: "+strings.Join(errs, "; "),
			))
		}
	}

	if hasDeviceCount {
		if _, err := ManagedDRADeviceCount(annotations); err != nil {
			allErrs = append(allErrs, field.Invalid(
				annotationsPath.Key(constants.ManagedDRADeviceCountAnnotationKey),
				annotations[constants.ManagedDRADeviceCountAnnotationKey],
				err.Error(),
			))
		}
	}

	if hasCelSelector && len(ManagedDRACelSelectors(annotations)) == 0 {
		allErrs = append(allErrs, field.Invalid(
			annotationsPath.Key(constants.ManagedDRACelSelectorAnnotationKey),
			annotations[constants.ManagedDRACelSelectorAnnotationKey],
			"cel selector must contain at least one non-empty CEL expression",
		))
	}

	if trimmed, present := ManagedDRAContainerName(annotations); present {
		raw := annotations[constants.ManagedDRAContainerNameAnnotationKey]
		if trimmed == "" {
			allErrs = append(allErrs, field.Invalid(
				annotationsPath.Key(constants.ManagedDRAContainerNameAnnotationKey),
				raw,
				"container name must not be empty",
			))
		} else if errs := validation.IsDNS1123Label(trimmed); len(errs) > 0 {
			allErrs = append(allErrs, field.Invalid(
				annotationsPath.Key(constants.ManagedDRAContainerNameAnnotationKey),
				raw,
				"container name must be a DNS label: "+strings.Join(errs, "; "),
			))
		}
	}

	return allErrs
}

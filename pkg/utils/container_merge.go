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
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
)

// MergeContainerWithPatch merges overlay onto base using strategic merge patch,
// reconciles Value/ValueFrom env conflicts, and restores Name from base.
//
// Callers are responsible for any additional field restoration (Args, Command)
// after the merge — those policies vary by call site.
func MergeContainerWithPatch(base, overlay corev1.Container) (corev1.Container, error) {
	baseJSON, err := json.Marshal(base)
	if err != nil {
		return corev1.Container{}, fmt.Errorf("could not marshal base container: %w", err)
	}

	overlayJSON, err := json.Marshal(overlay)
	if err != nil {
		return corev1.Container{}, fmt.Errorf("could not marshal overlay container: %w", err)
	}

	mergedJSON, err := strategicpatch.StrategicMergePatch(baseJSON, overlayJSON, corev1.Container{})
	if err != nil {
		return corev1.Container{}, fmt.Errorf("could not apply strategic merge patch: %w", err)
	}

	// Unmarshal into a fresh Container to prevent stale field bleed:
	// json.Unmarshal does not clear struct fields absent from the JSON,
	// so reusing an existing struct lets old slice-element fields survive.
	var merged corev1.Container
	if err := json.Unmarshal(mergedJSON, &merged); err != nil {
		return corev1.Container{}, fmt.Errorf("could not unmarshal merged container: %w", err)
	}

	reconcileEnvValueConflicts(merged.Env, overlay.Env)

	if merged.Name == "" {
		merged.Name = base.Name
	}

	return merged, nil
}

// reconcileEnvValueConflicts fixes env entries where strategic merge patch left
// both Value and ValueFrom populated — a state Kubernetes admission rejects.
// The overlay's intent wins: if the overlay set ValueFrom, Value is cleared;
// if the overlay set Value, ValueFrom is cleared.
func reconcileEnvValueConflicts(merged []corev1.EnvVar, overlayEnv []corev1.EnvVar) {
	overlayByName := make(map[string]corev1.EnvVar, len(overlayEnv))
	for _, e := range overlayEnv {
		overlayByName[e.Name] = e
	}
	for i := range merged {
		e := &merged[i]
		if e.Value == "" || e.ValueFrom == nil {
			continue
		}
		overlayEntry, ok := overlayByName[e.Name]
		if !ok {
			continue
		}
		if overlayEntry.ValueFrom != nil {
			e.Value = ""
		} else if overlayEntry.Value != "" {
			e.ValueFrom = nil
		}
	}
}

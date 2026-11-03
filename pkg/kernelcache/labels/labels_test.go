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

package labels

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/util/validation"
)

func TestValuePreservesShortValidValue(t *testing.T) {
	const value = "gpu-node-1"
	if got := Value(value); got != value {
		t.Fatalf("Value(%q) = %q, want the original value", value, got)
	}
}

func TestValueHashesLongValue(t *testing.T) {
	value := strings.Repeat("n", validation.LabelValueMaxLength+1)
	got := Value(value)
	if len(got) != validation.LabelValueMaxLength {
		t.Fatalf("hashed label length = %d, want %d", len(got), validation.LabelValueMaxLength)
	}
	if len(validation.IsValidLabelValue(got)) != 0 {
		t.Fatalf("hashed label %q is invalid: %v", got, validation.IsValidLabelValue(got))
	}
	if got != Value(value) {
		t.Fatalf("hash representation is not deterministic")
	}
	if got == value {
		t.Fatalf("long value was not hashed")
	}
}

func TestMetadataPreservesFullIdentity(t *testing.T) {
	name := strings.Repeat("c", validation.LabelValueMaxLength+1)
	node := strings.Repeat("n", validation.LabelValueMaxLength+1)
	objectMeta := ObjectMeta(name, "team-a", node)

	if objectMeta.Labels[KernelCacheNameLabel] == name || objectMeta.Labels[KernelCacheNodeLabel] == node {
		t.Fatalf("long identity values must not be used directly as labels")
	}
	if objectMeta.Annotations[KernelCacheNameAnnotation] != name || objectMeta.Annotations[KernelCacheNodeAnnotation] != node {
		t.Fatalf("full identity values were not preserved in annotations")
	}
}

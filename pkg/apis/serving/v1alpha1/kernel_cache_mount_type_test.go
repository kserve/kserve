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

import "testing"

func TestResolveKernelCacheMountType(t *testing.T) {
	tests := []struct {
		name    string
		current KernelCacheMountType
		deflt   string
		want    KernelCacheMountType
	}{
		{name: "keeps explicit pvc", current: KernelCacheMountTypePVC, deflt: "oci", want: KernelCacheMountTypePVC},
		{name: "uses default pvc", current: "", deflt: "pvc", want: KernelCacheMountTypePVC},
		{name: "defaults to oci", current: "", deflt: "", want: KernelCacheMountTypeOCI},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ResolveKernelCacheMountType(test.current, test.deflt); got != test.want {
				t.Fatalf("got %q, want %q", got, test.want)
			}
		})
	}
}

func TestIsSupportedKernelCacheMountType(t *testing.T) {
	if !IsSupportedKernelCacheMountType(KernelCacheMountTypeOCI) || !IsSupportedKernelCacheMountType(KernelCacheMountTypePVC) {
		t.Fatal("expected oci and pvc to be supported")
	}
	if IsSupportedKernelCacheMountType("hostPath") || IsSupportedKernelCacheMountType("") {
		t.Fatal("expected unknown and empty mount types to be rejected")
	}
}

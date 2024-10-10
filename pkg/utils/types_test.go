/*
Copyright 2021 The KServe Authors.

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
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestBool(t *testing.T) {
	input := true
	expected := &input
	result := Bool(input)
	if diff := cmp.Diff(expected, result); diff != "" {
		t.Errorf("Test %q unexpected result (-want +got): %v", t.Name(), diff)
	}
}

func TestUInt64(t *testing.T) {
	input := uint64(63)
	expected := &input
	result := UInt64(input)
	if diff := cmp.Diff(expected, result); diff != "" {
		t.Errorf("Test %q unexpected result (-want +got): %v", t.Name(), diff)
	}
}

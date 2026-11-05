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
	"fmt"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation"
)

func Convert[T any](obj runtime.Object) (T, error) {
	v, ok := obj.(T)
	if !ok {
		var zero T
		return zero, fmt.Errorf("convert: expected %T, got %T", zero, obj)
	}

	return v, nil
}

// StringToInt32 converts a given integer to int32. If the number exceeds the int32 limit, it returns an error.
func StringToInt32(number string) (int32, error) {
	converted, err := strconv.ParseInt(number, 10, 32)
	if err != nil {
		return 0, err
	}
	return int32(converted), err
}

// ParsePort parses value as a TCP port and validates it against the valid
// Kubernetes port range.
func ParsePort(value string) (int32, error) {
	port, err := StringToInt32(value)
	if err != nil {
		return 0, fmt.Errorf("invalid port %q: %w", value, err)
	}
	if errs := validation.IsValidPortNum(int(port)); errs != nil {
		return 0, fmt.Errorf("invalid port %q: %s", value, strings.Join(errs, "; "))
	}
	return port, nil
}

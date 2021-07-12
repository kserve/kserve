/*
Copyright 2019 kubeflow.org.

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
	"github.com/kubeflow/kfserving/pkg/constants"
	v1 "k8s.io/api/core/v1"
	"strings"
)

/* NOTE TO AUTHORS:
 *
 * Only you can prevent ... the proliferation of useless "utility" classes.
 * Please add functional style container operations sparingly and intentionally.
 */

func Filter(origin map[string]string, predicate func(string) bool) map[string]string {
	result := make(map[string]string)
	for k, v := range origin {
		if predicate(k) {
			result[k] = v
		}
	}
	return result
}

func Union(maps ...map[string]string) map[string]string {
	result := make(map[string]string)
	for _, m := range maps {
		for k, v := range m {
			result[k] = v
		}
	}
	return result
}

func Includes(slice []string, value string) bool {
	for _, v := range slice {
		if v == value {
			return true
		}
	}
	return false
}

func IncludesArg(slice []string, arg string) bool {
	for _, v := range slice {
		if v == arg || strings.HasPrefix(v, arg) {
			return true
		}
	}
	return false
}

func AppendVolumeIfNotExists(slice []v1.Volume, volume v1.Volume) []v1.Volume {
	for i := range slice {
		if slice[i].Name == volume.Name {
			return slice
		}
	}
	return append(slice, volume)
}

func IsGPUEnabled(requirements v1.ResourceRequirements) bool {
	_, ok := requirements.Limits[constants.NvidiaGPUResourceType]
	return ok
}

// FirstNonNilError returns the first non nil interface in the slice
func FirstNonNilError(objects []error) error {
	for _, object := range objects {
		if object != nil {
			return object
		}
	}
	return nil
}

// Helper functions to remove string from a slice of strings.
func RemoveString(slice []string, s string) (result []string) {
	for _, item := range slice {
		if item == s {
			continue
		}
		result = append(result, item)
	}
	return
}

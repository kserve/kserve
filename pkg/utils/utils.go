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
	"encoding/json"
	"strconv"
	"strings"

	"github.com/kserve/kserve/pkg/constants"
	v1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
)

/* NOTE TO AUTHORS:
 *
 * Only you can prevent ... the proliferation of useless "utility" classes.
 * Please add functional style container operations sparingly and intentionally.
 */

var gvResourcesCache map[string]*metav1.APIResourceList

// Errors
const (
	ErrValueExceedsInt32Limit = "value exceeds int32 limit %d"
)

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

// RemoveString Helper functions to remove string from a slice of strings.
func RemoveString(slice []string, s string) (result []string) {
	for _, item := range slice {
		if item == s {
			continue
		}
		result = append(result, item)
	}
	return
}

// IsPrefixSupported Check if a given string contains one of the prefixes in the provided list.
func IsPrefixSupported(input string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(input, prefix) {
			return true
		}
	}
	return false
}

// MergeEnvs Merge a slice of EnvVars (`O`) into another slice of EnvVars (`B`), which does the following:
// 1. If an EnvVar is present in B but not in O, value remains unchanged in the result
// 2. If an EnvVar is present in `O` but not in `B`, appends to the result
// 3. If an EnvVar is present in both O and B, uses the value from O in the result
func MergeEnvs(baseEnvs []v1.EnvVar, overrideEnvs []v1.EnvVar) []v1.EnvVar {
	var extra []v1.EnvVar

	for _, override := range overrideEnvs {
		inBase := false

		for i, base := range baseEnvs {
			if override.Name == base.Name {
				inBase = true
				baseEnvs[i].Value = override.Value
				break
			}
		}

		if !inBase {
			extra = append(extra, override)
		}
	}

	return append(baseEnvs, extra...)
}

func AppendEnvVarIfNotExists(slice []v1.EnvVar, elems ...v1.EnvVar) []v1.EnvVar {
	for _, elem := range elems {
		isElemExists := false
		for _, item := range slice {
			if item.Name == elem.Name {
				isElemExists = true
				break
			}
		}
		if !isElemExists {
			slice = append(slice, elem)
		}
	}
	return slice
}

func AppendPortIfNotExists(slice []v1.ContainerPort, elems ...v1.ContainerPort) []v1.ContainerPort {
	for _, elem := range elems {
		isElemExists := false
		for _, item := range slice {
			if item.Name == elem.Name {
				isElemExists = true
				break
			}
		}
		if !isElemExists {
			slice = append(slice, elem)
		}
	}
	return slice
}

// IsCrdAvailable checks if a given CRD is present in the cluster by verifying the
// existence of its API.
func IsCrdAvailable(config *rest.Config, groupVersion, kind string) (bool, error) {
	gvResources, err := GetAvailableResourcesForApi(config, groupVersion)
	if err != nil {
		return false, err
	}

	found := false
	if gvResources != nil {
		for _, crd := range gvResources.APIResources {
			if crd.Kind == kind {
				found = true
				break
			}
		}
	}

	return found, nil
}

// GetAvailableResourcesForApi returns the list of discovered resources that belong
// to the API specified in groupVersion. The first query to a specifig groupVersion will
// query the cluster API server to discover the available resources and the discovered
// resources will be cached and returned to subsequent invocations to prevent additional
// queries to the API server.
func GetAvailableResourcesForApi(config *rest.Config, groupVersion string) (*metav1.APIResourceList, error) {
	var gvResources *metav1.APIResourceList
	var ok bool

	if gvResources, ok = gvResourcesCache[groupVersion]; !ok {
		discoveryClient, newClientErr := discovery.NewDiscoveryClientForConfig(config)
		if newClientErr != nil {
			return nil, newClientErr
		}

		var getGvResourcesErr error
		gvResources, getGvResourcesErr = discoveryClient.ServerResourcesForGroupVersion(groupVersion)
		if getGvResourcesErr != nil && !apierr.IsNotFound(getGvResourcesErr) {
			return nil, getGvResourcesErr
		}

		SetAvailableResourcesForApi(groupVersion, gvResources)
	}

	return gvResources, nil
}

// SetAvailableResourcesForApi stores the value fo resources argument in the global cache
// of discovered API resources. This function should never be called directly. It is exported
// for usage in tests.
func SetAvailableResourcesForApi(groupVersion string, resources *metav1.APIResourceList) {
	if gvResourcesCache == nil {
		gvResourcesCache = make(map[string]*metav1.APIResourceList)
	}

	gvResourcesCache[groupVersion] = resources
}

func GetEnvVarValue(envVars []v1.EnvVar, key string) (string, bool) {
	for _, envVar := range envVars {
		if envVar.Name == key {
			return envVar.Value, true // if key exist, return value, true
		}
	}
	return "", false // if key does not exist, return "", false
}

// IsUnknownGpuResourceType check if the provided gpu resource type is unknown one
func IsUnknownGpuResourceType(resources v1.ResourceRequirements, customGpuResourceTypes string) bool {
	basicResourceTypes := map[v1.ResourceName]struct{}{
		v1.ResourceCPU:              {},
		v1.ResourceMemory:           {},
		v1.ResourceStorage:          {},
		v1.ResourceEphemeralStorage: {},
	}

	possibleGPUResourceType := map[v1.ResourceName]struct{}{}

	// Helper function to add non-basic resources from the provided ResourceList
	addNonBasicResources := func(resources v1.ResourceList) {
		for resourceType := range resources {
			if _, exists := basicResourceTypes[resourceType]; !exists {
				possibleGPUResourceType[resourceType] = struct{}{}
			}
		}
	}

	// Add non-basic resources from both Limits and Requests
	addNonBasicResources(resources.Limits)
	addNonBasicResources(resources.Requests)

	// Validate GPU resource types
	// If CustomGPUResourceTypesAnnotationKey is set, the specified custom GPU resource will be added to the available GPUResourceTypeList.
	if customGpuResourceTypes != "" {
		constants.GPUResourceTypeList = append(constants.GPUResourceTypeList, strings.Split(customGpuResourceTypes, ",")...)
	}

	for _, gpuType := range constants.GPUResourceTypeList {
		allowedGPUResourceName := v1.ResourceName(gpuType)
		delete(possibleGPUResourceType, allowedGPUResourceName) // Remove allowed GPU resource if exists
	}

	// Return true if there are unknown GPU resources
	return len(possibleGPUResourceType) > 0
}

// IsValidCustomGPUArray checks if the input string is a valid JSON array of strings.
// It returns false if the array is empty, contains empty strings, or any non-string elements.
func IsValidCustomGPUArray(s string) bool {
	// Check if the input string is a valid JSON array
	var arr []interface{}
	if err := json.Unmarshal([]byte(s), &arr); err != nil {
		return false // Not a valid JSON array
	}

	// Check if the array is empty
	if len(arr) == 0 {
		return false
	}

	// Check each element to ensure they are all strings
	for _, item := range arr {
		if _, ok := item.(string); !ok {
			return false // Found a non-string element
		}
		if item.(string) == "" {
			return false // Found an empty string
		}
	}

	return true
}

// StringToInt32 converts a given integer to int32. If the number exceeds the int32 limit, it returns an error.
func StringToInt32(number string) (int32, error) {
	converted, err := strconv.ParseInt(number, 10, 32)
	if err != nil {
		return 0, err
	}
	return int32(converted), err
}

// ToPointer returns a pointer to the given value.
func ToPointer[T any](v T) *T {
	temp := v
	return &temp
}

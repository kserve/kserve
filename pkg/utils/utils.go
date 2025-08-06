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
	"fmt"
	"path/filepath"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"

	"github.com/kserve/kserve/pkg/constants"
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

func AppendVolumeIfNotExists(slice []corev1.Volume, volume corev1.Volume) []corev1.Volume {
	for i := range slice {
		if slice[i].Name == volume.Name {
			return slice
		}
	}
	return append(slice, volume)
}

func IsGPUEnabled(requirements corev1.ResourceRequirements) bool {
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
func MergeEnvs(baseEnvs []corev1.EnvVar, overrideEnvs []corev1.EnvVar) []corev1.EnvVar {
	var extra []corev1.EnvVar

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

func AppendEnvVarIfNotExists(slice []corev1.EnvVar, elems ...corev1.EnvVar) []corev1.EnvVar {
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

// Add an environment variable with the given value to the environments
// variables of the given container, potentially replacing an env var that already exists
// with this name
func AddOrReplaceEnv(container *corev1.Container, envKey string, envValue string) {
	if container.Env == nil {
		container.Env = []corev1.EnvVar{}
	}

	for i, envVar := range container.Env {
		if envVar.Name == envKey {
			container.Env[i].Value = envValue
			return
		}
	}

	container.Env = append(container.Env, corev1.EnvVar{
		Name:  envKey,
		Value: envValue,
	})
}

func AppendPortIfNotExists(slice []corev1.ContainerPort, elems ...corev1.ContainerPort) []corev1.ContainerPort {
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

func GetEnvVarValue(envVars []corev1.EnvVar, key string) (string, bool) {
	for _, envVar := range envVars {
		if envVar.Name == key {
			return envVar.Value, true // if key exist, return value, true
		}
	}
	return "", false // if key does not exist, return "", false
}

func GetContainerWithName(podSpec *corev1.PodSpec, name string) *corev1.Container {
	for idx, container := range podSpec.Containers {
		if strings.Compare(container.Name, name) == 0 {
			return &podSpec.Containers[idx]
		}
	}
	return nil
}

func GetInitContainerWithName(podSpec *corev1.PodSpec, name string) *corev1.Container {
	for idx, container := range podSpec.InitContainers {
		if strings.Compare(container.Name, name) == 0 {
			return &podSpec.InitContainers[idx]
		}
	}
	return nil
}

// AddVolumeMountIfNotPresent adds a volume mount to a given container but only if no volume mount
// with this name has been already added. Container must not be nil
func AddVolumeMountIfNotPresent(container *corev1.Container, mountName, mountPath string, readOnly bool) {
	for _, v := range container.VolumeMounts {
		if v.Name == mountName {
			return
		}
	}
	modelMount := corev1.VolumeMount{
		Name:      mountName,
		MountPath: mountPath,
		ReadOnly:  readOnly,
	}
	container.VolumeMounts = append(container.VolumeMounts, modelMount)
}

// Returns the value of the stop annotation
// Defaults to false if the annotation is not present
func GetForceStopRuntime(obj metav1.Object) bool {
	forceStopRuntime := false
	// Check that the object exists
	if obj == nil {
		return forceStopRuntime
	}

	// Check that the annotations exist
	anns := obj.GetAnnotations()
	if anns == nil {
		return forceStopRuntime
	}

	// Determine the value of the stop annotation
	if val, exist := anns[constants.StopAnnotationKey]; exist {
		forceStopRuntime = strings.EqualFold(val, "true")
	}

	return forceStopRuntime
}

// HasUnknownGpuResourceType check if the provided gpu resource type is unknown one
func HasUnknownGpuResourceType(resources corev1.ResourceRequirements, annotations map[string]string) (bool, error) {
	basicResourceTypes := map[corev1.ResourceName]struct{}{
		corev1.ResourceCPU:              {},
		corev1.ResourceMemory:           {},
		corev1.ResourceStorage:          {},
		corev1.ResourceEphemeralStorage: {},
	}

	possibleGPUResourceType := map[corev1.ResourceName]struct{}{}

	// Helper function to add non-basic resources from the provided ResourceList
	addNonBasicResources := func(resources corev1.ResourceList) {
		for resourceType := range resources {
			if _, exists := basicResourceTypes[resourceType]; !exists {
				possibleGPUResourceType[resourceType] = struct{}{}
			}
		}
	}

	// Add non-basic resources from both Limits and Requests
	addNonBasicResources(resources.Limits)
	addNonBasicResources(resources.Requests)

	// Update GPU resource type list
	newGPUResourceTypeList, err := UpdateGPUResourceTypeListByAnnotation(annotations)
	if err != nil {
		return false, err
	}

	// Validate GPU resource types
	for _, gpuType := range newGPUResourceTypeList {
		allowedGPUResourceName := corev1.ResourceName(gpuType)
		delete(possibleGPUResourceType, allowedGPUResourceName) // Remove allowed GPU resource if exists
	}

	// Return true if there are unknown GPU resources
	return len(possibleGPUResourceType) > 0, nil
}

// IsValidCustomGPUArray checks if the input string is a valid JSON array of strings.
// It returns false if the array is empty, contains empty strings, or any non-string elements.
// Otherwise, it returns true and the list of custom GPU types.
func IsValidCustomGPUArray(s string) ([]string, bool) {
	// Check if the input string is a valid JSON array
	var arr []interface{}
	if err := json.Unmarshal([]byte(s), &arr); err != nil {
		return nil, false // Not a valid JSON array
	}

	// Check if the array is empty
	if len(arr) == 0 {
		return nil, false
	}
	customGPUTypes := []string{}
	// Check each element to ensure they are all strings
	for _, item := range arr {
		if _, ok := item.(string); !ok {
			return nil, false // Found a non-string element
		}
		if item.(string) == "" {
			return nil, false // Found an empty string
		}
		customGPUTypes = append(customGPUTypes, item.(string))
	}

	return customGPUTypes, true
}

// UpdateGPUResourceTypeListByAnnotation updates the GPU resource type list
// by combining the global GPU resource types from inferenceservice-config with custom GPU resource types specified in the annotations.
func UpdateGPUResourceTypeListByAnnotation(isvcAnnotations map[string]string) ([]string, error) {
	// Deep copy
	updatedGPUResourceTypes := append([]string{}, constants.DefaultGPUResourceTypeList...)

	if customGPUResourceTypes := isvcAnnotations[constants.CustomGPUResourceTypesAnnotationKey]; customGPUResourceTypes != "" {
		newGPUResourceTypesFromAnnotation, isValid := IsValidCustomGPUArray(customGPUResourceTypes)
		if !isValid {
			return nil, fmt.Errorf("invalid GPU format(%s) for %s annotation: must be a valid JSON array", customGPUResourceTypes, constants.CustomGPUResourceTypesAnnotationKey)
		}

		// Use a map to avoid duplicates
		existingTypes := make(map[string]struct{}, len(constants.DefaultGPUResourceTypeList))
		for _, t := range constants.DefaultGPUResourceTypeList {
			existingTypes[t] = struct{}{}
		}

		// Add only unique GPU resource types
		for _, t := range newGPUResourceTypesFromAnnotation {
			if _, exists := existingTypes[t]; !exists {
				updatedGPUResourceTypes = append(updatedGPUResourceTypes, t)
				existingTypes[t] = struct{}{}
			}
		}
	}
	return updatedGPUResourceTypes, nil
}

// UpdateGlobalGPUResourceTypeList adds new GPU resource types from inferenceservice-config to constants.GPUResourceTypeList.
func UpdateGlobalGPUResourceTypeList(newGPUResourceTypes []string) error {
	// Use a map to avoid duplicates
	existingTypes := make(map[string]struct{}, len(constants.DefaultGPUResourceTypeList))
	for _, t := range constants.DefaultGPUResourceTypeList {
		existingTypes[t] = struct{}{}
	}

	// Add only unique GPU resource types
	for _, t := range newGPUResourceTypes {
		if _, exists := existingTypes[t]; !exists {
			constants.DefaultGPUResourceTypeList = append(constants.DefaultGPUResourceTypeList, t)
			existingTypes[t] = struct{}{}
		}
	}

	return nil
}

// GetGPUResourceQtyByType retrieves the GPU resource quantity from the given ResourceRequirements.
// It checks both Request and Limit based on the provided resourceType.
func GetGPUResourceQtyByType(resourceRequirements *corev1.ResourceRequirements, resourceType string) (corev1.ResourceName, *resource.Quantity, bool) {
	if resourceType == "Limit" {
		for resourceName, quantity := range resourceRequirements.Limits {
			for _, gpuResourceType := range constants.DefaultGPUResourceTypeList {
				if string(resourceName) == gpuResourceType {
					return resourceName, &quantity, true
				}
			}
		}
	} else {
		for resourceName, quantity := range resourceRequirements.Requests {
			for _, gpuResourceType := range constants.DefaultGPUResourceTypeList {
				if string(resourceName) == gpuResourceType {
					return resourceName, &quantity, true
				}
			}
		}
	}
	qty := resource.NewQuantity(0, resource.DecimalSI)

	return "", qty, false
}

// GetParentDirectory returns the parent directory of the given path,
// or "/" if the path is a top-level directory.
func GetParentDirectory(path string) string {
	// Get the parent directory
	parentDir := filepath.Dir(path)

	// Check if it's a top-level directory
	if parentDir == "." || parentDir == "/" {
		return "/"
	}

	return parentDir
}

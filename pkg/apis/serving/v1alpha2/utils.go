package v1alpha2

import (
	"fmt"
	"regexp"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/kubernetes/pkg/apis/core"
	"k8s.io/kubernetes/pkg/apis/core/validation"
)

func setResourceRequirementDefaults(requirements *v1.ResourceRequirements) {
	if requirements.Requests == nil {
		requirements.Requests = v1.ResourceList{}
	}

	if _, ok := requirements.Requests[v1.ResourceCPU]; !ok {
		requirements.Requests[v1.ResourceCPU] = DefaultCPU
	}
	if _, ok := requirements.Requests[v1.ResourceMemory]; !ok {
		requirements.Requests[v1.ResourceMemory] = DefaultMemory
	}

	if requirements.Limits == nil {
		requirements.Limits = v1.ResourceList{}
	}

	if _, ok := requirements.Limits[v1.ResourceCPU]; !ok {
		requirements.Limits[v1.ResourceCPU] = DefaultCPU
	}
	if _, ok := requirements.Limits[v1.ResourceMemory]; !ok {
		requirements.Limits[v1.ResourceMemory] = DefaultMemory
	}
}

func toCoreResourceRequirements(rr *v1.ResourceRequirements) *core.ResourceRequirements {
	resourceRequirements := &core.ResourceRequirements{
		Limits:   make(core.ResourceList),
		Requests: make(core.ResourceList),
	}

	for k, v := range rr.Requests {
		resourceName := core.ResourceName(string(k))
		resourceRequirements.Requests[resourceName] = v
	}
	for k, v := range rr.Limits {
		resourceName := core.ResourceName(string(k))
		resourceRequirements.Limits[resourceName] = v
	}

	return resourceRequirements
}

// validate the ResourceRequirements of the model.
func validateResourceRequirements(rr *v1.ResourceRequirements) error {
	if errs := validation.ValidateResourceRequirements(toCoreResourceRequirements(rr), field.NewPath("resources")); len(errs) != 0 {
		return fmt.Errorf("Unexpected error: %v", errs)
	}
	return nil
}

func validateStorageURI(storageURI string) error {
	if storageURI == "" {
		return nil
	}

	// local path (not some protocol?)
	if !regexp.MustCompile("\\w+?://").MatchString(storageURI) {
		return nil
	}

	// one of the prefixes we know?
	for _, prefix := range SupportedStorageURIPrefixList {
		if strings.HasPrefix(storageURI, prefix) {
			return nil
		}
	}

	azureURIMatcher := regexp.MustCompile(AzureBlobURIRegEx)
	if parts := azureURIMatcher.FindStringSubmatch(storageURI); parts != nil {
		return nil
	}

	return fmt.Errorf(UnsupportedStorageURIFormatError, strings.Join(SupportedStorageURIPrefixList, ", "), storageURI)
}

func validateReplicas(minReplicas int, maxReplicas int) error {
	if minReplicas < 0 {
		return fmt.Errorf(MinReplicasLowerBoundExceededError)
	}
	if maxReplicas < 0 {
		return fmt.Errorf(MaxReplicasLowerBoundExceededError)
	}
	if minReplicas > maxReplicas && maxReplicas != 0 {
		return fmt.Errorf(MinReplicasShouldBeLessThanMaxError)
	}
	return nil
}

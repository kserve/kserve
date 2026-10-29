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

package identity

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
)

const (
	NamespaceFactor          = "namespace"
	WorkloadKindFactor       = "workloadKind"
	WorkloadNameFactor       = "workloadName"
	RuntimeImageFactor       = "runtimeImage"
	ModelURIHashFactor       = "modelURIHash"
	TensorParallelSizeFactor = "option.tensorParallelSize"
	CommandHashFactor        = "commandHash"
	ArgsHashFactor           = "argsHash"
)

// KernelCacheIdentityFactors contains the factors used to build a KernelCache identity.
type KernelCacheIdentityFactors struct {
	Namespace            string
	WorkloadKind         string
	WorkloadName         string
	RuntimeImage         string
	ModelURIHash         string
	CommandHash          string
	ArgsHash             string
	RuntimeConfigFactors map[string]string
}

type imageReferenceType int

const (
	imageReferenceFloating imageReferenceType = iota
	imageReferenceTag
	imageReferenceDigest
)

func Build(identityFactors KernelCacheIdentityFactors) (v1alpha1.KernelCacheIdentity, error) {
	normalizedFactors, imageType, err := validateAndClassifyIdentityFactors(identityFactors)
	if err != nil {
		return v1alpha1.KernelCacheIdentity{}, err
	}

	identity := v1alpha1.KernelCacheIdentity{
		Factors: buildIdentityFactors(normalizedFactors),
	}
	switch imageType {
	case imageReferenceFloating:
		return identity, nil
	case imageReferenceTag:
		identity.Footprints.CompatibilityFootprint = calculateCompatibilityFootprint(normalizedFactors)
	case imageReferenceDigest:
		identity.Footprints.CompatibilityFootprint = calculateCompatibilityFootprint(normalizedFactors)
		identity.Footprints.WorkloadFootprint = calculateWorkloadFootprint(normalizedFactors)
	}
	return identity, nil
}

func validateAndClassifyIdentityFactors(identityFactors KernelCacheIdentityFactors) (KernelCacheIdentityFactors, imageReferenceType, error) {
	if identityFactors.Namespace == "" || identityFactors.WorkloadKind == "" || identityFactors.WorkloadName == "" {
		return KernelCacheIdentityFactors{}, imageReferenceFloating, errors.New("namespace, workload kind, and workload name are required")
	}
	if !IsSHA256(identityFactors.ModelURIHash) {
		return KernelCacheIdentityFactors{}, imageReferenceFloating, errors.New("model URI hash is required and must be a SHA-256 value")
	}

	identityFactors.RuntimeImage = strings.TrimSpace(identityFactors.RuntimeImage)
	imageType, err := classifyImageReference(identityFactors.RuntimeImage)
	if err != nil {
		return KernelCacheIdentityFactors{}, imageReferenceFloating, err
	}
	return identityFactors, imageType, nil
}

func buildIdentityFactors(identityFactors KernelCacheIdentityFactors) map[string]string {
	factorValues := map[string]string{
		NamespaceFactor:    identityFactors.Namespace,
		WorkloadKindFactor: identityFactors.WorkloadKind,
		WorkloadNameFactor: identityFactors.WorkloadName,
		RuntimeImageFactor: identityFactors.RuntimeImage,
		ModelURIHashFactor: identityFactors.ModelURIHash,
	}
	addOptionalFactor(factorValues, CommandHashFactor, identityFactors.CommandHash)
	addOptionalFactor(factorValues, ArgsHashFactor, identityFactors.ArgsHash)
	addRuntimeConfigFactors(factorValues, identityFactors.RuntimeConfigFactors)
	return factorValues
}

func calculateCompatibilityFootprint(identityFactors KernelCacheIdentityFactors) string {
	compatibility := map[string]string{
		RuntimeImageFactor: identityFactors.RuntimeImage,
		ModelURIHashFactor: identityFactors.ModelURIHash,
	}
	addRuntimeConfigFactors(compatibility, identityFactors.RuntimeConfigFactors)
	return Calculate(compatibility)
}

func calculateWorkloadFootprint(identityFactors KernelCacheIdentityFactors) string {
	workload := map[string]string{
		NamespaceFactor:    identityFactors.Namespace,
		WorkloadKindFactor: identityFactors.WorkloadKind,
		WorkloadNameFactor: identityFactors.WorkloadName,
		RuntimeImageFactor: identityFactors.RuntimeImage,
		ModelURIHashFactor: identityFactors.ModelURIHash,
	}
	addOptionalFactor(workload, CommandHashFactor, identityFactors.CommandHash)
	addOptionalFactor(workload, ArgsHashFactor, identityFactors.ArgsHash)
	return Calculate(workload)
}

func ModelURIHash(raw string) string {
	if raw == "" {
		return ""
	}
	uri, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	uri.User = nil
	uri.RawQuery = ""
	uri.ForceQuery = false
	uri.Fragment = ""
	return HashStrings([]string{uri.String()})
}

func HashStrings(values []string) string {
	encoded, _ := json.Marshal(values)
	sum := sha256.Sum256(encoded)
	return fmt.Sprintf("sha256:%x", sum)
}

func Calculate(factors map[string]string) string {
	encoded, _ := json.Marshal(factors)
	sum := sha256.Sum256(encoded)
	return fmt.Sprintf("sha256:%x", sum)
}

func IsSHA256(value string) bool {
	if !strings.HasPrefix(value, "sha256:") || len(value) != len("sha256:")+64 {
		return false
	}
	for _, character := range strings.TrimPrefix(value, "sha256:") {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func addOptionalFactor(factors map[string]string, key, value string) {
	if value != "" {
		factors[key] = value
	}
}

func addRuntimeConfigFactors(factors, runtimeConfigFactors map[string]string) {
	for key, value := range runtimeConfigFactors {
		if !isDefinedRuntimeConfigFactor(key) || value == "" {
			continue
		}
		factors[key] = value
	}
}

func classifyImageReference(image string) (imageReferenceType, error) {
	image = strings.TrimSpace(image)
	if image == "" || strings.ContainsAny(image, " \t\n") {
		return imageReferenceFloating, errors.New("runtime image is required")
	}
	if at := strings.LastIndex(image, "@sha256:"); at >= 0 {
		if at == 0 || !IsSHA256(image[at+1:]) {
			return imageReferenceFloating, errors.New("runtime image digest is invalid")
		}
		return imageReferenceDigest, nil
	}
	if strings.Contains(image, "@") {
		return imageReferenceFloating, errors.New("runtime image digest is unsupported")
	}

	lastSlash := strings.LastIndex(image, "/")
	lastColon := strings.LastIndex(image, ":")
	if lastColon <= lastSlash {
		return imageReferenceFloating, nil
	}
	tag := image[lastColon+1:]
	if tag == "" {
		return imageReferenceFloating, errors.New("runtime image tag is empty")
	}
	if tag == "latest" {
		return imageReferenceFloating, nil
	}
	return imageReferenceTag, nil
}

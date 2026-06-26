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

package kernelcachecommon

import (
	"context"
	"strings"

	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
)

// LoadKernelCacheConfig loads and parses the KernelCache configuration from the ConfigMap.
// It applies default values for any unset fields and returns the updated configuration.
func LoadKernelCacheConfig(ctx context.Context, clientset kubernetes.Interface) (*v1beta1.KernelCacheConfig, error) {
	isvcConfigMap, err := v1beta1.GetInferenceServiceConfigMap(ctx, clientset)
	if err != nil {
		return nil, err
	}

	kernelCacheConfig, err := v1beta1.NewKernelCacheConfig(isvcConfigMap)
	if err != nil {
		// Return defaults on parse error
		kernelCacheConfig = &v1beta1.KernelCacheConfig{}
	}

	// Apply defaults for unset fields
	if kernelCacheConfig.JobNamespace == "" {
		kernelCacheConfig.JobNamespace = DefaultJobNamespace
	}
	if kernelCacheConfig.ExtractImage == "" {
		kernelCacheConfig.ExtractImage = DefaultExtractImage
	}
	if kernelCacheConfig.JobTTLSecondsAfterFinished == nil {
		kernelCacheConfig.JobTTLSecondsAfterFinished = ptr.To(DefaultJobTTLSecondsAfterFinished)
	}
	if kernelCacheConfig.ReconcileIntervalSeconds == nil {
		kernelCacheConfig.ReconcileIntervalSeconds = ptr.To(DefaultReconcileIntervalSeconds)
	}

	return kernelCacheConfig, nil
}

// ReplaceUrlTag replaces the tag in an OCI image URL with a sha256 digest.
// This ensures the extraction Job uses the exact image that was verified by the webhook.
//
// Input formats:
//   - registry/image:tag -> registry/image@sha256:abc123
//   - registry/image:tag@sha256:old -> registry/image@sha256:abc123 (replace digest)
//   - registry/image (no tag) -> registry/image@sha256:abc123
//
// Returns empty string if imageURL or digest is empty.
//
// Adapted from GKM (github.com/redhat-et/GKM) pkg/utils/utils.go
func ReplaceUrlTag(imageURL, digest string) string {
	// If invalid input, return empty string
	if imageURL == "" || digest == "" {
		return ""
	}

	// Check if the image already has a digest (e.g., from Kyverno mutation)
	// Format: registry/image:tag@sha256:digest
	if strings.Contains(imageURL, "@") {
		// Image already has a digest, check if it matches
		atIndex := strings.Index(imageURL, "@")
		existingDigest := imageURL[atIndex+1:]
		if existingDigest == digest {
			// Same digest, return as-is
			return imageURL
		}
		// Different digest, replace it
		return imageURL[:atIndex] + "@" + digest
	}

	// Tokenize the Image URL
	lastColonIndex := strings.LastIndex(imageURL, ":")
	if lastColonIndex == -1 {
		// No tag found, append the digest
		return imageURL + "@" + digest
	}

	// Check if the last colon is part of a port number (e.g., registry.io:5000/image)
	// If there's a "/" after the last ":", it's part of the registry address, not a tag
	lastSlashIndex := strings.LastIndex(imageURL, "/")
	if lastSlashIndex > lastColonIndex {
		// Colon is part of port number, append digest
		return imageURL + "@" + digest
	}

	// Extract the part before the tag and append the digest
	return imageURL[:lastColonIndex] + "@" + digest
}

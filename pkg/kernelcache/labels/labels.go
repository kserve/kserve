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

// Package labels contains bounded metadata values shared by KernelCache
// controllers when identifying prefetch Jobs and Pods.
package labels

import (
	"crypto/sha256"
	"encoding/hex"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	KernelCacheNameLabel      = "serving.kserve.io/kernel-cache-name"
	KernelCacheNamespaceLabel = "serving.kserve.io/kernel-cache-namespace"
	KernelCacheNodeLabel      = "serving.kserve.io/kernel-cache-node"

	KernelCacheNameAnnotation      = "internal.serving.kserve.io/kernel-cache-name"
	KernelCacheNamespaceAnnotation = "internal.serving.kserve.io/kernel-cache-namespace"
	KernelCacheNodeAnnotation      = "internal.serving.kserve.io/kernel-cache-node"

	hashedLabelPrefix = "h-"
)

// Value returns a label-safe, deterministic representation of an identity value.
// Values that exceed Kubernetes' label limit are represented by a hash.
func Value(value string) string {
	if value == "" {
		return ""
	}
	if len(value) <= validation.LabelValueMaxLength && len(validation.IsValidLabelValue(value)) == 0 {
		return value
	}

	digest := sha256.Sum256([]byte(value))
	return hashedLabelPrefix + hex.EncodeToString(digest[:])[:validation.LabelValueMaxLength-len(hashedLabelPrefix)]
}

// Values returns the labels used to identify a KernelCache on a node.
func Values(kernelCacheName, namespace, nodeName string) map[string]string {
	return map[string]string{
		KernelCacheNameLabel:      Value(kernelCacheName),
		KernelCacheNamespaceLabel: Value(namespace),
		KernelCacheNodeLabel:      Value(nodeName),
	}
}

// Annotations preserves the original identity values for diagnostics.
func Annotations(kernelCacheName, namespace, nodeName string) map[string]string {
	return map[string]string{
		KernelCacheNameAnnotation:      kernelCacheName,
		KernelCacheNamespaceAnnotation: namespace,
		KernelCacheNodeAnnotation:      nodeName,
	}
}

// Metadata returns bounded labels and full-value annotations for a prefetch object.
func Metadata(kernelCacheName, namespace, nodeName string) (map[string]string, map[string]string) {
	return Values(kernelCacheName, namespace, nodeName), Annotations(kernelCacheName, namespace, nodeName)
}

// ObjectMeta returns metadata suitable for a prefetch object.
func ObjectMeta(kernelCacheName, namespace, nodeName string) metav1.ObjectMeta {
	labels, annotations := Metadata(kernelCacheName, namespace, nodeName)
	return metav1.ObjectMeta{Labels: labels, Annotations: annotations}
}

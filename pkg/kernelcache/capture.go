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

package kernelcache

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
)

// ResolveCaptureOverride returns the capture selected by a workload's
// annotations. An absent annotation returns nil so callers retain automatic
// capture behavior. An explicit reference must resolve to a live capture for the
// same InferenceService; errors must not fall back to an automatic capture.
func ResolveCaptureOverride(ctx context.Context, reader client.Reader, namespace string, annotations map[string]string, sourceName string) (*v1alpha1.KernelCacheCapture, error) {
	name, selected := annotations[constants.KernelCacheCaptureAnnotationKey]
	if !selected {
		return nil, nil
	}
	if problems := validation.IsDNS1123Subdomain(name); len(problems) > 0 {
		return nil, fmt.Errorf("%s must be a valid KernelCacheCapture name: %s", constants.KernelCacheCaptureAnnotationKey, strings.Join(problems, "; "))
	}
	capture := &v1alpha1.KernelCacheCapture{}
	if err := reader.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, capture); err != nil {
		return nil, fmt.Errorf("resolve %s=%q in namespace %q: %w", constants.KernelCacheCaptureAnnotationKey, name, namespace, err)
	}
	if capture.Spec.SourceRef.Kind != "InferenceService" || capture.Spec.SourceRef.Name != sourceName {
		return nil, fmt.Errorf("KernelCacheCapture %s/%s sourceRef must reference InferenceService %q", namespace, name, sourceName)
	}
	if capture.DeletionTimestamp != nil {
		return nil, fmt.Errorf("KernelCacheCapture %s/%s is deleting", namespace, name)
	}
	return capture, nil
}

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

package utils

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/constants"
)

// GetInferenceServiceConfigMap fetches the inferenceservice-config ConfigMap
// from the KServe namespace using the provided client.Reader.
// When called from a controller's Reconcile method, this reads from the
// manager's shared informer cache (no direct API call).
func GetInferenceServiceConfigMap(ctx context.Context, reader client.Reader) (*corev1.ConfigMap, error) {
	configMap := &corev1.ConfigMap{}
	key := client.ObjectKey{
		Name:      constants.InferenceServiceConfigMapName,
		Namespace: constants.KServeNamespace,
	}
	if err := reader.Get(ctx, key, configMap); err != nil {
		return nil, fmt.Errorf("failed to get ConfigMap %s/%s: %w",
			constants.KServeNamespace, constants.InferenceServiceConfigMapName, err)
	}
	return configMap, nil
}

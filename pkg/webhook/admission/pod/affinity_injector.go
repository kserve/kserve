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
package pod

import (
	"os"

	v1 "k8s.io/api/core/v1"
)

const (
	DefaultNodepoolEnvVar   = "KSERVE_DEFAULT_NODEPOOL"
	DefaultNodepoolLabelKey = "KSERVE_DEFAULT_NODEPOOL_LABEL_KEY"
	defaultNodepoolLabelKey = "cloud.google.com/gke-nodepool"
)

// InjectAffinity injects a default affinity config (configurable through environment variables) to each kserve pod
// if it is created without affinity. This allows cluster administrators to route inference workloads to specific
// node pools by default.
func InjectAffinity(pod *v1.Pod) error {
	// If the default nodepool is not set in environment variable, skip injection.
	pool := os.Getenv(DefaultNodepoolEnvVar)
	if len(pool) == 0 {
		return nil
	}

	// If the pod already has affinity configured, skip injection.
	if pod.Spec.Affinity != nil {
		return nil
	}

	// Get the label key from environment variable, or use default.
	labelKey := os.Getenv(DefaultNodepoolLabelKey)
	if len(labelKey) == 0 {
		labelKey = defaultNodepoolLabelKey
	}

	// Add default affinity config to the pod.
	pod.Spec.Affinity = defaultAffinity(labelKey, pool)
	return nil
}

func defaultAffinity(labelKey, pool string) *v1.Affinity {
	return &v1.Affinity{
		NodeAffinity: &v1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
				NodeSelectorTerms: []v1.NodeSelectorTerm{
					{
						MatchExpressions: []v1.NodeSelectorRequirement{
							{
								Key:      labelKey,
								Operator: v1.NodeSelectorOpIn,
								Values:   []string{pool},
							},
						},
					},
				},
			},
		},
	}
}

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

package localmodelnode

import (
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
)

func NewCacheOptions(currentNode string) (cache.Options, error) {
	// Jobs already carry these labels, including jobs created before upgrade.
	jobSelector, _ := labels.Parse("model,node")
	nodeSelector := fields.Everything()
	if currentNode != "" {
		requirement, err := labels.NewRequirement("node", selection.Equals, []string{currentNode})
		if err != nil {
			return cache.Options{}, err
		}
		jobSelector = jobSelector.Add(*requirement)
		nodeSelector = fields.OneTermEqualSelector("metadata.name", currentNode)
	}
	return cache.Options{
		ReaderFailOnMissingInformer: true,
		ByObject: map[client.Object]cache.ByObject{
			&batchv1.Job{}:             {Label: jobSelector},
			&v1alpha1.LocalModelNode{}: {Field: nodeSelector},
		},
	}, nil
}

func NewClientOptions() client.Options {
	return client.Options{Cache: &client.CacheOptions{DisableFor: []client.Object{
		// These lookups do not drive reconciliation; do not create per-agent informers.
		&corev1.Node{}, &v1alpha1.LocalModelNodeGroup{}, &v1alpha1.ClusterStorageContainer{},
	}}}
}

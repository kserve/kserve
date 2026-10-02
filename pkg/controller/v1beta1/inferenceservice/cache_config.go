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

package inferenceservice

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/constants"
)

func NewCacheOptions() (cache.Options, error) {
	// Build a label selector that matches pods with the InferenceService label (any value).
	// This filters at the API server level, so only ISVC pods are cached.
	isvcPodLabelReq, err := labels.NewRequirement(constants.InferenceServicePodLabelKey, selection.Exists, nil)
	if err != nil {
		return cache.Options{}, err
	}
	isvcPodLabelSelector := labels.NewSelector().Add(*isvcPodLabelReq)
	// InferenceService and InferenceGraph share this cache. Their resources
	// carry a common label so both controllers retain their child watches.
	managedSelector := labels.SelectorFromSet(labels.Set{
		constants.KServeManagedLabelKey: constants.KServeManagedLabelValue,
	})

	return cache.Options{
		ByObject: map[client.Object]cache.ByObject{
			&appsv1.Deployment{}: {Label: managedSelector},
			&corev1.Service{}:    {Label: managedSelector},
			&corev1.Pod{}: {
				Label: isvcPodLabelSelector,
			},
		},
	}, nil
}

// NewClientOptions keeps pre-existing, unlabeled resources visible during
// upgrades, including orphan cleanup. Watches still use the filtered cache.
func NewClientOptions() client.Options {
	return client.Options{Cache: &client.CacheOptions{
		DisableFor: []client.Object{&appsv1.Deployment{}, &corev1.Service{}},
	}}
}

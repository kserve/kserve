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

package llmisvc

import (
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
)

func NewCacheOptions() cache.Options {
	selector, _ := metav1.LabelSelectorAsSelector(&ChildResourcesLabelSelector)
	return cache.Options{
		ReaderFailOnMissingInformer: true,
		ByObject: map[client.Object]cache.ByObject{
			&corev1.Secret{}: {Label: selector},
			&corev1.ConfigMap{}: {Namespaces: map[string]cache.Config{
				cache.AllNamespaces: {LabelSelector: selector},
				// Namespace-specific options do not merge with AllNamespaces.
				constants.KServeNamespace: {FieldSelector: fields.OneTermEqualSelector("metadata.name", constants.InferenceServiceConfigMapName)},
			}},
			&appsv1.Deployment{}:                     {Label: selector},
			&corev1.Pod{}:                            {Label: selector},
			&autoscalingv2.HorizontalPodAutoscaler{}: {Label: selector},
		},
	}
}

// NewClientOptions makes lookup-only resources explicit. Their reads must not
// start cluster-wide informers when ReaderFailOnMissingInformer is enabled.
func NewClientOptions() client.Options {
	return client.Options{Cache: &client.CacheOptions{DisableFor: []client.Object{
		&corev1.ServiceAccount{}, &rbacv1.Role{}, &rbacv1.RoleBinding{}, &rbacv1.ClusterRoleBinding{},
		&gwapiv1.GatewayClass{}, &v1alpha1.ClusterStorageContainer{},
		&v1alpha1.LocalModelCache{}, &v1alpha1.LocalModelNamespaceCache{},
		// Runtime fallback must reach discovery when these optional APIs have
		// no informer. Existing watches still enqueue runtime image changes.
		&v1alpha1.ServingRuntime{}, &v1alpha1.ClusterServingRuntime{},
	}}}
}

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
	resourcev1 "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	igwapi "sigs.k8s.io/gateway-api-inference-extension/api/v1"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
	disaggregatedsetv1 "sigs.k8s.io/lws/api/disaggregatedset/v1"
	lwsapi "sigs.k8s.io/lws/api/leaderworkerset/v1"

	kedav1alpha1 "github.com/kedacore/keda/v2/apis/keda/v1alpha1"

	igwapiv1alpha2 "github.com/kserve/kserve/pkg/apis/gie/v1alpha2pool"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
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
func NewClientOptions(cfg *rest.Config) (client.Options, error) {
	disableFor := []client.Object{
		// ServiceAccounts are watched by the controller (enqueueOnServiceAccountChange),
		// so they stay cached; only true lookup-only types read direct.
		&rbacv1.Role{}, &rbacv1.RoleBinding{}, &rbacv1.ClusterRoleBinding{},
		&gwapiv1.GatewayClass{}, &v1alpha1.ClusterStorageContainer{},
		&v1alpha1.LocalModelCache{}, &v1alpha1.LocalModelNamespaceCache{},
		// Runtime fallback must reach discovery when these optional APIs have
		// no informer. Existing watches still enqueue runtime image changes.
		&v1alpha1.ServingRuntime{}, &v1alpha1.ClusterServingRuntime{},
	}

	// Optional CRDs installed after startup never get their informer in this
	// process. Reads must go direct for services that do not use those APIs;
	// ignoreMissing already handles NotFound for the ones that do until a
	// restart registers the controller watches.
	optionalKinds := []struct {
		groupVersion string
		kind         string
		obj          client.Object
	}{
		{gwapiv1.GroupVersion.String(), "HTTPRoute", &gwapiv1.HTTPRoute{}},
		{gwapiv1.GroupVersion.String(), "Gateway", &gwapiv1.Gateway{}},
		{igwapi.GroupVersion.String(), "InferencePool", &igwapi.InferencePool{}},
		{igwapiv1alpha2.GroupVersion.String(), "InferencePool", &igwapiv1alpha2.InferencePool{}},
		{kedav1alpha1.SchemeGroupVersion.String(), "ScaledObject", &kedav1alpha1.ScaledObject{}},
		{lwsapi.GroupVersion.String(), "LeaderWorkerSet", &lwsapi.LeaderWorkerSet{}},
		{disaggregatedsetv1.GroupVersion.String(), "DisaggregatedSet", &disaggregatedsetv1.DisaggregatedSet{}},
		{disaggregatedsetv1.GroupVersion.String(), "DisaggregatedSetRoleScaler", &disaggregatedsetv1.DisaggregatedSetRoleScaler{}},
		{resourcev1.SchemeGroupVersion.String(), "ResourceClaimTemplate", &resourcev1.ResourceClaimTemplate{}},
	}
	for _, optional := range optionalKinds {
		found, err := utils.IsCrdAvailable(cfg, optional.groupVersion, optional.kind)
		if err != nil {
			return client.Options{}, err
		}
		if !found {
			disableFor = append(disableFor, optional.obj)
		}
	}

	return client.Options{Cache: &client.CacheOptions{DisableFor: disableFor}}, nil
}

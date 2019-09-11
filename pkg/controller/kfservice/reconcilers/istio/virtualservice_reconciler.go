/*
Copyright 2019 kubeflow.org.

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

package istio

import (
	log "RRS-Cluster/FrontEnd/PerfTest/LangComp/Go/Log"
	"context"
	"fmt"

	istionetworkingv1alpha3 "github.com/aspenmesh/istio-client-go/pkg/apis/networking/v1alpha3"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha2"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"knative.dev/pkg/kmp"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type VirtualServiceReconciler struct {
	client client.Client
	scheme *runtime.Scheme
}

func NewRouteReconciler(client client.Client, scheme *runtime.Scheme) *RouteReconciler {
	return &RouteReconciler{
		client: client,
		scheme: scheme,
	}
}

func (r *RouteReconciler) Reconcile(kfsvc *v1alpha2.KFService) error {
	desired := istio.NewVirtualServiceBuilder().CreateVirtualService(kfsvc)

	err := r.reconcileVirtualService(kfsvc, desired)
	if err != nil {
		return err
	}

	// TODO: Update parent object's status
	// Possibly the URLs used should be different or else it should appear in describe?
	return nil
}

func (r *RouteReconciler) reconcileVirtualService(kfsvc *v1alpha2.KFService, desired *istionetworkingv1alpha3.VirtualService) error {
	if err := controllerutil.SetControllerReference(kfsvc, desired, r.scheme); err != nil {
		return nil, err
	}

	// Create vanity virtual service if does not exist
	// TODO: does this need to get created in knative-serving
	// TODO: or should we be creating a separate ingress for this?
	existing := &istionetworkingv1alpha3.VirtualService
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, existing)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("Creating Virtual Service", "namespace", desired.Namespace, "name", desired.Name)
			return &desired.Status, r.client.Create(context.TODO(), desired)
		}
		return nil, err
	}

	// Return if no differences to reconcile.
	if routeSemanticEquals(desired, existing) {
		return &existing.Status, nil
	}

	// Reconcile differences and update
	diff, err := kmp.SafeDiff(desired.Spec, existing.Spec)
	if err != nil {
		return nil, fmt.Errorf("failed to diff virtual service: %v", err)
	}
	log.Info("Reconciling virtual service diff (-desired, +observed):", "diff", diff)
	log.Info("Updating virtual service", "namespace", existing.Namespace, "name", existing.Name)
	existing.Spec = desired.Spec
	existing.ObjectMeta.Labels = desired.ObjectMeta.Labels
	existing.ObjectMeta.Annotations = desired.ObjectMeta.Annotations
	err = r.client.Update(context.TODO(), existing)
	if err != nil {
		return &existing.Status, err
	}

	return &existing.Status, nil
}

func routeSemanticEquals(desired, existing *istionetworkingv1alpha3.VirtualService) bool {
	return equality.Semantic.DeepEqual(desired.Spec, existing.Spec) &&
		equality.Semantic.DeepEqual(desired.ObjectMeta.Labels, existing.ObjectMeta.Labels) &&
		equality.Semantic.DeepEqual(desired.ObjectMeta.Annotations, existing.ObjectMeta.Annotations)
}

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
	"context"
	"fmt"

	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha2"
	"github.com/kubeflow/kfserving/pkg/controller/kfservice/resources/istio"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	istiov1alpha3 "knative.dev/pkg/apis/istio/v1alpha3"
	"knative.dev/pkg/kmp"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("VirtualServiceReconciler")

type VirtualServiceReconciler struct {
	client client.Client
	scheme *runtime.Scheme
}

func NewVirtualServiceReconciler(client client.Client, scheme *runtime.Scheme) *VirtualServiceReconciler {
	return &VirtualServiceReconciler{
		client: client,
		scheme: scheme,
	}
}

func (r *VirtualServiceReconciler) Reconcile(kfsvc *v1alpha2.KFService) error {
	desired, status := istio.NewVirtualServiceBuilder().CreateVirtualService(kfsvc)
	if desired == nil {
		kfsvc.Status.PropagateRouteStatus(status)
		return fmt.Errorf("failed to reconcile virtual service")
	}

	if err := r.reconcileVirtualService(kfsvc, desired); err != nil {
		return err
	}

	kfsvc.Status.PropagateRouteStatus(status)

	return nil
}

func (r *VirtualServiceReconciler) reconcileVirtualService(kfsvc *v1alpha2.KFService, desired *istiov1alpha3.VirtualService) error {
	if err := controllerutil.SetControllerReference(kfsvc, desired, r.scheme); err != nil {
		return err
	}

	// Create vanity virtual service if does not exist
	existing := &istiov1alpha3.VirtualService{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, existing)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("Creating Virtual Service", "namespace", desired.Namespace, "name", desired.Name)
			err = r.client.Create(context.TODO(), desired)
		}
		return err
	}

	// Return if no differences to reconcile.
	if routeSemanticEquals(desired, existing) {
		return nil
	}

	// Reconcile differences and update
	diff, err := kmp.SafeDiff(desired.Spec, existing.Spec)
	if err != nil {
		return fmt.Errorf("failed to diff virtual service: %v", err)
	}
	log.Info("Reconciling virtual service diff (-desired, +observed):", "diff", diff)
	log.Info("Updating virtual service", "namespace", existing.Namespace, "name", existing.Name)
	existing.Spec = desired.Spec
	existing.ObjectMeta.Labels = desired.ObjectMeta.Labels
	existing.ObjectMeta.Annotations = desired.ObjectMeta.Annotations
	err = r.client.Update(context.TODO(), existing)
	if err != nil {
		return err
	}

	return nil
}

func routeSemanticEquals(desired, existing *istiov1alpha3.VirtualService) bool {
	return equality.Semantic.DeepEqual(desired.Spec, existing.Spec) &&
		equality.Semantic.DeepEqual(desired.ObjectMeta.Labels, existing.ObjectMeta.Labels) &&
		equality.Semantic.DeepEqual(desired.ObjectMeta.Annotations, existing.ObjectMeta.Annotations)
}

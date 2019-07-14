/*
Copyright 2018 The Knative Authors

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

package clusteringress

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/knative/pkg/apis/istio/v1alpha3"
	istioinformers "github.com/knative/pkg/client/informers/externalversions/istio/v1alpha3"
	istiolisters "github.com/knative/pkg/client/listers/istio/v1alpha3"
	"github.com/knative/pkg/configmap"
	"github.com/knative/pkg/controller"
	"github.com/knative/pkg/logging"
	"github.com/knative/pkg/system"
	"github.com/knative/serving/pkg/apis/networking"
	"github.com/knative/serving/pkg/apis/networking/v1alpha1"
	informers "github.com/knative/serving/pkg/client/informers/externalversions/networking/v1alpha1"
	listers "github.com/knative/serving/pkg/client/listers/networking/v1alpha1"
	"github.com/knative/serving/pkg/network"
	"github.com/knative/serving/pkg/reconciler"
	"github.com/knative/serving/pkg/reconciler/v1alpha1/clusteringress/config"
	"github.com/knative/serving/pkg/reconciler/v1alpha1/clusteringress/resources"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

const (
	controllerAgentName = "clusteringress-controller"
)

// clusterIngressFinalizer is the name that we put into the resource finalizer list, e.g.
//  metadata:
//    finalizers:
//    - clusteringresses.networking.internal.knative.dev
var (
	clusterIngressResource  = v1alpha1.Resource("clusteringresses")
	clusterIngressFinalizer = clusterIngressResource.String()
)

type configStore interface {
	ToContext(ctx context.Context) context.Context
	WatchConfigs(w configmap.Watcher)
}

// Reconciler implements controller.Reconciler for ClusterIngress resources.
type Reconciler struct {
	*reconciler.Base

	// listers index properties about resources
	clusterIngressLister listers.ClusterIngressLister
	virtualServiceLister istiolisters.VirtualServiceLister
	gatewayLister        istiolisters.GatewayLister
	configStore          configStore

	enableReconcilingGateway bool
}

// Check that our Reconciler implements controller.Reconciler
var _ controller.Reconciler = (*Reconciler)(nil)

// NewController initializes the controller and is called by the generated code
// Registers eventhandlers to enqueue events.
func NewController(
	opt reconciler.Options,
	clusterIngressInformer informers.ClusterIngressInformer,
	virtualServiceInformer istioinformers.VirtualServiceInformer,
	gatewayInformer istioinformers.GatewayInformer,
) *controller.Impl {

	c := &Reconciler{
		Base:                     reconciler.NewBase(opt, controllerAgentName),
		clusterIngressLister:     clusterIngressInformer.Lister(),
		virtualServiceLister:     virtualServiceInformer.Lister(),
		gatewayLister:            gatewayInformer.Lister(),
		enableReconcilingGateway: false,
	}
	impl := controller.NewImpl(c, c.Logger, "ClusterIngresses", reconciler.MustNewStatsReporter("ClusterIngress", c.Logger))

	c.Logger.Info("Setting up event handlers")
	myFilterFunc := reconciler.AnnotationFilterFunc(networking.IngressClassAnnotationKey, network.IstioIngressClassName, true)
	ciHandler := cache.FilteringResourceEventHandler{
		FilterFunc: myFilterFunc,
		Handler:    reconciler.Handler(impl.Enqueue),
	}
	clusterIngressInformer.Informer().AddEventHandler(ciHandler)

	virtualServiceInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: myFilterFunc,
		Handler:    reconciler.Handler(impl.EnqueueLabelOfClusterScopedResource(networking.IngressLabelKey)),
	})

	c.Logger.Info("Setting up ConfigMap receivers")
	resyncIngressesOnIstioConfigChange := configmap.TypeFilter(&config.Istio{})(func(string, interface{}) {
		controller.SendGlobalUpdates(clusterIngressInformer.Informer(), ciHandler)
	})
	c.configStore = config.NewStore(c.Logger.Named("config-store"), resyncIngressesOnIstioConfigChange)
	c.configStore.WatchConfigs(opt.ConfigMapWatcher)
	return impl
}

// Reconcile compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the ClusterIngress resource
// with the current status of the resource.
func (c *Reconciler) Reconcile(ctx context.Context, key string) error {
	// Convert the namespace/name string into a distinct namespace and name
	_, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		c.Logger.Errorf("invalid resource key: %s", key)
		return nil
	}
	logger := logging.FromContext(ctx)

	ctx = c.configStore.ToContext(ctx)

	// Get the ClusterIngress resource with this name.
	original, err := c.clusterIngressLister.Get(name)
	if apierrs.IsNotFound(err) {
		// The resource may no longer exist, in which case we stop processing.
		logger.Errorf("clusteringress %q in work queue no longer exists", key)
		return nil
	} else if err != nil {
		return err
	}
	// Don't modify the informers copy
	ci := original.DeepCopy()

	// Reconcile this copy of the ClusterIngress and then write back any status
	// updates regardless of whether the reconciliation errored out.
	err = c.reconcile(ctx, ci)
	if equality.Semantic.DeepEqual(original.Status, ci.Status) {
		// If we didn't change anything then don't call updateStatus.
		// This is important because the copy we loaded from the informer's
		// cache may be stale and we don't want to overwrite a prior update
		// to status with this stale state.
	} else if _, err := c.updateStatus(ci); err != nil {
		logger.Warnw("Failed to update clusterIngress status", zap.Error(err))
		c.Recorder.Eventf(ci, corev1.EventTypeWarning, "UpdateFailed",
			"Failed to update status for ClusterIngress %q: %v", ci.Name, err)
		return err
	}
	if err != nil {
		c.Recorder.Event(ci, corev1.EventTypeWarning, "InternalError", err.Error())
	}
	return err
}

// Update the Status of the ClusterIngress.  Caller is responsible for checking
// for semantic differences before calling.
func (c *Reconciler) updateStatus(desired *v1alpha1.ClusterIngress) (*v1alpha1.ClusterIngress, error) {
	ci, err := c.clusterIngressLister.Get(desired.Name)
	if err != nil {
		return nil, err
	}
	// If there's nothing to update, just return.
	if reflect.DeepEqual(ci.Status, desired.Status) {
		return ci, nil
	}
	// Don't modify the informers copy
	existing := ci.DeepCopy()
	existing.Status = desired.Status
	return c.ServingClientSet.NetworkingV1alpha1().ClusterIngresses().UpdateStatus(existing)
}

func (c *Reconciler) reconcile(ctx context.Context, ci *v1alpha1.ClusterIngress) error {
	logger := logging.FromContext(ctx)
	if ci.GetDeletionTimestamp() != nil {
		return c.reconcileDeletion(ctx, ci)
	}

	// We may be reading a version of the object that was stored at an older version
	// and may not have had all of the assumed defaults specified.  This won't result
	// in this getting written back to the API Server, but lets downstream logic make
	// assumptions about defaulting.
	ci.SetDefaults(ctx)

	ci.Status.InitializeConditions()
	gatewayNames := gatewayNamesFromContext(ctx, ci)
	vs := resources.MakeVirtualService(ci, gatewayNames)

	logger.Infof("Reconciling clusterIngress: %#v", ci)
	logger.Info("Creating/Updating VirtualService")
	if err := c.reconcileVirtualService(ctx, ci, vs); err != nil {
		// TODO(lichuqiang): should we explicitly mark the ingress as unready
		// when error reconciling VirtualService?
		return err
	}

	// As underlying network programming (VirtualService now) is stateless,
	// here we simply mark the ingress as ready if the VirtualService
	// is successfully synced.
	ci.Status.MarkNetworkConfigured()
	ci.Status.MarkLoadBalancerReady(getLBStatus(gatewayServiceURLFromContext(ctx, ci)))
	ci.Status.ObservedGeneration = ci.Generation

	// TODO(zhiminx): Istio requires to put certificates under the namespace where Istio ingress
	// (pods) are deployed so that Istio ingress pods can consume them.
	// So we need to copy certificates from their origin namespace to the Istio ingress namespace.

	// TODO(zhiminx): currently we turn off Gateway reconciliation as it relies
	// on Istio 1.1, which is not ready.
	// We should eventually use a feature flag (in ConfigMap) to turn this on/off.

	if c.enableReconcilingGateway {
		// Add the finalizer before adding `Servers` into Gateway so that we can be sure
		// the `Servers` get cleaned up from Gateway.
		if err := c.ensureFinalizer(ci); err != nil {
			return err
		}

		desiredServers := resources.MakeServers(ci)
		if err := c.reconcileGateways(ctx, ci, gatewayNames, desiredServers); err != nil {
			return err
		}
	}

	// TODO(zhiminx): Mark Route status to indicate that Gateway is configured.

	logger.Info("ClusterIngress successfully synced")
	return nil
}

func getLBStatus(gatewayServiceURL string) []v1alpha1.LoadBalancerIngressStatus {
	// The ClusterIngress isn't load-balanced by any particular
	// Service, but through a Service mesh.
	if gatewayServiceURL == "" {
		return []v1alpha1.LoadBalancerIngressStatus{
			{MeshOnly: true},
		}
	}
	return []v1alpha1.LoadBalancerIngressStatus{
		{DomainInternal: gatewayServiceURL},
	}
}

// gatewayServiceURLFromContext return an address of a load-balancer
// that the given ClusterIngress is exposed to, or empty string if
// none.
func gatewayServiceURLFromContext(ctx context.Context, ci *v1alpha1.ClusterIngress) string {
	cfg := config.FromContext(ctx).Istio
	if len(cfg.IngressGateways) > 0 && ci.IsPublic() {
		return cfg.IngressGateways[0].ServiceURL
	}
	if len(cfg.LocalGateways) > 0 && !ci.IsPublic() {
		return cfg.LocalGateways[0].ServiceURL
	}
	return ""
}

func gatewayNamesFromContext(ctx context.Context, ci *v1alpha1.ClusterIngress) []string {
	gateways := []string{}
	if ci.IsPublic() {
		for _, gw := range config.FromContext(ctx).Istio.IngressGateways {
			gateways = append(gateways, gw.GatewayName)
		}
	} else {
		for _, gw := range config.FromContext(ctx).Istio.LocalGateways {
			gateways = append(gateways, gw.GatewayName)
		}
	}
	return dedup(gateways)
}

func dedup(strs []string) []string {
	existed := sets.NewString()
	unique := []string{}
	// We can't just do `sets.NewString(str)`, since we need to preserve the order.
	for _, s := range strs {
		if !existed.Has(s) {
			existed.Insert(s)
			unique = append(unique, s)
		}
	}
	return unique
}

func (c *Reconciler) reconcileVirtualService(ctx context.Context, ci *v1alpha1.ClusterIngress,
	desired *v1alpha3.VirtualService) error {
	logger := logging.FromContext(ctx)
	ns := desired.Namespace
	name := desired.Name

	vs, err := c.virtualServiceLister.VirtualServices(ns).Get(name)
	if apierrs.IsNotFound(err) {
		_, err = c.SharedClientSet.NetworkingV1alpha3().VirtualServices(ns).Create(desired)
		if err != nil {
			logger.Errorw("Failed to create VirtualService", zap.Error(err))
			c.Recorder.Eventf(ci, corev1.EventTypeWarning, "CreationFailed",
				"Failed to create VirtualService %q/%q: %v", ns, name, err)
			return err
		}
		c.Recorder.Eventf(ci, corev1.EventTypeNormal, "Created",
			"Created VirtualService %q", desired.Name)
	} else if err != nil {
		return err
	} else if !metav1.IsControlledBy(vs, ci) {
		// Surface an error in the ClusterIngress's status, and return an error.
		ci.Status.MarkResourceNotOwned("VirtualService", name)
		return fmt.Errorf("ClusterIngress: %q does not own VirtualService: %q", ci.Name, name)
	} else if !equality.Semantic.DeepEqual(vs.Spec, desired.Spec) {
		// Don't modify the informers copy
		existing := vs.DeepCopy()
		existing.Spec = desired.Spec
		_, err = c.SharedClientSet.NetworkingV1alpha3().VirtualServices(ns).Update(existing)
		if err != nil {
			logger.Errorw("Failed to update VirtualService", zap.Error(err))
			return err
		}
		c.Recorder.Eventf(ci, corev1.EventTypeNormal, "Updated",
			"Updated status for VirtualService %q/%q", ns, name)
	}

	return nil
}

func (c *Reconciler) ensureFinalizer(ci *v1alpha1.ClusterIngress) error {
	finalizers := sets.NewString(ci.Finalizers...)
	if finalizers.Has(clusterIngressFinalizer) {
		return nil
	}

	mergePatch := map[string]interface{}{
		"metadata": map[string]interface{}{
			"finalizers":      append(ci.Finalizers, clusterIngressFinalizer),
			"resourceVersion": ci.ResourceVersion,
		},
	}

	patch, err := json.Marshal(mergePatch)
	if err != nil {
		return err
	}

	_, err = c.ServingClientSet.NetworkingV1alpha1().ClusterIngresses().Patch(ci.Name, types.MergePatchType, patch)
	return err
}

func (c *Reconciler) reconcileDeletion(ctx context.Context, ci *v1alpha1.ClusterIngress) error {
	logger := logging.FromContext(ctx)

	// If our Finalizer is first, delete the `Servers` from Gateway for this ClusterIngress,
	// and remove the finalizer.
	if len(ci.Finalizers) == 0 || ci.Finalizers[0] != clusterIngressFinalizer {
		return nil
	}

	gatewayNames := gatewayNamesFromContext(ctx, ci)
	logger.Infof("Cleaning up Gateway Servers for ClusterIngress %s", ci.Name)
	// No desired Servers means deleting all of the existing Servers associated with the CI.
	if err := c.reconcileGateways(ctx, ci, gatewayNames, []v1alpha3.Server{}); err != nil {
		return err
	}

	// Update the ClusterIngress to remove the Finalizer.
	logger.Info("Removing Finalizer")
	ci.Finalizers = ci.Finalizers[1:]
	_, err := c.ServingClientSet.NetworkingV1alpha1().ClusterIngresses().Update(ci)
	return err
}

func (c *Reconciler) reconcileGateways(ctx context.Context, ci *v1alpha1.ClusterIngress, gatewayNames []string, desiredServers []v1alpha3.Server) error {
	for _, gatewayName := range gatewayNames {
		if err := c.reconcileGateway(ctx, ci, gatewayName, desiredServers); err != nil {
			return err
		}
	}
	return nil
}

func (c *Reconciler) reconcileGateway(ctx context.Context, ci *v1alpha1.ClusterIngress, gatewayName string, desired []v1alpha3.Server) error {
	// TODO(zhiminx): Need to handle the scenario when deleting ClusterIngress. In this scenario,
	// the Gateway servers of the ClusterIngress need also be removed from Gateway.
	logger := logging.FromContext(ctx)
	gateway, err := c.gatewayLister.Gateways(system.Namespace()).Get(gatewayName)
	if err != nil {
		// Not like VirtualService, A default gateway needs to be existed.
		// It should be installed when installing Knative.
		logger.Errorw("Failed to get Gateway.", zap.Error(err))
		return err
	}

	existing := resources.GetServers(gateway, ci)
	if equality.Semantic.DeepEqual(existing, desired) {
		return nil
	}

	copy := gateway.DeepCopy()
	copy = resources.UpdateGateway(copy, desired, existing)
	if _, err := c.SharedClientSet.NetworkingV1alpha3().Gateways(copy.Namespace).Update(copy); err != nil {
		logger.Errorw("Failed to update Gateway", zap.Error(err))
		return err
	}
	c.Recorder.Eventf(ci, corev1.EventTypeNormal, "Updated",
		"Updated Gateway %q/%q", gateway.Namespace, gateway.Name)
	return nil
}

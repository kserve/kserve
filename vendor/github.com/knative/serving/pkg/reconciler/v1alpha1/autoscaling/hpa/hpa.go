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

package hpa

import (
	"context"
	"fmt"
	"reflect"

	perrors "github.com/pkg/errors"
	"go.uber.org/zap"

	"github.com/knative/pkg/controller"
	"github.com/knative/pkg/logging"
	"github.com/knative/serving/pkg/apis/autoscaling"
	pav1alpha1 "github.com/knative/serving/pkg/apis/autoscaling/v1alpha1"
	nv1alpha1 "github.com/knative/serving/pkg/apis/networking/v1alpha1"
	informers "github.com/knative/serving/pkg/client/informers/externalversions/autoscaling/v1alpha1"
	ninformers "github.com/knative/serving/pkg/client/informers/externalversions/networking/v1alpha1"
	listers "github.com/knative/serving/pkg/client/listers/autoscaling/v1alpha1"
	nlisters "github.com/knative/serving/pkg/client/listers/networking/v1alpha1"
	"github.com/knative/serving/pkg/reconciler"
	"github.com/knative/serving/pkg/reconciler/v1alpha1/autoscaling/hpa/resources"
	aresources "github.com/knative/serving/pkg/reconciler/v1alpha1/autoscaling/resources"
	"github.com/knative/serving/pkg/reconciler/v1alpha1/autoscaling/resources/names"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	autoscalingv1informers "k8s.io/client-go/informers/autoscaling/v1"
	autoscalingv1listers "k8s.io/client-go/listers/autoscaling/v1"
	"k8s.io/client-go/tools/cache"
)

const (
	controllerAgentName = "hpa-class-podautoscaler-controller"
)

// Reconciler implements the control loop for the HPA resources.
type Reconciler struct {
	*reconciler.Base

	paLister  listers.PodAutoscalerLister
	sksLister nlisters.ServerlessServiceLister
	hpaLister autoscalingv1listers.HorizontalPodAutoscalerLister
}

var _ controller.Reconciler = (*Reconciler)(nil)

// NewController returns a new HPA reconcile controller.
func NewController(
	opts *reconciler.Options,
	paInformer informers.PodAutoscalerInformer,
	sksInformer ninformers.ServerlessServiceInformer,
	hpaInformer autoscalingv1informers.HorizontalPodAutoscalerInformer,
) *controller.Impl {
	c := &Reconciler{
		Base:      reconciler.NewBase(*opts, controllerAgentName),
		paLister:  paInformer.Lister(),
		hpaLister: hpaInformer.Lister(),
		sksLister: sksInformer.Lister(),
	}
	impl := controller.NewImpl(c, c.Logger, "HPA-Class Autoscaling", reconciler.MustNewStatsReporter("HPA-Class Autoscaling", c.Logger))

	c.Logger.Info("Setting up hpa-class event handlers")
	onlyHpaClass := reconciler.AnnotationFilterFunc(autoscaling.ClassAnnotationKey, autoscaling.HPA, false)
	paInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: onlyHpaClass,
		Handler:    reconciler.Handler(impl.Enqueue),
	})

	hpaInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: onlyHpaClass,
		Handler:    reconciler.Handler(impl.EnqueueControllerOf),
	})

	sksInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: onlyHpaClass,
		Handler:    reconciler.Handler(impl.EnqueueControllerOf),
	})
	return impl
}

// Reconcile is the entry point to the reconciliation control loop.
func (c *Reconciler) Reconcile(ctx context.Context, key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		runtime.HandleError(fmt.Errorf("invalid resource key %s: %v", key, err))
		return nil
	}
	logger := logging.FromContext(ctx)
	logger.Debug("Reconcile hpa-class PodAutoscaler")

	original, err := c.paLister.PodAutoscalers(namespace).Get(name)
	if errors.IsNotFound(err) {
		logger.Debug("PA no longer exists")
		return c.deleteHPA(ctx, key)
	} else if err != nil {
		return err
	}

	if original.Class() != autoscaling.HPA {
		logger.Warn("Ignoring non-hpa-class PA")
		return nil
	}

	// Don't modify the informer's copy.
	pa := original.DeepCopy()
	// Reconcile this copy of the pa and then write back any status
	// updates regardless of whether the reconciliation errored out.
	err = c.reconcile(ctx, key, pa)
	if equality.Semantic.DeepEqual(original.Status, pa.Status) {
		// If we didn't change anything then don't call updateStatus.
		// This is important because the copy we loaded from the informer's
		// cache may be stale and we don't want to overwrite a prior update
		// to status with this stale state.
	} else if _, err := c.updateStatus(pa); err != nil {
		logger.Warnw("Failed to update pa status", zap.Error(err))
		c.Recorder.Eventf(pa, corev1.EventTypeWarning, "UpdateFailed",
			"Failed to update status for PA %q: %v", pa.Name, err)
		return err
	}
	if err != nil {
		c.Recorder.Event(pa, corev1.EventTypeWarning, "InternalError", err.Error())
	}
	return err
}

func (c *Reconciler) reconcile(ctx context.Context, key string, pa *pav1alpha1.PodAutoscaler) error {
	logger := logging.FromContext(ctx)

	if pa.GetDeletionTimestamp() != nil {
		return nil
	}

	// We may be reading a version of the object that was stored at an older version
	// and may not have had all of the assumed defaults specified.  This won't result
	// in this getting written back to the API Server, but lets downstream logic make
	// assumptions about defaulting.
	pa.SetDefaults(ctx)

	pa.Status.InitializeConditions()
	logger.Debug("PA exists")

	// HPA-class PAs don't yet support scale-to-zero
	pa.Status.MarkActive()

	// HPA-class PA delegates autoscaling to the Kubernetes Horizontal Pod Autoscaler.
	desiredHpa := resources.MakeHPA(pa)
	hpa, err := c.hpaLister.HorizontalPodAutoscalers(pa.Namespace).Get(desiredHpa.Name)
	if errors.IsNotFound(err) {
		logger.Infof("Creating HPA %q", desiredHpa.Name)
		if hpa, err = c.KubeClientSet.AutoscalingV1().HorizontalPodAutoscalers(pa.Namespace).Create(desiredHpa); err != nil {
			logger.Errorf("Error creating HPA %q: %v", desiredHpa.Name, err)
			pa.Status.MarkResourceFailedCreation("HorizontalPodAutoscaler", desiredHpa.Name)
			return err
		}
	} else if err != nil {
		logger.Errorf("Error getting existing HPA %q: %v", desiredHpa.Name, err)
		return err
	} else if !metav1.IsControlledBy(hpa, pa) {
		// Surface an error in the PodAutoscaler's status, and return an error.
		pa.Status.MarkResourceNotOwned("HorizontalPodAutoscaler", desiredHpa.Name)
		return fmt.Errorf("PodAutoscaler: %q does not own HPA: %q", pa.Name, desiredHpa.Name)
	}
	if !equality.Semantic.DeepEqual(desiredHpa.Spec, hpa.Spec) {
		logger.Infof("Updating HPA %q", desiredHpa.Name)
		if _, err := c.KubeClientSet.AutoscalingV1().HorizontalPodAutoscalers(pa.Namespace).Update(desiredHpa); err != nil {
			logger.Errorf("Error updating HPA %q: %v", desiredHpa.Name, err)
			return err
		}
	}

	sks, err := c.reconcileSKS(ctx, pa)
	if err != nil {
		return perrors.Wrap(err, "error reconciling SKS")
	}
	// Propagate the service name regardless of the status.
	pa.Status.ServiceName = sks.Status.ServiceName
	if !sks.Status.IsReady() {
		pa.Status.MarkInactive("ServicesNotReady", "SKS Services are not ready yet")
	} else {
		pa.Status.MarkActive()
	}

	pa.Status.ObservedGeneration = pa.Generation
	return nil
}

func (c *Reconciler) reconcileSKS(ctx context.Context, pa *pav1alpha1.PodAutoscaler) (*nv1alpha1.ServerlessService, error) {
	logger := logging.FromContext(ctx)

	sksName := names.SKS(pa.Name)
	sks, err := c.sksLister.ServerlessServices(pa.Namespace).Get(sksName)
	if errors.IsNotFound(err) {
		logger.Infof("SKS %s/%s does not exist; creating.", pa.Namespace, sksName)
		// HPA doesn't scale to zero now, so the mode is always `Serve`.
		sks = aresources.MakeSKS(pa, nv1alpha1.SKSOperationModeServe)
		_, err = c.ServingClientSet.NetworkingV1alpha1().ServerlessServices(sks.Namespace).Create(sks)
		if err != nil {
			return nil, perrors.Wrapf(err, "error creating SKS %s", sksName)
		}
		logger.Info("Created SKS:", sksName)
	} else if err != nil {
		return nil, perrors.Wrapf(err, "error getting SKS: %s", sksName)
	} else if !metav1.IsControlledBy(sks, pa) {
		pa.Status.MarkResourceNotOwned("ServerlessService", sksName)
		return nil, fmt.Errorf("HPA: %q does not own SKS: %q", pa.Name, sksName)
	}
	tmpl := aresources.MakeSKS(pa, nv1alpha1.SKSOperationModeServe)
	if !equality.Semantic.DeepEqual(tmpl.Spec, sks.Spec) {
		want := sks.DeepCopy()
		want.Spec = tmpl.Spec
		logger.Info("SKS changed; reconciling:", sksName)
		// Just deploy the template, since the spec change will change
		// the service and the SKS status will change as a consequence.
		if sks, err = c.ServingClientSet.NetworkingV1alpha1().ServerlessServices(sks.Namespace).Update(want); err != nil {
			return nil, perrors.Wrapf(err, "error updating SKS %s", sksName)
		}
	}
	logger.Debug("Done reconciling SKS:", sksName)
	return sks, nil
}

func (c *Reconciler) deleteHPA(ctx context.Context, key string) error {
	logger := logging.FromContext(ctx)

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}
	err = c.KubeClientSet.AutoscalingV1().HorizontalPodAutoscalers(namespace).Delete(name, nil)
	if errors.IsNotFound(err) {
		// This is fine.
		return nil
	} else if err != nil {
		logger.Errorf("Error deleting HPA %q: %v", name, err)
		return err
	}
	logger.Infof("Deleted HPA %q", name)
	return nil
}

func (c *Reconciler) updateStatus(desired *pav1alpha1.PodAutoscaler) (*pav1alpha1.PodAutoscaler, error) {
	pa, err := c.paLister.PodAutoscalers(desired.Namespace).Get(desired.Name)
	if err != nil {
		return nil, err
	}
	// Check if there is anything to update.
	if !reflect.DeepEqual(pa.Status, desired.Status) {
		// Don't modify the informers copy
		existing := pa.DeepCopy()
		existing.Status = desired.Status
		return c.ServingClientSet.AutoscalingV1alpha1().PodAutoscalers(pa.Namespace).UpdateStatus(existing)
	}
	return pa, nil
}

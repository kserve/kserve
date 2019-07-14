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

package kpa

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
	"github.com/knative/serving/pkg/apis/serving"
	"github.com/knative/serving/pkg/autoscaler"
	informers "github.com/knative/serving/pkg/client/informers/externalversions/autoscaling/v1alpha1"
	ninformers "github.com/knative/serving/pkg/client/informers/externalversions/networking/v1alpha1"
	listers "github.com/knative/serving/pkg/client/listers/autoscaling/v1alpha1"
	nlisters "github.com/knative/serving/pkg/client/listers/networking/v1alpha1"
	"github.com/knative/serving/pkg/reconciler"
	"github.com/knative/serving/pkg/reconciler/v1alpha1/autoscaling/kpa/resources"
	"github.com/knative/serving/pkg/reconciler/v1alpha1/autoscaling/kpa/resources/names"
	aresources "github.com/knative/serving/pkg/reconciler/v1alpha1/autoscaling/resources"
	anames "github.com/knative/serving/pkg/reconciler/v1alpha1/autoscaling/resources/names"
	resourceutil "github.com/knative/serving/pkg/resources"

	autoscalingapi "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/runtime"
	corev1informers "k8s.io/client-go/informers/core/v1"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

const (
	controllerAgentName = "kpa-class-podautoscaler-controller"
)

// Deciders is an interface for notifying the presence or absence of KPAs.
type Deciders interface {
	// Get accesses the Decider resource for this key, returning any errors.
	Get(ctx context.Context, namespace, name string) (*autoscaler.Decider, error)

	// Create adds a Decider resource for a given key, returning any errors.
	Create(ctx context.Context, decider *autoscaler.Decider) (*autoscaler.Decider, error)

	// Delete removes the Decider resource for a given key, returning any errors.
	Delete(ctx context.Context, namespace, name string) error

	// Watch registers a function to call when Decider change.
	Watch(watcher func(string))

	// Update update the Decider resource, return the new Decider or any errors.
	Update(ctx context.Context, decider *autoscaler.Decider) (*autoscaler.Decider, error)
}

// Metrics is an interface for notifying the presence or absence of metric collection.
type Metrics interface {
	// Get accesses the Metric resource for this key, returning any errors.
	Get(ctx context.Context, namespace, name string) (*autoscaler.Metric, error)

	// Create adds a Metric resource for a given key, returning any errors.
	Create(ctx context.Context, metric *autoscaler.Metric) (*autoscaler.Metric, error)

	// Delete removes the Metric resource for a given key, returning any errors.
	Delete(ctx context.Context, namespace, name string) error

	// Update update the Metric resource, return the new Metric or any errors.
	Update(ctx context.Context, metric *autoscaler.Metric) (*autoscaler.Metric, error)
}

// Scaler knows how to scale the targets of KPA-Class PodAutoscalers.
type Scaler interface {
	// Scale attempts to scale the given PA's target to the desired scale.
	Scale(ctx context.Context, pa *pav1alpha1.PodAutoscaler, desiredScale int32) (int32, error)

	// GetScaleResource returns the current scale resource for the PA.
	GetScaleResource(pa *pav1alpha1.PodAutoscaler) (*autoscalingapi.Scale, error)
}

// Reconciler tracks PAs and right sizes the ScaleTargetRef based on the
// information from Deciders.
type Reconciler struct {
	*reconciler.Base
	paLister        listers.PodAutoscalerLister
	serviceLister   corev1listers.ServiceLister
	sksLister       nlisters.ServerlessServiceLister
	endpointsLister corev1listers.EndpointsLister
	kpaDeciders     Deciders
	metrics         Metrics
	scaler          Scaler
	dynConfig       *autoscaler.DynamicConfig
}

// Check that our Reconciler implements controller.Reconciler
var _ controller.Reconciler = (*Reconciler)(nil)

// NewController creates an autoscaling Controller.
func NewController(
	opts *reconciler.Options,
	paInformer informers.PodAutoscalerInformer,
	sksInformer ninformers.ServerlessServiceInformer,
	serviceInformer corev1informers.ServiceInformer,
	endpointsInformer corev1informers.EndpointsInformer,
	kpaDeciders Deciders,
	metrics Metrics,
	scaler Scaler,
	dynConfig *autoscaler.DynamicConfig,
) *controller.Impl {

	c := &Reconciler{
		Base:            reconciler.NewBase(*opts, controllerAgentName),
		paLister:        paInformer.Lister(),
		sksLister:       sksInformer.Lister(),
		serviceLister:   serviceInformer.Lister(),
		endpointsLister: endpointsInformer.Lister(),
		kpaDeciders:     kpaDeciders,
		metrics:         metrics,
		scaler:          scaler,
		dynConfig:       dynConfig,
	}
	impl := controller.NewImpl(c, c.Logger, "KPA-Class Autoscaling", reconciler.MustNewStatsReporter("KPA-Class Autoscaling", c.Logger))

	c.Logger.Info("Setting up KPA-Class event handlers")
	// Handler PodAutoscalers missing the class annotation for backward compatibility.
	onlyKpaClass := reconciler.AnnotationFilterFunc(autoscaling.ClassAnnotationKey, autoscaling.KPA, true)
	paInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: onlyKpaClass,
		Handler:    reconciler.Handler(impl.Enqueue),
	})

	endpointsInformer.Informer().AddEventHandler(
		reconciler.Handler(impl.EnqueueLabelOfNamespaceScopedResource("", autoscaling.KPALabelKey)))

	// Watch all the services that we have created.
	serviceInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: onlyKpaClass,
		Handler:    reconciler.Handler(impl.EnqueueControllerOf),
	})
	sksInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: onlyKpaClass,
		Handler:    reconciler.Handler(impl.EnqueueControllerOf),
	})

	// Have the Deciders enqueue the PAs whose decisions have changed.
	kpaDeciders.Watch(impl.EnqueueKey)
	return impl
}

// Reconcile right sizes PA ScaleTargetRefs based on the state of decisions in Deciders.
func (c *Reconciler) Reconcile(ctx context.Context, key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		runtime.HandleError(fmt.Errorf("invalid resource key %s: %v", key, err))
		return nil
	}
	logger := logging.FromContext(ctx)
	logger.Debug("Reconcile kpa-class PodAutoscaler")

	original, err := c.paLister.PodAutoscalers(namespace).Get(name)
	if errors.IsNotFound(err) {
		logger.Debug("PA no longer exists")
		err = c.kpaDeciders.Delete(ctx, namespace, name)
		err = c.metrics.Delete(ctx, namespace, name)
		return err
	} else if err != nil {
		return err
	}

	if original.Class() != autoscaling.KPA {
		logger.Warn("Ignoring non-kpa-class PA")
		return nil
	}

	// Don't modify the informer's copy.
	pa := original.DeepCopy()

	// Reconcile this copy of the pa and then write back any status
	// updates regardless of whether the reconciliation errored out.
	err = c.reconcile(ctx, pa)
	if equality.Semantic.DeepEqual(original.Status, pa.Status) {
		// If we didn't change anything then don't call updateStatus.
		// This is important because the copy we loaded from the informer's
		// cache may be stale and we don't want to overwrite a prior update
		// to status with this stale state.
	} else if _, err := c.updateStatus(pa); err != nil {
		logger.Warnw("Failed to update kpa status", zap.Error(err))
		c.Recorder.Eventf(pa, corev1.EventTypeWarning, "UpdateFailed",
			"Failed to update status for PA %q: %v", pa.Name, err)
		return err
	}
	if err != nil {
		c.Recorder.Event(pa, corev1.EventTypeWarning, "InternalError", err.Error())
	}
	return err
}

func (c *Reconciler) reconcile(ctx context.Context, pa *pav1alpha1.PodAutoscaler) error {
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

	scale, err := c.scaler.GetScaleResource(pa)
	if err != nil {
		return perrors.Wrap(err, "error retrieving scale")
	}
	selector, err := labels.ConvertSelectorToLabelsMap(scale.Status.Selector)
	if err != nil {
		return perrors.Wrapf(err, "error parsing selector %q", scale.Status.Selector)
	}
	logger.Debugf("PA's %s selector: %v", pa.Name, selector)

	if err := c.reconcileMetricsService(ctx, pa, selector); err != nil {
		return perrors.Wrap(err, "error reconciling metrics service")
	}
	sks, err := c.reconcileSKS(ctx, pa)
	if err != nil {
		return perrors.Wrap(err, "error reconciling SKS")
	}

	decider, err := c.reconcileDecider(ctx, pa, sks.Status.PrivateServiceName)
	if err != nil {
		return perrors.Wrap(err, "error reconciling decider")
	}

	if err := c.reconcileMetric(ctx, pa); err != nil {
		return perrors.Wrap(err, "error reconciling metric")
	}

	// Get the appropriate current scale from the metric, and right size
	// the scaleTargetRef based on it.
	want, err := c.scaler.Scale(ctx, pa, decider.Status.DesiredScale)
	if err != nil {
		return perrors.Wrap(err, "error scaling target")
	}

	// Compare the desired and observed resources to determine our situation.
	// We fetch private endpoints here, since for scaling we're interested in the actual
	// state of the deployment.
	got := 0
	// Propagate service name.
	pa.Status.ServiceName = sks.Status.ServiceName
	if sks.Status.IsReady() {
		got, err = resourceutil.FetchReadyAddressCount(c.endpointsLister, pa.Namespace, sks.Status.PrivateServiceName)
		if err != nil {
			return perrors.Wrapf(err, "error checking endpoints %s", sks.Status.PrivateServiceName)
		}
		logger.Infof("PA got=%v, want=%v", got, want)

	}
	err = reportMetrics(pa, want, got)
	if err != nil {
		return perrors.Wrap(err, "error reporting metrics")
	}

	updatePAStatus(pa, want, got)
	return nil
}

func (c *Reconciler) reconcileDecider(ctx context.Context, pa *pav1alpha1.PodAutoscaler, k8sSvc string) (*autoscaler.Decider, error) {
	desiredDecider := resources.MakeDecider(ctx, pa, c.dynConfig.Current())
	desiredDecider.Labels[serving.KubernetesServiceLabelKey] = k8sSvc
	decider, err := c.kpaDeciders.Get(ctx, desiredDecider.Namespace, desiredDecider.Name)
	if errors.IsNotFound(err) {
		decider, err = c.kpaDeciders.Create(ctx, desiredDecider)
		if err != nil {
			return nil, perrors.Wrap(err, "error creating decider")
		}
	} else if err != nil {
		return nil, perrors.Wrap(err, "error fetching decider")
	}

	// Ignore status when reconciling
	desiredDecider.Status = decider.Status
	if !equality.Semantic.DeepEqual(desiredDecider, decider) {
		decider, err = c.kpaDeciders.Update(ctx, desiredDecider)
		if err != nil {
			return nil, perrors.Wrap(err, "error updating decider")
		}
	}

	return decider, nil
}

func (c *Reconciler) reconcileSKS(ctx context.Context, pa *pav1alpha1.PodAutoscaler) (*nv1alpha1.ServerlessService, error) {
	logger := logging.FromContext(ctx)

	sksName := anames.SKS(pa.Name)
	sks, err := c.sksLister.ServerlessServices(pa.Namespace).Get(sksName)
	if errors.IsNotFound(err) {
		logger.Infof("SKS %s/%s does not exist; creating.", pa.Namespace, sksName)
		sks = aresources.MakeSKS(pa, nv1alpha1.SKSOperationModeServe)
		_, err = c.ServingClientSet.NetworkingV1alpha1().ServerlessServices(sks.Namespace).Create(sks)
		if err != nil {
			return nil, perrors.Wrapf(err, "error creating SKS %s", sksName)
		}
		logger.Info("Created SKS:", sksName)
	} else if err != nil {
		return nil, perrors.Wrapf(err, "error getting SKS %s", sksName)
	} else if !metav1.IsControlledBy(sks, pa) {
		pa.Status.MarkResourceNotOwned("ServerlessService", sksName)
		return nil, fmt.Errorf("KPA: %s does not own SKS: %s", pa.Name, sksName)
	}
	tmpl := aresources.MakeSKS(pa, nv1alpha1.SKSOperationModeServe)
	if !equality.Semantic.DeepEqual(tmpl.Spec, sks.Spec) {
		want := sks.DeepCopy()
		want.Spec = tmpl.Spec
		logger.Info("SKS changed; reconciling:", sksName)
		if sks, err = c.ServingClientSet.NetworkingV1alpha1().ServerlessServices(sks.Namespace).Update(want); err != nil {
			return nil, perrors.Wrapf(err, "error updating SKS %s", sksName)
		}
	}
	logger.Debug("Done reconciling SKS", sksName)
	return sks, nil
}

func (c *Reconciler) reconcileMetricsService(ctx context.Context, pa *pav1alpha1.PodAutoscaler, selector map[string]string) error {
	logger := logging.FromContext(ctx)

	sn := names.MetricsServiceName(pa.Name)
	svc, err := c.serviceLister.Services(pa.Namespace).Get(sn)
	if errors.IsNotFound(err) {
		logger.Infof("K8s service %s/%s does not exist; creating.", pa.Namespace, sn)
		svc = resources.MakeMetricsService(pa, selector)
		_, err := c.KubeClientSet.CoreV1().Services(pa.Namespace).Create(svc)
		if err != nil {
			return perrors.Wrapf(err, "error creating K8s Service %s/%s", pa.Namespace, sn)
		}
		logger.Info("Created K8s service:", sn)
	} else if err != nil {
		return perrors.Wrapf(err, "error getting K8s Service %s", sn)
	} else if !metav1.IsControlledBy(svc, pa) {
		pa.Status.MarkResourceNotOwned("Service", sn)
		return fmt.Errorf("KPA: %s does not own Service: %s", pa.Name, sn)
	}
	tmpl := resources.MakeMetricsService(pa, selector)
	want := svc.DeepCopy()
	want.Spec.Ports = tmpl.Spec.Ports
	want.Spec.Selector = tmpl.Spec.Selector

	if !equality.Semantic.DeepEqual(want.Spec, svc.Spec) {
		logger.Info("Metrics K8s Service changed; reconciling:", sn)
		if _, err = c.KubeClientSet.CoreV1().Services(pa.Namespace).Update(want); err != nil {
			return perrors.Wrapf(err, "error updating K8s Service %s", sn)
		}
	}
	logger.Debug("Done reconciling metrics K8s service", sn)
	return nil
}

func (c *Reconciler) reconcileMetric(ctx context.Context, pa *pav1alpha1.PodAutoscaler) error {
	desiredMetric := resources.MakeMetric(ctx, pa, c.dynConfig.Current())
	metric, err := c.metrics.Get(ctx, desiredMetric.Namespace, desiredMetric.Name)
	if errors.IsNotFound(err) {
		metric, err = c.metrics.Create(ctx, desiredMetric)
		if err != nil {
			return perrors.Wrap(err, "error creating metric")
		}
	} else if err != nil {
		return perrors.Wrap(err, "error fetching metric")
	}

	// Ignore status when reconciling
	desiredMetric.Status = metric.Status
	if !equality.Semantic.DeepEqual(desiredMetric, metric) {
		metric, err = c.metrics.Update(ctx, desiredMetric)
		if err != nil {
			return perrors.Wrap(err, "error updating metric")
		}
	}

	return nil
}

func reportMetrics(pa *pav1alpha1.PodAutoscaler, want int32, got int) error {
	var serviceLabel string
	var configLabel string
	if pa.Labels != nil {
		serviceLabel = pa.Labels[serving.ServiceLabelKey]
		configLabel = pa.Labels[serving.ConfigurationLabelKey]
	}
	reporter, err := autoscaler.NewStatsReporter(pa.Namespace, serviceLabel, configLabel, pa.Name)
	if err != nil {
		return err
	}

	reporter.ReportActualPodCount(int64(got))
	// Negative "want" values represent an empty metrics pipeline and thus no specific request is being made.
	if want >= 0 {
		reporter.ReportRequestedPodCount(int64(want))
	}
	return nil
}

func updatePAStatus(pa *pav1alpha1.PodAutoscaler, want int32, got int) {
	switch {
	case want == 0:
		pa.Status.MarkInactive("NoTraffic", "The target is not receiving traffic.")

	case got == 0 && want != 0:
		pa.Status.MarkActivating(
			"Queued", "Requests to the target are being buffered as resources are provisioned.")

	case got > 0:
		pa.Status.MarkActive()
	}

	pa.Status.ObservedGeneration = pa.Generation
}

func (c *Reconciler) updateStatus(desired *pav1alpha1.PodAutoscaler) (*pav1alpha1.PodAutoscaler, error) {
	pa, err := c.paLister.PodAutoscalers(desired.Namespace).Get(desired.Name)
	if err != nil {
		return nil, err
	}
	// If there's nothing to update, just return.
	if reflect.DeepEqual(pa.Status, desired.Status) {
		return pa, nil
	}
	// Don't modify the informers copy
	existing := pa.DeepCopy()
	existing.Status = desired.Status

	return c.ServingClientSet.AutoscalingV1alpha1().PodAutoscalers(pa.Namespace).UpdateStatus(existing)
}

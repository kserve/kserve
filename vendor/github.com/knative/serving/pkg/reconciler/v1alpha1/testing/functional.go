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

package testing

import (
	"fmt"
	"time"

	"github.com/knative/pkg/apis/duck"
	duckv1alpha1 "github.com/knative/pkg/apis/duck/v1alpha1"
	duckv1beta1 "github.com/knative/pkg/apis/duck/v1beta1"
	"github.com/knative/pkg/ptr"
	"github.com/knative/serving/pkg/apis/autoscaling"
	autoscalingv1alpha1 "github.com/knative/serving/pkg/apis/autoscaling/v1alpha1"
	"github.com/knative/serving/pkg/apis/networking"
	netv1alpha1 "github.com/knative/serving/pkg/apis/networking/v1alpha1"
	"github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"github.com/knative/serving/pkg/apis/serving/v1beta1"
	routenames "github.com/knative/serving/pkg/reconciler/v1alpha1/route/resources/names"
	"github.com/knative/serving/pkg/reconciler/v1alpha1/route/traffic"
	"github.com/knative/serving/pkg/reconciler/v1alpha1/serverlessservice/resources/names"
	servicenames "github.com/knative/serving/pkg/reconciler/v1alpha1/service/resources/names"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

// BuildOption enables further configuration of a Build.
type BuildOption func(*unstructured.Unstructured)

// WithSucceededTrue updates the status of the provided unstructured Build object with the
// expected success condition.
func WithSucceededTrue(orig *unstructured.Unstructured) {
	cp := orig.DeepCopy()
	cp.Object["status"] = map[string]interface{}{"conditions": duckv1alpha1.Conditions{{
		Type:   duckv1alpha1.ConditionSucceeded,
		Status: corev1.ConditionTrue,
	}}}
	duck.FromUnstructured(cp, orig) // prevent panic in b.DeepCopy()
}

// WithSucceededUnknown updates the status of the provided unstructured Build object with the
// expected in-flight condition.
func WithSucceededUnknown(reason, message string) BuildOption {
	return func(orig *unstructured.Unstructured) {
		cp := orig.DeepCopy()
		cp.Object["status"] = map[string]interface{}{"conditions": duckv1alpha1.Conditions{{
			Type:    duckv1alpha1.ConditionSucceeded,
			Status:  corev1.ConditionUnknown,
			Reason:  reason,
			Message: message,
		}}}
		duck.FromUnstructured(cp, orig) // prevent panic in b.DeepCopy()
	}
}

// WithSucceededFalse updates the status of the provided unstructured Build object with the
// expected failure condition.
func WithSucceededFalse(reason, message string) BuildOption {
	return func(orig *unstructured.Unstructured) {
		cp := orig.DeepCopy()
		cp.Object["status"] = map[string]interface{}{"conditions": duckv1alpha1.Conditions{{
			Type:    duckv1alpha1.ConditionSucceeded,
			Status:  corev1.ConditionFalse,
			Reason:  reason,
			Message: message,
		}}}
		duck.FromUnstructured(cp, orig) // prevent panic in b.DeepCopy()
	}
}

// ServiceOption enables further configuration of a Service.
type ServiceOption func(*v1alpha1.Service)

var (
	// configSpec is the spec used for the different styles of Service rollout.
	configSpec = v1alpha1.ConfigurationSpec{
		RevisionTemplate: &v1alpha1.RevisionTemplateSpec{
			Spec: v1alpha1.RevisionSpec{
				Container: &corev1.Container{
					Image: "busybox",
				},
				RevisionSpec: v1beta1.RevisionSpec{
					TimeoutSeconds: ptr.Int64(60),
				},
			},
		},
	}
)

// WithServiceDeletionTimestamp will set the DeletionTimestamp on the Service.
func WithServiceDeletionTimestamp(r *v1alpha1.Service) {
	t := metav1.NewTime(time.Unix(1e9, 0))
	r.ObjectMeta.SetDeletionTimestamp(&t)
}

// WithRunLatestRollout configures the Service to use a "runLatest" rollout.
func WithRunLatestRollout(s *v1alpha1.Service) {
	s.Spec = v1alpha1.ServiceSpec{
		RunLatest: &v1alpha1.RunLatestType{
			Configuration: *configSpec.DeepCopy(),
		},
	}
}

// WithRunLatestConfigSpec confgures the Service to use a "runLastest" configuration
func WithRunLatestConfigSpec(config v1alpha1.ConfigurationSpec) ServiceOption {
	return func(svc *v1alpha1.Service) {
		svc.Spec = v1alpha1.ServiceSpec{
			RunLatest: &v1alpha1.RunLatestType{
				Configuration: config,
			},
		}
	}
}

// WithServiceLabel attaches a particular label to the service.
func WithServiceLabel(key, value string) ServiceOption {
	return func(service *v1alpha1.Service) {
		if service.Labels == nil {
			service.Labels = make(map[string]string)
		}
		service.Labels[key] = value
	}
}

// WithResourceRequirements attaches resource requirements to the service
func WithResourceRequirements(resourceRequirements corev1.ResourceRequirements) ServiceOption {
	return func(svc *v1alpha1.Service) {
		svc.Spec.RunLatest.Configuration.GetTemplate().Spec.GetContainer().Resources = resourceRequirements
	}
}

// WithVolume adds a volume to the service
func WithVolume(name, mountPath string, volumeSource corev1.VolumeSource) ServiceOption {
	return func(svc *v1alpha1.Service) {
		rt := &svc.Spec.RunLatest.Configuration.GetTemplate().Spec

		rt.GetContainer().VolumeMounts = []corev1.VolumeMount{{
			Name:      name,
			MountPath: mountPath,
		}}

		rt.Volumes = []corev1.Volume{{
			Name:         name,
			VolumeSource: volumeSource,
		}}
	}
}

// WithConfigAnnotations assigns config annotations to a service
func WithConfigAnnotations(annotations map[string]string) ServiceOption {
	return func(service *v1alpha1.Service) {
		service.Spec.RunLatest.Configuration.GetTemplate().ObjectMeta.Annotations = annotations
	}
}

// WithRevisionTimeoutSeconds sets revision timeout
func WithRevisionTimeoutSeconds(revisionTimeoutSeconds int64) ServiceOption {
	return func(service *v1alpha1.Service) {
		service.Spec.RunLatest.Configuration.GetTemplate().Spec.TimeoutSeconds = ptr.Int64(revisionTimeoutSeconds)
	}
}

// MarkConfigurationNotOwned calls the function of the same name on the Service's status.
func MarkConfigurationNotOwned(service *v1alpha1.Service) {
	service.Status.MarkConfigurationNotOwned(servicenames.Configuration(service))
}

// MarkRouteNotOwned calls the function of the same name on the Service's status.
func MarkRouteNotOwned(service *v1alpha1.Service) {
	service.Status.MarkRouteNotOwned(servicenames.Route(service))
}

// WithPinnedRollout configures the Service to use a "pinned" rollout,
// which is pinned to the named revision.
// Deprecated, since PinnedType is deprecated.
func WithPinnedRollout(name string) ServiceOption {
	return WithPinnedRolloutConfigSpec(name, *configSpec.DeepCopy())
}

// WithPinnedRolloutConfigSpec WithPinnedRollout2
func WithPinnedRolloutConfigSpec(name string, config v1alpha1.ConfigurationSpec) ServiceOption {
	return func(s *v1alpha1.Service) {
		s.Spec = v1alpha1.ServiceSpec{
			DeprecatedPinned: &v1alpha1.PinnedType{
				RevisionName:  name,
				Configuration: config,
			},
		}
	}
}

// WithReleaseRolloutAndPercentage configures the Service to use a "release" rollout,
// which spans the provided revisions.
func WithReleaseRolloutAndPercentage(percentage int, names ...string) ServiceOption {
	return WithReleaseRolloutAndPercentageConfigSpec(percentage, *configSpec.DeepCopy(),
		names...)
}

// WithReleaseRolloutAndPercentageConfigSpec configures the Service to use a "release" rollout,
// which spans the provided revisions.
func WithReleaseRolloutAndPercentageConfigSpec(percentage int, config v1alpha1.ConfigurationSpec, names ...string) ServiceOption {
	return func(s *v1alpha1.Service) {
		s.Spec = v1alpha1.ServiceSpec{
			Release: &v1alpha1.ReleaseType{
				Revisions:      names,
				RolloutPercent: percentage,
				Configuration:  config,
			},
		}
	}
}

// WithReleaseRolloutConfigSpec configures the Service to use a "release" rollout,
// which spans the provided revisions.
func WithReleaseRolloutConfigSpec(config v1alpha1.ConfigurationSpec, names ...string) ServiceOption {
	return func(s *v1alpha1.Service) {
		s.Spec = v1alpha1.ServiceSpec{
			Release: &v1alpha1.ReleaseType{
				Revisions:     names,
				Configuration: config,
			},
		}
	}
}

// WithReleaseRollout configures the Service to use a "release" rollout,
// which spans the provided revisions.
func WithReleaseRollout(names ...string) ServiceOption {
	return func(s *v1alpha1.Service) {
		s.Spec = v1alpha1.ServiceSpec{
			Release: &v1alpha1.ReleaseType{
				Revisions:     names,
				Configuration: *configSpec.DeepCopy(),
			},
		}
	}
}

// WithManualRollout configures the Service to use a "manual" rollout.
func WithManualRollout(s *v1alpha1.Service) {
	s.Spec = v1alpha1.ServiceSpec{
		Manual: &v1alpha1.ManualType{},
	}
}

// WithInitSvcConditions initializes the Service's conditions.
func WithInitSvcConditions(s *v1alpha1.Service) {
	s.Status.InitializeConditions()
}

// WithManualStatus configures the Service to have the appropriate
// status for a "manual" rollout type.
func WithManualStatus(s *v1alpha1.Service) {
	s.Status.SetManualStatus()
}

// WithReadyRoute reflects the Route's readiness in the Service resource.
func WithReadyRoute(s *v1alpha1.Service) {
	s.Status.PropagateRouteStatus(&v1alpha1.RouteStatus{
		Status: duckv1beta1.Status{
			Conditions: duckv1beta1.Conditions{{
				Type:   "Ready",
				Status: "True",
			}},
		},
	})
}

// WithSvcStatusDomain propagates the domain name to the status of the Service.
func WithSvcStatusDomain(s *v1alpha1.Service) {
	n, ns := s.GetName(), s.GetNamespace()
	s.Status.Domain = fmt.Sprintf("%s.%s.example.com", n, ns)
	s.Status.DeprecatedDomainInternal = fmt.Sprintf("%s.%s.svc.cluster.local", n, ns)
}

// WithSvcStatusAddress updates the service's status with the address.
func WithSvcStatusAddress(s *v1alpha1.Service) {
	s.Status.Address = &duckv1alpha1.Addressable{
		Hostname: fmt.Sprintf("%s.%s.svc.cluster.local", s.Name, s.Namespace),
	}
}

// WithSvcStatusTraffic sets the Service's status traffic block to the specified traffic targets.
func WithSvcStatusTraffic(targets ...v1alpha1.TrafficTarget) ServiceOption {
	return func(r *v1alpha1.Service) {
		// Automatically inject URL into TrafficTarget status
		for _, tt := range targets {
			if tt.Name != "" {
				tt.URL = traffic.TagURL(traffic.HTTPScheme,
					tt.Name,
					"example.com")
			}
		}
		r.Status.Traffic = targets

	}
}

// WithFailedRoute reflects a Route's failure in the Service resource.
func WithFailedRoute(reason, message string) ServiceOption {
	return func(s *v1alpha1.Service) {
		s.Status.PropagateRouteStatus(&v1alpha1.RouteStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:    "Ready",
					Status:  "False",
					Reason:  reason,
					Message: message,
				}},
			},
		})
	}
}

// WithReadyConfig reflects the Configuration's readiness in the Service
// resource.  This must coincide with the setting of Latest{Created,Ready}
// to the provided revision name.
func WithReadyConfig(name string) ServiceOption {
	return func(s *v1alpha1.Service) {
		s.Status.PropagateConfigurationStatus(&v1alpha1.ConfigurationStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   "Ready",
					Status: "True",
				}},
			},
			ConfigurationStatusFields: v1alpha1.ConfigurationStatusFields{
				LatestCreatedRevisionName: name,
				LatestReadyRevisionName:   name,
			},
		})
	}
}

// WithFailedConfig reflects the Configuration's failure in the Service
// resource.  The failing revision's name is reflected in LatestCreated.
func WithFailedConfig(name, reason, message string) ServiceOption {
	return func(s *v1alpha1.Service) {
		s.Status.PropagateConfigurationStatus(&v1alpha1.ConfigurationStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:   "Ready",
					Status: "False",
					Reason: reason,
					Message: fmt.Sprintf("Revision %q failed with message: %s.",
						name, message),
				}},
			},
			ConfigurationStatusFields: v1alpha1.ConfigurationStatusFields{
				LatestCreatedRevisionName: name,
			},
		})
	}
}

// WithServiceLatestReadyRevision sets the latest ready revision on the Service's status.
func WithServiceLatestReadyRevision(lrr string) ServiceOption {
	return func(s *v1alpha1.Service) {
		s.Status.LatestReadyRevisionName = lrr
	}
}

// WithServiceStatusRouteNotReady sets the `RoutesReady` condition on the service to `Unknown`.
func WithServiceStatusRouteNotReady(s *v1alpha1.Service) {
	s.Status.MarkRouteNotYetReady()
}

// RouteOption enables further configuration of a Route.
type RouteOption func(*v1alpha1.Route)

// WithSpecTraffic sets the Route's traffic block to the specified traffic targets.
func WithSpecTraffic(traffic ...v1alpha1.TrafficTarget) RouteOption {
	return func(r *v1alpha1.Route) {
		r.Spec.Traffic = traffic
	}
}

// WithRouteUID sets the Route's UID
func WithRouteUID(uid types.UID) RouteOption {
	return func(r *v1alpha1.Route) {
		r.ObjectMeta.UID = uid
	}
}

// WithRouteDeletionTimestamp will set the DeletionTimestamp on the Route.
func WithRouteDeletionTimestamp(r *v1alpha1.Route) {
	t := metav1.NewTime(time.Unix(1e9, 0))
	r.ObjectMeta.SetDeletionTimestamp(&t)
}

// WithRouteFinalizer adds the Route finalizer to the Route.
func WithRouteFinalizer(r *v1alpha1.Route) {
	r.ObjectMeta.Finalizers = append(r.ObjectMeta.Finalizers, "routes.serving.knative.dev")
}

// WithAnotherRouteFinalizer adds a non-Route finalizer to the Route.
func WithAnotherRouteFinalizer(r *v1alpha1.Route) {
	r.ObjectMeta.Finalizers = append(r.ObjectMeta.Finalizers, "another.serving.knative.dev")
}

// WithConfigTarget sets the Route's traffic block to point at a particular Configuration.
func WithConfigTarget(config string) RouteOption {
	return WithSpecTraffic(v1alpha1.TrafficTarget{
		TrafficTarget: v1beta1.TrafficTarget{
			ConfigurationName: config,
			Percent:           100,
		},
	})
}

// WithRevTarget sets the Route's traffic block to point at a particular Revision.
func WithRevTarget(revision string) RouteOption {
	return WithSpecTraffic(v1alpha1.TrafficTarget{
		TrafficTarget: v1beta1.TrafficTarget{
			RevisionName: revision,
			Percent:      100,
		},
	})
}

// WithStatusTraffic sets the Route's status traffic block to the specified traffic targets.
func WithStatusTraffic(traffic ...v1alpha1.TrafficTarget) RouteOption {
	return func(r *v1alpha1.Route) {
		r.Status.Traffic = traffic
	}
}

// WithRouteOwnersRemoved clears the owner references of this Route.
func WithRouteOwnersRemoved(r *v1alpha1.Route) {
	r.OwnerReferences = nil
}

// MarkServiceNotOwned calls the function of the same name on the Service's status.
func MarkServiceNotOwned(r *v1alpha1.Route) {
	r.Status.MarkServiceNotOwned(routenames.K8sService(r))
}

// WithDomain sets the .Status.Domain field to the prototypical domain.
func WithDomain(r *v1alpha1.Route) {
	r.Status.Domain = fmt.Sprintf("%s.%s.example.com", r.Name, r.Namespace)
}

// WithDomainInternal sets the .Status.DomainInternal field to the prototypical internal domain.
func WithDomainInternal(r *v1alpha1.Route) {
	r.Status.DeprecatedDomainInternal = fmt.Sprintf("%s.%s.svc.cluster.local", r.Name, r.Namespace)
}

// WithAddress sets the .Status.Address field to the prototypical internal hostname.
func WithAddress(r *v1alpha1.Route) {
	r.Status.Address = &duckv1alpha1.Addressable{
		Hostname: fmt.Sprintf("%s.%s.svc.cluster.local", r.Name, r.Namespace),
	}
}

// WithAnotherDomain sets the .Status.Domain field to an atypical domain.
func WithAnotherDomain(r *v1alpha1.Route) {
	r.Status.Domain = fmt.Sprintf("%s.%s.another-example.com", r.Name, r.Namespace)
}

// WithLocalDomain sets the .Status.Domain field to use `svc.cluster.local` suffix.
func WithLocalDomain(r *v1alpha1.Route) {
	r.Status.Domain = fmt.Sprintf("%s.%s.svc.cluster.local", r.Name, r.Namespace)
}

// WithInitRouteConditions initializes the Service's conditions.
func WithInitRouteConditions(rt *v1alpha1.Route) {
	rt.Status.InitializeConditions()
}

// MarkTrafficAssigned calls the method of the same name on .Status
func MarkTrafficAssigned(r *v1alpha1.Route) {
	r.Status.MarkTrafficAssigned()
}

// MarkIngressReady propagates a Ready=True ClusterIngress status to the Route.
func MarkIngressReady(r *v1alpha1.Route) {
	r.Status.PropagateClusterIngressStatus(netv1alpha1.IngressStatus{
		Status: duckv1beta1.Status{
			Conditions: duckv1beta1.Conditions{{
				Type:   "Ready",
				Status: "True",
			}},
		},
	})
}

// MarkMissingTrafficTarget calls the method of the same name on .Status
func MarkMissingTrafficTarget(kind, revision string) RouteOption {
	return func(r *v1alpha1.Route) {
		r.Status.MarkMissingTrafficTarget(kind, revision)
	}
}

// MarkConfigurationNotReady calls the method of the same name on .Status
func MarkConfigurationNotReady(name string) RouteOption {
	return func(r *v1alpha1.Route) {
		r.Status.MarkConfigurationNotReady(name)
	}
}

// MarkConfigurationFailed calls the method of the same name on .Status
func MarkConfigurationFailed(name string) RouteOption {
	return func(r *v1alpha1.Route) {
		r.Status.MarkConfigurationFailed(name)
	}
}

// WithRouteLabel sets the specified label on the Route.
func WithRouteLabel(key, value string) RouteOption {
	return func(r *v1alpha1.Route) {
		if r.Labels == nil {
			r.Labels = make(map[string]string)
		}
		r.Labels[key] = value
	}
}

// WithIngressClass sets the ingress class annotation on the Route.
func WithIngressClass(ingressClass string) RouteOption {
	return func(r *v1alpha1.Route) {
		if r.Annotations == nil {
			r.Annotations = make(map[string]string)
		}
		r.Annotations[networking.IngressClassAnnotationKey] = ingressClass
	}
}

// ConfigOption enables further configuration of a Configuration.
type ConfigOption func(*v1alpha1.Configuration)

// WithConfigDeletionTimestamp will set the DeletionTimestamp on the Config.
func WithConfigDeletionTimestamp(r *v1alpha1.Configuration) {
	t := metav1.NewTime(time.Unix(1e9, 0))
	r.ObjectMeta.SetDeletionTimestamp(&t)
}

// WithBuild adds a Build to the provided Configuration.
func WithBuild(cfg *v1alpha1.Configuration) {
	cfg.Spec.Build = &v1alpha1.RawExtension{
		Object: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "testing.build.knative.dev/v1alpha1",
				"kind":       "Build",
				"spec": map[string]interface{}{
					"steps": []interface{}{
						map[string]interface{}{
							"image": "foo",
						},
						map[string]interface{}{
							"image": "bar",
						},
					},
				},
			},
		},
	}
}

// WithConfigOwnersRemoved clears the owner references of this Configuration.
func WithConfigOwnersRemoved(cfg *v1alpha1.Configuration) {
	cfg.OwnerReferences = nil
}

// WithConfigConcurrencyModel sets the given Configuration's concurrency model.
func WithConfigConcurrencyModel(ss v1alpha1.RevisionRequestConcurrencyModelType) ConfigOption {
	return func(cfg *v1alpha1.Configuration) {
		cfg.Spec.GetTemplate().Spec.DeprecatedConcurrencyModel = ss
	}
}

// WithGeneration sets the generation of the Configuration.
func WithGeneration(gen int64) ConfigOption {
	return func(cfg *v1alpha1.Configuration) {
		cfg.Generation = gen
		//TODO(dprotaso) remove this for 0.4 release
		cfg.Spec.DeprecatedGeneration = gen
	}
}

// WithObservedGen sets the observed generation of the Configuration.
func WithObservedGen(cfg *v1alpha1.Configuration) {
	cfg.Status.ObservedGeneration = cfg.Generation
}

// WithCreatedAndReady sets the latest{Created,Ready}RevisionName on the Configuration.
func WithCreatedAndReady(created, ready string) ConfigOption {
	return func(cfg *v1alpha1.Configuration) {
		cfg.Status.SetLatestCreatedRevisionName(created)
		cfg.Status.SetLatestReadyRevisionName(ready)
	}
}

// WithLatestCreated initializes the .status.latestCreatedRevisionName to be the name
// of the latest revision that the Configuration would have created.
func WithLatestCreated(name string) ConfigOption {
	return func(cfg *v1alpha1.Configuration) {
		cfg.Status.SetLatestCreatedRevisionName(name)
	}
}

// WithLatestReady initializes the .status.latestReadyRevisionName to be the name
// of the latest revision that the Configuration would have created.
func WithLatestReady(name string) ConfigOption {
	return func(cfg *v1alpha1.Configuration) {
		cfg.Status.SetLatestReadyRevisionName(name)
	}
}

// MarkRevisionCreationFailed calls .Status.MarkRevisionCreationFailed.
func MarkRevisionCreationFailed(msg string) ConfigOption {
	return func(cfg *v1alpha1.Configuration) {
		cfg.Status.MarkRevisionCreationFailed(msg)
	}
}

// MarkLatestCreatedFailed calls .Status.MarkLatestCreatedFailed.
func MarkLatestCreatedFailed(msg string) ConfigOption {
	return func(cfg *v1alpha1.Configuration) {
		cfg.Status.MarkLatestCreatedFailed(cfg.Status.LatestCreatedRevisionName, msg)
	}
}

// WithConfigLabel attaches a particular label to the configuration.
func WithConfigLabel(key, value string) ConfigOption {
	return func(config *v1alpha1.Configuration) {
		if config.Labels == nil {
			config.Labels = make(map[string]string)
		}
		config.Labels[key] = value
	}
}

// RevisionOption enables further configuration of a Revision.
type RevisionOption func(*v1alpha1.Revision)

// WithRevisionDeletionTimestamp will set the DeletionTimestamp on the Revision.
func WithRevisionDeletionTimestamp(r *v1alpha1.Revision) {
	t := metav1.NewTime(time.Unix(1e9, 0))
	r.ObjectMeta.SetDeletionTimestamp(&t)
}

// WithInitRevConditions calls .Status.InitializeConditions() on a Revision.
func WithInitRevConditions(r *v1alpha1.Revision) {
	r.Status.InitializeConditions()
}

// WithRevName sets the name of the revision
func WithRevName(name string) RevisionOption {
	return func(rev *v1alpha1.Revision) {
		rev.Name = name
	}
}

// WithBuildRef sets the .Spec.BuildRef on the Revision to match what we'd get
// using WithBuild(name).
func WithBuildRef(name string) RevisionOption {
	return func(rev *v1alpha1.Revision) {
		rev.Spec.BuildRef = &corev1.ObjectReference{
			APIVersion: "testing.build.knative.dev/v1alpha1",
			Kind:       "Build",
			Name:       name,
		}
	}
}

// WithServiceName propagates the given service name to the revision status.
func WithServiceName(sn string) RevisionOption {
	return func(rev *v1alpha1.Revision) {
		rev.Status.ServiceName = sn
	}
}

// MarkResourceNotOwned calls the function of the same name on the Revision's status.
func MarkResourceNotOwned(kind, name string) RevisionOption {
	return func(rev *v1alpha1.Revision) {
		rev.Status.MarkResourceNotOwned(kind, name)
	}
}

// WithRevConcurrencyModel sets the concurrency model on the Revision.
func WithRevConcurrencyModel(ss v1alpha1.RevisionRequestConcurrencyModelType) RevisionOption {
	return func(rev *v1alpha1.Revision) {
		rev.Spec.DeprecatedConcurrencyModel = ss
	}
}

// WithLogURL sets the .Status.LogURL to the expected value.
func WithLogURL(r *v1alpha1.Revision) {
	r.Status.LogURL = "http://logger.io/test-uid"
}

// WithCreationTimestamp sets the Revision's timestamp to the provided time.
// TODO(mattmoor): Ideally this could be a more generic Option and use meta.Accessor,
// but unfortunately Go's type system cannot support that.
func WithCreationTimestamp(t time.Time) RevisionOption {
	return func(rev *v1alpha1.Revision) {
		rev.ObjectMeta.CreationTimestamp = metav1.Time{Time: t}
	}
}

// WithNoBuild updates the status conditions to propagate a Build status as-if
// no BuildRef was specified.
func WithNoBuild(r *v1alpha1.Revision) {
	r.Status.PropagateBuildStatus(duckv1alpha1.Status{
		Conditions: duckv1alpha1.Conditions{{
			Type:   duckv1alpha1.ConditionSucceeded,
			Status: corev1.ConditionTrue,
			Reason: "NoBuild",
		}},
	})
}

// WithOngoingBuild propagates the status of an in-progress Build to the Revision's status.
func WithOngoingBuild(r *v1alpha1.Revision) {
	r.Status.PropagateBuildStatus(duckv1alpha1.Status{
		Conditions: duckv1alpha1.Conditions{{
			Type:   duckv1alpha1.ConditionSucceeded,
			Status: corev1.ConditionUnknown,
		}},
	})
}

// WithSuccessfulBuild propagates the status of a successful Build to the Revision's status.
func WithSuccessfulBuild(r *v1alpha1.Revision) {
	r.Status.PropagateBuildStatus(duckv1alpha1.Status{
		Conditions: duckv1alpha1.Conditions{{
			Type:   duckv1alpha1.ConditionSucceeded,
			Status: corev1.ConditionTrue,
		}},
	})
}

// WithFailedBuild propagates the status of a failed Build to the Revision's status.
func WithFailedBuild(reason, message string) RevisionOption {
	return func(r *v1alpha1.Revision) {
		r.Status.PropagateBuildStatus(duckv1alpha1.Status{
			Conditions: duckv1alpha1.Conditions{{
				Type:    duckv1alpha1.ConditionSucceeded,
				Status:  corev1.ConditionFalse,
				Reason:  reason,
				Message: message,
			}},
		})
	}
}

// WithLastPinned updates the "last pinned" annotation to the provided timestamp.
func WithLastPinned(t time.Time) RevisionOption {
	return func(rev *v1alpha1.Revision) {
		rev.SetLastPinned(t)
	}
}

// WithRevStatus is a generic escape hatch for creating hard-to-craft
// status orientations.
func WithRevStatus(st v1alpha1.RevisionStatus) RevisionOption {
	return func(rev *v1alpha1.Revision) {
		rev.Status = st
	}
}

// MarkActive calls .Status.MarkActive on the Revision.
func MarkActive(r *v1alpha1.Revision) {
	r.Status.MarkActive()
}

// MarkInactive calls .Status.MarkInactive on the Revision.
func MarkInactive(reason, message string) RevisionOption {
	return func(r *v1alpha1.Revision) {
		r.Status.MarkInactive(reason, message)
	}
}

// MarkActivating calls .Status.MarkActivating on the Revision.
func MarkActivating(reason, message string) RevisionOption {
	return func(r *v1alpha1.Revision) {
		r.Status.MarkActivating(reason, message)
	}
}

// MarkDeploying calls .Status.MarkDeploying on the Revision.
func MarkDeploying(reason string) RevisionOption {
	return func(r *v1alpha1.Revision) {
		r.Status.MarkDeploying(reason)
	}
}

// MarkProgressDeadlineExceeded calls the method of the same name on the Revision
// with the message we expect the Revision Reconciler to pass.
func MarkProgressDeadlineExceeded(r *v1alpha1.Revision) {
	r.Status.MarkProgressDeadlineExceeded("Unable to create pods for more than 120 seconds.")
}

// MarkServiceTimeout calls .Status.MarkServiceTimeout on the Revision.
func MarkServiceTimeout(r *v1alpha1.Revision) {
	r.Status.MarkServiceTimeout()
}

// MarkContainerMissing calls .Status.MarkContainerMissing on the Revision.
func MarkContainerMissing(rev *v1alpha1.Revision) {
	rev.Status.MarkContainerMissing("It's the end of the world as we know it")
}

// MarkContainerExiting calls .Status.MarkContainerExiting on the Revision.
func MarkContainerExiting(exitCode int32, message string) RevisionOption {
	return func(r *v1alpha1.Revision) {
		r.Status.MarkContainerExiting(exitCode, message)
	}
}

// MarkRevisionReady calls the necessary helpers to make the Revision Ready=True.
func MarkRevisionReady(r *v1alpha1.Revision) {
	WithInitRevConditions(r)
	WithNoBuild(r)
	MarkActive(r)
	r.Status.MarkResourcesAvailable()
	r.Status.MarkContainerHealthy()
}

// PodAutoscalerOption is an option that can be applied to a PA.
type PodAutoscalerOption func(*autoscalingv1alpha1.PodAutoscaler)

// WithProtocolType sets the protocol type on the PodAutoscaler.
func WithProtocolType(pt networking.ProtocolType) PodAutoscalerOption {
	return func(pa *autoscalingv1alpha1.PodAutoscaler) {
		pa.Spec.ProtocolType = pt
	}
}

// WithPAOwnersRemoved clears the owner references of this PA resource.
func WithPAOwnersRemoved(pa *autoscalingv1alpha1.PodAutoscaler) {
	pa.OwnerReferences = nil
}

// MarkResourceNotOwnedByPA marks PA when it's now owning a resources it is supposed to own.
func MarkResourceNotOwnedByPA(rType, name string) PodAutoscalerOption {
	return func(pa *autoscalingv1alpha1.PodAutoscaler) {
		pa.Status.MarkResourceNotOwned(rType, name)
	}
}

// WithPodAutoscalerOwnersRemoved clears the owner references of this PodAutoscaler.
func WithPodAutoscalerOwnersRemoved(r *autoscalingv1alpha1.PodAutoscaler) {
	r.OwnerReferences = nil
}

// WithTraffic updates the PA to reflect it receiving traffic.
func WithTraffic(pa *autoscalingv1alpha1.PodAutoscaler) {
	pa.Status.MarkActive()
}

// WithPAStatusService annotats PA Status with the provided service name.
func WithPAStatusService(svc string) PodAutoscalerOption {
	return func(pa *autoscalingv1alpha1.PodAutoscaler) {
		pa.Status.ServiceName = svc
	}
}

// WithBufferedTraffic updates the PA to reflect that it has received
// and buffered traffic while it is being activated.
func WithBufferedTraffic(reason, message string) PodAutoscalerOption {
	return func(pa *autoscalingv1alpha1.PodAutoscaler) {
		pa.Status.MarkActivating(reason, message)
	}
}

// WithNoTraffic updates the PA to reflect the fact that it is not
// receiving traffic.
func WithNoTraffic(reason, message string) PodAutoscalerOption {
	return func(pa *autoscalingv1alpha1.PodAutoscaler) {
		pa.Status.MarkInactive(reason, message)
	}
}

// WithPADeletionTimestamp will set the DeletionTimestamp on the PodAutoscaler.
func WithPADeletionTimestamp(r *autoscalingv1alpha1.PodAutoscaler) {
	t := metav1.NewTime(time.Unix(1e9, 0))
	r.ObjectMeta.SetDeletionTimestamp(&t)
}

// WithHPAClass updates the PA to add the hpa class annotation.
func WithHPAClass(pa *autoscalingv1alpha1.PodAutoscaler) {
	if pa.Annotations == nil {
		pa.Annotations = make(map[string]string)
	}
	pa.Annotations[autoscaling.ClassAnnotationKey] = autoscaling.HPA
}

// WithKPAClass updates the PA to add the kpa class annotation.
func WithKPAClass(pa *autoscalingv1alpha1.PodAutoscaler) {
	if pa.Annotations == nil {
		pa.Annotations = make(map[string]string)
	}
	pa.Annotations[autoscaling.ClassAnnotationKey] = autoscaling.KPA
}

// WithContainerConcurrency returns a PodAutoscalerOption which sets
// the PodAutoscaler containerConcurrency to the provided value.
func WithContainerConcurrency(cc v1beta1.RevisionContainerConcurrencyType) PodAutoscalerOption {
	return func(pa *autoscalingv1alpha1.PodAutoscaler) {
		pa.Spec.ContainerConcurrency = cc
	}
}

// WithTargetAnnotation returns a PodAutoscalerOption which sets
// the PodAutoscaler autoscaling.knative.dev/target to the provided
// value.
func WithTargetAnnotation(target string) PodAutoscalerOption {
	return func(pa *autoscalingv1alpha1.PodAutoscaler) {
		if pa.Annotations == nil {
			pa.Annotations = make(map[string]string)
		}
		pa.Annotations[autoscaling.TargetAnnotationKey] = target
	}
}

// WithMetricAnnotation adds a metric annotation to the PA.
func WithMetricAnnotation(metric string) PodAutoscalerOption {
	return func(pa *autoscalingv1alpha1.PodAutoscaler) {
		if pa.Annotations == nil {
			pa.Annotations = make(map[string]string)
		}
		pa.Annotations[autoscaling.MetricAnnotationKey] = metric
	}
}

// K8sServiceOption enables further configuration of the Kubernetes Service.
type K8sServiceOption func(*corev1.Service)

// MutateK8sService changes the service in a way that must be reconciled.
func MutateK8sService(svc *corev1.Service) {
	// An effective hammer ;-P
	svc.Spec = corev1.ServiceSpec{}
}

// WithClusterIP assigns a ClusterIP to the K8s Service.
func WithClusterIP(ip string) K8sServiceOption {
	return func(svc *corev1.Service) {
		svc.Spec.ClusterIP = ip
	}
}

// WithExternalName gives external name to the K8s Service.
func WithExternalName(name string) K8sServiceOption {
	return func(svc *corev1.Service) {
		svc.Spec.ExternalName = name
	}
}

// WithK8sSvcOwnersRemoved clears the owner references of this Service.
func WithK8sSvcOwnersRemoved(svc *corev1.Service) {
	svc.OwnerReferences = nil
}

// EndpointsOption enables further configuration of the Kubernetes Endpoints.
type EndpointsOption func(*corev1.Endpoints)

// WithSubsets adds subsets to the body of a Revision, enabling us to refer readiness.
func WithSubsets(ep *corev1.Endpoints) {
	ep.Subsets = []corev1.EndpointSubset{{
		Addresses: []corev1.EndpointAddress{{IP: "127.0.0.1"}},
	}}
}

// WithEndpointsOwnersRemoved clears the owner references of this Endpoints resource.
func WithEndpointsOwnersRemoved(eps *corev1.Endpoints) {
	eps.OwnerReferences = nil
}

// PodOption enables further configuration of a Pod.
type PodOption func(*corev1.Pod)

// WithFailingContainer sets the .Status.ContainerStatuses on the pod to
// include a container named accordingly to fail with the given state.
func WithFailingContainer(name string, exitCode int, message string) PodOption {
	return func(pod *corev1.Pod) {
		pod.Status.ContainerStatuses = []corev1.ContainerStatus{{
			Name: name,
			LastTerminationState: corev1.ContainerState{
				Terminated: &corev1.ContainerStateTerminated{
					ExitCode: int32(exitCode),
					Message:  message,
				},
			},
		}}
	}
}

// SKSOption is a callback type for decorate SKS objects.
type SKSOption func(sks *netv1alpha1.ServerlessService)

// WithPubService annotates SKS status with the given service name.
func WithPubService(sks *netv1alpha1.ServerlessService) {
	sks.Status.ServiceName = names.PublicService(sks.Name)
}

// WithDeployRef annotates SKS with a deployment objectRef
func WithDeployRef(name string) SKSOption {
	return func(sks *netv1alpha1.ServerlessService) {
		sks.Spec.ObjectRef = autoscalingv1.CrossVersionObjectReference{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
			Name:       name,
		}
	}
}

// WithSKSReady marks SKS as ready.
func WithSKSReady(sks *netv1alpha1.ServerlessService) {
	WithPrivateService(sks)
	WithPubService(sks)
	sks.Status.MarkEndpointsReady()
}

// WithPrivateService annotates SKS status with the private service name.
func WithPrivateService(sks *netv1alpha1.ServerlessService) {
	sks.Status.PrivateServiceName = names.PrivateService(sks.Name)
}

// WithSKSOwnersRemoved clears the owner references of this SKS resource.
func WithSKSOwnersRemoved(sks *netv1alpha1.ServerlessService) {
	sks.OwnerReferences = nil
}

// WithProxyMode puts SKS into proxy mode.
func WithProxyMode(sks *netv1alpha1.ServerlessService) {
	sks.Spec.Mode = netv1alpha1.SKSOperationModeProxy
}

// SKS creates a generic ServerlessService object.
func SKS(ns, name string, so ...SKSOption) *netv1alpha1.ServerlessService {
	s := &netv1alpha1.ServerlessService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			UID:       "test-uid",
		},
		Spec: netv1alpha1.ServerlessServiceSpec{
			Mode:         netv1alpha1.SKSOperationModeServe,
			ProtocolType: networking.ProtocolHTTP1,
			ObjectRef: autoscalingv1.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       "foo-deployment",
			},
		},
	}
	for _, opt := range so {
		opt(s)
	}
	return s
}

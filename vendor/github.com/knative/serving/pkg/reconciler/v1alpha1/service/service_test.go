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

package service

import (
	"fmt"
	"testing"

	duckv1beta1 "github.com/knative/pkg/apis/duck/v1beta1"
	fakesharedclientset "github.com/knative/pkg/client/clientset/versioned/fake"
	"github.com/knative/pkg/controller"
	logtesting "github.com/knative/pkg/logging/testing"
	"github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"github.com/knative/serving/pkg/apis/serving/v1beta1"
	fakeclientset "github.com/knative/serving/pkg/client/clientset/versioned/fake"
	informers "github.com/knative/serving/pkg/client/informers/externalversions"
	"github.com/knative/serving/pkg/reconciler"
	"github.com/knative/serving/pkg/reconciler/v1alpha1/service/resources"
	. "github.com/knative/serving/pkg/reconciler/v1alpha1/testing"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fakekubeclientset "k8s.io/client-go/kubernetes/fake"
	clientgotesting "k8s.io/client-go/testing"

	. "github.com/knative/pkg/reconciler/testing"
)

// This is heavily based on the way the OpenShift Ingress controller tests its reconciliation method.
func TestReconcile(t *testing.T) {
	table := TableTest{{
		Name: "bad workqueue key",
		Key:  "too/many/parts",
	}, {
		Name: "key not found",
		Key:  "foo/not-found",
	}, {
		Name: "nop deletion reconcile",
		// Test that with a DeletionTimestamp we do nothing.
		Objects: []runtime.Object{
			Service("delete-pending", "foo", WithServiceDeletionTimestamp),
		},
		Key: "foo/delete-pending",
	}, {
		Name: "incomplete service",
		Objects: []runtime.Object{
			// There is no spec.{runLatest,pinned} in this Service to
			// trigger the error condition.
			Service("incomplete", "foo", WithInitSvcConditions),
		},
		Key:     "foo/incomplete",
		WantErr: true,
		WantEvents: []string{
			Eventf(corev1.EventTypeWarning, "CreationFailed", "Failed to create Configuration %q: %v",
				"incomplete", "malformed Service: MakeConfiguration requires one of runLatest, pinned, or release must be present"),
			Eventf(corev1.EventTypeWarning, "InternalError", "malformed Service: MakeConfiguration requires one of runLatest, pinned, or release must be present"),
		},
	}, {
		Name: "runLatest - create route and service",
		Objects: []runtime.Object{
			Service("run-latest", "foo", WithRunLatestRollout),
		},
		Key: "foo/run-latest",
		WantCreates: []metav1.Object{
			config("run-latest", "foo", WithRunLatestRollout),
			route("run-latest", "foo", WithRunLatestRollout),
		},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: Service("run-latest", "foo", WithRunLatestRollout,
				// The first reconciliation will initialize the status conditions.
				WithInitSvcConditions),
		}},
		WantEvents: []string{
			Eventf(corev1.EventTypeNormal, "Created", "Created Configuration %q", "run-latest"),
			Eventf(corev1.EventTypeNormal, "Created", "Created Route %q", "run-latest"),
			Eventf(corev1.EventTypeNormal, "Updated", "Updated Service %q", "run-latest"),
		},
	}, {
		Name: "pinned - create route and service",
		Objects: []runtime.Object{
			Service("pinned", "foo", WithPinnedRollout("pinned-0001")),
		},
		Key: "foo/pinned",
		WantCreates: []metav1.Object{
			config("pinned", "foo", WithPinnedRollout("pinned-0001")),
			route("pinned", "foo", WithPinnedRollout("pinned-0001")),
		},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: Service("pinned", "foo", WithPinnedRollout("pinned-0001"),
				// The first reconciliation will initialize the status conditions.
				WithInitSvcConditions),
		}},
		WantEvents: []string{
			Eventf(corev1.EventTypeNormal, "Created", "Created Configuration %q", "pinned"),
			Eventf(corev1.EventTypeNormal, "Created", "Created Route %q", "pinned"),
			Eventf(corev1.EventTypeNormal, "Updated", "Updated Service %q", "pinned"),
		},
	}, {
		// Pinned rollouts are deprecated, so test the same functionality
		// using Release.
		Name: "pinned - create route and service - via release",
		Objects: []runtime.Object{
			Service("pinned2", "foo", WithReleaseRollout("pinned2-0001")),
		},
		Key: "foo/pinned2",
		WantCreates: []metav1.Object{
			config("pinned2", "foo", WithReleaseRollout("pinned2-0001")),
			route("pinned2", "foo", WithReleaseRollout("pinned2-0001")),
		},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: Service("pinned2", "foo", WithReleaseRollout("pinned2-0001"),
				// The first reconciliation will initialize the status conditions.
				WithInitSvcConditions),
		}},
		WantEvents: []string{
			Eventf(corev1.EventTypeNormal, "Created", "Created Configuration %q", "pinned2"),
			Eventf(corev1.EventTypeNormal, "Created", "Created Route %q", "pinned2"),
			Eventf(corev1.EventTypeNormal, "Updated", "Updated Service %q", "pinned2"),
		},
	}, {
		Name: "pinned - with ready config and route",
		Objects: []runtime.Object{
			Service("pinned3", "foo", WithReleaseRollout("pinned3-00001"),
				WithInitSvcConditions),
			config("pinned3", "foo", WithReleaseRollout("pinned3-00001"), WithGeneration(1),
				WithObservedGen,
				WithLatestCreated("pinned3-00001"),
				WithLatestReady("pinned3-00001")),
			route("pinned3", "foo", WithReleaseRollout("pinned3-00001"),
				WithDomain, WithDomainInternal, WithAddress, WithInitRouteConditions,
				WithStatusTraffic(v1alpha1.TrafficTarget{
					Name: v1alpha1.CurrentTrafficTarget,
					TrafficTarget: v1beta1.TrafficTarget{
						RevisionName: "pinned3-00001",
						Percent:      100,
					},
				}, v1alpha1.TrafficTarget{
					Name: v1alpha1.LatestTrafficTarget,
					TrafficTarget: v1beta1.TrafficTarget{
						RevisionName: "pinned3-00001",
						Percent:      0,
					},
				}), MarkTrafficAssigned, MarkIngressReady),
		},
		Key: "foo/pinned3",
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			// Make sure that status contains all the required propagated fields
			// from config and route status.
			Object: Service("pinned3", "foo",
				// Initial setup conditions.
				WithReleaseRollout("pinned3-00001"),
				// The delta induced by configuration object.
				WithReadyConfig("pinned3-00001"),
				// The delta induced by route object.
				WithReadyRoute, WithSvcStatusDomain, WithSvcStatusAddress,
				WithSvcStatusTraffic(v1alpha1.TrafficTarget{
					Name: v1alpha1.CurrentTrafficTarget,
					TrafficTarget: v1beta1.TrafficTarget{
						RevisionName: "pinned3-00001",
						Percent:      100,
					},
				}, v1alpha1.TrafficTarget{
					Name: v1alpha1.LatestTrafficTarget,
					TrafficTarget: v1beta1.TrafficTarget{
						RevisionName: "pinned3-00001",
						Percent:      0,
					},
				})),
		}},
		WantEvents: []string{
			Eventf(corev1.EventTypeNormal, "Updated", "Updated Service %q", "pinned3"),
		},
		WantServiceReadyStats: map[string]int{
			"foo/pinned3": 1,
		},
	}, {
		Name: "release - with @latest",
		Objects: []runtime.Object{
			Service("release", "foo", WithReleaseRollout(v1alpha1.ReleaseLatestRevisionKeyword)),
		},
		Key: "foo/release",
		WantCreates: []metav1.Object{
			config("release", "foo", WithReleaseRollout("release-00001")),
			route("release", "foo", WithReleaseRollout(v1alpha1.ReleaseLatestRevisionKeyword)),
		},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: Service("release", "foo", WithReleaseRollout(v1alpha1.ReleaseLatestRevisionKeyword),
				// The first reconciliation will initialize the status conditions.
				WithInitSvcConditions),
		}},
		WantEvents: []string{
			Eventf(corev1.EventTypeNormal, "Created", "Created Configuration %q", "release"),
			Eventf(corev1.EventTypeNormal, "Created", "Created Route %q", "release"),
			Eventf(corev1.EventTypeNormal, "Updated", "Updated Service %q", "release"),
		},
	}, {
		Name: "release - create route and service",
		Objects: []runtime.Object{
			Service("release", "foo", WithReleaseRollout("release-00001", "release-00002")),
		},
		Key: "foo/release",
		WantCreates: []metav1.Object{
			config("release", "foo", WithReleaseRollout("release-00001", "release-00002")),
			route("release", "foo", WithReleaseRollout("release-00001", "release-00002")),
		},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: Service("release", "foo", WithReleaseRollout("release-00001", "release-00002"),
				// The first reconciliation will initialize the status conditions.
				WithInitSvcConditions),
		}},
		WantEvents: []string{
			Eventf(corev1.EventTypeNormal, "Created", "Created Configuration %q", "release"),
			Eventf(corev1.EventTypeNormal, "Created", "Created Route %q", "release"),
			Eventf(corev1.EventTypeNormal, "Updated", "Updated Service %q", "release"),
		},
	}, {
		Name: "release - update service, route not ready",
		Objects: []runtime.Object{
			Service("release-nr", "foo", WithReleaseRollout("release-nr-00002"), WithInitSvcConditions),
			config("release-nr", "foo", WithReleaseRollout("release-nr-00002"),
				WithCreatedAndReady("release-nr-00002", "release-nr-00002")),
			// NB: route points to the previous revision.
			route("release-nr", "foo", WithReleaseRollout("release-nr-00002"), RouteReady,
				WithDomain, WithDomainInternal, WithAddress, WithInitRouteConditions,
				WithStatusTraffic(v1alpha1.TrafficTarget{
					TrafficTarget: v1beta1.TrafficTarget{
						RevisionName: "release-nr-00001",
						Percent:      100,
					},
				}), MarkTrafficAssigned, MarkIngressReady),
		},
		Key: "foo/release-nr",
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: Service("release-nr", "foo",
				WithReleaseRollout("release-nr-00002"),
				WithReadyConfig("release-nr-00002"),
				WithServiceStatusRouteNotReady, WithSvcStatusDomain, WithSvcStatusAddress,
				WithSvcStatusTraffic(v1alpha1.TrafficTarget{
					TrafficTarget: v1beta1.TrafficTarget{
						RevisionName: "release-nr-00001",
						Percent:      100,
					},
				})),
		}},
		WantEvents: []string{
			Eventf(corev1.EventTypeNormal, "Updated", "Updated Service %q", "release-nr"),
		},
	}, {
		Name: "release - update service, route not ready, 2 rev, no split",
		Objects: []runtime.Object{
			Service("release-nr", "foo", WithReleaseRollout("release-nr-00002", "release-nr-00003"), WithInitSvcConditions),
			config("release-nr", "foo", WithReleaseRollout("release-nr-00002", "release-nr-00003"),
				WithCreatedAndReady("release-nr-00003", "release-nr-00003")),
			// NB: route points to the previous revision.
			route("release-nr", "foo", WithReleaseRollout("release-nr-00002", "release-nr-00003"),
				RouteReady, WithDomain, WithDomainInternal, WithAddress, WithInitRouteConditions,
				WithStatusTraffic(v1alpha1.TrafficTarget{
					TrafficTarget: v1beta1.TrafficTarget{
						RevisionName: "release-nr-00001",
						Percent:      100,
					},
				}), MarkTrafficAssigned, MarkIngressReady),
		},
		Key: "foo/release-nr",
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: Service("release-nr", "foo",
				WithReleaseRollout("release-nr-00002", "release-nr-00003"),
				WithReadyConfig("release-nr-00003"),
				WithServiceStatusRouteNotReady, WithSvcStatusDomain, WithSvcStatusAddress,
				WithSvcStatusTraffic(v1alpha1.TrafficTarget{
					TrafficTarget: v1beta1.TrafficTarget{
						RevisionName: "release-nr-00001",
						Percent:      100,
					},
				})),
		}},
		WantEvents: []string{
			Eventf(corev1.EventTypeNormal, "Updated", "Updated Service %q", "release-nr"),
		},
	}, {
		Name: "release - update service, route not ready, traffic split",
		Objects: []runtime.Object{
			Service("release-nr-ts", "foo",
				WithReleaseRolloutAndPercentage(42, "release-nr-ts-00002", "release-nr-ts-00003"),
				WithInitSvcConditions),
			config("release-nr-ts", "foo",
				WithReleaseRolloutAndPercentage(42, "release-nr-ts-00002", "release-nr-ts-00003"),
				WithCreatedAndReady("release-nr-ts-00003", "release-nr-ts-00003")),
			route("release-nr-ts", "foo",
				WithReleaseRolloutAndPercentage(42, "release-nr-ts-00002", "release-nr-ts-00003"),
				RouteReady, WithDomain, WithDomainInternal, WithAddress, WithInitRouteConditions,
				WithStatusTraffic(v1alpha1.TrafficTarget{
					TrafficTarget: v1beta1.TrafficTarget{
						RevisionName: "release-nr-ts-00001",
						Percent:      58,
					},
				}, v1alpha1.TrafficTarget{
					TrafficTarget: v1beta1.TrafficTarget{
						RevisionName: "release-nr-ts-00002",
						Percent:      42,
					},
				}), MarkTrafficAssigned, MarkIngressReady),
		},
		Key: "foo/release-nr-ts",
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: Service("release-nr-ts", "foo",
				WithReleaseRolloutAndPercentage(42, "release-nr-ts-00002", "release-nr-ts-00003"),
				WithReadyConfig("release-nr-ts-00003"),
				WithServiceStatusRouteNotReady, WithSvcStatusDomain, WithSvcStatusAddress,
				WithSvcStatusTraffic(v1alpha1.TrafficTarget{
					TrafficTarget: v1beta1.TrafficTarget{
						RevisionName: "release-nr-ts-00001",
						Percent:      58,
					},
				}, v1alpha1.TrafficTarget{
					TrafficTarget: v1beta1.TrafficTarget{
						RevisionName: "release-nr-ts-00002",
						Percent:      42,
					},
				})),
		}},
		WantEvents: []string{
			Eventf(corev1.EventTypeNormal, "Updated", "Updated Service %q", "release-nr-ts"),
		},
	}, {
		Name: "release - update service, route not ready, traffic split, percentage changed",
		Objects: []runtime.Object{
			Service("release-nr-ts2", "foo",
				WithReleaseRolloutAndPercentage(58, "release-nr-ts2-00002", "release-nr-ts2-00003"),
				WithInitSvcConditions),
			config("release-nr-ts2", "foo",
				WithReleaseRolloutAndPercentage(58, "release-nr-ts2-00002", "release-nr-ts2-00003"),
				WithCreatedAndReady("release-nr-ts2-00003", "release-nr-ts2-00003")),
			route("release-nr-ts2", "foo",
				WithReleaseRolloutAndPercentage(58, "release-nr-ts2-00002", "release-nr-ts2-00003"),
				RouteReady, WithDomain, WithDomainInternal, WithAddress, WithInitRouteConditions,
				// NB: here the revisions match, but percentages, don't.
				WithStatusTraffic(v1alpha1.TrafficTarget{
					TrafficTarget: v1beta1.TrafficTarget{
						RevisionName: "release-nr-ts2-00002",
						Percent:      58,
					},
				}, v1alpha1.TrafficTarget{
					TrafficTarget: v1beta1.TrafficTarget{
						RevisionName: "release-nr-ts2-00003",
						Percent:      42,
					},
				}), MarkTrafficAssigned, MarkIngressReady),
		},
		Key: "foo/release-nr-ts2",
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: Service("release-nr-ts2", "foo",
				WithReleaseRolloutAndPercentage(58, "release-nr-ts2-00002", "release-nr-ts2-00003"),
				WithReadyConfig("release-nr-ts2-00003"),
				WithServiceStatusRouteNotReady, WithSvcStatusDomain, WithSvcStatusAddress,
				WithSvcStatusTraffic(v1alpha1.TrafficTarget{
					TrafficTarget: v1beta1.TrafficTarget{
						RevisionName: "release-nr-ts2-00002",
						Percent:      58,
					},
				}, v1alpha1.TrafficTarget{
					TrafficTarget: v1beta1.TrafficTarget{
						RevisionName: "release-nr-ts2-00003",
						Percent:      42,
					},
				})),
		}},
		WantEvents: []string{
			Eventf(corev1.EventTypeNormal, "Updated", "Updated Service %q", "release-nr-ts2"),
		},
	}, {
		Name: "release - route and config ready, using @latest",
		Objects: []runtime.Object{
			Service("release-ready-lr", "foo",
				WithReleaseRollout(v1alpha1.ReleaseLatestRevisionKeyword), WithInitSvcConditions),
			route("release-ready-lr", "foo",
				WithReleaseRollout(v1alpha1.ReleaseLatestRevisionKeyword),
				RouteReady, WithDomain, WithDomainInternal, WithAddress, WithInitRouteConditions,
				WithStatusTraffic([]v1alpha1.TrafficTarget{{
					Name: v1alpha1.CurrentTrafficTarget,
					TrafficTarget: v1beta1.TrafficTarget{
						RevisionName: "release-ready-lr-00001",
						Percent:      100,
					},
				}, {
					Name: v1alpha1.LatestTrafficTarget,
					TrafficTarget: v1beta1.TrafficTarget{
						RevisionName: "release-ready-lr-00001",
					},
				}}...), MarkTrafficAssigned, MarkIngressReady),
			config("release-ready-lr", "foo", WithReleaseRollout("release-ready-lr"), WithGeneration(1),
				// These turn a Configuration to Ready=true
				WithLatestCreated("release-ready-lr-00001"),
				WithLatestReady("release-ready-lr-00001")),
		},
		Key: "foo/release-ready-lr",
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: Service("release-ready-lr", "foo",
				WithReleaseRollout(v1alpha1.ReleaseLatestRevisionKeyword),
				// The delta induced by the config object.
				WithReadyConfig("release-ready-lr-00001"),
				// The delta induced by route object.
				WithReadyRoute, WithSvcStatusDomain, WithSvcStatusAddress,
				WithSvcStatusTraffic([]v1alpha1.TrafficTarget{{
					Name: v1alpha1.CurrentTrafficTarget,
					TrafficTarget: v1beta1.TrafficTarget{
						RevisionName: "release-ready-lr-00001",
						Percent:      100,
					},
				}, {
					Name: v1alpha1.LatestTrafficTarget,
					TrafficTarget: v1beta1.TrafficTarget{
						RevisionName: "release-ready-lr-00001",
					},
				}}...),
			),
		}},
		WantEvents: []string{
			Eventf(corev1.EventTypeNormal, "Updated", "Updated Service %q", "release-ready-lr"),
		},
		WantServiceReadyStats: map[string]int{
			"foo/release-ready-lr": 1,
		},
	}, {
		Name: "release - route and config ready, traffic split, using @latest",
		Objects: []runtime.Object{
			Service("release-ready-lr", "foo",
				WithReleaseRolloutAndPercentage(
					42, "release-ready-lr-00001", v1alpha1.ReleaseLatestRevisionKeyword), WithInitSvcConditions),
			route("release-ready-lr", "foo",
				WithReleaseRolloutAndPercentage(
					42, "release-ready-lr-00001", v1alpha1.ReleaseLatestRevisionKeyword),
				RouteReady, WithDomain, WithDomainInternal, WithAddress, WithInitRouteConditions,
				WithStatusTraffic([]v1alpha1.TrafficTarget{{
					Name: v1alpha1.CurrentTrafficTarget,
					TrafficTarget: v1beta1.TrafficTarget{
						RevisionName: "release-ready-lr-00001",
						Percent:      58,
					},
				}, {
					Name: v1alpha1.CandidateTrafficTarget,
					TrafficTarget: v1beta1.TrafficTarget{
						RevisionName: "release-ready-lr-00002",
						Percent:      42,
					},
				}, {
					Name: v1alpha1.LatestTrafficTarget,
					TrafficTarget: v1beta1.TrafficTarget{
						RevisionName: "release-ready-lr-00002",
					},
				}}...), MarkTrafficAssigned, MarkIngressReady),
			config("release-ready-lr", "foo", WithReleaseRollout("release-ready-lr"), WithGeneration(2),
				// These turn a Configuration to Ready=true
				WithLatestCreated("release-ready-lr-00002"),
				WithLatestReady("release-ready-lr-00002")),
		},
		Key: "foo/release-ready-lr",
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: Service("release-ready-lr", "foo",
				WithReleaseRolloutAndPercentage(
					42, "release-ready-lr-00001", v1alpha1.ReleaseLatestRevisionKeyword),
				// The delta induced by the config object.
				WithReadyConfig("release-ready-lr-00002"),
				// The delta induced by route object.
				WithReadyRoute, WithSvcStatusDomain, WithSvcStatusAddress,
				WithSvcStatusTraffic([]v1alpha1.TrafficTarget{{
					Name: v1alpha1.CurrentTrafficTarget,
					TrafficTarget: v1beta1.TrafficTarget{
						RevisionName: "release-ready-lr-00001",
						Percent:      58,
					},
				}, {
					Name: v1alpha1.CandidateTrafficTarget,
					TrafficTarget: v1beta1.TrafficTarget{
						RevisionName: "release-ready-lr-00002",
						Percent:      42,
					},
				}, {
					Name: v1alpha1.LatestTrafficTarget,
					TrafficTarget: v1beta1.TrafficTarget{
						RevisionName: "release-ready-lr-00002",
					},
				}}...),
			),
		}},
		WantEvents: []string{
			Eventf(corev1.EventTypeNormal, "Updated", "Updated Service %q", "release-ready-lr"),
		},
		WantServiceReadyStats: map[string]int{
			"foo/release-ready-lr": 1,
		},
	}, {
		Name: "release - route and config ready, propagate ready, percentage set",
		Objects: []runtime.Object{
			Service("release-ready", "foo",
				WithReleaseRolloutAndPercentage(58, /*candidate traffic percentage*/
					"release-ready-00001", "release-ready-00002"), WithInitSvcConditions),
			route("release-ready", "foo",
				WithReleaseRolloutAndPercentage(58, /*candidate traffic percentage*/
					"release-ready-00001", "release-ready-00002"),
				RouteReady, WithDomain, WithDomainInternal, WithAddress, WithInitRouteConditions,
				WithStatusTraffic(v1alpha1.TrafficTarget{
					Name: v1alpha1.CurrentTrafficTarget,
					TrafficTarget: v1beta1.TrafficTarget{
						RevisionName: "release-ready-00001",
						Percent:      42,
					},
				}, v1alpha1.TrafficTarget{
					Name: v1alpha1.CandidateTrafficTarget,
					TrafficTarget: v1beta1.TrafficTarget{
						RevisionName: "release-ready-00002",
						Percent:      58,
					},
				}, v1alpha1.TrafficTarget{
					Name: v1alpha1.LatestTrafficTarget,
					TrafficTarget: v1beta1.TrafficTarget{
						RevisionName: "release-ready-00002",
						Percent:      0,
					},
				}), MarkTrafficAssigned, MarkIngressReady),
			config("release-ready", "foo", WithRunLatestRollout, WithGeneration(2),
				// These turn a Configuration to Ready=true
				WithLatestCreated("release-ready-00002"), WithLatestReady("release-ready-00002")),
		},
		Key: "foo/release-ready",
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: Service("release-ready", "foo",
				WithReleaseRolloutAndPercentage(58, /*candidate traffic percentage*/
					"release-ready-00001", "release-ready-00002"),
				// The delta induced by the config object.
				WithReadyConfig("release-ready-00002"),
				// The delta induced by route object.
				WithReadyRoute, WithSvcStatusDomain, WithSvcStatusAddress,
				WithSvcStatusTraffic(v1alpha1.TrafficTarget{
					Name: v1alpha1.CurrentTrafficTarget,
					TrafficTarget: v1beta1.TrafficTarget{
						RevisionName: "release-ready-00001",
						Percent:      42,
					},
				}, v1alpha1.TrafficTarget{
					Name: v1alpha1.CandidateTrafficTarget,
					TrafficTarget: v1beta1.TrafficTarget{
						RevisionName: "release-ready-00002",
						Percent:      58,
					},
				}, v1alpha1.TrafficTarget{
					Name: v1alpha1.LatestTrafficTarget,
					TrafficTarget: v1beta1.TrafficTarget{
						RevisionName: "release-ready-00002",
						Percent:      0,
					},
				}),
			),
		}},
		WantEvents: []string{
			Eventf(corev1.EventTypeNormal, "Updated", "Updated Service %q", "release-ready"),
		},
		WantServiceReadyStats: map[string]int{
			"foo/release-ready": 1,
		},
	}, {
		Name: "release - create route and service and percentage",
		Objects: []runtime.Object{
			Service("release-with-percent", "foo", WithReleaseRolloutAndPercentage(10, /*candidate traffic percentage*/
				"release-with-percent-00001", "release-with-percent-00002")),
		},
		Key: "foo/release-with-percent",
		WantCreates: []metav1.Object{
			config("release-with-percent", "foo", WithReleaseRolloutAndPercentage(10, "release-with-percent-00001", "release-with-percent-00002")),
			route("release-with-percent", "foo", WithReleaseRolloutAndPercentage(10, "release-with-percent-00001", "release-with-percent-00002")),
		},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: Service("release-with-percent", "foo", WithReleaseRolloutAndPercentage(10, "release-with-percent-00001", "release-with-percent-00002"),
				// The first reconciliation will initialize the status conditions.
				WithInitSvcConditions),
		}},
		WantEvents: []string{
			Eventf(corev1.EventTypeNormal, "Created", "Created Configuration %q", "release-with-percent"),
			Eventf(corev1.EventTypeNormal, "Created", "Created Route %q", "release-with-percent"),
			Eventf(corev1.EventTypeNormal, "Updated", "Updated Service %q", "release-with-percent"),
		},
	}, {
		Name: "manual- no creates",
		Objects: []runtime.Object{
			Service("manual", "foo", WithManualRollout),
		},
		Key: "foo/manual",
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: Service("manual", "foo", WithManualRollout,
				// The first reconciliation will initialize the status conditions.
				WithManualStatus),
		}},
		WantEvents: []string{
			Eventf(corev1.EventTypeNormal, "Updated", "Updated Service %q", "manual"),
		},
	}, {
		Name: "runLatest - no updates",
		Objects: []runtime.Object{
			Service("no-updates", "foo", WithRunLatestRollout, WithInitSvcConditions),
			route("no-updates", "foo", WithRunLatestRollout),
			config("no-updates", "foo", WithRunLatestRollout),
		},
		Key: "foo/no-updates",
	}, {
		Name: "runLatest - update route and service",
		Objects: []runtime.Object{
			Service("update-route-and-config", "foo", WithRunLatestRollout, WithInitSvcConditions),
			// Mutate the Config/Route to have a different body than we want.
			config("update-route-and-config", "foo", WithRunLatestRollout,
				// WithBuild is just an unexpected mutation of the config spec vs. the service spec.
				WithBuild),
			route("update-route-and-config", "foo", WithRunLatestRollout, MutateRoute),
		},
		Key: "foo/update-route-and-config",
		WantUpdates: []clientgotesting.UpdateActionImpl{{
			Object: config("update-route-and-config", "foo", WithRunLatestRollout),
		}, {
			Object: route("update-route-and-config", "foo", WithRunLatestRollout),
		}},
	}, {
		Name: "runLatest - update route and service (bad existing Revision)",
		Objects: []runtime.Object{
			Service("update-route-and-config", "foo", WithRunLatestRollout, func(svc *v1alpha1.Service) {
				svc.Spec.RunLatest.Configuration.GetTemplate().Name = "update-route-and-config-blah"
			}, WithInitSvcConditions),
			// Mutate the Config/Route to have a different body than we want.
			config("update-route-and-config", "foo", WithRunLatestRollout,
				// Change the concurrency model to ensure it is corrected.
				WithConfigConcurrencyModel("Single")),
			route("update-route-and-config", "foo", WithRunLatestRollout, MutateRoute),
			&v1alpha1.Revision{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "update-route-and-config-blah",
					Namespace: "foo",
					// Not labeled with the configuration or the right generation.
				},
			},
		},
		Key: "foo/update-route-and-config",
		WantUpdates: []clientgotesting.UpdateActionImpl{{
			Object: config("update-route-and-config", "foo", WithRunLatestRollout,
				func(cfg *v1alpha1.Configuration) {
					cfg.Spec.GetTemplate().Name = "update-route-and-config-blah"
				}),
		}},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: Service("update-route-and-config", "foo", WithRunLatestRollout, func(svc *v1alpha1.Service) {
				svc.Spec.RunLatest.Configuration.GetTemplate().Name = "update-route-and-config-blah"
			}, WithInitSvcConditions, func(svc *v1alpha1.Service) {
				svc.Status.MarkRevisionNameTaken("update-route-and-config-blah")
			}),
		}},
		WantEvents: []string{
			Eventf(corev1.EventTypeNormal, "Updated", "Updated Service %q", "update-route-and-config"),
		},
	}, {
		Name: "runLatest - update route and config labels",
		Objects: []runtime.Object{
			// Mutate the Service to add some more labels
			Service("update-route-and-config-labels", "foo", WithRunLatestRollout, WithInitSvcConditions, WithServiceLabel("new-label", "new-value")),
			config("update-route-and-config-labels", "foo", WithRunLatestRollout),
			route("update-route-and-config-labels", "foo", WithRunLatestRollout),
		},
		Key: "foo/update-route-and-config-labels",
		WantUpdates: []clientgotesting.UpdateActionImpl{{
			Object: config("update-route-and-config-labels", "foo", WithRunLatestRollout, WithConfigLabel("new-label", "new-value")),
		}, {
			Object: route("update-route-and-config-labels", "foo", WithRunLatestRollout, WithRouteLabel("new-label", "new-value")),
		}},
	}, {
		Name: "runLatest - update route config labels ignoring serving.knative.dev/route",
		Objects: []runtime.Object{
			// Mutate the Service to add some more labels
			Service("update-child-labels-ignore-route-label", "foo",
				WithRunLatestRollout, WithInitSvcConditions, WithServiceLabel("new-label", "new-value")),
			config("update-child-labels-ignore-route-label", "foo",
				WithRunLatestRollout, WithConfigLabel("serving.knative.dev/route", "update-child-labels-ignore-route-label")),
			route("update-child-labels-ignore-route-label", "foo", WithRunLatestRollout),
		},
		Key: "foo/update-child-labels-ignore-route-label",
		WantUpdates: []clientgotesting.UpdateActionImpl{{
			Object: config("update-child-labels-ignore-route-label", "foo", WithRunLatestRollout, WithConfigLabel("new-label", "new-value"),
				WithConfigLabel("serving.knative.dev/route", "update-child-labels-ignore-route-label")),
		}, {
			Object: route("update-child-labels-ignore-route-label", "foo", WithRunLatestRollout, WithRouteLabel("new-label", "new-value")),
		}},
	}, {
		Name: "runLatest - bad config update",
		Objects: []runtime.Object{
			// There is no spec.{runLatest,pinned} in this Service, which triggers the error
			// path updating Configuration.
			Service("bad-config-update", "foo", WithInitSvcConditions),
			config("bad-config-update", "foo", WithRunLatestRollout),
			route("bad-config-update", "foo", WithRunLatestRollout, MutateRoute),
		},
		Key:     "foo/bad-config-update",
		WantErr: true,
		WantEvents: []string{
			Eventf(corev1.EventTypeWarning, "InternalError", "malformed Service: MakeConfiguration requires one of runLatest, pinned, or release must be present"),
		},
	}, {
		Name: "runLatest - route creation failure",
		// Induce a failure during route creation
		WantErr: true,
		WithReactors: []clientgotesting.ReactionFunc{
			InduceFailure("create", "routes"),
		},
		Objects: []runtime.Object{
			Service("create-route-failure", "foo", WithRunLatestRollout),
		},
		Key: "foo/create-route-failure",
		WantCreates: []metav1.Object{
			config("create-route-failure", "foo", WithRunLatestRollout),
			route("create-route-failure", "foo", WithRunLatestRollout),
		},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: Service("create-route-failure", "foo", WithRunLatestRollout,
				// First reconcile initializes conditions.
				WithInitSvcConditions),
		}},
		WantEvents: []string{
			Eventf(corev1.EventTypeNormal, "Created", "Created Configuration %q", "create-route-failure"),
			Eventf(corev1.EventTypeWarning, "CreationFailed", "Failed to create Route %q: %v",
				"create-route-failure", "inducing failure for create routes"),
			Eventf(corev1.EventTypeWarning, "InternalError", "inducing failure for create routes"),
		},
	}, {
		Name: "runLatest - configuration creation failure",
		// Induce a failure during configuration creation
		WantErr: true,
		WithReactors: []clientgotesting.ReactionFunc{
			InduceFailure("create", "configurations"),
		},
		Objects: []runtime.Object{
			Service("create-config-failure", "foo", WithRunLatestRollout),
		},
		Key: "foo/create-config-failure",
		WantCreates: []metav1.Object{
			config("create-config-failure", "foo", WithRunLatestRollout),
			// We don't get to creating the Route.
		},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: Service("create-config-failure", "foo", WithRunLatestRollout,
				// First reconcile initializes conditions.
				WithInitSvcConditions),
		}},
		WantEvents: []string{
			Eventf(corev1.EventTypeWarning, "CreationFailed", "Failed to create Configuration %q: %v",
				"create-config-failure", "inducing failure for create configurations"),
			Eventf(corev1.EventTypeWarning, "InternalError", "inducing failure for create configurations"),
		},
	}, {
		Name: "runLatest - update route failure",
		// Induce a failure updating the route
		WantErr: true,
		WithReactors: []clientgotesting.ReactionFunc{
			InduceFailure("update", "routes"),
		},
		Objects: []runtime.Object{
			Service("update-route-failure", "foo", WithRunLatestRollout, WithInitSvcConditions),
			// Mutate the Route to have an unexpected body to trigger an update.
			route("update-route-failure", "foo", WithRunLatestRollout, MutateRoute),
			config("update-route-failure", "foo", WithRunLatestRollout),
		},
		Key: "foo/update-route-failure",
		WantUpdates: []clientgotesting.UpdateActionImpl{{
			Object: route("update-route-failure", "foo", WithRunLatestRollout),
		}},
		WantEvents: []string{
			Eventf(corev1.EventTypeWarning, "InternalError", "inducing failure for update routes"),
		},
	}, {
		Name: "runLatest - update config failure",
		// Induce a failure updating the config
		WantErr: true,
		WithReactors: []clientgotesting.ReactionFunc{
			InduceFailure("update", "configurations"),
		},
		Objects: []runtime.Object{
			Service("update-config-failure", "foo", WithRunLatestRollout, WithInitSvcConditions),
			route("update-config-failure", "foo", WithRunLatestRollout),
			// Mutate the Config to have an unexpected body to trigger an update.
			config("update-config-failure", "foo", WithRunLatestRollout,
				// WithBuild is just an unexpected mutation of the config spec vs. the service spec.
				WithBuild),
		},
		Key: "foo/update-config-failure",
		WantUpdates: []clientgotesting.UpdateActionImpl{{
			Object: config("update-config-failure", "foo", WithRunLatestRollout),
		}},
		WantEvents: []string{
			Eventf(corev1.EventTypeWarning, "InternalError", "inducing failure for update configurations"),
		},
	}, {
		Name: "runLatest - failure updating service status",
		// Induce a failure updating the service status.
		WantErr: true,
		WithReactors: []clientgotesting.ReactionFunc{
			InduceFailure("update", "services"),
		},
		Objects: []runtime.Object{
			Service("run-latest", "foo", WithRunLatestRollout),
		},
		Key: "foo/run-latest",
		WantCreates: []metav1.Object{
			config("run-latest", "foo", WithRunLatestRollout),
			route("run-latest", "foo", WithRunLatestRollout),
		},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: Service("run-latest", "foo", WithRunLatestRollout,
				// We attempt to update the Service to initialize its
				// conditions, which is where we induce the failure.
				WithInitSvcConditions),
		}},
		WantEvents: []string{
			Eventf(corev1.EventTypeNormal, "Created", "Created Configuration %q", "run-latest"),
			Eventf(corev1.EventTypeNormal, "Created", "Created Route %q", "run-latest"),
			Eventf(corev1.EventTypeWarning, "UpdateFailed", "Failed to update status for Service %q: %v",
				"run-latest", "inducing failure for update services"),
		},
	}, {
		Name: "runLatest - route and config ready, propagate ready",
		// When both route and config are ready, the service should become ready.
		Objects: []runtime.Object{
			Service("all-ready", "foo", WithRunLatestRollout, WithInitSvcConditions),
			route("all-ready", "foo", WithRunLatestRollout, RouteReady,
				WithDomain, WithDomainInternal, WithAddress, WithInitRouteConditions,
				WithStatusTraffic(v1alpha1.TrafficTarget{
					TrafficTarget: v1beta1.TrafficTarget{
						RevisionName: "all-ready-00001",
						Percent:      100,
					},
				}), MarkTrafficAssigned, MarkIngressReady),
			config("all-ready", "foo", WithRunLatestRollout, WithGeneration(1),
				// These turn a Configuration to Ready=true
				WithLatestCreated("all-ready-00001"), WithLatestReady("all-ready-00001")),
		},
		Key: "foo/all-ready",
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: Service("all-ready", "foo", WithRunLatestRollout,
				WithReadyConfig("all-ready-00001"),
				// The delta induced by route object.
				WithReadyRoute, WithSvcStatusDomain, WithSvcStatusAddress,
				WithSvcStatusTraffic(v1alpha1.TrafficTarget{
					TrafficTarget: v1beta1.TrafficTarget{
						RevisionName: "all-ready-00001",
						Percent:      100,
					},
				})),
		}},
		WantEvents: []string{
			Eventf(corev1.EventTypeNormal, "Updated", "Updated Service %q", "all-ready"),
		},
		WantServiceReadyStats: map[string]int{
			"foo/all-ready": 1,
		},
	}, {
		Name: "runLatest - route ready previous version and config ready, service not ready",
		// When both route and config are ready, but the route points to the previous revision
		// the service should not be ready.
		Objects: []runtime.Object{
			Service("config-only-ready", "foo", WithRunLatestRollout, WithInitSvcConditions),
			route("config-only-ready", "foo", WithRunLatestRollout, RouteReady,
				WithDomain, WithDomainInternal, WithAddress, WithInitRouteConditions,
				WithStatusTraffic(v1alpha1.TrafficTarget{
					TrafficTarget: v1beta1.TrafficTarget{
						RevisionName: "config-only-ready-00001",
						Percent:      100,
					},
				}), MarkTrafficAssigned, MarkIngressReady),
			config("config-only-ready", "foo", WithRunLatestRollout, WithGeneration(2 /*will generate revision -00002*/),
				// These turn a Configuration to Ready=true
				WithLatestCreated("config-only-ready-00002"), WithLatestReady("config-only-ready-00002")),
		},
		Key: "foo/config-only-ready",
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: Service("config-only-ready", "foo", WithRunLatestRollout,
				WithReadyConfig("config-only-ready-00002"),
				WithServiceStatusRouteNotReady, WithSvcStatusDomain, WithSvcStatusAddress,
				WithSvcStatusTraffic(v1alpha1.TrafficTarget{
					TrafficTarget: v1beta1.TrafficTarget{
						RevisionName: "config-only-ready-00001",
						Percent:      100,
					},
				})),
		}},
		WantEvents: []string{
			Eventf(corev1.EventTypeNormal, "Updated", "Updated Service %q", "config-only-ready"),
		},
	}, {
		Name: "runLatest - config fails, new gen, propagate failure",
		// Gen 1: everything is fine;
		// Gen 2: config update fails;
		//    => service is still OK serving Gen 1.
		Objects: []runtime.Object{
			Service("config-fails", "foo", WithRunLatestRollout, WithInitSvcConditions),
			route("config-fails", "foo", WithRunLatestRollout, RouteReady,
				WithDomain, WithDomainInternal, WithAddress, WithInitRouteConditions,
				WithStatusTraffic(v1alpha1.TrafficTarget{
					TrafficTarget: v1beta1.TrafficTarget{
						RevisionName: "config-fails-00001",
						Percent:      100,
					},
				}), MarkTrafficAssigned, MarkIngressReady),
			config("config-fails", "foo", WithRunLatestRollout,
				WithGeneration(2),
				WithLatestReady("config-fails-00001"),
				WithLatestCreated("config-fails-00002"), MarkLatestCreatedFailed("blah")),
		},
		Key: "foo/config-fails",
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: Service("config-fails", "foo", WithRunLatestRollout, WithInitSvcConditions,
				WithReadyRoute, WithSvcStatusDomain, WithSvcStatusAddress,
				WithSvcStatusTraffic(v1alpha1.TrafficTarget{
					TrafficTarget: v1beta1.TrafficTarget{
						RevisionName: "config-fails-00001",
						Percent:      100,
					},
				}),
				WithFailedConfig("config-fails-00002", "RevisionFailed", "blah"),
				WithServiceLatestReadyRevision("config-fails-00001")),
		}},
		WantEvents: []string{
			Eventf(corev1.EventTypeNormal, "Updated", "Updated Service %q", "config-fails"),
		},
	}, {
		Name: "runLatest - config fails, propagate failure",
		// When config fails, the service should fail.
		Objects: []runtime.Object{
			Service("config-fails", "foo", WithRunLatestRollout, WithInitSvcConditions),
			route("config-fails", "foo", WithRunLatestRollout, RouteReady),
			config("config-fails", "foo", WithRunLatestRollout, WithGeneration(1),
				WithLatestCreated("config-fails-00001"), MarkLatestCreatedFailed("blah")),
		},
		Key: "foo/config-fails",
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: Service("config-fails", "foo", WithRunLatestRollout, WithInitSvcConditions,
				WithServiceStatusRouteNotReady, WithFailedConfig(
					"config-fails-00001", "RevisionFailed", "blah")),
		}},
		WantEvents: []string{
			Eventf(corev1.EventTypeNormal, "Updated", "Updated Service %q", "config-fails"),
		},
	}, {
		Name: "runLatest - route fails, propagate failure",
		// When route fails, the service should fail.
		Objects: []runtime.Object{
			Service("route-fails", "foo", WithRunLatestRollout, WithInitSvcConditions),
			route("route-fails", "foo", WithRunLatestRollout,
				RouteFailed("Propagate me, please", "")),
			config("route-fails", "foo", WithRunLatestRollout, WithGeneration(1),
				// These turn a Configuration to Ready=true
				WithLatestCreated("route-fails-00001"), WithLatestReady("route-fails-00001")),
		},
		Key: "foo/route-fails",
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: Service("route-fails", "foo", WithRunLatestRollout, WithInitSvcConditions,
				// When the Configuration is Ready, and the Route has failed,
				// we expect the following changed to our status conditions.
				WithReadyConfig("route-fails-00001"),
				WithFailedRoute("Propagate me, please", "")),
		}},
		WantEvents: []string{
			Eventf(corev1.EventTypeNormal, "Updated", "Updated Service %q", "route-fails"),
		},
	}, {
		Name:    "runLatest - not owned config exists",
		WantErr: true,
		Objects: []runtime.Object{
			Service("run-latest", "foo", WithRunLatestRollout),
			config("run-latest", "foo", WithRunLatestRollout, WithConfigOwnersRemoved),
		},
		Key: "foo/run-latest",
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: Service("run-latest", "foo", WithRunLatestRollout,
				// The first reconciliation will initialize the status conditions.
				WithInitSvcConditions, MarkConfigurationNotOwned),
		}},
		WantEvents: []string{
			Eventf(corev1.EventTypeWarning, "InternalError", `Service: "run-latest" does not own Configuration: "run-latest"`),
		},
	}, {
		Name:    "runLatest - not owned route exists",
		WantErr: true,
		Objects: []runtime.Object{
			Service("run-latest", "foo", WithRunLatestRollout),
			config("run-latest", "foo", WithRunLatestRollout),
			route("run-latest", "foo", WithRunLatestRollout, WithRouteOwnersRemoved),
		},
		Key: "foo/run-latest",
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: Service("run-latest", "foo", WithRunLatestRollout,
				// The first reconciliation will initialize the status conditions.
				WithInitSvcConditions, MarkRouteNotOwned),
		}},
		WantEvents: []string{
			Eventf(corev1.EventTypeWarning, "InternalError", `Service: "run-latest" does not own Route: "run-latest"`),
		},
	}, {
		Name: "runLatest - correct not owned by adding owner refs",
		// If ready Route/Configuration that weren't owned have OwnerReferences attached,
		// then a Reconcile will result in the Service becoming happy.
		Objects: []runtime.Object{
			Service("new-owner", "foo", WithRunLatestRollout, WithInitSvcConditions,
				// This service was unhappy with the prior owner situation.
				MarkConfigurationNotOwned, MarkRouteNotOwned),
			// The service owns these, which should result in a happy result.
			route("new-owner", "foo", WithRunLatestRollout, RouteReady,
				WithDomain, WithDomainInternal, WithAddress, WithInitRouteConditions,
				WithStatusTraffic(v1alpha1.TrafficTarget{
					TrafficTarget: v1beta1.TrafficTarget{
						RevisionName: "new-owner-00001",
						Percent:      100,
					},
				}), MarkTrafficAssigned, MarkIngressReady),
			config("new-owner", "foo", WithRunLatestRollout, WithGeneration(1),
				// These turn a Configuration to Ready=true
				WithLatestCreated("new-owner-00001"), WithLatestReady("new-owner-00001")),
		},
		Key: "foo/new-owner",
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: Service("new-owner", "foo", WithRunLatestRollout,
				WithReadyConfig("new-owner-00001"),
				// The delta induced by route object.
				WithReadyRoute, WithSvcStatusDomain, WithSvcStatusAddress,
				WithSvcStatusTraffic(v1alpha1.TrafficTarget{
					TrafficTarget: v1beta1.TrafficTarget{
						RevisionName: "new-owner-00001",
						Percent:      100,
					},
				})),
		}},
		WantEvents: []string{
			Eventf(corev1.EventTypeNormal, "Updated", "Updated Service %q", "new-owner"),
		},
		WantServiceReadyStats: map[string]int{
			"foo/new-owner": 1,
		},
	}}

	defer logtesting.ClearAll()
	table.Test(t, MakeFactory(func(listers *Listers, opt reconciler.Options) controller.Reconciler {
		return &Reconciler{
			Base:                reconciler.NewBase(opt, controllerAgentName),
			serviceLister:       listers.GetServiceLister(),
			configurationLister: listers.GetConfigurationLister(),
			revisionLister:      listers.GetRevisionLister(),
			routeLister:         listers.GetRouteLister(),
		}
	}))
}

func TestNew(t *testing.T) {
	defer logtesting.ClearAll()
	kubeClient := fakekubeclientset.NewSimpleClientset()
	sharedClient := fakesharedclientset.NewSimpleClientset()
	servingClient := fakeclientset.NewSimpleClientset()
	servingInformer := informers.NewSharedInformerFactory(servingClient, 0)

	serviceInformer := servingInformer.Serving().V1alpha1().Services()
	routeInformer := servingInformer.Serving().V1alpha1().Routes()
	configurationInformer := servingInformer.Serving().V1alpha1().Configurations()
	revisionInformer := servingInformer.Serving().V1alpha1().Revisions()

	c := NewController(reconciler.Options{
		KubeClientSet:    kubeClient,
		SharedClientSet:  sharedClient,
		ServingClientSet: servingClient,
		Logger:           logtesting.TestLogger(t),
	}, serviceInformer, configurationInformer, revisionInformer, routeInformer)

	if c == nil {
		t.Fatal("Expected NewController to return a non-nil value")
	}
}

func config(name, namespace string, so ServiceOption, co ...ConfigOption) *v1alpha1.Configuration {
	s := Service(name, namespace, so)
	cfg, err := resources.MakeConfiguration(s)
	if err != nil {
		panic(fmt.Sprintf("MakeConfiguration() = %v", err))
	}
	for _, opt := range co {
		opt(cfg)
	}
	return cfg
}

func route(name, namespace string, so ServiceOption, ro ...RouteOption) *v1alpha1.Route {
	s := Service(name, namespace, so)
	route, err := resources.MakeRoute(s)
	if err != nil {
		panic(fmt.Sprintf("MakeRoute() = %v", err))
	}
	for _, opt := range ro {
		opt(route)
	}
	return route
}

// TODO(mattmoor): Replace these when we refactor Route's table_test.go
func MutateRoute(rt *v1alpha1.Route) {
	rt.Spec = v1alpha1.RouteSpec{}
}

func RouteReady(cfg *v1alpha1.Route) {
	cfg.Status = v1alpha1.RouteStatus{
		Status: duckv1beta1.Status{
			Conditions: duckv1beta1.Conditions{{
				Type:   "Ready",
				Status: "True",
			}},
		},
	}
}

func RouteFailed(reason, message string) RouteOption {
	return func(cfg *v1alpha1.Route) {
		cfg.Status = v1alpha1.RouteStatus{
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{{
					Type:    "Ready",
					Status:  "False",
					Reason:  reason,
					Message: message,
				}},
			},
		}
	}
}

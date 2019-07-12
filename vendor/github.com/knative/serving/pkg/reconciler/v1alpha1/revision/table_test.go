/*
Copyright 2018 The Knative Authors.

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

package revision

import (
	"context"
	"strconv"
	"testing"
	"time"

	caching "github.com/knative/caching/pkg/apis/caching/v1alpha1"
	"github.com/knative/pkg/apis/duck"
	"github.com/knative/pkg/configmap"
	"github.com/knative/pkg/controller"
	"github.com/knative/pkg/logging"
	logtesting "github.com/knative/pkg/logging/testing"
	autoscalingv1alpha1 "github.com/knative/serving/pkg/apis/autoscaling/v1alpha1"
	"github.com/knative/serving/pkg/apis/networking"
	"github.com/knative/serving/pkg/apis/serving"
	"github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"github.com/knative/serving/pkg/autoscaler"
	"github.com/knative/serving/pkg/network"
	"github.com/knative/serving/pkg/reconciler"
	"github.com/knative/serving/pkg/reconciler/v1alpha1/revision/config"
	"github.com/knative/serving/pkg/reconciler/v1alpha1/revision/resources"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientgotesting "k8s.io/client-go/testing"

	. "github.com/knative/pkg/reconciler/testing"
	. "github.com/knative/serving/pkg/reconciler/v1alpha1/testing"
)

// This is heavily based on the way the OpenShift Ingress controller tests its reconciliation method.
func TestReconcile(t *testing.T) {
	table := TableTest{{
		Name: "bad workqueue key",
		// Make sure Reconcile handles bad keys.
		Key: "too/many/parts",
	}, {
		Name: "key not found",
		// Make sure Reconcile handles good keys that don't exist.
		Key: "foo/not-found",
	}, {
		Name: "nop deletion reconcile",
		// Test that with a DeletionTimestamp we do nothing.
		Objects: []runtime.Object{
			rev("foo", "delete-pending", WithRevisionDeletionTimestamp),
		},
		Key: "foo/delete-pending",
	}, {
		Name: "first revision reconciliation",
		// Test the simplest successful reconciliation flow.
		// We feed in a well formed Revision where none of its sub-resources exist,
		// and we exect it to create them and initialize the Revision's status.
		Objects: []runtime.Object{
			rev("foo", "first-reconcile"),
		},
		WantCreates: []metav1.Object{
			// The first reconciliation of a Revision creates the following resources.
			kpa("foo", "first-reconcile"),
			deploy("foo", "first-reconcile"),
			image("foo", "first-reconcile"),
		},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: rev("foo", "first-reconcile",
				// The first reconciliation Populates the following status properties.
				WithLogURL, AllUnknownConditions, MarkDeploying("Deploying")),
		}},
		Key: "foo/first-reconcile",
	}, {
		Name: "failure updating revision status",
		// This starts from the first reconciliation case above and induces a failure
		// updating the revision status.
		WantErr: true,
		WithReactors: []clientgotesting.ReactionFunc{
			InduceFailure("update", "revisions"),
		},
		Objects: []runtime.Object{
			rev("foo", "update-status-failure"),
			kpa("foo", "update-status-failure"),
		},
		WantCreates: []metav1.Object{
			// We still see the following creates before the failure is induced.
			deploy("foo", "update-status-failure"),
			image("foo", "update-status-failure"),
		},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: rev("foo", "update-status-failure",
				// Despite failure, the following status properties are set.
				WithLogURL, AllUnknownConditions, MarkDeploying("Deploying")),
		}},
		WantEvents: []string{
			Eventf(corev1.EventTypeWarning, "UpdateFailed", "Failed to update status for Revision %q: %v",
				"update-status-failure", "inducing failure for update revisions"),
		},
		Key: "foo/update-status-failure",
	}, {
		Name: "failure creating kpa",
		// This starts from the first reconciliation case above and induces a failure
		// creating the KPA.
		WantErr: true,
		WithReactors: []clientgotesting.ReactionFunc{
			InduceFailure("create", "podautoscalers"),
		},
		Objects: []runtime.Object{
			rev("foo", "create-kpa-failure"),
		},
		WantCreates: []metav1.Object{
			// We still see the following creates before the failure is induced.
			kpa("foo", "create-kpa-failure"),
			deploy("foo", "create-kpa-failure"),
			image("foo", "create-kpa-failure"),
		},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: rev("foo", "create-kpa-failure",
				// Despite failure, the following status properties are set.
				WithLogURL, WithInitRevConditions,
				WithNoBuild, MarkDeploying("Deploying")),
		}},
		WantEvents: []string{
			Eventf(corev1.EventTypeWarning, "InternalError", "inducing failure for create podautoscalers"),
		},
		Key: "foo/create-kpa-failure",
	}, {
		Name: "failure creating user deployment",
		// This starts from the first reconciliation case above and induces a failure
		// creating the user's deployment
		WantErr: true,
		WithReactors: []clientgotesting.ReactionFunc{
			InduceFailure("create", "deployments"),
		},
		Objects: []runtime.Object{
			rev("foo", "create-user-deploy-failure"),
			kpa("foo", "create-user-deploy-failure"),
		},
		WantCreates: []metav1.Object{
			// We still see the following creates before the failure is induced.
			deploy("foo", "create-user-deploy-failure"),
		},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: rev("foo", "create-user-deploy-failure",
				// Despite failure, the following status properties are set.
				WithLogURL, WithInitRevConditions,
				WithNoBuild, MarkDeploying("Deploying")),
		}},
		WantEvents: []string{
			Eventf(corev1.EventTypeWarning, "InternalError", "inducing failure for create deployments"),
		},
		Key: "foo/create-user-deploy-failure",
	}, {
		Name: "stable revision reconciliation",
		// Test a simple stable reconciliation of an Active Revision.
		// We feed in a Revision and the resources it controls in a steady
		// state (immediately post-creation), and verify that no changes
		// are necessary.
		Objects: []runtime.Object{
			rev("foo", "stable-reconcile", WithLogURL, AllUnknownConditions),
			kpa("foo", "stable-reconcile"),
			deploy("foo", "stable-reconcile"),
			image("foo", "stable-reconcile"),
		},
		// No changes are made to any objects.
		Key: "foo/stable-reconcile",
	}, {
		Name: "update deployment containers",
		// Test that we update a deployment with new containers when they disagree
		// with our desired spec.
		Objects: []runtime.Object{
			rev("foo", "fix-containers",
				WithLogURL, AllUnknownConditions),
			kpa("foo", "fix-containers"),
			changeContainers(deploy("foo", "fix-containers")),
			image("foo", "fix-containers"),
		},
		WantUpdates: []clientgotesting.UpdateActionImpl{{
			Object: deploy("foo", "fix-containers"),
		}},
		Key: "foo/fix-containers",
	}, {
		Name: "failure updating deployment",
		// Test that we handle an error updating the deployment properly.
		WantErr: true,
		WithReactors: []clientgotesting.ReactionFunc{
			InduceFailure("update", "deployments"),
		},
		Objects: []runtime.Object{
			rev("foo", "failure-update-deploy",
				withK8sServiceName("whateves"), WithLogURL, AllUnknownConditions),
			kpa("foo", "failure-update-deploy"),
			changeContainers(deploy("foo", "failure-update-deploy")),
			image("foo", "failure-update-deploy"),
		},
		WantUpdates: []clientgotesting.UpdateActionImpl{{
			Object: deploy("foo", "failure-update-deploy"),
		}},
		WantEvents: []string{
			Eventf(corev1.EventTypeWarning, "InternalError", "inducing failure for update deployments"),
		},
		Key: "foo/failure-update-deploy",
	}, {
		Name: "deactivated revision is stable",
		// Test a simple stable reconciliation of an inactive Revision.
		// We feed in a Revision and the resources it controls in a steady
		// state (port-Reserve), and verify that no changes are necessary.
		Objects: []runtime.Object{
			rev("foo", "stable-deactivation",
				WithLogURL, MarkRevisionReady,
				MarkInactive("NoTraffic", "This thing is inactive.")),
			kpa("foo", "stable-deactivation",
				WithNoTraffic("NoTraffic", "This thing is inactive.")),
			deploy("foo", "stable-deactivation"),
			image("foo", "stable-deactivation"),
		},
		Key: "foo/stable-deactivation",
	}, {
		Name: "endpoint is created (not ready)",
		// Test the transition when a Revision's Endpoints are created (but not yet ready)
		// This initializes the state of the world to the steady-state after a Revision's
		// first reconciliation*.  It then introduces an Endpoints resource that's not yet
		// ready.  This should result in no change, since there isn't really any new
		// information.
		//
		// * - One caveat is that we have to explicitly set LastTransitionTime to keep us
		// from thinking we've been waiting for this Endpoint since the beginning of time
		// and declaring a timeout (this is the main difference from that test below).
		Objects: []runtime.Object{
			rev("foo", "endpoint-created-not-ready",
				WithLogURL, AllUnknownConditions),
			kpa("foo", "endpoint-created-not-ready"),
			deploy("foo", "endpoint-created-not-ready"),
			image("foo", "endpoint-created-not-ready"),
		},
		// No updates, since the endpoint didn't have meaningful status.
		Key: "foo/endpoint-created-not-ready",
	}, {
		Name: "kpa is ready",
		// Test the transition that Reconcile makes when Endpoints become ready.
		// This puts the world into the stable post-reconcile state for an Active
		// Revision.  It then creates an Endpoints resource with active subsets.
		// This signal should make our Reconcile mark the Revision as Ready.
		Objects: []runtime.Object{
			rev("foo", "kpa-ready",
				withK8sServiceName("old-stuff"), WithLogURL, AllUnknownConditions),
			kpa("foo", "kpa-ready", WithTraffic, WithPAStatusService("new-stuff")),
			deploy("foo", "kpa-ready"),
			image("foo", "kpa-ready"),
		},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: rev("foo", "kpa-ready", withK8sServiceName("new-stuff"),
				WithLogURL,
				// When the endpoint and KPA are ready, then we will see the
				// Revision become ready.
				MarkRevisionReady),
		}},
		WantEvents: []string{
			Eventf(corev1.EventTypeNormal, "RevisionReady", "Revision becomes ready upon all resources being ready"),
		},
		Key: "foo/kpa-ready",
	}, {
		Name: "kpa not ready",
		// Test propagating the KPA not ready status to the Revision.
		Objects: []runtime.Object{
			rev("foo", "kpa-not-ready",
				withK8sServiceName("somebody-told-me"), WithLogURL,
				MarkRevisionReady),
			kpa("foo", "kpa-not-ready",
				WithPAStatusService("its-not-confidential"),
				WithBufferedTraffic("Something", "This is something longer")),
			deploy("foo", "kpa-not-ready"),
			image("foo", "kpa-not-ready"),
		},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: rev("foo", "kpa-not-ready",
				WithLogURL, MarkRevisionReady,
				withK8sServiceName("its-not-confidential"),
				// When we reconcile a ready state and our KPA is in an activating
				// state, we should see the following mutation.
				MarkActivating("Something", "This is something longer"),
			),
		}},
		Key: "foo/kpa-not-ready",
	}, {
		Name: "kpa inactive",
		// Test propagating the inactivity signal from the KPA to the Revision.
		Objects: []runtime.Object{
			rev("foo", "kpa-inactive",
				withK8sServiceName("something-in-the-way"), WithLogURL, MarkRevisionReady),
			kpa("foo", "kpa-inactive",
				WithNoTraffic("NoTraffic", "This thing is inactive.")),
			deploy("foo", "kpa-inactive"),
			image("foo", "kpa-inactive"),
		},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: rev("foo", "kpa-inactive",
				WithLogURL, MarkRevisionReady,
				// When we reconcile an "all ready" revision when the KPA
				// is inactive, we should see the following change.
				MarkInactive("NoTraffic", "This thing is inactive.")),
		}},
		Key: "foo/kpa-inactive",
	}, {
		Name: "kpa inactive, but has service",
		// Test propagating the inactivity signal from the KPA to the Revision.
		// But propagatethe service name.
		Objects: []runtime.Object{
			rev("foo", "kpa-inactive",
				withK8sServiceName("here-comes-the-sun"), WithLogURL, MarkRevisionReady),
			kpa("foo", "kpa-inactive",
				WithNoTraffic("NoTraffic", "This thing is inactive."),
				WithPAStatusService("kpa-inactive-svc")),
			deploy("foo", "kpa-inactive"),
			image("foo", "kpa-inactive"),
		},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: rev("foo", "kpa-inactive",
				WithLogURL, MarkRevisionReady,
				withK8sServiceName("kpa-inactive-svc"),
				// When we reconcile an "all ready" revision when the KPA
				// is inactive, we should see the following change.
				MarkInactive("NoTraffic", "This thing is inactive.")),
		}},
		Key: "foo/kpa-inactive",
	}, {
		Name: "mutated KPA gets fixed",
		// This test validates, that when users mess with the KPA directly
		// we bring it back to the required shape.
		// Protocol type is the only thing that can be changed on KPA
		Objects: []runtime.Object{
			rev("foo", "fix-mutated-kpa",
				withK8sServiceName("ill-follow-the-sun"), WithLogURL, MarkRevisionReady),
			kpa("foo", "fix-mutated-kpa", WithProtocolType(networking.ProtocolH2C),
				WithTraffic, WithPAStatusService("fix-mutated-kpa")),
			deploy("foo", "fix-mutated-kpa"),
			image("foo", "fix-mutated-kpa"),
		},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: rev("foo", "fix-mutated-kpa",
				WithLogURL, AllUnknownConditions,
				// When our reconciliation has to change the service
				// we should see the following mutations to status.
				withK8sServiceName("fix-mutated-kpa"), WithLogURL, MarkRevisionReady),
		}},
		WantUpdates: []clientgotesting.UpdateActionImpl{{
			Object: kpa("foo", "fix-mutated-kpa", WithTraffic,
				WithPAStatusService("fix-mutated-kpa")),
		}},
		Key: "foo/fix-mutated-kpa",
	}, {
		Name: "mutated KPA gets error during the fix",
		// Same as above, but will fail during the update.
		Objects: []runtime.Object{
			rev("foo", "fix-mutated-kpa-fail",
				withK8sServiceName("some-old-stuff"),
				WithLogURL, AllUnknownConditions),
			kpa("foo", "fix-mutated-kpa-fail", WithProtocolType(networking.ProtocolH2C)),
			deploy("foo", "fix-mutated-kpa-fail"),
			image("foo", "fix-mutated-kpa-fail"),
		},
		WantErr: true,
		WithReactors: []clientgotesting.ReactionFunc{
			InduceFailure("update", "podautoscalers"),
		},
		WantUpdates: []clientgotesting.UpdateActionImpl{{
			Object: kpa("foo", "fix-mutated-kpa-fail"),
		}},
		WantEvents: []string{
			Eventf(corev1.EventTypeWarning, "InternalError", "inducing failure for update podautoscalers"),
		},
		Key: "foo/fix-mutated-kpa-fail",
	}, {
		Name: "surface deployment timeout",
		// Test the propagation of ProgressDeadlineExceeded from Deployment.
		// This initializes the world to the stable state after its first reconcile,
		// but changes the user deployment to have a ProgressDeadlineExceeded
		// condition.  It then verifies that Reconcile propagates this into the
		// status of the Revision.
		Objects: []runtime.Object{
			rev("foo", "deploy-timeout",
				withK8sServiceName("the-taxman"), WithLogURL, MarkActive),
			kpa("foo", "deploy-timeout"), // KPA can't be ready since deployment times out.
			timeoutDeploy(deploy("foo", "deploy-timeout")),
			image("foo", "deploy-timeout"),
		},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: rev("foo", "deploy-timeout",
				WithLogURL, AllUnknownConditions,
				// When the revision is reconciled after a Deployment has
				// timed out, we should see it marked with the PDE state.
				MarkProgressDeadlineExceeded),
		}},
		WantEvents: []string{
			Eventf(corev1.EventTypeNormal, "ProgressDeadlineExceeded",
				"Revision %s not ready due to Deployment timeout",
				"deploy-timeout"),
		},
		Key: "foo/deploy-timeout",
	}, {
		Name: "surface pod errors",
		// Test the propagation of the termination state of a Pod into the revision.
		// This initializes the world to the stable state after its first reconcile,
		// but changes the user deployment to have a failing pod. It then verifies
		// that Reconcile propagates this into the status of the Revision.
		Objects: []runtime.Object{
			rev("foo", "pod-error",
				withK8sServiceName("a-pod-error"), WithLogURL, AllUnknownConditions, MarkActive),
			kpa("foo", "pod-error"), // PA can't be ready, since no traffic.
			pod("foo", "pod-error", WithFailingContainer("user-container", 5, "I failed man!")),
			deploy("foo", "pod-error"),
			image("foo", "pod-error"),
		},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: rev("foo", "pod-error",
				WithLogURL, AllUnknownConditions, MarkContainerExiting(5, "I failed man!")),
		}},
		Key: "foo/pod-error",
	}, {
		Name: "build missing",
		// Test a Reconcile of a Revision with a Build that is not found.
		// We seed the world with a freshly created Revision that has a BuildName.
		// We then verify that a Reconcile does effectively nothing but initialize
		// the conditions of this Revision.  It is notable that unlike the tests
		// above, this will include a BuildSucceeded condition.
		Objects: []runtime.Object{
			rev("foo", "missing-build", WithBuildRef("the-build")),
		},
		WantErr: true,
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: rev("foo", "missing-build", WithBuildRef("the-build"),
				// When we first reconcile a revision with a Build (that's missing)
				// we should see the following status changes.
				WithLogURL, WithInitRevConditions),
		}},
		WantEvents: []string{
			Eventf(corev1.EventTypeWarning, "InternalError", `builds.testing.build.knative.dev "the-build" not found`),
		},
		Key: "foo/missing-build",
	}, {
		Name: "build running",
		// Test a Reconcile of a Revision with a Build that is not done.
		// We seed the world with a freshly created Revision that has a BuildName.
		// We then verify that a Reconcile does effectively nothing but initialize
		// the conditions of this Revision.  It is notable that unlike the tests
		// above, this will include a BuildSucceeded condition.
		Objects: []runtime.Object{
			rev("foo", "running-build", WithBuildRef("the-build")),
			build("foo", "the-build", WithSucceededUnknown("", "")),
		},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: rev("foo", "running-build", WithBuildRef("the-build"),
				// When we first reconcile a revision with a Build (not done)
				// we should see the following status changes.
				WithLogURL, WithInitRevConditions, WithOngoingBuild),
		}},
		Key: "foo/running-build",
	}, {
		Name: "build newly done",
		// Test a Reconcile of a Revision with a Build that is just done.
		// We seed the world with a freshly created Revision that has a BuildName,
		// and a Build that has a `Succeeded: True` status. We then verify that a
		// Reconcile toggles the BuildSucceeded status and then acts similarly to
		// the first reconcile of a BYO-Container Revision.
		Objects: []runtime.Object{
			rev("foo", "done-build", WithBuildRef("the-build"), WithInitRevConditions),
			build("foo", "the-build", WithSucceededTrue),
		},
		WantCreates: []metav1.Object{
			// The first reconciliation of a Revision creates the following resources.
			kpa("foo", "done-build"),
			deploy("foo", "done-build"),
			image("foo", "done-build"),
		},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: rev("foo", "done-build", WithBuildRef("the-build"), WithInitRevConditions,
				// When we reconcile a Revision after the Build completes, we should
				// see the following updates to its status.
				WithLogURL, WithSuccessfulBuild,
				MarkDeploying("Deploying"), MarkActivating("Deploying", "")),
		}},
		WantEvents: []string{
			Eventf(corev1.EventTypeNormal, "BuildSucceeded", ""),
		},
		Key: "foo/done-build",
	}, {
		Name: "stable revision reconciliation (with build)",
		// Test a simple stable reconciliation of an Active Revision with a done Build.
		// We feed in a Revision and the resources it controls in a steady
		// state (immediately post-build completion), and verify that no changes
		// are necessary.
		Objects: []runtime.Object{
			rev("foo", "stable-reconcile-with-build",
				WithBuildRef("the-build"), WithLogURL,
				WithInitRevConditions, WithSuccessfulBuild,
				MarkDeploying("Deploying"), MarkActivating("Deploying", "")),
			kpa("foo", "stable-reconcile-with-build"),
			build("foo", "the-build", WithSucceededTrue),
			deploy("foo", "stable-reconcile-with-build"),
			svc("foo", "stable-reconcile-with-build"),
			image("foo", "stable-reconcile-with-build"),
		},
		// No changes are made to any objects.
		Key: "foo/stable-reconcile-with-build",
	}, {
		Name: "build newly failed",
		// Test a Reconcile of a Revision with a Build that has just failed.
		// We seed the world with a freshly created Revision that has a BuildName,
		// and a Build that has just failed. We then verify that a Reconcile toggles
		// the BuildSucceeded status and stops.
		Objects: []runtime.Object{
			rev("foo", "failed-build", WithBuildRef("the-build"), WithLogURL, WithInitRevConditions),
			build("foo", "the-build",
				WithSucceededFalse("SomeReason", "This is why the build failed.")),
		},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: rev("foo", "failed-build",
				WithBuildRef("the-build"), WithLogURL, WithInitRevConditions,
				// When we reconcile a Revision whose build has failed, we sill see that
				// failure reflected in the Revision status as follows:
				WithFailedBuild("SomeReason", "This is why the build failed.")),
		}},
		WantEvents: []string{
			Eventf(corev1.EventTypeWarning, "BuildFailed", "This is why the build failed."),
		},
		Key: "foo/failed-build",
	}, {
		Name: "build failed stable",
		// Test a Reconcile of a Revision with a Build that has previously failed.
		// We seed the world with a Revision that has a BuildName, and a Build that
		// has failed, which has been previously reconcile. We then verify that a
		// Reconcile has nothing to change.
		Objects: []runtime.Object{
			rev("foo", "failed-build-stable", WithBuildRef("the-build"), WithInitRevConditions,
				WithLogURL, WithFailedBuild("SomeReason", "This is why the build failed.")),
			build("foo", "the-build",
				WithSucceededFalse("SomeReason", "This is why the build failed.")),
		},
		Key: "foo/failed-build-stable",
	}, {
		Name: "ready steady state",
		// Test the transition that Reconcile makes when Endpoints become ready on the
		// SKS owned services, which is signalled by KPA having servince name.
		// This puts the world into the stable post-reconcile state for an Active
		// Revision.  It then creates an Endpoints resource with active subsets.
		// This signal should make our Reconcile mark the Revision as Ready.
		Objects: []runtime.Object{
			rev("foo", "steady-ready", withK8sServiceName("very-steady"), WithLogURL),
			kpa("foo", "steady-ready", WithTraffic, WithPAStatusService("steadier-even")),
			deploy("foo", "steady-ready"),
			svc("foo", "steady-ready"),
			endpoints("foo", "steady-ready", WithSubsets),
			image("foo", "steady-ready"),
		},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: rev("foo", "steady-ready", withK8sServiceName("steadier-even"), WithLogURL,
				// All resources are ready to go, we should see the revision being
				// marked ready
				MarkRevisionReady),
		}},
		WantEvents: []string{
			Eventf(corev1.EventTypeNormal, "RevisionReady", "Revision becomes ready upon all resources being ready"),
		},
		Key: "foo/steady-ready",
	}, {
		Name:    "lost kpa owner ref",
		WantErr: true,
		Objects: []runtime.Object{
			rev("foo", "missing-owners", withK8sServiceName("lesser-revision"), WithLogURL,
				MarkRevisionReady),
			kpa("foo", "missing-owners", WithTraffic, WithPodAutoscalerOwnersRemoved),
			deploy("foo", "missing-owners"),
			svc("foo", "missing-owners"),
			endpoints("foo", "missing-owners", WithSubsets),
			image("foo", "missing-owners"),
		},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: rev("foo", "missing-owners", withK8sServiceName("lesser-revision"), WithLogURL,
				MarkRevisionReady,
				// When we're missing the OwnerRef for PodAutoscaler we see this update.
				MarkResourceNotOwned("PodAutoscaler", "missing-owners")),
		}},
		WantEvents: []string{
			Eventf(corev1.EventTypeWarning, "InternalError", `Revision: "missing-owners" does not own PodAutoscaler: "missing-owners"`),
		},
		Key: "foo/missing-owners",
	}, {
		Name:    "lost deployment owner ref",
		WantErr: true,
		Objects: []runtime.Object{
			rev("foo", "missing-owners", withK8sServiceName("youre-gonna-lose"), WithLogURL,
				MarkRevisionReady),
			kpa("foo", "missing-owners", WithTraffic),
			noOwner(deploy("foo", "missing-owners")),
			svc("foo", "missing-owners"),
			endpoints("foo", "missing-owners", WithSubsets),
			image("foo", "missing-owners"),
		},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: rev("foo", "missing-owners", withK8sServiceName("youre-gonna-lose"), WithLogURL,
				MarkRevisionReady,
				// When we're missing the OwnerRef for Deployment we see this update.
				MarkResourceNotOwned("Deployment", "missing-owners-deployment")),
		}},
		WantEvents: []string{
			Eventf(corev1.EventTypeWarning, "InternalError", `Revision: "missing-owners" does not own Deployment: "missing-owners-deployment"`),
		},
		Key: "foo/missing-owners",
	}}

	defer logtesting.ClearAll()
	table.Test(t, MakeFactory(func(listers *Listers, opt reconciler.Options) controller.Reconciler {
		t := &NullTracker{}
		buildInformerFactory := KResourceTypedInformerFactory(opt)
		return &Reconciler{
			Base:                reconciler.NewBase(opt, controllerAgentName),
			revisionLister:      listers.GetRevisionLister(),
			podAutoscalerLister: listers.GetPodAutoscalerLister(),
			imageLister:         listers.GetImageLister(),
			deploymentLister:    listers.GetDeploymentLister(),
			serviceLister:       listers.GetK8sServiceLister(),
			endpointsLister:     listers.GetEndpointsLister(),
			configMapLister:     listers.GetConfigMapLister(),
			resolver:            &nopResolver{},
			tracker:             t,
			configStore:         &testConfigStore{config: ReconcilerTestConfig()},

			buildInformerFactory: newDuckInformerFactory(t, buildInformerFactory),
		}
	}))
}

func TestReconcileWithVarLogEnabled(t *testing.T) {
	table := TableTest{{
		Name: "first revision reconciliation (with /var/log enabled)",
		// Test a successful reconciliation flow with /var/log enabled.
		// We feed in a well formed Revision where none of its sub-resources exist,
		// and we exect it to create them and initialize the Revision's status.
		// This is similar to "first-reconcile", but should also create a fluentd configmap.
		Objects: []runtime.Object{
			rev("foo", "first-reconcile-var-log"),
		},
		WantCreates: []metav1.Object{
			// The first reconciliation of a Revision creates the following resources.
			kpa("foo", "first-reconcile-var-log"),
			deploy("foo", "first-reconcile-var-log", EnableVarLog),
			fluentdConfigMap("foo", "first-reconcile-var-log", EnableVarLog),
			image("foo", "first-reconcile-var-log", EnableVarLog),
		},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: rev("foo", "first-reconcile-var-log",
				// After the first reconciliation of a Revision the status looks like this.
				WithLogURL, AllUnknownConditions, MarkDeploying("Deploying")),
		}},
		Key: "foo/first-reconcile-var-log",
	}, {
		Name: "failure creating fluentd configmap",
		// Induce a failure creating the fluentd configmap.
		WantErr: true,
		WithReactors: []clientgotesting.ReactionFunc{
			InduceFailure("create", "configmaps"),
		},
		Objects: []runtime.Object{
			rev("foo", "create-configmap-failure"),
		},
		WantCreates: []metav1.Object{
			deploy("foo", "create-configmap-failure", EnableVarLog),
			fluentdConfigMap("foo", "create-configmap-failure", EnableVarLog),
			image("foo", "create-configmap-failure", EnableVarLog),
		},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: rev("foo", "create-configmap-failure",
				// When our first reconciliation is interrupted by a failure creating
				// the fluentd configmap, we should still see the following reflected
				// in our status.
				WithLogURL, WithInitRevConditions,
				WithNoBuild, MarkDeploying("Deploying")),
		}},
		WantEvents: []string{
			Eventf(corev1.EventTypeWarning, "InternalError", "inducing failure for create configmaps"),
		},
		Key: "foo/create-configmap-failure",
	}, {
		Name: "steady state after initial creation",
		// Verify that after creating the things from an initial reconcile that we're stable.
		Objects: []runtime.Object{
			rev("foo", "steady-state",
				WithLogURL, AllUnknownConditions),
			kpa("foo", "steady-state"),
			deploy("foo", "steady-state", EnableVarLog),
			fluentdConfigMap("foo", "steady-state", EnableVarLog),
			image("foo", "steady-state", EnableVarLog),
		},
		Key: "foo/steady-state",
	}, {
		Name: "update a bad fluentd configmap",
		// Verify that after creating the things from an initial reconcile that we're stable.
		Objects: []runtime.Object{
			rev("foo", "update-fluentd-config",
				WithLogURL, AllUnknownConditions),
			kpa("foo", "update-fluentd-config"),
			deploy("foo", "update-fluentd-config", EnableVarLog),
			&corev1.ConfigMap{
				// Use the ObjectMeta, but discard the rest.
				ObjectMeta: fluentdConfigMap("foo", "update-fluentd-config",
					EnableVarLog).ObjectMeta,
				Data: map[string]string{
					"bad key": "bad value",
				},
			},
			image("foo", "update-fluentd-config", EnableVarLog),
		},
		WantUpdates: []clientgotesting.UpdateActionImpl{{
			// We should see a single update to the configmap we expect.
			Object: fluentdConfigMap("foo", "update-fluentd-config", EnableVarLog),
		}},
		Key: "foo/update-fluentd-config",
	}, {
		Name: "failure updating fluentd configmap",
		// Induce a failure trying to update the fluentd configmap
		WantErr: true,
		WithReactors: []clientgotesting.ReactionFunc{
			InduceFailure("update", "configmaps"),
		},
		Objects: []runtime.Object{
			rev("foo", "update-configmap-failure",
				withK8sServiceName("more-failures"), WithLogURL, AllUnknownConditions),
			deploy("foo", "update-configmap-failure", EnableVarLog),
			&corev1.ConfigMap{
				// Use the ObjectMeta, but discard the rest.
				ObjectMeta: fluentdConfigMap("foo", "update-configmap-failure",
					EnableVarLog).ObjectMeta,
				Data: map[string]string{
					"bad key": "bad value",
				},
			},
			image("foo", "update-configmap-failure", EnableVarLog),
		},
		WantUpdates: []clientgotesting.UpdateActionImpl{{
			// We should see a single update to the configmap we expect.
			Object: fluentdConfigMap("foo", "update-configmap-failure", EnableVarLog),
		}},
		WantEvents: []string{
			Eventf(corev1.EventTypeWarning, "InternalError", "inducing failure for update configmaps"),
		},
		Key: "foo/update-configmap-failure",
	}}

	config := ReconcilerTestConfig()
	EnableVarLog(config)

	defer logtesting.ClearAll()

	table.Test(t, MakeFactory(func(listers *Listers, opt reconciler.Options) controller.Reconciler {
		return &Reconciler{
			Base:                reconciler.NewBase(opt, controllerAgentName),
			revisionLister:      listers.GetRevisionLister(),
			podAutoscalerLister: listers.GetPodAutoscalerLister(),
			imageLister:         listers.GetImageLister(),
			deploymentLister:    listers.GetDeploymentLister(),
			serviceLister:       listers.GetK8sServiceLister(),
			endpointsLister:     listers.GetEndpointsLister(),
			configMapLister:     listers.GetConfigMapLister(),
			resolver:            &nopResolver{},
			tracker:             &NullTracker{},
			configStore:         &testConfigStore{config: config},
		}
	}))
}

func timeoutDeploy(deploy *appsv1.Deployment) *appsv1.Deployment {
	deploy.Status.Conditions = []appsv1.DeploymentCondition{{
		Type:   appsv1.DeploymentProgressing,
		Status: corev1.ConditionFalse,
		Reason: "ProgressDeadlineExceeded",
	}}
	return deploy
}

func noOwner(deploy *appsv1.Deployment) *appsv1.Deployment {
	deploy.OwnerReferences = nil
	return deploy
}

func changeContainers(deploy *appsv1.Deployment) *appsv1.Deployment {
	podSpec := deploy.Spec.Template.Spec
	for i := range podSpec.Containers {
		podSpec.Containers[i].Image = "asdf"
	}
	return deploy
}

// Build is a special case of resource creation because it isn't owned by
// the Revision, just tracked.
func build(namespace, name string, bo ...BuildOption) *unstructured.Unstructured {
	b := &unstructured.Unstructured{}
	b.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "testing.build.knative.dev",
		Version: "v1alpha1",
		Kind:    "Build",
	})
	b.SetName(name)
	b.SetNamespace(namespace)
	u := &unstructured.Unstructured{}
	duck.FromUnstructured(b, u) // prevent panic in b.DeepCopy()

	for _, opt := range bo {
		opt(u)
	}
	return u
}

func rev(namespace, name string, ro ...RevisionOption) *v1alpha1.Revision {
	r := &v1alpha1.Revision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       "test-uid",
		},
		Spec: v1alpha1.RevisionSpec{
			Container: &corev1.Container{
				Image: "busybox",
			},
		},
	}

	for _, opt := range ro {
		opt(r)
	}
	return r
}

func withK8sServiceName(sn string) RevisionOption {
	return func(r *v1alpha1.Revision) {
		r.Status.ServiceName = sn
	}
}

// TODO(mattmoor): Come up with a better name for this.
func AllUnknownConditions(r *v1alpha1.Revision) {
	WithInitRevConditions(r)
	WithNoBuild(r)
	MarkDeploying("")(r)
	MarkActivating("Deploying", "")(r)
}

type configOption func(*config.Config)

func deploy(namespace, name string, opts ...interface{}) *appsv1.Deployment {
	cfg := ReconcilerTestConfig()

	for _, opt := range opts {
		if configOpt, ok := opt.(configOption); ok {
			configOpt(cfg)
		}
	}

	rev := rev(namespace, name)

	for _, opt := range opts {
		if revOpt, ok := opt.(RevisionOption); ok {
			revOpt(rev)
		}
	}

	// Do this here instead of in `rev` itself to ensure that we populate defaults
	// before calling MakeDeployment within Reconcile.
	rev.SetDefaults(context.Background())
	return resources.MakeDeployment(rev, cfg.Logging, cfg.Network,
		cfg.Observability, cfg.Autoscaler, cfg.Controller,
	)

}

func image(namespace, name string, co ...configOption) *caching.Image {
	config := ReconcilerTestConfig()
	for _, opt := range co {
		opt(config)
	}

	return resources.MakeImageCache(rev(namespace, name))
}

func fluentdConfigMap(namespace, name string, co ...configOption) *corev1.ConfigMap {
	config := ReconcilerTestConfig()
	for _, opt := range co {
		opt(config)
	}

	rev := rev(namespace, name)
	return resources.MakeFluentdConfigMap(rev, config.Observability)
}

func kpa(namespace, name string, ko ...PodAutoscalerOption) *autoscalingv1alpha1.PodAutoscaler {
	rev := rev(namespace, name)
	k := resources.MakeKPA(rev)

	for _, opt := range ko {
		opt(k)
	}
	return k
}

func svc(namespace, name string, so ...K8sServiceOption) *corev1.Service {
	rev := rev(namespace, name)
	s := resources.MakeK8sService(rev)
	for _, opt := range so {
		opt(s)
	}
	return s
}

func endpoints(namespace, name string, eo ...EndpointsOption) *corev1.Endpoints {
	service := svc(namespace, name)
	ep := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:         service.Namespace,
			Name:              service.Name,
			CreationTimestamp: metav1.Time{time.Now()},
		},
	}
	for _, opt := range eo {
		opt(ep)
	}
	return ep
}

func pod(namespace, name string, po ...PodOption) *corev1.Pod {
	deploy := deploy(namespace, name)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
			Labels:    deploy.Labels,
		},
	}

	for _, opt := range po {
		opt(pod)
	}
	return pod
}

type testConfigStore struct {
	config *config.Config
}

func (t *testConfigStore) ToContext(ctx context.Context) context.Context {
	return config.ToContext(ctx, t.config)
}

func (t *testConfigStore) Load() *config.Config {
	return t.config
}

func (t *testConfigStore) WatchConfigs(w configmap.Watcher) {}

var _ configStore = (*testConfigStore)(nil)

func ReconcilerTestConfig() *config.Config {
	return &config.Config{
		Controller: getTestControllerConfig(),
		Network:    &network.Config{IstioOutboundIPRanges: "*"},
		Observability: &config.Observability{
			LoggingURLTemplate: "http://logger.io/${REVISION_UID}",
		},
		Logging:    &logging.Config{},
		Autoscaler: &autoscaler.Config{},
	}
}

// this forces the type to be a 'configOption'
var EnableVarLog configOption = func(cfg *config.Config) {
	cfg.Observability.EnableVarLogCollection = true
}

// WithConfigurationGenerationLabel sets the label on the revision
func WithConfigurationGenerationLabel(generation int) RevisionOption {
	return func(r *v1alpha1.Revision) {
		if r.Labels == nil {
			r.Labels = make(map[string]string)
		}
		r.Labels[serving.ConfigurationGenerationLabelKey] = strconv.Itoa(generation)
	}
}

// WithConfigurationGenerationLabel sets the label on the revision
func WithConfigurationGenerationAnnotation(generation int) RevisionOption {
	return func(r *v1alpha1.Revision) {
		if r.Annotations == nil {
			r.Annotations = make(map[string]string)
		}
		r.Annotations[serving.ConfigurationGenerationLabelKey] = strconv.Itoa(generation)
	}
}

// WithDeprecatedConfigurationMetadataGenerationLabel sets the label on the revision
func WithDeprecatedConfigurationMetadataGenerationLabel(generation int) RevisionOption {
	return func(r *v1alpha1.Revision) {
		if r.Labels == nil {
			r.Labels = make(map[string]string)
		}

		r.Labels[serving.DeprecatedConfigurationMetadataGenerationLabelKey] = strconv.Itoa(generation)
	}
}

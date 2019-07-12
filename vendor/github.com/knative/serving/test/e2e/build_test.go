// +build e2e

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

package e2e

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
	buildv1alpha1 "github.com/knative/build/pkg/apis/build/v1alpha1"
	"github.com/knative/pkg/apis/duck"
	testbuildv1alpha1 "github.com/knative/serving/test/apis/testing/v1alpha1"
	"github.com/mattbaird/jsonpatch"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	duckv1alpha1 "github.com/knative/pkg/apis/duck/v1alpha1"
	_ "github.com/knative/pkg/system/testing"
	pkgTest "github.com/knative/pkg/test"
	"github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"github.com/knative/serving/test"
)

var buildCondSet = duckv1alpha1.NewBatchConditionSet()

func TestBuildSpecAndServe(t *testing.T) {
	t.Parallel()
	clients := Setup(t)

	t.Log("Creating a new Route and Configuration with build")

	svcName := test.ObjectNameForTest(t)
	names := test.ResourceNames{
		Config: svcName,
		Route:  svcName,
		Image:  "helloworld",
	}

	build := &v1alpha1.RawExtension{
		BuildSpec: &buildv1alpha1.BuildSpec{
			Steps: []corev1.Container{{
				Image: "ubuntu",
				Args:  []string{"echo", "built"},
			}},
		},
	}

	if _, err := clients.ServingClient.Configs.Create(test.ConfigurationWithBuild(names, build)); err != nil {
		t.Fatalf("Failed to create Configuration: %v", err)
	}
	if _, err := clients.ServingClient.Routes.Create(test.Route(names)); err != nil {
		t.Fatalf("Failed to create Route: %v", err)
	}

	test.CleanupOnInterrupt(func() { test.TearDown(clients, names) })
	defer test.TearDown(clients, names)

	t.Log("When the Revision can have traffic routed to it, the Route is marked as Ready.")
	if err := test.WaitForRouteState(clients.ServingClient, names.Route, test.IsRouteReady, "RouteIsReady"); err != nil {
		t.Fatalf("The Route %s was not marked as Ready to serve traffic: %v", names.Route, err)
	}

	route, err := clients.ServingClient.Routes.Get(names.Route, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Error fetching Route %s: %v", names.Route, err)
	}
	domain := route.Status.Domain

	endState := test.RetryingRouteInconsistency(pkgTest.MatchesAllOf(pkgTest.IsStatusOK, pkgTest.MatchesBody(helloWorldExpectedOutput)))
	if _, err := pkgTest.WaitForEndpointState(clients.KubeClient, t.Logf, domain, endState, "HelloWorldServesText", test.ServingFlags.ResolvableDomain); err != nil {
		t.Fatalf("The endpoint for Route %s at domain %s didn't serve the expected text \"%s\": %v", names.Route, domain, helloWorldExpectedOutput, err)
	}

	// Get Configuration's latest ready Revision's Build, and check that the Build was successful.
	t.Log("Revision is ready and serving, checking Build status.")
	config, err := clients.ServingClient.Configs.Get(names.Config, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get Configuration after it was seen to be live: %v", err)
	}
	rev, err := clients.ServingClient.Revisions.Get(config.Status.LatestReadyRevisionName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get latest Revision: %v", err)
	}
	buildName := rev.Spec.BuildRef.Name
	t.Logf("Latest ready Revision is %q; its build is: %q", rev.Name, buildName)
	u, err := clients.Dynamic.Resource(schema.GroupVersionResource{
		Group:    buildv1alpha1.SchemeGroupVersion.Group,
		Version:  buildv1alpha1.SchemeGroupVersion.Version,
		Resource: "builds",
	}).Namespace(test.ServingNamespace).Get(buildName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get build for latest revision: %v", err)
	}
	b := &duckv1alpha1.KResource{}
	if err := duck.FromUnstructured(u, b); err != nil {
		t.Fatalf("Failed to cast to KResource: %+v", u)
	}
	if cond := buildCondSet.Manage(&b.Status).GetCondition(duckv1alpha1.ConditionSucceeded); cond == nil {
		t.Fatalf("Condition for build %q was nil", buildName)
	} else if cond.Status != corev1.ConditionTrue {
		t.Fatalf("Build %q was not successful", buildName)
	}

	// Update the Configuration with an environment variable, which should trigger a new revision
	// to be created, but without creating a new build.
	if err := updateConfigWithEnvVars(clients, names, []corev1.EnvVar{{
		Name:  "FOO",
		Value: "bar",
	}}); err != nil {
		t.Fatalf("Failed to update config with environment variables: %v", err)
	}

	nextRevName, err := getNextRevisionName(clients, names)
	if err != nil {
		t.Fatalf("Error waiting for next revision to be created: %v", err)
	}
	names.Revision = nextRevName

	nextRev, err := clients.ServingClient.Revisions.Get(nextRevName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get latest Revision: %v", err)
	}

	if diff := cmp.Diff(rev.Spec.BuildRef, nextRev.Spec.BuildRef); diff != "" {
		t.Fatalf("Unexpected differences in BuildRef: %v", diff)
	}
}

func TestBuildAndServe(t *testing.T) {
	t.Parallel()
	clients := Setup(t)
	t.Log("Creating a new Route and Configuration with build")

	svcName := test.ObjectNameForTest(t)
	names := test.ResourceNames{
		Config: svcName,
		Route:  svcName,
		Image:  "helloworld",
	}

	build := &v1alpha1.RawExtension{
		Object: &testbuildv1alpha1.Build{
			TypeMeta: metav1.TypeMeta{
				APIVersion: testbuildv1alpha1.SchemeGroupVersion.String(),
				Kind:       "Build",
			},
		},
	}

	if _, err := clients.ServingClient.Configs.Create(test.ConfigurationWithBuild(names, build)); err != nil {
		t.Fatalf("Failed to create Configuration: %v", err)
	}
	if _, err := clients.ServingClient.Routes.Create(test.Route(names)); err != nil {
		t.Fatalf("Failed to create Route: %v", err)
	}

	test.CleanupOnInterrupt(func() { test.TearDown(clients, names) })
	defer test.TearDown(clients, names)

	t.Log("When the Revision can have traffic routed to it, the Route is marked as Ready.")
	if err := test.WaitForRouteState(clients.ServingClient, names.Route, test.IsRouteReady, "RouteIsReady"); err != nil {
		t.Fatalf("The Route %s was not marked as Ready to serve traffic: %v", names.Route, err)
	}

	route, err := clients.ServingClient.Routes.Get(names.Route, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Error fetching Route %s: %v", names.Route, err)
	}
	domain := route.Status.Domain

	endState := test.RetryingRouteInconsistency(pkgTest.MatchesAllOf(pkgTest.IsStatusOK, pkgTest.MatchesBody(helloWorldExpectedOutput)))
	if _, err := pkgTest.WaitForEndpointState(clients.KubeClient, t.Logf, domain, endState, "HelloWorldServesText", test.ServingFlags.ResolvableDomain); err != nil {
		t.Fatalf("The endpoint for Route %s at domain %s didn't serve the expected text \"%s\": %v", names.Route, domain, helloWorldExpectedOutput, err)
	}

	// Get Configuration's latest ready Revision's Build, and check that the Build was successful.
	t.Log("Revision is ready and serving, checking Build status.")
	config, err := clients.ServingClient.Configs.Get(names.Config, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get Configuration after it was seen to be live: %v", err)
	}
	rev, err := clients.ServingClient.Revisions.Get(config.Status.LatestReadyRevisionName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get latest Revision: %v", err)
	}
	names.Revision = rev.Name
	buildName := rev.Spec.BuildRef.Name
	t.Logf("Latest ready Revision is %q, its build is: %s", rev.Name, buildName)
	b, err := clients.BuildClient.TestBuilds.Get(buildName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get build for latest revision: %v", err)
	}
	if cond := b.Status.GetCondition(duckv1alpha1.ConditionSucceeded); cond == nil {
		t.Fatalf("Condition for build %q was nil", buildName)
	} else if cond.Status != corev1.ConditionTrue {
		t.Fatalf("Build %q was not successful", buildName)
	}

	// Update the Configuration with an environment variable, which should trigger a new revision
	// to be created, but without creating a new build.
	if err := updateConfigWithEnvVars(clients, names, []corev1.EnvVar{{
		Name:  "FOO",
		Value: "bar",
	}}); err != nil {
		t.Fatalf("Failed to update config with environment variables: %v", err)
	}

	nextRevName, err := getNextRevisionName(clients, names)
	if err != nil {
		t.Fatalf("Error waiting for next revision to be created: %v", err)
	}
	names.Revision = nextRevName

	nextRev, err := clients.ServingClient.Revisions.Get(nextRevName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get latest Revision: %v", err)
	}

	if diff := cmp.Diff(rev.Spec.BuildRef, nextRev.Spec.BuildRef); diff != "" {
		t.Fatalf("Unexpected differences in BuildRef: %v", diff)
	}

	// Update the Configuration's Build with an annotation, which should trigger the creation
	// of BOTH a Build and a Revision.
	if err := updateConfigWithBuildAnnotation(clients, names, map[string]string{
		"testing.knative.dev/foo": "bar",
	}); err != nil {
		t.Fatalf("Failed to update config with environment variables: %v", err)
	}

	nextRevName, err = getNextRevisionName(clients, names)
	if err != nil {
		t.Fatalf("Error waiting for next revision to be created: %v", err)
	}
	names.Revision = nextRevName

	nextRev, err = clients.ServingClient.Revisions.Get(nextRevName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get latest Revision: %v", err)
	}

	if diff := cmp.Diff(rev.Spec.BuildRef, nextRev.Spec.BuildRef); diff == "" {
		t.Fatalf("Got matching BuildRef, wanted different: %#v", rev.Spec.BuildRef)
	}
}

func TestBuildFailure(t *testing.T) {
	t.Parallel()
	clients := Setup(t)

	t.Log("Creating a new Configuration with failing build")
	names := test.ResourceNames{
		Config: test.ObjectNameForTest(t),
		Image:  "helloworld",
	}

	// Request a build that doesn't succeed.
	build := &v1alpha1.RawExtension{
		Object: &testbuildv1alpha1.Build{
			TypeMeta: metav1.TypeMeta{
				APIVersion: testbuildv1alpha1.SchemeGroupVersion.String(),
				Kind:       "Build",
			},
			Spec: testbuildv1alpha1.BuildSpec{
				Failure: &testbuildv1alpha1.FailureInfo{
					Message: "Test build has failed",
					Reason:  "InjectedFailure",
				},
			},
		},
	}

	config, err := clients.ServingClient.Configs.Create(test.ConfigurationWithBuild(names, build))
	if err != nil {
		t.Fatalf("Failed to create Configuration with failing build: %v", err)
	}

	test.CleanupOnInterrupt(func() { test.TearDown(clients, names) })
	defer test.TearDown(clients, names)

	// Wait for the Config have a LatestCreatedRevisionName
	if err := test.WaitForConfigurationState(clients.ServingClient, names.Config, test.ConfigurationHasCreatedRevision, "ConfigurationHasCreatedRevision"); err != nil {
		t.Fatalf("The Configuration %q does not have a LatestCreatedRevisionName: %v", names.Config, err)
	}
	// Get Configuration's latest Revision's Build, and check that the Build failed.
	config, err = clients.ServingClient.Configs.Get(config.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get Configuration after it was seen to be live: %v", err)
	}
	// Wait for the Revision to notice its Build failed.
	if err := test.WaitForRevisionState(clients.ServingClient, config.Status.LatestCreatedRevisionName, test.IsRevisionBuildFailed, "RevisionIsBuildFailed"); err != nil {
		t.Fatalf("The Revision %q was not marked as having a failed build: %v", names.Revision, err)
	}
	t.Logf("Revision %q is not ready because its build failed.", config.Status.LatestCreatedRevisionName)
	rev, err := clients.ServingClient.Revisions.Get(config.Status.LatestCreatedRevisionName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get latest Revision: %v", err)
	}
	buildName := rev.Spec.BuildRef.Name
	t.Logf("Latest created Revision is %q; it's build is: %q", rev.Name, buildName)
	b, err := clients.BuildClient.TestBuilds.Get(buildName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get build for latest revision: %v", err)
	}
	if cond := b.Status.GetCondition(duckv1alpha1.ConditionSucceeded); cond == nil {
		t.Fatalf("Condition for build %q was nil", buildName)
	} else if cond.Status != corev1.ConditionFalse {
		t.Fatalf("Build %q was not unsuccessful", buildName)
	}
}

func getNextRevisionName(clients *test.Clients, names test.ResourceNames) (string, error) {
	var newRevisionName string
	err := test.WaitForConfigurationState(clients.ServingClient, names.Config, func(c *v1alpha1.Configuration) (bool, error) {
		if c.Status.LatestCreatedRevisionName != names.Revision {
			newRevisionName = c.Status.LatestCreatedRevisionName
			return true, nil
		}
		return false, nil
	}, "ConfigurationUpdatedWithRevision")
	return newRevisionName, err
}

func updateConfigWithEnvVars(clients *test.Clients, names test.ResourceNames, ev []corev1.EnvVar) error {
	patches := []jsonpatch.JsonPatchOperation{{
		Operation: "add",
		Path:      "/spec/template/spec/containers/0/env",
		Value:     ev,
	}}
	patchBytes, err := json.Marshal(patches)
	if err != nil {
		return err
	}
	_, err = clients.ServingClient.Configs.Patch(names.Config, types.JSONPatchType, patchBytes, "")
	if err != nil {
		return err
	}
	return nil
}

func updateConfigWithBuildAnnotation(clients *test.Clients, names test.ResourceNames, ann map[string]string) error {
	patches := []jsonpatch.JsonPatchOperation{{
		Operation: "add",
		Path:      "/spec/build/metadata/annotations",
		Value:     ann,
	}}
	patchBytes, err := json.Marshal(patches)
	if err != nil {
		return err
	}
	_, err = clients.ServingClient.Configs.Patch(names.Config, types.JSONPatchType, patchBytes, "")
	if err != nil {
		return err
	}
	return nil
}

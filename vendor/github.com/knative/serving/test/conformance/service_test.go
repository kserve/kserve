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

package conformance

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	pkgTest "github.com/knative/pkg/test"
	"github.com/knative/serving/pkg/apis/serving"
	"github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"github.com/knative/serving/pkg/apis/serving/v1beta1"
	"github.com/knative/serving/test"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const userPort = int32(8081)

// Validates the state of Configuration, Revision, and Route objects for a runLatest Service. The checks in this method should be able to be performed at any point in a
// runLatest Service's lifecycle so long as the service is in a "Ready" state.
func validateRunLatestControlPlane(t *testing.T, clients *test.Clients, names test.ResourceNames, expectedGeneration string) error {
	t.Log("Checking to ensure Revision is in desired state with generation: ", expectedGeneration)
	err := test.CheckRevisionState(clients.ServingClient, names.Revision, func(r *v1alpha1.Revision) (bool, error) {
		if ready, err := test.IsRevisionReady(r); !ready {
			return false, fmt.Errorf("revision %s did not become ready to serve traffic: %v", names.Revision, err)
		}
		if r.Status.ImageDigest == "" {
			return false, fmt.Errorf("imageDigest not present for revision %s", names.Revision)
		}
		if validDigest, err := validateImageDigest(names.Image, r.Status.ImageDigest); !validDigest {
			return false, fmt.Errorf("imageDigest %s is not valid for imageName %s: %v", r.Status.ImageDigest, names.Image, err)
		}
		return true, nil
	})
	if err != nil {
		return err
	}
	err = test.CheckRevisionState(clients.ServingClient, names.Revision, test.IsRevisionAtExpectedGeneration(expectedGeneration))
	if err != nil {
		return fmt.Errorf("revision %s did not have an expected annotation with generation %s: %v", names.Revision, expectedGeneration, err)
	}

	t.Log("Checking to ensure Configuration is in desired state.")
	err = test.CheckConfigurationState(clients.ServingClient, names.Config, func(c *v1alpha1.Configuration) (bool, error) {
		if c.Status.LatestCreatedRevisionName != names.Revision {
			return false, fmt.Errorf("the Configuration %s was not updated indicating that the Revision %s was created: %v", names.Config, names.Revision, err)
		}
		if c.Status.LatestReadyRevisionName != names.Revision {
			return false, fmt.Errorf("the Configuration %s was not updated indicating that the Revision %s was ready: %v", names.Config, names.Revision, err)
		}
		return true, nil
	})
	if err != nil {
		return err
	}

	t.Log("Checking to ensure Route is in desired state with generation: ", expectedGeneration)
	err = test.CheckRouteState(clients.ServingClient, names.Route, test.AllRouteTrafficAtRevision(names))
	if err != nil {
		return fmt.Errorf("the Route %s was not updated to route traffic to the Revision %s: %v", names.Route, names.Revision, err)
	}

	return nil
}

// Validates service health and vended content match for a runLatest Service. The checks in this method should be able to be performed at any point in a
// runLatest Service's lifecycle so long as the service is in a "Ready" state.
func validateRunLatestDataPlane(t *testing.T, clients *test.Clients, names test.ResourceNames, expectedText string) error {
	t.Logf("Checking that the endpoint vends the expected text: %s", expectedText)
	_, err := pkgTest.WaitForEndpointState(
		clients.KubeClient,
		t.Logf,
		names.Domain,
		test.RetryingRouteInconsistency(pkgTest.MatchesAllOf(pkgTest.IsStatusOK, pkgTest.EventuallyMatchesBody(expectedText))),
		"WaitForEndpointToServeText",
		test.ServingFlags.ResolvableDomain)
	if err != nil {
		return fmt.Errorf("the endpoint for Route %s at domain %s didn't serve the expected text \"%s\": %v", names.Route, names.Domain, expectedText, err)
	}

	t.Log("TODO: The Service's Route is accessible from inside the cluster without external DNS")
	err = test.CheckServiceState(clients.ServingClient, names.Service, test.TODO_ServiceTrafficToRevisionWithInClusterDNS)
	if err != nil {
		return fmt.Errorf("the Service %s was not able to route traffic to the Revision %s with in cluster DNS: %v", names.Service, names.Revision, err)
	}

	return nil

}

// Validates labels on Revision, Configuration, and Route objects when created by a Service
// see spec here: https://github.com/knative/serving/blob/master/docs/spec/spec.md#revision
func validateLabelsPropagation(t *testing.T, objects test.ResourceObjects, names test.ResourceNames) error {
	t.Log("Validate Labels on Revision Object")
	revision := objects.Revision

	if revision.Labels["serving.knative.dev/configuration"] != names.Config {
		return fmt.Errorf("expect Confguration name in Revision label %q but got %q ", names.Config, revision.Labels["serving.knative.dev/configuration"])
	}
	if revision.Labels["serving.knative.dev/service"] != names.Service {
		return fmt.Errorf("expect Service name in Revision label %q but got %q ", names.Service, revision.Labels["serving.knative.dev/service"])
	}

	t.Log("Validate Labels on Configuration Object")
	config := objects.Config
	if config.Labels["serving.knative.dev/service"] != names.Service {
		return fmt.Errorf("expect Service name in Configuration label %q but got %q ", names.Service, config.Labels["serving.knative.dev/service"])
	}
	if config.Labels["serving.knative.dev/route"] != names.Route {
		return fmt.Errorf("expect Route name in Configuration label %q but got %q ", names.Route, config.Labels["serving.knative.dev/route"])
	}

	t.Log("Validate Labels on Route Object")
	route := objects.Route
	if route.Labels["serving.knative.dev/service"] != names.Service {
		return fmt.Errorf("expect Service name in Route label %q but got %q ", names.Service, route.Labels["serving.knative.dev/service"])
	}
	return nil
}

func validateAnnotations(objs *test.ResourceObjects) error {
	// This checks whether the annotations are set on the resources that
	// expect them to have.
	// List of issues listing annotations that we check: #1642.

	anns := objs.Service.GetAnnotations()
	for _, a := range []string{serving.CreatorAnnotation, serving.UpdaterAnnotation} {
		if got := anns[a]; got == "" {
			return fmt.Errorf("Expected %s annotation to be set, but was empty", a)
		}
	}
	return nil
}

func validateReleaseServiceShape(objs *test.ResourceObjects) error {
	// Check that Spec.Revisions is as expected.
	if got, want := objs.Service.Spec.Release.Revisions, []string{v1alpha1.ReleaseLatestRevisionKeyword}; !cmp.Equal(got, want) {
		return fmt.Errorf("Spec.Release.Revisions mismatch: diff: %s", cmp.Diff(got, want))
	}
	// Traffic should be routed to the lastest created revision.
	if got, want := objs.Service.Status.Traffic[0].RevisionName, objs.Config.Status.LatestReadyRevisionName; got != want {
		return fmt.Errorf("Status.Traffic[0].RevisionsName = %s, want: %s", got, want)
	}
	return nil
}

// TestRunLatestService tests both Creation and Update paths of a runLatest service. The test performs a series of Update/Validate steps to ensure that
// the service transitions as expected during each step.
// Currently the test performs the following updates:
// 1. Update Container Image
// 2. Update Metadata
//    a. Update Labels
//    b. Update Annotations
// 3. Update UserPort
func TestRunLatestService(t *testing.T) {
	t.Parallel()
	clients := setup(t)

	names := test.ResourceNames{
		Service: test.ObjectNameForTest(t),
		Image:   pizzaPlanet1,
	}

	// Clean up on test failure or interrupt
	defer test.TearDown(clients, names)
	test.CleanupOnInterrupt(func() { test.TearDown(clients, names) })

	// Setup initial Service
	objects, err := test.CreateRunLatestServiceReady(t, clients, &names, &test.Options{})
	if err != nil {
		t.Fatalf("Failed to create initial Service %v: %v", names.Service, err)
	}

	// Validate State after Creation

	if err = validateRunLatestControlPlane(t, clients, names, "1"); err != nil {
		t.Error(err)
	}

	if err = validateRunLatestDataPlane(t, clients, names, pizzaPlanetText1); err != nil {
		t.Error(err)
	}

	if err = validateLabelsPropagation(t, *objects, names); err != nil {
		t.Error(err)
	}

	if err := validateAnnotations(objects); err != nil {
		t.Errorf("Service annotations are incorrect: %v", err)
	}

	// We start a background prober to test if Route is always healthy even during Route update.
	prober := test.RunRouteProber(t, clients, names.Domain)
	defer test.AssertProberDefault(t, prober)

	// Update Container Image
	t.Log("Updating the Service to use a different image.")
	names.Image = printport
	image2 := pkgTest.ImagePath(names.Image)
	if _, err := test.PatchServiceImage(t, clients, objects.Service, image2); err != nil {
		t.Fatalf("Patch update for Service %s with new image %s failed: %v", names.Service, image2, err)
	}

	t.Log("Service should reflect new revision created and ready in status.")
	names.Revision, err = test.WaitForServiceLatestRevision(clients, names)
	if err != nil {
		t.Fatalf("New image not reflected in Service: %v", err)
	}

	t.Log("Waiting for Service to transition to Ready.")
	if err := test.WaitForServiceState(clients.ServingClient, names.Service, test.IsServiceReady, "ServiceIsReady"); err != nil {
		t.Fatalf("Error waiting for the service to become ready for the latest revision: %v", err)
	}

	// Validate State after Image Update
	if err = validateRunLatestControlPlane(t, clients, names, "2"); err != nil {
		t.Error(err)
	}
	if err = validateRunLatestDataPlane(t, clients, names, strconv.Itoa(v1alpha1.DefaultUserPort)); err != nil {
		t.Error(err)
	}

	// Update Metadata (Labels)
	t.Logf("Updating labels of the RevisionTemplateSpec for service %s.", names.Service)
	metadata := metav1.ObjectMeta{
		Labels: map[string]string{
			"labelX": "abc",
			"labelY": "def",
		},
	}
	if objects.Service, err = test.PatchServiceRevisionTemplateMetadata(t, clients, objects.Service, metadata); err != nil {
		t.Fatalf("Service %s was not updated with labels in its RevisionTemplateSpec: %v", names.Service, err)
	}

	t.Log("Waiting for the new revision to appear as LatestRevision.")
	if names.Revision, err = test.WaitForServiceLatestRevision(clients, names); err != nil {
		t.Fatalf("The Service %s was not updated with new revision %s after updating labels in its RevisionTemplateSpec: %v", names.Service, names.Revision, err)
	}

	// Update Metadata (Annotations)
	t.Logf("Updating annotations of RevisionTemplateSpec for service %s", names.Service)
	metadata = metav1.ObjectMeta{
		Annotations: map[string]string{
			"annotationA": "123",
			"annotationB": "456",
		},
	}
	if objects.Service, err = test.PatchServiceRevisionTemplateMetadata(t, clients, objects.Service, metadata); err != nil {
		t.Fatalf("Service %s was not updated with annotation in its RevisionTemplateSpec: %v", names.Service, err)
	}

	t.Log("Waiting for the new revision to appear as LatestRevision.")
	names.Revision, err = test.WaitForServiceLatestRevision(clients, names)
	if err != nil {
		t.Fatalf("The new revision has not become ready in Service: %v", err)
	}

	t.Log("Waiting for Service to transition to Ready.")
	if err := test.WaitForServiceState(clients.ServingClient, names.Service, test.IsServiceReady, "ServiceIsReady"); err != nil {
		t.Fatalf("Error waiting for the service to become ready for the latest revision: %v", err)
	}

	// Validate the Service shape.
	if err = validateRunLatestControlPlane(t, clients, names, "4"); err != nil {
		t.Error(err)
	}
	if err = validateRunLatestDataPlane(t, clients, names, strconv.Itoa(v1alpha1.DefaultUserPort)); err != nil {
		t.Error(err)
	}

	// Update container with user port.
	t.Logf("Updating the port of the user container for service %s to %d", names.Service, userPort)
	desiredSvc := objects.Service.DeepCopy()
	desiredSvc.Spec.RunLatest.Configuration.GetTemplate().Spec.GetContainer().Ports = []corev1.ContainerPort{{
		ContainerPort: userPort,
	}}
	if objects.Service, err = test.PatchService(t, clients, objects.Service, desiredSvc); err != nil {
		t.Fatalf("Service %s was not updated with a new port for the user container: %v", names.Service, err)
	}

	t.Log("Waiting for the new revision to appear as LatestRevision.")
	if names.Revision, err = test.WaitForServiceLatestRevision(clients, names); err != nil {
		t.Fatalf("The new revision has not become ready in Service: %v", err)
	}

	t.Log("Waiting for Service to transition to Ready.")
	if err := test.WaitForServiceState(clients.ServingClient, names.Service, test.IsServiceReady, "ServiceIsReady"); err != nil {
		t.Fatalf("Error waiting for the service to become ready for the latest revision: %v", err)
	}

	// Validate Service
	if err = validateRunLatestControlPlane(t, clients, names, "5"); err != nil {
		t.Error(err)
	}

	if err = validateRunLatestDataPlane(t, clients, names, strconv.Itoa(int(userPort))); err != nil {
		t.Error(err)
	}
}

func waitForDesiredTrafficShape(t *testing.T, sName string, want map[string]v1alpha1.TrafficTarget, clients *test.Clients) error {
	return test.WaitForServiceState(
		clients.ServingClient, sName, func(s *v1alpha1.Service) (bool, error) {
			// IsServiceReady never returns an error.
			if ok, _ := test.IsServiceReady(s); !ok {
				return false, nil
			}
			// Match the traffic shape.
			got := map[string]v1alpha1.TrafficTarget{}
			for _, tt := range s.Status.Traffic {
				got[tt.Name] = tt
			}
			ignoreURLs := cmpopts.IgnoreFields(v1alpha1.TrafficTarget{},
				"TrafficTarget.URL")
			if !cmp.Equal(got, want, ignoreURLs) {
				t.Logf("For service %s traffic shape mismatch: (-got, +want) %s",
					sName, cmp.Diff(got, want, ignoreURLs))
				return false, nil
			}
			return true, nil
		}, "Verify Service Traffic Shape",
	)
}

func TestRunLatestServiceBYOName(t *testing.T) {
	t.Parallel()
	clients := setup(t)

	names := test.ResourceNames{
		Service: test.ObjectNameForTest(t),
		Image:   pizzaPlanet1,
	}

	// Clean up on test failure or interrupt
	defer test.TearDown(clients, names)
	test.CleanupOnInterrupt(func() { test.TearDown(clients, names) })

	revName := names.Service + "-byoname"

	// Setup initial Service
	objects, err := test.CreateRunLatestServiceReady(t, clients, &names, &test.Options{}, func(svc *v1alpha1.Service) {
		svc.Spec.RunLatest.Configuration.GetTemplate().Name = revName
	})
	if err != nil {
		t.Fatalf("Failed to create initial Service %v: %v", names.Service, err)
	}
	if got, want := names.Revision, revName; got != want {
		t.Errorf("CreateRunLatestServiceReady() = %s, wanted %s", got, want)
	}

	// Validate State after Creation

	if err = validateRunLatestControlPlane(t, clients, names, "1"); err != nil {
		t.Error(err)
	}

	if err = validateRunLatestDataPlane(t, clients, names, pizzaPlanetText1); err != nil {
		t.Error(err)
	}

	if err = validateLabelsPropagation(t, *objects, names); err != nil {
		t.Error(err)
	}

	if err := validateAnnotations(objects); err != nil {
		t.Errorf("Service annotations are incorrect: %v", err)
	}

	// We start a background prober to test if Route is always healthy even during Route update.
	prober := test.RunRouteProber(t, clients, names.Domain)
	defer test.AssertProberDefault(t, prober)

	// Update Container Image
	t.Log("Updating the Service to use a different image.")
	names.Image = printport
	image2 := pkgTest.ImagePath(names.Image)
	if _, err := test.PatchServiceImage(t, clients, objects.Service, image2); err == nil {
		t.Fatalf("Patch update for Service %s didn't fail.", names.Service)
	}
}

// TestReleaseService creates a Service in `release` mode with the only revision
// being `@latest`. Once this succeeded, the test goes through Update/Validate to
// try different possible configurations for a release service.
// Currently tests for the following combinations:
// 1. One Revision Specified, current == latest
// 2. One Revision Specified, current != latset
// 3. Two Revisions Specified, 50% rollout,  candidate == latest
// 4. Two Revisions Specified, 50% rollout, candidate != latest
// 5. Two Revisions Specified, 50% rollout, candidate != latest, latest referred to as `@latest`.
func TestReleaseService(t *testing.T) {
	t.Parallel()
	// Create Initial Service
	clients := setup(t)
	releaseImagePath2 := pkgTest.ImagePath(pizzaPlanet2)
	releaseImagePath3 := pkgTest.ImagePath(helloworld)
	names := test.ResourceNames{
		Service: test.ObjectNameForTest(t),
		Image:   pizzaPlanet1,
	}
	defer test.TearDown(clients, names)
	test.CleanupOnInterrupt(func() { test.TearDown(clients, names) })

	// Expected Text for different revisions.
	const (
		expectedFirstRev  = pizzaPlanetText1
		expectedSecondRev = pizzaPlanetText2
		expectedThirdRev  = helloWorldText
	)

	objects, err := test.CreateReleaseServiceWithLatest(t, clients, &names, &test.Options{})
	if err != nil {
		t.Fatalf("Failed to create initial Service %v: %v", names.Service, err)
	}

	t.Log("Validating service shape.")
	if err := validateReleaseServiceShape(objects); err != nil {
		t.Fatalf("Release shape is incorrect: %v", err)
	}
	if err := validateAnnotations(objects); err != nil {
		t.Errorf("Service annotations are incorrect: %v", err)
	}
	revisions := []string{names.Revision}

	// 1. One Revision Specified, current == latest.
	t.Log("1. Updating Service to ReleaseType using lastCreatedRevision")
	objects.Service, err = test.PatchReleaseService(t, clients, objects.Service, revisions, 0)
	if err != nil {
		t.Fatalf("Service %s was not updated to release: %v", names.Service, err)
	}
	desiredTrafficShape := map[string]v1alpha1.TrafficTarget{
		v1alpha1.CurrentTrafficTarget: {
			Name: v1alpha1.CurrentTrafficTarget,
			TrafficTarget: v1beta1.TrafficTarget{
				RevisionName: objects.Config.Status.LatestReadyRevisionName,
				Percent:      100,
			},
		},
		v1alpha1.LatestTrafficTarget: {
			Name: v1alpha1.LatestTrafficTarget,
			TrafficTarget: v1beta1.TrafficTarget{
				RevisionName: objects.Config.Status.LatestReadyRevisionName,
			},
		},
	}
	t.Log("Waiting for Service to become ready with the new shape.")
	if err := waitForDesiredTrafficShape(t, names.Service, desiredTrafficShape, clients); err != nil {
		t.Fatal("Service never obtained expected shape")
	}

	t.Log("Service traffic should go to the first revision and be available on two names traffic targets: 'current' and 'latest'")
	if err := validateDomains(t, clients,
		names.Domain,
		[]string{expectedFirstRev},
		[]string{v1alpha1.LatestTrafficTarget, v1alpha1.CurrentTrafficTarget},
		[]string{expectedFirstRev, expectedFirstRev}); err != nil {
		t.Fatal(err)
	}

	// 2. One Revision Specified, current != latest.
	t.Log("2. Updating the Service Spec with a new image")
	if _, err := test.PatchServiceImage(t, clients, objects.Service, releaseImagePath2); err != nil {
		t.Fatalf("Patch update for Service %s with new image %s failed: %v", names.Service, releaseImagePath2, err)
	}

	t.Log("Since the Service was updated a new Revision will be created")
	if names.Revision, err = test.WaitForServiceLatestRevision(clients, names); err != nil {
		t.Fatalf("The Service %s was not updated with new revision %s: %v", names.Service, names.Revision, err)
	}
	revisions = append(revisions, names.Revision)

	// Also verify traffic is in the correct shape.
	desiredTrafficShape[v1alpha1.LatestTrafficTarget] = v1alpha1.TrafficTarget{
		Name: v1alpha1.LatestTrafficTarget,
		TrafficTarget: v1beta1.TrafficTarget{
			RevisionName: names.Revision,
		},
	}
	t.Log("Waiting for Service to become ready with the new shape.")
	if err := waitForDesiredTrafficShape(t, names.Service, desiredTrafficShape, clients); err != nil {
		t.Fatal("Service never obtained expected shape")
	}

	t.Log("Since the Service is using release the Route will not be updated, but new revision will be available at 'latest'")
	if err := validateDomains(t, clients,
		names.Domain,
		[]string{expectedFirstRev},
		[]string{v1alpha1.LatestTrafficTarget, v1alpha1.CurrentTrafficTarget},
		[]string{expectedSecondRev, expectedFirstRev}); err != nil {
		t.Fatal(err)
	}

	// 3. Two Revisions Specified, 50% rollout, candidate == latest.
	t.Log("3. Updating Service to split traffic between two revisions using Release mode")
	if objects.Service, err = test.PatchReleaseService(t, clients, objects.Service, revisions, 50); err != nil {
		t.Fatalf("Service %s was not updated to release: %v", names.Service, err)
	}

	desiredTrafficShape = map[string]v1alpha1.TrafficTarget{
		v1alpha1.CurrentTrafficTarget: {
			Name: v1alpha1.CurrentTrafficTarget,
			TrafficTarget: v1beta1.TrafficTarget{
				RevisionName: revisions[0],
				Percent:      50,
			},
		},
		v1alpha1.CandidateTrafficTarget: {
			Name: v1alpha1.CandidateTrafficTarget,
			TrafficTarget: v1beta1.TrafficTarget{
				RevisionName: revisions[1],
				Percent:      50,
			},
		},
		v1alpha1.LatestTrafficTarget: {
			Name: v1alpha1.LatestTrafficTarget,
			TrafficTarget: v1beta1.TrafficTarget{
				RevisionName: revisions[1],
			},
		},
	}
	t.Log("Waiting for Service to become ready with the new shape.")
	if err := waitForDesiredTrafficShape(t, names.Service, desiredTrafficShape, clients); err != nil {
		t.Fatal("Service never obtained expected shape")
	}

	t.Log("Traffic should be split between the two revisions and available on three named traffic targets, 'current', 'candidate', and 'latest'")
	if err := validateDomains(t, clients,
		names.Domain,
		[]string{expectedFirstRev, expectedSecondRev},
		[]string{v1alpha1.CandidateTrafficTarget, v1alpha1.LatestTrafficTarget, v1alpha1.CurrentTrafficTarget},
		[]string{expectedSecondRev, expectedSecondRev, expectedFirstRev}); err != nil {
		t.Fatal(err)
	}

	// 4. Two Revisions Specified, 50% rollout, candidate != latest.
	t.Log("4. Updating the Service Spec with a new image")
	if _, err := test.PatchServiceImage(t, clients, objects.Service, releaseImagePath3); err != nil {
		t.Fatalf("Patch update for Service %s with new image %s failed: %v", names.Service, releaseImagePath3, err)
	}
	t.Log("Since the Service was updated a new Revision will be created")
	if names.Revision, err = test.WaitForServiceLatestRevision(clients, names); err != nil {
		t.Fatalf("The Service %s was not updated with new revision %s: %v", names.Service, names.Revision, err)
	}

	desiredTrafficShape[v1alpha1.LatestTrafficTarget] = v1alpha1.TrafficTarget{
		Name: v1alpha1.LatestTrafficTarget,
		TrafficTarget: v1beta1.TrafficTarget{
			RevisionName: names.Revision,
		},
	}
	t.Log("Waiting for Service to become ready with the new shape.")
	if err := waitForDesiredTrafficShape(t, names.Service, desiredTrafficShape, clients); err != nil {
		t.Fatal("Service never obtained expected shape")
	}

	t.Log("Traffic should remain between the two images, and the new revision should be available on the named traffic target 'latest'")
	if err := validateDomains(t, clients,
		names.Domain,
		[]string{expectedFirstRev, expectedSecondRev},
		[]string{v1alpha1.LatestTrafficTarget, v1alpha1.CandidateTrafficTarget, v1alpha1.CurrentTrafficTarget},
		[]string{expectedThirdRev, expectedSecondRev, expectedFirstRev}); err != nil {
		t.Fatal(err)
	}

	// Now update the service to use `@latest` as candidate.
	revisions[1] = v1alpha1.ReleaseLatestRevisionKeyword
	t.Log("5. Updating Service to split traffic between two `current` and `@latest`")
	if objects.Service, err = test.PatchReleaseService(t, clients, objects.Service, revisions, 50); err != nil {
		t.Fatalf("Service %s was not updated to release: %v", names.Service, err)
	}
	// Verify in the end it's still the case.
	if err := validateAnnotations(objects); err != nil {
		t.Errorf("Service annotations are incorrect: %v", err)
	}

	// `candidate` now points to the latest.
	desiredTrafficShape[v1alpha1.CandidateTrafficTarget] = v1alpha1.TrafficTarget{
		Name: v1alpha1.CandidateTrafficTarget,
		TrafficTarget: v1beta1.TrafficTarget{
			RevisionName: names.Revision,
			Percent:      50,
		},
	}
	t.Log("Waiting for Service to become ready with the new shape.")
	if err := waitForDesiredTrafficShape(t, names.Service, desiredTrafficShape, clients); err != nil {
		t.Fatal("Service never obtained expected shape")
	}

	if err := validateDomains(t, clients,
		names.Domain,
		[]string{expectedFirstRev, expectedThirdRev},
		[]string{v1alpha1.LatestTrafficTarget, v1alpha1.CandidateTrafficTarget, v1alpha1.CurrentTrafficTarget},
		[]string{expectedThirdRev, expectedThirdRev, expectedFirstRev}); err != nil {
		t.Fatal(err)
	}
}

// TODO(jonjohnsonjr): Examples of deploying from source.

// +build e2e

/*
Copyright 2019 The Knative Authors
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
	"testing"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/knative/pkg/apis/duck"
	duckv1alpha1 "github.com/knative/pkg/apis/duck/v1alpha1"
	pkgTest "github.com/knative/pkg/test"
	"github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"github.com/knative/serving/test"
)

func TestPipeline(t *testing.T) {
	t.Parallel()
	pipelineName := test.ObjectNameForTest(t)

	testCases := []struct {
		name         string
		rawExtension *v1alpha1.RawExtension
		preFn        func(*testing.T, *test.Clients)
		validateFn   func(*testing.T, string, *test.Clients)
	}{{
		name: "task run",
		rawExtension: &v1alpha1.RawExtension{
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "pipeline.knative.dev/v1alpha1",
					"kind":       "TaskRun",
					"metadata": map[string]interface{}{
						"namespace": test.ServingNamespace,
					},
					"spec": map[string]interface{}{
						"trigger": map[string]interface{}{
							"type": "manual",
						},
						"taskSpec": map[string]interface{}{
							"steps": []corev1.Container{{
								Name:  "foo",
								Image: "busybox",
								Args:  []string{"echo", "hellow"},
							}},
						},
					},
				},
			},
		},
		validateFn: func(t *testing.T, buildName string, clients *test.Clients) {
			taskRunClient := clients.Dynamic.Resource(schema.GroupVersionResource{
				Group:    "pipeline.knative.dev",
				Version:  "v1alpha1",
				Resource: "taskruns",
			})
			t.Logf("Revision's Build is taskrun %q", buildName)
			u, err := taskRunClient.Namespace(test.ServingNamespace).Get(buildName, metav1.GetOptions{})
			if err != nil {
				t.Fatalf("Failed to get build for latest revision: %v", err)
			}

			b := &duckv1alpha1.KResource{}
			duck.FromUnstructured(u, b)
			if cond := b.Status.GetCondition(duckv1alpha1.ConditionSucceeded); cond == nil {
				t.Fatalf("Condition for build %q was nil", buildName)
			} else if cond.Status != corev1.ConditionTrue {
				t.Fatalf("Build %q was not successful", buildName)
			}
		},
	}, {
		name: "pipeline run",
		rawExtension: &v1alpha1.RawExtension{
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "pipeline.knative.dev/v1alpha1",
					"kind":       "PipelineRun",
					"metadata": map[string]interface{}{
						"namespace": test.ServingNamespace,
					},
					"spec": map[string]interface{}{
						"trigger": map[string]interface{}{
							"type": "manual",
						},
						"pipelineRef": map[string]interface{}{
							"name": pipelineName,
						},
					},
				},
			},
		},
		preFn: func(t *testing.T, clients *test.Clients) {
			pipelineClient := clients.Dynamic.Resource(schema.GroupVersionResource{
				Group:    "pipeline.knative.dev",
				Version:  "v1alpha1",
				Resource: "pipelines",
			})
			taskClient := clients.Dynamic.Resource(schema.GroupVersionResource{
				Group:    "pipeline.knative.dev",
				Version:  "v1alpha1",
				Resource: "tasks",
			})
			t.Log("Creating Pipeline and Task for the build with PipelineRun")
			taskName := test.ObjectNameForTest(t)

			if _, err := taskClient.Namespace(test.ServingNamespace).Create(&unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "pipeline.knative.dev/v1alpha1",
					"kind":       "Task",
					"metadata": map[string]interface{}{
						"name":      taskName,
						"namespace": test.ServingNamespace,
					},
					"spec": map[string]interface{}{
						"steps": []corev1.Container{{
							Name:  "foo",
							Image: "busybox",
							Args:  []string{"echo", "hellow"},
						}},
					},
				},
			}, metav1.CreateOptions{}); err != nil {
				t.Fatalf("Failed to create Task: %v", err)
			}
			if _, err := pipelineClient.Namespace(test.ServingNamespace).Create(&unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "pipeline.knative.dev/v1alpha1",
					"kind":       "Pipeline",
					"metadata": map[string]interface{}{
						"name":      pipelineName,
						"namespace": test.ServingNamespace,
					},
					"spec": map[string]interface{}{
						"tasks": []map[string]interface{}{{
							"name": "test-pipe-test-task",
							"taskRef": map[string]interface{}{
								"name": taskName,
							},
						}},
					},
				},
			}, metav1.CreateOptions{}); err != nil {
				t.Fatalf("Failed to create Pipeline: %v", err)
			}
		},
		validateFn: func(t *testing.T, buildName string, clients *test.Clients) {
			pipelineRunClient := clients.Dynamic.Resource(schema.GroupVersionResource{
				Group:    "pipeline.knative.dev",
				Version:  "v1alpha1",
				Resource: "pipelineruns",
			})
			t.Logf("Revision's Build is pipelinerun %q", buildName)
			u, err := pipelineRunClient.Namespace(test.ServingNamespace).Get(buildName, metav1.GetOptions{})
			if err != nil {
				t.Fatalf("Failed to get build for latest revision: %v", err)
			}

			b := &duckv1alpha1.KResource{}
			duck.FromUnstructured(u, b)
			if cond := b.Status.GetCondition(duckv1alpha1.ConditionSucceeded); cond == nil {
				t.Fatalf("Condition for build %q was nil", buildName)
			} else if cond.Status != corev1.ConditionTrue {
				t.Fatalf("Build %q was not successful", buildName)
			}
		},
	}}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			clients := Setup(t)

			t.Log("Creating a new Route and Configuration with build")
			svcName := test.ObjectNameForTest(t)
			names := test.ResourceNames{
				Config: svcName,
				Route:  svcName,
				Image:  "helloworld",
			}

			if tc.preFn != nil {
				tc.preFn(t, clients)
			}

			test.CleanupOnInterrupt(func() { test.TearDown(clients, names) })
			defer test.TearDown(clients, names)

			if _, err := clients.ServingClient.Configs.Create(test.ConfigurationWithBuild(names, tc.rawExtension)); err != nil {
				t.Fatalf("Failed to create Configuration: %v", err)
			}
			if _, err := clients.ServingClient.Routes.Create(test.Route(names)); err != nil {
				t.Fatalf("Failed to create Route: %v", err)
			}

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
			if tc.validateFn != nil {
				t.Logf("Latest ready Revision is %q", rev.Name)
				buildName := rev.Spec.BuildRef.Name
				tc.validateFn(t, buildName, clients)
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
		})
	}
}

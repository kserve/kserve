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
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/knative/build/pkg/apis/build/v1alpha1"
	"github.com/knative/pkg/apis"
	duckv1alpha1 "github.com/knative/pkg/apis/duck/v1alpha1"
	"github.com/knative/pkg/test"
	"go.opencensus.io/trace"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var ignoreVolatileTime = cmp.Comparer(func(_, _ apis.VolatileTime) bool {
	return true
})

// TestSimpleBuild tests that a simple build that does nothing interesting
// succeeds.
func TestSimpleBuild(t *testing.T) {
	buildTestNamespace, clients := initialize(t)

	// Emit a metric for null-build latency (i.e., time to schedule and execute
	// and finish watching a build).
	_, span := trace.StartSpan(context.Background(), "NullBuildLatency")
	defer span.End()

	buildName := "simple-build"

	test.CleanupOnInterrupt(func() { teardownBuild(t, clients, buildTestNamespace, buildName) }, t.Logf)
	defer teardownBuild(t, clients, buildTestNamespace, buildName)
	defer teardownNamespace(t, clients, buildTestNamespace)

	if _, err := clients.buildClient.builds.Create(&v1alpha1.Build{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: buildTestNamespace,
			Name:      buildName,
		},
		Spec: v1alpha1.BuildSpec{
			Timeout: &metav1.Duration{Duration: 120 * time.Second},
			Steps: []corev1.Container{{
				Image: "busybox",
				Args:  []string{"echo", "simple"},
			}},
		},
	}); err != nil {
		t.Fatalf("Error creating build: %v", err)
	}

	if _, err := clients.buildClient.watchBuild(buildName); err != nil {
		t.Fatalf("Error watching build: %v", err)
	}
}

// TestFailingBuild tests that a simple build that fails, fails as expected.
func TestFailingBuild(t *testing.T) {
	buildTestNamespace, clients := initialize(t)

	buildName := "failing-build"

	test.CleanupOnInterrupt(func() { teardownBuild(t, clients, buildTestNamespace, buildName) }, t.Logf)
	defer teardownBuild(t, clients, buildTestNamespace, buildName)
	defer teardownNamespace(t, clients, buildTestNamespace)

	if _, err := clients.buildClient.builds.Create(&v1alpha1.Build{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: buildTestNamespace,
			Name:      buildName,
		},
		Spec: v1alpha1.BuildSpec{
			Steps: []corev1.Container{{
				Image: "busybox",
				Args:  []string{"false"}, // fails.
			}},
		},
	}); err != nil {
		t.Fatalf("Error creating build: %v", err)
	}

	if _, err := clients.buildClient.watchBuild(buildName); err == nil || err == errWatchTimeout {
		t.Fatalf("watchBuild did not return expected error: %v", err)
	}
}

func TestBuildWithTemplate(t *testing.T) {
	buildTestNamespace, clients := initialize(t)

	buildName := "build-with-template"
	templateName := "simple-template"

	test.CleanupOnInterrupt(func() { teardownBuild(t, clients, buildTestNamespace, buildName) }, t.Logf)
	defer teardownBuild(t, clients, buildTestNamespace, buildName)
	defer teardownNamespace(t, clients, buildTestNamespace)

	if _, err := clients.buildClient.buildTemplates.Create(&v1alpha1.BuildTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: buildTestNamespace,
			Name:      templateName,
		},
		Spec: v1alpha1.BuildTemplateSpec{
			Steps: []corev1.Container{{
				Image:   "ubuntu:latest",
				Command: []string{"/bin/bash"},
				Args:    []string{"-c", "echo some stuff > /im/a/custom/mount/path/file"},
				VolumeMounts: []corev1.VolumeMount{{
					Name:      "custom",
					MountPath: "/im/a/custom/mount/path",
				}},
			}},
			Volumes: []corev1.Volume{{
				Name:         "custom",
				VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
			}},
		},
	}); err != nil {
		t.Fatalf("Error creating build template: %v", err)
	}

	if _, err := clients.buildClient.builds.Create(&v1alpha1.Build{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: buildTestNamespace,
			Name:      buildName,
		},
		Spec: v1alpha1.BuildSpec{
			Template: &v1alpha1.TemplateInstantiationSpec{
				Name: templateName,
				Kind: v1alpha1.BuildTemplateKind,
			},
		},
	}); err != nil {
		t.Fatalf("Error creating build: %v", err)
	}

	if _, err := clients.buildClient.watchBuild(buildName); err != nil {
		t.Fatalf("watchBuild returned unexpected error: %s", err.Error())
	}
}

func TestBuildWithClusterTemplate(t *testing.T) {
	buildTestNamespace, clients := initialize(t)

	buildName := "build-with-template"
	templateName := "cluster-template"

	test.CleanupOnInterrupt(func() { teardownBuild(t, clients, buildTestNamespace, buildName) }, t.Logf)
	defer teardownBuild(t, clients, buildTestNamespace, buildName)
	defer teardownClusterTemplate(t, clients, templateName)
	defer teardownNamespace(t, clients, buildTestNamespace)

	if _, err := clients.buildClient.clusterTemplates.Create(&v1alpha1.ClusterBuildTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name: templateName,
		},
		Spec: v1alpha1.BuildTemplateSpec{
			Steps: []corev1.Container{{
				Image:   "ubuntu:latest",
				Command: []string{"/bin/bash"},
				Args:    []string{"-c", "echo some stuff > /im/a/custom/mount/path/file"},
				VolumeMounts: []corev1.VolumeMount{{
					Name:      "custom",
					MountPath: "/im/a/custom/mount/path",
				}},
			}},
			Volumes: []corev1.Volume{{
				Name:         "custom",
				VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
			}},
		},
	}); err != nil {
		t.Fatalf("Error creating cluster build template: %v", err)
	}

	if _, err := clients.buildClient.builds.Create(&v1alpha1.Build{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: buildTestNamespace,
			Name:      buildName,
		},
		Spec: v1alpha1.BuildSpec{
			Template: &v1alpha1.TemplateInstantiationSpec{
				Name: templateName,
				Kind: v1alpha1.ClusterBuildTemplateKind,
			},
		},
	}); err != nil {
		t.Fatalf("Error creating build: %v", err)
	}

	if _, err := clients.buildClient.watchBuild(buildName); err != nil {
		t.Fatalf("watchBuild returned unexpected error: %s", err.Error())
	}
}

func TestBuildLowTimeout(t *testing.T) {
	buildTestNamespace, clients := initialize(t)

	// TODO(jasonhall): builds created in quick succession, e.g., by `go
	// test --count=N`, can sometimes produce confusing behavior around
	// statuses, as the Build controller sees updates for a previous
	// iteration of the identically-named build. Generate a unique name
	// here to avoid this behavior. We need a better general solution.
	buildName := fmt.Sprintf("build-low-timeout-%d", time.Now().Unix())

	test.CleanupOnInterrupt(func() { teardownBuild(t, clients, buildTestNamespace, buildName) }, t.Logf)
	defer teardownBuild(t, clients, buildTestNamespace, buildName)
	defer teardownNamespace(t, clients, buildTestNamespace)

	buildTimeout := 10 * time.Second

	if _, err := clients.buildClient.builds.Create(&v1alpha1.Build{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: buildTestNamespace,
			Name:      buildName,
		},
		Spec: v1alpha1.BuildSpec{
			Timeout: &metav1.Duration{Duration: buildTimeout},
			Steps: []corev1.Container{{
				Image:   "ubuntu",
				Command: []string{"/bin/bash"},
				Args:    []string{"-c", "sleep 2000"},
			}},
		},
	}); err != nil {
		t.Fatalf("Error creating build: %v", err)
	}

	b, err := clients.buildClient.watchBuild(buildName)
	if err != errBuildFailed {
		t.Fatalf("watchBuild got %v, want %v (build status: %+v)", err, errBuildFailed, b.Status)
	}

	if d := cmp.Diff(b.Status.GetCondition(duckv1alpha1.ConditionSucceeded), &duckv1alpha1.Condition{
		Type:    duckv1alpha1.ConditionSucceeded,
		Status:  corev1.ConditionFalse,
		Reason:  "BuildTimeout",
		Message: fmt.Sprintf("Build %q failed to finish within %q", b.Name, buildTimeout),
	}, ignoreVolatileTime); d != "" {
		t.Errorf("Unexpected build status %#v", b.Status)
	}

	if b.Status.CompletionTime == nil || b.Status.StartTime == nil {
		t.Fatalf("missing start time (%v) or completion time (%v)", b.Status.StartTime, b.Status.CompletionTime)
	}

	buildDuration := b.Status.CompletionTime.Time.Sub(b.Status.StartTime.Time).Seconds()
	higherEnd := buildTimeout + 30*time.Second + 10*time.Second // build timeout + 30 sec poll time + 10 sec

	if !(buildDuration >= buildTimeout.Seconds() && buildDuration < higherEnd.Seconds()) {
		t.Fatalf("Expected the build duration to be within range %.2fs to %.2fs; but got build duration: %f, start time: %q and completed time: %q \n",
			buildTimeout.Seconds(),
			higherEnd.Seconds(),
			buildDuration,
			b.Status.StartTime.Time,
			b.Status.CompletionTime.Time,
		)
	}
}

// TestPendingBuild tests that a build with non existent node selector will remain in pending
// state until watch timeout.
func TestPendingBuild(t *testing.T) {
	buildTestNamespace, clients := initialize(t)

	buildName := "pending-build"

	test.CleanupOnInterrupt(func() { teardownBuild(t, clients, buildTestNamespace, buildName) }, t.Logf)
	defer teardownBuild(t, clients, buildTestNamespace, buildName)
	defer teardownNamespace(t, clients, buildTestNamespace)

	if _, err := clients.buildClient.builds.Create(&v1alpha1.Build{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: buildTestNamespace,
			Name:      buildName,
		},
		Spec: v1alpha1.BuildSpec{
			NodeSelector: map[string]string{"disk": "fake-ssd"},
			Steps: []corev1.Container{{
				Image: "busybox",
				Args:  []string{"false"}, // fails.
			}},
		},
	}); err != nil {
		t.Fatalf("Error creating build: %v", err)
	}

	if _, err := clients.buildClient.watchBuild(buildName); err == nil || err != errWatchTimeout {
		t.Fatalf("watchBuild did not return expected `watch timeout` error")
	}
}

// TestPodAffinity tests that a build with non existent pod affinity does not scheduled
// and fails after watch timeout
func TestPodAffinity(t *testing.T) {
	buildTestNamespace, clients := initialize(t)

	buildName := "affinity-build"

	test.CleanupOnInterrupt(func() { teardownBuild(t, clients, buildTestNamespace, buildName) }, t.Logf)
	defer teardownBuild(t, clients, buildTestNamespace, buildName)
	defer teardownNamespace(t, clients, buildTestNamespace)

	if _, err := clients.buildClient.builds.Create(&v1alpha1.Build{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: buildTestNamespace,
			Name:      buildName,
		},
		Spec: v1alpha1.BuildSpec{
			Affinity: &corev1.Affinity{
				NodeAffinity: &corev1.NodeAffinity{
					// This node affinity rule says the pod can only be placed on a node with a label whose key is kubernetes.io/e2e-az-name
					// and whose value is either e2e-az1 or e2e-az2. Test cluster does not have any nodes that meets this constraint so the build
					// will wait for pod to scheduled until timeout.
					RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{{
							MatchExpressions: []corev1.NodeSelectorRequirement{{
								Key:      "kubernetes.io/e2e-az-name",
								Operator: corev1.NodeSelectorOpIn,
								Values:   []string{"e2e-az1", "e2e-az2"},
							}},
						}},
					},
				},
			},
			Steps: []corev1.Container{{
				Image: "busybox",
				Args:  []string{"true"},
			}},
		},
	}); err != nil {
		t.Fatalf("Error creating build: %v", err)
	}

	if _, err := clients.buildClient.watchBuild(buildName); err == nil || err != errWatchTimeout {
		t.Fatalf("watchBuild did not return expected `watch timeout` error")
	}
}

// TestPersistentVolumeClaim tests that two builds that specify a volume backed
// by the same persist volume claim can use it to pass build information
// between builds.
func TestPersistentVolumeClaim(t *testing.T) {
	buildTestNamespace, clients := initialize(t)

	buildName := "persistent-volume-claim"

	test.CleanupOnInterrupt(func() { teardownBuild(t, clients, buildTestNamespace, buildName) }, t.Logf)
	defer teardownBuild(t, clients, buildTestNamespace, buildName)
	defer teardownNamespace(t, clients, buildTestNamespace)

	// First, create the PVC.
	if _, err := clients.kubeClient.Kube.CoreV1().PersistentVolumeClaims(buildTestNamespace).Create(&corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: buildTestNamespace,
			Name:      "cache-claim",
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: *resource.NewQuantity(1024, resource.BinarySI), // 1Ki
				},
			},
		},
	}); err != nil {
		t.Fatalf("Error creating PVC: %v", err)
	}
	t.Logf("Created PVC")

	// Then, run a build that populates the PVC.
	firstBuild := "cache-populate"
	if _, err := clients.buildClient.builds.Create(&v1alpha1.Build{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: buildTestNamespace,
			Name:      firstBuild,
		},
		Spec: v1alpha1.BuildSpec{
			Steps: []corev1.Container{{
				Image:   "ubuntu",
				Command: []string{"bash"},
				Args:    []string{"-c", "echo foo > /cache/foo"},
				VolumeMounts: []corev1.VolumeMount{{
					Name:      "cache",
					MountPath: "/cache",
				}},
			}},
			Volumes: []corev1.Volume{{
				Name: "cache",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: "cache-claim",
					},
				},
			}},
		},
	}); err != nil {
		t.Fatalf("Error creating first build: %v", err)
	}
	t.Logf("Created first build")
	if _, err := clients.buildClient.watchBuild(firstBuild); err != nil {
		t.Fatalf("Error watching build: %v", err)
	}
	t.Logf("First build finished successfully")

	// Then, run a build that reads from the PVC.
	secondBuild := "cache-retrieve"
	if _, err := clients.buildClient.builds.Create(&v1alpha1.Build{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: buildTestNamespace,
			Name:      secondBuild,
		},
		Spec: v1alpha1.BuildSpec{
			Steps: []corev1.Container{{
				Image: "ubuntu",
				Args:  []string{"cat", "/cache/foo"},
				VolumeMounts: []corev1.VolumeMount{{
					Name:      "cache",
					MountPath: "/cache",
				}},
			}},
			Volumes: []corev1.Volume{{
				Name: "cache",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: "cache-claim",
					},
				},
			}},
		},
	}); err != nil {
		t.Fatalf("Error creating second build: %v", err)
	}
	t.Logf("Created second build")
	if _, err := clients.buildClient.watchBuild(secondBuild); err != nil {
		t.Fatalf("Error watching build: %v", err)
	}
	t.Logf("Second build finished successfully")
}

// TestBuildWithSources tests that a build can have multiple similar sources
// under different names
func TestBuildWithSources(t *testing.T) {
	buildTestNamespace, clients := initialize(t)

	buildName := "build-sources"

	test.CleanupOnInterrupt(func() { teardownBuild(t, clients, buildTestNamespace, buildName) }, t.Logf)
	defer teardownBuild(t, clients, buildTestNamespace, buildName)
	defer teardownNamespace(t, clients, buildTestNamespace)

	if _, err := clients.buildClient.builds.Create(&v1alpha1.Build{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: buildTestNamespace,
			Name:      buildName,
		},
		Spec: v1alpha1.BuildSpec{
			Sources: []v1alpha1.SourceSpec{{
				Name:       "bazel",
				TargetPath: "bazel",
				Git: &v1alpha1.GitSourceSpec{
					Url:      "https://github.com/bazelbuild/rules_docker",
					Revision: "master",
				},
			}, {
				Name:       "rocks",
				TargetPath: "rocks",
				Git: &v1alpha1.GitSourceSpec{
					Url:      "https://github.com/bazelbuild/rules_docker",
					Revision: "master",
				},
			}},
			Steps: []corev1.Container{{
				Name:    "compare",
				Image:   "ubuntu",
				Command: []string{"bash"},
				// compare contents between
				Args: []string{
					"-c",
					"cmp --silent bazel/WORKSPACE rocks/WORKSPACE && echo '### SUCCESS: Files Are Identical! ###'",
				},
			}},
		},
	}); err != nil {
		t.Fatalf("Error creating build: %v", err)
	}

	if _, err := clients.buildClient.watchBuild(buildName); err != nil {
		t.Fatalf("Error watching build: %v", err)
	}
}

// TestSimpleBuildWithHybridSources tests hybrid input sources can be accessed in all steps
func TestSimpleBuildWithHybridSources(t *testing.T) {
	buildTestNamespace, clients := initialize(t)

	buildName := "hybrid-sources"

	test.CleanupOnInterrupt(func() { teardownBuild(t, clients, buildTestNamespace, buildName) }, t.Logf)
	defer teardownBuild(t, clients, buildTestNamespace, buildName)
	defer teardownNamespace(t, clients, buildTestNamespace)

	if _, err := clients.buildClient.builds.Create(&v1alpha1.Build{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: buildTestNamespace,
			Name:      buildName,
		},
		Spec: v1alpha1.BuildSpec{
			Sources: []v1alpha1.SourceSpec{{
				Name:       "git-bazel",
				TargetPath: "bazel",
				Git: &v1alpha1.GitSourceSpec{
					Url:      "https://github.com/bazelbuild/rules_docker",
					Revision: "master",
				},
			}, {
				Name: "rocks",
				Custom: &corev1.Container{
					Image: "gcr.io/cloud-builders/git:latest",
					Args: []string{
						"clone",
						"https://github.com/bazelbuild/rules_docker.git",
						"somewhere",
					},
				},
			}, {
				Name:       "gcs-rules",
				TargetPath: "gcs",
				GCS: &v1alpha1.GCSSourceSpec{
					Type:     "Archive",
					Location: "gs://build-crd-tests/rules_docker-master.zip",
				},
			}},
			Steps: []corev1.Container{{
				Name:    "check-git-custom",
				Image:   "ubuntu",
				Command: []string{"bash"},
				// compare contents between custom and git
				Args: []string{
					"-c",
					"cmp --silent bazel/WORKSPACE /workspace/somewhere/WORKSPACE && echo '### SUCCESS: Files Are Identical! ###'",
				},
			}, {
				Name:    "checkgitgcs",
				Image:   "ubuntu",
				Command: []string{"bash"},
				// compare contents between gcs and git
				Args: []string{
					"-c",
					"cmp --silent bazel/WORKSPACE gcs/WORKSPACE || echo '### SUCCESS: Files Are Not Identical! ###'",
				},
			}},
		},
	}); err != nil {
		t.Fatalf("Error creating build: %v", err)
	}

	if _, err := clients.buildClient.watchBuild(buildName); err != nil {
		t.Fatalf("Error watching build: %v", err)
	}
}

func TestFailedBuildWithParamsInVolume(t *testing.T) {
	buildTestNamespace, clients := initialize(t)

	buildName := "build-cm-not-exist"
	templateName := "simple-template-with-params-in-volume"

	test.CleanupOnInterrupt(func() { teardownBuild(t, clients, buildTestNamespace, buildName) }, t.Logf)
	defer teardownBuild(t, clients, buildTestNamespace, buildName)
	defer teardownNamespace(t, clients, buildTestNamespace)

	if _, err := clients.buildClient.buildTemplates.Create(&v1alpha1.BuildTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: buildTestNamespace,
			Name:      templateName,
		},
		Spec: v1alpha1.BuildTemplateSpec{
			Steps: []corev1.Container{{
				Image:   "ubuntu:latest",
				Command: []string{"/bin/bash"},
				Args:    []string{"-c", "echo hello"},
			}},
			Volumes: []corev1.Volume{{
				Name: "custom",
				VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: "${cmname}"},
				}},
			}},
			Parameters: []v1alpha1.ParameterSpec{{
				Name: "cmname",
			}},
		},
	}); err != nil {
		t.Fatalf("Error creating build template: %v", err)
	}

	if _, err := clients.buildClient.builds.Create(&v1alpha1.Build{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: buildTestNamespace,
			Name:      buildName,
		},
		Spec: v1alpha1.BuildSpec{
			Template: &v1alpha1.TemplateInstantiationSpec{
				Name: templateName,
				Kind: v1alpha1.BuildTemplateKind,
				Arguments: []v1alpha1.ArgumentSpec{{
					Name:  "cmname",
					Value: "cm-not-exist",
				}},
			},
			Timeout: &metav1.Duration{5 * time.Minute},
		},
	}); err != nil {
		t.Fatalf("Error creating build: %v", err)
	}

	if _, err := clients.buildClient.watchBuild(buildName); err == nil || err != errWatchTimeout {
		t.Fatalf("watchBuild did not return expected error: %v", err)
	}
}

// TestDuplicatePodBuild creates 10 builds and checks that each of them has only one build pod.
func TestDuplicatePodBuild(t *testing.T) {
	buildTestNamespace, clients := initialize(t)
	defer teardownNamespace(t, clients, buildTestNamespace)

	for i := 0; i < 10; i++ {
		buildName := fmt.Sprintf("duplicate-pod-build-%d", i)
		test.CleanupOnInterrupt(func() { teardownBuild(t, clients, buildTestNamespace, buildName) }, t.Logf)
		defer teardownBuild(t, clients, buildTestNamespace, buildName)

		t.Logf("Creating build %q", buildName)
		if _, err := clients.buildClient.builds.Create(&v1alpha1.Build{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: buildTestNamespace,
				Name:      buildName,
			},
			Spec: v1alpha1.BuildSpec{
				Timeout: &metav1.Duration{Duration: 120 * time.Second},
				Steps: []corev1.Container{{
					Image: "busybox",
					Args:  []string{"echo", "simple"},
				}},
			},
		}); err != nil {
			t.Fatalf("Error creating build: %v", err)
		}
		if _, err := clients.buildClient.watchBuild(buildName); err != nil {
			t.Fatalf("Error watching build: %v", err)
		}

		pods, err := clients.kubeClient.Kube.CoreV1().Pods(buildTestNamespace).List(metav1.ListOptions{
			LabelSelector: fmt.Sprintf("build.knative.dev/buildName=%s", buildName),
		})
		if err != nil {
			t.Fatalf("Error getting build pod list: %v", err)
		}
		if n := len(pods.Items); n != 1 {
			t.Fatalf("Error matching the number of build pods: expecting 1 pod, got %d", n)
		}
	}
}

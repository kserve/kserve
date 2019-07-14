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
	"errors"
	"flag"
	"fmt"
	"testing"

	"github.com/knative/build/pkg/apis/build/v1alpha1"
	buildversioned "github.com/knative/build/pkg/client/clientset/versioned"
	buildtyped "github.com/knative/build/pkg/client/clientset/versioned/typed/build/v1alpha1"
	"github.com/knative/pkg/test"
	"github.com/knative/pkg/test/logging"
	corev1 "k8s.io/api/core/v1"
	kuberrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/tools/clientcmd"
)

type clients struct {
	kubeClient  *test.KubeClient
	buildClient *buildClient
}

var (
	// Sentinel error from watchBuild when the build failed.
	errBuildFailed = errors.New("build failed")

	// Sentinal error from watchBuild when watch timed out before build finished
	errWatchTimeout = errors.New("watch ended before build finished")
)

func teardownNamespace(t *testing.T, clients *clients, buildTestNamespace string) {
	if clients != nil && clients.kubeClient != nil {
		t.Logf("Deleting namespace %q", buildTestNamespace)

		if err := clients.kubeClient.Kube.CoreV1().Namespaces().Delete(buildTestNamespace, &metav1.DeleteOptions{}); err != nil && !kuberrors.IsNotFound(err) {
			t.Fatalf("Error deleting namespace %q: %v", buildTestNamespace, err)
		}
	}
}

func teardownBuild(t *testing.T, clients *clients, buildTestNamespace, name string) {
	if clients != nil && clients.buildClient != nil {
		t.Logf("Deleting build %q in namespace %q", name, buildTestNamespace)

		if err := clients.buildClient.builds.Delete(name, &metav1.DeleteOptions{}); err != nil && !kuberrors.IsNotFound(err) {
			t.Fatalf("Error deleting build %q: %v", name, err)
		}
	}
}

func teardownClusterTemplate(t *testing.T, clients *clients, name string) {
	if clients != nil && clients.buildClient != nil {
		t.Logf("Deleting cluster template %q", name)

		if err := clients.buildClient.clusterTemplates.Delete(name, &metav1.DeleteOptions{}); err != nil && !kuberrors.IsNotFound(err) {
			t.Fatalf("Error deleting cluster template %q: %v", name, err)
		}
	}
}

func buildClients(t *testing.T, buildTestNamespace string) *clients {
	clients, err := newClients(test.Flags.Kubeconfig, test.Flags.Cluster, buildTestNamespace)
	if err != nil {
		t.Fatalf("Error creating newClients: %v", err)
	}
	return clients
}

func createTestNamespace(t *testing.T) (string, *clients) {
	buildTestNamespace := AppendRandomString("build-tests")
	clients := buildClients(t, buildTestNamespace)

	// Ensure the test namespace exists, by trying to create it and ignoring
	// already-exists errors.
	if _, err := clients.kubeClient.Kube.CoreV1().Namespaces().Create(&corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: buildTestNamespace,
		},
	}); err == nil {
		t.Logf("Created namespace %q", buildTestNamespace)
	} else if kuberrors.IsAlreadyExists(err) {
		t.Logf("Namespace %q already exists", buildTestNamespace)
	} else {
		t.Fatalf("Error creating namespace %q: %v", buildTestNamespace, err)
	}
	return buildTestNamespace, clients
}

func verifyDefaultServiceAccountCreation(namespace string, t *testing.T, c *clients) {
	defaultSA := "default"
	t.Logf("Verify SA %q is created in namespace %q", defaultSA, namespace)
	if err := wait.PollImmediate(interval, timeout, func() (bool, error) {
		_, err := c.kubeClient.Kube.CoreV1().ServiceAccounts(namespace).Get(defaultSA, metav1.GetOptions{})
		if err != nil && kuberrors.IsNotFound(err) {
			return false, nil
		}
		return true, err
	}); err != nil {
		t.Fatalf("Failed to get SA %q in namespace %q for tests: %s", defaultSA, namespace, err)
	}
}

func newClients(configPath string, clusterName string, namespace string) (*clients, error) {
	overrides := clientcmd.ConfigOverrides{}
	// Override the cluster name if provided.
	if clusterName != "" {
		overrides.Context.Cluster = clusterName
	}
	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(&clientcmd.ClientConfigLoadingRules{
		ExplicitPath: configPath,
	}, &overrides).ClientConfig()
	if err != nil {
		return nil, err
	}

	kubeClient, err := test.NewKubeClient(configPath, clusterName)
	if err != nil {
		return nil, err
	}

	bcs, err := buildversioned.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	buildClient := &buildClient{
		builds:           bcs.BuildV1alpha1().Builds(namespace),
		buildTemplates:   bcs.BuildV1alpha1().BuildTemplates(namespace),
		clusterTemplates: bcs.BuildV1alpha1().ClusterBuildTemplates(),
	}

	return &clients{
		kubeClient:  kubeClient,
		buildClient: buildClient,
	}, nil
}

type buildClient struct {
	builds           buildtyped.BuildInterface
	buildTemplates   buildtyped.BuildTemplateInterface
	clusterTemplates buildtyped.ClusterBuildTemplateInterface
}

func (c *buildClient) watchBuild(name string) (*v1alpha1.Build, error) {
	ls := metav1.SingleObject(metav1.ObjectMeta{Name: name})
	// TODO: Update watchBuild function to take this as parameter depending on test requirements

	// Any build that takes longer than this timeout will result in
	// errWatchTimeout.
	var timeout int64 = 120
	ls.TimeoutSeconds = &timeout

	w, err := c.builds.Watch(ls)
	if err != nil {
		return nil, err
	}
	var latest *v1alpha1.Build
	for evt := range w.ResultChan() {
		switch evt.Type {
		case watch.Deleted:
			return nil, errors.New("build deleted")
		case watch.Error:
			return nil, fmt.Errorf("error event: %v", evt.Object)
		}

		b, ok := evt.Object.(*v1alpha1.Build)
		if !ok {
			return nil, fmt.Errorf("object was not a Build: %v", err)
		}
		latest = b

		for _, cond := range b.Status.Conditions {
			if cond.Type == v1alpha1.BuildSucceeded {
				switch cond.Status {
				case corev1.ConditionTrue:
					return b, nil
				case corev1.ConditionFalse:
					return b, errBuildFailed
				case corev1.ConditionUnknown:
					continue
				}
			}
		}
	}
	return latest, errWatchTimeout
}

// initialize is responsible for setting up and tearing down the testing environment,
// namely the test namespace.
func initialize(t *testing.T) (string, *clients) {
	flag.Parse()
	logging.InitializeLogger(test.Flags.LogVerbose)
	flag.Set("alsologtostderr", "true")
	if test.Flags.EmitMetrics {
		logging.InitializeMetricExporter(t.Name())
	}

	buildTestNamespace, clients := createTestNamespace(t)
	verifyDefaultServiceAccountCreation(buildTestNamespace, t, clients)
	// Cleanup namespace
	test.CleanupOnInterrupt(func() { teardownNamespace(t, clients, buildTestNamespace) }, t.Logf)

	return buildTestNamespace, buildClients(t, buildTestNamespace)
}

package e2e

import (
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	// Mysteriously required to support GCP auth (required by k8s libs).
	// Apparently just importing it is enough. @_@ side effects @_@.
	// https://github.com/kubernetes/client-go/issues/242
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	pkgTest "github.com/knative/pkg/test"
	"github.com/knative/serving/pkg/autoscaler"
	"github.com/knative/serving/test"
)

const (
	pizzaPlanet1 = "pizzaplanetv1"
	pizzaPlanet2 = "pizzaplanetv2"

	pizzaPlanetText1         = "What a spaceport!"
	pizzaPlanetText2         = "Re-energize yourself with a slice of pepperoni!"
	helloWorldExpectedOutput = "Hello World! How about some tasty noodles?"
)

// Setup creates the client objects needed in the e2e tests.
func Setup(t *testing.T) *test.Clients {
	return SetupWithNamespace(t, test.ServingNamespace)
}

// SetupWithNamespace creates the client objects needed in the e2e tests under the specified namespace.
func SetupWithNamespace(t *testing.T, namespace string) *test.Clients {
	clients, err := test.NewClients(
		pkgTest.Flags.Kubeconfig,
		pkgTest.Flags.Cluster,
		namespace)
	if err != nil {
		t.Fatalf("Couldn't initialize clients: %v", err)
	}
	return clients
}

// CreateRouteAndConfig will create Route and Config objects using clients.
// The Config object will serve requests to a container started from the image at imagePath.
func CreateRouteAndConfig(t *testing.T, clients *test.Clients, image string, options *test.Options) (test.ResourceNames, error) {
	svcName := test.ObjectNameForTest(t)
	names := test.ResourceNames{
		Config: svcName,
		Route:  svcName,
		Image:  image,
	}

	if _, err := test.CreateConfiguration(t, clients, names, options); err != nil {
		return test.ResourceNames{}, err
	}
	_, err := test.CreateRoute(t, clients, names)
	return names, err
}

// WaitForScaleToZero will wait for the specified deployment to scale to 0 replicas.
// Will wait up to 6 times the configured ScaleToZeroGracePeriod before failing.
func WaitForScaleToZero(t *testing.T, deploymentName string, clients *test.Clients) error {
	t.Helper()

	t.Logf("Waiting for %q to scale to zero", deploymentName)

	return pkgTest.WaitForDeploymentState(
		clients.KubeClient,
		deploymentName,
		func(d *appsv1.Deployment) (bool, error) {
			t.Logf("Deployment %q has %d replicas", deploymentName, d.Status.ReadyReplicas)
			return d.Status.ReadyReplicas == 0, nil
		},
		"DeploymentIsScaledDown",
		test.ServingNamespace,
		scaleToZeroGracePeriod(t, clients.KubeClient)*6,
	)
}

func scaleToZeroGracePeriod(t *testing.T, client *pkgTest.KubeClient) time.Duration {
	t.Helper()

	autoscalerCM, err := client.Kube.CoreV1().ConfigMaps("knative-serving").Get("config-autoscaler", metav1.GetOptions{})
	if err != nil {
		t.Logf("Failed to Get autoscaler configmap = %v, falling back to DefaultScaleToZeroGracePeriod", err)
		return autoscaler.DefaultScaleToZeroGracePeriod
	}

	config, err := autoscaler.NewConfigFromConfigMap(autoscalerCM)
	if err != nil {
		t.Log("Failed to build autoscaler config, falling back to DefaultScaleToZeroGracePeriod")
		return autoscaler.DefaultScaleToZeroGracePeriod
	}

	return config.ScaleToZeroGracePeriod
}

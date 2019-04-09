package knservice

import (
	knservingv1alpha1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"os"
	"path/filepath"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"testing"
)

var cfg *rest.Config
var c client.Client

func TestMain(m *testing.M) {
	t := &envtest.Environment{
		CRDDirectoryPaths: []string{filepath.Join("..", "..", "..", "config", "crds")},
	}

	err := v1alpha1.SchemeBuilder.AddToScheme(scheme.Scheme)

	if err != nil {
		log.Error(err, "Failed to add kfserving scheme")
	}

	err = knservingv1alpha1.SchemeBuilder.AddToScheme(scheme.Scheme)

	if err != nil {
		log.Error(err, "Failed to add knative serving scheme")
	}

	if cfg, err = t.Start(); err != nil {
		log.Error(err, "Failed to start testing panel")
	}

	if c, err = client.New(cfg, client.Options{Scheme: scheme.Scheme}); err != nil {
		log.Error(err, "Failed to start client")
	}
	code := m.Run()
	t.Stop()
	os.Exit(code)
}

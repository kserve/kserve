package ingress

import (
	"net/http"
	"os"
	"strings"
	"testing"

	istioclientv1beta1 "istio.io/client-go/pkg/apis/networking/v1beta1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
)

// mockTransport is a mock HTTP transport used for ingress probing.
type mockTransport struct{}

var (
	probeNotReadyHostPrefix = "not-ready-isvc"
	fakeClient              client.Client
)

func (t *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if strings.Contains(req.Host, probeNotReadyHostPrefix) {
		return &http.Response{
			StatusCode: http.StatusNotFound,
		}, nil
	} else {
		return &http.Response{
			StatusCode: http.StatusOK,
		}, nil
	}
}

func TestMain(m *testing.M) {
	err := v1beta1.AddToScheme(scheme.Scheme)
	if err != nil {
		panic(err)
	}
	if err := istioclientv1beta1.SchemeBuilder.AddToScheme(scheme.Scheme); err != nil {
		log.Error(err, "Failed to add istio scheme")
	}
	fakeClient = fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	//	Mock transport for ingress probing
	Transport = &mockTransport{}

	os.Exit(m.Run())
}

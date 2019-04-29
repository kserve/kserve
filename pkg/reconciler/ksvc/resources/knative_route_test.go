package resources

import (
	"github.com/google/go-cmp/cmp"
	knservingv1alpha1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"github.com/knative/serving/pkg/apis/serving/v1beta1"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func TestKnativeRoute(t *testing.T) {
	scenarios := map[string]struct {
		kfService     *v1alpha1.KFService
		expectedRoute *knservingv1alpha1.Route
		shouldFail    bool
	}{
		"RunLatestModel": {
			kfService: &v1alpha1.KFService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mnist",
					Namespace: "default",
				},
				Spec: v1alpha1.KFServiceSpec{
					MinReplicas: 1,
					MaxReplicas: 3,
					Default: v1alpha1.ModelSpec{
						Tensorflow: &v1alpha1.TensorflowSpec{
							ModelURI:       "s3://test/mnist/export",
							RuntimeVersion: "1.13",
						},
					},
				},
			},
			expectedRoute: &knservingv1alpha1.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mnist",
					Namespace: "default",
				},
				Spec: knservingv1alpha1.RouteSpec{
					Traffic: []knservingv1alpha1.TrafficTarget{
						{
							TrafficTarget: v1beta1.TrafficTarget{
								ConfigurationName: "mnist-default",
								Percent:           100,
							},
						},
					},
				},
			},
		},
		"RunCanaryModel": {
			kfService: &v1alpha1.KFService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mnist",
					Namespace: "default",
				},
				Spec: v1alpha1.KFServiceSpec{
					MinReplicas: 1,
					MaxReplicas: 3,
					Default: v1alpha1.ModelSpec{
						Tensorflow: &v1alpha1.TensorflowSpec{
							ModelURI:       "s3://test/mnist/export",
							RuntimeVersion: "1.13",
						},
					},
					Canary: &v1alpha1.CanarySpec{
						TrafficPercent: 20,
						ModelSpec: v1alpha1.ModelSpec{
							Tensorflow: &v1alpha1.TensorflowSpec{
								ModelURI:       "s3://test/mnist-2/export",
								RuntimeVersion: "1.13",
							},
						},
					},
				},
				Status: v1alpha1.KFServiceStatus{
					Default: v1alpha1.StatusConfigurationSpec{
						Name: "v1",
					},
				},
			},
			expectedRoute: &knservingv1alpha1.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mnist",
					Namespace: "default",
				},
				Spec: knservingv1alpha1.RouteSpec{
					Traffic: []knservingv1alpha1.TrafficTarget{
						{
							TrafficTarget: v1beta1.TrafficTarget{
								ConfigurationName: "mnist-default",
								Percent:           80,
							},
						},
						{
							TrafficTarget: v1beta1.TrafficTarget{
								ConfigurationName: "mnist-canary",
								Percent:           20,
							},
						},
					},
				},
			},
		},
	}

	for name, scenario := range scenarios {
		route := CreateKnativeRoute(scenario.kfService)
		// Validate
		if scenario.shouldFail {
			t.Errorf("Test %q failed: returned success but expected error", name)
		} else {
			if diff := cmp.Diff(scenario.expectedRoute, route); diff != "" {
				t.Errorf("Test %q unexpected default configuration (-want +got): %v", name, diff)
			}
		}

	}
}

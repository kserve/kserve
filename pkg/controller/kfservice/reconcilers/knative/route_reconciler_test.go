package knative

import (
	"context"
	"github.com/google/go-cmp/cmp"
	knservingv1alpha1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"github.com/knative/serving/pkg/apis/serving/v1beta1"
	"github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func TestKnativeRouteReconcile(t *testing.T) {
	existingRoute := &knservingv1alpha1.Route{
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
	}
	g := gomega.NewGomegaWithT(t)
	scenarios := map[string]struct {
		desiredRoute *knservingv1alpha1.Route
		update       bool
		shouldFail   bool
	}{
		"Reconcile new model serving": {
			update: false,
			desiredRoute: &knservingv1alpha1.Route{
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
		"Reconcile route update": {
			update: true,
			desiredRoute: &knservingv1alpha1.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mnist",
					Namespace: "default",
					Labels: map[string]string{
						"serving.knative.dev/route": "dream",
					},
					Annotations: map[string]string{
						"cherub": "rock",
					},
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

	routeReconciler := NewRouteReconciler(c)
	for name, scenario := range scenarios {
		if scenario.update {
			g.Expect(c.Create(context.TODO(), existingRoute)).NotTo(gomega.HaveOccurred())
		}
		route, err := routeReconciler.Reconcile(context.TODO(), scenario.desiredRoute)
		// Validate
		if scenario.shouldFail && err == nil {
			t.Errorf("Test %q failed: returned success but expected error", name)
		}
		if !scenario.shouldFail {
			if err != nil {
				t.Errorf("Test %q failed: returned error: %v", name, err)
			}
			if diff := cmp.Diff(scenario.desiredRoute.Spec, route.Spec); diff != "" {
				t.Errorf("Test %q unexpected route spec (-want +got): %v", name, diff)
			}
			if diff := cmp.Diff(scenario.desiredRoute.ObjectMeta.Labels, route.ObjectMeta.Labels); diff != "" {
				t.Errorf("Test %q unexpected route labels (-want +got): %v", name, diff)
			}
			if diff := cmp.Diff(scenario.desiredRoute.ObjectMeta.Annotations, route.ObjectMeta.Annotations); diff != "" {
				t.Errorf("Test %q unexpected route annotations (-want +got): %v", name, diff)
			}
		}
		g.Expect(c.Delete(context.TODO(), existingRoute)).NotTo(gomega.HaveOccurred())
	}
}

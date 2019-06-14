package knative

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	knservingv1alpha1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"github.com/knative/serving/pkg/apis/serving/v1beta1"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha1"
	testutils "github.com/kubeflow/kfserving/pkg/testing"
	"github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/manager"
)

func TestKnativeRouteReconcile(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	mgr, err := manager.New(cfg, manager.Options{})
	stopMgr, mgrStopped := testutils.StartTestManager(mgr, g)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	c := mgr.GetClient()

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	routeReconciler := NewRouteReconciler(c, mgr.GetScheme())
	scenarios := map[string]struct {
		kfsvc        v1alpha1.KFService
		desiredRoute knservingv1alpha1.Route
	}{
		"Reconcile new model serving": {
			kfsvc: v1alpha1.KFService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mnist",
					Namespace: "default",
				},
				Spec: v1alpha1.KFServiceSpec{
					Default: v1alpha1.ModelSpec{
						Tensorflow: &v1alpha1.TensorflowSpec{
							RuntimeVersion: v1alpha1.DefaultTensorflowRuntimeVersion,
							ModelURI:       "gs://testuri",
						},
					},
				},
			},
			desiredRoute: knservingv1alpha1.Route{
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
	}

	for name, scenario := range scenarios {
		t.Logf("Scenario: %s", name)
		g.Expect(c.Create(context.TODO(), &scenario.kfsvc)).NotTo(gomega.HaveOccurred())
		g.Expect(c.Create(context.TODO(), &knservingv1alpha1.Route{ObjectMeta: scenario.desiredRoute.ObjectMeta})).NotTo(gomega.HaveOccurred())

		if err := routeReconciler.Reconcile(&scenario.kfsvc); err != nil {
			t.Errorf("Test %q failed: returned error: %v", name, err)
		}

		// Assert default
		actualRoute := knservingv1alpha1.Route{}
		g.Eventually(func() error {
			return c.Get(context.TODO(), types.NamespacedName{
				Name:      scenario.kfsvc.Name,
				Namespace: scenario.kfsvc.Namespace,
			}, &actualRoute)
		}, timeout).Should(gomega.Succeed())

		if err != nil {
			t.Errorf("Test %q failed: returned error: %v", name, err)
		}
		if diff := cmp.Diff(scenario.desiredRoute.Spec, actualRoute.Spec); diff != "" {
			t.Errorf("Test %q unexpected route spec (-want +got): %v", name, diff)
		}
		if diff := cmp.Diff(scenario.desiredRoute.ObjectMeta.Labels, actualRoute.ObjectMeta.Labels); diff != "" {
			t.Errorf("Test %q unexpected route labels (-want +got): %v", name, diff)
		}
		if diff := cmp.Diff(scenario.desiredRoute.ObjectMeta.Annotations, actualRoute.ObjectMeta.Annotations); diff != "" {
			t.Errorf("Test %q unexpected route annotations (-want +got): %v", name, diff)
		}
		g.Expect(c.Delete(context.TODO(), &scenario.kfsvc)).NotTo(gomega.HaveOccurred())
		g.Expect(c.Delete(context.TODO(), &knservingv1alpha1.Route{ObjectMeta: scenario.desiredRoute.ObjectMeta})).NotTo(gomega.HaveOccurred())
	}
}

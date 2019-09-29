/*
Copyright 2019 kubeflow.org.

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

package knative

import (
	"context"
	"fmt"
	"github.com/kubeflow/kfserving/pkg/constants"
	"k8s.io/api/core/v1"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/kubeflow/kfserving/pkg/apis/serving/v1alpha2"
	testutils "github.com/kubeflow/kfserving/pkg/testing"
	"github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	knativeserving "knative.dev/serving/pkg/apis/serving/v1beta1"

	"sigs.k8s.io/controller-runtime/pkg/client"
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
	latestRevision := true
	routeReconciler := NewRouteReconciler(c, mgr.GetScheme())
	scenarios := map[string]struct {
		kfsvc        v1alpha2.KFService
		desiredRoute *knativeserving.Route
	}{
		"Reconcile new model serving": {
			kfsvc: v1alpha2.KFService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mnist",
					Namespace: "default",
				},
				Spec: v1alpha2.KFServiceSpec{
					Default: v1alpha2.EndpointSpec{
						Predictor: v1alpha2.PredictorSpec{
							Tensorflow: &v1alpha2.TensorflowSpec{
								RuntimeVersion: v1alpha2.DefaultTensorflowRuntimeVersion,
								StorageURI:     "gs://testuri",
							},
						},
					},
				},
			},
			desiredRoute: &knativeserving.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.PredictRouteName("mnist"),
					Namespace: "default",
				},
				Spec: knativeserving.RouteSpec{
					Traffic: []knativeserving.TrafficTarget{
						{
							ConfigurationName: constants.DefaultPredictorServiceName("mnist"),
							LatestRevision:    &latestRevision,
							Percent:           100,
						},
					},
				},
			},
		},
		"Reconcile route with transformer": {
			kfsvc: v1alpha2.KFService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mnist",
					Namespace: "default",
				},
				Spec: v1alpha2.KFServiceSpec{
					Default: v1alpha2.EndpointSpec{
						Transformer: &v1alpha2.TransformerSpec{
							Custom: &v1alpha2.CustomSpec{
								Container: v1.Container{
									Image: "transformer:v1",
								},
							},
						},
						Predictor: v1alpha2.PredictorSpec{
							Tensorflow: &v1alpha2.TensorflowSpec{
								RuntimeVersion: v1alpha2.DefaultTensorflowRuntimeVersion,
								StorageURI:     "gs://testuri",
							},
						},
					},
				},
			},
			desiredRoute: &knativeserving.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.PredictRouteName("mnist"),
					Namespace: "default",
				},
				Spec: knativeserving.RouteSpec{
					Traffic: []knativeserving.TrafficTarget{
						{
							ConfigurationName: constants.DefaultTransformerServiceName("mnist"),
							LatestRevision:    &latestRevision,
							Percent:           100,
						},
					},
				},
			},
		},
		"Reconcile transformer route with canary": {
			kfsvc: v1alpha2.KFService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mnist",
					Namespace: "default",
				},
				Spec: v1alpha2.KFServiceSpec{
					Default: v1alpha2.EndpointSpec{
						Transformer: &v1alpha2.TransformerSpec{
							Custom: &v1alpha2.CustomSpec{
								Container: v1.Container{
									Image: "transformer:v1",
								},
							},
						},
						Predictor: v1alpha2.PredictorSpec{
							Tensorflow: &v1alpha2.TensorflowSpec{
								RuntimeVersion: v1alpha2.DefaultTensorflowRuntimeVersion,
								StorageURI:     "gs://testuri",
							},
						},
					},
					Canary: &v1alpha2.EndpointSpec{
						Transformer: &v1alpha2.TransformerSpec{
							Custom: &v1alpha2.CustomSpec{
								Container: v1.Container{
									Image: "transformer:v2",
								},
							},
						},
						Predictor: v1alpha2.PredictorSpec{
							Tensorflow: &v1alpha2.TensorflowSpec{
								RuntimeVersion: v1alpha2.DefaultTensorflowRuntimeVersion,
								StorageURI:     "gs://testuri",
							},
						},
					},
					CanaryTrafficPercent: 20,
				},
			},
			desiredRoute: &knativeserving.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.PredictRouteName("mnist"),
					Namespace: "default",
				},
				Spec: knativeserving.RouteSpec{
					Traffic: []knativeserving.TrafficTarget{
						{
							ConfigurationName: constants.DefaultTransformerServiceName("mnist"),
							LatestRevision:    &latestRevision,
							Percent:           80,
						},
						{
							ConfigurationName: constants.CanaryTransformerServiceName("mnist"),
							LatestRevision:    &latestRevision,
							Percent:           20,
						},
					},
				},
			},
		},
	}

	for name, scenario := range scenarios {
		t.Logf("Scenario: %s", name)
		g.Expect(c.Create(context.TODO(), &scenario.kfsvc)).NotTo(gomega.HaveOccurred())

		if err := routeReconciler.Reconcile(&scenario.kfsvc); err != nil {
			t.Errorf("Test %q failed: returned error: %v", name, err)
		}

		// Assert default

		g.Eventually(func() error { return awaitDesiredRoute(c, scenario.desiredRoute) }, timeout).Should(gomega.Succeed())

		g.Expect(c.Delete(context.TODO(), &scenario.kfsvc)).NotTo(gomega.HaveOccurred())
	}
}

func awaitDesiredRoute(c client.Client, desired *knativeserving.Route) error {
	actual := knativeserving.Route{}
	if err := c.Get(context.TODO(), types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, &actual); err != nil {
		return err
	}
	if diff := cmp.Diff(desired.Spec, actual.Spec); diff != "" {
		return fmt.Errorf("Unexpected route spec (-want +got): %v", diff)
	}
	if diff := cmp.Diff(desired.ObjectMeta.Labels, actual.ObjectMeta.Labels); diff != "" {
		return fmt.Errorf("Unexpected route labels (-want +got): %v", diff)
	}
	if diff := cmp.Diff(desired.ObjectMeta.Annotations, actual.ObjectMeta.Annotations); diff != "" {
		return fmt.Errorf("Unexpected route annotations (-want +got): %v", diff)
	}
	return nil
}

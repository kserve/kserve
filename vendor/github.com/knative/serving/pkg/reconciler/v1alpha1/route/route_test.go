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

package route

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/knative/pkg/apis"
	duckv1beta1 "github.com/knative/pkg/apis/duck/v1beta1"
	"github.com/knative/pkg/configmap"
	ctrl "github.com/knative/pkg/controller"
	logtesting "github.com/knative/pkg/logging/testing"
	"github.com/knative/pkg/system"
	netv1alpha1 "github.com/knative/serving/pkg/apis/networking/v1alpha1"
	"github.com/knative/serving/pkg/apis/serving"
	"github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"github.com/knative/serving/pkg/apis/serving/v1beta1"
	fakeclientset "github.com/knative/serving/pkg/client/clientset/versioned/fake"
	informers "github.com/knative/serving/pkg/client/informers/externalversions"
	"github.com/knative/serving/pkg/gc"
	"github.com/knative/serving/pkg/network"
	rclr "github.com/knative/serving/pkg/reconciler"
	"github.com/knative/serving/pkg/reconciler/v1alpha1/route/config"
	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	kubeinformers "k8s.io/client-go/informers"
	fakekubeclientset "k8s.io/client-go/kubernetes/fake"

	. "github.com/knative/pkg/reconciler/testing"
)

const (
	testNamespace       = "test"
	defaultDomainSuffix = "test-domain.dev"
	prodDomainSuffix    = "prod-domain.com"
)

func getTestRouteWithTrafficTargets(traffic []v1alpha1.TrafficTarget) *v1alpha1.Route {
	return &v1alpha1.Route{
		ObjectMeta: metav1.ObjectMeta{
			SelfLink:  "/apis/serving/v1alpha1/namespaces/test/Routes/test-route",
			Name:      "test-route",
			Namespace: testNamespace,
			Labels: map[string]string{
				"route": "test-route",
			},
		},
		Spec: v1alpha1.RouteSpec{
			Traffic: traffic,
		},
	}
}

func getTestRevision(name string) *v1alpha1.Revision {
	return getTestRevisionWithCondition(name, apis.Condition{
		Type:   v1alpha1.RevisionConditionReady,
		Status: corev1.ConditionTrue,
		Reason: "ServiceReady",
	})
}

func getTestRevisionWithCondition(name string, cond apis.Condition) *v1alpha1.Revision {
	return &v1alpha1.Revision{
		ObjectMeta: metav1.ObjectMeta{
			SelfLink:  fmt.Sprintf("/apis/serving/v1alpha1/namespaces/test/revisions/%s", name),
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: v1alpha1.RevisionSpec{
			Container: &corev1.Container{
				Image: "test-image",
			},
		},
		Status: v1alpha1.RevisionStatus{
			ServiceName: fmt.Sprintf("%s-pub", name),
			Status: duckv1beta1.Status{
				Conditions: duckv1beta1.Conditions{cond},
			},
		},
	}
}

func getTestConfiguration() *v1alpha1.Configuration {
	return &v1alpha1.Configuration{
		ObjectMeta: metav1.ObjectMeta{
			SelfLink:  "/apis/serving/v1alpha1/namespaces/test/revisiontemplates/test-config",
			Name:      "test-config",
			Namespace: testNamespace,
		},
		Spec: v1alpha1.ConfigurationSpec{
			// This is a workaround for generation initialization
			DeprecatedGeneration: 1,
			RevisionTemplate: &v1alpha1.RevisionTemplateSpec{
				Spec: v1alpha1.RevisionSpec{
					Container: &corev1.Container{
						Image: "test-image",
					},
				},
			},
		},
	}
}

func getTestRevisionForConfig(config *v1alpha1.Configuration) *v1alpha1.Revision {
	rev := &v1alpha1.Revision{
		ObjectMeta: metav1.ObjectMeta{
			SelfLink:  "/apis/serving/v1alpha1/namespaces/test/revisions/p-deadbeef",
			Name:      "p-deadbeef",
			Namespace: testNamespace,
			Labels: map[string]string{
				serving.ConfigurationLabelKey: config.Name,
			},
		},
		Spec: *config.Spec.GetTemplate().Spec.DeepCopy(),
		Status: v1alpha1.RevisionStatus{
			ServiceName: "p-deadbeef-pub",
		},
	}
	rev.Status.MarkResourcesAvailable()
	rev.Status.MarkContainerHealthy()
	return rev
}

func newTestReconciler(t *testing.T, configs ...*corev1.ConfigMap) (
	kubeClient *fakekubeclientset.Clientset,
	servingClient *fakeclientset.Clientset,
	reconciler *Reconciler,
	kubeInformer kubeinformers.SharedInformerFactory,
	servingInformer informers.SharedInformerFactory,
	configMapWatcher *configmap.ManualWatcher) {
	kubeClient, servingClient, _, reconciler, kubeInformer, servingInformer, configMapWatcher = newTestSetup(t)
	return
}

func newTestSetup(t *testing.T, configs ...*corev1.ConfigMap) (
	kubeClient *fakekubeclientset.Clientset,
	servingClient *fakeclientset.Clientset,
	controller *ctrl.Impl,
	reconciler *Reconciler,
	kubeInformer kubeinformers.SharedInformerFactory,
	servingInformer informers.SharedInformerFactory,
	configMapWatcher *configmap.ManualWatcher) {

	// Create fake clients
	kubeClient = fakekubeclientset.NewSimpleClientset()
	cms := []*corev1.ConfigMap{{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.DomainConfigName,
			Namespace: system.Namespace(),
		},
		Data: map[string]string{
			defaultDomainSuffix: "",
			prodDomainSuffix:    "selector:\n  app: prod",
		},
	}, {
		ObjectMeta: metav1.ObjectMeta{
			Name:      network.ConfigName,
			Namespace: system.Namespace(),
		},
		Data: map[string]string{},
	}, {
		ObjectMeta: metav1.ObjectMeta{
			Name:      gc.ConfigName,
			Namespace: system.Namespace(),
		},
		Data: map[string]string{},
	}}
	cms = append(cms, configs...)

	configMapWatcher = &configmap.ManualWatcher{Namespace: system.Namespace()}
	servingClient = fakeclientset.NewSimpleClientset()

	// Create informer factories with fake clients. The second parameter sets the
	// resync period to zero, disabling it.
	kubeInformer = kubeinformers.NewSharedInformerFactory(kubeClient, 0)
	servingInformer = informers.NewSharedInformerFactory(servingClient, 0)

	controller = NewController(
		rclr.Options{
			KubeClientSet:    kubeClient,
			ServingClientSet: servingClient,
			ConfigMapWatcher: configMapWatcher,
			Logger:           logtesting.TestLogger(t),
		},
		servingInformer.Serving().V1alpha1().Routes(),
		servingInformer.Serving().V1alpha1().Configurations(),
		servingInformer.Serving().V1alpha1().Revisions(),
		kubeInformer.Core().V1().Services(),
		servingInformer.Networking().V1alpha1().ClusterIngresses(),
	)

	reconciler = controller.Reconciler.(*Reconciler)

	for _, cfg := range cms {
		configMapWatcher.OnChange(cfg)
	}

	return
}

func getRouteIngressFromClient(t *testing.T, servingClient *fakeclientset.Clientset, route *v1alpha1.Route) *netv1alpha1.ClusterIngress {
	opts := metav1.ListOptions{
		LabelSelector: labels.Set(map[string]string{
			serving.RouteLabelKey:          route.Name,
			serving.RouteNamespaceLabelKey: route.Namespace,
		}).AsSelector().String(),
	}
	cis, err := servingClient.NetworkingV1alpha1().ClusterIngresses().List(opts)
	if err != nil {
		t.Errorf("ClusterIngress.Get(%v) = %v", opts, err)
	}

	if len(cis.Items) != 1 {
		t.Errorf("ClusterIngress.Get(%v), expect 1 instance, but got %d", opts, len(cis.Items))
	}

	return &cis.Items[0]
}

func addResourcesToInformers(
	t *testing.T, servingClient *fakeclientset.Clientset,
	servingInformer informers.SharedInformerFactory, route *v1alpha1.Route) {
	t.Helper()

	ns := route.Namespace

	route, err := servingClient.ServingV1alpha1().Routes(ns).Get(route.Name, metav1.GetOptions{})
	if err != nil {
		t.Errorf("Route.Get(%v) = %v", route.Name, err)
	}
	servingInformer.Serving().V1alpha1().Routes().Informer().GetIndexer().Add(route)

	ci := getRouteIngressFromClient(t, servingClient, route)
	servingInformer.Networking().V1alpha1().ClusterIngresses().Informer().GetIndexer().Add(ci)
}

// Test the only revision in the route is in Reserve (inactive) serving status.
func TestCreateRouteForOneReserveRevision(t *testing.T) {
	kubeClient, servingClient, controller, _, servingInformer, _ := newTestReconciler(t)

	h := NewHooks()
	// Look for the events. Events are delivered asynchronously so we need to use
	// hooks here. Each hook tests for a specific event.
	h.OnCreate(&kubeClient.Fake, "events", ExpectNormalEventDelivery(t, `Created service "test-route"`))
	h.OnCreate(&kubeClient.Fake, "events", ExpectNormalEventDelivery(t, "^Created ClusterIngress.*" /*ingress name is unset in test*/))

	// An inactive revision
	rev := getTestRevision("test-rev")
	rev.Status.MarkInactive("NoTraffic", "no message")

	servingClient.ServingV1alpha1().Revisions(testNamespace).Create(rev)
	servingInformer.Serving().V1alpha1().Revisions().Informer().GetIndexer().Add(rev)

	// A route targeting the revision
	route := getTestRouteWithTrafficTargets(
		[]v1alpha1.TrafficTarget{{
			TrafficTarget: v1beta1.TrafficTarget{
				RevisionName:      "test-rev",
				ConfigurationName: "test-config",
				Percent:           100,
			},
		}},
	)
	servingClient.ServingV1alpha1().Routes(testNamespace).Create(route)
	// Since Reconcile looks in the lister, we need to add it to the informer
	servingInformer.Serving().V1alpha1().Routes().Informer().GetIndexer().Add(route)

	controller.Reconcile(context.Background(), KeyOrDie(route))

	ci := getRouteIngressFromClient(t, servingClient, route)

	// Check labels
	expectedLabels := map[string]string{
		serving.RouteLabelKey:          route.Name,
		serving.RouteNamespaceLabelKey: route.Namespace,
	}
	if diff := cmp.Diff(expectedLabels, ci.Labels); diff != "" {
		t.Errorf("Unexpected label diff (-want +got): %v", diff)
	}

	domain := strings.Join([]string{route.Name, route.Namespace, defaultDomainSuffix}, ".")
	expectedSpec := netv1alpha1.IngressSpec{
		Visibility: netv1alpha1.IngressVisibilityExternalIP,
		Rules: []netv1alpha1.ClusterIngressRule{{
			Hosts: []string{
				domain,
				"test-route.test.svc.cluster.local",
			},
			HTTP: &netv1alpha1.HTTPClusterIngressRuleValue{
				Paths: []netv1alpha1.HTTPClusterIngressPath{{
					Splits: []netv1alpha1.ClusterIngressBackendSplit{{
						ClusterIngressBackend: netv1alpha1.ClusterIngressBackend{
							ServiceNamespace: system.Namespace(),
							ServiceName:      "activator-service",
							ServicePort:      intstr.FromInt(80),
						},
						Percent: 100,
					}},
					AppendHeaders: map[string]string{
						"knative-serving-revision":  "test-rev",
						"knative-serving-namespace": testNamespace,
					},
				}},
			},
		}},
	}
	if diff := cmp.Diff(expectedSpec, ci.Spec); diff != "" {
		t.Errorf("Unexpected rule spec diff (-want +got): %s", diff)
	}

	// Update ingress loadbalancer to trigger placeholder service creation.
	ci.Status = netv1alpha1.IngressStatus{
		LoadBalancer: &netv1alpha1.LoadBalancerStatus{
			Ingress: []netv1alpha1.LoadBalancerIngressStatus{{
				DomainInternal: "test-domain",
			}},
		},
	}
	servingInformer.Networking().V1alpha1().ClusterIngresses().Informer().GetIndexer().Update(ci)
	controller.Reconcile(context.Background(), KeyOrDie(route))

	if err := h.WaitForHooks(time.Second * 3); err != nil {
		t.Error(err)
	}
}

func TestCreateRouteWithMultipleTargets(t *testing.T) {
	_, servingClient, controller, _, servingInformer, _ := newTestReconciler(t)
	// A standalone revision
	rev := getTestRevision("test-rev")
	servingClient.ServingV1alpha1().Revisions(testNamespace).Create(rev)
	servingInformer.Serving().V1alpha1().Revisions().Informer().GetIndexer().Add(rev)

	// A configuration and associated revision. Normally the revision would be
	// created by the configuration controller.
	config := getTestConfiguration()
	cfgrev := getTestRevisionForConfig(config)
	config.Status.SetLatestCreatedRevisionName(cfgrev.Name)
	config.Status.SetLatestReadyRevisionName(cfgrev.Name)
	servingClient.ServingV1alpha1().Configurations(testNamespace).Create(config)
	// Since Reconcile looks in the lister, we need to add it to the informer
	servingInformer.Serving().V1alpha1().Configurations().Informer().GetIndexer().Add(config)
	servingClient.ServingV1alpha1().Revisions(testNamespace).Create(cfgrev)
	servingInformer.Serving().V1alpha1().Revisions().Informer().GetIndexer().Add(cfgrev)

	// A route targeting both the config and standalone revision.
	route := getTestRouteWithTrafficTargets(
		[]v1alpha1.TrafficTarget{{
			TrafficTarget: v1beta1.TrafficTarget{
				ConfigurationName: config.Name,
				Percent:           90,
			},
		}, {
			TrafficTarget: v1beta1.TrafficTarget{
				RevisionName: rev.Name,
				Percent:      10,
			},
		}},
	)
	servingClient.ServingV1alpha1().Routes(testNamespace).Create(route)
	// Since Reconcile looks in the lister, we need to add it to the informer.
	servingInformer.Serving().V1alpha1().Routes().Informer().GetIndexer().Add(route)

	controller.Reconcile(context.Background(), KeyOrDie(route))

	ci := getRouteIngressFromClient(t, servingClient, route)
	domain := strings.Join([]string{route.Name, route.Namespace, defaultDomainSuffix}, ".")
	expectedSpec := netv1alpha1.IngressSpec{
		Visibility: netv1alpha1.IngressVisibilityExternalIP,
		Rules: []netv1alpha1.ClusterIngressRule{{
			Hosts: []string{
				domain,
				"test-route.test.svc.cluster.local",
			},
			HTTP: &netv1alpha1.HTTPClusterIngressRuleValue{
				Paths: []netv1alpha1.HTTPClusterIngressPath{{
					Splits: []netv1alpha1.ClusterIngressBackendSplit{{
						ClusterIngressBackend: netv1alpha1.ClusterIngressBackend{
							ServiceNamespace: testNamespace,
							ServiceName:      fmt.Sprintf("%s-pub", cfgrev.Name),
							ServicePort:      intstr.FromInt(80),
						},
						Percent: 90,
					}, {
						ClusterIngressBackend: netv1alpha1.ClusterIngressBackend{
							ServiceNamespace: testNamespace,
							ServiceName:      fmt.Sprintf("%s-pub", rev.Name),
							ServicePort:      intstr.FromInt(80),
						},
						Percent: 10,
					}},
					AppendHeaders: map[string]string{
						"knative-serving-revision":  cfgrev.Name,
						"knative-serving-namespace": testNamespace,
					},
				}},
			},
		}},
	}

	if diff := cmp.Diff(expectedSpec, ci.Spec); diff != "" {
		t.Errorf("Unexpected rule spec diff (-want +got): %v", diff)
	}
}

// Test one out of multiple target revisions is in Reserve serving state.
func TestCreateRouteWithOneTargetReserve(t *testing.T) {
	_, servingClient, controller, _, servingInformer, _ := newTestReconciler(t)
	// A standalone inactive revision
	rev := getTestRevision("test-rev")
	rev.Status.MarkInactive("NoTraffic", "no message")

	servingClient.ServingV1alpha1().Revisions(testNamespace).Create(rev)
	servingInformer.Serving().V1alpha1().Revisions().Informer().GetIndexer().Add(rev)

	// A configuration and associated revision. Normally the revision would be
	// created by the configuration controller.
	config := getTestConfiguration()
	cfgrev := getTestRevisionForConfig(config)
	config.Status.SetLatestCreatedRevisionName(cfgrev.Name)
	config.Status.SetLatestReadyRevisionName(cfgrev.Name)
	servingClient.ServingV1alpha1().Configurations(testNamespace).Create(config)
	// Since Reconcile looks in the lister, we need to add it to the informer
	servingInformer.Serving().V1alpha1().Configurations().Informer().GetIndexer().Add(config)
	servingClient.ServingV1alpha1().Revisions(testNamespace).Create(cfgrev)
	servingInformer.Serving().V1alpha1().Revisions().Informer().GetIndexer().Add(cfgrev)

	// A route targeting both the config and standalone revision
	route := getTestRouteWithTrafficTargets(
		[]v1alpha1.TrafficTarget{{
			TrafficTarget: v1beta1.TrafficTarget{
				ConfigurationName: config.Name,
				Percent:           90,
			},
		}, {
			TrafficTarget: v1beta1.TrafficTarget{
				RevisionName:      rev.Name,
				ConfigurationName: "test-config",
				Percent:           10,
			},
		}},
	)
	servingClient.ServingV1alpha1().Routes(testNamespace).Create(route)
	// Since Reconcile looks in the lister, we need to add it to the informer
	servingInformer.Serving().V1alpha1().Routes().Informer().GetIndexer().Add(route)

	controller.Reconcile(context.Background(), KeyOrDie(route))

	ci := getRouteIngressFromClient(t, servingClient, route)
	domain := strings.Join([]string{route.Name, route.Namespace, defaultDomainSuffix}, ".")
	expectedSpec := netv1alpha1.IngressSpec{
		Visibility: netv1alpha1.IngressVisibilityExternalIP,
		Rules: []netv1alpha1.ClusterIngressRule{{
			Hosts: []string{
				domain,
				"test-route.test.svc.cluster.local",
			},
			HTTP: &netv1alpha1.HTTPClusterIngressRuleValue{
				Paths: []netv1alpha1.HTTPClusterIngressPath{{
					Splits: []netv1alpha1.ClusterIngressBackendSplit{{
						ClusterIngressBackend: netv1alpha1.ClusterIngressBackend{
							ServiceNamespace: testNamespace,
							ServiceName:      fmt.Sprintf("%s-pub", cfgrev.Name),
							ServicePort:      intstr.FromInt(80),
						},
						Percent: 90,
					}, {
						ClusterIngressBackend: netv1alpha1.ClusterIngressBackend{
							ServiceNamespace: system.Namespace(),
							ServiceName:      "activator-service",
							ServicePort:      intstr.FromInt(80),
						},
						Percent: 10,
					}},
					AppendHeaders: map[string]string{
						"knative-serving-revision":  "test-rev",
						"knative-serving-namespace": testNamespace,
					},
				}},
			},
		}},
	}
	if diff := cmp.Diff(expectedSpec, ci.Spec); diff != "" {
		t.Errorf("Unexpected rule spec diff (-want +got): %v", diff)
	}
}

func TestCreateRouteWithDuplicateTargets(t *testing.T) {
	_, servingClient, controller, _, servingInformer, _ := newTestReconciler(t)

	// A standalone revision
	rev := getTestRevision("test-rev")
	servingClient.ServingV1alpha1().Revisions(testNamespace).Create(rev)
	servingInformer.Serving().V1alpha1().Revisions().Informer().GetIndexer().Add(rev)

	// A configuration and associated revision. Normally the revision would be
	// created by the configuration controller.
	config := getTestConfiguration()
	cfgrev := getTestRevisionForConfig(config)
	config.Status.SetLatestCreatedRevisionName(cfgrev.Name)
	config.Status.SetLatestReadyRevisionName(cfgrev.Name)
	servingClient.ServingV1alpha1().Configurations(testNamespace).Create(config)
	// Since Reconcile looks in the lister, we need to add it to the informer
	servingInformer.Serving().V1alpha1().Configurations().Informer().GetIndexer().Add(config)
	servingClient.ServingV1alpha1().Revisions(testNamespace).Create(cfgrev)
	servingInformer.Serving().V1alpha1().Revisions().Informer().GetIndexer().Add(cfgrev)

	// A route with duplicate targets. These will be deduped.
	route := getTestRouteWithTrafficTargets(
		[]v1alpha1.TrafficTarget{{
			TrafficTarget: v1beta1.TrafficTarget{
				ConfigurationName: "test-config",
				Percent:           30,
			},
		}, {
			TrafficTarget: v1beta1.TrafficTarget{
				ConfigurationName: "test-config",
				Percent:           20,
			},
		}, {
			TrafficTarget: v1beta1.TrafficTarget{
				RevisionName: "test-rev",
				Percent:      10,
			},
		}, {
			TrafficTarget: v1beta1.TrafficTarget{
				RevisionName: "test-rev",
				Percent:      5,
			},
		}, {
			Name: "test-revision-1",
			TrafficTarget: v1beta1.TrafficTarget{
				RevisionName: "test-rev",
				Percent:      10,
			},
		}, {
			Name: "test-revision-1",
			TrafficTarget: v1beta1.TrafficTarget{
				RevisionName: "test-rev",
				Percent:      10,
			},
		}, {
			Name: "test-revision-2",
			TrafficTarget: v1beta1.TrafficTarget{
				RevisionName: "test-rev",
				Percent:      15,
			},
		}},
	)
	servingClient.ServingV1alpha1().Routes(testNamespace).Create(route)
	// Since Reconcile looks in the lister, we need to add it to the informer
	servingInformer.Serving().V1alpha1().Routes().Informer().GetIndexer().Add(route)

	controller.Reconcile(context.Background(), KeyOrDie(route))

	ci := getRouteIngressFromClient(t, servingClient, route)
	domain := strings.Join([]string{route.Name, route.Namespace, defaultDomainSuffix}, ".")
	expectedSpec := netv1alpha1.IngressSpec{
		Visibility: netv1alpha1.IngressVisibilityExternalIP,
		Rules: []netv1alpha1.ClusterIngressRule{{
			Hosts: []string{
				domain,
				"test-route.test.svc.cluster.local",
			},
			HTTP: &netv1alpha1.HTTPClusterIngressRuleValue{
				Paths: []netv1alpha1.HTTPClusterIngressPath{{
					Splits: []netv1alpha1.ClusterIngressBackendSplit{{
						ClusterIngressBackend: netv1alpha1.ClusterIngressBackend{
							ServiceNamespace: testNamespace,
							ServiceName:      fmt.Sprintf("%s-pub", cfgrev.Name),
							ServicePort:      intstr.FromInt(80),
						},
						Percent: 50,
					}, {
						ClusterIngressBackend: netv1alpha1.ClusterIngressBackend{
							ServiceNamespace: testNamespace,
							ServiceName:      fmt.Sprintf("%s-pub", rev.Name),
							ServicePort:      intstr.FromInt(80),
						},
						Percent: 50,
					}},
					AppendHeaders: map[string]string{
						"knative-serving-revision":  rev.Name,
						"knative-serving-namespace": testNamespace,
					},
				}},
			},
		}, {
			Hosts: []string{"test-revision-1." + domain},
			HTTP: &netv1alpha1.HTTPClusterIngressRuleValue{
				Paths: []netv1alpha1.HTTPClusterIngressPath{{
					Splits: []netv1alpha1.ClusterIngressBackendSplit{{
						ClusterIngressBackend: netv1alpha1.ClusterIngressBackend{
							ServiceNamespace: testNamespace,
							ServiceName:      "test-rev-pub",
							ServicePort:      intstr.FromInt(80),
						},
						Percent: 100,
					}},
					AppendHeaders: map[string]string{
						"knative-serving-revision":  rev.Name,
						"knative-serving-namespace": testNamespace,
					},
				}},
			},
		}, {
			Hosts: []string{"test-revision-2." + domain},
			HTTP: &netv1alpha1.HTTPClusterIngressRuleValue{
				Paths: []netv1alpha1.HTTPClusterIngressPath{{
					Splits: []netv1alpha1.ClusterIngressBackendSplit{{
						ClusterIngressBackend: netv1alpha1.ClusterIngressBackend{
							ServiceNamespace: testNamespace,
							ServiceName:      "test-rev-pub",
							ServicePort:      intstr.FromInt(80),
						},
						Percent: 100,
					}},
					AppendHeaders: map[string]string{
						"knative-serving-revision":  rev.Name,
						"knative-serving-namespace": testNamespace,
					},
				}},
			},
		}},
	}

	if diff := cmp.Diff(expectedSpec, ci.Spec); diff != "" {
		fmt.Printf("%+v\n", ci.Spec)
		t.Errorf("Unexpected rule spec diff (-want +got): %v", diff)
	}
}

func TestCreateRouteWithNamedTargets(t *testing.T) {
	_, servingClient, controller, _, servingInformer, _ := newTestReconciler(t)
	// A standalone revision
	rev := getTestRevision("test-rev")
	servingClient.ServingV1alpha1().Revisions(testNamespace).Create(rev)
	servingInformer.Serving().V1alpha1().Revisions().Informer().GetIndexer().Add(rev)

	// A configuration and associated revision. Normally the revision would be
	// created by the configuration controller.
	config := getTestConfiguration()
	cfgrev := getTestRevisionForConfig(config)
	config.Status.SetLatestCreatedRevisionName(cfgrev.Name)
	config.Status.SetLatestReadyRevisionName(cfgrev.Name)
	servingClient.ServingV1alpha1().Configurations(testNamespace).Create(config)
	// Since Reconcile looks in the lister, we need to add it to the informer
	servingInformer.Serving().V1alpha1().Configurations().Informer().GetIndexer().Add(config)
	servingClient.ServingV1alpha1().Revisions(testNamespace).Create(cfgrev)
	servingInformer.Serving().V1alpha1().Revisions().Informer().GetIndexer().Add(cfgrev)

	// A route targeting both the config and standalone revision with named
	// targets
	route := getTestRouteWithTrafficTargets(
		[]v1alpha1.TrafficTarget{{
			Name: "foo",
			TrafficTarget: v1beta1.TrafficTarget{
				RevisionName: "test-rev",
				Percent:      50,
			},
		}, {
			Name: "bar",
			TrafficTarget: v1beta1.TrafficTarget{
				ConfigurationName: "test-config",
				Percent:           50,
			},
		}},
	)

	servingClient.ServingV1alpha1().Routes(testNamespace).Create(route)
	// Since Reconcile looks in the lister, we need to add it to the informer
	servingInformer.Serving().V1alpha1().Routes().Informer().GetIndexer().Add(route)

	controller.Reconcile(context.Background(), KeyOrDie(route))

	ci := getRouteIngressFromClient(t, servingClient, route)
	domain := strings.Join([]string{route.Name, route.Namespace, defaultDomainSuffix}, ".")
	expectedSpec := netv1alpha1.IngressSpec{
		Visibility: netv1alpha1.IngressVisibilityExternalIP,
		Rules: []netv1alpha1.ClusterIngressRule{{
			Hosts: []string{
				domain,
				"test-route.test.svc.cluster.local",
			},
			HTTP: &netv1alpha1.HTTPClusterIngressRuleValue{
				Paths: []netv1alpha1.HTTPClusterIngressPath{{
					Splits: []netv1alpha1.ClusterIngressBackendSplit{{
						ClusterIngressBackend: netv1alpha1.ClusterIngressBackend{
							ServiceNamespace: testNamespace,
							ServiceName:      fmt.Sprintf("%s-pub", rev.Name),
							ServicePort:      intstr.FromInt(80),
						},
						Percent: 50,
					}, {
						ClusterIngressBackend: netv1alpha1.ClusterIngressBackend{
							ServiceNamespace: testNamespace,
							ServiceName:      fmt.Sprintf("%s-pub", cfgrev.Name),
							ServicePort:      intstr.FromInt(80),
						},
						Percent: 50,
					}},
					AppendHeaders: map[string]string{
						"knative-serving-revision":  cfgrev.Name,
						"knative-serving-namespace": testNamespace,
					},
				}},
			},
		}, {
			Hosts: []string{"bar." + domain},
			HTTP: &netv1alpha1.HTTPClusterIngressRuleValue{
				Paths: []netv1alpha1.HTTPClusterIngressPath{{
					Splits: []netv1alpha1.ClusterIngressBackendSplit{{
						ClusterIngressBackend: netv1alpha1.ClusterIngressBackend{
							ServiceNamespace: testNamespace,
							ServiceName:      fmt.Sprintf("%s-pub", cfgrev.Name),
							ServicePort:      intstr.FromInt(80),
						},
						Percent: 100,
					}},
					AppendHeaders: map[string]string{
						"knative-serving-revision":  cfgrev.Name,
						"knative-serving-namespace": testNamespace,
					},
				}},
			},
		}, {
			Hosts: []string{"foo." + domain},
			HTTP: &netv1alpha1.HTTPClusterIngressRuleValue{
				Paths: []netv1alpha1.HTTPClusterIngressPath{{
					Splits: []netv1alpha1.ClusterIngressBackendSplit{{
						ClusterIngressBackend: netv1alpha1.ClusterIngressBackend{
							ServiceNamespace: testNamespace,
							ServiceName:      fmt.Sprintf("%s-pub", rev.Name),
							ServicePort:      intstr.FromInt(80),
						},
						Percent: 100,
					}},
					AppendHeaders: map[string]string{
						"knative-serving-revision":  rev.Name,
						"knative-serving-namespace": testNamespace,
					},
				}},
			},
		}},
	}

	if diff := cmp.Diff(expectedSpec, ci.Spec); diff != "" {
		fmt.Printf("%+v\n", ci.Spec)
		t.Errorf("Unexpected rule spec diff (-want +got): %v", diff)
	}
}

func TestUpdateDomainConfigMap(t *testing.T) {
	_, servingClient, controller, _, servingInformer, watcher := newTestReconciler(t)
	route := getTestRouteWithTrafficTargets([]v1alpha1.TrafficTarget{})
	routeClient := servingClient.ServingV1alpha1().Routes(route.Namespace)

	// Create a route.
	servingInformer.Serving().V1alpha1().Routes().Informer().GetIndexer().Add(route)
	routeClient.Create(route)
	controller.Reconcile(context.Background(), KeyOrDie(route))
	addResourcesToInformers(t, servingClient, servingInformer, route)

	route.ObjectMeta.Labels = map[string]string{"app": "prod"}

	// Test changes in domain config map. Routes should get updated appropriately.
	expectations := []struct {
		apply                func()
		expectedDomainSuffix string
	}{{
		expectedDomainSuffix: prodDomainSuffix,
		apply:                func() {},
	}, {
		expectedDomainSuffix: "mytestdomain.com",
		apply: func() {
			domainConfig := corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      config.DomainConfigName,
					Namespace: system.Namespace(),
				},
				Data: map[string]string{
					defaultDomainSuffix: "",
					"mytestdomain.com":  "selector:\n  app: prod",
				},
			}
			watcher.OnChange(&domainConfig)
		},
	}, {
		expectedDomainSuffix: "newdefault.net",
		apply: func() {
			domainConfig := corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      config.DomainConfigName,
					Namespace: system.Namespace(),
				},
				Data: map[string]string{
					"newdefault.net":   "",
					"mytestdomain.com": "selector:\n  app: prod",
				},
			}
			watcher.OnChange(&domainConfig)
			route.Labels = make(map[string]string)
		},
	}, {
		// When no domain with an open selector is specified, we fallback
		// on the default of example.com.
		expectedDomainSuffix: config.DefaultDomain,
		apply: func() {
			domainConfig := corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      config.DomainConfigName,
					Namespace: system.Namespace(),
				},
				Data: map[string]string{
					"mytestdomain.com": "selector:\n  app: prod",
				},
			}
			watcher.OnChange(&domainConfig)
			route.Labels = make(map[string]string)
		},
	}}

	for _, expectation := range expectations {
		t.Run(expectation.expectedDomainSuffix, func(t *testing.T) {
			expectation.apply()
			servingInformer.Serving().V1alpha1().Routes().Informer().GetIndexer().Add(route)
			routeClient.Update(route)
			controller.Reconcile(context.Background(), KeyOrDie(route))
			addResourcesToInformers(t, servingClient, servingInformer, route)

			route, _ = routeClient.Get(route.Name, metav1.GetOptions{})
			expectedDomain := fmt.Sprintf("%s.%s.%s", route.Name, route.Namespace, expectation.expectedDomainSuffix)
			if route.Status.Domain != expectedDomain {
				t.Errorf("Expected domain %q but saw %q", expectedDomain, route.Status.Domain)
			}
		})
	}
}

func TestGlobalResyncOnUpdateDomainConfigMap(t *testing.T) {
	defer logtesting.ClearAll()
	// Test changes in domain config map. Routes should get updated appropriately.
	// We're expecting exactly one route modification per config-map change.
	tests := []struct {
		doThings             func(*configmap.ManualWatcher)
		expectedDomainSuffix string
	}{{
		expectedDomainSuffix: prodDomainSuffix,
		doThings:             func(*configmap.ManualWatcher) {}, // The update will still happen: status will be updated to match the route labels
	}, {
		expectedDomainSuffix: "mytestdomain.com",
		doThings: func(watcher *configmap.ManualWatcher) {
			domainConfig := corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      config.DomainConfigName,
					Namespace: system.Namespace(),
				},
				Data: map[string]string{
					defaultDomainSuffix: "",
					"mytestdomain.com":  "selector:\n  app: prod",
				},
			}
			watcher.OnChange(&domainConfig)
		},
	}, {
		expectedDomainSuffix: "newprod.net",
		doThings: func(watcher *configmap.ManualWatcher) {
			domainConfig := corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      config.DomainConfigName,
					Namespace: system.Namespace(),
				},
				Data: map[string]string{
					defaultDomainSuffix: "",
					"newprod.net":       "selector:\n  app: prod",
				},
			}
			watcher.OnChange(&domainConfig)
		},
	}, {
		expectedDomainSuffix: defaultDomainSuffix,
		doThings: func(watcher *configmap.ManualWatcher) {
			domainConfig := corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      config.DomainConfigName,
					Namespace: system.Namespace(),
				},
				Data: map[string]string{
					defaultDomainSuffix: "",
				},
			}
			watcher.OnChange(&domainConfig)
		},
	}}

	for _, test := range tests {
		test := test
		t.Run(test.expectedDomainSuffix, func(t *testing.T) {
			_, servingClient, controller, _, kubeInformer, servingInformer, watcher := newTestSetup(t)

			stopCh := make(chan struct{})
			grp := errgroup.Group{}
			defer func() {
				close(stopCh)
				if err := grp.Wait(); err != nil {
					t.Errorf("Wait() = %v", err)
				}
			}()
			h := NewHooks()

			// Check for ClusterIngress created as a signal that syncHandler ran
			h.OnUpdate(&servingClient.Fake, "routes", func(obj runtime.Object) HookResult {
				rt := obj.(*v1alpha1.Route)
				t.Logf("route updated: %q", rt.Name)

				expectedDomain := fmt.Sprintf("%s.%s.%s", rt.Name, rt.Namespace, test.expectedDomainSuffix)
				if rt.Status.Domain != expectedDomain {
					t.Logf("Expected domain %q but saw %q", expectedDomain, rt.Status.Domain)
					return HookIncomplete
				}

				return HookComplete
			})

			servingInformer.Start(stopCh)
			kubeInformer.Start(stopCh)

			servingInformer.WaitForCacheSync(stopCh)
			kubeInformer.WaitForCacheSync(stopCh)

			if err := watcher.Start(stopCh); err != nil {
				t.Fatalf("failed to start configuration manager: %v", err)
			}

			grp.Go(func() error { return controller.Run(1, stopCh) })

			// Create a route.
			route := getTestRouteWithTrafficTargets([]v1alpha1.TrafficTarget{})
			route.Labels = map[string]string{"app": "prod"}

			servingClient.ServingV1alpha1().Routes(route.Namespace).Create(route)

			test.doThings(watcher)

			if err := h.WaitForHooks(3 * time.Second); err != nil {
				t.Error(err)
			}
		})
	}
}

func TestRouteDomain(t *testing.T) {
	route := &v1alpha1.Route{
		ObjectMeta: metav1.ObjectMeta{
			SelfLink:  "/apis/serving/v1alpha1/namespaces/test/Routes/myapp",
			Name:      "myapp",
			Namespace: "default",
			Labels: map[string]string{
				"route": "myapp",
			},
		},
	}

	context := context.Background()
	cfg := ReconcilerTestConfig()
	context = config.ToContext(context, cfg)

	tests := []struct {
		Name     string
		Template string
		Pass     bool
		Expected string
	}{{
		Name:     "Default",
		Template: "{{.Name}}.{{.Namespace}}.{{.Domain}}",
		Pass:     true,
		Expected: "myapp.default.example.com",
	}, {
		Name:     "Dash",
		Template: "{{.Name}}-{{.Namespace}}.{{.Domain}}",
		Pass:     true,
		Expected: "myapp-default.example.com",
	}, {
		Name:     "Short",
		Template: "{{.Name}}.{{.Domain}}",
		Pass:     true,
		Expected: "myapp.example.com",
	}, {
		Name:     "SuperShort",
		Template: "{{.Name}}",
		Pass:     true,
		Expected: "myapp",
	}, {
		// This cannot get through our validation, but verify we handle errors.
		Name:     "BadVarName",
		Template: "{{.Name}}.{{.NNNamespace}}.{{.Domain}}",
		Pass:     false,
		Expected: "",
	}}

	for _, test := range tests {
		cfg.Network.DomainTemplate = test.Template

		res, err := routeDomain(context, route)

		if test.Pass != (err == nil) {
			t.Fatalf("TestRouteDomain %q test: supposed to fail but didn't",
				test.Name)
		}
		if res != test.Expected {
			t.Fatalf("TestRouteDomain %q test: got: %q exp: %q",
				test.Name, res, test.Expected)
		}
	}
}

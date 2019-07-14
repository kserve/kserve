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

package kpa

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"go.uber.org/atomic"

	"github.com/knative/pkg/configmap"
	"github.com/knative/pkg/controller"
	"github.com/knative/pkg/kmeta"
	logtesting "github.com/knative/pkg/logging/testing"
	"github.com/knative/pkg/system"
	_ "github.com/knative/pkg/system/testing"
	"github.com/knative/serving/pkg/apis/autoscaling"
	asv1a1 "github.com/knative/serving/pkg/apis/autoscaling/v1alpha1"
	nv1a1 "github.com/knative/serving/pkg/apis/networking/v1alpha1"
	"github.com/knative/serving/pkg/apis/serving"
	"github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"github.com/knative/serving/pkg/autoscaler"
	fakeKna "github.com/knative/serving/pkg/client/clientset/versioned/fake"
	informers "github.com/knative/serving/pkg/client/informers/externalversions"
	"github.com/knative/serving/pkg/reconciler"
	rpkg "github.com/knative/serving/pkg/reconciler"
	"github.com/knative/serving/pkg/reconciler/v1alpha1/autoscaling/kpa/resources"
	"github.com/knative/serving/pkg/reconciler/v1alpha1/autoscaling/kpa/resources/names"
	aresources "github.com/knative/serving/pkg/reconciler/v1alpha1/autoscaling/resources"
	revisionresources "github.com/knative/serving/pkg/reconciler/v1alpha1/revision/resources"
	perrors "github.com/pkg/errors"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubeinformers "k8s.io/client-go/informers"
	fakeK8s "k8s.io/client-go/kubernetes/fake"
	scalefake "k8s.io/client-go/scale/fake"
	clientgotesting "k8s.io/client-go/testing"

	. "github.com/knative/pkg/reconciler/testing"
	. "github.com/knative/serving/pkg/reconciler/v1alpha1/testing"
)

var (
	gracePeriod   = 60 * time.Second
	stableWindow  = 5 * time.Minute
	configMapData = map[string]string{
		"max-scale-up-rate":                       "1.0",
		"container-concurrency-target-percentage": "0.5",
		"container-concurrency-target-default":    "10.0",
		"stable-window":                           stableWindow.String(),
		"panic-window":                            "10s",
		"scale-to-zero-grace-period":              gracePeriod.String(),
		"tick-interval":                           "2s",
	}
)

func newConfigWatcher() configmap.Watcher {
	return configmap.NewStaticWatcher(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: system.Namespace(),
			Name:      autoscaler.ConfigName,
		},
		Data: configMapData,
	})
}

func newDynamicConfig(t *testing.T) *autoscaler.DynamicConfig {
	dynConfig, err := autoscaler.NewDynamicConfigFromMap(configMapData, logtesting.TestLogger(t))
	if err != nil {
		t.Errorf("Error creating DynamicConfig: %v", err)
	}
	return dynConfig
}

// TODO(#3591): Convert KPA tests to table tests.

func TestMetricsSvcIsReconciled(t *testing.T) {
	defer logtesting.ClearAll()

	rev := newTestRevision(testNamespace, testRevision)
	ep := addEndpoint(makeEndpoints(rev))
	kpa := revisionresources.MakeKPA(rev)
	tests := []struct {
		name               string
		wantErr            string
		before             *corev1.Service
		crHook             func(runtime.Object) HookResult
		upHook             func(runtime.Object) HookResult
		hookShouldTO       bool // we expect 0 updates.
		scaleClientReactor func(*scalefake.FakeScaleClient)
		cubeClientReactor  func(*clientgotesting.Fake)
	}{{
		name: "svc does not exist",
		crHook: func(obj runtime.Object) HookResult {
			svc := obj.(*corev1.Service)
			if got, want := svc.Name, names.MetricsServiceName(kpa.Name); got != want {
				t.Errorf("MetricsServiceName = %s, want = %s", got, want)
			}
			return HookComplete
		},
	}, {
		name:         "svc does not exist and we fail to create",
		wantErr:      "this service shall not pass",
		hookShouldTO: true,
		crHook: func(obj runtime.Object) HookResult {
			svc := obj.(*corev1.Service)
			if got, want := svc.Name, names.MetricsServiceName(kpa.Name); got != want {
				t.Errorf("MetricsServiceName = %s, want = %s", got, want)
			}
			return HookComplete
		},
		cubeClientReactor: func(f *clientgotesting.Fake) {
			f.PrependReactor("create", "services", func(action clientgotesting.Action) (bool, runtime.Object, error) {
				return true, nil, errors.New("this service shall not pass")
			})
		},
	}, {
		name:         "scale fail",
		wantErr:      "I like to fail, and I cannot lie",
		hookShouldTO: true,
		before:       resources.MakeMetricsService(kpa, map[string]string{"a": "b"}),
		upHook: func(obj runtime.Object) HookResult {
			svc := obj.(*corev1.Service)
			t.Errorf("Unexpected update for service %s", svc.Name)
			return HookComplete
		},
		scaleClientReactor: func(fsc *scalefake.FakeScaleClient) {
			fsc.AddReactor("get", "deployments", func(action clientgotesting.Action) (bool, runtime.Object, error) {
				return true, nil, errors.New("I like to fail, and I cannot lie")
			})
		},
	}, {
		name:         "bad selector",
		wantErr:      "invalid selector: [i-am-not-a-valid-selector]",
		hookShouldTO: true,
		before:       resources.MakeMetricsService(kpa, map[string]string{"a": "b"}),
		upHook: func(obj runtime.Object) HookResult {
			svc := obj.(*corev1.Service)
			t.Errorf("Unexpected update for service %s", svc.Name)
			return HookComplete
		},
		scaleClientReactor: func(fsc *scalefake.FakeScaleClient) {
			fsc.AddReactor("get", "deployments", func(action clientgotesting.Action) (bool, runtime.Object, error) {
				ga := action.(clientgotesting.GetAction)
				return true, scaleResource(ga.GetNamespace(), ga.GetName(), withLabelSelector("i-am-not-a-valid-selector,¡so-bite-me!")), nil
			})
		},
	}, {
		name:   "svc exists, no change",
		before: resources.MakeMetricsService(kpa, map[string]string{"a": "b"}),
		upHook: func(obj runtime.Object) HookResult {
			svc := obj.(*corev1.Service)
			t.Errorf("Unexpected update for service %s", svc.Name)
			return HookComplete
		},
		scaleClientReactor: func(fsc *scalefake.FakeScaleClient) {
			fsc.AddReactor("get", "deployments", func(action clientgotesting.Action) (bool, runtime.Object, error) {
				ga := action.(clientgotesting.GetAction)
				return true, scaleResource(ga.GetNamespace(), ga.GetName()), nil
			})
		},
		hookShouldTO: true,
	}, {
		name:   "svc exists, need update",
		before: resources.MakeMetricsService(kpa, map[string]string{"hot": "stuff"}),
		upHook: func(obj runtime.Object) HookResult {
			svc := obj.(*corev1.Service)
			// What's being updated.
			if got, want := svc.Spec.Selector, map[string]string{"a": "b"}; !cmp.Equal(got, want) {
				t.Errorf("Selector = %v, want = %v, diff = %s", got, want, cmp.Diff(got, want))
			}
			return HookComplete
		},
		scaleClientReactor: func(fsc *scalefake.FakeScaleClient) {
			fsc.AddReactor("get", "deployments", func(action clientgotesting.Action) (bool, runtime.Object, error) {
				ga := action.(clientgotesting.GetAction)
				return true, scaleResource(ga.GetNamespace(), ga.GetName()), nil
			})
		},
	}, {
		name:         "svc exists, need update, update fails",
		before:       resources.MakeMetricsService(kpa, map[string]string{"hot": "stuff"}),
		wantErr:      "I think I'm immutable",
		hookShouldTO: true,
		upHook: func(obj runtime.Object) HookResult {
			svc := obj.(*corev1.Service)
			// What's being updated.
			if got, want := svc.Spec.Selector, map[string]string{"a": "b"}; !cmp.Equal(got, want) {
				t.Errorf("Selector = %v, want = %v, diff = %s", got, want, cmp.Diff(got, want))
			}
			return HookComplete
		},
		scaleClientReactor: func(fsc *scalefake.FakeScaleClient) {
			fsc.AddReactor("get", "deployments", func(action clientgotesting.Action) (bool, runtime.Object, error) {
				ga := action.(clientgotesting.GetAction)
				return true, scaleResource(ga.GetNamespace(), ga.GetName()), nil
			})
		},
		cubeClientReactor: func(f *clientgotesting.Fake) {
			f.PrependReactor("update", "services", func(action clientgotesting.Action) (bool, runtime.Object, error) {
				return true, nil, errors.New("I think I'm immutable")
			})
		},
	}, {
		name: "svc exists, wrong owner",
		before: func() *corev1.Service {
			s := resources.MakeMetricsService(kpa, map[string]string{"hot": "stuff"})
			s.OwnerReferences[0].UID = types.UID("1984")
			return s
		}(),
		wantErr:      "does not own Service",
		hookShouldTO: true,
	}}
	for _, test := range tests {
		test := test
		// TODO(vagababov): refactor to avoid duplicate work for setup.
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			kubeClient := fakeK8s.NewSimpleClientset()
			servingClient := fakeKna.NewSimpleClientset()

			opts := reconciler.Options{
				KubeClientSet:    kubeClient,
				ServingClientSet: servingClient,
				Logger:           logtesting.TestLogger(t),
			}

			servingInformer := informers.NewSharedInformerFactory(servingClient, 0)
			kubeInformer := kubeinformers.NewSharedInformerFactory(kubeClient, 0)

			scaleClient := &scalefake.FakeScaleClient{}
			if test.scaleClientReactor != nil {
				test.scaleClientReactor(scaleClient)
			}
			if test.cubeClientReactor != nil {
				test.cubeClientReactor(&kubeClient.Fake)
			}
			kpaScaler := NewScaler(servingClient, scaleClient, logtesting.TestLogger(t), newConfigWatcher())

			// This makes controller reconcile synchronously.
			dynConf := newDynamicConfig(t)
			fakeDeciders := newTestDeciders()
			fakeDeciders.Create(context.Background(), resources.MakeDecider(context.Background(), kpa, dynConf.Current()))
			fakeMetrics := newTestMetrics()
			ctl := NewController(&opts,
				servingInformer.Autoscaling().V1alpha1().PodAutoscalers(),
				servingInformer.Networking().V1alpha1().ServerlessServices(),
				kubeInformer.Core().V1().Services(),
				kubeInformer.Core().V1().Endpoints(),
				fakeDeciders,
				fakeMetrics,
				kpaScaler,
				dynConf,
			)

			servingClient.ServingV1alpha1().Revisions(testNamespace).Create(rev)
			servingInformer.Serving().V1alpha1().Revisions().Informer().GetIndexer().Add(rev)
			kubeClient.CoreV1().Endpoints(testNamespace).Create(ep)
			kubeInformer.Core().V1().Endpoints().Informer().GetIndexer().Add(ep)
			servingClient.AutoscalingV1alpha1().PodAutoscalers(testNamespace).Create(kpa)
			servingInformer.Autoscaling().V1alpha1().PodAutoscalers().Informer().GetIndexer().Add(kpa)

			if test.before != nil {
				kubeClient.CoreV1().Services(testNamespace).Create(test.before)
				kubeInformer.Core().V1().Services().Informer().GetIndexer().Add(test.before)
			}
			h := NewHooks()
			if test.crHook != nil {
				h.OnCreate(&kubeClient.Fake, "services", test.crHook)
			}
			if test.upHook != nil {
				h.OnUpdate(&kubeClient.Fake, "services", test.upHook)
			}
			if test.scaleClientReactor != nil {
				test.scaleClientReactor(scaleClient)
			}
			if test.cubeClientReactor != nil {
				test.cubeClientReactor(&kubeClient.Fake)
			}
			err := ctl.Reconciler.Reconcile(context.Background(), testNamespace+"/"+testRevision)
			if err != nil {
				if got, want := err.Error(), test.wantErr; !strings.Contains(got, want) {
					t.Errorf("Error = %q, want: %q", got, want)
				}
			} else if test.wantErr != "" {
				t.Fatal("Expected an error")
			}

			// Hooks should be completed by now, for non TO tests.
			if err := h.WaitForHooks(30 * time.Millisecond); err != nil && !test.hookShouldTO {
				t.Errorf("Metrics Service manipulation faltered: %v", err)
			}
		})
	}
}

func metricsSvc(ns, n string, opts ...K8sServiceOption) *corev1.Service {
	pa := kpa(ns, n)
	svc := resources.MakeMetricsService(pa, map[string]string{})
	for _, opt := range opts {
		opt(svc)
	}
	return svc
}

func sks(ns, n string, so ...SKSOption) *nv1a1.ServerlessService {
	kpa := kpa(ns, n)
	s := aresources.MakeSKS(kpa, nv1a1.SKSOperationModeServe)
	for _, opt := range so {
		opt(s)
	}
	return s
}

func withSvcSelector(sel map[string]string) K8sServiceOption {
	return func(s *corev1.Service) {
		s.Spec.Selector = sel
	}
}

func markActivating(pa *asv1a1.PodAutoscaler) {
	pa.Status.MarkActivating("Queued", "Requests to the target are being buffered as resources are provisioned.")
}

func markActive(pa *asv1a1.PodAutoscaler) {
	pa.Status.MarkActive()
}

func sksWithOwnership(pa *asv1a1.PodAutoscaler) SKSOption {
	return func(sks *nv1a1.ServerlessService) {
		sks.ObjectMeta.OwnerReferences = []metav1.OwnerReference{*kmeta.NewControllerRef(pa)}
	}
}

func kpa(ns, n string, opts ...PodAutoscalerOption) *asv1a1.PodAutoscaler {
	rev := newTestRevision(ns, n)
	kpa := revisionresources.MakeKPA(rev)
	kpa.Annotations["autoscaling.knative.dev/class"] = "kpa.autoscaling.knative.dev"
	for _, opt := range opts {
		opt(kpa)
	}
	return kpa
}

func markResourceNotOwned(rType, name string) PodAutoscalerOption {
	return func(pa *asv1a1.PodAutoscaler) {
		pa.Status.MarkResourceNotOwned(rType, name)
	}
}

func makeTestEndpoints(num int, ns, n string) *corev1.Endpoints {
	rev := newTestRevision(ns, n)
	eps := makeEndpoints(rev)
	for i := 0; i < num; i++ {
		eps = addEndpoint(eps)
	}
	return eps
}

func TestReconcile(t *testing.T) {
	const key = testNamespace + "/" + testRevision
	const deployName = testRevision + "-deployment"
	usualSelector := map[string]string{"a": "b"}

	// Note: due to how KPA reconciler works we are dependent on the
	// two constant objects above, which means, that all tests must share
	// the same namespace and revision name.
	table := TableTest{{
		Name:                    "bad workqueue key, Part I",
		Key:                     "too/many/parts",
		SkipNamespaceValidation: true,
	}, {
		Name:                    "bad workqueue key, Part II",
		Key:                     "too-few-parts",
		SkipNamespaceValidation: true,
	}, {
		Name:                    "key not found",
		Key:                     "foo/not-found",
		SkipNamespaceValidation: true,
	}, {
		Name:                    "key not found",
		Key:                     "foo/not-found",
		SkipNamespaceValidation: true,
	}, {
		Name: "steady state",
		Key:  key,
		Objects: []runtime.Object{
			kpa(testNamespace, testRevision, markActive,
				WithPAStatusService(testRevision+"-pub")),
			sks(testNamespace, testRevision, WithDeployRef(deployName), WithSKSReady),
			metricsSvc(testNamespace, testRevision, withSvcSelector(usualSelector)),
			deploy(testNamespace, testRevision),
			makeSKSPrivateEndpoints(1, testNamespace, testRevision),
		},
	}, {
		Name: "sks is still not ready",
		Key:  key,
		Objects: []runtime.Object{
			kpa(testNamespace, testRevision, markActive, WithPAStatusService(testRevision+"-pub")),
			sks(testNamespace, testRevision, WithDeployRef(deployName), WithPubService, WithPrivateService),
			metricsSvc(testNamespace, testRevision, withSvcSelector(usualSelector)),
			deploy(testNamespace, testRevision),
			makeSKSPrivateEndpoints(1, testNamespace, testRevision),
		},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: kpa(testNamespace, testRevision, markActivating, WithPAStatusService(testRevision+"-pub")),
		}},
	}, {
		Name: "sks becomes ready",
		Key:  key,
		Objects: []runtime.Object{
			kpa(testNamespace, testRevision),
			sks(testNamespace, testRevision, WithDeployRef(deployName), WithSKSReady),
			metricsSvc(testNamespace, testRevision, withSvcSelector(usualSelector)),
			deploy(testNamespace, testRevision),
			makeSKSPrivateEndpoints(1, testNamespace, testRevision),
		},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: kpa(testNamespace, testRevision, markActive, WithPAStatusService(testRevision+"-pub")),
		}},
	}, {
		Name: "sks does not exist",
		Key:  key,
		Objects: []runtime.Object{
			kpa(testNamespace, testRevision, markActive),
			metricsSvc(testNamespace, testRevision, withSvcSelector(usualSelector)),
			deploy(testNamespace, testRevision),
			makeSKSPrivateEndpoints(1, testNamespace, testRevision),
		},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			// SKS does not exist, so we're just creating and have no status.
			Object: kpa(testNamespace, testRevision, markActivating),
		}},
		WantCreates: []metav1.Object{
			sks(testNamespace, testRevision, WithDeployRef(deployName)),
		},
	}, {
		Name: "sks is out of whack",
		Key:  key,
		Objects: []runtime.Object{
			kpa(testNamespace, testRevision, markActive),
			sks(testNamespace, testRevision, WithDeployRef("bar"),
				WithPubService),
			metricsSvc(testNamespace, testRevision, withSvcSelector(usualSelector)),
			deploy(testNamespace, testRevision),
		},
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			// SKS just got updated and we don't have up to date status.
			Object: kpa(testNamespace, testRevision, markActivating, WithPAStatusService(testRevision+"-pub")),
		}},
		WantUpdates: []clientgotesting.UpdateActionImpl{{
			Object: sks(testNamespace, testRevision, WithPubService,
				WithDeployRef(deployName)),
		}},
	}, {
		Name: "sks cannot be created",
		Key:  key,
		Objects: []runtime.Object{
			kpa(testNamespace, testRevision, markActive),
			metricsSvc(testNamespace, testRevision, withSvcSelector(usualSelector)),
			deploy(testNamespace, testRevision),
		},
		WithReactors: []clientgotesting.ReactionFunc{
			InduceFailure("create", "serverlessservices"),
		},
		WantErr: true,
		WantCreates: []metav1.Object{
			sks(testNamespace, testRevision, WithDeployRef(deployName)),
		},
		WantEvents: []string{
			Eventf(corev1.EventTypeWarning, "InternalError",
				"error reconciling SKS: error creating SKS test-revision: inducing failure for create serverlessservices"),
		},
	}, {
		Name: "sks cannot be updated",
		Key:  key,
		Objects: []runtime.Object{
			kpa(testNamespace, testRevision, markActive),
			sks(testNamespace, testRevision, WithDeployRef("bar")),
			metricsSvc(testNamespace, testRevision, withSvcSelector(usualSelector)),
			deploy(testNamespace, testRevision),
		},
		WithReactors: []clientgotesting.ReactionFunc{
			InduceFailure("update", "serverlessservices"),
		},
		WantErr: true,
		WantUpdates: []clientgotesting.UpdateActionImpl{{
			Object: sks(testNamespace, testRevision, WithDeployRef(deployName)),
		}},
		WantEvents: []string{
			Eventf(corev1.EventTypeWarning, "InternalError", "error reconciling SKS: error updating SKS test-revision: inducing failure for update serverlessservices"),
		},
	}, {
		Name: "sks is disowned",
		Key:  key,
		Objects: []runtime.Object{
			kpa(testNamespace, testRevision, markActive),
			sks(testNamespace, testRevision, WithDeployRef(deployName), WithSKSReady,
				WithSKSOwnersRemoved),
			metricsSvc(testNamespace, testRevision, withSvcSelector(usualSelector)),
			deploy(testNamespace, testRevision),
		},
		WantErr: true,
		WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
			Object: kpa(testNamespace, testRevision, markResourceNotOwned("ServerlessService", testRevision)),
		}},
		WantEvents: []string{
			Eventf(corev1.EventTypeWarning, "InternalError", "error reconciling SKS: KPA: test-revision does not own SKS: test-revision"),
		},
	}}

	defer logtesting.ClearAll()
	table.Test(t, MakeFactory(func(listers *Listers, opt rpkg.Options) controller.Reconciler {
		dynConf := newDynamicConfig(t)
		fakeDeciders := newTestDeciders()
		// TODO(vagababov): see if we can get rid of the static piece of configuration and
		// constant namespace and revision names.
		fakeDeciders.Create(context.Background(), resources.MakeDecider(
			context.Background(), kpa(testNamespace, testRevision), dynConf.Current()))

		fakeMetrics := newTestMetrics()
		kpaScaler := NewScaler(opt.ServingClientSet, opt.ScaleClientSet, logtesting.TestLogger(t), newConfigWatcher())
		return &Reconciler{
			Base:            rpkg.NewBase(opt, controllerAgentName),
			paLister:        listers.GetPodAutoscalerLister(),
			sksLister:       listers.GetServerlessServiceLister(),
			serviceLister:   listers.GetK8sServiceLister(),
			endpointsLister: listers.GetEndpointsLister(),
			kpaDeciders:     fakeDeciders,
			metrics:         fakeMetrics,
			scaler:          kpaScaler,
			dynConfig:       dynConf,
		}
	}))
}

type scaleOpt func(*autoscalingv1.Scale)

func withLabelSelector(selector string) scaleOpt {
	return func(s *autoscalingv1.Scale) {
		s.Status.Selector = selector
	}
}

func scaleResource(namespace, name string, opts ...scaleOpt) *autoscalingv1.Scale {
	s := &autoscalingv1.Scale{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: autoscalingv1.ScaleSpec{
			Replicas: 1,
		},
		Status: autoscalingv1.ScaleStatus{
			Replicas: 42,
			Selector: "a=b",
		},
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

type deploymentOption func(*appsv1.Deployment)

func deploy(namespace, name string, opts ...deploymentOption) *appsv1.Deployment {
	s := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name + "-deployment",
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"a": "b",
				},
			},
		},
		Status: appsv1.DeploymentStatus{
			Replicas: 42,
		},
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func TestControllerSynchronizesCreatesAndDeletes(t *testing.T) {
	defer logtesting.ClearAll()

	kubeClient := fakeK8s.NewSimpleClientset()
	servingClient := fakeKna.NewSimpleClientset()

	opts := reconciler.Options{
		KubeClientSet:    kubeClient,
		ServingClientSet: servingClient,
		Logger:           logtesting.TestLogger(t),
	}

	servingInformer := informers.NewSharedInformerFactory(servingClient, 0)
	kubeInformer := kubeinformers.NewSharedInformerFactory(kubeClient, 0)

	scaleClient := &scalefake.FakeScaleClient{}
	scaler := NewScaler(servingClient, scaleClient, logtesting.TestLogger(t), newConfigWatcher())

	fakeDeciders := newTestDeciders()
	fakeMetrics := newTestMetrics()
	ctl := NewController(&opts,
		servingInformer.Autoscaling().V1alpha1().PodAutoscalers(),
		servingInformer.Networking().V1alpha1().ServerlessServices(),
		kubeInformer.Core().V1().Services(),
		kubeInformer.Core().V1().Endpoints(),
		fakeDeciders,
		fakeMetrics,
		scaler,
		newDynamicConfig(t),
	)

	rev := newTestRevision(testNamespace, testRevision)
	servingClient.ServingV1alpha1().Revisions(testNamespace).Create(rev)
	servingInformer.Serving().V1alpha1().Revisions().Informer().GetIndexer().Add(rev)
	ep := makeSKSPrivateEndpoints(1, testNamespace, testRevision)
	kubeClient.CoreV1().Endpoints(testNamespace).Create(ep)
	kubeInformer.Core().V1().Endpoints().Informer().GetIndexer().Add(ep)
	kpa := revisionresources.MakeKPA(rev)
	servingClient.AutoscalingV1alpha1().PodAutoscalers(testNamespace).Create(kpa)
	servingInformer.Autoscaling().V1alpha1().PodAutoscalers().Informer().GetIndexer().Add(kpa)

	sks := sks(testNamespace, testRevision, WithDeployRef(kpa.Spec.ScaleTargetRef.Name),
		WithSKSReady)
	servingClient.NetworkingV1alpha1().ServerlessServices(testNamespace).Create(sks)
	servingInformer.Networking().V1alpha1().ServerlessServices().Informer().GetIndexer().Add(sks)

	msvc := resources.MakeMetricsService(kpa, map[string]string{
		serving.RevisionLabelKey: rev.Name,
	})
	kubeClient.CoreV1().Services(testNamespace).Create(msvc)
	kubeInformer.Core().V1().Services().Informer().GetIndexer().Add(msvc)

	// Wait for the Reconcile to complete.
	if err := ctl.Reconciler.Reconcile(context.Background(), testNamespace+"/"+testRevision); err != nil {
		t.Errorf("Reconcile() = %v", err)
	}

	if count := fakeDeciders.createCallCount.Load(); count != 1 {
		t.Fatalf("Create called %d times instead of once", count)
	}

	newKPA, err := servingClient.AutoscalingV1alpha1().PodAutoscalers(kpa.Namespace).Get(
		kpa.Name, metav1.GetOptions{})
	if err != nil {
		t.Errorf("Get() = %v", err)
	}
	if !newKPA.Status.IsReady() {
		t.Error("Status.IsReady() was false")
	}

	servingClient.ServingV1alpha1().Revisions(testNamespace).Delete(testRevision, nil)
	servingInformer.Serving().V1alpha1().Revisions().Informer().GetIndexer().Delete(rev)
	servingClient.AutoscalingV1alpha1().PodAutoscalers(testNamespace).Delete(testRevision, nil)
	servingInformer.Autoscaling().V1alpha1().PodAutoscalers().Informer().GetIndexer().Delete(kpa)
	if err := ctl.Reconciler.Reconcile(context.Background(), testNamespace+"/"+testRevision); err != nil {
		t.Errorf("Reconcile() = %v", err)
	}

	if fakeDeciders.deleteCallCount.Load() == 0 {
		t.Fatal("Decider was not deleted")
	}
	if fakeMetrics.deleteCallCount.Load() == 0 {
		t.Fatal("Metric was not deleted")
	}

	if fakeDeciders.deleteBeforeCreate.Load() {
		t.Fatal("Deciders.Delete ran before OnPresent")
	}
	if fakeMetrics.deleteBeforeCreate.Load() {
		t.Fatal("Deciders.Delete ran before OnPresent")
	}
}

func TestUpdate(t *testing.T) {
	defer logtesting.ClearAll()

	kubeClient := fakeK8s.NewSimpleClientset()
	servingClient := fakeKna.NewSimpleClientset()

	opts := reconciler.Options{
		KubeClientSet:    kubeClient,
		ServingClientSet: servingClient,
		Logger:           logtesting.TestLogger(t),
	}

	servingInformer := informers.NewSharedInformerFactory(servingClient, 0)
	kubeInformer := kubeinformers.NewSharedInformerFactory(kubeClient, 0)

	scaleClient := &scalefake.FakeScaleClient{}
	scaler := NewScaler(servingClient, scaleClient, logtesting.TestLogger(t), newConfigWatcher())

	fakeDeciders := newTestDeciders()
	fakeMetrics := newTestMetrics()
	dynConf := newDynamicConfig(t)
	ctl := NewController(&opts,
		servingInformer.Autoscaling().V1alpha1().PodAutoscalers(),
		servingInformer.Networking().V1alpha1().ServerlessServices(),
		kubeInformer.Core().V1().Services(),
		kubeInformer.Core().V1().Endpoints(),
		fakeDeciders,
		fakeMetrics,
		scaler,
		dynConf,
	)

	rev := newTestRevision(testNamespace, testRevision)
	servingClient.ServingV1alpha1().Revisions(testNamespace).Create(rev)
	servingInformer.Serving().V1alpha1().Revisions().Informer().GetIndexer().Add(rev)
	ep := makeSKSPrivateEndpoints(1, testNamespace, testRevision)
	kubeClient.CoreV1().Endpoints(testNamespace).Create(ep)
	kubeInformer.Core().V1().Endpoints().Informer().GetIndexer().Add(ep)
	kpa := revisionresources.MakeKPA(rev)
	kpa.SetDefaults(context.Background())
	servingClient.AutoscalingV1alpha1().PodAutoscalers(testNamespace).Create(kpa)
	servingInformer.Autoscaling().V1alpha1().PodAutoscalers().Informer().GetIndexer().Add(kpa)

	msvc := resources.MakeMetricsService(kpa, map[string]string{
		serving.RevisionLabelKey: rev.Name,
	})
	kubeClient.CoreV1().Services(testNamespace).Create(msvc)
	kubeInformer.Core().V1().Services().Informer().GetIndexer().Add(msvc)

	sks := sks(testNamespace, testRevision, WithDeployRef(kpa.Spec.ScaleTargetRef.Name),
		WithSKSReady)
	servingClient.NetworkingV1alpha1().ServerlessServices(testNamespace).Create(sks)
	servingInformer.Networking().V1alpha1().ServerlessServices().Informer().GetIndexer().Add(sks)

	decider := resources.MakeDecider(context.Background(), kpa, dynConf.Current())
	decider.Labels[serving.KubernetesServiceLabelKey] = sks.Status.PrivateServiceName

	// Wait for the Reconcile to complete.
	if err := ctl.Reconciler.Reconcile(context.Background(), testNamespace+"/"+testRevision); err != nil {
		t.Errorf("Reconcile() = %v", err)
	}

	if count := fakeDeciders.createCallCount.Load(); count != 1 {
		t.Fatalf("Deciders.Create called %d times instead of once", count)
	}
	if count := fakeMetrics.createCallCount.Load(); count != 1 {
		t.Fatalf("Metrics.Create called %d times instead of once", count)
	}

	// Verify decider shape.
	if got, want := fakeDeciders.decider, decider; !cmp.Equal(got, want) {
		t.Errorf("decider mismatch: diff(+got, -want): %s", cmp.Diff(got, want))
	}

	newKPA, err := servingClient.AutoscalingV1alpha1().PodAutoscalers(kpa.Namespace).Get(
		kpa.Name, metav1.GetOptions{})
	if err != nil {
		t.Errorf("Get() = %v", err)
	}
	if cond := newKPA.Status.GetCondition("Ready"); cond == nil || cond.Status != "True" {
		t.Errorf("GetCondition(Ready) = %v, wanted True", cond)
	}

	// Update the KPA container concurrency.
	kpa.Spec.ContainerConcurrency = 2
	servingClient.AutoscalingV1alpha1().PodAutoscalers(testNamespace).Update(kpa)
	servingInformer.Autoscaling().V1alpha1().PodAutoscalers().Informer().GetIndexer().Update(kpa)

	if err := ctl.Reconciler.Reconcile(context.Background(), testNamespace+"/"+testRevision); err != nil {
		t.Errorf("Reconcile() = %v", err)
	}

	if fakeDeciders.updateCallCount.Load() == 0 {
		t.Fatal("Deciders.Update was not called")
	}
}

func TestNonKPAClass(t *testing.T) {
	defer logtesting.ClearAll()

	kubeClient := fakeK8s.NewSimpleClientset()
	servingClient := fakeKna.NewSimpleClientset()

	opts := reconciler.Options{
		KubeClientSet:    kubeClient,
		ServingClientSet: servingClient,
		Logger:           logtesting.TestLogger(t),
	}

	servingInformer := informers.NewSharedInformerFactory(servingClient, 0)
	kubeInformer := kubeinformers.NewSharedInformerFactory(kubeClient, 0)

	scaleClient := &scalefake.FakeScaleClient{}
	scaler := NewScaler(servingClient, scaleClient, logtesting.TestLogger(t), newConfigWatcher())

	fakeDeciders := newTestDeciders()
	fakeMetrics := newTestMetrics()
	ctl := NewController(&opts,
		servingInformer.Autoscaling().V1alpha1().PodAutoscalers(),
		servingInformer.Networking().V1alpha1().ServerlessServices(),
		kubeInformer.Core().V1().Services(),
		kubeInformer.Core().V1().Endpoints(),
		fakeDeciders,
		fakeMetrics,
		scaler,
		newDynamicConfig(t),
	)

	rev := newTestRevision(testNamespace, testRevision)
	rev.Annotations = map[string]string{
		autoscaling.ClassAnnotationKey: autoscaling.HPA, // non "kpa" class
	}

	servingClient.ServingV1alpha1().Revisions(testNamespace).Create(rev)
	servingInformer.Serving().V1alpha1().Revisions().Informer().GetIndexer().Add(rev)
	kpa := revisionresources.MakeKPA(rev)
	servingClient.AutoscalingV1alpha1().PodAutoscalers(testNamespace).Create(kpa)
	servingInformer.Autoscaling().V1alpha1().PodAutoscalers().Informer().GetIndexer().Add(kpa)

	// Wait for the Reconcile to complete.
	if err := ctl.Reconciler.Reconcile(context.Background(), testNamespace+"/"+testRevision); err != nil {
		t.Errorf("Reconcile() = %v", err)
	}

	// Verify no Deciders or Metrics were created
	if fakeDeciders.createCallCount.Load() != 0 {
		t.Error("Unexpected Deciders created")
	}
	if fakeMetrics.createCallCount.Load() != 0 {
		t.Error("Unexpected Metrics created")
	}
}

func TestNoEndpoints(t *testing.T) {
	defer logtesting.ClearAll()

	kubeClient := fakeK8s.NewSimpleClientset()
	servingClient := fakeKna.NewSimpleClientset()

	opts := reconciler.Options{
		KubeClientSet:    kubeClient,
		ServingClientSet: servingClient,
		Logger:           logtesting.TestLogger(t),
	}

	servingInformer := informers.NewSharedInformerFactory(servingClient, 0)
	kubeInformer := kubeinformers.NewSharedInformerFactory(kubeClient, 0)

	scaleClient := &scalefake.FakeScaleClient{}
	scaler := NewScaler(servingClient, scaleClient, logtesting.TestLogger(t), newConfigWatcher())

	ctl := NewController(&opts,
		servingInformer.Autoscaling().V1alpha1().PodAutoscalers(),
		servingInformer.Networking().V1alpha1().ServerlessServices(),
		kubeInformer.Core().V1().Services(),
		kubeInformer.Core().V1().Endpoints(),
		newTestDeciders(),
		newTestMetrics(),
		scaler,
		newDynamicConfig(t),
	)

	rev := newTestRevision(testNamespace, testRevision)
	servingClient.ServingV1alpha1().Revisions(testNamespace).Create(rev)
	servingInformer.Serving().V1alpha1().Revisions().Informer().GetIndexer().Add(rev)
	// These do not exist yet.
	// ep := addEndpoint(makeEndpoints(rev))
	// kubeClient.CoreV1().Endpoints(testNamespace).Create(ep)
	// kubeInformer.Core().V1().Endpoints().Informer().GetIndexer().Add(ep)
	kpa := revisionresources.MakeKPA(rev)
	servingClient.AutoscalingV1alpha1().PodAutoscalers(testNamespace).Create(kpa)
	servingInformer.Autoscaling().V1alpha1().PodAutoscalers().Informer().GetIndexer().Add(kpa)

	// Wait for the Reconcile to complete.
	if err := ctl.Reconciler.Reconcile(context.Background(), testNamespace+"/"+testRevision); err != nil {
		t.Errorf("Reconcile() = %v", err)
	}

	newKPA, err := servingClient.AutoscalingV1alpha1().PodAutoscalers(kpa.Namespace).Get(
		kpa.Name, metav1.GetOptions{})
	if err != nil {
		t.Errorf("Get() = %v", err)
	}
	if cond := newKPA.Status.GetCondition("Ready"); cond == nil || cond.Status != "Unknown" {
		t.Errorf("GetCondition(Ready) = %v, wanted Unknown", cond)
	}
}

func TestEmptyEndpoints(t *testing.T) {
	defer logtesting.ClearAll()

	kubeClient := fakeK8s.NewSimpleClientset()
	servingClient := fakeKna.NewSimpleClientset()

	opts := reconciler.Options{
		KubeClientSet:    kubeClient,
		ServingClientSet: servingClient,
		Logger:           logtesting.TestLogger(t),
	}

	servingInformer := informers.NewSharedInformerFactory(servingClient, 0)
	kubeInformer := kubeinformers.NewSharedInformerFactory(kubeClient, 0)

	scaleClient := &scalefake.FakeScaleClient{}
	scaler := NewScaler(servingClient, scaleClient, logtesting.TestLogger(t), newConfigWatcher())

	ctl := NewController(&opts,
		servingInformer.Autoscaling().V1alpha1().PodAutoscalers(),
		servingInformer.Networking().V1alpha1().ServerlessServices(),
		kubeInformer.Core().V1().Services(),
		kubeInformer.Core().V1().Endpoints(),
		newTestDeciders(),
		newTestMetrics(),
		scaler,
		newDynamicConfig(t),
	)

	rev := newTestRevision(testNamespace, testRevision)
	servingClient.ServingV1alpha1().Revisions(testNamespace).Create(rev)
	servingInformer.Serving().V1alpha1().Revisions().Informer().GetIndexer().Add(rev)
	// This is empty still.
	ep := makeEndpoints(rev)
	kubeClient.CoreV1().Endpoints(testNamespace).Create(ep)
	kubeInformer.Core().V1().Endpoints().Informer().GetIndexer().Add(ep)
	kpa := revisionresources.MakeKPA(rev)
	servingClient.AutoscalingV1alpha1().PodAutoscalers(testNamespace).Create(kpa)
	servingInformer.Autoscaling().V1alpha1().PodAutoscalers().Informer().GetIndexer().Add(kpa)

	// Wait for the Reconcile to complete.
	if err := ctl.Reconciler.Reconcile(context.Background(), testNamespace+"/"+testRevision); err != nil {
		t.Errorf("Reconcile() = %v", err)
	}

	newKPA, err := servingClient.AutoscalingV1alpha1().PodAutoscalers(kpa.Namespace).Get(
		kpa.Name, metav1.GetOptions{})
	if err != nil {
		t.Errorf("Get() = %v", err)
	}
	if cond := newKPA.Status.GetCondition("Ready"); cond == nil || cond.Status != "Unknown" {
		t.Errorf("GetCondition(Ready) = %v, wanted Unknown", cond)
	}
}

func TestControllerCreateError(t *testing.T) {
	defer logtesting.ClearAll()

	kubeClient := fakeK8s.NewSimpleClientset()
	servingClient := fakeKna.NewSimpleClientset()

	key := testNamespace + "/" + testRevision
	want := apierrors.NewBadRequest("asdf")

	opts := reconciler.Options{
		KubeClientSet:    kubeClient,
		ServingClientSet: servingClient,
		Logger:           logtesting.TestLogger(t),
	}

	servingInformer := informers.NewSharedInformerFactory(servingClient, 0)
	kubeInformer := kubeinformers.NewSharedInformerFactory(kubeClient, 0)
	scaleClient := &scalefake.FakeScaleClient{}
	scaler := NewScaler(servingClient, scaleClient, logtesting.TestLogger(t), newConfigWatcher())

	ctl := NewController(&opts,
		servingInformer.Autoscaling().V1alpha1().PodAutoscalers(),
		servingInformer.Networking().V1alpha1().ServerlessServices(),
		kubeInformer.Core().V1().Services(),
		kubeInformer.Core().V1().Endpoints(),
		&failingDeciders{
			getErr:    apierrors.NewNotFound(asv1a1.Resource("Deciders"), key),
			createErr: want,
		},
		newTestMetrics(),
		scaler,
		newDynamicConfig(t),
	)

	kpa := revisionresources.MakeKPA(newTestRevision(testNamespace, testRevision))
	servingClient.AutoscalingV1alpha1().PodAutoscalers(testNamespace).Create(kpa)
	servingInformer.Autoscaling().V1alpha1().PodAutoscalers().Informer().GetIndexer().Add(kpa)

	got := perrors.Cause(ctl.Reconciler.Reconcile(context.Background(), key))
	if got != want {
		t.Errorf("Reconcile() = %v, wanted %v", got, want)
	}
}

func TestControllerUpdateError(t *testing.T) {
	defer logtesting.ClearAll()

	kubeClient := fakeK8s.NewSimpleClientset()
	servingClient := fakeKna.NewSimpleClientset()

	key := testNamespace + "/" + testRevision
	want := apierrors.NewBadRequest("asdf")

	opts := reconciler.Options{
		KubeClientSet:    kubeClient,
		ServingClientSet: servingClient,
		Logger:           logtesting.TestLogger(t),
	}

	servingInformer := informers.NewSharedInformerFactory(servingClient, 0)
	kubeInformer := kubeinformers.NewSharedInformerFactory(kubeClient, 0)
	scaleClient := &scalefake.FakeScaleClient{}
	scaler := NewScaler(servingClient, scaleClient, logtesting.TestLogger(t), newConfigWatcher())

	ctl := NewController(&opts,
		servingInformer.Autoscaling().V1alpha1().PodAutoscalers(),
		servingInformer.Networking().V1alpha1().ServerlessServices(),
		kubeInformer.Core().V1().Services(),
		kubeInformer.Core().V1().Endpoints(),
		&failingDeciders{
			getErr:    apierrors.NewNotFound(asv1a1.Resource("Deciders"), key),
			createErr: want,
		},
		newTestMetrics(),
		scaler,
		newDynamicConfig(t),
	)

	kpa := revisionresources.MakeKPA(newTestRevision(testNamespace, testRevision))
	servingClient.AutoscalingV1alpha1().PodAutoscalers(testNamespace).Create(kpa)
	servingInformer.Autoscaling().V1alpha1().PodAutoscalers().Informer().GetIndexer().Add(kpa)

	got := perrors.Cause(ctl.Reconciler.Reconcile(context.Background(), key))
	if got != want {
		t.Errorf("Reconcile() = %v, wanted %v", got, want)
	}
}

func TestControllerGetError(t *testing.T) {
	defer logtesting.ClearAll()

	kubeClient := fakeK8s.NewSimpleClientset()
	servingClient := fakeKna.NewSimpleClientset()

	key := testNamespace + "/" + testRevision
	want := apierrors.NewBadRequest("asdf")

	opts := reconciler.Options{
		KubeClientSet:    kubeClient,
		ServingClientSet: servingClient,
		Logger:           logtesting.TestLogger(t),
	}

	servingInformer := informers.NewSharedInformerFactory(servingClient, 0)
	kubeInformer := kubeinformers.NewSharedInformerFactory(kubeClient, 0)
	scaleClient := &scalefake.FakeScaleClient{}
	scaler := NewScaler(servingClient, scaleClient, logtesting.TestLogger(t), newConfigWatcher())

	ctl := NewController(&opts,
		servingInformer.Autoscaling().V1alpha1().PodAutoscalers(),
		servingInformer.Networking().V1alpha1().ServerlessServices(),
		kubeInformer.Core().V1().Services(),
		kubeInformer.Core().V1().Endpoints(),
		&failingDeciders{
			getErr: want,
		},
		newTestMetrics(),
		scaler,
		newDynamicConfig(t),
	)

	kpa := revisionresources.MakeKPA(newTestRevision(testNamespace, testRevision))
	servingClient.AutoscalingV1alpha1().PodAutoscalers(testNamespace).Create(kpa)
	servingInformer.Autoscaling().V1alpha1().PodAutoscalers().Informer().GetIndexer().Add(kpa)

	got := perrors.Cause(ctl.Reconciler.Reconcile(context.Background(), key))
	if got != want {
		t.Errorf("Reconcile() = %v, wanted %v", got, want)
	}
}

func TestScaleFailure(t *testing.T) {
	defer logtesting.ClearAll()

	kubeClient := fakeK8s.NewSimpleClientset()
	servingClient := fakeKna.NewSimpleClientset()

	opts := reconciler.Options{
		KubeClientSet:    kubeClient,
		ServingClientSet: servingClient,
		Logger:           logtesting.TestLogger(t),
	}

	servingInformer := informers.NewSharedInformerFactory(servingClient, 0)
	kubeInformer := kubeinformers.NewSharedInformerFactory(kubeClient, 0)

	scaleClient := &scalefake.FakeScaleClient{}
	scaler := NewScaler(servingClient, scaleClient, logtesting.TestLogger(t), newConfigWatcher())

	ctl := NewController(&opts,
		servingInformer.Autoscaling().V1alpha1().PodAutoscalers(),
		servingInformer.Networking().V1alpha1().ServerlessServices(),
		kubeInformer.Core().V1().Services(),
		kubeInformer.Core().V1().Endpoints(),
		newTestDeciders(),
		newTestMetrics(),
		scaler,
		newDynamicConfig(t),
	)

	// Only put the KPA in the lister, which will prompt failures scaling it.
	rev := newTestRevision(testNamespace, testRevision)
	kpa := revisionresources.MakeKPA(rev)
	servingInformer.Autoscaling().V1alpha1().PodAutoscalers().Informer().GetIndexer().Add(kpa)

	if err := ctl.Reconciler.Reconcile(context.Background(), testNamespace+"/"+testRevision); err == nil {
		t.Error("Reconcile() = nil, wanted error")
	}
}

func TestBadKey(t *testing.T) {
	defer logtesting.ClearAll()

	kubeClient := fakeK8s.NewSimpleClientset()
	servingClient := fakeKna.NewSimpleClientset()

	opts := reconciler.Options{
		KubeClientSet:    kubeClient,
		ServingClientSet: servingClient,
		Logger:           logtesting.TestLogger(t),
	}

	servingInformer := informers.NewSharedInformerFactory(servingClient, 0)
	kubeInformer := kubeinformers.NewSharedInformerFactory(kubeClient, 0)
	scaleClient := &scalefake.FakeScaleClient{}
	scaler := NewScaler(servingClient, scaleClient, logtesting.TestLogger(t), newConfigWatcher())

	ctl := NewController(&opts,
		servingInformer.Autoscaling().V1alpha1().PodAutoscalers(),
		servingInformer.Networking().V1alpha1().ServerlessServices(),
		kubeInformer.Core().V1().Services(),
		kubeInformer.Core().V1().Endpoints(),
		newTestDeciders(),
		newTestMetrics(),
		scaler,
		newDynamicConfig(t),
	)

	err := ctl.Reconciler.Reconcile(context.Background(), "too/many/parts")
	if err != nil {
		t.Errorf("Reconcile() = %v", err)
	}
}

func newTestDeciders() *testDeciders {
	return &testDeciders{
		createCallCount:    atomic.NewUint32(0),
		deleteCallCount:    atomic.NewUint32(0),
		updateCallCount:    atomic.NewUint32(0),
		deleteBeforeCreate: atomic.NewBool(false),
	}
}

type testDeciders struct {
	createCallCount    *atomic.Uint32
	deleteCallCount    *atomic.Uint32
	updateCallCount    *atomic.Uint32
	deleteBeforeCreate *atomic.Bool
	decider            *autoscaler.Decider
}

func (km *testDeciders) Get(ctx context.Context, namespace, name string) (*autoscaler.Decider, error) {
	if km.decider == nil {
		return nil, apierrors.NewNotFound(asv1a1.Resource("Deciders"), autoscaler.NewMetricKey(namespace, name))
	}
	return km.decider, nil
}

func (km *testDeciders) Create(ctx context.Context, desider *autoscaler.Decider) (*autoscaler.Decider, error) {
	km.decider = desider
	km.createCallCount.Add(1)
	return desider, nil
}

func (km *testDeciders) Delete(ctx context.Context, namespace, name string) error {
	km.decider = nil
	km.deleteCallCount.Add(1)
	if km.createCallCount.Load() == 0 {
		km.deleteBeforeCreate.Store(true)
	}
	return nil
}

func (km *testDeciders) Update(ctx context.Context, decider *autoscaler.Decider) (*autoscaler.Decider, error) {
	km.decider = decider
	km.updateCallCount.Add(1)
	return decider, nil
}

func (km *testDeciders) Watch(fn func(string)) {}

type failingDeciders struct {
	getErr    error
	createErr error
	deleteErr error
}

func (km *failingDeciders) Get(ctx context.Context, namespace, name string) (*autoscaler.Decider, error) {
	return nil, km.getErr
}

func (km *failingDeciders) Create(ctx context.Context, decider *autoscaler.Decider) (*autoscaler.Decider, error) {
	return nil, km.createErr
}

func (km *failingDeciders) Delete(ctx context.Context, namespace, name string) error {
	return km.deleteErr
}

func (km *failingDeciders) Watch(fn func(string)) {
}

func (km *failingDeciders) Update(ctx context.Context, decider *autoscaler.Decider) (*autoscaler.Decider, error) {
	return decider, nil
}

func newTestMetrics() *testMetrics {
	return &testMetrics{
		createCallCount:    atomic.NewUint32(0),
		deleteCallCount:    atomic.NewUint32(0),
		updateCallCount:    atomic.NewUint32(0),
		deleteBeforeCreate: atomic.NewBool(false),
	}
}

type testMetrics struct {
	createCallCount    *atomic.Uint32
	deleteCallCount    *atomic.Uint32
	updateCallCount    *atomic.Uint32
	deleteBeforeCreate *atomic.Bool
	metric             *autoscaler.Metric
}

func (km *testMetrics) Get(ctx context.Context, namespace, name string) (*autoscaler.Metric, error) {
	if km.metric == nil {
		return nil, apierrors.NewNotFound(asv1a1.Resource("Metric"), autoscaler.NewMetricKey(namespace, name))
	}
	return km.metric, nil
}

func (km *testMetrics) Create(ctx context.Context, metric *autoscaler.Metric) (*autoscaler.Metric, error) {
	km.metric = metric
	km.createCallCount.Add(1)
	return metric, nil
}

func (km *testMetrics) Delete(ctx context.Context, namespace, name string) error {
	km.metric = nil
	km.deleteCallCount.Add(1)
	if km.createCallCount.Load() == 0 {
		km.deleteBeforeCreate.Store(true)
	}
	return nil
}

func (km *testMetrics) Update(ctx context.Context, metric *autoscaler.Metric) (*autoscaler.Metric, error) {
	km.metric = metric
	km.updateCallCount.Add(1)
	return metric, nil
}

func newTestRevision(namespace string, name string) *v1alpha1.Revision {
	return &v1alpha1.Revision{
		ObjectMeta: metav1.ObjectMeta{
			SelfLink:  fmt.Sprintf("/apis/ela/v1alpha1/namespaces/%s/revisions/%s", namespace, name),
			Name:      name,
			Namespace: namespace,
		},
		Spec: v1alpha1.RevisionSpec{
			Container: &corev1.Container{
				Image:      "gcr.io/repo/image",
				Command:    []string{"echo"},
				Args:       []string{"hello", "world"},
				WorkingDir: "/tmp",
			},
			DeprecatedConcurrencyModel: v1alpha1.RevisionRequestConcurrencyModelSingle,
		},
	}
}

func makeSKSPrivateEndpoints(num int, ns, n string) *corev1.Endpoints {
	s := sks(testNamespace, testRevision, WithPrivateService)
	eps := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: s.Namespace,
			Name:      s.Status.PrivateServiceName,
		},
	}
	for i := 0; i < num; i++ {
		eps = addEndpoint(eps)
	}
	return eps
}

func makeEndpoints(rev *v1alpha1.Revision) *corev1.Endpoints {
	service := revisionresources.MakeK8sService(rev)
	return &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: service.Namespace,
			Name:      service.Name,
		},
	}
}

func addEndpoint(ep *corev1.Endpoints) *corev1.Endpoints {
	ep.Subsets = []corev1.EndpointSubset{{
		Addresses: []corev1.EndpointAddress{{IP: "127.0.0.1"}},
	}}
	return ep
}

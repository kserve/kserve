/*
Copyright 2018 The Knative Authors.

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

package labeler

import (
	"fmt"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fakekubeclientset "k8s.io/client-go/kubernetes/fake"
	clientgotesting "k8s.io/client-go/testing"

	"github.com/knative/pkg/controller"
	"github.com/knative/pkg/kmeta"
	logtesting "github.com/knative/pkg/logging/testing"
	"github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"github.com/knative/serving/pkg/apis/serving/v1beta1"
	fakeclientset "github.com/knative/serving/pkg/client/clientset/versioned/fake"
	informers "github.com/knative/serving/pkg/client/informers/externalversions"
	"github.com/knative/serving/pkg/reconciler"

	. "github.com/knative/pkg/reconciler/testing"
	. "github.com/knative/serving/pkg/reconciler/v1alpha1/testing"
)

// This is heavily based on the way the OpenShift Ingress controller tests its reconciliation method.
func TestReconcile(t *testing.T) {
	table := TableTest{{
		Name: "bad workqueue key",
		// Make sure Reconcile handles bad keys.
		Key: "too/many/parts",
	}, {
		Name: "key not found",
		// Make sure Reconcile handles good keys that don't exist.
		Key: "foo/not-found",
	}, {
		Name: "label runLatest configuration",
		Objects: []runtime.Object{
			simpleRunLatest("default", "first-reconcile", "the-config"),
			simpleConfig("default", "the-config"),
			simpleRevision("default", "the-config"),
		},
		WantPatches: []clientgotesting.PatchActionImpl{
			patchAddLabel("default", "the-config", "serving.knative.dev/route", "first-reconcile", "v1"),
		},
		Key: "default/first-reconcile",
	}, {
		Name: "steady state",
		Objects: []runtime.Object{
			simpleRunLatest("default", "steady-state", "the-config"),
			routeLabel(simpleConfig("default", "the-config"), "steady-state"),
			simpleRevision("default", "the-config"),
		},
		Key: "default/steady-state",
	}, {
		Name: "failure adding label",
		// Induce a failure during patching
		WantErr: true,
		WithReactors: []clientgotesting.ReactionFunc{
			InduceFailure("patch", "configurations"),
		},
		Objects: []runtime.Object{
			simpleRunLatest("default", "add-label-failure", "the-config"),
			simpleConfig("default", "the-config"),
			simpleRevision("default", "the-config"),
		},
		WantPatches: []clientgotesting.PatchActionImpl{
			patchAddLabel("default", "the-config", "serving.knative.dev/route", "add-label-failure", "v1"),
		},
		Key: "default/add-label-failure",
	}, {
		Name:    "label config with incorrect label",
		WantErr: true,
		Objects: []runtime.Object{
			simpleRunLatest("default", "the-route", "the-config"),
			routeLabel(simpleConfig("default", "the-config"), "another-route"),
			simpleRevision("default", "the-config"),
		},
		Key: "default/the-route",
	}, {
		Name: "change configurations",
		Objects: []runtime.Object{
			simpleRunLatest("default", "config-change", "new-config"),
			routeLabel(simpleConfig("default", "old-config"), "config-change"),
			simpleConfig("default", "new-config"),
			simpleRevision("default", "new-config"),
		},
		WantPatches: []clientgotesting.PatchActionImpl{
			patchRemoveLabel("default", "old-config", "serving.knative.dev/route", "v1"),
			patchAddLabel("default", "new-config", "serving.knative.dev/route", "config-change", "v1"),
		},
		Key: "default/config-change",
	}, {
		Name: "delete route",
		Objects: []runtime.Object{
			routeLabel(simpleConfig("default", "the-config"), "delete-route"),
		},
		WantPatches: []clientgotesting.PatchActionImpl{
			patchRemoveLabel("default", "the-config", "serving.knative.dev/route", "v1"),
		},
		Key: "default/delete-route",
	}, {
		Name: "failure while removing an annotation should return an error",
		// Induce a failure during patching
		WantErr: true,
		WithReactors: []clientgotesting.ReactionFunc{
			InduceFailure("patch", "configurations"),
		},
		Objects: []runtime.Object{
			simpleRunLatest("default", "delete-label-failure", "new-config"),
			routeLabel(simpleConfig("default", "old-config"), "delete-label-failure"),
			routeLabel(simpleConfig("default", "new-config"), "delete-label-failure"),
			simpleRevision("default", "new-config"),
		},
		WantPatches: []clientgotesting.PatchActionImpl{
			patchRemoveLabel("default", "old-config", "serving.knative.dev/route", "v1"),
		},
		Key: "default/delete-label-failure",
	}}

	defer logtesting.ClearAll()
	table.Test(t, MakeFactory(func(listers *Listers, opt reconciler.Options) controller.Reconciler {
		return &Reconciler{
			Base:                reconciler.NewBase(opt, controllerAgentName),
			routeLister:         listers.GetRouteLister(),
			configurationLister: listers.GetConfigurationLister(),
			revisionLister:      listers.GetRevisionLister(),
		}
	}))
}

func routeWithTraffic(namespace, name string, traffic ...v1alpha1.TrafficTarget) *v1alpha1.Route {
	return &v1alpha1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Status: v1alpha1.RouteStatus{
			RouteStatusFields: v1alpha1.RouteStatusFields{
				Traffic: traffic,
			},
		},
	}
}

func simpleRunLatest(namespace, name, config string) *v1alpha1.Route {
	return routeWithTraffic(namespace, name, v1alpha1.TrafficTarget{
		TrafficTarget: v1beta1.TrafficTarget{
			RevisionName: config + "-dbnfd",
			Percent:      100,
		},
	})
}

func routeLabel(cfg *v1alpha1.Configuration, route string) *v1alpha1.Configuration {
	if cfg.Labels == nil {
		cfg.Labels = make(map[string]string)
	}
	cfg.Labels["serving.knative.dev/route"] = route
	return cfg
}

func simpleConfig(namespace, name string) *v1alpha1.Configuration {
	cfg := &v1alpha1.Configuration{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       namespace,
			Name:            name,
			ResourceVersion: "v1",
		},
	}
	cfg.Status.InitializeConditions()
	cfg.Status.SetLatestCreatedRevisionName(name + "-dbnfd")
	cfg.Status.SetLatestReadyRevisionName(name + "-dbnfd")
	return cfg
}

func simpleRevision(namespace, name string) *v1alpha1.Revision {
	cfg := simpleConfig(namespace, name)
	return &v1alpha1.Revision{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       namespace,
			Name:            cfg.Status.LatestCreatedRevisionName,
			OwnerReferences: []metav1.OwnerReference{*kmeta.NewControllerRef(cfg)},
		},
	}
}

func patchRemoveLabel(namespace, name, key, version string) clientgotesting.PatchActionImpl {
	action := clientgotesting.PatchActionImpl{}
	action.Name = name
	action.Namespace = namespace

	patch := fmt.Sprintf(`{"metadata":{"labels":{"%s":null},"resourceVersion":"%s"}}`, key, version)

	action.Patch = []byte(patch)
	return action
}

func patchAddLabel(namespace, name, key, value, version string) clientgotesting.PatchActionImpl {
	action := clientgotesting.PatchActionImpl{}
	action.Name = name
	action.Namespace = namespace

	patch := fmt.Sprintf(`{"metadata":{"labels":{"%s":"%s"},"resourceVersion":"%s"}}`, key, value, version)

	action.Patch = []byte(patch)
	return action
}

func TestNew(t *testing.T) {
	defer logtesting.ClearAll()
	kubeClient := fakekubeclientset.NewSimpleClientset()
	servingClient := fakeclientset.NewSimpleClientset()
	servingInformer := informers.NewSharedInformerFactory(servingClient, 0)

	routeInformer := servingInformer.Serving().V1alpha1().Routes()
	configurationInformer := servingInformer.Serving().V1alpha1().Configurations()
	revisionInformer := servingInformer.Serving().V1alpha1().Revisions()

	c := NewRouteToConfigurationController(reconciler.Options{
		KubeClientSet:    kubeClient,
		ServingClientSet: servingClient,
		Logger:           logtesting.TestLogger(t),
	}, routeInformer, configurationInformer, revisionInformer)

	if c == nil {
		t.Fatal("Expected NewController to return a non-nil value")
	}
}

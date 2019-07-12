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

package route

import (
	"testing"
	"time"

	fakesharedclientset "github.com/knative/pkg/client/clientset/versioned/fake"
	"github.com/knative/pkg/configmap"
	logtesting "github.com/knative/pkg/logging/testing"
	"github.com/knative/pkg/system"
	netv1alpha1 "github.com/knative/serving/pkg/apis/networking/v1alpha1"
	"github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"github.com/knative/serving/pkg/apis/serving/v1beta1"
	fakeclientset "github.com/knative/serving/pkg/client/clientset/versioned/fake"
	informers "github.com/knative/serving/pkg/client/informers/externalversions"
	"github.com/knative/serving/pkg/gc"
	"github.com/knative/serving/pkg/network"
	"github.com/knative/serving/pkg/reconciler"
	"github.com/knative/serving/pkg/reconciler/v1alpha1/route/config"
	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubeinformers "k8s.io/client-go/informers"
	fakekubeclientset "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/record"

	. "github.com/knative/pkg/reconciler/testing"
)

/* TODO tests:
- syncHandler returns error (in processNextWorkItem)
- invalid key in workqueue (in processNextWorkItem)
- object cannot be converted to key (in enqueueConfiguration)
- invalid key given to syncHandler
- resource doesn't exist in lister (from syncHandler)
*/

func TestNewRouteCallsSyncHandler(t *testing.T) {
	defer logtesting.ClearAll()
	// A standalone revision
	rev := getTestRevision("test-rev")
	// A route targeting the revision
	route := getTestRouteWithTrafficTargets(
		[]v1alpha1.TrafficTarget{{
			TrafficTarget: v1beta1.TrafficTarget{
				RevisionName: "test-rev",
				Percent:      100,
			},
		}},
	)

	// Create fake clients
	kubeClient := fakekubeclientset.NewSimpleClientset()
	configMapWatcher := configmap.NewStaticWatcher(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.DomainConfigName,
			Namespace: system.Namespace(),
		},
		Data: map[string]string{
			defaultDomainSuffix: "",
			prodDomainSuffix:    "selector:\n  app: prod",
		},
	}, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      network.ConfigName,
			Namespace: system.Namespace(),
		},
		Data: map[string]string{},
	}, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gc.ConfigName,
			Namespace: system.Namespace(),
		},
		Data: map[string]string{},
	})
	sharedClient := fakesharedclientset.NewSimpleClientset()
	servingClient := fakeclientset.NewSimpleClientset()

	// Create informer factories with fake clients. The second parameter sets the
	// resync period to zero, disabling it.
	kubeInformer := kubeinformers.NewSharedInformerFactory(kubeClient, 0)
	servingInformer := informers.NewSharedInformerFactory(servingClient, 0)

	controller := NewController(
		reconciler.Options{
			KubeClientSet:    kubeClient,
			SharedClientSet:  sharedClient,
			ServingClientSet: servingClient,
			ConfigMapWatcher: configMapWatcher,
			Logger:           logtesting.TestLogger(t),
			Recorder:         record.NewFakeRecorder(1000),
		},
		servingInformer.Serving().V1alpha1().Routes(),
		servingInformer.Serving().V1alpha1().Configurations(),
		servingInformer.Serving().V1alpha1().Revisions(),
		kubeInformer.Core().V1().Services(),
		servingInformer.Networking().V1alpha1().ClusterIngresses(),
	)

	h := NewHooks()

	// Check for ClusterIngress created as a signal that syncHandler ran
	h.OnCreate(&servingClient.Fake, "clusteringresses", func(obj runtime.Object) HookResult {
		ci := obj.(*netv1alpha1.ClusterIngress)
		t.Logf("ingress created: %q", ci.Name)

		return HookComplete
	})

	stopCh := make(chan struct{})
	eg := errgroup.Group{}
	defer func() {
		close(stopCh)
		if err := eg.Wait(); err != nil {
			t.Fatalf("Error running controller: %v", err)
		}
	}()

	kubeInformer.Start(stopCh)
	servingInformer.Start(stopCh)
	configMapWatcher.Start(stopCh)

	kubeInformer.WaitForCacheSync(stopCh)
	servingInformer.WaitForCacheSync(stopCh)

	// Run the controller.
	eg.Go(func() error {
		return controller.Run(2, stopCh)
	})

	if _, err := servingClient.ServingV1alpha1().Revisions(rev.Namespace).Create(rev); err != nil {
		t.Errorf("Unexpected error creating revision: %v", err)
	}

	if _, err := servingClient.ServingV1alpha1().Routes(route.Namespace).Create(route); err != nil {
		t.Errorf("Unexpected error creating route: %v", err)
	}

	if err := h.WaitForHooks(time.Second * 3); err != nil {
		t.Error(err)
	}
}

/*
Copyright 2026 The KServe Authors.

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

package inferenceservice

import (
	"context"
	"reflect"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/kserve/kserve/pkg/constants"
)

func TestManagedResourceCacheSelectors(t *testing.T) {
	opts, err := NewCacheOptions()
	if err != nil {
		t.Fatal(err)
	}
	for _, object := range []client.Object{&appsv1.Deployment{}, &corev1.Service{}} {
		t.Run(reflect.TypeOf(object).String(), func(t *testing.T) {
			var selector labels.Selector
			for configured, config := range opts.ByObject {
				if reflect.TypeOf(configured) == reflect.TypeOf(object) {
					selector = config.Label
				}
			}
			if selector == nil {
				t.Fatal("missing server-side cache label selector")
			}
			if selector.Matches(labels.Set{"app": "unrelated"}) {
				t.Fatal("cache includes unrelated resources")
			}
			for _, ownerLabel := range []string{"serving.kserve.io/inferenceservice", "serving.kserve.io/inferencegraph"} {
				if !selector.Matches(labels.Set{ownerLabel: "model", "serving.kserve.io/managed-by": "kserve-controller-manager"}) {
					t.Fatalf("cache excludes resources owned by %s", ownerLabel)
				}
			}
		})
	}
}

var _ = Describe("managed resource cache", func() {
	It("filters child watches while retaining direct reads of legacy resources", func(ctx SpecContext) {
		opts, err := NewCacheOptions()
		Expect(err).NotTo(HaveOccurred())
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme: k8sClient.Scheme(), Cache: opts, Client: NewClientOptions(),
			Metrics: metricsserver.Options{BindAddress: "0"},
		})
		Expect(err).NotTo(HaveOccurred())
		for _, object := range []client.Object{&appsv1.Deployment{}, &corev1.Service{}} {
			_, err = mgr.GetCache().GetInformer(ctx, object, cache.BlockUntilSynced(false))
			Expect(err).NotTo(HaveOccurred())
		}
		managerCtx, cancel := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() { done <- mgr.Start(managerCtx) }()
		DeferCleanup(func() {
			cancel()
			Eventually(done, 10*time.Second).Should(Receive(Succeed()))
		})
		Expect(mgr.GetCache().WaitForCacheSync(ctx)).To(BeTrue())

		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "cache-filter-"}}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		DeferCleanup(func(ctx SpecContext) { Expect(k8sClient.Delete(ctx, ns)).To(Succeed()) })
		for _, name := range []string{"isvc", "graph", "legacy", "unrelated"} {
			resourceLabels := map[string]string{"app": name}
			if name == "isvc" || name == "graph" {
				resourceLabels[constants.KServeManagedLabelKey] = constants.KServeManagedLabelValue
			}
			if name == "graph" {
				resourceLabels[constants.InferenceGraphLabel] = name
			} else if name != "unrelated" {
				resourceLabels[constants.InferenceServicePodLabelKey] = name
			}
			metadata := metav1.ObjectMeta{Name: name, Namespace: ns.Name, Labels: resourceLabels}
			Expect(k8sClient.Create(ctx, &appsv1.Deployment{
				ObjectMeta: metadata,
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": name}},
						Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "model", Image: "example.com/model:test"}}},
					},
				},
			})).To(Succeed())
			Expect(k8sClient.Create(ctx, &corev1.Service{
				ObjectMeta: metadata,
				Spec:       corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 80}}},
			})).To(Succeed())
		}

		for _, list := range []client.ObjectList{&appsv1.DeploymentList{}, &corev1.ServiceList{}} {
			Eventually(func(g Gomega) {
				g.Expect(mgr.GetCache().List(ctx, list, client.InNamespace(ns.Name))).To(Succeed())
				switch resources := list.(type) {
				case *appsv1.DeploymentList:
					g.Expect(resources.Items).To(HaveLen(2))
				case *corev1.ServiceList:
					g.Expect(resources.Items).To(HaveLen(2))
				}
			}, 10*time.Second).Should(Succeed())
			// Orphan cleanup must also see legacy resources through List.
			Expect(mgr.GetClient().List(ctx, list, client.InNamespace(ns.Name))).To(Succeed())
			switch resources := list.(type) {
			case *appsv1.DeploymentList:
				Expect(resources.Items).To(HaveLen(4))
			case *corev1.ServiceList:
				Expect(resources.Items).To(HaveLen(4))
			}
		}
		for _, object := range []client.Object{&appsv1.Deployment{}, &corev1.Service{}} {
			key := client.ObjectKey{Namespace: ns.Name, Name: "legacy"}
			Expect(apierrors.IsNotFound(mgr.GetCache().Get(ctx, key, object))).To(BeTrue())
			Expect(mgr.GetClient().Get(ctx, key, object)).To(Succeed())
			object.GetLabels()[constants.KServeManagedLabelKey] = constants.KServeManagedLabelValue
			Expect(mgr.GetClient().Update(ctx, object)).To(Succeed())
			Eventually(func() error { return mgr.GetCache().Get(ctx, key, object) }, 10*time.Second).Should(Succeed())
		}
	})
})

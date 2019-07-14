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

package testing

import (
	"context"
	"testing"

	logtesting "github.com/knative/pkg/logging/testing"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	fakedynamicclientset "k8s.io/client-go/dynamic/fake"
	fakekubeclientset "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/scale"
	fakescaleclient "k8s.io/client-go/scale/fake"
	ktesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/record"

	fakecachingclientset "github.com/knative/caching/pkg/client/clientset/versioned/fake"
	fakesharedclientset "github.com/knative/pkg/client/clientset/versioned/fake"
	"github.com/knative/pkg/controller"
	fakeclientset "github.com/knative/serving/pkg/client/clientset/versioned/fake"
	"github.com/knative/serving/pkg/reconciler"

	. "github.com/knative/pkg/reconciler/testing"
)

const (
	// maxEventBufferSize is the estimated max number of event notifications that
	// can be buffered during reconciliation.
	maxEventBufferSize = 10
)

// Ctor functions create a k8s controller with given params.
type Ctor func(*Listers, reconciler.Options) controller.Reconciler

// scaleClient returns a Scale fake K8s client, that returns the scale resource
// defined by the underlying Deployment resource. That deployment resource must
// exist in the passed in `f` clientset.
func scaleClient(f *fakekubeclientset.Clientset) scale.ScalesGetter {
	scaleClient := &fakescaleclient.FakeScaleClient{}
	scaleClient.PrependReactor("get", "deployments", func(action ktesting.Action) (bool, runtime.Object, error) {
		ga := action.(ktesting.GetAction)
		d, err := f.AppsV1().Deployments(ga.GetNamespace()).Get(ga.GetName(), metav1.GetOptions{})
		if err != nil {
			return true, nil, err
		}
		replicas := int32(1)
		if d.Spec.Replicas != nil {
			replicas = *d.Spec.Replicas
		}
		return true, &autoscalingv1.Scale{
			ObjectMeta: d.ObjectMeta,
			Spec: autoscalingv1.ScaleSpec{
				Replicas: replicas,
			},
			Status: autoscalingv1.ScaleStatus{
				Replicas: d.Status.Replicas,
				Selector: labels.FormatLabels(d.Spec.Selector.MatchLabels),
			},
		}, nil
	})
	return scaleClient
}

// MakeFactory creates a reconciler factory with fake clients and controller created by `ctor`.
func MakeFactory(ctor Ctor) Factory {
	return func(t *testing.T, r *TableRow) (controller.Reconciler, ActionRecorderList, EventList, *FakeStatsReporter) {
		ls := NewListers(r.Objects)

		kubeClient := fakekubeclientset.NewSimpleClientset(ls.GetKubeObjects()...)
		sharedClient := fakesharedclientset.NewSimpleClientset(ls.GetSharedObjects()...)
		client := fakeclientset.NewSimpleClientset(ls.GetServingObjects()...)

		dynamicClient := fakedynamicclientset.NewSimpleDynamicClient(runtime.NewScheme(), ls.GetBuildObjects()...)
		cachingClient := fakecachingclientset.NewSimpleClientset(ls.GetCachingObjects()...)
		eventRecorder := record.NewFakeRecorder(maxEventBufferSize)
		statsReporter := &FakeStatsReporter{}

		PrependGenerateNameReactor(&client.Fake)
		PrependGenerateNameReactor(&dynamicClient.Fake)

		// Set up our Controller from the fakes.
		c := ctor(&ls, reconciler.Options{
			KubeClientSet:    kubeClient,
			SharedClientSet:  sharedClient,
			DynamicClientSet: dynamicClient,
			CachingClientSet: cachingClient,
			ServingClientSet: client,
			ScaleClientSet:   scaleClient(kubeClient),
			Recorder:         eventRecorder,
			StatsReporter:    statsReporter,
			Logger:           logtesting.TestLogger(t),
		})

		for _, reactor := range r.WithReactors {
			kubeClient.PrependReactor("*", "*", reactor)
			sharedClient.PrependReactor("*", "*", reactor)
			client.PrependReactor("*", "*", reactor)
			dynamicClient.PrependReactor("*", "*", reactor)
			cachingClient.PrependReactor("*", "*", reactor)
		}

		// Validate all Create operations through the serving client.
		client.PrependReactor("create", "*", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
			// TODO(n3wscott): context.Background is the best we can do at the moment, but it should be set-able.
			return ValidateCreates(context.Background(), action)
		})
		client.PrependReactor("update", "*", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
			// TODO(n3wscott): context.Background is the best we can do at the moment, but it should be set-able.
			return ValidateUpdates(context.Background(), action)
		})

		actionRecorderList := ActionRecorderList{sharedClient, dynamicClient, client, kubeClient, cachingClient}
		eventList := EventList{Recorder: eventRecorder}

		return c, actionRecorderList, eventList, statsReporter
	}
}

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

package build

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	v1alpha1 "github.com/knative/build/pkg/apis/build/v1alpha1"
	"github.com/knative/build/pkg/client/clientset/versioned/fake"
	informers "github.com/knative/build/pkg/client/informers/externalversions"
	"github.com/knative/pkg/apis"
	duckv1alpha1 "github.com/knative/pkg/apis/duck/v1alpha1"
	"github.com/knative/pkg/controller"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	kuberrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubeinformers "k8s.io/client-go/informers"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	clientgotesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
)

// TODO(jasonhall): Test pod creation fails
// TODO(jasonhall): Test build update fails

const noResyncPeriod time.Duration = 0

var ignoreVolatileTime = cmp.Comparer(func(_, _ apis.VolatileTime) bool { return true })

type fixture struct {
	t *testing.T

	client     *fake.Clientset
	kubeclient *k8sfake.Clientset
	objects    []runtime.Object
}

func newBuild(name string) *v1alpha1.Build {
	return &v1alpha1.Build{
		TypeMeta: metav1.TypeMeta{APIVersion: v1alpha1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: metav1.NamespaceDefault,
		},
		Spec: v1alpha1.BuildSpec{
			Timeout: &metav1.Duration{Duration: 20 * time.Minute},
		},
	}
}

func (f *fixture) createServiceAccount() {
	f.t.Helper()
	f.createServiceAccounts(&corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: "default"},
	})
}

func (f *fixture) createServiceAccounts(serviceAccounts ...*corev1.ServiceAccount) {
	f.t.Helper()

	for _, sa := range serviceAccounts {
		if _, err := f.kubeclient.CoreV1().ServiceAccounts(metav1.NamespaceDefault).Create(sa); err != nil {
			f.t.Fatalf("Failed to create ServiceAccount: %v", err)
		}
	}
}

func (f *fixture) newReconciler(stopCh <-chan struct{}) (controller.Reconciler, informers.SharedInformerFactory, kubeinformers.SharedInformerFactory) {
	k8sI := kubeinformers.NewSharedInformerFactory(f.kubeclient, noResyncPeriod)
	logger := zap.NewExample().Sugar()
	i := informers.NewSharedInformerFactory(f.client, noResyncPeriod)
	buildInformer := i.Build().V1alpha1().Builds()
	buildTemplateInformer := i.Build().V1alpha1().BuildTemplates()
	clusterBuildTemplateInformer := i.Build().V1alpha1().ClusterBuildTemplates()
	podInformer := k8sI.Core().V1().Pods()
	timeoutHandler := NewTimeoutHandler(logger, f.kubeclient, f.client, stopCh)
	c := NewController(logger, f.kubeclient, podInformer, f.client, buildInformer, buildTemplateInformer, clusterBuildTemplateInformer, timeoutHandler)
	return c.Reconciler, i, k8sI
}

func (f *fixture) updateIndex(i informers.SharedInformerFactory, b *v1alpha1.Build) {
	i.Build().V1alpha1().Builds().Informer().GetIndexer().Add(b)
}

func (f *fixture) updatePodIndex(i kubeinformers.SharedInformerFactory, p *corev1.Pod) {
	i.Core().V1().Pods().Informer().GetIndexer().Add(p)
}

func getKey(b *v1alpha1.Build, t *testing.T) string {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(b)
	if err != nil {
		t.Errorf("unexpected error getting key for build %v: %v", b.Name, err)
		return ""
	}
	return key
}

func TestBuildNotFoundFlow(t *testing.T) {
	b := newBuild("test")
	f := &fixture{
		t:          t,
		objects:    []runtime.Object{b},
		client:     fake.NewSimpleClientset(b),
		kubeclient: k8sfake.NewSimpleClientset(),
	}
	f.createServiceAccount()

	// induce failure when fetching build information in controller
	reactor := func(action clientgotesting.Action) (handled bool, ret runtime.Object, err error) {
		if action.GetVerb() == "get" && action.GetResource().Resource == "builds" {
			return true, nil, fmt.Errorf("Inducing failure for %q action of %q", action.GetVerb(), action.GetResource().Resource)
		}
		return false, nil, nil
	}
	f.client.PrependReactor("*", "*", reactor)

	stopCh := make(chan struct{})
	defer close(stopCh)

	r, i, k8sI := f.newReconciler(stopCh)
	f.updateIndex(i, b)
	i.Start(stopCh)
	k8sI.Start(stopCh)

	if err := r.Reconcile(context.Background(), getKey(b, t)); err == nil {
		t.Errorf("Expect error syncing build")
	}
}

func TestBuildWithBadKey(t *testing.T) {
	f := &fixture{
		t:          t,
		kubeclient: k8sfake.NewSimpleClientset(),
	}
	f.createServiceAccount()

	stopCh := make(chan struct{})
	defer close(stopCh)

	r, _, _ := f.newReconciler(stopCh)
	if err := r.Reconcile(context.Background(), "bad/worse/worst"); err != nil {
		t.Errorf("Unexpected error while syncing build: %s", err.Error())
	}
}

func TestBuildNotFoundError(t *testing.T) {
	b := newBuild("test")
	f := &fixture{
		t:          t,
		objects:    []runtime.Object{b},
		client:     fake.NewSimpleClientset(b),
		kubeclient: k8sfake.NewSimpleClientset(),
	}
	f.createServiceAccount()

	stopCh := make(chan struct{})
	defer close(stopCh)

	r, i, k8sI := f.newReconciler(stopCh)
	// Don't update build informers with test build object
	i.Start(stopCh)
	k8sI.Start(stopCh)

	if err := r.Reconcile(context.Background(), getKey(b, t)); err != nil {
		t.Errorf("Unexpected error while syncing build: %s", err.Error())
	}
}

func TestBuildWithMissingServiceAccount(t *testing.T) {
	b := newBuild("test-missing-serviceaccount")

	b.Spec = v1alpha1.BuildSpec{
		ServiceAccountName: "missing-sa",
	}

	f := &fixture{
		t:          t,
		objects:    []runtime.Object{b},
		client:     fake.NewSimpleClientset(b),
		kubeclient: k8sfake.NewSimpleClientset(),
	}

	stopCh := make(chan struct{})
	defer close(stopCh)

	r, i, k8sI := f.newReconciler(stopCh)
	f.updateIndex(i, b)
	i.Start(stopCh)
	k8sI.Start(stopCh)

	if err := r.Reconcile(context.Background(), getKey(b, t)); err == nil {
		t.Errorf("Expect error syncing build")
	} else if !kuberrors.IsNotFound(err) {
		t.Errorf("Expect error to be not found err: %s", err.Error())
	}
	buildClient := f.client.BuildV1alpha1().Builds(b.Namespace)
	b, err := buildClient.Get(b.Name, metav1.GetOptions{})
	if err != nil {
		t.Errorf("error fetching build: %v", err)
	}
	// Check that build has the expected status.
	gotCondition := b.Status.GetCondition(duckv1alpha1.ConditionSucceeded)
	if d := cmp.Diff(gotCondition, &duckv1alpha1.Condition{
		Type:    v1alpha1.BuildSucceeded,
		Status:  corev1.ConditionFalse,
		Reason:  "BuildValidationFailed",
		Message: `serviceaccounts "missing-sa" not found`,
	}, ignoreVolatileTime); d != "" {
		t.Errorf("Unexpected build status %s", d)
	}
}

func TestBuildWithMissingSecret(t *testing.T) {
	b := newBuild("test-missing-secret")

	b.Spec = v1alpha1.BuildSpec{
		ServiceAccountName: "banana-sa",
	}

	f := &fixture{
		t:          t,
		objects:    []runtime.Object{b},
		client:     fake.NewSimpleClientset(b),
		kubeclient: k8sfake.NewSimpleClientset(),
	}
	f.createServiceAccounts(&corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: "banana-sa"},
		Secrets:    []corev1.ObjectReference{{Name: "missing-secret"}},
	})

	stopCh := make(chan struct{})
	defer close(stopCh)

	r, i, k8sI := f.newReconciler(stopCh)
	f.updateIndex(i, b)
	i.Start(stopCh)
	k8sI.Start(stopCh)

	if err := r.Reconcile(context.Background(), getKey(b, t)); err == nil {
		t.Errorf("Expect error syncing build")
	} else if !kuberrors.IsNotFound(err) {
		t.Errorf("Expect error to be not found err: %s", err.Error())
	}
	buildClient := f.client.BuildV1alpha1().Builds(b.Namespace)
	b, err := buildClient.Get(b.Name, metav1.GetOptions{})
	if err != nil {
		t.Errorf("error fetching build: %v", err)
	}
	// Check that build has the expected status.
	gotCondition := b.Status.GetCondition(duckv1alpha1.ConditionSucceeded)
	if d := cmp.Diff(gotCondition, &duckv1alpha1.Condition{
		Type:    v1alpha1.BuildSucceeded,
		Status:  corev1.ConditionFalse,
		Reason:  "BuildValidationFailed",
		Message: `secrets "missing-secret" not found`,
	}, ignoreVolatileTime); d != "" {
		t.Errorf("Unexpected build status %s", d)
	}
}

func TestBuildWithNonExistentTemplates(t *testing.T) {
	for _, kind := range []v1alpha1.TemplateKind{v1alpha1.BuildTemplateKind, v1alpha1.ClusterBuildTemplateKind} {
		b := newBuild("test-buildtemplate")

		b.Spec = v1alpha1.BuildSpec{
			Template: &v1alpha1.TemplateInstantiationSpec{
				Kind: kind,
				Name: "not-existent-template",
			},
		}
		f := &fixture{
			t:          t,
			objects:    []runtime.Object{b},
			client:     fake.NewSimpleClientset(b),
			kubeclient: k8sfake.NewSimpleClientset(),
		}
		f.createServiceAccount()

		stopCh := make(chan struct{})
		defer close(stopCh)

		r, i, k8sI := f.newReconciler(stopCh)
		f.updateIndex(i, b)
		i.Start(stopCh)
		k8sI.Start(stopCh)

		if err := r.Reconcile(context.Background(), getKey(b, t)); err == nil {
			t.Errorf("Expect error syncing build")
		} else if !kuberrors.IsNotFound(err) {
			t.Errorf("Expect error to be not found err: %s", err.Error())
		}
		buildClient := f.client.BuildV1alpha1().Builds(b.Namespace)
		b, err := buildClient.Get(b.Name, metav1.GetOptions{})
		if err != nil {
			t.Errorf("error fetching build: %v", err)
		}
		// Check that build has the expected status.
		gotCondition := b.Status.GetCondition(duckv1alpha1.ConditionSucceeded)
		if d := cmp.Diff(gotCondition, &duckv1alpha1.Condition{
			Type:    v1alpha1.BuildSucceeded,
			Status:  corev1.ConditionFalse,
			Reason:  "BuildValidationFailed",
			Message: fmt.Sprintf(`%ss.build.knative.dev "not-existent-template" not found`, strings.ToLower(string(kind))),
		}, ignoreVolatileTime); d != "" {
			t.Errorf("Unexpected build status %s", d)
		}
	}
}

func TestBuildWithTemplate(t *testing.T) {
	tmpl := &v1alpha1.BuildTemplate{
		TypeMeta: metav1.TypeMeta{APIVersion: v1alpha1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-template",
			Namespace: metav1.NamespaceDefault,
		},
	}
	buildTemplateSpec := &v1alpha1.TemplateInstantiationSpec{
		Kind: v1alpha1.BuildTemplateKind,
		Name: tmpl.Name,
		Env:  []corev1.EnvVar{{Value: "testvalue", Name: "testkey"}},
	}

	b := newBuild("test-buildtemplate")
	b.Spec = v1alpha1.BuildSpec{
		Template: buildTemplateSpec,
	}

	f := &fixture{
		t:          t,
		objects:    []runtime.Object{b, tmpl},
		client:     fake.NewSimpleClientset(b, tmpl),
		kubeclient: k8sfake.NewSimpleClientset(),
	}
	f.createServiceAccount()

	stopCh := make(chan struct{})
	defer close(stopCh)

	r, i, k8sI := f.newReconciler(stopCh)
	f.updateIndex(i, b)
	i.Start(stopCh)
	k8sI.Start(stopCh)

	err := i.Build().V1alpha1().BuildTemplates().Informer().GetIndexer().Add(tmpl)
	if err != nil {
		t.Errorf("Unexpected error when adding cluster build template to build informer: %s", err.Error())
	}

	f.updateIndex(i, b)
	i.Start(stopCh)
	k8sI.Start(stopCh)

	if err = r.Reconcile(context.Background(), getKey(b, t)); err != nil {
		t.Errorf("unexpected expecting error while syncing build: %s", err.Error())
	}

	buildClient := f.client.BuildV1alpha1().Builds(b.Namespace)
	b, err = buildClient.Get(b.Name, metav1.GetOptions{})
	if err != nil {
		t.Errorf("error fetching build: %v", err)
	}
	if d := cmp.Diff(b.Spec.Template, buildTemplateSpec); d != "" {
		t.Errorf("error matching build template spec: expected %#v; got %#v; diff %v", buildTemplateSpec, b.Spec.Template, d)
	}
}

func TestBasicFlows(t *testing.T) {
	for _, c := range []struct {
		desc          string
		podStatus     corev1.PodStatus
		wantCondition *duckv1alpha1.Condition
	}{{
		desc:      "success",
		podStatus: corev1.PodStatus{Phase: corev1.PodSucceeded},
		wantCondition: &duckv1alpha1.Condition{
			Type:   v1alpha1.BuildSucceeded,
			Status: corev1.ConditionTrue,
		},
	}, {
		desc:      "running",
		podStatus: corev1.PodStatus{Phase: corev1.PodRunning},
		wantCondition: &duckv1alpha1.Condition{
			Type:   v1alpha1.BuildSucceeded,
			Status: corev1.ConditionUnknown,
			Reason: "Building",
		},
	}, {
		desc: "failure-message",
		podStatus: corev1.PodStatus{
			Phase:   corev1.PodFailed,
			Message: "boom",
		},
		wantCondition: &duckv1alpha1.Condition{
			Type:    v1alpha1.BuildSucceeded,
			Status:  corev1.ConditionFalse,
			Message: "boom",
		},
	}, {
		desc: "pending-waiting-message",
		podStatus: corev1.PodStatus{
			Phase: corev1.PodPending,
			InitContainerStatuses: []corev1.ContainerStatus{{
				// creds-init status; ignored
			}, {
				Name: "status-name",
				State: corev1.ContainerState{
					Waiting: &corev1.ContainerStateWaiting{
						Message: "i'm pending",
					},
				},
			}},
		},
		wantCondition: &duckv1alpha1.Condition{
			Type:    v1alpha1.BuildSucceeded,
			Status:  corev1.ConditionUnknown,
			Reason:  "Pending",
			Message: `build step "status-name" is pending with reason "i'm pending"`,
		},
	}} {
		t.Run(c.desc, func(t *testing.T) {
			b := newBuild(c.desc)
			f := &fixture{
				t:          t,
				objects:    []runtime.Object{b},
				client:     fake.NewSimpleClientset(b),
				kubeclient: k8sfake.NewSimpleClientset(),
			}
			f.createServiceAccount()

			stopCh := make(chan struct{})
			defer close(stopCh)

			r, i, k8sI := f.newReconciler(stopCh)
			f.updateIndex(i, b)
			i.Start(stopCh)
			k8sI.Start(stopCh)

			// Reconcile to pick up changes.
			ctx := context.Background()
			if err := r.Reconcile(ctx, getKey(b, t)); err != nil {
				t.Errorf("error syncing build: %v", err)
			}

			buildClient := f.client.BuildV1alpha1().Builds(b.Namespace)
			b, err := buildClient.Get(b.Name, metav1.GetOptions{})
			if err != nil {
				t.Errorf("error fetching build: %v", err)
			}

			// Update the underlying pod's status.
			b, err = buildClient.Get(b.Name, metav1.GetOptions{})
			if err != nil {
				t.Fatalf("error getting build: %v", err)
			}
			if b.Status.Cluster == nil || b.Status.Cluster.PodName == "" {
				t.Fatalf("build status did not specify podName: %v", b.Status.Cluster)
			}

			podName := b.Status.Cluster.PodName
			p, err := f.kubeclient.CoreV1().Pods(metav1.NamespaceDefault).Get(podName, metav1.GetOptions{})
			if err != nil {
				t.Fatalf("error getting pod %q: %v", podName, err)
			}
			p.Status = c.podStatus
			if _, err := f.kubeclient.CoreV1().Pods(metav1.NamespaceDefault).Update(p); err != nil {
				t.Fatalf("error updating pod %q: %v", podName, err)
			}

			// Reconcile to pick up pod changes.
			f.updatePodIndex(k8sI, p)
			f.updateIndex(i, b)
			if err := r.Reconcile(ctx, getKey(b, t)); err != nil {
				t.Errorf("error syncing build: %v", err)
			}

			b, err = buildClient.Get(b.Name, metav1.GetOptions{})
			if err != nil {
				t.Errorf("error fetching build: %v", err)
			}

			// Check that build has the expected status.
			gotCondition := b.Status.GetCondition(duckv1alpha1.ConditionSucceeded)
			if d := cmp.Diff(gotCondition, c.wantCondition, ignoreVolatileTime); d != "" {
				t.Errorf("Unexpected build status %s", d)
			}
		})
	}
}

func TestTimeoutFlow(t *testing.T) {
	b := newBuild("timeout")
	b.Spec.Timeout = &metav1.Duration{Duration: 500 * time.Millisecond}

	f := &fixture{
		t:          t,
		objects:    []runtime.Object{b},
		client:     fake.NewSimpleClientset(b),
		kubeclient: k8sfake.NewSimpleClientset(),
	}
	f.createServiceAccount()

	stopCh := make(chan struct{})
	defer close(stopCh)
	r, i, k8sI := f.newReconciler(stopCh)
	f.updateIndex(i, b)
	i.Start(stopCh)
	k8sI.Start(stopCh)

	ctx := context.Background()
	if err := r.Reconcile(ctx, getKey(b, t)); err != nil {
		t.Errorf("Not Expect error when syncing build")
	}

	buildClient := f.client.BuildV1alpha1().Builds(b.Namespace)
	b, err := buildClient.Get(b.Name, metav1.GetOptions{})
	if err != nil {
		t.Errorf("error fetching build: %v", err)
	}

	// Right now there is no better way to test timeout rather than wait for it
	time.Sleep(600 * time.Millisecond)

	// Check that the build has the expected timeout status.
	b, err = buildClient.Get(b.Name, metav1.GetOptions{})
	if err != nil {
		t.Errorf("error fetching build: %v", err)
	}
	if d := cmp.Diff(b.Status.GetCondition(duckv1alpha1.ConditionSucceeded), &duckv1alpha1.Condition{
		Type:    duckv1alpha1.ConditionSucceeded,
		Status:  corev1.ConditionFalse,
		Reason:  "BuildTimeout",
		Message: fmt.Sprintf("Build %q failed to finish within \"500ms\"", b.Name),
	}, ignoreVolatileTime); d != "" {
		t.Errorf("Unexpected build status %s", d)
	}
}

func TestCancelledFlow(t *testing.T) {
	b := newBuild("cancelled")

	f := &fixture{
		t:          t,
		objects:    []runtime.Object{b},
		client:     fake.NewSimpleClientset(b),
		kubeclient: k8sfake.NewSimpleClientset(),
	}
	f.createServiceAccount()

	stopCh := make(chan struct{})
	defer close(stopCh)

	r, i, k8sI := f.newReconciler(stopCh)
	f.updateIndex(i, b)
	i.Start(stopCh)
	k8sI.Start(stopCh)

	ctx := context.Background()
	if err := r.Reconcile(ctx, getKey(b, t)); err != nil {
		t.Errorf("Not Expect error when syncing build")
	}

	buildClient := f.client.BuildV1alpha1().Builds(b.Namespace)
	b, err := buildClient.Get(b.Name, metav1.GetOptions{})
	if err != nil {
		t.Errorf("error fetching build: %v", err)
	}

	// Get pod info
	b, err = buildClient.Get(b.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("error getting build: %v", err)
	}
	if b.Status.Cluster == nil || b.Status.Cluster.PodName == "" {
		t.Fatalf("build status did not specify podName: %v", b.Status.Cluster)
	}
	b.Spec.Status = v1alpha1.BuildSpecStatusCancelled

	f.updateIndex(i, b)
	if err := r.Reconcile(ctx, getKey(b, t)); err != nil {
		t.Errorf("error syncing build: %v", err)
	}

	// Check that the build has the expected cancelled status.
	b, err = buildClient.Get(b.Name, metav1.GetOptions{})
	if err != nil {
		t.Errorf("error fetching build: %v", err)
	}
	if d := cmp.Diff(b.Status.GetCondition(duckv1alpha1.ConditionSucceeded), &duckv1alpha1.Condition{
		Type:    duckv1alpha1.ConditionSucceeded,
		Status:  corev1.ConditionFalse,
		Reason:  "BuildCancelled",
		Message: fmt.Sprintf("Build %q was cancelled", b.Name),
	}, ignoreVolatileTime); d != "" {
		t.Errorf("Unexpected build status %s", d)
	}
}

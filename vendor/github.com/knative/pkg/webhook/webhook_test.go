/*
Copyright 2017 The Knative Authors

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

package webhook

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	// "github.com/knative/pkg/apis/duck"
	"golang.org/x/sync/errgroup"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/knative/pkg/apis"
	"github.com/mattbaird/jsonpatch"
	"go.uber.org/zap"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	admissionregistrationv1beta1 "k8s.io/api/admissionregistration/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	fakekubeclientset "k8s.io/client-go/kubernetes/fake"

	. "github.com/knative/pkg/logging/testing"
	. "github.com/knative/pkg/testing"
)

func newDefaultOptions() ControllerOptions {
	return ControllerOptions{
		Namespace:   "knative-something",
		ServiceName: "webhook",
		Port:        443,
		SecretName:  "webhook-certs",
		WebhookName: "webhook.knative.dev",
	}
}

const (
	testNamespace    = "test-namespace"
	testResourceName = "test-resource"
	user1            = "brutto@knative.dev"
	user2            = "arrabbiato@knative.dev"
)

func newNonRunningTestAdmissionController(t *testing.T, options ControllerOptions) (
	kubeClient *fakekubeclientset.Clientset,
	ac *AdmissionController) {
	t.Helper()
	// Create fake clients
	kubeClient = fakekubeclientset.NewSimpleClientset()

	ac, err := NewAdmissionController(kubeClient, options, TestLogger(t))
	if err != nil {
		t.Fatalf("Failed to create new admission controller: %v", err)
	}
	return
}

func TestDeleteAllowed(t *testing.T) {
	_, ac := newNonRunningTestAdmissionController(t, newDefaultOptions())

	req := &admissionv1beta1.AdmissionRequest{
		Operation: admissionv1beta1.Delete,
	}

	if resp := ac.admit(TestContextWithLogger(t), req); !resp.Allowed {
		t.Fatal("Unexpected denial of delete")
	}
}

func TestConnectAllowed(t *testing.T) {
	_, ac := newNonRunningTestAdmissionController(t, newDefaultOptions())

	req := &admissionv1beta1.AdmissionRequest{
		Operation: admissionv1beta1.Connect,
	}

	resp := ac.admit(TestContextWithLogger(t), req)
	if !resp.Allowed {
		t.Fatalf("Unexpected denial of connect")
	}
}

func TestUnknownKindFails(t *testing.T) {
	_, ac := newNonRunningTestAdmissionController(t, newDefaultOptions())

	req := &admissionv1beta1.AdmissionRequest{
		Operation: admissionv1beta1.Create,
		Kind: metav1.GroupVersionKind{
			Group:   "pkg.knative.dev",
			Version: "v1alpha1",
			Kind:    "Garbage",
		},
	}

	expectFailsWith(t, ac.admit(TestContextWithLogger(t), req), "unhandled kind")
}

func TestUnknownVersionFails(t *testing.T) {
	_, ac := newNonRunningTestAdmissionController(t, newDefaultOptions())
	req := &admissionv1beta1.AdmissionRequest{
		Operation: admissionv1beta1.Create,
		Kind: metav1.GroupVersionKind{
			Group:   "pkg.knative.dev",
			Version: "v1beta2",
			Kind:    "Resource",
		},
	}
	expectFailsWith(t, ac.admit(TestContextWithLogger(t), req), "unhandled kind")
}

func TestUnknownFieldFails(t *testing.T) {
	_, ac := newNonRunningTestAdmissionController(t, newDefaultOptions())
	req := &admissionv1beta1.AdmissionRequest{
		Operation: admissionv1beta1.Create,
		Kind: metav1.GroupVersionKind{
			Group:   "pkg.knative.dev",
			Version: "v1alpha1",
			Kind:    "Resource",
		},
	}

	marshaled, err := json.Marshal(map[string]interface{}{
		"spec": map[string]interface{}{
			"foo": "bar",
		},
	})
	if err != nil {
		panic("failed to marshal resource")
	}
	req.Object.Raw = marshaled

	expectFailsWith(t, ac.admit(TestContextWithLogger(t), req),
		`mutation failed: cannot decode incoming new object: json: unknown field "foo"`)
}

func TestAdmitCreates(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(context.Context, *Resource)
		rejection string
		patches   []jsonpatch.JsonPatchOperation
	}{{
		name: "test simple creation (alpha, no diff)",
		setup: func(ctx context.Context, r *Resource) {
			r.TypeMeta.APIVersion = "v1alpha1"
			r.SetDefaults(ctx)
		},
		patches: []jsonpatch.JsonPatchOperation{},
	}, {
		name: "test simple creation (beta, no diff)",
		setup: func(ctx context.Context, r *Resource) {
			r.TypeMeta.APIVersion = "v1beta1"
			r.SetDefaults(ctx)
		},
		patches: []jsonpatch.JsonPatchOperation{},
	}, {
		name: "test simple creation (with defaults)",
		setup: func(ctx context.Context, r *Resource) {
		},
		patches: []jsonpatch.JsonPatchOperation{{
			Operation: "add",
			Path:      "/metadata/annotations",
			Value: map[string]interface{}{
				"testing.knative.dev/creator": user1,
				"testing.knative.dev/updater": user1,
			},
		}, {
			Operation: "add",
			Path:      "/spec/fieldThatsImmutableWithDefault",
			Value:     "this is another default value",
		}, {
			Operation: "add",
			Path:      "/spec/fieldWithDefault",
			Value:     "I'm a default.",
		}},
	}, {
		name: "test simple creation (with defaults around annotations)",
		setup: func(ctx context.Context, r *Resource) {
			r.Annotations = map[string]string{
				"foo": "bar",
			}
		},
		patches: []jsonpatch.JsonPatchOperation{{
			Operation: "add",
			Path:      "/metadata/annotations/testing.knative.dev~1creator",
			Value:     user1,
		}, {
			Operation: "add",
			Path:      "/metadata/annotations/testing.knative.dev~1updater",
			Value:     user1,
		}, {
			Operation: "add",
			Path:      "/spec/fieldThatsImmutableWithDefault",
			Value:     "this is another default value",
		}, {
			Operation: "add",
			Path:      "/spec/fieldWithDefault",
			Value:     "I'm a default.",
		}},
	}, {
		name: "test simple creation (with partially overridden defaults)",
		setup: func(ctx context.Context, r *Resource) {
			r.Spec.FieldThatsImmutableWithDefault = "not the default"
		},
		patches: []jsonpatch.JsonPatchOperation{{
			Operation: "add",
			Path:      "/metadata/annotations",
			Value: map[string]interface{}{
				"testing.knative.dev/creator": user1,
				"testing.knative.dev/updater": user1,
			},
		}, {
			Operation: "add",
			Path:      "/spec/fieldWithDefault",
			Value:     "I'm a default.",
		}},
	}, {
		name: "test simple creation (webhook corrects user annotation)",
		setup: func(ctx context.Context, r *Resource) {
			r.SetDefaults(apis.WithUserInfo(ctx,
				// THIS IS NOT WHO IS CREATING IT, IT IS LIES!
				&authenticationv1.UserInfo{Username: user2}))
		},
		patches: []jsonpatch.JsonPatchOperation{{
			Operation: "replace",
			Path:      "/metadata/annotations/testing.knative.dev~1creator",
			Value:     user1,
		}, {
			Operation: "replace",
			Path:      "/metadata/annotations/testing.knative.dev~1updater",
			Value:     user1,
		}},
	}, {
		name: "with bad field",
		setup: func(ctx context.Context, r *Resource) {
			// Put a bad value in.
			r.Spec.FieldWithValidation = "not what's expected"
		},
		rejection: "invalid value",
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := createResource("a name")
			ctx := apis.WithinCreate(apis.WithUserInfo(
				TestContextWithLogger(t),
				&authenticationv1.UserInfo{Username: user1}))

			// Setup the resource.
			tc.setup(ctx, r)

			_, ac := newNonRunningTestAdmissionController(t, newDefaultOptions())
			resp := ac.admit(ctx, createCreateResource(ctx, r))

			if tc.rejection == "" {
				expectAllowed(t, resp)
				expectPatches(t, resp.Patch, tc.patches)
			} else {
				expectFailsWith(t, resp, tc.rejection)
			}
		})
	}
}

func createCreateResource(ctx context.Context, r *Resource) *admissionv1beta1.AdmissionRequest {
	req := &admissionv1beta1.AdmissionRequest{
		Operation: admissionv1beta1.Create,
		Kind: metav1.GroupVersionKind{
			Group:   "pkg.knative.dev",
			Version: "v1alpha1",
			Kind:    "Resource",
		},
		UserInfo: *apis.GetUserInfo(ctx),
	}
	marshaled, err := json.Marshal(r)
	if err != nil {
		panic("failed to marshal resource")
	}
	req.Object.Raw = marshaled
	return req
}

func TestAdmitUpdates(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(context.Context, *Resource)
		mutate    func(context.Context, *Resource)
		rejection string
		patches   []jsonpatch.JsonPatchOperation
	}{{
		name: "test simple creation (no diff)",
		setup: func(ctx context.Context, r *Resource) {
			r.SetDefaults(ctx)
		},
		mutate: func(ctx context.Context, r *Resource) {
			// If we don't change anything, the updater
			// annotation doesn't change.
		},
		patches: []jsonpatch.JsonPatchOperation{},
	}, {
		name: "test simple creation (update updater annotation)",
		setup: func(ctx context.Context, r *Resource) {
			r.SetDefaults(ctx)
		},
		mutate: func(ctx context.Context, r *Resource) {
			// When we change the spec, the updater
			// annotation changes.
			r.Spec.FieldWithDefault = "not the default"
		},
		patches: []jsonpatch.JsonPatchOperation{{
			Operation: "replace",
			Path:      "/metadata/annotations/testing.knative.dev~1updater",
			Value:     user2,
		}},
	}, {
		name: "test simple creation (annotation change doesn't change updater)",
		setup: func(ctx context.Context, r *Resource) {
			r.SetDefaults(ctx)
		},
		mutate: func(ctx context.Context, r *Resource) {
			// When we change an annotation, the updater doesn't change.
			r.Annotations["foo"] = "bar"
		},
		patches: []jsonpatch.JsonPatchOperation{},
	}, {
		name: "test that updates dropping immutable defaults are filled back in",
		setup: func(ctx context.Context, r *Resource) {
			r.SetDefaults(ctx)
			r.Spec.FieldThatsImmutableWithDefault = ""
		},
		mutate: func(ctx context.Context, r *Resource) {
			r.Spec.FieldThatsImmutableWithDefault = ""
		},
		patches: []jsonpatch.JsonPatchOperation{{
			Operation: "replace",
			Path:      "/metadata/annotations/testing.knative.dev~1updater",
			Value:     user2,
		}, {
			Operation: "add",
			Path:      "/spec/fieldThatsImmutableWithDefault",
			Value:     "this is another default value",
		}},
	}, {
		name: "bad mutation (immutable)",
		setup: func(ctx context.Context, r *Resource) {
			r.SetDefaults(ctx)
		},
		mutate: func(ctx context.Context, r *Resource) {
			r.Spec.FieldThatsImmutableWithDefault = "something different"
		},
		rejection: "Immutable field changed",
	}, {
		name: "bad mutation (validation)",
		setup: func(ctx context.Context, r *Resource) {
			r.SetDefaults(ctx)
		},
		mutate: func(ctx context.Context, r *Resource) {
			r.Spec.FieldWithValidation = "not what's expected"
		},
		rejection: "invalid value",
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			old := createResource("a name")
			ctx := TestContextWithLogger(t)

			// Setup the resource using a creation context as user1
			createCtx := apis.WithUserInfo(apis.WithinCreate(ctx),
				&authenticationv1.UserInfo{Username: user1})
			tc.setup(createCtx, old)

			new := old.DeepCopy()

			// Mutate the resource using the update context as user2
			ctx = apis.WithUserInfo(apis.WithinUpdate(ctx, old),
				&authenticationv1.UserInfo{Username: user2})
			tc.mutate(ctx, new)

			_, ac := newNonRunningTestAdmissionController(t, newDefaultOptions())
			resp := ac.admit(ctx, createUpdateResource(ctx, old, new))

			if tc.rejection == "" {
				expectAllowed(t, resp)
				expectPatches(t, resp.Patch, tc.patches)
			} else {
				expectFailsWith(t, resp, tc.rejection)
			}
		})
	}
}

func createUpdateResource(ctx context.Context, old, new *Resource) *admissionv1beta1.AdmissionRequest {
	req := &admissionv1beta1.AdmissionRequest{
		Operation: admissionv1beta1.Update,
		Kind: metav1.GroupVersionKind{
			Group:   "pkg.knative.dev",
			Version: "v1alpha1",
			Kind:    "Resource",
		},
		UserInfo: *apis.GetUserInfo(ctx),
	}
	marshaled, err := json.Marshal(new)
	if err != nil {
		panic("failed to marshal resource")
	}
	req.Object.Raw = marshaled
	marshaledOld, err := json.Marshal(old)
	if err != nil {
		panic("failed to marshal resource")
	}
	req.OldObject.Raw = marshaledOld
	return req
}

func TestValidCreateResourceSucceedsWithRoundTripAndDefaultPatch(t *testing.T) {
	req := &admissionv1beta1.AdmissionRequest{
		Operation: admissionv1beta1.Create,
		Kind: metav1.GroupVersionKind{
			Group:   "pkg.knative.dev",
			Version: "v1alpha1",
			Kind:    "InnerDefaultResource",
		},
	}
	req.Object.Raw = createInnerDefaultResourceWithoutSpec(t)

	_, ac := newNonRunningTestAdmissionController(t, newDefaultOptions())
	resp := ac.admit(TestContextWithLogger(t), req)
	expectAllowed(t, resp)
	expectPatches(t, resp.Patch, []jsonpatch.JsonPatchOperation{{
		Operation: "add",
		Path:      "/spec",
		Value:     map[string]interface{}{},
	}, {
		Operation: "add",
		Path:      "/spec/fieldWithDefault",
		Value:     "I'm a default.",
	}})
}

func createInnerDefaultResourceWithoutSpec(t *testing.T) []byte {
	t.Helper()
	r := InnerDefaultResource{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      "a name",
		},
	}
	// Remove the 'spec' field of the generated JSON by marshaling it to JSON, parsing that as a
	// generic map[string]interface{}, removing 'spec', and marshaling it again.
	origBytes, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("Error marshaling origBytes: %v", err)
	}
	var q map[string]interface{}
	if err := json.Unmarshal(origBytes, &q); err != nil {
		t.Fatalf("Error unmarshaling origBytes: %v", err)
	}
	delete(q, "spec")
	b, err := json.Marshal(q)
	if err != nil {
		t.Fatalf("Error marshaling q: %v", err)
	}
	return b
}

func TestValidWebhook(t *testing.T) {
	_, ac := newNonRunningTestAdmissionController(t, newDefaultOptions())
	createDeployment(ac)
	ac.register(TestContextWithLogger(t), ac.Client.AdmissionregistrationV1beta1().MutatingWebhookConfigurations(), []byte{})
	_, err := ac.Client.AdmissionregistrationV1beta1().MutatingWebhookConfigurations().Get(ac.Options.WebhookName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to create webhook: %s", err)
	}
}

func TestUpdatingWebhook(t *testing.T) {
	_, ac := newNonRunningTestAdmissionController(t, newDefaultOptions())
	webhook := &admissionregistrationv1beta1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: ac.Options.WebhookName,
		},
		Webhooks: []admissionregistrationv1beta1.Webhook{{
			Name:         ac.Options.WebhookName,
			Rules:        []admissionregistrationv1beta1.RuleWithOperations{{}},
			ClientConfig: admissionregistrationv1beta1.WebhookClientConfig{},
		}},
	}

	createDeployment(ac)
	createWebhook(ac, webhook)
	ac.register(TestContextWithLogger(t), ac.Client.AdmissionregistrationV1beta1().MutatingWebhookConfigurations(), []byte{})
	currentWebhook, _ := ac.Client.AdmissionregistrationV1beta1().MutatingWebhookConfigurations().Get(ac.Options.WebhookName, metav1.GetOptions{})
	if reflect.DeepEqual(currentWebhook.Webhooks, webhook.Webhooks) {
		t.Fatalf("Expected webhook to be updated")
	}
}

func TestRegistrationStopChanFire(t *testing.T) {
	opts := newDefaultOptions()
	_, ac := newNonRunningTestAdmissionController(t, opts)
	webhook := &admissionregistrationv1beta1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: ac.Options.WebhookName,
		},
		Webhooks: []admissionregistrationv1beta1.Webhook{
			{
				Name:         ac.Options.WebhookName,
				Rules:        []admissionregistrationv1beta1.RuleWithOperations{{}},
				ClientConfig: admissionregistrationv1beta1.WebhookClientConfig{},
			},
		},
	}
	createWebhook(ac, webhook)

	ac.Options.RegistrationDelay = 1 * time.Minute
	stopCh := make(chan struct{})

	var g errgroup.Group
	g.Go(func() error {
		return ac.Run(stopCh)
	})
	close(stopCh)

	if err := g.Wait(); err != nil {
		t.Fatal("Error during run: ", err)
	}
	conn, err := net.Dial("tcp", fmt.Sprintf(":%d", opts.Port))
	if err == nil {
		conn.Close()
		t.Errorf("Unexpected success to dial to port %d", opts.Port)
	}
}

func TestRegistrationForAlreadyExistingWebhook(t *testing.T) {
	_, ac := newNonRunningTestAdmissionController(t, newDefaultOptions())
	webhook := &admissionregistrationv1beta1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: ac.Options.WebhookName,
		},
		Webhooks: []admissionregistrationv1beta1.Webhook{
			{
				Name:         ac.Options.WebhookName,
				Rules:        []admissionregistrationv1beta1.RuleWithOperations{{}},
				ClientConfig: admissionregistrationv1beta1.WebhookClientConfig{},
			},
		},
	}
	createWebhook(ac, webhook)

	ac.Options.RegistrationDelay = 1 * time.Millisecond
	stopCh := make(chan struct{})

	var g errgroup.Group
	g.Go(func() error {
		return ac.Run(stopCh)
	})
	err := g.Wait()
	if err == nil {
		t.Fatal("Expected webhook controller to fail")
	}

	if ac.Options.ClientAuth >= tls.VerifyClientCertIfGiven && !strings.Contains(err.Error(), "configmaps") {
		t.Fatal("Expected error msg to contain configmap key missing error")
	}
}

func TestCertConfigurationForAlreadyGeneratedSecret(t *testing.T) {
	secretName := "test-secret"
	ns := "test-namespace"
	opts := newDefaultOptions()
	opts.SecretName = secretName
	opts.Namespace = ns
	kubeClient, ac := newNonRunningTestAdmissionController(t, opts)

	ctx := TestContextWithLogger(t)
	newSecret, err := generateSecret(ctx, &opts)
	if err != nil {
		t.Fatalf("Failed to generate secret: %v", err)
	}
	_, err = kubeClient.CoreV1().Secrets(ns).Create(newSecret)
	if err != nil {
		t.Fatalf("Failed to create secret: %v", err)
	}

	createNamespace(t, ac.Client, metav1.NamespaceSystem)
	createTestConfigMap(t, ac.Client)

	tlsConfig, caCert, err := configureCerts(ctx, kubeClient, &ac.Options)
	if err != nil {
		t.Fatalf("Failed to configure secret: %v", err)
	}
	expectedCert, err := tls.X509KeyPair(newSecret.Data[secretServerCert], newSecret.Data[secretServerKey])
	if err != nil {
		t.Fatalf("Failed to create cert from x509 key pair: %v", err)
	}

	if tlsConfig == nil {
		t.Fatal("Expected TLS config not to be nil")
	}
	if len(tlsConfig.Certificates) < 1 {
		t.Fatalf("Expected TLS Config Cert to be set")
	}

	if diff := cmp.Diff(expectedCert.Certificate, tlsConfig.Certificates[0].Certificate, cmp.AllowUnexported()); diff != "" {
		t.Fatalf("Unexpected cert diff (-want, +got) %v", diff)
	}
	if diff := cmp.Diff(newSecret.Data[secretCACert], caCert, cmp.AllowUnexported()); diff != "" {
		t.Fatalf("Unexpected CA cert diff (-want, +got) %v", diff)
	}
}

func TestCertConfigurationForGeneratedSecret(t *testing.T) {
	secretName := "test-secret"
	ns := "test-namespace"
	opts := newDefaultOptions()
	opts.SecretName = secretName
	opts.Namespace = ns
	kubeClient, ac := newNonRunningTestAdmissionController(t, opts)

	ctx := TestContextWithLogger(t)
	createNamespace(t, ac.Client, metav1.NamespaceSystem)
	createTestConfigMap(t, ac.Client)

	tlsConfig, caCert, err := configureCerts(ctx, kubeClient, &ac.Options)
	if err != nil {
		t.Fatalf("Failed to configure certificates: %v", err)
	}

	if tlsConfig == nil {
		t.Fatal("Expected TLS config not to be nil")
	}
	if len(tlsConfig.Certificates) < 1 {
		t.Fatalf("Expected TLS Certfificate to be set on webhook server")
	}

	p, _ := pem.Decode(caCert)
	if p == nil {
		t.Fatalf("Expected PEM encoded CA cert ")
	}
	if p.Type != "CERTIFICATE" {
		t.Fatalf("Expectet type to be CERTIFICATE but got %s", string(p.Type))
	}
}

func TestSettingWebhookClientAuth(t *testing.T) {
	opts := newDefaultOptions()
	if opts.ClientAuth != tls.NoClientCert {
		t.Fatalf("Expected default ClientAuth to be NoClientCert (%v) but got (%v)",
			tls.NoClientCert, opts.ClientAuth)
	}
}

func createDeployment(ac *AdmissionController) {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "whatever",
			Namespace: "knative-something",
		},
	}
	ac.Client.Apps().Deployments("knative-something").Create(deployment)
}

func createResource(name string) *Resource {
	return &Resource{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      name,
		},
		Spec: ResourceSpec{
			FieldWithValidation: "magic value",
		},
	}
}

func createWebhook(ac *AdmissionController, webhook *admissionregistrationv1beta1.MutatingWebhookConfiguration) {
	client := ac.Client.AdmissionregistrationV1beta1().MutatingWebhookConfigurations()
	_, err := client.Create(webhook)
	if err != nil {
		panic(fmt.Sprintf("failed to create test webhook: %s", err))
	}
}

func expectAllowed(t *testing.T, resp *admissionv1beta1.AdmissionResponse) {
	t.Helper()
	if !resp.Allowed {
		t.Errorf("Expected allowed, but failed with %+v", resp.Result)
	}
}

func expectFailsWith(t *testing.T, resp *admissionv1beta1.AdmissionResponse, contains string) {
	t.Helper()
	if resp.Allowed {
		t.Error("Expected denial, got allowed")
		return
	}
	if !strings.Contains(resp.Result.Message, contains) {
		t.Errorf("Expected failure containing %q got %q", contains, resp.Result.Message)
	}
}

func expectPatches(t *testing.T, a []byte, e []jsonpatch.JsonPatchOperation) {
	t.Helper()
	var got []jsonpatch.JsonPatchOperation

	err := json.Unmarshal(a, &got)
	if err != nil {
		t.Errorf("Failed to unmarshal patches: %s", err)
		return
	}

	// Give the patch a deterministic ordering.
	// Technically this can change the meaning, but the ordering is otherwise unstable
	// and difficult to test.
	sort.Slice(e, func(i, j int) bool {
		lhs, rhs := e[i], e[j]
		if lhs.Operation != rhs.Operation {
			return lhs.Operation < rhs.Operation
		}
		return lhs.Path < rhs.Path
	})
	sort.Slice(got, func(i, j int) bool {
		lhs, rhs := got[i], got[j]
		if lhs.Operation != rhs.Operation {
			return lhs.Operation < rhs.Operation
		}
		return lhs.Path < rhs.Path
	})

	// Even though diff is useful, seeing the whole objects
	// one under another helps a lot.
	t.Logf("Got Patches:  %#v", got)
	t.Logf("Want Patches: %#v", e)
	if diff := cmp.Diff(e, got, cmpopts.EquateEmpty()); diff != "" {
		t.Logf("diff Patches: %v", diff)
		t.Errorf("expectPatches (-want, +got) = %s", diff)
	}
}

func updateAnnotationsWithUser(userU string) []jsonpatch.JsonPatchOperation {
	// Just keys is being updated, so format is iffy.
	return []jsonpatch.JsonPatchOperation{{
		Operation: "replace",
		Path:      "/metadata/annotations/testing.knative.dev~1updater",
		Value:     userU,
	}}
}

func setUserAnnotation(userC, userU string) jsonpatch.JsonPatchOperation {
	return jsonpatch.JsonPatchOperation{
		Operation: "add",
		Path:      "/metadata/annotations",
		Value: map[string]interface{}{
			CreatorAnnotation: userC,
			UpdaterAnnotation: userU,
		},
	}
}

func NewAdmissionController(client kubernetes.Interface, options ControllerOptions,
	logger *zap.SugaredLogger) (*AdmissionController, error) {
	return &AdmissionController{
		Client:                client,
		Options:               options,
		DisallowUnknownFields: true,
		// Use different versions and domains, for coverage.
		Handlers: map[schema.GroupVersionKind]GenericCRD{
			{
				Group:   "pkg.knative.dev",
				Version: "v1alpha1",
				Kind:    "Resource",
			}: &Resource{},
			{
				Group:   "pkg.knative.dev",
				Version: "v1beta1",
				Kind:    "Resource",
			}: &Resource{},
			{
				Group:   "pkg.knative.dev",
				Version: "v1alpha1",
				Kind:    "InnerDefaultResource",
			}: &InnerDefaultResource{},
			{
				Group:   "pkg.knative.io",
				Version: "v1alpha1",
				Kind:    "InnerDefaultResource",
			}: &InnerDefaultResource{},
		},
		Logger: logger,
	}, nil
}

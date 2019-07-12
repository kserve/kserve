/*
Copyright 2018 The Kubernetes Authors.

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
	"go/token"
	"reflect"
	"testing"

	admissionregistrationv1beta1 "k8s.io/api/admissionregistration/v1beta1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	webhooktypes "sigs.k8s.io/controller-runtime/pkg/webhook/types"
	"sigs.k8s.io/controller-tools/pkg/internal/general"
)

func TestParseWebhook(t *testing.T) {
	failFP := admissionregistrationv1beta1.Fail
	ignoreFP := admissionregistrationv1beta1.Ignore
	tests := []struct {
		content string
		exp     []webhook.Webhook
	}{
		{
			content: `package foo
	import (
		"fmt"
		"time"
	)

	// comment only

	// +kubebuilder:webhook:groups=apps,resources=deployments,verbs=CREATE;UPDATE
	// +kubebuilder:webhook:name=bar-webhook,path=/bar,type=mutating,failure-policy=Fail
	// bar function
	func bar() {
		fmt.Println(time.Now())
	}

	// +kubebuilder:webhook:groups=crew,versions=v1,resources=firstmates,verbs=delete
	// +kubebuilder:webhook:name=baz-webhook,path=/baz,type=validating,failure-policy=ignore
	// baz function
	func baz() {
		fmt.Println(time.Now())
	}`,
			exp: []webhook.Webhook{
				&admission.Webhook{
					Name: "bar-webhook",
					Type: webhooktypes.WebhookTypeMutating,
					Path: "/bar",
					Rules: []admissionregistrationv1beta1.RuleWithOperations{
						{
							Rule: admissionregistrationv1beta1.Rule{
								APIGroups: []string{"apps"},
								Resources: []string{"deployments"},
							},
							Operations: []admissionregistrationv1beta1.OperationType{
								admissionregistrationv1beta1.Create,
								admissionregistrationv1beta1.Update,
							},
						},
					},
					FailurePolicy: &failFP,
					Handlers:      []admission.Handler{admission.HandlerFunc(nil)},
				},
				&admission.Webhook{
					Name: "baz-webhook",
					Type: webhooktypes.WebhookTypeValidating,
					Path: "/baz",
					Rules: []admissionregistrationv1beta1.RuleWithOperations{
						{
							Rule: admissionregistrationv1beta1.Rule{
								APIGroups:   []string{"crew"},
								APIVersions: []string{"v1"},
								Resources:   []string{"firstmates"},
							},
							Operations: []admissionregistrationv1beta1.OperationType{
								admissionregistrationv1beta1.Delete,
							},
						},
					},
					FailurePolicy: &ignoreFP,
					Handlers:      []admission.Handler{admission.HandlerFunc(nil)},
				},
			},
		},
	}

	for _, test := range tests {
		o := &ManifestOptions{
			InputDir: "test.go",
			webhooks: []webhook.Webhook{},
		}
		fset := token.NewFileSet()
		err := general.ParseFile(fset, "test.go", test.content, o.parseAnnotation)
		if err != nil {
			t.Errorf("processFile should have succeeded, but got error: %v", err)
		}
		if !reflect.DeepEqual(test.exp, o.webhooks) {
			t.Errorf("webhooks should have matched, expected %#v and got %#v", test.exp, o.webhooks)
		}
	}
}

func TestParseWebhookServer(t *testing.T) {
	tests := []struct {
		content string
		exp     *webhook.ServerOptions
	}{
		{
			content: `package foo
	import (
		"fmt"
		"time"
	)

	// +kubebuilder:webhook:port=7890,cert-dir=/tmp/test-cert,service=test-system:webhook-service,selector=app:webhook-server
	// +kubebuilder:webhook:secret=test-system:webhook-secret
	// +kubebuilder:webhook:mutating-webhook-config-name=test-mutating-webhook-cfg,validating-webhook-config-name=test-validating-webhook-cfg
	// bar function
	func bar() {
		fmt.Println(time.Now())
	}`,
			exp: &webhook.ServerOptions{
				Port:    7890,
				CertDir: "/tmp/test-cert",
				BootstrapOptions: &webhook.BootstrapOptions{
					MutatingWebhookConfigName:   "test-mutating-webhook-cfg",
					ValidatingWebhookConfigName: "test-validating-webhook-cfg",
					Service: &webhook.Service{
						Namespace: "test-system",
						Name:      "webhook-service",
						Selectors: map[string]string{
							"app": "webhook-server",
						},
					},
					Secret: &types.NamespacedName{
						Namespace: "test-system",
						Name:      "webhook-secret",
					},
				},
			},
		},
	}

	for _, test := range tests {
		o := &ManifestOptions{
			InputDir: "test.go",
			svrOps:   &webhook.ServerOptions{},
		}
		fset := token.NewFileSet()
		err := general.ParseFile(fset, "test.go", test.content, o.parseAnnotation)
		if err != nil {
			t.Errorf("processFile should have succeeded, but got error: %v", err)
		}
		if !reflect.DeepEqual(test.exp, o.svrOps) {
			t.Errorf("webhook server should have matched, expected %#v and got %#v", test.exp, o.svr)
		}
	}
}

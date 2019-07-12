/*
Copyright 2019 The Knative Authors

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

package v1alpha1

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/knative/pkg/apis"
	networking "github.com/knative/serving/pkg/apis/networking"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
)

func TestServerlessServiceSpecValidation(t *testing.T) {
	tests := []struct {
		name string
		skss *ServerlessServiceSpec
		want *apis.FieldError
	}{{
		name: "valid proxy",
		skss: &ServerlessServiceSpec{
			Mode: SKSOperationModeProxy,
			ObjectRef: autoscalingv1.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       "foo",
			},
			ProtocolType: networking.ProtocolHTTP1,
		},
		want: nil,
	}, {
		name: "valid serve",
		skss: &ServerlessServiceSpec{
			Mode: SKSOperationModeServe,
			ObjectRef: autoscalingv1.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       "foo",
			},
			ProtocolType: networking.ProtocolH2C,
		},
		want: nil,
	}, {
		name: "invalid protocol",
		skss: &ServerlessServiceSpec{
			Mode: SKSOperationModeServe,
			ObjectRef: autoscalingv1.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       "foo",
			},
			ProtocolType: networking.ProtocolType("gRPC"),
		},
		want: apis.ErrInvalidValue("gRPC", "protocolType"),
	}, {
		name: "wrong mode",
		skss: &ServerlessServiceSpec{
			Mode: "bombastic",
			ObjectRef: autoscalingv1.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       "foo",
			},
			ProtocolType: networking.ProtocolH2C,
		},
		want: apis.ErrInvalidValue("bombastic", "mode"),
	}, {
		name: "no mode",
		skss: &ServerlessServiceSpec{
			Mode: "",
			ObjectRef: autoscalingv1.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       "foo",
			},
			ProtocolType: networking.ProtocolHTTP1,
		},
		want: apis.ErrMissingField("mode"),
	}, {
		name: "no object reference",
		skss: &ServerlessServiceSpec{
			Mode:         SKSOperationModeProxy,
			ProtocolType: networking.ProtocolH2C,
		},
		want: apis.ErrMissingField("objectRef"),
	}, {
		name: "empty object reference",
		skss: &ServerlessServiceSpec{
			Mode:         SKSOperationModeProxy,
			ObjectRef:    autoscalingv1.CrossVersionObjectReference{},
			ProtocolType: networking.ProtocolHTTP1,
		},
		want: apis.ErrMissingField("objectRef"),
	}, {
		name: "missing kind",
		skss: &ServerlessServiceSpec{
			Mode: SKSOperationModeProxy,
			ObjectRef: autoscalingv1.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Name:       "foo",
			},
			ProtocolType: networking.ProtocolHTTP1,
		},
		want: apis.ErrMissingField("objectRef.kind"),
	}, {
		name: "multiple errors",
		skss: &ServerlessServiceSpec{
			ObjectRef: autoscalingv1.CrossVersionObjectReference{
				Kind: "Deployment",
			},
			ProtocolType: networking.ProtocolH2C,
		},
		want: apis.ErrMissingField("mode", "objectRef.apiVersion", "objectRef.name"),
	}, {
		name: "empty spec",
		skss: &ServerlessServiceSpec{},
		want: apis.ErrMissingField(apis.CurrentField),
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.skss.Validate(context.Background())
			if diff := cmp.Diff(test.want.Error(), got.Error()); diff != "" {
				t.Errorf("Validate (-want, +got) = %v", diff)
			}
			// Now validate via parent object
			got = (&ServerlessService{
				Spec: *test.skss,
			}).Validate(context.Background())
			if diff := cmp.Diff(test.want.ViaField("spec").Error(), got.Error()); diff != "" {
				t.Errorf("Validate (-want, +got) = %v", diff)
			}
		})
	}
}

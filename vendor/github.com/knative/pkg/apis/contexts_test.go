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

package apis

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestContexts(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name  string
		ctx   context.Context
		check func(context.Context) bool
		want  bool
	}{{
		name:  "is in create",
		ctx:   WithinCreate(ctx),
		check: IsInCreate,
		want:  true,
	}, {
		name:  "not in create (bare)",
		ctx:   ctx,
		check: IsInCreate,
		want:  false,
	}, {
		name:  "not in create (update)",
		ctx:   WithinUpdate(ctx, struct{}{}),
		check: IsInCreate,
		want:  false,
	}, {
		name:  "is in update",
		ctx:   WithinUpdate(ctx, struct{}{}),
		check: IsInUpdate,
		want:  true,
	}, {
		name:  "not in update (bare)",
		ctx:   ctx,
		check: IsInUpdate,
		want:  false,
	}, {
		name:  "not in update (create)",
		ctx:   WithinCreate(ctx),
		check: IsInUpdate,
		want:  false,
	}, {
		name:  "in spec",
		ctx:   WithinSpec(ctx),
		check: IsInSpec,
		want:  true,
	}, {
		name:  "not in spec",
		ctx:   WithinStatus(ctx),
		check: IsInSpec,
		want:  false,
	}, {
		name:  "in status",
		ctx:   WithinStatus(ctx),
		check: IsInStatus,
		want:  true,
	}, {
		name:  "not in status",
		ctx:   WithinSpec(ctx),
		check: IsInStatus,
		want:  false,
	}, {
		name:  "disallow deprecated",
		ctx:   DisallowDeprecated(ctx),
		check: IsDeprecatedAllowed,
		want:  false,
	}, {
		name:  "allow deprecated",
		ctx:   ctx,
		check: IsDeprecatedAllowed,
		want:  true,
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.check(tc.ctx)
			if tc.want != got {
				t.Errorf("check() = %v, wanted %v", tc.want, got)
			}
		})
	}
}

func TestGetBaseline(t *testing.T) {
	ctx := context.Background()

	if got := GetBaseline(ctx); got != nil {
		t.Errorf("GetBaseline() = %v, wanted %v", got, nil)
	}

	var foo interface{} = "this is the object"
	ctx = WithinUpdate(ctx, foo)

	if want, got := foo, GetBaseline(ctx); got != want {
		t.Errorf("GetBaseline() = %v, wanted %v", got, want)
	}
}

func TestGetUserInfo(t *testing.T) {
	ctx := context.Background()

	if got := GetUserInfo(ctx); got != nil {
		t.Errorf("GetUserInfo() = %v, wanted %v", got, nil)
	}

	bob := &authenticationv1.UserInfo{Username: "bob"}
	ctx = WithUserInfo(ctx, bob)

	if want, got := bob, GetUserInfo(ctx); got != want {
		t.Errorf("GetUserInfo() = %v, wanted %v", got, want)
	}
}

func TestParentMeta(t *testing.T) {
	ctx := context.Background()

	if got, want := ParentMeta(ctx), (metav1.ObjectMeta{}); !cmp.Equal(want, got) {
		t.Errorf("ParentMeta() = %v, wanted %v", got, want)
	}

	want := metav1.ObjectMeta{
		Name:      "foo",
		Namespace: "bar",
	}
	ctx = WithinParent(ctx, want)

	if got := ParentMeta(ctx); !cmp.Equal(want, got) {
		t.Errorf("ParentMeta() = %v, wanted %v", got, want)
	}
}

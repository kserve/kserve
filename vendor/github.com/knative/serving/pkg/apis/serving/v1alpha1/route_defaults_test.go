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

package v1alpha1

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/knative/pkg/ptr"
	"github.com/knative/serving/pkg/apis/serving/v1beta1"
)

func TestRouteDefaulting(t *testing.T) {
	tests := []struct {
		name string
		in   *Route
		want *Route
	}{{
		name: "empty",
		in: &Route{
			Spec: RouteSpec{
				Traffic: []TrafficTarget{{
					TrafficTarget: v1beta1.TrafficTarget{
						ConfigurationName: "foo",
						Percent:           50,
					},
				}, {
					TrafficTarget: v1beta1.TrafficTarget{
						RevisionName: "foo",
						Percent:      50,
					},
				}},
			},
		},
		want: &Route{
			Spec: RouteSpec{
				Traffic: []TrafficTarget{{
					TrafficTarget: v1beta1.TrafficTarget{
						ConfigurationName: "foo",
						Percent:           50,
						LatestRevision:    ptr.Bool(true),
					},
				}, {
					TrafficTarget: v1beta1.TrafficTarget{
						RevisionName:   "foo",
						Percent:        50,
						LatestRevision: ptr.Bool(false),
					},
				}},
			},
		},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.in
			got.SetDefaults(context.Background())
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("SetDefaults (-want, +got) = %v", diff)
			}
		})
	}
}

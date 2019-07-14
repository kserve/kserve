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

package kmeta

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"k8s.io/client-go/tools/cache"

	. "github.com/knative/pkg/testing"
)

// Ensure our resource satisfies the interface.
var _ Accessor = (*Resource)(nil)

func TestAccessor(t *testing.T) {
	goodResource := &Resource{}

	tests := []struct {
		name string
		o    interface{}
		want Accessor
	}{{
		name: "bad object returns error",
		o:    struct{}{},
	}, {
		name: "deleted with bad final state",
		o: cache.DeletedFinalStateUnknown{
			Obj: struct{}{},
		},
	}, {
		name: "good object",
		o:    goodResource,
		want: Accessor(goodResource),
	}, {
		name: "deleted with good final state",
		o: cache.DeletedFinalStateUnknown{
			Obj: goodResource,
		},
		want: Accessor(goodResource),
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.want == nil {
				got, err := DeletionHandlingAccessor(test.o)
				if err == nil {
					t.Errorf("DeletionHandlingAccessor() = %v, wanted error", got)
				}
			} else {
				got, err := DeletionHandlingAccessor(test.o)
				if err != nil {
					t.Errorf("DeletionHandlingAccessor() = %v", err)
				}
				if diff := cmp.Diff(got, test.want); diff != "" {
					t.Errorf("DeletionHandlingAccessor = %s", diff)
				}
			}
		})
	}
}

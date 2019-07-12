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

package rbac

import (
	"go/token"
	"reflect"
	"testing"

	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/controller-tools/pkg/internal/general"
)

func TestParseFile(t *testing.T) {
	tests := []struct {
		content string
		exp     []rbacv1.PolicyRule
	}{
		{
			content: `package foo
	import (
		"fmt"
		"time"
	)

	// RBAC annotation with kubebuilder prefix
	// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;delete
	// bar function
	func bar() {
		fmt.Println(time.Now())
	}`,
			exp: []rbacv1.PolicyRule{{
				Verbs:     []string{"get", "list", "watch", "delete"},
				APIGroups: []string{"apps"},
				Resources: []string{"deployments"},
			}},
		},
		{
			content: `package foo
	import (
		"fmt"
		"time"
	)

	// RBAC annotation without kuebuilder prefix
	// +rbac:groups=apps,resources=deployments,verbs=get;list;watch;delete
	// bar function
	func bar() {
		fmt.Println(time.Now())
	}`,
			exp: []rbacv1.PolicyRule{{
				Verbs:     []string{"get", "list", "watch", "delete"},
				APIGroups: []string{"apps"},
				Resources: []string{"deployments"},
			}},
		},
		{
			content: `package foo
	import (
		"fmt"
		"time"
	)

	// RBAC annotation with kubebuilder prefix
	// +kubebuilder:rbac:groups=,resources=pods,verbs=get;list;watch;delete
	// bar function
	func bar() {
		fmt.Println(time.Now())
	}`,
			exp: []rbacv1.PolicyRule{{
				Verbs:     []string{"get", "list", "watch", "delete"},
				APIGroups: []string{""},
				Resources: []string{"pods"},
			}},
		},
		{
			content: `package foo
	import (
		"fmt"
		"time"
	)

	// RBAC annotation with kubebuilder prefix
	// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;delete
	// bar function
	func bar() {
		fmt.Println(time.Now())
	}`,
			exp: []rbacv1.PolicyRule{{
				Verbs:     []string{"get", "list", "watch", "delete"},
				APIGroups: []string{""},
				Resources: []string{"pods"},
			}},
		},
	}

	for _, test := range tests {
		fset := token.NewFileSet()
		ops := parserOptions{
			rules: []rbacv1.PolicyRule{},
		}
		err := general.ParseFile(fset, "test.go", test.content, ops.parseAnnotation)
		if err != nil {
			t.Errorf("processFile should have succeeded, but got error: %v", err)
		}
		if !reflect.DeepEqual(ops.rules, test.exp) {
			t.Errorf("RBAC rules should have matched, expected %v and got %v", test.exp, ops.rules)
		}
	}
}

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

package names

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/knative/serving/pkg/apis/serving/v1alpha1"
)

func TestNamer(t *testing.T) {
	tests := []struct {
		name string
		rev  *v1alpha1.Revision
		f    func(*v1alpha1.Revision) string
		want string
	}{{
		name: "Deployment",
		rev: &v1alpha1.Revision{
			ObjectMeta: metav1.ObjectMeta{
				Name: "foo",
			},
		},
		f:    Deployment,
		want: "foo-deployment",
	}, {
		name: "ImageCache",
		rev: &v1alpha1.Revision{
			ObjectMeta: metav1.ObjectMeta{
				Name: "foo",
			},
		},
		f:    ImageCache,
		want: "foo-cache",
	}, {
		name: "KPA",
		rev: &v1alpha1.Revision{
			ObjectMeta: metav1.ObjectMeta{
				Name: "baz",
			},
		},
		f:    KPA,
		want: "baz",
	}, {
		name: "K8sService",
		rev: &v1alpha1.Revision{
			ObjectMeta: metav1.ObjectMeta{
				Name: "blah",
			},
		},
		f:    K8sService,
		want: "blah-service",
	}, {
		name: "FluentdConfigMap",
		rev: &v1alpha1.Revision{
			ObjectMeta: metav1.ObjectMeta{
				Name: "bazinga",
			},
		},
		f:    FluentdConfigMap,
		want: "bazinga-fluentd",
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.f(test.rev)
			if got != test.want {
				t.Errorf("%s() = %v, wanted %v", test.name, got, test.want)
			}
		})
	}
}

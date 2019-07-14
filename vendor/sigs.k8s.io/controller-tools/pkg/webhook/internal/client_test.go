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

package internal

import (
	"context"
	"reflect"
	"testing"

	"github.com/spf13/afero"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestGet(t *testing.T) {
	type testcase struct {
		name        string
		path        string
		content     []byte
		expected    runtime.Object
		expectedErr error
	}

	testcases := []testcase{
		{
			name:        "file doesn't exist",
			path:        "foo/bar.yaml",
			content:     nil,
			expectedErr: apierrors.NewNotFound(schema.GroupResource{}, "test-cm"),
		},
		{
			name: "empty file",
			path: "foo/bar.yaml",
			content: []byte(`
		`),
			expectedErr: apierrors.NewNotFound(schema.GroupResource{}, "test-cm"),
		},
		{
			name: "file exists",
			path: "foo/bar.yaml",
			content: []byte(`apiVersion: v1
data:
  foo: bar
kind: ConfigMap
metadata:
  name: test-cm
  namespace: test-ns
`),
			expected: &corev1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "ConfigMap",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-ns",
					Name:      "test-cm",
				},
				Data: map[string]string{
					"foo": "bar",
				},
			},
		},
		{
			name: "multiple objects exist in file",
			path: "foo/bar.yaml",
			content: []byte(`apiVersion: v1
data:
  foo: bar
kind: ConfigMap
metadata:
  name: test-cm
  namespace: test-ns
---
apiVersion: v1
kind: Pod
metadata:
  name: test-pod
  namespace: test-ns
spec:
  containers:
    - name: nginx
      image: nginx
`),
			expected: &corev1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "ConfigMap",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-ns",
					Name:      "test-cm",
				},
				Data: map[string]string{
					"foo": "bar",
				},
			},
		},
	}

	for _, ts := range testcases {
		appFS := afero.NewMemMapFs()
		afero.WriteFile(appFS, ts.path, ts.content, 0644)

		c := manifestClient{ManifestFile: ts.path, fs: appFS}
		actual := &corev1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "v1",
				Kind:       "ConfigMap",
			},
		}
		err := c.Get(context.Background(), client.ObjectKey{Namespace: "test-ns", Name: "test-cm"}, actual)
		var actualObj runtime.Object = actual
		switch {
		case ts.expectedErr != nil:
			if !reflect.DeepEqual(err, ts.expectedErr) {
				t.Errorf("in test case: %q, expect error %v, but got error %v", ts.name, ts.expectedErr, err)
			}
			continue
		case err != nil && ts.expectedErr == nil:
			t.Errorf("in test case: %q, unexpected error: %v", ts.name, err)
			continue
		}

		if !reflect.DeepEqual(ts.expected, actualObj) {
			t.Errorf("in test case: %q, expect %#v, but got %#v", ts.name, ts.expected, actualObj)
		}
	}
}

func TestCreate(t *testing.T) {
	type testcase struct {
		name        string
		path        string
		content     []byte
		object      runtime.Object
		expected    []byte
		expectedErr error
	}

	testcases := []testcase{
		{
			name:    "file doesn't exist",
			path:    "foo/bar.yaml",
			content: nil,
			object: &corev1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "ConfigMap",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-ns",
					Name:      "test-cm",
				},
				Data: map[string]string{
					"foo": "bar",
				},
			},
			expected: []byte(`apiVersion: v1
data:
  foo: bar
kind: ConfigMap
metadata:
  creationTimestamp: null
  name: test-cm
  namespace: test-ns
`),
		},
		{
			name: "empty file",
			path: "foo/bar.yaml",
			content: []byte(`
		`),
			object: &corev1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "ConfigMap",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-ns",
					Name:      "test-cm",
				},
				Data: map[string]string{
					"foo": "bar",
				},
			},
			expected: []byte(`apiVersion: v1
data:
  foo: bar
kind: ConfigMap
metadata:
  creationTimestamp: null
  name: test-cm
  namespace: test-ns
`),
		},
		{
			name: "object doesn't exist in file",
			path: "foo/bar.yaml",
			content: []byte(`apiVersion: v1
kind: Pod
metadata:
  name: test-pod
  namespace: test-ns
spec:
  containers:
    - name: nginx
      image: nginx
`),
			object: &corev1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "ConfigMap",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-ns",
					Name:      "test-cm",
				},
				Data: map[string]string{
					"foo": "bar",
				},
			},
			expected: []byte(`apiVersion: v1
data:
  foo: bar
kind: ConfigMap
metadata:
  creationTimestamp: null
  name: test-cm
  namespace: test-ns
---
apiVersion: v1
kind: Pod
metadata:
  name: test-pod
  namespace: test-ns
spec:
  containers:
    - name: nginx
      image: nginx
`),
		},
		{
			name: "object exists",
			path: "foo/bar.yaml",
			content: []byte(`apiVersion: v1
kind: ConfigMap
metadata:
 name: test-cm
 namespace: test-ns
`),
			object: &corev1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "ConfigMap",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-ns",
					Name:      "test-cm",
				},
				Data: map[string]string{
					"foo": "bar",
				},
			},
			expectedErr: apierrors.NewAlreadyExists(schema.GroupResource{}, "test-cm"),
		},
	}

	for _, ts := range testcases {
		appFS := afero.NewMemMapFs()
		afero.WriteFile(appFS, ts.path, ts.content, 0644)

		c := manifestClient{ManifestFile: ts.path, fs: appFS}
		err := c.Create(context.Background(), ts.object)
		switch {
		case ts.expectedErr != nil:
			if !reflect.DeepEqual(err, ts.expectedErr) {
				t.Errorf("in test case: %q, expect error %v, but got error %v", ts.name, ts.expectedErr, err)
			}
			continue
		case err != nil && ts.expectedErr == nil:
			t.Errorf("in test case: %q, unexpected error: %v", ts.name, err)
			continue
		}

		actual, err := afero.ReadFile(appFS, ts.path)
		if err != nil {
			t.Errorf("in test case: %q, unexpected error: %v", ts.name, err)
		}
		if !reflect.DeepEqual(ts.expected, actual) {
			t.Errorf("in test case: %q, expect %s, but got %s", ts.name, ts.expected, actual)
		}
	}
}
func TestUpdate(t *testing.T) {
	type testcase struct {
		name        string
		path        string
		content     []byte
		object      runtime.Object
		expected    []byte
		expectedErr error
	}

	testcases := []testcase{
		{
			name:    "file doesn't exist",
			path:    "foo/bar.yaml",
			content: nil,
			object: &corev1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "ConfigMap",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-ns",
					Name:      "test-cm",
				},
				Data: map[string]string{
					"foo": "bar",
				},
			},
			expected: []byte(`apiVersion: v1
data:
  foo: bar
kind: ConfigMap
metadata:
  creationTimestamp: null
  name: test-cm
  namespace: test-ns
`),
		},
		{
			name: "empty file",
			path: "foo/bar.yaml",
			content: []byte(`
		`),
			object: &corev1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "ConfigMap",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-ns",
					Name:      "test-cm",
				},
				Data: map[string]string{
					"foo": "bar",
				},
			},
			expected: []byte(`apiVersion: v1
data:
  foo: bar
kind: ConfigMap
metadata:
  creationTimestamp: null
  name: test-cm
  namespace: test-ns
`),
		},
		{
			name: "object doesn't exist in file",
			path: "foo/bar.yaml",
			content: []byte(`apiVersion: v1
kind: Pod
metadata:
  name: test-pod
  namespace: test-ns
spec:
  containers:
    - name: nginx
      image: nginx
`),
			object: &corev1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "ConfigMap",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-ns",
					Name:      "test-cm",
				},
				Data: map[string]string{
					"foo": "bar",
				},
			},
			expected: []byte(`apiVersion: v1
data:
  foo: bar
kind: ConfigMap
metadata:
  creationTimestamp: null
  name: test-cm
  namespace: test-ns
---
apiVersion: v1
kind: Pod
metadata:
  name: test-pod
  namespace: test-ns
spec:
  containers:
    - name: nginx
      image: nginx
`),
		},
		{
			name: "object exists",
			path: "foo/bar.yaml",
			content: []byte(`apiVersion: v1
kind: ConfigMap
metadata:
 name: test-cm
 namespace: test-ns
`),
			object: &corev1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "ConfigMap",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-ns",
					Name:      "test-cm",
				},
				Data: map[string]string{
					"foo": "bar",
				},
			},
			expected: []byte(`apiVersion: v1
data:
  foo: bar
kind: ConfigMap
metadata:
  creationTimestamp: null
  name: test-cm
  namespace: test-ns
`),
		},
		{
			name: "object exists along with other objects",
			path: "foo/bar.yaml",
			content: []byte(`apiVersion: v1
kind: ConfigMap
metadata:
 name: test-cm
 namespace: test-ns
---
apiVersion: v1
kind: Pod
metadata:
  name: test-pod
  namespace: test-ns
spec:
  containers:
    - name: nginx
      image: nginx
`),
			object: &corev1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "ConfigMap",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-ns",
					Name:      "test-cm",
				},
				Data: map[string]string{
					"foo": "bar",
				},
			},
			expected: []byte(`apiVersion: v1
data:
  foo: bar
kind: ConfigMap
metadata:
  creationTimestamp: null
  name: test-cm
  namespace: test-ns
---
apiVersion: v1
kind: Pod
metadata:
  name: test-pod
  namespace: test-ns
spec:
  containers:
    - name: nginx
      image: nginx
`),
		},
	}

	for _, ts := range testcases {
		appFS := afero.NewMemMapFs()
		afero.WriteFile(appFS, ts.path, ts.content, 0644)

		c := manifestClient{ManifestFile: ts.path, fs: appFS}
		err := c.Update(context.Background(), ts.object)
		switch {
		case ts.expectedErr != nil:
			if !reflect.DeepEqual(err, ts.expectedErr) {
				t.Errorf("in test case: %q, expect error %v, but got error %v", ts.name, ts.expectedErr, err)
			}
			continue
		case err != nil && ts.expectedErr == nil:
			t.Errorf("in test case: %q, unexpected error: %v", ts.name, err)
			continue
		}

		actual, err := afero.ReadFile(appFS, ts.path)
		if err != nil {
			t.Errorf("in test case: %q, unexpected error: %v", ts.name, err)
		}
		if !reflect.DeepEqual(ts.expected, actual) {
			t.Errorf("in test case: %q, expect %s, but got %s", ts.name, ts.expected, actual)
		}
	}
}

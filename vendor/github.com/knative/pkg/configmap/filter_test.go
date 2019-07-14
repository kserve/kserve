/*
Copyright 2018 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package configmap

import "testing"

type foo struct{}
type bar struct{}

func TestTypeFilter(t *testing.T) {
	count := 0

	var f = func(name string, value interface{}) {
		count++
	}

	f("foo", &foo{})
	f("bar", &bar{})

	if want, got := 2, count; want != got {
		t.Fatalf("plain call: count: want %v, got %v", want, got)
	}

	filtered := TypeFilter(&foo{})(f)

	filtered("foo", &foo{})
	filtered("bar", &bar{})

	if want, got := 3, count; want != got {
		t.Fatalf("filtered call: count: want %v, got %v", want, got)
	}
}

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

package helpers

import (
	"fmt"
	"regexp"
)

var matcher = regexp.MustCompile("abcd-[a-z]{8}")

func ExampleAppendRandomString() {
	const s = "abcd"
	t := AppendRandomString(s)
	o := AppendRandomString(s)
	fmt.Println(matcher.MatchString(t), matcher.MatchString(o), o != t)
	// Output: true true true
}

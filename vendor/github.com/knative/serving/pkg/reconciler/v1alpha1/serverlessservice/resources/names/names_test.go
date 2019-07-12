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

package names

import (
	"testing"
)

func TestServiceNames(t *testing.T) {
	if got, want := PublicService("test-name"), "test-name-pub"; got != want {
		t.Errorf("PublicService name = %s, want: %s", got, want)
	}
	if got, want := PrivateService("test-name"), "test-name-priv"; got != want {
		t.Errorf("PrivateService name = %s, want: %s", got, want)
	}
}

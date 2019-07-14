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

package network

import (
	"strings"
	"testing"
)

func TestGetDomainName(t *testing.T) {
	tests := []struct {
		name       string
		resolvConf string
		want       string
	}{
		{
			name: "all good",
			resolvConf: `
nameserver 1.1.1.1
search default.svc.abc.com svc.abc.com abc.com
options ndots:5
`,
			want: "abc.com",
		},
		{
			name: "missing search line",
			resolvConf: `
nameserver 1.1.1.1
options ndots:5
`,
			want: defaultDomainName,
		},
		{
			name: "non k8s resolv.conf format",
			resolvConf: `
nameserver 1.1.1.1
search  abc.com xyz.com
options ndots:5
`,
			want: defaultDomainName,
		},
	}
	for _, tt := range tests {
		got := getClusterDomainName(strings.NewReader(tt.resolvConf))
		if got != tt.want {
			t.Errorf("Test %s failed expected: %s but got: %s", tt.name, tt.want, got)
		}
	}
}

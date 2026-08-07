/*
Copyright 2025 The KServe Authors.

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

package types

import "testing"

func TestResolveOciModelMode(t *testing.T) {
	cases := []struct {
		name string
		cfg  StorageInitializerConfig
		want string
	}{
		{
			name: "explicit OciModelMode wins over everything",
			cfg:  StorageInitializerConfig{OciModelMode: OciModelModeNative, EnableOciImageSource: true},
			want: OciModelModeNative,
		},
		{
			name: "OciModelMode fetch",
			cfg:  StorageInitializerConfig{OciModelMode: OciModelModeFetch},
			want: OciModelModeFetch,
		},
		{
			name: "backcompat: EnableOciImageSource true resolves to modelcar",
			cfg:  StorageInitializerConfig{EnableOciImageSource: true},
			want: OciModelModeModelcar,
		},
		{
			name: "backcompat: EnableOciModelSupport true resolves to modelcar",
			cfg:  StorageInitializerConfig{EnableOciModelSupport: true},
			want: OciModelModeModelcar,
		},
		{
			name: "neither set returns empty (disabled)",
			cfg:  StorageInitializerConfig{},
			want: "",
		},
		{
			name: "both OCI flags false, no mode: disabled",
			cfg:  StorageInitializerConfig{EnableOciImageSource: false, EnableOciModelSupport: false},
			want: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveOciModelMode(&tc.cfg)
			if got != tc.want {
				t.Errorf("ResolveOciModelMode() = %q, want %q", got, tc.want)
			}
		})
	}
}

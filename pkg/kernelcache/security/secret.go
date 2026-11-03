/*
Copyright 2026 The KServe Authors.

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

package security

import "context"

// SecretSource loads the key and trust material that verifiers and signers need
// (CA bundles, signing keys, trusted roots). It is a plain data fetcher so
// parsing stays with each mode and adding a mode never changes this interface.
// References use the "namespace/name" form.
//
// A Kubernetes client-backed implementation is added with the wiring changes.
type SecretSource interface {
	GetSecret(ctx context.Context, ref string) (map[string][]byte, error)
	GetConfigMap(ctx context.Context, ref string) (map[string]string, error)
}

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

package credentials

// DeepCopyInto copies all fields of CredentialConfig into out. in must be non-nil.
func (in *CredentialConfig) DeepCopyInto(out *CredentialConfig) {
	*out = *in
}

// DeepCopy creates a new CredentialConfig by deep copying the receiver.
func (in *CredentialConfig) DeepCopy() *CredentialConfig {
	if in == nil {
		return nil
	}
	out := new(CredentialConfig)
	in.DeepCopyInto(out)
	return out
}

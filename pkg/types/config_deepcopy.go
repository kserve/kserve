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

// DeepCopyInto copies all fields of StorageInitializerConfig into out. in must be non-nil.
func (in *StorageInitializerConfig) DeepCopyInto(out *StorageInitializerConfig) {
	*out = *in
	if in.UidModelcar != nil {
		in, out := &in.UidModelcar, &out.UidModelcar
		*out = new(int64)
		**out = **in
	}
}

// DeepCopy creates a new StorageInitializerConfig by deep copying the receiver.
func (in *StorageInitializerConfig) DeepCopy() *StorageInitializerConfig {
	if in == nil {
		return nil
	}
	out := new(StorageInitializerConfig)
	in.DeepCopyInto(out)
	return out
}

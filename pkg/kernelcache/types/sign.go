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

package types

// SignRequest describes the image to sign.
type SignRequest struct {
	// ImageRef is the image to sign, by tag or digest.
	ImageRef string
}

// SignResult reports the outcome of a signing operation.
type SignResult struct {
	// Digest is the resolved image digest the signature was bound to.
	Digest string

	// Mode is the signing mode that produced the result.
	Mode Mode
}

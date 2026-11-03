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

package imgbuild

import "time"

const (
	CreateStateSucceeded = "Succeeded"
	CreateStateUnchanged = "Unchanged"
)

// CreateResult contains the facts produced by an OCI capture.
type CreateResult struct {
	State          string    `json:"state"`
	ImageReference string    `json:"imageReference"`
	CacheSizeBytes int64     `json:"cacheSizeBytes"`
	CompletedAt    time.Time `json:"completedAt"`
}

// ResultImageBuilder is implemented by builders that can return an immutable artifact reference.
type ResultImageBuilder interface {
	CreateImageWithResult(imageName, cacheDir string) (*CreateResult, error)
}

// DeltaImageBuilder creates an image from directories added after a snapshot.
type DeltaImageBuilder interface {
	CreateDeltaImageWithResult(imageName, cacheDir, snapshotPath string) (*CreateResult, error)
}

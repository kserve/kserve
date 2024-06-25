/*
Copyright 2021 The KServe Authors.

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

package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"

	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

type OCIProvider struct {
	Client *http.Client
}

func (m *OCIProvider) DownloadModel(modelDir string, modelName string, storageUri string) error {
	log.Info("Download model ", "modelName", modelName, "storageUri", storageUri, "modelDir", modelDir)
	uri, err := url.Parse(storageUri)
	if err != nil {
		return fmt.Errorf("unable to parse storage uri: %w", err)
	}
	OCIDownloader := &OCIDownloader{
		StorageUri: storageUri,
		ModelDir:   modelDir,
		ModelName:  modelName,
		Uri:        uri,
	}
	if err := OCIDownloader.Download(*m.Client); err != nil {
		return err
	}
	return nil
}

type OCIDownloader struct {
	StorageUri string
	ModelDir   string
	ModelName  string
	Uri        *url.URL
}

// generateContentKey generates a unique key for each content descriptor, using
// its digest and name if applicable.
func generateContentKey(desc ocispec.Descriptor) string {
	return desc.Digest.String() + desc.Annotations[ocispec.AnnotationTitle]
}

func (h *OCIDownloader) Download(client http.Client) error {
	// Copy Options
	var printed sync.Map
	copyOptions := oras.DefaultCopyOptions

	var getConfigOnce sync.Once
	copyOptions.FindSuccessors = func(ctx context.Context, fetcher content.Fetcher, desc ocispec.Descriptor) ([]ocispec.Descriptor, error) {
		statusFetcher := content.FetcherFunc(func(ctx context.Context, target ocispec.Descriptor) (fetched io.ReadCloser, fetchErr error) {
			if _, ok := printed.LoadOrStore(generateContentKey(target), true); ok {
				return fetcher.Fetch(ctx, target)
			}

			/*
				// print status log for first-time fetching
				if err := display.PrintStatus(target, "Downloading", opts.Verbose); err != nil {
					return nil, err
				}
			*/
			rc, err := fetcher.Fetch(ctx, target)
			if err != nil {
				return nil, err
			}
			defer func() {
				if fetchErr != nil {
					rc.Close()
				}
			}()
			return rc, nil
		})

		nodes, subject, config, err := Successors(ctx, statusFetcher, desc)
		if err != nil {
			return nil, err
		}
		if subject != nil {
			nodes = append(nodes, *subject)
		}
		/*
			if config != nil {
				getConfigOnce.Do(func() {
					if configPath != "" && (configMediaType == "" || config.MediaType == configMediaType) {
						if config.Annotations == nil {
							config.Annotations = make(map[string]string)
						}
						config.Annotations[ocispec.AnnotationTitle] = configPath
					}
				})
				nodes = append(nodes, *config)
			}
		*/

		var ret []ocispec.Descriptor
		for _, s := range nodes {
			if s.Annotations[ocispec.AnnotationTitle] == "" {
				ss, err := content.Successors(ctx, fetcher, s)
				if err != nil {
					return nil, err
				}
				if len(ss) == 0 {
					/*
						// skip s if it is unnamed AND has no successors.
						if err := printOnce(&printed, s, "Skipped    ", opts.Verbose); err != nil {
							return nil, err
						}
					*/
					continue
				}
			}
			ret = append(ret, s)
		}

		return ret, nil
	}

	return nil
}

// MediaTypeArtifactManifest specifies the media type for a content descriptor.
const MediaTypeArtifactManifest = "application/vnd.oci.artifact.manifest.v1+json"

// Artifact describes an artifact manifest.
// This structure provides `application/vnd.oci.artifact.manifest.v1+json` mediatype when marshalled to JSON.
//
// This manifest type was introduced in image-spec v1.1.0-rc1 and was removed in
// image-spec v1.1.0-rc3. It is not part of the current image-spec and is kept
// here for Go compatibility.
//
// Reference: https://github.com/opencontainers/image-spec/pull/999
type Artifact struct {
	// MediaType is the media type of the object this schema refers to.
	MediaType string `json:"mediaType"`

	// ArtifactType is the IANA media type of the artifact this schema refers to.
	ArtifactType string `json:"artifactType"`

	// Blobs is a collection of blobs referenced by this manifest.
	Blobs []ocispec.Descriptor `json:"blobs,omitempty"`

	// Subject (reference) is an optional link from the artifact to another manifest forming an association between the artifact and the other manifest.
	Subject *ocispec.Descriptor `json:"subject,omitempty"`

	// Annotations contains arbitrary metadata for the artifact manifest.
	Annotations map[string]string `json:"annotations,omitempty"`
}

// Successors returns the nodes directly pointed by the current node, picking
// out subject and config descriptor if applicable.
// Returning nil when no subject and config found.
func Successors(ctx context.Context, fetcher content.Fetcher, node ocispec.Descriptor) (nodes []ocispec.Descriptor, subject, config *ocispec.Descriptor, err error) {
	switch node.MediaType {
	case ocispec.MediaTypeImageManifest:
		var fetched []byte
		fetched, err = content.FetchAll(ctx, fetcher, node)
		if err != nil {
			return
		}
		var manifest ocispec.Manifest
		if err = json.Unmarshal(fetched, &manifest); err != nil {
			return
		}
		nodes = manifest.Layers
		subject = manifest.Subject
		config = &manifest.Config
	case MediaTypeArtifactManifest:
		var fetched []byte
		fetched, err = content.FetchAll(ctx, fetcher, node)
		if err != nil {
			return
		}
		var manifest Artifact
		if err = json.Unmarshal(fetched, &manifest); err != nil {
			return
		}
		nodes = manifest.Blobs
		subject = manifest.Subject
	case ocispec.MediaTypeImageIndex:
		var fetched []byte
		fetched, err = content.FetchAll(ctx, fetcher, node)
		if err != nil {
			return
		}
		var index ocispec.Index
		if err = json.Unmarshal(fetched, &index); err != nil {
			return
		}
		nodes = index.Manifests
		subject = index.Subject
	default:
		nodes, err = content.Successors(ctx, fetcher, node)
	}
	return
}

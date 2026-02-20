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

package logger

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"

	"go.uber.org/zap"

	"github.com/kserve/kserve/pkg/agent/storage"
)

type StorageStrategy string

const (
	S3Storage    StorageStrategy = "s3"
	GCSStorage   StorageStrategy = "gcs"
	AzureStorage StorageStrategy = "abfs"
	HttpStorage  StorageStrategy = "http"
)

const (
	S3Prefix    string = "s3"
	GCSPrefix   string = "gs"
	AzurePrefix string = "abfs"
)

const DefaultStorage = HttpStorage

func GetStorageStrategy(url string) StorageStrategy {
	// http, https
	switch {
	case strings.HasPrefix(url, "http"): // http, https
		return HttpStorage

	case strings.HasPrefix(url, "s3"): // s3, s3a
		return S3Storage
	case strings.HasPrefix(url, "gs"): // gcs
		return GCSStorage
	case strings.HasPrefix(url, "abfs"):
		return AzureStorage
	default:
		return DefaultStorage
	}
}

// MarshalResponse contains the marshalled output for a batch.
type MarshalResponse struct {
	Data      []byte
	Extension string
}

// Marshaller transforms a batch of log requests into marshalled bytes for storage.
type Marshaller interface {
	Marshal(batch []LogRequest) (*MarshalResponse, error)
}

// BatchStrategy accumulates individual log requests and emits batches.
// Run reads from in, batches records according to its policy, and writes
// batches to out. Run MUST close out when in is closed and all remaining
// records have been flushed. Run MUST respect ctx cancellation.
type BatchStrategy interface {
	Run(ctx context.Context, in <-chan LogRequest, out chan<- []LogRequest)
}

type Store interface {
	Store(logUrl *url.URL, batch []LogRequest) error
}

type BlobStore struct {
	storePath  string
	log        *zap.SugaredLogger
	marshaller Marshaller
	provider   storage.Provider
}

var _ Store = &BlobStore{}

func NewBlobStore(logStorePath string, marshaller Marshaller, provider storage.Provider, log *zap.SugaredLogger) *BlobStore {
	return &BlobStore{
		storePath:  logStorePath,
		marshaller: marshaller,
		log:        log,
		provider:   provider,
	}
}

func NewStoreForScheme(scheme string, logStorePath string, marshaller Marshaller, log *zap.SugaredLogger) (Store, error) {
	// Convert to a Protocol to reuse existing types
	if !strings.HasSuffix(scheme, "://") {
		scheme += "://"
	}
	protocol := storage.Protocol(scheme)
	provider, err := storage.GetProvider(map[storage.Protocol]storage.Provider{}, protocol)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage provider: %w", err)
	}
	switch protocol {
	case storage.AZURE:
		fallthrough
	case storage.GCS:
		fallthrough
	case storage.S3:
		return NewBlobStore(logStorePath, marshaller, provider, log), nil
	}
	return nil, fmt.Errorf("unsupported protocol %s", protocol)
}

func (s *BlobStore) Store(logUrl *url.URL, batch []LogRequest) error {
	if logUrl == nil {
		return errors.New("log url is invalid")
	}
	if len(batch) == 0 {
		return errors.New("empty batch")
	}

	response, err := s.marshaller.Marshal(batch)
	if err != nil {
		s.log.Error(err)
		return err
	}

	bucket, configPrefix, err := parseBlobStoreURL(logUrl.String(), s.log)
	if err != nil {
		s.log.Error(err)
		return err
	}

	if bucket == "" {
		return errors.New("no bucket specified in url")
	}

	// Use the first record for object key generation (prefix, id, type).
	objectKey, err := s.getObjectKey(configPrefix, &batch[0], response.Extension)
	if err != nil {
		s.log.Error(err)
		return err
	}

	err = s.provider.UploadObject(bucket, objectKey, response.Data)
	if err != nil {
		s.log.Error(err)
		return err
	}
	s.log.Info("Successfully uploaded object")
	return nil
}

func (s *BlobStore) getObjectPrefix(configPrefix string, request *LogRequest) (string, error) {
	if request == nil {
		return "", errors.New("log request is invalid")
	}

	var parts []string
	if configPrefix != "" {
		parts = append(parts, configPrefix)
	}
	if request.Namespace != "" {
		parts = append(parts, request.Namespace)
	}
	if request.InferenceService != "" {
		parts = append(parts, request.InferenceService)
	}
	if request.Component != "" {
		parts = append(parts, request.Component)
	}

	if s.storePath != "" {
		parts = append(parts, s.storePath)
	}
	return path.Join(parts...), nil
}

func (s *BlobStore) getObjectKey(configPrefix string, request *LogRequest, extension string) (string, error) {
	if request == nil {
		return "", errors.New("log request is invalid")
	}

	prefix, err := s.getObjectPrefix(configPrefix, request)
	if err != nil {
		return "", err
	}

	typeEnd := strings.LastIndex(request.ReqType, ".")
	if typeEnd == -1 {
		return "", fmt.Errorf("invalid request type: %s", request.ReqType)
	}

	reqType := request.ReqType[typeEnd+1:]

	return fmt.Sprintf("%s/%s-%s.%s", prefix, request.Id, reqType, extension), nil
}

func isValidScheme(scheme string) bool {
	return strings.HasPrefix(scheme, S3Prefix) || strings.HasPrefix(scheme, GCSPrefix) || strings.HasPrefix(scheme, AzurePrefix)
}

func parseBlobStoreURL(blobStoreUrl string, log *zap.SugaredLogger) (bucket, key string, err error) {
	u, err := url.Parse(blobStoreUrl)
	if err != nil {
		return "", "", err
	}

	if !isValidScheme(u.Scheme) {
		return "", "", fmt.Errorf("invalid scheme: %q", u.Scheme)
	}

	bucket = u.Host
	if u.User != nil {
		// azure URLs follow the format https://user@host/path/to/file where user is the bucket name.
		bucket = u.User.Username()
	}
	// u.Path starts with a "/" so trim it off.
	key = strings.TrimPrefix(u.Path, "/")
	log.Debugf("Returning bucket: %s", bucket)
	return bucket, key, nil
}

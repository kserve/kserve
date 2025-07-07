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
	"encoding/json"
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
	S3Storage   StorageStrategy = "s3"
	GCSStorage  StorageStrategy = "gcs"
	HttpStorage StorageStrategy = "http"
)

const DefaultStorage = HttpStorage

func GetStorageStrategy(url string) StorageStrategy {
	// http, https
	switch {
	case strings.HasPrefix(url, "http"): // http, https
		return HttpStorage

	case strings.HasPrefix(url, "s3"): // s3, s3a
		return S3Storage

	default:
		return DefaultStorage
	}
}

type Marshaller interface {
	Marshal(v interface{}) ([]byte, error)
}

type JSONMarshaller struct{}

func (j *JSONMarshaller) Marshal(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

func getMarshaller(format string) (Marshaller, error) {
	switch format {
	case "json":
		return &JSONMarshaller{}, nil
	default:
		return nil, fmt.Errorf("unsupported format %s", format)
	}
}

type Store interface {
	Store(logUrl *url.URL, logRequest LogRequest) error
}

type BlobStore struct {
	storePath   string
	storeFormat string
	log         *zap.SugaredLogger
	marshaller  Marshaller
	provider    storage.Provider
}

var _ Store = &BlobStore{}

func NewBlobStore(logStorePath string, logStoreFormat string, marshaller Marshaller, provider storage.Provider, log *zap.SugaredLogger) *BlobStore {
	return &BlobStore{
		storePath:   logStorePath,
		storeFormat: logStoreFormat,
		marshaller:  marshaller,
		log:         log,
		provider:    provider,
	}
}

func NewStoreForScheme(scheme string, logStorePath string, logStoreFormat string, log *zap.SugaredLogger) (Store, error) {
	if logStoreFormat == "" {
		logStoreFormat = "json"
	}
	marshaller, err := getMarshaller(logStoreFormat)
	if err != nil {
		return nil, err
	}

	// Convert to a Protocol to reuse existing types
	if !strings.HasSuffix(scheme, "://") {
		scheme += "://"
	}
	protocol := storage.Protocol(scheme)
	provider, err := storage.GetProvider(map[storage.Protocol]storage.Provider{}, protocol)
	if err != nil {
		return nil, fmt.Errorf("failed to create S3 provider: %w", err)
	}
	if protocol == storage.S3 {
		return NewBlobStore(logStorePath, logStoreFormat, marshaller, provider, log), nil
	}
	return nil, fmt.Errorf("unsupported protocol %s", protocol)
}

func (s *BlobStore) Store(logUrl *url.URL, logRequest LogRequest) error {
	if logUrl == nil {
		return errors.New("log url is invalid")
	}

	value, err := s.marshaller.Marshal(logRequest)
	if err != nil {
		s.log.Error(err)
		return err
	}

	bucket, configPrefix, err := parseBlobStoreURL(logUrl.String())
	if err != nil {
		s.log.Error(err)
		return err
	}

	if bucket == "" {
		return errors.New("no bucket specified in url")
	}

	objectKey, err := s.getObjectKey(configPrefix, &logRequest)
	if err != nil {
		s.log.Error(err)
		return err
	}

	err = s.provider.UploadObject(bucket, objectKey, value)
	if err != nil {
		s.log.Error(err)
		return err
	}
	s.log.Info("Successfully uploaded object to S3")
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

func (s *BlobStore) getObjectKey(configPrefix string, request *LogRequest) (string, error) {
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

	return fmt.Sprintf("%s/%s-%s.%s", prefix, request.Id, reqType, s.storeFormat), nil
}

func parseBlobStoreURL(s3url string) (bucket, key string, err error) {
	u, err := url.Parse(s3url)
	if err != nil {
		return "", "", err
	}

	if !strings.HasPrefix(u.Scheme, "s3") &&
		!strings.HasPrefix(u.Scheme, "gcs") {
		return "", "", fmt.Errorf("invalid scheme: %q", u.Scheme)
	}

	bucket = u.Host
	// u.Path starts with a "/" so trim it off.
	key = strings.TrimPrefix(u.Path, "/")
	return bucket, key, nil
}

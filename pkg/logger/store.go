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
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"
	"sync"

	"github.com/kserve/kserve/pkg/logger/marshaller"
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
	S3Prefix    string = string(S3Storage)
	GCSPrefix   string = string(GCSStorage)
	AzurePrefix string = string(AzureStorage)
)

const DefaultStorage = HttpStorage
const DefaultFormat = marshaller.LogStoreFormatJson
const DefaultBatchSize = 1

var registeredStrategies = map[string]StorageStrategy{
	S3Prefix:    S3Storage,
	GCSPrefix:   GCSStorage,
	AzurePrefix: AzureStorage,
}

func RegisterStorageStrategy(uriPrefix string, strategy StorageStrategy) {
	registeredStrategies[uriPrefix] = strategy
}

func GetStorageStrategy(url string) StorageStrategy {
	if str, ok := registeredStrategies[url]; ok {
		return str
	}
	return DefaultStorage
}

type Store interface {
	Store(logUrl *url.URL, logRequest LogRequest) error
}

type Batch struct {
	buffer map[url.URL][]interface{}
}

type BlobStore struct {
	mutex        sync.Mutex
	buffer       map[url.URL][]interface{}
	storePath    string
	storeFormat  string
	marshaller   marshaller.Marshaller
	provider     storage.Provider
	maxBatchSize int
	log          *zap.SugaredLogger
}

var _ Store = &BlobStore{}

func NewBlobStore(logStorePath string, logStoreFormat string, marshaller marshaller.Marshaller, provider storage.Provider, batchSize int, log *zap.SugaredLogger) *BlobStore {
	return &BlobStore{
		mutex:        sync.Mutex{},
		buffer:       make(map[url.URL][]interface{}),
		storePath:    logStorePath,
		storeFormat:  logStoreFormat,
		marshaller:   marshaller,
		provider:     provider,
		maxBatchSize: batchSize,
		log:          log,
	}
}

func NewStoreForScheme(scheme string, logStorePath string, logStoreFormat string, batchSize int, log *zap.SugaredLogger) (Store, error) {
	if logStoreFormat == "" {
		logStoreFormat = DefaultFormat
	}
	m, err := marshaller.GetMarshaller(logStoreFormat)
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
		return nil, fmt.Errorf("failed to create storage provider: %w", err)
	}

	if batchSize < DefaultBatchSize {
		batchSize = DefaultBatchSize
	}

	switch protocol {
	case storage.AZURE:
		fallthrough
	case storage.GCS:
		fallthrough
	case storage.S3:
		return NewBlobStore(logStorePath, logStoreFormat, m, provider, batchSize, log), nil
	}
	return nil, fmt.Errorf("unsupported protocol %s", protocol)
}

func (s *BlobStore) Store(logUrl *url.URL, logRequest LogRequest) error {
	if logUrl == nil {
		return errors.New("log url is invalid")
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if _, ok := s.buffer[*logUrl]; !ok {
		s.buffer[*logUrl] = make([]interface{}, 0)
	}
	s.buffer[*logUrl] = append(s.buffer[*logUrl], logRequest)
	size := 0
	for _, values := range s.buffer {
		size += len(values)
	}
	if size < s.maxBatchSize {
		return nil
	}

	for batchUrl, req := range s.buffer {
		value, err := s.marshaller.Marshal(req)
		if err != nil {
			s.log.Error(err)
			return err
		}
		bucket, configPrefix, err := parseBlobStoreURL(batchUrl.String(), s.log)
		if err != nil {
			s.log.Error(err)
			return err
		}

		if bucket == "" {
			return errors.New("no bucket specified in url")
		}

		// the most recent request in the batch is used to generate the key
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
		delete(s.buffer, *logUrl)
		s.log.Info("Successfully uploaded object to blob store")
	}

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

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
	"fmt"
	"github.com/kserve/kserve/pkg/agent/storage"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"go.uber.org/zap"
	"net/url"
	"path"
	"strings"
)

type StorageStrategy string

const (
	S3Storage   StorageStrategy = "s3"
	HttpStorage StorageStrategy = "http"
)

const DefaultStorage = HttpStorage

func GetStorageStrategy(url string) StorageStrategy {
	// http, https
	if strings.HasPrefix(url, "http") {
		return HttpStorage
	}

	// s3, s3a
	if strings.HasPrefix(url, "s3") {
		return S3Storage
	}

	return DefaultStorage
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
	GetStorageSpec() *v1beta1.StorageSpec
}

type S3Store struct {
	storeSpec  *v1beta1.StorageSpec
	log        *zap.SugaredLogger
	marshaller Marshaller
	provider   storage.Provider
}

var _ Store = &S3Store{}

func NewS3Store(storeSpec *v1beta1.StorageSpec, marshaller Marshaller, provider storage.Provider, log *zap.SugaredLogger) *S3Store {
	return &S3Store{
		storeSpec:  storeSpec,
		marshaller: marshaller,
		log:        log,
		provider:   provider,
	}
}

func NewStoreForScheme(scheme string, storeSpec *v1beta1.StorageSpec, log *zap.SugaredLogger) (Store, error) {
	if storeSpec == nil {
		return nil, fmt.Errorf("storage spec is invalid")
	}
	if storeSpec.StorageKey == nil {
		return nil, fmt.Errorf("no storage key specified in storage spec")
	}

	if storeSpec.Parameters == nil {
		return nil, fmt.Errorf("no parameters specified for store spec")
	}
	params := *storeSpec.Parameters
	format := DefaultStoreFormat
	if f, ok := params["format"]; ok {
		format = f
	}

	marshaller, err := getMarshaller(format)
	if err != nil {
		return nil, err
	}

	// Convert to a Protocol to reuse existing types
	if !strings.HasSuffix(scheme, "://") {
		scheme = scheme + "://"
	}
	protocol := storage.Protocol(scheme)
	provider, err := storage.GetProvider(map[storage.Protocol]storage.Provider{}, protocol)
	if err != nil {
		return nil, fmt.Errorf("failed to create S3 provider: %w", err)
	}
	if protocol == storage.S3 {
		return NewS3Store(storeSpec, marshaller, provider, log), nil
	}
	return nil, fmt.Errorf("unsupported protocol %s", protocol)
}

func (s *S3Store) Store(logUrl *url.URL, logRequest LogRequest) error {
	if logUrl == nil {
		return fmt.Errorf("log url is invalid")
	}

	value, err := s.marshaller.Marshal(logRequest)
	if err != nil {
		s.log.Error(err)
		return err
	}

	bucket, configPrefix, err := parseS3URL(logUrl.String())
	if err != nil {
		s.log.Error(err)
		return err
	}

	if bucket == "" {
		return fmt.Errorf("no bucket specified in url")
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

func (s *S3Store) GetStorageSpec() *v1beta1.StorageSpec {
	return s.storeSpec
}

func (s *S3Store) getObjectPrefix(configPrefix string, request *LogRequest) (string, error) {
	if request == nil {
		return "", fmt.Errorf("log request is invalid")
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

	if s.storeSpec != nil && s.storeSpec.Path != nil {
		parts = append(parts, *s.storeSpec.Path)
	}
	return path.Join(parts...), nil
}

func (s *S3Store) getObjectKey(configPrefix string, request *LogRequest) (string, error) {
	if request == nil {
		return "", fmt.Errorf("log request is invalid")
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
	params := *s.storeSpec.Parameters
	format := params["format"]

	return fmt.Sprintf("%s/%s-%s.%s", prefix, request.Id, reqType, format), nil
}

func parseS3URL(s3url string) (bucket, key string, err error) {
	u, err := url.Parse(s3url)
	if err != nil {
		return "", "", err
	}

	if !strings.HasPrefix(u.Scheme, "s3") {
		return "", "", fmt.Errorf("invalid scheme: %q", u.Scheme)
	}

	bucket = u.Host
	// u.Path starts with a "/" so trim it off.
	key = strings.TrimPrefix(u.Path, "/")
	return bucket, key, nil
}

/*
Copyright 2020 kubeflow.org.

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
	gstorage "cloud.google.com/go/storage"
	"context"
	"crypto/sha256"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/googleapis/google-cloud-go-testing/storage/stiface"
	gcscredential "github.com/kubeflow/kfserving/pkg/credentials/gcs"
	s3credential "github.com/kubeflow/kfserving/pkg/credentials/s3"
	"google.golang.org/api/option"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func FileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	if err == nil && !info.IsDir() {
		return true
	} else {
		return false
	}
}

func AsSha256(o interface{}) string {
	h := sha256.New()
	h.Write([]byte(fmt.Sprintf("%v", o)))

	return fmt.Sprintf("%x", h.Sum(nil))
}

func Create(fileName string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(fileName), 0770); err != nil {
		return nil, err
	}
	return os.Create(fileName)
}

func RemoveDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	names, err := d.Readdirnames(-1)
	if err != nil {
		return err
	}
	for _, name := range names {
		err = os.RemoveAll(filepath.Join(dir, name))
		if err != nil {
			return err
		}
	}
	// Remove empty dir
	if err := os.Remove(dir); err != nil {
		return fmt.Errorf("dir is unable to be deleted: %v", err)
	}
	return nil
}

func GetProvider(providers map[Protocol]Provider, protocol Protocol) (Provider, error) {
	if provider, ok := providers[protocol]; ok {
		return provider, nil
	}

	switch protocol {
	case GCS:
		var gcsClient *gstorage.Client
		var err error

		ctx := context.Background()
		if _, ok := os.LookupEnv(gcscredential.GCSCredentialEnvKey); ok {
			// GCS relies on environment variable GOOGLE_APPLICATION_CREDENTIALS to point to the service-account-key
			// If set, it will be automatically be picked up by the client.
			gcsClient, err = gstorage.NewClient(ctx)
		} else {
			gcsClient, err = gstorage.NewClient(ctx, option.WithoutAuthentication())
		}

		if err != nil {
			return nil, err
		}

		providers[GCS] = &GCSProvider{
			Client: stiface.AdaptClient(gcsClient),
		}
	case S3:
		if endpoint, ok := os.LookupEnv(s3credential.AWSEndpointUrl); ok {
			region, _ := os.LookupEnv(s3credential.AWSRegion)
			useVirtualBucketString, ok := os.LookupEnv(s3credential.S3UseVirtualBucket)
			useVirtualBucket := true
			if ok && strings.ToLower(useVirtualBucketString) == "false" {
				useVirtualBucket = false
			}
			sess, err := session.NewSession(&aws.Config{
				Endpoint:         aws.String(endpoint),
				Region:           aws.String(region),
				S3ForcePathStyle: aws.Bool(!useVirtualBucket)},
			)
			if err != nil {
				return nil, err
			}
			sessionClient := s3.New(sess)
			providers[S3] = &S3Provider{
				Client: sessionClient,
				Downloader: s3manager.NewDownloaderWithClient(sessionClient, func(d *s3manager.Downloader) {
				}),
			}
		}
	case HTTPS:
		var httpsClient HTTPSClient = &http.Client{}
		providers[HTTPS] = &HTTPSProvider{
			Client: &httpsClient,
		}
	}

	return providers[protocol], nil
}

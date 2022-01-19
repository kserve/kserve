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

package credentials

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/kserve/kserve/pkg/credentials/https"

	"github.com/kserve/kserve/pkg/credentials/azure"
	"github.com/kserve/kserve/pkg/credentials/gcs"
	"github.com/kserve/kserve/pkg/credentials/s3"
	"github.com/kserve/kserve/pkg/utils"
	v1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	CredentialConfigKeyName     = "credentials"
	UriSchemePlaceholder        = "<scheme-placeholder>"
	StorageConfigEnvKey         = "STORAGE_CONFIG"
	StorageOverrideConfigEnvKey = "STORAGE_OVERRIDE_CONFIG"
	DefaultStorageSecretKey     = "default"
)

type CredentialConfig struct {
	S3  s3.S3Config   `json:"s3,omitempty"`
	GCS gcs.GCSConfig `json:"gcs,omitempty"`
}

type CredentialBuilder struct {
	client client.Client
	config CredentialConfig
}

var log = logf.Log.WithName("CredentialBulder")

func NewCredentialBulder(client client.Client, config *v1.ConfigMap) *CredentialBuilder {
	credentialConfig := CredentialConfig{}
	if credential, ok := config.Data[CredentialConfigKeyName]; ok {
		err := json.Unmarshal([]byte(credential), &credentialConfig)
		if err != nil {
			panic(fmt.Errorf("Unable to unmarshall json string due to %v ", err))
		}
	}
	return &CredentialBuilder{
		client: client,
		config: credentialConfig,
	}
}

func (c *CredentialBuilder) CreateStorageSpecSecretEnvs(namespace string, storageKey string,
	storageSecretName string, overrideParams map[string]string, container *v1.Container) error {

	stype, ok := overrideParams["type"]

	secret := &v1.Secret{}
	var storageData []byte
	if err := c.client.Get(context.TODO(),
		types.NamespacedName{Name: storageSecretName, Namespace: namespace}, secret); err == nil {
		if storageKey != "" {
			if storageData = secret.Data[storageKey]; storageData == nil {
				return fmt.Errorf("specified storage key %s not found in storage secret %s",
					storageKey, storageSecretName)
			}
		} else {
			if stype == "" {
				storageKey = DefaultStorageSecretKey
			} else {
				storageKey = fmt.Sprintf("%s_%s", DefaultStorageSecretKey, stype)
			}
			// It's ok for the entry not to be found in the default/fallback cases
			storageData = secret.Data[storageKey]
		}
	} else if storageKey != "" || !apierr.IsNotFound(err) { // Don't fail if not found and no storage key was specified
		return fmt.Errorf("can't read storage secret %s: %w", storageSecretName, err)
	}

	if storageData != nil {
		if stype == "" {
			var storageDataJson map[string]string
			if err := json.Unmarshal(storageData, &storageDataJson); err != nil {
				return fmt.Errorf("invalid json encountered in key %s of storage secret %s: %w",
					storageKey, storageSecretName, err)
			}
			if stype, ok = storageDataJson["type"]; ok && stype != "s3" {
				return errors.New("only S3 type storage is currently supported with storage spec")
			}
		}
		// Pass storage config json as SecretKeyRef env var
		container.Env = append(container.Env, v1.EnvVar{
			Name: StorageConfigEnvKey,
			ValueFrom: &v1.EnvVarSource{
				SecretKeyRef: &v1.SecretKeySelector{
					LocalObjectReference: v1.LocalObjectReference{
						Name: storageSecretName,
					},
					Key: storageKey,
				},
			},
		})
	}

	if stype == "" {
		return errors.New("unable to determine storage type")
	}

	if strings.HasPrefix(container.Args[0], UriSchemePlaceholder+"://") {
		container.Args[0] = stype + container.Args[0][len(UriSchemePlaceholder):]
	}

	// Provide override secret values if parameters are provided
	if len(overrideParams) != 0 {
		if overrideParamsJSON, err := json.Marshal(overrideParams); err == nil {
			container.Env = append(container.Env, v1.EnvVar{
				Name:  StorageOverrideConfigEnvKey,
				Value: string(overrideParamsJSON),
			})
		}
	}

	return nil
}

func (c *CredentialBuilder) CreateSecretVolumeAndEnv(namespace string, serviceAccountName string,
	container *v1.Container, volumes *[]v1.Volume) error {
	if serviceAccountName == "" {
		serviceAccountName = "default"
	}
	s3SecretAccessKeyName := s3.AWSSecretAccessKeyName
	gcsCredentialFileName := gcs.GCSCredentialFileName

	if c.config.S3.S3SecretAccessKeyName != "" {
		s3SecretAccessKeyName = c.config.S3.S3SecretAccessKeyName
	}

	if c.config.GCS.GCSCredentialFileName != "" {
		gcsCredentialFileName = c.config.GCS.GCSCredentialFileName
	}

	serviceAccount := &v1.ServiceAccount{}
	err := c.client.Get(context.TODO(), types.NamespacedName{Name: serviceAccountName,
		Namespace: namespace}, serviceAccount)
	if err != nil {
		log.Error(err, "Failed to find service account", "ServiceAccountName", serviceAccountName,
			"Namespace", namespace)
		return nil
	}

	for _, secretRef := range serviceAccount.Secrets {
		log.Info("found secret", "SecretName", secretRef.Name)
		secret := &v1.Secret{}
		err := c.client.Get(context.TODO(), types.NamespacedName{Name: secretRef.Name,
			Namespace: namespace}, secret)
		if err != nil {
			log.Error(err, "Failed to find secret", "SecretName", secretRef.Name)
			continue
		}
		if _, ok := secret.Data[s3SecretAccessKeyName]; ok {
			log.Info("Setting secret envs for s3", "S3Secret", secret.Name)
			envs := s3.BuildSecretEnvs(secret, &c.config.S3)
			container.Env = append(container.Env, envs...)
		} else if _, ok := secret.Data[gcsCredentialFileName]; ok {
			log.Info("Setting secret volume for gcs", "GCSSecret", secret.Name)
			volume, volumeMount := gcs.BuildSecretVolume(secret)
			*volumes = utils.AppendVolumeIfNotExists(*volumes, volume)
			container.VolumeMounts =
				append(container.VolumeMounts, volumeMount)
			container.Env = append(container.Env,
				v1.EnvVar{
					Name:  gcs.GCSCredentialEnvKey,
					Value: gcs.GCSCredentialVolumeMountPath + gcsCredentialFileName,
				})
		} else if _, ok := secret.Data[azure.AzureClientSecret]; ok {
			log.Info("Setting secret envs for azure", "AzureSecret", secret.Name)
			envs := azure.BuildSecretEnvs(secret)
			container.Env = append(container.Env, envs...)
		} else if _, ok := secret.Data[https.HTTPSHost]; ok {
			log.Info("Setting secret volume from uri", "HTTP(S)Secret", secret.Name)
			envs := https.BuildSecretEnvs(secret)
			container.Env = append(container.Env, envs...)
		} else {
			log.V(5).Info("Skipping non gcs/s3/azure secret", "Secret", secret.Name)
		}
	}

	return nil
}

/*
Copyright 2019 kubeflow.org.

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

package s3

import (
	"github.com/kubeflow/kfserving/pkg/constants"
	"k8s.io/api/core/v1"
)

const (
	AWSAccessKeyId         = "AWS_ACCESS_KEY_ID"
	AWSSecretAccessKey     = "AWS_SECRET_ACCESS_KEY"
	AWSAccessKeyIdName     = "awsAccessKeyID"
	AWSSecretAccessKeyName = "awsSecretAccessKey"
	AWSEndpointUrl         = "AWS_ENDPOINT_URL"
	AWSRegion              = "AWS_REGION"
	S3Endpoint             = "S3_ENDPOINT"
	S3UseHttps             = "S3_USE_HTTPS"
	S3VerifySSL            = "S3_VERIFY_SSL"
	S3UseVirtualBucket     = "S3_USER_VIRTUAL_BUCKET"
)

type S3Config struct {
	S3AccessKeyIDName     string `json:"s3AccessKeyIDName,omitempty"`
	S3SecretAccessKeyName string `json:"s3SecretAccessKeyName,omitempty"`
	S3Endpoint            string `json:"s3Endpoint,omitempty"`
	S3UseHttps            string `json:"s3UseHttps,omitempty"`
}

var (
	InferenceServiceS3SecretEndpointAnnotation   = constants.KFServingAPIGroupName + "/" + "s3-endpoint"
	InferenceServiceS3SecretRegionAnnotation     = constants.KFServingAPIGroupName + "/" + "s3-region"
	InferenceServiceS3SecretSSLAnnotation        = constants.KFServingAPIGroupName + "/" + "s3-verifyssl"
	InferenceServiceS3SecretHttpsAnnotation      = constants.KFServingAPIGroupName + "/" + "s3-usehttps"
	InferenceServiceS3UseVirtualBucketAnnotation = constants.KFServingAPIGroupName + "/" + "s3-usevirtualbucket"
)

func BuildSecretEnvs(secret *v1.Secret, s3Config *S3Config) []v1.EnvVar {
	s3SecretAccessKeyName := AWSSecretAccessKeyName
	s3AccessKeyIdName := AWSAccessKeyIdName
	if s3Config.S3AccessKeyIDName != "" {
		s3AccessKeyIdName = s3Config.S3AccessKeyIDName
	}

	if s3Config.S3SecretAccessKeyName != "" {
		s3SecretAccessKeyName = s3Config.S3SecretAccessKeyName
	}
	envs := []v1.EnvVar{
		{
			Name: AWSAccessKeyId,
			ValueFrom: &v1.EnvVarSource{
				SecretKeyRef: &v1.SecretKeySelector{
					LocalObjectReference: v1.LocalObjectReference{
						Name: secret.Name,
					},
					Key: s3AccessKeyIdName,
				},
			},
		},
		{
			Name: AWSSecretAccessKey,
			ValueFrom: &v1.EnvVarSource{
				SecretKeyRef: &v1.SecretKeySelector{
					LocalObjectReference: v1.LocalObjectReference{
						Name: secret.Name,
					},
					Key: s3SecretAccessKeyName,
				},
			},
		},
	}

	if s3Endpoint, ok := secret.Annotations[InferenceServiceS3SecretEndpointAnnotation]; ok {
		s3EndpointUrl := "https://" + s3Endpoint
		if s3UseHttps, ok := secret.Annotations[InferenceServiceS3SecretHttpsAnnotation]; ok {
			if s3UseHttps == "0" {
				s3EndpointUrl = "http://" + secret.Annotations[InferenceServiceS3SecretEndpointAnnotation]
			}
			envs = append(envs, v1.EnvVar{
				Name:  S3UseHttps,
				Value: s3UseHttps,
			})
		}
		envs = append(envs, v1.EnvVar{
			Name:  S3Endpoint,
			Value: s3Endpoint,
		})
		envs = append(envs, v1.EnvVar{
			Name:  AWSEndpointUrl,
			Value: s3EndpointUrl,
		})

	} else if s3Config.S3Endpoint != "" {
		s3EndpointUrl := "https://" + s3Config.S3Endpoint
		if s3Config.S3UseHttps == "0" {
			s3EndpointUrl = "http://" + s3Config.S3Endpoint
			envs = append(envs, v1.EnvVar{
				Name:  S3UseHttps,
				Value: s3Config.S3UseHttps,
			})
		}
		envs = append(envs, v1.EnvVar{
			Name:  S3Endpoint,
			Value: s3Config.S3Endpoint,
		})
		envs = append(envs, v1.EnvVar{
			Name:  AWSEndpointUrl,
			Value: s3EndpointUrl,
		})

	}

	if s3Region, ok := secret.Annotations[InferenceServiceS3SecretRegionAnnotation]; ok {
		envs = append(envs, v1.EnvVar{
			Name:  AWSRegion,
			Value: s3Region,
		})
	}

	if verifySsl, ok := secret.Annotations[InferenceServiceS3SecretSSLAnnotation]; ok {
		envs = append(envs, v1.EnvVar{
			Name:  S3VerifySSL,
			Value: verifySsl,
		})
	}

	if useVirtualBucket, ok := secret.Annotations[InferenceServiceS3UseVirtualBucketAnnotation]; ok {
		envs = append(envs, v1.EnvVar{
			Name:  S3UseVirtualBucket,
			Value: useVirtualBucket,
		})
	}
	return envs
}

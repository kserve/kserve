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

import "k8s.io/api/core/v1"

const (
	AWSAccessKeyId     = "AWS_ACCESS_KEY_ID"
	AWSSecretAccessKey = "AWS_SECRET_ACCESS_KEY"
	AWSEndpointUrl     = "AWS_ENDPOINT_URL"
	S3Endpoint         = "S3_ENDPOINT"
	S3UseHttps         = "S3_USE_HTTPS"
	S3VerifySSL        = "S3_VERIFY_SSL"
)

func CreateS3SecretEnvs(secret *v1.Secret, endPoint string) []v1.EnvVar {
	envs := make([]v1.EnvVar, 0)
	s3SecretKey := v1.EnvVar{
		Name: AWSAccessKeyId,
		ValueFrom: &v1.EnvVarSource{
			SecretKeyRef: &v1.SecretKeySelector{
				LocalObjectReference: v1.LocalObjectReference{
					Name: secret.Name,
				},
				Key: AWSAccessKeyId,
			},
		},
	}
	s3SecretAccess := v1.EnvVar{
		Name: AWSSecretAccessKey,
		ValueFrom: &v1.EnvVarSource{
			SecretKeyRef: &v1.SecretKeySelector{
				LocalObjectReference: v1.LocalObjectReference{
					Name: secret.Name,
				},
				Key: AWSSecretAccessKey,
			},
		},
	}
	s3Endpoint := v1.EnvVar{
		Name:  S3Endpoint,
		Value: endPoint,
	}
	s3UseHttps := v1.EnvVar{
		Name:  S3UseHttps,
		Value: "0",
	}
	s3EndpointUrl := v1.EnvVar{
		Name:  AWSEndpointUrl,
		Value: "http://" + endPoint,
	}

	s3VerifySSL := v1.EnvVar{
		Name:  S3VerifySSL,
		Value: "0",
	}

	envs = append(envs, s3SecretKey)
	envs = append(envs, s3SecretAccess)
	envs = append(envs, s3Endpoint)
	envs = append(envs, s3EndpointUrl)
	envs = append(envs, s3UseHttps)
	envs = append(envs, s3VerifySSL)

	return envs
}

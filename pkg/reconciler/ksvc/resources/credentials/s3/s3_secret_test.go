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
	"github.com/google/go-cmp/cmp"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func TestS3Secret(t *testing.T) {
	scenarios := map[string]struct {
		secret   *v1.Secret
		expected []v1.EnvVar
	}{
		"S3SecretEnvs": {
			secret: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "s3-secret",
					Annotations: map[string]string{
						KFServiceS3SecretEndpointAnnotation: "s3.aws.com",
					},
				},
			},
			expected: []v1.EnvVar{
				{
					Name: AWSAccessKeyId,
					ValueFrom: &v1.EnvVarSource{
						SecretKeyRef: &v1.SecretKeySelector{
							LocalObjectReference: v1.LocalObjectReference{
								Name: "s3-secret",
							},
							Key: AWSAccessKeyIdName,
						},
					},
				},
				{
					Name: AWSSecretAccessKey,
					ValueFrom: &v1.EnvVarSource{
						SecretKeyRef: &v1.SecretKeySelector{
							LocalObjectReference: v1.LocalObjectReference{
								Name: "s3-secret",
							},
							Key: AWSSecretAccessKeyName,
						},
					},
				},
				{
					Name:  S3Endpoint,
					Value: "s3.aws.com",
				},
				{
					Name:  AWSEndpointUrl,
					Value: "https://s3.aws.com",
				},
			},
		},

		"S3SecretHttpsOverrideEnvs": {
			secret: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "s3-secret",
					Annotations: map[string]string{
						KFServiceS3SecretEndpointAnnotation: "s3.aws.com",
						KFServiceS3SecretHttpsAnnotation:    "0",
						KFServiceS3SecretSSLAnnotation:      "0",
					},
				},
			},

			expected: []v1.EnvVar{
				{
					Name: AWSAccessKeyId,
					ValueFrom: &v1.EnvVarSource{
						SecretKeyRef: &v1.SecretKeySelector{
							LocalObjectReference: v1.LocalObjectReference{
								Name: "s3-secret",
							},
							Key: AWSAccessKeyIdName,
						},
					},
				},
				{
					Name: AWSSecretAccessKey,
					ValueFrom: &v1.EnvVarSource{
						SecretKeyRef: &v1.SecretKeySelector{
							LocalObjectReference: v1.LocalObjectReference{
								Name: "s3-secret",
							},
							Key: AWSSecretAccessKeyName,
						},
					},
				},
				{
					Name:  S3UseHttps,
					Value: "0",
				},
				{
					Name:  S3Endpoint,
					Value: "s3.aws.com",
				},
				{
					Name:  AWSEndpointUrl,
					Value: "http://s3.aws.com",
				},
				{
					Name:  S3VerifySSL,
					Value: "0",
				},
			},
		},
	}

	for name, scenario := range scenarios {
		envs := BuildSecretEnvs(scenario.secret, AWSAccessKeyIdName, AWSSecretAccessKeyName)

		if diff := cmp.Diff(scenario.expected, envs); diff != "" {
			t.Errorf("Test %q unexpected result (-want +got): %v", name, diff)
		}
	}
}

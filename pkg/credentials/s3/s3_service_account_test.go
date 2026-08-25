/*
Copyright 2022 The KServe Authors.

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
	"testing"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestS3ServiceAccount(t *testing.T) {
	scenarios := map[string]struct {
		config         S3Config
		serviceAccount *corev1.ServiceAccount
		expected       []corev1.EnvVar
	}{
		"NoConfig": {
			serviceAccount: &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name: "s3-service-account",
				},
			},
			expected: []corev1.EnvVar{},
		},

		"S3Endpoint": {
			serviceAccount: &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name: "s3-service-account",
					Annotations: map[string]string{
						InferenceServiceS3SecretEndpointAnnotation: "s3.aws.com",
					},
				},
			},
			expected: []corev1.EnvVar{
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

		"S3HttpsOverrideEnvs": {
			serviceAccount: &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name: "s3-service-account",
					Annotations: map[string]string{
						InferenceServiceS3SecretEndpointAnnotation: "s3.aws.com",
						InferenceServiceS3SecretHttpsAnnotation:    "0",
						InferenceServiceS3SecretSSLAnnotation:      "0",
					},
				},
			},
			expected: []corev1.EnvVar{
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

		"S3EndpointWithHttpsProtocol": {
			serviceAccount: &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name: "s3-service-account",
					Annotations: map[string]string{
						InferenceServiceS3SecretEndpointAnnotation: "https://s3.aws.com",
						InferenceServiceS3SecretHttpsAnnotation:    "0",
						InferenceServiceS3SecretSSLAnnotation:      "1",
					},
				},
			},
			expected: []corev1.EnvVar{
				{
					Name:  S3UseHttps,
					Value: "1",
				},
				{
					Name:  S3Endpoint,
					Value: "s3.aws.com",
				},
				{
					Name:  AWSEndpointUrl,
					Value: "https://s3.aws.com",
				},
				{
					Name:  S3VerifySSL,
					Value: "1",
				},
			},
		},

		"S3EndpointWithHttpProtocol": {
			serviceAccount: &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name: "s3-service-account",
					Annotations: map[string]string{
						InferenceServiceS3SecretEndpointAnnotation: "http://s3.aws.com",
						InferenceServiceS3SecretHttpsAnnotation:    "1",
						InferenceServiceS3SecretSSLAnnotation:      "0",
					},
				},
			},
			expected: []corev1.EnvVar{
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

		"S3EnvsWithAnonymousCredentials": {
			serviceAccount: &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name: "s3-service-account",
					Annotations: map[string]string{
						InferenceServiceS3SecretEndpointAnnotation: "s3.aws.com",
						InferenceServiceS3UseAnonymousCredential:   "true",
					},
				},
			},
			expected: []corev1.EnvVar{
				{
					Name:  S3Endpoint,
					Value: "s3.aws.com",
				},
				{
					Name:  AWSEndpointUrl,
					Value: "https://s3.aws.com",
				},
				{
					Name:  AWSAnonymousCredential,
					Value: "true",
				},
			},
		},

		"S3Config": {
			config: S3Config{
				S3UseHttps:               "0",
				S3Endpoint:               "s3.aws.com",
				S3UseAnonymousCredential: "true",
			},
			serviceAccount: &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name: "s3-service-account",
				},
			},
			expected: []corev1.EnvVar{
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
					Name:  AWSAnonymousCredential,
					Value: "true",
				},
			},
		},
	}

	for name, scenario := range scenarios {
		envs := BuildServiceAccountEnvs(scenario.serviceAccount, &scenario.config)

		if diff := cmp.Diff(scenario.expected, envs); diff != "" {
			t.Errorf("Test %q unexpected result (-want +got): %v", name, diff)
		}
	}
}

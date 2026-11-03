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

package hdfs

import (
	corev1 "k8s.io/api/core/v1"
)

const (
	HdfsNamenode      = "HDFS_NAMENODE"
	HdfsRootPath      = "HDFS_ROOTPATH"
	KerberosPrincipal = "KERBEROS_PRINCIPAL"
	KerberosKeytab    = "KERBEROS_KEYTAB"
	TlsCert           = "TLS_CERT"
	TlsKey            = "TLS_KEY"
	TlsCa             = "TLS_CA"
	MountPath         = "/var/secrets/kserve-hdfscreds"
	HdfsVolumeName    = "hdfs-secrets"
)

func BuildSecret(secret *corev1.Secret) (corev1.Volume, corev1.VolumeMount) {
	volume := corev1.Volume{
		Name: HdfsVolumeName,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: secret.Name,
			},
		},
	}

	volumeMount := corev1.VolumeMount{
		MountPath: MountPath,
		Name:      HdfsVolumeName,
		ReadOnly:  true,
	}

	return volume, volumeMount
}

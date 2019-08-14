# Copyright 2019 The Kubeflow Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import logging
import json
import configparser
from os.path import expanduser

from kubernetes import client
from ..constants import constants

logger = logging.getLogger(__name__)


def create_gcs_credentials(namespace, credentials_file=None, service_account=None):
    '''
    Create GCP Credentails (secret and service account) with GCP credentials file.
    Args:
        credentials_file(str): The path for the gcp credentials file (Optional).
        service_account(str): The name of service account(Optional). If the service_account
                              is specified, will attach created secret with the service account,
                              otherwise will create new one and attach with created secret.
    return:
        gcs_sa_name(str): The name of created service account.
    '''

    if credentials_file is None:
        credentials_file = constants.GCS_DEFAULT_CREDS_FILE

    with open(expanduser(credentials_file)) as f:
        gcs_creds_content = f.read()

    # Try to get GCP creds file name from configmap, set default value then if cannot.
    gcs_creds_file_name = get_creds_name_from_config_map(
        'gcsCredentialFileName')
    if not gcs_creds_file_name:
        gcs_creds_file_name = constants.GCS_CREDS_FILE_DEFAULT_NAME

    string_data = {gcs_creds_file_name: gcs_creds_content}

    secret_name = create_secret(
        namespace=namespace, string_data=string_data)

    if 'service_account' is None:
        sa_name = create_service_account(
            secret_name=secret_name,
            namespace=namespace)
    else:
        sa_name = patch_service_account(
            secret_name=secret_name,
            namespace=namespace,
            sa_name=service_account)

    return sa_name


def create_s3_credentials(namespace, **kwargs):  # pylint: disable=too-many-locals
    '''
    Create S3 Credentails (secret and service account).
    Args:
        credentials_file(str): The path for the S3 credentials file (Optional).
        service_account(str): The name of service account(Optional). If the service_account
                              is specified, will attach created secret with the service account,
                              otherwise will create new one and attach with created secret.
        s3_endpoint(str): S3 settings variable S3_ENDPOINT (Optional).
        s3_region(str): S3 settings variable AWS_REGION (Optional).
        s3_use_https(str): S3 settings variable S3_USE_HTTPS (Optional).
        s3_verify_ssl(str): S3 settings variable S3_VERIFY_SSL (Optional).
    return:
        s3_sa_name(str): The name of created service account.
    '''

    if 'credentials_file' in kwargs.keys():
        credentials_file = kwargs['credentials_file']
    else:
        credentials_file = constants.S3_DEFAULT_CREDS_FILE

    if 's3_profile' in kwargs.keys():
        s3_creds_profile = kwargs['s3_profile']
    else:
        s3_creds_profile = 'default'

    config = configparser.ConfigParser()
    config.read([expanduser(credentials_file)])
    s3_access_key_id = config.get(s3_creds_profile, 'aws_access_key_id')
    s3_secret_access_key = config.get(
        s3_creds_profile, 'aws_secret_access_key')

    # Try to get AWS creds name from configmap, set default value then if cannot.
    s3_access_key_id_name = get_creds_name_from_config_map(
        's3AccessKeyIDName')
    if not s3_access_key_id_name:
        s3_access_key_id_name = constants.S3_ACCESS_KEY_ID_DEFAULT_NAME

    s3_secret_access_key_name = get_creds_name_from_config_map(
        's3SecretAccessKeyName')
    if not s3_secret_access_key_name:
        s3_secret_access_key_name = constants.S3_SECRET_ACCESS_KEY_DEFAULT_NAME

    data = {
        s3_access_key_id_name: s3_access_key_id,
        s3_secret_access_key_name: s3_secret_access_key,
    }

    s3_cred_sets = {
        's3_endpoint': constants.KFSERVING_GROUP + "/s3-endpoint",
        's3_region': constants.KFSERVING_GROUP + "/s3-region",
        's3_use_https': constants.KFSERVING_GROUP + "/s3-usehttps",
        's3_verify_ssl': constants.KFSERVING_GROUP + "/s3-verifyssl",
    }

    s3_annotations = {}
    for key, value in s3_cred_sets.items():
        if key in kwargs.keys():
            s3_annotations.update({value: kwargs[key]})

    secret_name = create_secret(
        namespace=namespace, annotations=s3_annotations, data=data)

    if 'service_account' not in kwargs.keys():
        sa_name = create_service_account(
            secret_name=secret_name,
            namespace=namespace)
    else:
        sa_name = patch_service_account(
            secret_name=secret_name,
            namespace=namespace,
            sa_name=kwargs['service_account'])

    return sa_name

def create_secret(namespace, annotations=None, data=None, string_data=None):
    'Create namespaced secret, and return the secret name.'
    try:
        created_secret = client.CoreV1Api().create_namespaced_secret(
            namespace,
            client.V1Secret(
                api_version='v1',
                kind='Secret',
                metadata=client.V1ObjectMeta(
                    generate_name=constants.DEFAULT_SECRET_NAME,
                    annotations=annotations),
                data=data,
                string_data=string_data))
    except client.rest.ApiException as e:
        raise RuntimeError(
            "Exception when calling CoreV1Api->create_namespaced_secret: %s\n" % e)

    secret_name = created_secret.metadata.name
    logger.info('Created Secret: %s in namespace %s', secret_name, namespace)
    return secret_name


def create_service_account(secret_name, namespace):
    'Create namespaced service account, and return the service account name'
    try:
        created_sa = client.CoreV1Api().create_namespaced_service_account(
            namespace,
            client.V1ServiceAccount(
                metadata=client.V1ObjectMeta(
                    generate_name=constants.DEFAULT_SA_NAME
                ),
                secrets=[client.V1ObjectReference(
                    kind='Secret',
                    name=secret_name)]))
    except client.rest.ApiException as e:
        raise RuntimeError(
            "Exception when calling CoreV1Api->create_namespaced_service_account: %s\n" % e)

    sa_name = created_sa.metadata.name
    logger.info('Created Service account: %s in namespace %s',
                sa_name, namespace)
    return sa_name


def patch_service_account(secret_name, namespace, sa_name):
    'Patch namespaced service account to attach with created secret.'
    try:
        client.CoreV1Api().patch_namespaced_service_account(
            sa_name,
            namespace,
            client.V1ServiceAccount(
                secrets=[client.V1ObjectReference(
                    kind='Secret',
                    name=secret_name)]))
    except client.rest.ApiException as e:
        raise RuntimeError(
            "Exception when calling CoreV1Api->patch_namespaced_service_account: %s\n" % e)

    logger.info('Pacthed Service account: %s in namespace %s',
                sa_name, namespace)
    return sa_name


def get_creds_name_from_config_map(creds):
    '''Get the credentials name from kfservice config map.'''
    try:
        kfsvc_config_map = client.CoreV1Api().read_namespaced_config_map(
            constants.KFSERVICE_CONFIG_MAP_NAME,
            constants.KFSERVICE_SYSTEM_NAMESPACE)
    except client.rest.ApiException as e:
        raise RuntimeError(
            "Exception when calling CoreV1Api->read_namespaced_config_map: %s\n" % e)

    kfsvc_creds_str = kfsvc_config_map.data['credentials']
    kfsvc_creds_json = json.loads(kfsvc_creds_str)

    if creds == 'gcsCredentialFileName':
        return kfsvc_creds_json['gcs']['gcsCredentialFileName']
    elif creds == 's3AccessKeyIDName':
        return kfsvc_creds_json['s3']['s3AccessKeyIDName']
    elif creds == 's3SecretAccessKeyName':
        return kfsvc_creds_json['s3']['s3SecretAccessKeyName']
    else:
        raise RuntimeError("Unknown credentials.")

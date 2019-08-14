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

from kubernetes import client
from ..constants import constants

logger = logging.getLogger(__name__)


def create_gcp_credentials(namespace, **kwargs):
    '''
    Create GCP Credentails (secret and service account) with GCP credentials file.
    :param str GCP_CREDS_FILE: The path for the gcp credentials file (required).
    :return: str  The name of created service account.
    '''

    check_key_in_args(kwargs, constants.GCP_CREDS_ARG_NAME)

    with open(kwargs[constants.GCP_CREDS_ARG_NAME]) as f:
        gcp_creds_content = f.read()

    # Try to get GCP creds file name from configmap, set default value then if cannot.
    gcp_creds_file_name = get_creds_name_from_config_map(
        'gcsCredentialFileName')
    if not gcp_creds_file_name:
        gcp_creds_file_name = constants.GCP_CREDS_FILE_DEFAULT_NAME

    string_data = {gcp_creds_file_name: gcp_creds_content}

    gcp_secret_name = create_secret(
        namespace=namespace, string_data=string_data)

    gcp_sa_name = create_service_account(
        secret_name=gcp_secret_name,
        namespace=namespace)

    return gcp_sa_name


def create_aws_credentials(namespace, **kwargs):
    '''
    Create AWS Credentails (secret and service account).
    :param str AWS_ACCESS_KEY_ID: AWS access key ID (required).
    :param str AWS_SECRET_ACCESS_KEY: AWS secret access key (required).
    :param str S3_ENDPOINT: S3 settings variables S3_ENDPOINT (optional).
    :param str AWS_REGION: S3 settings variables AWS_REGION (optional).
    :param str S3_USE_HTTPS: S3 settings variables S3_USE_HTTPS (optional).
    :param str S3_VERIFY_SSL: S3 settings variables S3_VERIFY_SSL (optional).
    :return: str  The name of created service account.
    '''

    check_key_in_args(kwargs, constants.AWS_ACCESS_KEY_ID)
    check_key_in_args(kwargs, constants.AWS_SECRET_ACCESS_KEY)

    # Try to get AWS creds name from configmap, set default value then if cannot.
    aws_access_key_id_name = get_creds_name_from_config_map(
        's3AccessKeyIDName')
    if not aws_access_key_id_name:
        aws_access_key_id_name = constants.AWS_ACCESS_KEY_ID_NAME

    aws_secret_access_key_name = get_creds_name_from_config_map(
        's3SecretAccessKeyName')
    if not aws_secret_access_key_name:
        aws_secret_access_key_name = constants.AWS_SECRET_ACCESS_KEY_NAME

    data = {
        aws_access_key_id_name: kwargs[constants.AWS_ACCESS_KEY_ID],
        aws_secret_access_key_name: kwargs[constants.AWS_SECRET_ACCESS_KEY],
    }

    aws_cred_sets = {
        'S3_ENDPOINT': constants.KFSERVING_GROUP + "/s3-endpoint",
        'AWS_REGION': constants.KFSERVING_GROUP + "/s3-region",
        'S3_USE_HTTPS': constants.KFSERVING_GROUP + "/s3-usehttps",
        'S3_VERIFY_SSL': constants.KFSERVING_GROUP + "/s3-verifyssl",
    }

    aws_annotations = {}
    for key, value in aws_cred_sets.items():
        if key in kwargs.keys():
            aws_annotations.update({value: kwargs[key]})

    aws_secret_name = create_secret(
        namespace=namespace, annotations=aws_annotations, data=data)

    aws_sa_name = create_service_account(
        secret_name=aws_secret_name, namespace=namespace)

    return aws_sa_name


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


def check_key_in_args(input_dict, key):
    '''Check if has specific key in the inputs parameters, raise expection if not.'''
    if key not in input_dict.keys():
        raise RuntimeError(
            'The %s must be specified for setting credentials.' % key)


def set_service_account(kfservice, sa_name):
    '''Attach the created service account with the kfserving.'''
    if kfservice.spec.default.service_account_name is None:
        kfservice.spec.default.service_account_name = sa_name

    if kfservice.spec.canary is not None and kfservice.spec.canary.service_account_name is None:
        kfservice.spec.canary.service_account_name = sa_name

    return kfservice


def get_creds_name_from_config_map(creds):
    '''Get the credentials name from kfservice config map.'''
    try:
        kfsvc_config_map = client.CoreV1Api().read_namespaced_config_map(
            constants.KFSERVICE_CONFIG_MAP_NAME,
            constants.KFSERVICE_SYSTEM_NAMESPACE)
    except ApiException as e:
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

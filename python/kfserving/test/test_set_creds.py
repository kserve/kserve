# Copyright 2019 kubeflow.org.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import configparser
from os.path import expanduser
from kubernetes import client
from kfserving import KFServingClient
from kfserving import constants

gcp_testing_creds = '''ewogICAgImNsaWVudF9pZCI6ICI3NjA1MTg1MDY0MDgtNnFyNHA2Z3BpNmhuNTA2cH\
Q4ZWp1cTgzZGkzNDFodXIuYXBwcy5nb29nbGV1c2VyY29udGVudC5jb20iLAogICAgImNsaWVudF9zZWNyZXQiOiAi\
ZC1GTDk1UTE5cTdNUW1IRDBUeUZwZDdoIiwKICAgICJyZWZyZXNoX3Rva2VuIjogIjEvYnFZbWt4bkRieEVzdEcxMlh\
jbU9ack4wLWV5STNiZWFuSmJSZDRrcXM2ZyIsCiAgICAidHlwZSI6ICJhdXRob3JpemVkX3VzZXIiCn0K'''


def get_created_secret(secret_name):
    return client.CoreV1Api().read_namespaced_secret(
        name=secret_name,
        namespace='kubeflow'
    )


def get_created_sa(sa_name):
    return client.CoreV1Api().read_namespaced_service_account(
        name=sa_name,
        namespace='kubeflow'
    )


def delete_sa(sa_name):
    return client.CoreV1Api().delete_namespaced_service_account( # pylint:disable=no-value-for-parameter
        name=sa_name,
        namespace='kubeflow'
    )

def check_sa_exists(service_account):
    '''Check if the specified service account existing.'''
    sa_list = client.CoreV1Api().list_namespaced_service_account(namespace='kubeflow')
    sa_name_list = []
    for item in range(0, len(sa_list.items)-1):
        sa_name_list.append(sa_list.items[item].metadata.name)
    if service_account in sa_name_list:
        return True
    return False

def test_set_credentials_s3():
    '''Test S3 credentials creating.'''
    KFServing = KFServingClient()
    credentials_file = './aws_credentials'

    #Test creating service account case.
    sa_name = constants.DEFAULT_SA_NAME
    if check_sa_exists(sa_name):
        delete_sa(sa_name)

    KFServing.set_credentials(storage_type='s3',
                              namespace='kubeflow',
                              credentials_file=credentials_file,
                              s3_profile='default',
                              s3_endpoint='s3.us-west-2.amazonaws.com',
                              s3_region='us-west-2',
                              s3_use_https='1',
                              s3_verify_ssl='0')

    sa_body = get_created_sa(sa_name)
    created_secret_name = sa_body.secrets[0].name
    created_secret = get_created_secret(created_secret_name)

    config = configparser.ConfigParser()
    config.read([expanduser(credentials_file)])
    s3_access_key_id = config.get('default', 'aws_access_key_id')
    s3_secret_access_key = config.get(
        'default', 'aws_secret_access_key')

    assert created_secret.data[constants.S3_ACCESS_KEY_ID_DEFAULT_NAME] == s3_access_key_id
    assert created_secret.data[constants.S3_SECRET_ACCESS_KEY_DEFAULT_NAME] == s3_secret_access_key
    assert created_secret.metadata.annotations[constants.KFSERVING_GROUP +
                                               '/s3-endpoint'] == 's3.us-west-2.amazonaws.com'
    assert created_secret.metadata.annotations[constants.KFSERVING_GROUP +
                                               '/s3-region'] == 'us-west-2'
    assert created_secret.metadata.annotations[constants.KFSERVING_GROUP +
                                               '/s3-usehttps'] == '1'
    assert created_secret.metadata.annotations[constants.KFSERVING_GROUP +
                                               '/s3-verifyssl'] == '0'


def test_set_credentials_gcp():
    '''Test GCP credentials creating'''
    KFServing = KFServingClient()
    sa_name = constants.DEFAULT_SA_NAME
    KFServing.set_credentials(storage_type='gcs',
                              namespace='kubeflow',
                              credentials_file='./gcp_credentials.json',
                              sa_name=sa_name)
    created_sa = get_created_sa(sa_name)
    created_secret_name = created_sa.secrets[0].name
    created_secret = get_created_secret(created_secret_name)
    assert created_secret.data[constants.GCS_CREDS_FILE_DEFAULT_NAME] == gcp_testing_creds


def test_azure_credentials():
    '''Test Azure credentials creating'''
    KFServing = KFServingClient()
    sa_name = constants.DEFAULT_SA_NAME
    KFServing.set_credentials(storage_type='Azure',
                              namespace='kubeflow',
                              credentials_file='./azure_credentials.json',
                              sa_name=sa_name)
    created_sa = get_created_sa(sa_name)
    created_secret_name = created_sa.secrets[0].name
    created_secret = get_created_secret(created_secret_name)
    assert created_secret.data['AZ_CLIENT_ID'] == 'YTJhYjExYWYtMDFhYS00NzU5LTgzNDUtNzgwMzI4N2RiZD'
    assert created_secret.data['AZ_CLIENT_SECRET'] == 'password'
    assert created_secret.data['AZ_SUBSCRIPTION_ID'] == 'MzMzMzMzMzMtMzMzMy0zMzMzLTMzMzMtMzMzMzMz'
    assert created_secret.data['AZ_TENANT_ID'] == 'QUJDREVGR0gtMTIzNC0xMjM0LTEyMzQtQUJDREVGR0hJSk'

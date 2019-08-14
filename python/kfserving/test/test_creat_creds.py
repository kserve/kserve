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


def test_create_credentials_s3():
    '''Test S3 credentials creating'''
    KFServing = KFServingClient()
    credentials_file = './aws_credentials'
    created_sa_name = KFServing.create_credentials(storage_type='s3',
                                                   namespace='kubeflow',
                                                   credentials_file=credentials_file,
                                                   s3_endpoint='s3.us-west-2.amazonaws.com',
                                                   s3_region='us-west-2',
                                                   s3_use_https='1',
                                                   s3_verify_ssl='0')
    created_sa = get_created_sa(created_sa_name)
    created_secret_name = created_sa.secrets[0].name
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



def test_create_credentials_gcp():
    '''Test GCP credentials creating'''
    KFServing = KFServingClient()
    created_sa_name = KFServing.create_credentials(storage_type='gcs',
                                                   namespace='kubeflow',
                                                   credentials_file='./gcp_credentials.json')
    created_sa = get_created_sa(created_sa_name)
    created_secret_name = created_sa.secrets[0].name
    created_secret = get_created_secret(created_secret_name)
    assert created_secret.data[constants.GCP_CREDS_FILE_DEFAULT_NAME] == gcp_testing_creds

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


def test_create_creds_aws():
    '''Test AWS credentials creating'''
    KFServing = KFServingClient()
    created_sa_name = KFServing.create_creds(platform='AWS',
                                             namespace='kubeflow',
                                             AWS_ACCESS_KEY_ID='MWYyZDFlMmU2N2Rm',
                                             AWS_SECRET_ACCESS_KEY='YWRtaW4=',
                                             S3_ENDPOINT='s3.us-west-2.amazonaws.com',
                                             AWS_REGION='us-west-2',
                                             S3_USE_HTTPS='1',
                                             S3_VERIFY_SSL='0')
    created_sa = get_created_sa(created_sa_name)
    created_secret_name = created_sa.secrets[0].name
    created_secret = get_created_secret(created_secret_name)
    assert created_secret.data[constants.AWS_ACCESS_KEY_ID_NAME] == 'MWYyZDFlMmU2N2Rm'
    assert created_secret.data[constants.AWS_SECRET_ACCESS_KEY_NAME] == 'YWRtaW4='
    assert created_secret.metadata.annotations[constants.KFSERVING_GROUP +
                                               '/s3-endpoint'] == 's3.us-west-2.amazonaws.com'
    assert created_secret.metadata.annotations[constants.KFSERVING_GROUP +
                                               '/s3-region'] == 'us-west-2'
    assert created_secret.metadata.annotations[constants.KFSERVING_GROUP +
                                               '/s3-usehttps'] == '1'
    assert created_secret.metadata.annotations[constants.KFSERVING_GROUP +
                                               '/s3-verifyssl'] == '0'



def test_create_creds_gcp():
    '''Test GCP credentials creating'''
    KFServing = KFServingClient()
    created_sa_name = KFServing.create_creds(platform='GCP',
                                             namespace='kubeflow',
                                             GCP_CREDS_FILE='./gcp_credentials.json')
    created_sa = get_created_sa(created_sa_name)
    created_secret_name = created_sa.secrets[0].name
    created_secret = get_created_secret(created_secret_name)
    assert created_secret.data[constants.GCP_CREDS_FILE_DEFAULT_NAME] == gcp_testing_creds

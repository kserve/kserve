# Copyright 2021 The KServe Authors.
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


import json
import configparser
from os.path import expanduser

from kubernetes import client
from ..constants import constants

from kserve.logging import logger


def set_gcs_credentials(namespace, credentials_file, service_account):
    """
    Set GCS Credentials (secret and service account) with credentials file.
    Args:
        namespace(str): The kubernetes namespace.
        credentials_file(str): The path for the gcs credentials file.
        service_account(str): The name of service account. If the service_account
                              is specified, will attach created secret with the service account,
                              otherwise will create new one and attach with created secret.
    """

    with open(expanduser(credentials_file)) as f:
        gcs_creds_content = f.read()

    # Try to get GCS creds file name from configmap, set default value then if cannot.
    gcs_creds_file_name = get_creds_name_from_config_map("gcsCredentialFileName")
    if not gcs_creds_file_name:
        gcs_creds_file_name = constants.GCS_CREDS_FILE_DEFAULT_NAME

    string_data = {gcs_creds_file_name: gcs_creds_content}

    secret_name = create_secret(namespace=namespace, string_data=string_data)

    set_service_account(
        namespace=namespace, service_account=service_account, secret_name=secret_name
    )


def set_s3_credentials(
    namespace,
    credentials_file,
    service_account,
    s3_profile="default",  # pylint: disable=too-many-locals,too-many-arguments
    s3_endpoint=None,
    s3_region=None,
    s3_use_https=None,
    s3_verify_ssl=None,
    s3_cabundle=None,
):  # pylint: disable=unused-argument
    """
    Set S3 Credentials (secret and service account).
    Args:
        namespace(str): The kubernetes namespace.
        credentials_file(str): The path for the S3 credentials file.
        s3_profile(str): The profile for S3, default value is 'default'.
        service_account(str): The name of service account(Optional). If the service_account
                              is specified, will attach created secret with the service account,
                              otherwise will create new one and attach with created secret.
        s3_endpoint(str): S3 settings variable S3_ENDPOINT.
        s3_region(str): S3 settings variable AWS_DEFAULT_REGION.
        s3_use_https(str): S3 settings variable S3_USE_HTTPS.
        s3_verify_ssl(str): S3 settings variable S3_VERIFY_SSL.
        s3_cabundle(str): S3 settings variable AWS_CA_BUNDLE.
    """

    config = configparser.ConfigParser()
    config.read([expanduser(credentials_file)])
    s3_access_key_id = config.get(s3_profile, "aws_access_key_id")
    s3_secret_access_key = config.get(s3_profile, "aws_secret_access_key")

    # Try to get S3 creds name from configmap, set default value then if cannot.
    s3_access_key_id_name = get_creds_name_from_config_map("s3AccessKeyIDName")
    if not s3_access_key_id_name:
        s3_access_key_id_name = constants.S3_ACCESS_KEY_ID_DEFAULT_NAME

    s3_secret_access_key_name = get_creds_name_from_config_map("s3SecretAccessKeyName")
    if not s3_secret_access_key_name:
        s3_secret_access_key_name = constants.S3_SECRET_ACCESS_KEY_DEFAULT_NAME

    data = {
        s3_access_key_id_name: s3_access_key_id,
        s3_secret_access_key_name: s3_secret_access_key,
    }

    s3_cred_sets = {
        "s3_endpoint": constants.KSERVE_GROUP + "/s3-endpoint",
        "s3_region": constants.KSERVE_GROUP + "/s3-region",
        "s3_use_https": constants.KSERVE_GROUP + "/s3-usehttps",
        "s3_verify_ssl": constants.KSERVE_GROUP + "/s3-verifyssl",
        "s3_cabundle": constants.KSERVE_GROUP + "/s3-cabundle",
    }

    s3_annotations = {}
    for key, value in s3_cred_sets.items():
        arg = vars()[key]
        if arg is not None:
            s3_annotations.update({value: arg})

    secret_name = create_secret(
        namespace=namespace, annotations=s3_annotations, data=data
    )

    set_service_account(
        namespace=namespace, service_account=service_account, secret_name=secret_name
    )


def set_azure_credentials(namespace, credentials_file, service_account):
    """
    Set Azure Credentials (secret and service account) with credentials file.
    Args:
        namespace(str): The kubernetes namespace.
        credentials_file(str): The path for the Azure credentials file.
        service_account(str): The name of service account. If the service_account
                              is specified, will attach created secret with the service account,
                              otherwise will create new one and attach with created secret.
    """

    with open(expanduser(credentials_file)) as azure_creds_file:
        azure_creds = json.load(azure_creds_file)

    data = {
        "AZURE_CLIENT_ID": azure_creds["clientId"],
        "AZURE_CLIENT_SECRET": azure_creds["clientSecret"],
        "AZURE_SUBSCRIPTION_ID": azure_creds["subscriptionId"],
        "AZURE_TENANT_ID": azure_creds["tenantId"],
    }

    secret_name = create_secret(namespace=namespace, data=data)

    set_service_account(
        namespace=namespace, service_account=service_account, secret_name=secret_name
    )


def create_secret(namespace, annotations=None, data=None, string_data=None):
    "Create namespaced secret, and return the secret name."
    try:
        created_secret = client.CoreV1Api().create_namespaced_secret(
            namespace,
            client.V1Secret(
                api_version="v1",
                kind="Secret",
                metadata=client.V1ObjectMeta(
                    generate_name=constants.DEFAULT_SECRET_NAME, annotations=annotations
                ),
                data=data,
                string_data=string_data,
            ),
        )
    except client.rest.ApiException as e:
        raise RuntimeError(
            "Exception when calling CoreV1Api->create_namespaced_secret: %s\n" % e
        )

    secret_name = created_secret.metadata.name
    logger.info("Created Secret: %s in namespace %s", secret_name, namespace)
    return secret_name


def set_service_account(namespace, service_account, secret_name):
    """
    Set service account, create if service_account does not exist, otherwise patch it.
    """
    if check_sa_exists(namespace=namespace, service_account=service_account):
        patch_service_account(
            secret_name=secret_name, namespace=namespace, sa_name=service_account
        )
    else:
        create_service_account(
            secret_name=secret_name, namespace=namespace, sa_name=service_account
        )


def check_sa_exists(namespace, service_account):
    """
    Check if the specified service account existing.
    """
    sa_list = client.CoreV1Api().list_namespaced_service_account(namespace=namespace)

    sa_name_list = [sa.metadata.name for sa in sa_list.items]

    if service_account in sa_name_list:
        return True

    return False


def create_service_account(secret_name, namespace, sa_name):
    """
    Create namespaced service account, and return the service account name
    """
    try:
        client.CoreV1Api().create_namespaced_service_account(
            namespace,
            client.V1ServiceAccount(
                metadata=client.V1ObjectMeta(name=sa_name),
                secrets=[client.V1ObjectReference(kind="Secret", name=secret_name)],
            ),
        )
    except client.rest.ApiException as e:
        raise RuntimeError(
            "Exception when calling CoreV1Api->create_namespaced_service_account: %s\n"
            % e
        )

    logger.info("Created Service account: %s in namespace %s", sa_name, namespace)


def patch_service_account(secret_name, namespace, sa_name):
    """
    Patch namespaced service account to attach with created secret.
    """
    try:
        client.CoreV1Api().patch_namespaced_service_account(
            sa_name,
            namespace,
            client.V1ServiceAccount(
                secrets=[client.V1ObjectReference(kind="Secret", name=secret_name)]
            ),
        )
    except client.rest.ApiException as e:
        raise RuntimeError(
            "Exception when calling CoreV1Api->patch_namespaced_service_account: %s\n"
            % e
        )

    logger.info("Patched Service account: %s in namespace %s", sa_name, namespace)


def get_creds_name_from_config_map(creds):
    """
    Get the credentials name from inferenceservice config map.
    """
    try:
        isvc_config_map = client.CoreV1Api().read_namespaced_config_map(
            constants.INFERENCESERVICE_CONFIG_MAP_NAME,
            constants.INFERENCESERVICE_SYSTEM_NAMESPACE,
        )
    except client.rest.ApiException:
        logger.warning(
            "Cannot get configmap %s in namespace %s.",
            constants.INFERENCESERVICE_CONFIG_MAP_NAME,
            constants.INFERENCESERVICE_SYSTEM_NAMESPACE,
        )
        return None

    isvc_creds_str = isvc_config_map.data["credentials"]
    isvc_creds_json = json.loads(isvc_creds_str)

    if creds == "gcsCredentialFileName":
        return isvc_creds_json["gcs"]["gcsCredentialFileName"]
    elif creds == "s3AccessKeyIDName":
        return isvc_creds_json["s3"]["s3AccessKeyIDName"]
    elif creds == "s3SecretAccessKeyName":
        return isvc_creds_json["s3"]["s3SecretAccessKeyName"]
    else:
        raise RuntimeError("Unknown credentials.")

# Copyright 2023 The KServe Authors.
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

import base64
import glob
import gzip
import json
import mimetypes
import os
import re
import shutil
import tarfile
import tempfile
import time
import zipfile
from pathlib import Path
from typing import Dict
from urllib.parse import urlparse
import requests

from .logging import logger

MODEL_MOUNT_DIRS = "/mnt/models"

_GCS_PREFIX = "gs://"
_S3_PREFIX = "s3://"
_HDFS_PREFIX = "hdfs://"
_WEBHDFS_PREFIX = "webhdfs://"
_AZURE_BLOB_RE = [
    "https://(.+?).blob.core.windows.net/(.+)",
    "https://(.+?).z[0-9]{1,2}.blob.storage.azure.net/(.+)",
]
_AZURE_FILE_RE = [
    "https://(.+?).file.core.windows.net/(.+)",
    "https://(.+?).z[0-9]{1,2}.file.storage.azure.net/(.+)",
]
_LOCAL_PREFIX = "file://"
_URI_RE = "https?://(.+)/(.+)"
_HTTP_PREFIX = "http(s)://"
_HEADERS_SUFFIX = "-headers"
_PVC_PREFIX = "/mnt/pvc"
_HF_PREFIX = "hf://"

_HDFS_SECRET_DIRECTORY = "/var/secrets/kserve-hdfscreds"
_HDFS_FILE_SECRETS = ["KERBEROS_KEYTAB", "TLS_CERT", "TLS_KEY", "TLS_CA"]


class Storage(object):
    @staticmethod
    def download(uri: str, out_dir: str = None) -> str:
        start = time.monotonic()
        Storage._update_with_storage_spec()
        logger.info("Copying contents of %s to local", uri)

        if uri.startswith(_PVC_PREFIX) and not os.path.exists(uri):
            raise Exception(f"Cannot locate source uri {uri} for PVC")

        is_local = uri.startswith(_LOCAL_PREFIX) or os.path.exists(uri)
        if is_local:
            if out_dir is None:
                # noop if out_dir is not set and the path is local
                model_dir = Storage._download_local(uri)
            else:
                if not os.path.exists(out_dir):
                    os.mkdir(out_dir)
                model_dir = Storage._download_local(uri, out_dir)
        else:
            if out_dir is None:
                out_dir = tempfile.mkdtemp()
            elif not os.path.exists(out_dir):
                os.mkdir(out_dir)

            if uri.startswith(MODEL_MOUNT_DIRS):
                # Don't need to download models if this InferenceService is running in the multi-model
                # serving mode. The model agent will download models.
                model_dir = out_dir
            elif uri.startswith(_GCS_PREFIX):
                model_dir = Storage._download_gcs(uri, out_dir)
            elif uri.startswith(_S3_PREFIX):
                model_dir = Storage._download_s3(uri, out_dir)
            elif uri.startswith(_HDFS_PREFIX) or uri.startswith(_WEBHDFS_PREFIX):
                model_dir = Storage._download_hdfs(uri, out_dir)
            elif any(re.search(pattern, uri) for pattern in _AZURE_BLOB_RE):
                model_dir = Storage._download_azure_blob(uri, out_dir)
            elif any(re.search(pattern, uri) for pattern in _AZURE_FILE_RE):
                model_dir = Storage._download_azure_file_share(uri, out_dir)
            elif re.search(_URI_RE, uri):
                model_dir = Storage._download_from_uri(uri, out_dir)
            elif uri.startswith(_HF_PREFIX):
                model_dir = Storage._download_hf(uri, out_dir)
            else:
                raise Exception(
                    "Cannot recognize storage type for "
                    + uri
                    + "\n'%s', '%s', '%s', and '%s' are the current available storage type."
                    % (_GCS_PREFIX, _S3_PREFIX, _LOCAL_PREFIX, _HTTP_PREFIX)
                )

        logger.info("Successfully copied %s to %s", uri, out_dir)
        logger.info(f"Model downloaded in {time.monotonic() - start} seconds.")
        return model_dir

    @staticmethod
    def _update_with_storage_spec():
        storage_secret_json = json.loads(os.environ.get("STORAGE_CONFIG", "{}"))
        storage_secret_override_params = json.loads(
            os.environ.get("STORAGE_OVERRIDE_CONFIG", "{}")
        )
        if storage_secret_override_params:
            for key, value in storage_secret_override_params.items():
                storage_secret_json[key] = value

        if storage_secret_json.get("type", "") == "s3":
            for env_var, key in (
                ("AWS_ENDPOINT_URL", "endpoint_url"),
                ("AWS_ACCESS_KEY_ID", "access_key_id"),
                ("AWS_SECRET_ACCESS_KEY", "secret_access_key"),
                ("AWS_DEFAULT_REGION", "region"),
                ("AWS_CA_BUNDLE", "ca_bundle"),
                ("S3_VERIFY_SSL", "verify_ssl"),
                ("awsAnonymousCredential", "anonymous"),
            ):
                if key in storage_secret_json:
                    os.environ[env_var] = storage_secret_json.get(key)

        if (
            storage_secret_json.get("type", "") == "hdfs"
            or storage_secret_json.get("type", "") == "webhdfs"
        ):
            temp_dir = tempfile.mkdtemp()
            os.environ["HDFS_SECRET_DIR"] = temp_dir
            for key, value in storage_secret_json.items():
                mode = "w"

                # If the secret is supposed to be a file, then it was base64 encoded in the json
                if key in _HDFS_FILE_SECRETS:
                    value = base64.b64decode(value)
                    mode = "wb"

                with open(f"{temp_dir}/{key}", mode) as f:
                    f.write(value)
                    f.flush()

    @staticmethod
    def get_S3_config():
        from botocore import UNSIGNED
        from botocore.client import Config

        # default s3 config
        c = Config()

        # anon environment variable defined in s3_secret.go
        anon = "true" == os.getenv("awsAnonymousCredential", "false").lower()
        # S3UseVirtualBucket environment variable defined in s3_secret.go
        # use virtual hosted-style URLs if enabled
        virtual = "true" == os.getenv("S3_USER_VIRTUAL_BUCKET", "false").lower()
        # S3UseAccelerate environment variable defined in s3_secret.go
        # use transfer acceleration if enabled
        accelerate = "true" == os.getenv("S3_USE_ACCELERATE", "false").lower()

        if anon:
            c = c.merge(Config(signature_version=UNSIGNED))
        if virtual:
            c = c.merge(Config(s3={"addressing_style": "virtual"}))
        if accelerate:
            c = c.merge(Config(s3={"use_accelerate_endpoint": accelerate}))

        # NOTE: If endpoint_url provided is legacy ("https://s3.amazonaws.com") and region is not global (us-east-1), set to virtual addressing style
        # So that request would not return PermanentRedirect due to region not in the endpoint url
        # AWS SDK retries under the hood to set the correct region when the valid virtual addressing style endpoint url is provided
        endpoint_url = os.getenv("AWS_ENDPOINT_URL")
        region = os.getenv("AWS_DEFAULT_REGION")
        if endpoint_url == "https://s3.amazonaws.com" and region not in (
            None,
            "us-east-1",
        ):
            c = c.merge(Config(s3={"addressing_style": "virtual"}))

        return c

    @staticmethod
    def _download_s3(uri, temp_dir: str) -> str:
        import boto3

        # Boto3 looks at various configuration locations until it finds configuration values.
        # lookup order:
        # 1. Config object passed in as the config parameter when creating S3 resource
        #    if awsAnonymousCredential env var true, passed in via config
        # 2. Environment variables
        # 3. ~/.aws/config file
        kwargs = {"config": Storage.get_S3_config()}
        endpoint_url = os.getenv("AWS_ENDPOINT_URL")
        if endpoint_url:
            kwargs.update({"endpoint_url": endpoint_url})
        verify_ssl = os.getenv("S3_VERIFY_SSL")
        if verify_ssl:
            verify_ssl = not verify_ssl.lower() in ["0", "false"]
            kwargs.update({"verify": verify_ssl})
        else:
            verify_ssl = True

        # If verify_ssl is true, then check there is custom ca bundle cert
        if verify_ssl:
            global_ca_bundle_configmap = os.getenv("CA_BUNDLE_CONFIGMAP_NAME")
            if global_ca_bundle_configmap:
                isvc_aws_ca_bundle_path = os.getenv("AWS_CA_BUNDLE")
                if isvc_aws_ca_bundle_path and isvc_aws_ca_bundle_path != "":
                    ca_bundle_full_path = isvc_aws_ca_bundle_path
                else:
                    global_ca_bundle_volume_mount_path = os.getenv(
                        "CA_BUNDLE_VOLUME_MOUNT_POINT"
                    )
                    ca_bundle_full_path = (
                        global_ca_bundle_volume_mount_path + "/cabundle.crt"
                    )
                if os.path.exists(ca_bundle_full_path):
                    logger.info("ca bundle file(%s) exists." % (ca_bundle_full_path))
                    kwargs.update({"verify": ca_bundle_full_path})
                else:
                    raise RuntimeError(
                        "Failed to find ca bundle file(%s)." % ca_bundle_full_path
                    )
        s3 = boto3.resource("s3", **kwargs)
        parsed = urlparse(uri, scheme="s3")
        bucket_name = parsed.netloc
        bucket_path = parsed.path.lstrip("/")

        file_count = 0
        exact_obj_found = False
        bucket = s3.Bucket(bucket_name)
        for obj in bucket.objects.filter(Prefix=bucket_path):
            # Skip where boto3 lists the directory as an object
            if obj.key.endswith("/"):
                continue
            # In the case where bucket_path points to a single object, set the target key to bucket_path
            # Otherwise, remove the bucket_path prefix, strip any extra slashes, then prepend the target_dir
            # Example:
            # s3://test-bucket
            # Objects: /a/b/c/model.bin /a/model.bin /model.bin
            #
            # If 'uri' is set to "s3://test-bucket", then the downloader will
            # download all the objects listed above, re-creating their subpaths
            # under the temp_dir.
            # If 'uri' is set to "s3://test-bucket/a", then the downloader will
            # add to temp_dir: b/c/model.bin and model.bin.
            # If 'uri' is set to "s3://test-bucket/a/b/c/model.bin", then
            # the downloader will add to temp dir: model.bin
            # (without any subpaths).
            # If the bucket path is s3://test/models
            # Objects: churn, churn-pickle, churn-pickle-logs

            if bucket_path == obj.key:
                target_key = obj.key.rsplit("/", 1)[-1]
                exact_obj_found = True

            else:
                target_key = re.sub(r"^" + re.escape(bucket_path) + r"/?", "", obj.key)

            target = f"{temp_dir}/{target_key}"
            if not os.path.exists(os.path.dirname(target)):
                os.makedirs(os.path.dirname(target), exist_ok=True)
            bucket.download_file(obj.key, target)
            logger.info("Downloaded object %s to %s" % (obj.key, target))
            file_count += 1

            # If the exact object is found, then it is sufficient to download that and break the loop
            if exact_obj_found:
                break
        if file_count == 0:
            raise RuntimeError(
                "Failed to fetch model. No model found in %s." % bucket_path
            )

        # Unpack compressed file, supports .tgz, tar.gz and zip file formats.
        if file_count == 1:
            mimetype, _ = mimetypes.guess_type(target)
            if mimetype in ["application/x-tar", "application/zip"]:
                temp_dir = Storage._unpack_archive_file(target, mimetype, temp_dir)
        return temp_dir

    @staticmethod
    def _download_hf(uri, temp_dir: str) -> str:
        from huggingface_hub import snapshot_download

        components = uri[len(_HF_PREFIX) :].split("/")

        # Validate that the URI has two parts: repo and model (optional hash)
        if len(components) != 2:
            raise ValueError(
                "URI must contain exactly one '/' separating the repo and model name"
            )

        repo = components[0]
        model_part = components[1]

        if not repo:
            raise ValueError("Repository name cannot be empty")
        if not model_part:
            raise ValueError("Model name cannot be empty")

        model, _, hash_value = model_part.partition(":")
        # Ensure model is non-empty
        if not model:
            raise ValueError("Model name cannot be empty")

        revision = hash_value if hash_value else None

        snapshot_download(
            repo_id=f"{repo}/{model}", revision=revision, local_dir=temp_dir
        )
        return temp_dir

    @staticmethod
    def _download_gcs(uri, temp_dir: str) -> str:
        from google.auth import exceptions
        from google.cloud import storage
        import copy

        try:
            storage_client = storage.Client()
        except exceptions.DefaultCredentialsError:
            storage_client = storage.Client.create_anonymous_client()
        bucket_args = uri.replace(_GCS_PREFIX, "", 1).split("/", 1)
        bucket_name = bucket_args[0]
        bucket_path = bucket_args[1] if len(bucket_args) > 1 else ""
        bucket = storage_client.bucket(bucket_name)
        prefix = bucket_path
        if not prefix.endswith("/"):
            prefix = prefix + "/"
        blobs = bucket.list_blobs(prefix=prefix)
        file_count = 0

        # Shallow copy, otherwise Iterator has already started
        shallow_blobs = copy.copy(blobs)
        blob = bucket.blob(bucket_path)
        # checks if the blob is a file or a directory
        if blob.name == bucket_path and len(list(shallow_blobs)) == 0:
            dest_path = os.path.join(temp_dir, os.path.basename(bucket_path))
            logger.info("Downloading single file to: %s", dest_path)
            blob.download_to_filename(dest_path)
            file_count = 1

        else:
            for blob in blobs:
                # Replace any prefix from the object key with temp_dir
                subdir_object_key = blob.name.replace(bucket_path, "", 1).lstrip("/")
                # Create necessary subdirectory to store the object locally
                if "/" in subdir_object_key:
                    local_object_dir = os.path.join(
                        temp_dir, subdir_object_key.rsplit("/", 1)[0]
                    )
                    if not os.path.isdir(local_object_dir):
                        os.makedirs(local_object_dir, exist_ok=True)
                if subdir_object_key.strip() != "" and not subdir_object_key.endswith(
                    "/"
                ):
                    dest_path = os.path.join(temp_dir, subdir_object_key)
                    logger.info("Downloading: %s", dest_path)
                    blob.download_to_filename(dest_path)
                    file_count += 1

        if file_count == 0:
            raise RuntimeError("Failed to fetch model. No model found in %s." % uri)

        # Unpack compressed file, supports .tgz, tar.gz and zip file formats.
        if file_count == 1:
            mimetype, _ = mimetypes.guess_type(blob.name)
            if mimetype in ["application/x-tar", "application/zip"]:
                temp_dir = Storage._unpack_archive_file(dest_path, mimetype, temp_dir)
        return temp_dir

    @staticmethod
    def _load_hdfs_configuration() -> Dict:
        config = {
            "HDFS_NAMENODE": None,
            "USER_PROXY": None,
            "HDFS_ROOTPATH": None,
            "TLS_CERT": None,
            "TLS_KEY": None,
            "TLS_CA": None,
            "TLS_SKIP_VERIFY": "false",
            "HEADERS": None,
            "N_THREADS": "2",
            "KERBEROS_KEYTAB": None,
            "KERBEROS_PRINCIPAL": None,
        }

        secret_dir = _HDFS_SECRET_DIRECTORY
        if os.environ.get("HDFS_SECRET_DIR"):
            secret_dir = os.environ["HDFS_SECRET_DIR"]

        for filename in os.listdir(secret_dir):
            if filename not in config:
                continue

            # We don't read files which are supposed to be files, just save their path
            if filename in _HDFS_FILE_SECRETS:
                config[filename] = f"{secret_dir}/{filename}"
                continue

            # Read file and save value in config dict
            with open(f"{secret_dir}/{filename}") as f:
                config[filename] = f.read()

        return config

    @staticmethod
    def _download_hdfs(uri, out_dir: str) -> str:
        from krbcontext.context import krbContext
        from hdfs.ext.kerberos import Client, KerberosClient

        config = Storage._load_hdfs_configuration()

        logger.info(f"Using the following hdfs config\n{config}")

        # Remove hdfs:// or webhdfs:// from the uri to get just the path
        # e.g. hdfs://user/me/model -> user/me/model
        if uri.startswith(_HDFS_PREFIX):
            path = uri[len(_HDFS_PREFIX) :]
        else:
            path = uri[len(_WEBHDFS_PREFIX) :]

        if not config["HDFS_ROOTPATH"]:
            path = "/" + path

        s = requests.Session()

        if config["TLS_CERT"]:
            s.cert = (config["TLS_CERT"], config["TLS_KEY"])
        # s.verify = , True, False, or CA PATH
        if config["TLS_CA"]:
            s.verify = config["TLS_CA"]
        if config["TLS_SKIP_VERIFY"].lower() == "true":
            s.verify = False

        if config["HEADERS"]:
            headers = json.loads(config["HEADERS"])
            s.headers.update(headers)

        if config["KERBEROS_PRINCIPAL"]:
            context = krbContext(
                using_keytab=True,
                principal=config["KERBEROS_PRINCIPAL"],
                keytab_file=config["KERBEROS_KEYTAB"],
            )
            context.init_with_keytab()
            client = KerberosClient(
                config["HDFS_NAMENODE"],
                proxy=config["USER_PROXY"],
                root=config["HDFS_ROOTPATH"],
                session=s,
            )
        else:
            client = Client(
                config["HDFS_NAMENODE"],
                proxy=config["USER_PROXY"],
                root=config["HDFS_ROOTPATH"],
                session=s,
            )
        file_count = 0
        dest_file_path = ""

        # Check path exists and get path status
        # Raises HdfsError when path does not exist
        status = client.status(path)

        if status["type"] == "FILE":
            client.download(path, out_dir, n_threads=1)
            file_count += 1
            file_name = path.rsplit("/", 1)[-1]
            dest_file_path = f"{out_dir}/{file_name}"
        else:
            files = client.list(path)
            file_count += len(files)
            for f in files:
                client.download(
                    f"{path}/{f}", out_dir, n_threads=int(config["N_THREADS"])
                )
                dest_file_path = f"{out_dir}/{f}"

        if file_count == 1:
            mimetype, _ = mimetypes.guess_type(dest_file_path)
            if mimetype in ["application/x-tar", "application/zip"]:
                out_dir = Storage._unpack_archive_file(
                    dest_file_path, mimetype, out_dir
                )
        return out_dir

    @staticmethod
    def _download_azure_blob(
        uri, out_dir: str
    ) -> str:  # pylint: disable=too-many-locals
        from azure.storage.blob import BlobServiceClient
        from azure.storage.blob._list_blobs_helper import BlobPrefix

        account_name, account_url, container_name, prefix = Storage._parse_azure_uri(
            uri
        )
        logger.info(
            "Connecting to BLOB account: [%s], container: [%s], prefix: [%s]",
            account_name,
            container_name,
            prefix,
        )
        token = (
            Storage._get_azure_storage_token()
            or Storage._get_azure_storage_access_key()
        )
        if token is None:
            logger.warning(
                "Azure credentials or shared access signature token not found, retrying anonymous access"
            )

        blob_service_client = BlobServiceClient(account_url, credential=token)
        container_client = blob_service_client.get_container_client(container_name)
        file_count = 0
        blobs = []
        max_depth = 5
        stack = [(prefix, max_depth)]
        while stack:
            curr_prefix, depth = stack.pop()
            if depth < 0:
                continue
            for item in container_client.walk_blobs(name_starts_with=curr_prefix):
                if isinstance(item, BlobPrefix):
                    stack.append((item.name, depth - 1))
                else:
                    blobs += container_client.list_blobs(
                        name_starts_with=item.name, include=["snapshots"]
                    )
        for blob in blobs:
            file_name = blob.name.replace(prefix, "", 1).lstrip("/")
            if not file_name:
                file_name = os.path.basename(prefix)
            dest_path = os.path.join(out_dir, file_name)
            Path(os.path.dirname(dest_path)).mkdir(parents=True, exist_ok=True)
            logger.info("Downloading: %s to %s", blob.name, dest_path)
            downloader = container_client.download_blob(blob.name)
            with open(dest_path, "wb+") as f:
                f.write(downloader.readall())
            file_count += 1
        if file_count == 0:
            raise RuntimeError("Failed to fetch model. No model found in %s." % (uri))

        # Unpack compressed file, supports .tgz, tar.gz and zip file formats.
        if file_count == 1:
            mimetype, _ = mimetypes.guess_type(dest_path)
            if mimetype in ["application/x-tar", "application/zip"]:
                out_dir = Storage._unpack_archive_file(dest_path, mimetype, out_dir)
        return out_dir

    @staticmethod
    def _download_azure_file_share(
        uri, out_dir: str
    ) -> str:  # pylint: disable=too-many-locals
        from azure.storage.fileshare import ShareServiceClient

        account_name, account_url, share_name, prefix = Storage._parse_azure_uri(uri)
        logger.info(
            "Connecting to file share account: [%s], container: [%s], prefix: [%s]",
            account_name,
            share_name,
            prefix,
        )
        access_key = Storage._get_azure_storage_access_key()
        if access_key is None:
            logger.warning(
                "Azure storage access key not found, retrying anonymous access"
            )

        share_service_client = ShareServiceClient(account_url, credential=access_key)
        share_client = share_service_client.get_share_client(share_name)
        file_count = 0
        share_files = []
        max_depth = 5
        stack = [(prefix, max_depth)]
        while stack:
            curr_prefix, depth = stack.pop()
            if depth < 0:
                continue
            for item in share_client.list_directories_and_files(
                directory_name=curr_prefix
            ):
                if item.is_directory:
                    stack.append(
                        ("/".join([curr_prefix, item.name]).strip("/"), depth - 1)
                    )
                else:
                    share_files.append((curr_prefix, item))
        for prefix, file_item in share_files:
            parts = [prefix] if prefix else []
            parts.append(file_item.name)
            file_path = "/".join(parts).lstrip("/")
            dest_path = os.path.join(out_dir, file_path)
            Path(os.path.dirname(dest_path)).mkdir(parents=True, exist_ok=True)
            logger.info("Downloading: %s to %s", file_item.name, dest_path)
            file_client = share_client.get_file_client(file_path)
            with open(dest_path, "wb+") as f:
                data = file_client.download_file()
                data.readinto(f)
            file_count += 1
        if file_count == 0:
            raise RuntimeError("Failed to fetch model. No model found in %s." % (uri))

        # Unpack compressed file, supports .tgz, tar.gz and zip file formats.
        if file_count == 1:
            mimetype, _ = mimetypes.guess_type(dest_path)
            if mimetype in ["application/x-tar", "application/zip"]:
                out_dir = Storage._unpack_archive_file(dest_path, mimetype, out_dir)
        return out_dir

    @staticmethod
    def _parse_azure_uri(uri):  # pylint: disable=too-many-locals
        parsed = urlparse(uri)
        account_name = parsed.netloc.split(".")[0]
        account_url = "https://{}{}".format(
            parsed.netloc, "?" + parsed.query if parsed.query else ""
        )
        object_name, prefix = parsed.path.lstrip("/").split("/", 1)
        prefix = prefix.strip("/")
        return account_name, account_url, object_name, prefix

    @staticmethod
    def _get_azure_storage_token():
        tenant_id = os.getenv("AZ_TENANT_ID", "")
        client_id = os.getenv("AZ_CLIENT_ID", "")
        client_secret = os.getenv("AZ_CLIENT_SECRET", "")

        # convert old environment variable to conform Azure defaults
        # see azure/identity/_constants.py
        if tenant_id:
            os.environ["AZURE_TENANT_ID"] = tenant_id
        if client_id:
            os.environ["AZURE_CLIENT_ID"] = client_id
        if client_secret:
            os.environ["AZURE_CLIENT_SECRET"] = client_secret

        client_id = os.getenv("AZURE_CLIENT_ID", "")
        if not client_id:
            return None

        # note the SP must have "Storage Blob Data Owner" perms for this to work
        from azure.identity import DefaultAzureCredential

        token_credential = DefaultAzureCredential()

        logger.info("Retrieved SP token credential for client_id: %s", client_id)
        return token_credential

    @staticmethod
    def _get_azure_storage_access_key():
        return os.getenv("AZURE_STORAGE_ACCESS_KEY")

    @staticmethod
    def _download_local(uri, out_dir=None) -> str:
        local_path = uri.replace(_LOCAL_PREFIX, "", 1)
        if not os.path.exists(local_path):
            raise RuntimeError("Local path %s does not exist." % (uri))

        if out_dir is None:
            return local_path
        elif not os.path.isdir(out_dir):
            os.makedirs(out_dir, exist_ok=True)

        if os.path.isdir(local_path):
            local_path = os.path.join(local_path, "*")

        file_count = 0
        for src in glob.glob(local_path):
            _, tail = os.path.split(src)
            dest_path = os.path.join(out_dir, tail)
            logger.info("Linking: %s to %s", src, dest_path)
            if not os.path.exists(dest_path):
                os.symlink(src, dest_path)
            else:
                logger.info("File %s already exist", dest_path)
            file_count += 1
        if file_count == 0:
            raise RuntimeError("Failed to fetch model. No model found in %s." % (uri))
        # Unpack compressed file, supports .tgz, tar.gz and zip file formats.
        if file_count == 1:
            mimetype, _ = mimetypes.guess_type(dest_path)
            if mimetype in ["application/x-tar", "application/zip"]:
                out_dir = Storage._unpack_archive_file(dest_path, mimetype, out_dir)
        return out_dir

    @staticmethod
    def _download_from_uri(uri, out_dir=None) -> str:
        url = urlparse(uri)
        filename = os.path.basename(url.path)
        # Determine if the symbol '?' exists in the path
        if mimetypes.guess_type(url.path)[0] is None and url.query != "":
            mimetype, encoding = mimetypes.guess_type(url.query)
        else:
            mimetype, encoding = mimetypes.guess_type(url.path)
        local_path = os.path.join(out_dir, filename)

        if filename == "":
            raise ValueError("No filename contained in URI: %s" % (uri))

        # Get header information from host url
        headers = {}
        host_uri = url.hostname

        headers_json = os.getenv(host_uri + _HEADERS_SUFFIX, "{}")
        headers = json.loads(headers_json)

        with requests.get(uri, stream=True, headers=headers) as response:
            if response.status_code != 200:
                raise RuntimeError(
                    "URI: %s returned a %s response code." % (uri, response.status_code)
                )
            zip_content_types = (
                "application/x-zip-compressed",
                "application/zip",
                "application/zip-compressed",
            )
            if mimetype == "application/zip" and not response.headers.get(
                "Content-Type", ""
            ).startswith(zip_content_types):
                raise RuntimeError(
                    "URI: %s did not respond with any of following 'Content-Type': "
                    % uri
                    + ", ".join(zip_content_types)
                )
            tar_content_types = (
                "application/x-tar",
                "application/x-gtar",
                "application/x-gzip",
                "application/gzip",
            )
            if mimetype == "application/x-tar" and not response.headers.get(
                "Content-Type", ""
            ).startswith(tar_content_types):
                raise RuntimeError(
                    "URI: %s did not respond with any of following 'Content-Type': "
                    % uri
                    + ", ".join(tar_content_types)
                )
            if (
                mimetype != "application/zip" and mimetype != "application/x-tar"
            ) and not response.headers.get("Content-Type", "").startswith(
                "application/octet-stream"
            ):
                raise RuntimeError(
                    "URI: %s did not respond with 'Content-Type': 'application/octet-stream'"
                    % uri
                )

            if encoding == "gzip":
                stream = gzip.GzipFile(fileobj=response.raw)
                local_path = os.path.join(out_dir, f"{filename}.tar")
            else:
                stream = response.raw
            with open(local_path, "wb") as out:
                shutil.copyfileobj(stream, out)

        if mimetype in ["application/x-tar", "application/zip"]:
            out_dir = Storage._unpack_archive_file(local_path, mimetype, out_dir)
        return out_dir

    @staticmethod
    def _unpack_archive_file(file_path, mimetype, target_dir=None) -> str:
        if not target_dir:
            target_dir = os.path.dirname(file_path)

        try:
            logger.info("Unpacking: %s", file_path)
            if mimetype == "application/x-tar":
                archive = tarfile.open(file_path, "r", encoding="utf-8")
            else:
                archive = zipfile.ZipFile(file_path, "r")
            archive.extractall(target_dir)
            archive.close()
        except (tarfile.TarError, zipfile.BadZipfile):
            raise RuntimeError(
                "Failed to unpack archive file. \
The file format is not valid."
            )
        os.remove(file_path)
        return target_dir

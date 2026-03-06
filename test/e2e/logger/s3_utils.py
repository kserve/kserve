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

import os
import time

import boto3
from botocore.config import Config as BotoConfig

S3_ENDPOINT = os.environ.get("S3_ENDPOINT", "http://localhost:8333")
S3_ACCESS_KEY = os.environ.get("S3_ACCESS_KEY", "s3admin")
S3_SECRET_KEY = os.environ.get("S3_SECRET_KEY", "s3admin123")
S3_REGION = os.environ.get("S3_REGION", "us-south")
LOGGER_BUCKET = "logger-output"


def create_s3_client():
    """Create a boto3 S3 client configured for the SeaweedFS test backend.

    Defaults to localhost:8333 (requires port-forward for local dev):
        kubectl port-forward -n kserve svc/s3-service 8333:8333

    Override via environment variables for CI or in-cluster access:
        S3_ENDPOINT=http://s3-service.kserve:8333
    """
    return boto3.client(
        "s3",
        endpoint_url=S3_ENDPOINT,
        aws_access_key_id=S3_ACCESS_KEY,
        aws_secret_access_key=S3_SECRET_KEY,
        region_name=S3_REGION,
        config=BotoConfig(signature_version="s3v4"),
    )


def wait_for_s3_objects(
    s3_client,
    bucket: str,
    prefix: str,
    expected_count: int,
    timeout: int = 60,
    poll_interval: int = 5,
) -> list[dict]:
    """Poll S3 until at least expected_count objects appear under prefix.

    Returns the list of S3 object metadata dicts.
    Raises TimeoutError if objects do not appear within timeout seconds.
    """
    deadline = time.time() + timeout
    while time.time() < deadline:
        response = s3_client.list_objects_v2(Bucket=bucket, Prefix=prefix)
        objects = response.get("Contents", [])
        if len(objects) >= expected_count:
            return objects
        time.sleep(poll_interval)
    response = s3_client.list_objects_v2(Bucket=bucket, Prefix=prefix)
    objects = response.get("Contents", [])
    raise TimeoutError(
        f"Expected {expected_count} objects under s3://{bucket}/{prefix}, "
        f"found {len(objects)} after {timeout}s"
    )


def get_s3_object_body(s3_client, bucket: str, key: str) -> bytes:
    """Download an S3 object and return its body as bytes."""
    response = s3_client.get_object(Bucket=bucket, Key=key)
    return response["Body"].read()


def cleanup_s3_prefix(s3_client, bucket: str, prefix: str):
    """Delete all objects under a given S3 prefix."""
    response = s3_client.list_objects_v2(Bucket=bucket, Prefix=prefix)
    objects = response.get("Contents", [])
    if objects:
        delete_keys = [{"Key": obj["Key"]} for obj in objects]
        s3_client.delete_objects(
            Bucket=bucket, Delete={"Objects": delete_keys}
        )

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
import re

import pytest

from kserve import KServeClient
from ..common.utils import predict_isvc, KSERVE_TEST_NAMESPACE
from .format_verifiers import (
    verify_json_object,
    verify_csv_object,
    verify_parquet_object,
)
from .marshaller_helpers import create_logger_isvc
from .s3_utils import (
    LOGGER_BUCKET,
    cleanup_s3_prefix,
    create_s3_client,
    get_s3_object_body,
    wait_for_s3_objects,
)

kserve_client = KServeClient(config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))


# --- JSON tests ---


@pytest.mark.marshaller
@pytest.mark.asyncio(scope="session")
async def test_json_immediate(rest_v1_client):
    """JSON marshaller with immediate batching (batch_size=1).

    Send 1 prediction. Expect 2 S3 objects (request + response),
    each containing a single JSON LogRequest object.
    """
    s3 = create_s3_client()
    service_name, isvc = create_logger_isvc(format="json", batch_size=1)
    store_path = isvc.spec.predictor.logger.storage.path
    prefix = f"logs/{KSERVE_TEST_NAMESPACE}/{service_name}/predictor/{store_path}/"

    try:
        kserve_client.create(isvc)
        kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

        res = await predict_isvc(rest_v1_client, service_name, "./data/iris_input.json")
        assert res["predictions"] == [1, 1]

        objects = wait_for_s3_objects(s3, LOGGER_BUCKET, prefix, 2, timeout=60)

        request_objects = [obj for obj in objects if "-request." in obj["Key"]]
        response_objects = [obj for obj in objects if "-response." in obj["Key"]]
        assert len(request_objects) == 1, "Should have exactly 1 request object"
        assert len(response_objects) == 1, "Should have exactly 1 response object"

        for obj in objects:
            body = get_s3_object_body(s3, LOGGER_BUCKET, obj["Key"])
            assert obj["Key"].endswith(".json"), (
                f"Expected .json extension, got: {obj['Key']}"
            )
            verify_json_object(body, expected_records=1)

        keys = [obj["Key"].split("/")[-1] for obj in objects]
        for key in keys:
            assert re.match(r".+-(request|response)\.json$", key), (
                f"Object key does not match expected pattern: {key}"
            )
    finally:
        kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
        cleanup_s3_prefix(s3, LOGGER_BUCKET, prefix)


@pytest.mark.marshaller
@pytest.mark.asyncio(scope="session")
async def test_json_size_batch(rest_v1_client):
    """JSON marshaller with size-based batching (batch_size=5).

    Send 5 predictions. Expect 2 S3 objects: one batch of 5 request
    records and one batch of 5 response records, each as a JSON array.
    """
    s3 = create_s3_client()
    service_name, isvc = create_logger_isvc(format="json", batch_size=5)
    store_path = isvc.spec.predictor.logger.storage.path
    prefix = f"logs/{KSERVE_TEST_NAMESPACE}/{service_name}/predictor/{store_path}/"

    try:
        kserve_client.create(isvc)
        kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

        for _ in range(5):
            res = await predict_isvc(
                rest_v1_client, service_name, "./data/iris_input.json"
            )
            assert res["predictions"] == [1, 1]

        objects = wait_for_s3_objects(s3, LOGGER_BUCKET, prefix, 2, timeout=90)

        for obj in objects:
            body = get_s3_object_body(s3, LOGGER_BUCKET, obj["Key"])
            assert obj["Key"].endswith(".json")
            verify_json_object(body, expected_records=5)
    finally:
        kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
        cleanup_s3_prefix(s3, LOGGER_BUCKET, prefix)


@pytest.mark.marshaller
@pytest.mark.asyncio(scope="session")
async def test_json_timed_batch(rest_v1_client):
    """JSON marshaller with timed batching (batch_size=2, interval=5s).

    Phase 1: Send 2 requests to fill the batch. Verify 2 objects with 2 records.
    Phase 2: Send 1 request, wait for timer flush. Verify 2 more objects with 1 record.
    """
    s3 = create_s3_client()
    service_name, isvc = create_logger_isvc(
        format="json", batch_size=2, batch_interval="5s"
    )
    store_path = isvc.spec.predictor.logger.storage.path
    prefix = f"logs/{KSERVE_TEST_NAMESPACE}/{service_name}/predictor/{store_path}/"

    try:
        kserve_client.create(isvc)
        kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

        # Phase 1: fill the batch
        for _ in range(2):
            res = await predict_isvc(
                rest_v1_client, service_name, "./data/iris_input.json"
            )
            assert res["predictions"] == [1, 1]

        objects = wait_for_s3_objects(s3, LOGGER_BUCKET, prefix, 2, timeout=60)
        for obj in objects:
            body = get_s3_object_body(s3, LOGGER_BUCKET, obj["Key"])
            verify_json_object(body, expected_records=2)

        # Phase 2: partial batch flushed by timer
        res = await predict_isvc(rest_v1_client, service_name, "./data/iris_input.json")
        assert res["predictions"] == [1, 1]

        objects = wait_for_s3_objects(
            s3, LOGGER_BUCKET, prefix, 4, timeout=30, poll_interval=3
        )
        new_objects = sorted(objects, key=lambda o: o["Key"])[2:]
        for obj in new_objects:
            body = get_s3_object_body(s3, LOGGER_BUCKET, obj["Key"])
            verify_json_object(body, expected_records=1)
    finally:
        kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
        cleanup_s3_prefix(s3, LOGGER_BUCKET, prefix)


# --- CSV tests ---


@pytest.mark.marshaller
@pytest.mark.asyncio(scope="session")
async def test_csv_immediate(rest_v1_client):
    """CSV marshaller with immediate batching (batch_size=1).

    Send 1 prediction. Expect 2 S3 objects (request + response),
    each containing a CSV with header row + 1 data row.
    """
    s3 = create_s3_client()
    service_name, isvc = create_logger_isvc(format="csv", batch_size=1)
    store_path = isvc.spec.predictor.logger.storage.path
    prefix = f"logs/{KSERVE_TEST_NAMESPACE}/{service_name}/predictor/{store_path}/"

    try:
        kserve_client.create(isvc)
        kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

        res = await predict_isvc(rest_v1_client, service_name, "./data/iris_input.json")
        assert res["predictions"] == [1, 1]

        objects = wait_for_s3_objects(s3, LOGGER_BUCKET, prefix, 2, timeout=60)

        request_objects = [obj for obj in objects if "-request." in obj["Key"]]
        response_objects = [obj for obj in objects if "-response." in obj["Key"]]
        assert len(request_objects) == 1, "Should have exactly 1 request object"
        assert len(response_objects) == 1, "Should have exactly 1 response object"

        for obj in objects:
            body = get_s3_object_body(s3, LOGGER_BUCKET, obj["Key"])
            assert obj["Key"].endswith(".csv"), (
                f"Expected .csv extension, got: {obj['Key']}"
            )
            verify_csv_object(body, expected_records=1)
    finally:
        kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
        cleanup_s3_prefix(s3, LOGGER_BUCKET, prefix)


@pytest.mark.marshaller
@pytest.mark.asyncio(scope="session")
async def test_csv_size_batch(rest_v1_client):
    """CSV marshaller with size-based batching (batch_size=5).

    Send 5 predictions. Expect 2 S3 objects: one batch of 5 request
    rows and one batch of 5 response rows.
    """
    s3 = create_s3_client()
    service_name, isvc = create_logger_isvc(format="csv", batch_size=5)
    store_path = isvc.spec.predictor.logger.storage.path
    prefix = f"logs/{KSERVE_TEST_NAMESPACE}/{service_name}/predictor/{store_path}/"

    try:
        kserve_client.create(isvc)
        kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

        for _ in range(5):
            res = await predict_isvc(
                rest_v1_client, service_name, "./data/iris_input.json"
            )
            assert res["predictions"] == [1, 1]

        objects = wait_for_s3_objects(s3, LOGGER_BUCKET, prefix, 2, timeout=90)

        for obj in objects:
            body = get_s3_object_body(s3, LOGGER_BUCKET, obj["Key"])
            assert obj["Key"].endswith(".csv")
            verify_csv_object(body, expected_records=5)
    finally:
        kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
        cleanup_s3_prefix(s3, LOGGER_BUCKET, prefix)


@pytest.mark.marshaller
@pytest.mark.asyncio(scope="session")
async def test_csv_timed_batch(rest_v1_client):
    """CSV marshaller with timed batching (batch_size=2, interval=5s).

    Phase 1: Send 2 requests to fill the batch. Verify 2 objects with 2 rows.
    Phase 2: Send 1 request, wait for timer flush. Verify 2 more objects with 1 row.
    """
    s3 = create_s3_client()
    service_name, isvc = create_logger_isvc(
        format="csv", batch_size=2, batch_interval="5s"
    )
    store_path = isvc.spec.predictor.logger.storage.path
    prefix = f"logs/{KSERVE_TEST_NAMESPACE}/{service_name}/predictor/{store_path}/"

    try:
        kserve_client.create(isvc)
        kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

        for _ in range(2):
            res = await predict_isvc(
                rest_v1_client, service_name, "./data/iris_input.json"
            )
            assert res["predictions"] == [1, 1]

        objects = wait_for_s3_objects(s3, LOGGER_BUCKET, prefix, 2, timeout=60)
        for obj in objects:
            body = get_s3_object_body(s3, LOGGER_BUCKET, obj["Key"])
            verify_csv_object(body, expected_records=2)

        res = await predict_isvc(rest_v1_client, service_name, "./data/iris_input.json")
        assert res["predictions"] == [1, 1]

        objects = wait_for_s3_objects(
            s3, LOGGER_BUCKET, prefix, 4, timeout=30, poll_interval=3
        )
        new_objects = sorted(objects, key=lambda o: o["Key"])[2:]
        for obj in new_objects:
            body = get_s3_object_body(s3, LOGGER_BUCKET, obj["Key"])
            verify_csv_object(body, expected_records=1)
    finally:
        kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
        cleanup_s3_prefix(s3, LOGGER_BUCKET, prefix)


# --- Parquet tests ---


@pytest.mark.marshaller
@pytest.mark.asyncio(scope="session")
async def test_parquet_immediate(rest_v1_client):
    """Parquet marshaller with immediate batching (batch_size=1).

    Send 1 prediction. Expect 2 S3 objects (request + response),
    each containing a Parquet file with 1 row.
    """
    s3 = create_s3_client()
    service_name, isvc = create_logger_isvc(format="parquet", batch_size=1)
    store_path = isvc.spec.predictor.logger.storage.path
    prefix = f"logs/{KSERVE_TEST_NAMESPACE}/{service_name}/predictor/{store_path}/"

    try:
        kserve_client.create(isvc)
        kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

        res = await predict_isvc(rest_v1_client, service_name, "./data/iris_input.json")
        assert res["predictions"] == [1, 1]

        objects = wait_for_s3_objects(s3, LOGGER_BUCKET, prefix, 2, timeout=60)

        request_objects = [obj for obj in objects if "-request." in obj["Key"]]
        response_objects = [obj for obj in objects if "-response." in obj["Key"]]
        assert len(request_objects) == 1, "Should have exactly 1 request object"
        assert len(response_objects) == 1, "Should have exactly 1 response object"

        for obj in objects:
            body = get_s3_object_body(s3, LOGGER_BUCKET, obj["Key"])
            assert obj["Key"].endswith(".parquet"), (
                f"Expected .parquet extension, got: {obj['Key']}"
            )
            verify_parquet_object(body, expected_records=1)
    finally:
        kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
        cleanup_s3_prefix(s3, LOGGER_BUCKET, prefix)


@pytest.mark.marshaller
@pytest.mark.asyncio(scope="session")
async def test_parquet_size_batch(rest_v1_client):
    """Parquet marshaller with size-based batching (batch_size=5).

    Send 5 predictions. Expect 2 S3 objects: one batch of 5 request
    rows and one batch of 5 response rows.
    """
    s3 = create_s3_client()
    service_name, isvc = create_logger_isvc(format="parquet", batch_size=5)
    store_path = isvc.spec.predictor.logger.storage.path
    prefix = f"logs/{KSERVE_TEST_NAMESPACE}/{service_name}/predictor/{store_path}/"

    try:
        kserve_client.create(isvc)
        kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

        for _ in range(5):
            res = await predict_isvc(
                rest_v1_client, service_name, "./data/iris_input.json"
            )
            assert res["predictions"] == [1, 1]

        objects = wait_for_s3_objects(s3, LOGGER_BUCKET, prefix, 2, timeout=90)

        for obj in objects:
            body = get_s3_object_body(s3, LOGGER_BUCKET, obj["Key"])
            assert obj["Key"].endswith(".parquet")
            verify_parquet_object(body, expected_records=5)
    finally:
        kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
        cleanup_s3_prefix(s3, LOGGER_BUCKET, prefix)


@pytest.mark.marshaller
@pytest.mark.asyncio(scope="session")
async def test_parquet_timed_batch(rest_v1_client):
    """Parquet marshaller with timed batching (batch_size=2, interval=5s).

    Phase 1: Send 2 requests to fill the batch. Verify 2 objects with 2 rows.
    Phase 2: Send 1 request, wait for timer flush. Verify 2 more objects with 1 row.
    """
    s3 = create_s3_client()
    service_name, isvc = create_logger_isvc(
        format="parquet", batch_size=2, batch_interval="5s"
    )
    store_path = isvc.spec.predictor.logger.storage.path
    prefix = f"logs/{KSERVE_TEST_NAMESPACE}/{service_name}/predictor/{store_path}/"

    try:
        kserve_client.create(isvc)
        kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

        for _ in range(2):
            res = await predict_isvc(
                rest_v1_client, service_name, "./data/iris_input.json"
            )
            assert res["predictions"] == [1, 1]

        objects = wait_for_s3_objects(s3, LOGGER_BUCKET, prefix, 2, timeout=60)
        for obj in objects:
            body = get_s3_object_body(s3, LOGGER_BUCKET, obj["Key"])
            verify_parquet_object(body, expected_records=2)

        res = await predict_isvc(rest_v1_client, service_name, "./data/iris_input.json")
        assert res["predictions"] == [1, 1]

        objects = wait_for_s3_objects(
            s3, LOGGER_BUCKET, prefix, 4, timeout=30, poll_interval=3
        )
        new_objects = sorted(objects, key=lambda o: o["Key"])[2:]
        for obj in new_objects:
            body = get_s3_object_body(s3, LOGGER_BUCKET, obj["Key"])
            verify_parquet_object(body, expected_records=1)
    finally:
        kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
        cleanup_s3_prefix(s3, LOGGER_BUCKET, prefix)

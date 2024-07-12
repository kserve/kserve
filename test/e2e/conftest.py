# Copyright 2024 The KServe Authors.
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

import asyncio

import pytest
import pytest_asyncio
from httpx import Timeout

import kserve
from kserve import InferenceRESTClient, RESTConfig
from kserve.constants.constants import PredictorProtocol
from kserve.logging import logger, KSERVE_LOG_CONFIG


@pytest.fixture(scope="session", autouse=True)
def configure_logger():
    KSERVE_LOG_CONFIG["loggers"]["kserve"]["propagate"] = True
    KSERVE_LOG_CONFIG["loggers"]["kserve.trace"]["propagate"] = True
    kserve.logging.configure_logging(KSERVE_LOG_CONFIG)
    logger.info("Logger configured")


@pytest.fixture(scope="session")
def event_loop():
    return asyncio.get_event_loop()


@pytest_asyncio.fixture(scope="session")
async def rest_v1_client():
    v1_client = InferenceRESTClient(
        config=RESTConfig(
            timeout=Timeout(60, read=120),
            verbose=True,
            protocol=PredictorProtocol.REST_V1,
        )
    )
    yield v1_client
    await v1_client.close()


@pytest_asyncio.fixture(scope="session")
async def rest_v2_client():
    v2_client = InferenceRESTClient(
        config=RESTConfig(
            timeout=Timeout(60, read=120),
            verbose=True,
            protocol=PredictorProtocol.REST_V2,
        )
    )
    yield v2_client
    await v2_client.close()

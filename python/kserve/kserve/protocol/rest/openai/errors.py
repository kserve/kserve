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

from http import HTTPStatus
from fastapi.responses import JSONResponse
from kserve.logging import logger


class OpenAIError(Exception):
    """
    Exception class for generic OpenAI error.
    """

    def __init__(self, reason):
        self.reason = reason

    def __str__(self):
        return self.reason


async def openai_error_handler(_, exc):
    logger.error("Exception:", exc_info=exc)
    return JSONResponse(
        status_code=HTTPStatus.INTERNAL_SERVER_ERROR,
        content={"error": f"{type(exc).__name__} : {str(exc)}"},
    )

# Copyright 2025 The KServe Authors.
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

from typing import Union
from http import HTTPStatus
from fastapi.responses import JSONResponse
from kserve.logging import logger
from .types import Error, ErrorResponse


class TimeSeriesError(Exception):
    """
    Exception class for generic Time Series error.
    """

    def __init__(self, response: Union[str, ErrorResponse]):
        self.response = response

    def __str__(self):
        return (
            self.response.error.message
            if isinstance(self.response, ErrorResponse)
            else self.response
        )


def create_error_response(
    message: str,
    err_type: str = "BadRequestError",
    param: str = "",
    status_code: HTTPStatus = HTTPStatus.BAD_REQUEST,
) -> ErrorResponse:
    error = Error(
        message=message, type=err_type, param=param, code=str(status_code.value)
    )
    return ErrorResponse(error=error)


async def time_series_error_handler(_, exc: TimeSeriesError):
    logger.error("Exception:", exc_info=exc)

    response = (
        exc.response
        if isinstance(exc.response, ErrorResponse)
        else create_error_response(
            message=str(exc),
            err_type=type(exc).__name__,
            param=exc.response.error.param if exc.response.error.param else "",
            status_code=HTTPStatus.BAD_REQUEST,
        )
    )

    return JSONResponse(
        status_code=int(response.error.code),
        content=response.model_dump(),
    )

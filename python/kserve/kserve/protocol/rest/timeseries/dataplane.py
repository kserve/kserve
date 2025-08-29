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


from http import HTTPStatus
from typing import Union, List

from fastapi import Request, Response
from starlette.datastructures import Headers
from kserve.protocol.dataplane import DataPlane
from .time_series_model import (
    HuggingFaceTimeSeriesModel,
    TimeSeriesModel,
)

from .types import (
    ForecastRequest,
    ForecastResponse,
    ErrorResponse,
)
from kserve.protocol.rest.timeseries.error import create_error_response


class TimeSeriesDataPlane(DataPlane):
    """Time Series DataPlane"""

    async def forecast(
        self,
        request_body: ForecastRequest,
        raw_request: Request,
        headers: Headers,
        response: Response,
    ) -> Union[ForecastResponse, ErrorResponse]:
        """Forecast the time series data.

        Args:
            request_body (ForecastRequest): Params to forecast the time series data.
            raw_request (Request): fastapi request object.
            headers: (Headers): Request headers.
            response: (Response): FastAPI response object
        Returns:
            response: A forecast response or an error response.
        """
        model_name = request_body.model
        model = await self.get_model(model_name)
        if not isinstance(model, HuggingFaceTimeSeriesModel):
            return create_error_response(
                message=f"Model {model_name} does not support Time Series API",
                status_code=HTTPStatus.NOT_IMPLEMENTED,
                error_type="not_implemented",
            )

        context = {"headers": dict(headers), "response": response}
        return await model.forecast(request=request_body, context=context)

    async def models(self) -> List[TimeSeriesModel]:
        """Retrieve a list of models

        Returns:
            response: A list of TimeSeriesModel instances
        """
        return [
            model
            for model in self.model_registry.get_models().values()
            if isinstance(model, TimeSeriesModel)
        ]

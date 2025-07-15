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

from fastapi import APIRouter, FastAPI, Request, Response
from fastapi.responses import ORJSONResponse

from kserve.protocol.rest.timeseries.dataplane import TimeSeriesDataPlane
from kserve.protocol.rest.timeseries.types import ForecastRequest, ForecastResponse, ErrorResponse, Error

from kserve.logging import logger


class TimeSeriesEndpoints:
    def __init__(self, dataplane: TimeSeriesDataPlane):
        self.dataplane = dataplane

    async def forecast(
        self,
        request_body: ForecastRequest,
        raw_request: Request,
        response: Response
    ):
        logger.info(f">>> Forecast request: {request_body}")
        request_headers = raw_request.headers
        forecast_response = await self.dataplane.forecast(
            request_body,
            raw_request,
            request_headers,
            response,
        )
        if isinstance(forecast_response, ErrorResponse):
            return ORJSONResponse(
                content=forecast_response.model_dump(), status_code=int(forecast_response.error.code)
            )
        else:
            return forecast_response
        
    async def models(self):
        models = await self.dataplane.models()
        return [model.name for model in models]


def register_time_series_endpoints(app: FastAPI, dataplane: TimeSeriesDataPlane):
    ts_endpoints = TimeSeriesEndpoints(dataplane)
    ts_router = APIRouter()
    ts_router.add_api_route(
        r"/v1/timeseries/forecast",
        ts_endpoints.forecast,
        methods=["POST"],
    )
    ts_router.add_api_route(
        r"/v1/timeseries/models",
        ts_endpoints.models,
        methods=["GET"],
    )
    app.include_router(ts_router)
    logger.info(f">>> Time series endpoints registered")

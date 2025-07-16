from abc import abstractmethod

from fastapi import Request

from .types import (
    ForecastRequest,
    ForecastResponse,
    ErrorResponse,
)
from ....model import BaseKServeModel

from typing import Union, Optional, Dict, Any
import pathlib
import torch
import numpy as np
import uuid
import time

from transformers import (
    AutoModelForTimeSeriesPrediction,
    PretrainedConfig,
)

from kserve.logging import logger
from kserve.protocol.rest.timeseries.types import (
    ForecastRequest,
    ForecastResponse,
    ErrorResponse,
    Error,
    TimeSeriesType,
    TimeSeriesForecast,
    ForecastOutput,
    Status,
    Usage,
    Frequency,
)


class TimeSeriesModel(BaseKServeModel):
    """Time Series Model""" 

    def __init__(self, name: str):
        super().__init__(name)
        self.ready = True

    @abstractmethod
    async def forecast(
        self,
        request: ForecastRequest,
        raw_request: Optional[Request] = None,
        context: Optional[Dict[str, Any]] = None,
    ) -> Union[ForecastResponse, ErrorResponse]:
        """Forecast the time series data.

        Args:
            request: ForecastRequest
            raw_request: Optional[Request]
            context: Optional[Dict[str, Any]]
        Returns:
            ForecastResponse: The forecasted time series data.
            ErrorResponse: The error response if the forecast fails.
        """
        pass


class HuggingFaceTimeSeriesModel(TimeSeriesModel):
    """
    A class to represent a Hugging Face time series model.
    """

    def __init__(
        self,
        model_name: str,
        model_id_or_path: Union[pathlib.Path, str],
        model_config: Optional[PretrainedConfig] = None,
        model_revision: Optional[str] = None,
        dtype: torch.dtype = torch.float16,
    ):
        super().__init__(model_name)
        self._device = torch.device("cuda" if torch.cuda.is_available() else "cpu")
        self.model_id_or_path = model_id_or_path
        self.model_config = model_config
        self.model_revision = model_revision
        self.dtype = dtype

        logger.debug(f"Time Series Model ID or Path: {self.model_id_or_path}")
        logger.debug(f"Time Series Model Config: {self.model_config}")

    def load(self):
        model_kwargs = {}
        model_kwargs["torch_dtype"] = self.dtype

        self._model = AutoModelForTimeSeriesPrediction.from_pretrained(
            self.model_id_or_path,
            revision=self.model_revision,
            device_map=self._device,
            **model_kwargs,
        )
        self._model.eval()
        logger.info(f"Loaded Time Series Model {self._model.__class__.__name__}")

        self.ready = True
        return self.ready

    async def forecast(
        self,
        request: ForecastRequest,
        context: Optional[Dict[str, Any]] = None,
    ) -> Union[ForecastResponse, ErrorResponse]:
        
        if "timesfm" in request.model.lower():
            
            TIMESFM_FREQUENCY_MAP = {
                Frequency.SECOND: 0,
                Frequency.SECOND_SHORT: 0,
                Frequency.MINUTE: 0,
                Frequency.MINUTE_SHORT: 0,
                Frequency.HOUR: 0,
                Frequency.HOUR_SHORT: 0,
                Frequency.DAY: 0,
                Frequency.DAY_SHORT: 0,
                Frequency.WEEK: 1,
                Frequency.WEEK_SHORT: 1,
                Frequency.MONTH: 1,
                Frequency.MONTH_SHORT: 1,
                Frequency.QUARTER: 2,
                Frequency.QUARTER_SHORT: 2,
                Frequency.YEAR: 2,
                Frequency.YEAR_SHORT: 2,
            }
            
            # check if the horizon is valid
            if request.options.horizon > self.model_config.horizon_length:
                return ErrorResponse(
                    error=Error(
                        type="invalid_horizon",
                        message=f"Invalid horizon: {request.options.horizon}"
                    )
                )
            # check if the quantiles are valid
            quantiles_idx = []
            model_quantiles = {q: i for i, q in enumerate(self.model_config.quantiles)}
            for q in request.options.quantiles:
                if q not in model_quantiles:
                    return ErrorResponse(
                        error=Error(
                            type="invalid_quantile",
                            message=f"Invalid quantile: {q}"
                        )
                    )
                # the first quantile is the mean, so we need to add 1 to the index
                quantiles_idx.append(model_quantiles[q] + 1)
            
            forecast_input_tensor = []
            frequency_input_tensor = []
            for input in request.inputs:
                if input.type == TimeSeriesType.UNIVARIATE:
                    forecast_input_tensor.append(torch.tensor(input.series, dtype=self.dtype).to(self._device))
                    try:
                        freq = Frequency(input.frequency)
                    except:
                        return ErrorResponse(
                            error=Error(
                                type="invalid_frequency",
                                message=f"Invalid frequency: {input.frequency}"
                            )
                        )
                    frequency_input_tensor.append(np.float32(TIMESFM_FREQUENCY_MAP[freq]))
                else:
                    return ErrorResponse(
                        error=Error(
                            type="unsupported_time_series_type",
                            message=f"Only univariate time series are supported at this time."
                        )
                    )

            frequency_input_tensor = torch.tensor(frequency_input_tensor, dtype=torch.long).to(self._device)
            model_output = self._model(forecast_input_tensor, frequency_input_tensor, return_dict=True)   
            full_predictions = model_output.full_predictions.cpu().detach().numpy()

            forecast_outputs = []
            for i in range(len(request.inputs)):
                # trim mean prediction to the horizon
                trimmed_point_forecast = full_predictions[i, :request.options.horizon, 0].tolist()
                # format and trim quantiles
                trimmed_quantile_forecast = {}
                for j, q in enumerate(request.options.quantiles):
                    trimmed_quantile_forecast[str(q)] = full_predictions[i, :request.options.horizon, quantiles_idx[j]].tolist()

                ts_output = TimeSeriesForecast(
                    type=TimeSeriesType.UNIVARIATE,
                    name=request.inputs[i].name,
                    mean_forecast=trimmed_point_forecast,
                    frequency=request.inputs[i].frequency,
                    start_timestamp=request.inputs[i].start_timestamp,
                    quantiles=trimmed_quantile_forecast,
                )

                forecast_output = ForecastOutput(
                    type="time_series_forecast",
                    id=str(uuid.uuid4()),
                    status=Status.COMPLETED,
                    content=[ts_output],
                    error=None
                )

                forecast_outputs.append(forecast_output)

            usage = Usage(
                prompt_tokens=0,
                completion_tokens=0,
                total_tokens=0
            )

            forecast_response = ForecastResponse(
                id=str(uuid.uuid4()),
                created_at=int(time.time()),
                status=Status.COMPLETED,
                error=None, 
                model=request.model,
                outputs=forecast_outputs,
                usage=usage
            )

            return forecast_response

        else:
            return ErrorResponse(
                error=Error(
                    type="model_not_found",
                    message=f"Only TimesFM models are supported at this time."
                )
            )

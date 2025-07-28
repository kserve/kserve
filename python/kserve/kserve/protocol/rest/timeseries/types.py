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

from __future__ import annotations

from typing import Dict, List, Optional, Union
from enum import Enum
from pydantic import BaseModel, Field

# the basic time series type
TimeSeries = Union[List[float], List[List[float]]]


class Error(BaseModel):
    code: Optional[str] = Field(None, description="Error code.")
    message: str = Field(..., description="Error message.")
    param: Optional[str] = Field(None, description="Parameter related to the error.")
    type: str = Field(..., description="Type of error.")


class ErrorResponse(BaseModel):
    error: Error


class Frequency(str, Enum):
    """Frequency of the time series data."""

    SECOND = "second"
    SECOND_SHORT = "S"
    MINUTE = "minute"
    MINUTE_SHORT = "T"
    HOUR = "hour"
    HOUR_SHORT = "H"
    DAY = "day"
    DAY_SHORT = "D"
    WEEK = "week"
    WEEK_SHORT = "W"
    MONTH = "month"
    MONTH_SHORT = "M"
    QUARTER = "quarter"
    QUARTER_SHORT = "Q"
    YEAR = "year"
    YEAR_SHORT = "Y"


class Status(str, Enum):
    """Status of the overall request or an individual output."""

    COMPLETED = "completed"
    ERROR = "error"
    PENDING = "pending"
    PARTIAL = "partial"


class TimeSeriesType(str, Enum):
    """Type of the time series data."""

    UNIVARIATE = "univariate_time_series"
    MULTIVARIATE = "multivariate_time_series"


class TimeSeriesInput(BaseModel):
    type: TimeSeriesType = Field(
        ..., description="Whether the time series is univariate or multivariate."
    )
    name: str = Field(..., description="The name of the time series.")
    series: TimeSeries = Field(
        ...,
        description="The observed time series data. List[float] (univariate) or List[List[float]] (multivariate).",
    )
    frequency: Frequency = Field(..., description="The frequency of the time series.")
    start_timestamp: Optional[str] = Field(
        None, description="ISO8601 start timestamp of the series."
    )
    model_config = {"extra": "allow"}


class ForecastOptions(BaseModel):
    horizon: int = Field(..., description="The number of steps to forecast.")
    quantiles: Optional[List[float]] = Field(
        None, description="Quantiles to forecast, e.g., [0.1, 0.5, 0.9]."
    )
    model_config = {"extra": "allow"}


class Metadata(BaseModel):
    model_config = {"extra": "allow"}


class ForecastRequest(BaseModel):
    model: str = Field(..., description="The model to use for forecasting.")
    inputs: List[TimeSeriesInput] = Field(..., description="The input time series.")
    options: ForecastOptions = Field(
        ..., description="Forecasting options and hyperparameters."
    )
    metadata: Optional[Metadata] = Field(
        None, description="Optional user-provided metadata."
    )
    model_config = {"extra": "allow"}


class TimeSeriesForecast(BaseModel):
    type: TimeSeriesType = Field(
        ...,
        description="Whether the forecast is for a univariate or multivariate time series.",
    )
    name: str = Field(..., description="The name of the time series.")
    mean_forecast: TimeSeries = Field(
        ...,
        description="The mean forecasted values. List[float] (univariate) or List[List[float]] (multivariate).",
    )
    frequency: Frequency = Field(..., description="The frequency of the time series.")
    start_timestamp: str = Field(
        ..., description="ISO8601 start timestamp of the forecast."
    )
    quantiles: Optional[Dict[str, TimeSeries]] = Field(
        None, description="Optional: Quantile forecasts for each horizon."
    )
    model_config = {"extra": "allow"}


class ForecastOutput(BaseModel):
    type: str = Field(..., description="Type of output.")
    id: str = Field(..., description="Unique forecast identifier.")
    status: Status = Field(..., description="Status of this forecast result.")
    content: List[TimeSeriesForecast] = Field(
        ..., description="The list of forecast results (one per input time series)."
    )
    error: Optional[Error] = Field(
        None, description="Error details if the forecast failed."
    )
    model_config = {"extra": "allow"}


class Usage(BaseModel):
    prompt_tokens: int = Field(
        ..., description="Number of tokens in prompt (if using LLM-style accounting)."
    )
    completion_tokens: int = Field(
        ..., description="Number of tokens generated in completion."
    )
    total_tokens: int = Field(..., description="Total tokens used.")
    model_config = {"extra": "allow"}


class ForecastResponse(BaseModel):
    id: str = Field(..., description="Unique response identifier.")
    created_at: int = Field(..., description="Unix timestamp of response creation.")
    status: Status = Field(..., description="Overall status of the request.")
    error: Optional[Error] = Field(None, description="Top-level error details, if any.")
    model: str = Field(..., description="The model used for forecasting.")
    outputs: List[ForecastOutput] = Field(..., description="List of forecast outputs.")
    usage: Optional[Usage] = Field(None, description="Token usage information.")
    model_config = {"extra": "allow"}

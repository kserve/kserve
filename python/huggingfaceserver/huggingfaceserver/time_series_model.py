from typing import Union, Optional
import pathlib
import torch

from kserve.model import BaseKServeModel
from .task import (
    MLTask,
    get_model_class_for_task,
)


class HuggingFaceTimeSeriesModel(BaseKServeModel):
    """
    A class to represent a Hugging Face time series model.
    """

    def __init__(
        self,
        model_name: str,
        model_id_or_path: Union[pathlib.Path, str],
        task: Optional[MLTask] = None,
        model_revision: Optional[str] = None,
        dtype: torch.dtype = torch.float16,
    ):
        super().__init__(model_name)
        self._device = torch.device("cuda" if torch.cuda.is_available() else "cpu")
        self.model_id_or_path = model_id_or_path
        self.model_revision = model_revision
        self.task = task
        self.dtype = dtype

    def load(self):
        model_kwargs = {}
        model_kwargs["torch_dtype"] = self.dtype
        
        model_cls = get_model_class_for_task(self.task)
        self._model = model_cls.from_pretrained(
            self.model_id_or_path,
            revision=self.model_revision,
            device_map=self._device,
            **model_kwargs,
        )
        self._model.eval()

        self.ready = True
        return self.ready

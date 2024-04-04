import argparse
import logging
import json
from uuid import uuid4
from typing import Dict, List, Union

import kserve
from kserve.protocol.infer_type import (
    InferInput,
    InferOutput,
    InferRequest,
    InferResponse,
)
from kserve.protocol.grpc.grpc_predict_v2_pb2 import ModelInferResponse
import numpy as np
from transformers import AutoTokenizer
from pydantic import BaseModel
import multiprocessing as mp

mp.set_start_method("fork")


logging.basicConfig(level=kserve.constants.KSERVE_LOGLEVEL)
logger = logging.getLogger(__name__)


def get_output(outputs: List[InferOutput], name: str) -> InferOutput:
    for o in outputs:
        if o.name == name:
            return o
    raise KeyError("Unknown output name: {}".format(name))


class Input(BaseModel):
    input: str
    output_len: int


class Request(BaseModel):
    inputs: List[Input]


class Transformer(kserve.Model):
    def __init__(
        self,
        name: str,
        predictor_host: str,
        protocol: str,
        tokenizer_path: str,
    ):
        super().__init__(name)
        self.predictor_host = predictor_host
        self.protocol = protocol
        self.tokenizer = AutoTokenizer.from_pretrained(
            tokenizer_path,
            local_files_only=True,
        )
        logger.info(self.tokenizer)

    def preprocess(self, _request: Dict, headers: Dict) -> InferRequest:
        request = Request(**_request)
        input_token_ids, input_lengths = self._tokenize_input(request)
        output_lens = np.array(
            [[i.output_len] for i in request.inputs], dtype=np.uint32
        )
        infer_inputs = [
            InferInput(
                name="input_ids",
                shape=input_token_ids.shape,
                datatype="UINT32",
                data=input_token_ids,
            ),
            InferInput(
                name="input_lengths",
                shape=input_lengths.shape,
                datatype="UINT32",
                data=input_lengths,
            ),
            InferInput(
                name="request_output_len",
                shape=output_lens.shape,
                datatype="UINT32",
                data=output_lens,
            ),
        ]
        return InferRequest(
            self.name, infer_inputs=infer_inputs, request_id=str(uuid4())
        )

    def postprocess(
        self, response: Union[ModelInferResponse, Dict], headers: Dict
    ) -> str:
        if isinstance(response, ModelInferResponse):
            outputs = InferResponse.from_grpc(response).outputs
        else:
            outputs = [InferOutput(**o) for o in response["outputs"]]
        output_ids = get_output(outputs, "output_ids").as_numpy()
        results = []
        for o in output_ids:
            outputs = [self.tokenizer.decode(beam) for beam in o]
            results.append(outputs)
        return json.dumps(results)

    def _tokenize_input(self, request: Request):
        """
        Convert input strings to tokens
        """
        inputs = [i.input for i in request.inputs]
        encoded_inputs = self.tokenizer(inputs, padding=True, return_tensors="np")
        input_token_ids = encoded_inputs["input_ids"].astype(np.uint32)
        input_lengths = (
            encoded_inputs["attention_mask"]
            .sum(axis=-1, dtype=np.uint32)
            .reshape((-1, 1))
        )
        input_lengths = np.array(input_lengths, dtype=np.uint32)
        return input_token_ids, input_lengths


if __name__ == "__main__":
    parser = argparse.ArgumentParser(parents=[kserve.model_server.parser])
    parser.add_argument("--model_name", help="The name that the model is served under.")
    parser.add_argument(
        "--predictor_host", help="The URL for the model predict function", required=True
    )
    parser.add_argument(
        "--protocol", help="The protocol for the predictor", default="v2"
    )
    parser.add_argument(
        "--tokenizer_path", help="The path to the tokenizer", required=True
    )
    args, _ = parser.parse_known_args()

    transformer = Transformer(
        name=args.model_name,
        predictor_host=args.predictor_host,
        protocol=args.protocol,
        tokenizer_path=args.tokenizer_path,
    )
    server = kserve.ModelServer()
    server.start(models=[transformer])

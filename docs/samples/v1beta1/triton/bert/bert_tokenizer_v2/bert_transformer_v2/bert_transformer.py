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

import kserve
from typing import Dict
import numpy as np
from . import tokenization
from . import data_processing
import tritonclient.http as httpclient
import logging

logging.basicConfig(level=logging.DEBUG)


class BertTransformer(kserve.Model):
    def __init__(self, name: str, predictor_host: str):
        super().__init__(name)
        self.short_paragraph_text = "The Apollo program was the third United States human spaceflight program. " \
                                    "First conceived as a three-man spacecraft to follow the one-man Project Mercury " \
                                    "which put the first Americans in space, Apollo was dedicated to President" \
                                    " John F. Kennedy's national goal of landing a man on the Moon. The first manned " \
                                    "flight of Apollo was in 1968. Apollo ran from 1961 to 1972 followed by " \
                                    "the Apollo-Soyuz Test Project a joint Earth orbit mission with " \
                                    "the Soviet Union in 1975."

        self.predictor_host = predictor_host
        self.tokenizer = tokenization.FullTokenizer(vocab_file="/mnt/models/vocab.txt", do_lower_case=True)
        self.model_name = "bert_tf_v2_large_fp16_128_v2"
        self.triton_client = None

    def preprocess(self, inputs: Dict) -> Dict:
        self.doc_tokens = data_processing.convert_doc_tokens(self.short_paragraph_text)
        self.features = data_processing.convert_examples_to_features(self.doc_tokens, inputs["instances"][0],
                                                                     self.tokenizer, 128, 128, 64)
        return self.features

    def predict(self, features: Dict) -> Dict:
        if not self.triton_client:
            self.triton_client = httpclient.InferenceServerClient(
                url=self.predictor_host, verbose=True)

        unique_ids = np.zeros([1, 1], dtype=np.int32)
        segment_ids = features["segment_ids"].reshape(1, 128)
        input_ids = features["input_ids"].reshape(1, 128)
        input_mask = features["input_mask"].reshape(1, 128)

        inputs = [httpclient.InferInput('unique_ids', [1, 1], "INT32"),
                  httpclient.InferInput('segment_ids', [1, 128], "INT32"),
                  httpclient.InferInput('input_ids', [1, 128], "INT32"),
                  httpclient.InferInput('input_mask', [1, 128], "INT32")]
        inputs[0].set_data_from_numpy(unique_ids)
        inputs[1].set_data_from_numpy(segment_ids)
        inputs[2].set_data_from_numpy(input_ids)
        inputs[3].set_data_from_numpy(input_mask)

        outputs = [httpclient.InferRequestedOutput('start_logits', binary_data=False),
                   httpclient.InferRequestedOutput('end_logits', binary_data=False)]
        result = self.triton_client.infer(self.model_name, inputs, outputs=outputs)
        return result.get_response()

    def postprocess(self, result: Dict) -> Dict:
        logging.info(result)
        end_logits = result['outputs'][0]['data']
        start_logits = result['outputs'][1]['data']
        n_best_size = 20

        # The maximum length of an answer that can be generated. This is needed
        #  because the start and end predictions are not conditioned on one another
        max_answer_length = 30

        (prediction, nbest_json, scores_diff_json) = \
            data_processing.get_predictions(self.doc_tokens, self.features, start_logits, end_logits, n_best_size,
                                            max_answer_length)
        return {"predictions": prediction, "prob": nbest_json[0]['probability'] * 100.0}

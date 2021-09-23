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
from tensorrtserver.api import InferContext, ProtocolType


class BertTransformer(kserve.KFModel):
    def __init__(self, name: str, predictor_host: str):
        super().__init__(name)
        self.short_paragraph_text = "The Apollo program was the third United States human spaceflight program. " \
                                    "First conceived as a three-man spacecraft to follow the one-man Project Mercury " \
                                    "which put the first Americans in space, Apollo was dedicated to " \
                                    "President John F. Kennedy's national goal of landing a man on the Moon. " \
                                    "The first manned flight of Apollo was in 1968. Apollo ran from 1961 to 1972" \
                                    " followed by the Apollo-Soyuz Test Project a joint Earth orbit mission with " \
                                    "the Soviet Union in 1975."

        self.predictor_host = predictor_host
        self.tokenizer = tokenization.FullTokenizer(vocab_file="/mnt/models/vocab.txt", do_lower_case=True)
        self.model_name = "bert_tf_v2_large_fp16_128_v2"
        self.model_version = -1
        self.protocol = ProtocolType.from_str('http')
        self.infer_ctx = None

    def preprocess(self, inputs: Dict) -> Dict:
        self.doc_tokens = data_processing.convert_doc_tokens(self.short_paragraph_text)
        self.features = data_processing.convert_examples_to_features(self.doc_tokens, inputs["instances"][0],
                                                                     self.tokenizer, 128, 128, 64)
        return self.features

    def predict(self, features: Dict) -> Dict:
        if not self.infer_ctx:
            self.infer_ctx = InferContext(self.predictor_host, self.protocol, self.model_name, self.model_version,
                                          http_headers='', verbose=True)

        batch_size = 1
        unique_ids = np.int32([1])
        segment_ids = features["segment_ids"]
        input_ids = features["input_ids"]
        input_mask = features["input_mask"]
        result = self.infer_ctx.run({'unique_ids': (unique_ids,),
                                     'segment_ids': (segment_ids,),
                                     'input_ids': (input_ids,),
                                     'input_mask': (input_mask,)},
                                    {'end_logits': InferContext.ResultFormat.RAW,
                                     'start_logits': InferContext.ResultFormat.RAW}, batch_size)
        return result

    def postprocess(self, result: Dict) -> Dict:
        end_logits = result['end_logits'][0]
        start_logits = result['start_logits'][0]
        n_best_size = 20

        # The maximum length of an answer that can be generated. This is needed
        #  because the start and end predictions are not conditioned on one another
        max_answer_length = 30

        (prediction, nbest_json, scores_diff_json) = \
            data_processing.get_predictions(self.doc_tokens, self.features, start_logits, end_logits, n_best_size,
                                            max_answer_length)
        return {"predictions": prediction, "prob": nbest_json[0]['probability'] * 100.0}

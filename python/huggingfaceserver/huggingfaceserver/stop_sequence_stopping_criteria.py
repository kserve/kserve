# Copyright 2023 The KServe Authors.
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

from typing import List

import torch
from transformers import StoppingCriteria


class StopSequenceStoppingCriteria(StoppingCriteria):
    """
    This class can be used to stop generation whenever a sequence is encountered in the generated output.

    Note: when processing batched requests we will stop if _any_ of the sequences in the batch match a stop sequence.
    """

    stop_sequences: List[torch.LongTensor]
    start_length: int
    triggered: bool = False

    def __init__(self, input_length: int, stop_sequences: List[torch.LongTensor]):
        self.input_length = input_length
        self.stop_sequences = stop_sequences

    def __call__(
        self, input_ids: torch.LongTensor, scores: torch.FloatTensor, **kwargs
    ) -> bool:
        for seq in self.stop_sequences:
            # Make sure we have generated enough tokens to check this sequence against
            if seq.shape[-1] > input_ids.shape[-1] - self.input_length:
                continue
            # If any of the sequences in the batch match the stop sequence we should
            # stop generation
            if torch.any(torch.all(input_ids[:, -len(seq) :] == seq, dim=1)):
                self.triggered = True
                return True
        return False

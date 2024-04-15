# Copyright 2024 The KServe Authors.
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

from asyncio import Queue
from typing import Optional

from transformers import TextStreamer, AutoTokenizer


class AsyncGenerateStream(TextStreamer):
    """A stream of GenerateOutputs for a request that can be
    iterated over asynchronously."""

    def __init__(
        self,
        tokenizer: AutoTokenizer,
        skip_prompt: bool = False,
        timeout: Optional[float] = None,
        **decode_kwargs,
    ):
        super().__init__(tokenizer, skip_prompt, **decode_kwargs)
        self.text_queue = Queue()
        self.stop_signal = None
        self.timeout = timeout

    def on_finalized_text(self, text: str, stream_end: bool = False):
        """Put the new text in the queue. If the stream is ending, also put a stop signal in the queue."""
        self.text_queue.put_nowait(text)
        if stream_end:
            self.text_queue.put_nowait(self.stop_signal)

    def __aiter__(self):
        return self

    def __anext__(self):
        value = self.text_queue.get()
        if value == self.stop_signal:
            raise StopIteration()
        else:
            return value

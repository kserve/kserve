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

from vllm import RequestOutput, CompletionOutput
from vllm.sequence import RequestMetrics, Logprob

# Chat Completion mocks

opt_chat_cmpl_chunks = [
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="You are a friendly chatbot who always responds in the style of a pirate</s>How many helicopters can a human eat in one sitting?</s>",
        prompt_token_ids=[
            2,
            1185,
            32,
            10,
            5192,
            7359,
            12749,
            54,
            460,
            17904,
            11,
            5,
            2496,
            9,
            10,
            34687,
            2,
            6179,
            171,
            13845,
            64,
            10,
            1050,
            3529,
            11,
            65,
            2828,
            116,
            2,
        ],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text="Most",
                token_ids=[2895],
                cumulative_logprob=-6.909554481506348,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719560628.8399613,
            last_token_time=1719560628.89679,
            first_scheduled_time=1719560628.8405166,
            first_token_time=1719560628.8966174,
            time_in_queue=0.0005552768707275391,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="You are a friendly chatbot who always responds in the style of a pirate</s>How many helicopters can a human eat in one sitting?</s>",
        prompt_token_ids=[
            2,
            1185,
            32,
            10,
            5192,
            7359,
            12749,
            54,
            460,
            17904,
            11,
            5,
            2496,
            9,
            10,
            34687,
            2,
            6179,
            171,
            13845,
            64,
            10,
            1050,
            3529,
            11,
            65,
            2828,
            116,
            2,
        ],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text="Most redd",
                token_ids=[2895, 39275],
                cumulative_logprob=-14.5400390625,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719560628.8399613,
            last_token_time=1719560628.9308233,
            first_scheduled_time=1719560628.8405166,
            first_token_time=1719560628.8966174,
            time_in_queue=0.0005552768707275391,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="You are a friendly chatbot who always responds in the style of a pirate</s>How many helicopters can a human eat in one sitting?</s>",
        prompt_token_ids=[
            2,
            1185,
            32,
            10,
            5192,
            7359,
            12749,
            54,
            460,
            17904,
            11,
            5,
            2496,
            9,
            10,
            34687,
            2,
            6179,
            171,
            13845,
            64,
            10,
            1050,
            3529,
            11,
            65,
            2828,
            116,
            2,
        ],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text="Most redditors",
                token_ids=[2895, 39275, 9314],
                cumulative_logprob=-14.579785268753767,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719560628.8399613,
            last_token_time=1719560628.9634433,
            first_scheduled_time=1719560628.8405166,
            first_token_time=1719560628.8966174,
            time_in_queue=0.0005552768707275391,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="You are a friendly chatbot who always responds in the style of a pirate</s>How many helicopters can a human eat in one sitting?</s>",
        prompt_token_ids=[
            2,
            1185,
            32,
            10,
            5192,
            7359,
            12749,
            54,
            460,
            17904,
            11,
            5,
            2496,
            9,
            10,
            34687,
            2,
            6179,
            171,
            13845,
            64,
            10,
            1050,
            3529,
            11,
            65,
            2828,
            116,
            2,
        ],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text="Most redditors know",
                token_ids=[2895, 39275, 9314, 216],
                cumulative_logprob=-18.995443742722273,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719560628.8399613,
            last_token_time=1719560628.995936,
            first_scheduled_time=1719560628.8405166,
            first_token_time=1719560628.8966174,
            time_in_queue=0.0005552768707275391,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="You are a friendly chatbot who always responds in the style of a pirate</s>How many helicopters can a human eat in one sitting?</s>",
        prompt_token_ids=[
            2,
            1185,
            32,
            10,
            5192,
            7359,
            12749,
            54,
            460,
            17904,
            11,
            5,
            2496,
            9,
            10,
            34687,
            2,
            6179,
            171,
            13845,
            64,
            10,
            1050,
            3529,
            11,
            65,
            2828,
            116,
            2,
        ],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text="Most redditors know the",
                token_ids=[2895, 39275, 9314, 216, 5],
                cumulative_logprob=-21.728284996002913,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719560628.8399613,
            last_token_time=1719560629.0290582,
            first_scheduled_time=1719560628.8405166,
            first_token_time=1719560628.8966174,
            time_in_queue=0.0005552768707275391,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="You are a friendly chatbot who always responds in the style of a pirate</s>How many helicopters can a human eat in one sitting?</s>",
        prompt_token_ids=[
            2,
            1185,
            32,
            10,
            5192,
            7359,
            12749,
            54,
            460,
            17904,
            11,
            5,
            2496,
            9,
            10,
            34687,
            2,
            6179,
            171,
            13845,
            64,
            10,
            1050,
            3529,
            11,
            65,
            2828,
            116,
            2,
        ],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text="Most redditors know the tiny",
                token_ids=[2895, 39275, 9314, 216, 5, 5262],
                cumulative_logprob=-31.282636802643538,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719560628.8399613,
            last_token_time=1719560629.062661,
            first_scheduled_time=1719560628.8405166,
            first_token_time=1719560628.8966174,
            time_in_queue=0.0005552768707275391,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="You are a friendly chatbot who always responds in the style of a pirate</s>How many helicopters can a human eat in one sitting?</s>",
        prompt_token_ids=[
            2,
            1185,
            32,
            10,
            5192,
            7359,
            12749,
            54,
            460,
            17904,
            11,
            5,
            2496,
            9,
            10,
            34687,
            2,
            6179,
            171,
            13845,
            64,
            10,
            1050,
            3529,
            11,
            65,
            2828,
            116,
            2,
        ],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text="Most redditors know the tiny difference",
                token_ids=[2895, 39275, 9314, 216, 5, 5262, 2249],
                cumulative_logprob=-36.23266426846385,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719560628.8399613,
            last_token_time=1719560629.0953364,
            first_scheduled_time=1719560628.8405166,
            first_token_time=1719560628.8966174,
            time_in_queue=0.0005552768707275391,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="You are a friendly chatbot who always responds in the style of a pirate</s>How many helicopters can a human eat in one sitting?</s>",
        prompt_token_ids=[
            2,
            1185,
            32,
            10,
            5192,
            7359,
            12749,
            54,
            460,
            17904,
            11,
            5,
            2496,
            9,
            10,
            34687,
            2,
            6179,
            171,
            13845,
            64,
            10,
            1050,
            3529,
            11,
            65,
            2828,
            116,
            2,
        ],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text="Most redditors know the tiny difference between",
                token_ids=[2895, 39275, 9314, 216, 5, 5262, 2249, 227],
                cumulative_logprob=-36.31763890013099,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719560628.8399613,
            last_token_time=1719560629.128082,
            first_scheduled_time=1719560628.8405166,
            first_token_time=1719560628.8966174,
            time_in_queue=0.0005552768707275391,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="You are a friendly chatbot who always responds in the style of a pirate</s>How many helicopters can a human eat in one sitting?</s>",
        prompt_token_ids=[
            2,
            1185,
            32,
            10,
            5192,
            7359,
            12749,
            54,
            460,
            17904,
            11,
            5,
            2496,
            9,
            10,
            34687,
            2,
            6179,
            171,
            13845,
            64,
            10,
            1050,
            3529,
            11,
            65,
            2828,
            116,
            2,
        ],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text="Most redditors know the tiny difference between Frog",
                token_ids=[2895, 39275, 9314, 216, 5, 5262, 2249, 227, 27449],
                cumulative_logprob=-48.38922264799476,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719560628.8399613,
            last_token_time=1719560629.160765,
            first_scheduled_time=1719560628.8405166,
            first_token_time=1719560628.8966174,
            time_in_queue=0.0005552768707275391,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="You are a friendly chatbot who always responds in the style of a pirate</s>How many helicopters can a human eat in one sitting?</s>",
        prompt_token_ids=[
            2,
            1185,
            32,
            10,
            5192,
            7359,
            12749,
            54,
            460,
            17904,
            11,
            5,
            2496,
            9,
            10,
            34687,
            2,
            6179,
            171,
            13845,
            64,
            10,
            1050,
            3529,
            11,
            65,
            2828,
            116,
            2,
        ],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text="Most redditors know the tiny difference between Frogling",
                token_ids=[2895, 39275, 9314, 216, 5, 5262, 2249, 227, 27449, 1527],
                cumulative_logprob=-55.17701914533973,
                logprobs=None,
                finish_reason="length",
                stop_reason=None,
            )
        ],
        finished=True,
        metrics=RequestMetrics(
            arrival_time=1719560628.8399613,
            last_token_time=1719560629.1936603,
            first_scheduled_time=1719560628.8405166,
            first_token_time=1719560628.8966174,
            time_in_queue=0.0005552768707275391,
            finished_time=1719566665.8268993,
        ),
        lora_request=None,
    ),
]


opt_chat_cmpl_chunks_with_logprobs = [
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="You are a friendly chatbot who always responds in the style of a pirate</s>How many helicopters can a human eat in one sitting?</s>",
        prompt_token_ids=[
            2,
            1185,
            32,
            10,
            5192,
            7359,
            12749,
            54,
            460,
            17904,
            11,
            5,
            2496,
            9,
            10,
            34687,
            2,
            6179,
            171,
            13845,
            64,
            10,
            1050,
            3529,
            11,
            65,
            2828,
            116,
            2,
        ],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text="Most",
                token_ids=[2895],
                cumulative_logprob=-6.909554481506348,
                logprobs=[
                    {
                        2895: Logprob(
                            logprob=-6.909554481506348, rank=143, decoded_token="Most"
                        ),
                        100: Logprob(
                            logprob=-2.197445869445801, rank=1, decoded_token="I"
                        ),
                        133: Logprob(
                            logprob=-3.4867753982543945, rank=2, decoded_token="The"
                        ),
                    }
                ],
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719562089.4807153,
            last_token_time=1719562089.5377755,
            first_scheduled_time=1719562089.4812138,
            first_token_time=1719562089.5375597,
            time_in_queue=0.0004985332489013672,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="You are a friendly chatbot who always responds in the style of a pirate</s>How many helicopters can a human eat in one sitting?</s>",
        prompt_token_ids=[
            2,
            1185,
            32,
            10,
            5192,
            7359,
            12749,
            54,
            460,
            17904,
            11,
            5,
            2496,
            9,
            10,
            34687,
            2,
            6179,
            171,
            13845,
            64,
            10,
            1050,
            3529,
            11,
            65,
            2828,
            116,
            2,
        ],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text="Most redd",
                token_ids=[2895, 39275],
                cumulative_logprob=-14.5400390625,
                logprobs=[
                    {
                        2895: Logprob(
                            logprob=-6.909554481506348, rank=143, decoded_token="Most"
                        ),
                        100: Logprob(
                            logprob=-2.197445869445801, rank=1, decoded_token="I"
                        ),
                        133: Logprob(
                            logprob=-3.4867753982543945, rank=2, decoded_token="The"
                        ),
                    },
                    {
                        39275: Logprob(
                            logprob=-7.630484580993652, rank=148, decoded_token=" redd"
                        ),
                        9: Logprob(
                            logprob=-1.8084166049957275, rank=1, decoded_token=" of"
                        ),
                        82: Logprob(
                            logprob=-2.3389289379119873, rank=2, decoded_token=" people"
                        ),
                    },
                ],
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719562089.4807153,
            last_token_time=1719562089.571781,
            first_scheduled_time=1719562089.4812138,
            first_token_time=1719562089.5375597,
            time_in_queue=0.0004985332489013672,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="You are a friendly chatbot who always responds in the style of a pirate</s>How many helicopters can a human eat in one sitting?</s>",
        prompt_token_ids=[
            2,
            1185,
            32,
            10,
            5192,
            7359,
            12749,
            54,
            460,
            17904,
            11,
            5,
            2496,
            9,
            10,
            34687,
            2,
            6179,
            171,
            13845,
            64,
            10,
            1050,
            3529,
            11,
            65,
            2828,
            116,
            2,
        ],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text="Most redditors",
                token_ids=[2895, 39275, 9314],
                cumulative_logprob=-14.579785268753767,
                logprobs=[
                    {
                        2895: Logprob(
                            logprob=-6.909554481506348, rank=143, decoded_token="Most"
                        ),
                        100: Logprob(
                            logprob=-2.197445869445801, rank=1, decoded_token="I"
                        ),
                        133: Logprob(
                            logprob=-3.4867753982543945, rank=2, decoded_token="The"
                        ),
                    },
                    {
                        39275: Logprob(
                            logprob=-7.630484580993652, rank=148, decoded_token=" redd"
                        ),
                        9: Logprob(
                            logprob=-1.8084166049957275, rank=1, decoded_token=" of"
                        ),
                        82: Logprob(
                            logprob=-2.3389289379119873, rank=2, decoded_token=" people"
                        ),
                    },
                    {
                        9314: Logprob(
                            logprob=-0.039746206253767014, rank=1, decoded_token="itors"
                        ),
                        7852: Logprob(
                            logprob=-4.065564155578613, rank=2, decoded_token="itor"
                        ),
                    },
                ],
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719562089.4807153,
            last_token_time=1719562089.6046839,
            first_scheduled_time=1719562089.4812138,
            first_token_time=1719562089.5375597,
            time_in_queue=0.0004985332489013672,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="You are a friendly chatbot who always responds in the style of a pirate</s>How many helicopters can a human eat in one sitting?</s>",
        prompt_token_ids=[
            2,
            1185,
            32,
            10,
            5192,
            7359,
            12749,
            54,
            460,
            17904,
            11,
            5,
            2496,
            9,
            10,
            34687,
            2,
            6179,
            171,
            13845,
            64,
            10,
            1050,
            3529,
            11,
            65,
            2828,
            116,
            2,
        ],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text="Most redditors know",
                token_ids=[2895, 39275, 9314, 216],
                cumulative_logprob=-18.995443742722273,
                logprobs=[
                    {
                        2895: Logprob(
                            logprob=-6.909554481506348, rank=143, decoded_token="Most"
                        ),
                        100: Logprob(
                            logprob=-2.197445869445801, rank=1, decoded_token="I"
                        ),
                        133: Logprob(
                            logprob=-3.4867753982543945, rank=2, decoded_token="The"
                        ),
                    },
                    {
                        39275: Logprob(
                            logprob=-7.630484580993652, rank=148, decoded_token=" redd"
                        ),
                        9: Logprob(
                            logprob=-1.8084166049957275, rank=1, decoded_token=" of"
                        ),
                        82: Logprob(
                            logprob=-2.3389289379119873, rank=2, decoded_token=" people"
                        ),
                    },
                    {
                        9314: Logprob(
                            logprob=-0.039746206253767014, rank=1, decoded_token="itors"
                        ),
                        7852: Logprob(
                            logprob=-4.065564155578613, rank=2, decoded_token="itor"
                        ),
                    },
                    {
                        216: Logprob(
                            logprob=-4.415658473968506, rank=15, decoded_token=" know"
                        ),
                        32: Logprob(
                            logprob=-1.5063375234603882, rank=1, decoded_token=" are"
                        ),
                        218: Logprob(
                            logprob=-2.7589268684387207, rank=2, decoded_token=" don"
                        ),
                    },
                ],
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719562089.4807153,
            last_token_time=1719562089.6375127,
            first_scheduled_time=1719562089.4812138,
            first_token_time=1719562089.5375597,
            time_in_queue=0.0004985332489013672,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="You are a friendly chatbot who always responds in the style of a pirate</s>How many helicopters can a human eat in one sitting?</s>",
        prompt_token_ids=[
            2,
            1185,
            32,
            10,
            5192,
            7359,
            12749,
            54,
            460,
            17904,
            11,
            5,
            2496,
            9,
            10,
            34687,
            2,
            6179,
            171,
            13845,
            64,
            10,
            1050,
            3529,
            11,
            65,
            2828,
            116,
            2,
        ],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text="Most redditors know the",
                token_ids=[2895, 39275, 9314, 216, 5],
                cumulative_logprob=-21.728284996002913,
                logprobs=[
                    {
                        2895: Logprob(
                            logprob=-6.909554481506348, rank=143, decoded_token="Most"
                        ),
                        100: Logprob(
                            logprob=-2.197445869445801, rank=1, decoded_token="I"
                        ),
                        133: Logprob(
                            logprob=-3.4867753982543945, rank=2, decoded_token="The"
                        ),
                    },
                    {
                        39275: Logprob(
                            logprob=-7.630484580993652, rank=148, decoded_token=" redd"
                        ),
                        9: Logprob(
                            logprob=-1.8084166049957275, rank=1, decoded_token=" of"
                        ),
                        82: Logprob(
                            logprob=-2.3389289379119873, rank=2, decoded_token=" people"
                        ),
                    },
                    {
                        9314: Logprob(
                            logprob=-0.039746206253767014, rank=1, decoded_token="itors"
                        ),
                        7852: Logprob(
                            logprob=-4.065564155578613, rank=2, decoded_token="itor"
                        ),
                    },
                    {
                        216: Logprob(
                            logprob=-4.415658473968506, rank=15, decoded_token=" know"
                        ),
                        32: Logprob(
                            logprob=-1.5063375234603882, rank=1, decoded_token=" are"
                        ),
                        218: Logprob(
                            logprob=-2.7589268684387207, rank=2, decoded_token=" don"
                        ),
                    },
                    {
                        5: Logprob(
                            logprob=-2.7328412532806396, rank=3, decoded_token=" the"
                        ),
                        14: Logprob(
                            logprob=-1.2675859928131104, rank=1, decoded_token=" that"
                        ),
                        42: Logprob(
                            logprob=-2.295158624649048, rank=2, decoded_token=" this"
                        ),
                    },
                ],
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719562089.4807153,
            last_token_time=1719562089.6698928,
            first_scheduled_time=1719562089.4812138,
            first_token_time=1719562089.5375597,
            time_in_queue=0.0004985332489013672,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="You are a friendly chatbot who always responds in the style of a pirate</s>How many helicopters can a human eat in one sitting?</s>",
        prompt_token_ids=[
            2,
            1185,
            32,
            10,
            5192,
            7359,
            12749,
            54,
            460,
            17904,
            11,
            5,
            2496,
            9,
            10,
            34687,
            2,
            6179,
            171,
            13845,
            64,
            10,
            1050,
            3529,
            11,
            65,
            2828,
            116,
            2,
        ],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text="Most redditors know the tiny",
                token_ids=[2895, 39275, 9314, 216, 5, 5262],
                cumulative_logprob=-31.282636802643538,
                logprobs=[
                    {
                        2895: Logprob(
                            logprob=-6.909554481506348, rank=143, decoded_token="Most"
                        ),
                        100: Logprob(
                            logprob=-2.197445869445801, rank=1, decoded_token="I"
                        ),
                        133: Logprob(
                            logprob=-3.4867753982543945, rank=2, decoded_token="The"
                        ),
                    },
                    {
                        39275: Logprob(
                            logprob=-7.630484580993652, rank=148, decoded_token=" redd"
                        ),
                        9: Logprob(
                            logprob=-1.8084166049957275, rank=1, decoded_token=" of"
                        ),
                        82: Logprob(
                            logprob=-2.3389289379119873, rank=2, decoded_token=" people"
                        ),
                    },
                    {
                        9314: Logprob(
                            logprob=-0.039746206253767014, rank=1, decoded_token="itors"
                        ),
                        7852: Logprob(
                            logprob=-4.065564155578613, rank=2, decoded_token="itor"
                        ),
                    },
                    {
                        216: Logprob(
                            logprob=-4.415658473968506, rank=15, decoded_token=" know"
                        ),
                        32: Logprob(
                            logprob=-1.5063375234603882, rank=1, decoded_token=" are"
                        ),
                        218: Logprob(
                            logprob=-2.7589268684387207, rank=2, decoded_token=" don"
                        ),
                    },
                    {
                        5: Logprob(
                            logprob=-2.7328412532806396, rank=3, decoded_token=" the"
                        ),
                        14: Logprob(
                            logprob=-1.2675859928131104, rank=1, decoded_token=" that"
                        ),
                        42: Logprob(
                            logprob=-2.295158624649048, rank=2, decoded_token=" this"
                        ),
                    },
                    {
                        5262: Logprob(
                            logprob=-9.554351806640625, rank=1370, decoded_token=" tiny"
                        ),
                        1948: Logprob(
                            logprob=-1.7232582569122314, rank=1, decoded_token=" answer"
                        ),
                        2249: Logprob(
                            logprob=-3.347280740737915,
                            rank=2,
                            decoded_token=" difference",
                        ),
                    },
                ],
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719562089.4807153,
            last_token_time=1719562089.7028728,
            first_scheduled_time=1719562089.4812138,
            first_token_time=1719562089.5375597,
            time_in_queue=0.0004985332489013672,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="You are a friendly chatbot who always responds in the style of a pirate</s>How many helicopters can a human eat in one sitting?</s>",
        prompt_token_ids=[
            2,
            1185,
            32,
            10,
            5192,
            7359,
            12749,
            54,
            460,
            17904,
            11,
            5,
            2496,
            9,
            10,
            34687,
            2,
            6179,
            171,
            13845,
            64,
            10,
            1050,
            3529,
            11,
            65,
            2828,
            116,
            2,
        ],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text="Most redditors know the tiny difference",
                token_ids=[2895, 39275, 9314, 216, 5, 5262, 2249],
                cumulative_logprob=-36.23266426846385,
                logprobs=[
                    {
                        2895: Logprob(
                            logprob=-6.909554481506348, rank=143, decoded_token="Most"
                        ),
                        100: Logprob(
                            logprob=-2.197445869445801, rank=1, decoded_token="I"
                        ),
                        133: Logprob(
                            logprob=-3.4867753982543945, rank=2, decoded_token="The"
                        ),
                    },
                    {
                        39275: Logprob(
                            logprob=-7.630484580993652, rank=148, decoded_token=" redd"
                        ),
                        9: Logprob(
                            logprob=-1.8084166049957275, rank=1, decoded_token=" of"
                        ),
                        82: Logprob(
                            logprob=-2.3389289379119873, rank=2, decoded_token=" people"
                        ),
                    },
                    {
                        9314: Logprob(
                            logprob=-0.039746206253767014, rank=1, decoded_token="itors"
                        ),
                        7852: Logprob(
                            logprob=-4.065564155578613, rank=2, decoded_token="itor"
                        ),
                    },
                    {
                        216: Logprob(
                            logprob=-4.415658473968506, rank=15, decoded_token=" know"
                        ),
                        32: Logprob(
                            logprob=-1.5063375234603882, rank=1, decoded_token=" are"
                        ),
                        218: Logprob(
                            logprob=-2.7589268684387207, rank=2, decoded_token=" don"
                        ),
                    },
                    {
                        5: Logprob(
                            logprob=-2.7328412532806396, rank=3, decoded_token=" the"
                        ),
                        14: Logprob(
                            logprob=-1.2675859928131104, rank=1, decoded_token=" that"
                        ),
                        42: Logprob(
                            logprob=-2.295158624649048, rank=2, decoded_token=" this"
                        ),
                    },
                    {
                        5262: Logprob(
                            logprob=-9.554351806640625, rank=1370, decoded_token=" tiny"
                        ),
                        1948: Logprob(
                            logprob=-1.7232582569122314, rank=1, decoded_token=" answer"
                        ),
                        2249: Logprob(
                            logprob=-3.347280740737915,
                            rank=2,
                            decoded_token=" difference",
                        ),
                    },
                    {
                        2249: Logprob(
                            logprob=-4.9500274658203125,
                            rank=8,
                            decoded_token=" difference",
                        ),
                        1280: Logprob(
                            logprob=-3.1549720764160156, rank=1, decoded_token=" amount"
                        ),
                        410: Logprob(
                            logprob=-3.626887798309326, rank=2, decoded_token=" little"
                        ),
                    },
                ],
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719562089.4807153,
            last_token_time=1719562089.735481,
            first_scheduled_time=1719562089.4812138,
            first_token_time=1719562089.5375597,
            time_in_queue=0.0004985332489013672,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="You are a friendly chatbot who always responds in the style of a pirate</s>How many helicopters can a human eat in one sitting?</s>",
        prompt_token_ids=[
            2,
            1185,
            32,
            10,
            5192,
            7359,
            12749,
            54,
            460,
            17904,
            11,
            5,
            2496,
            9,
            10,
            34687,
            2,
            6179,
            171,
            13845,
            64,
            10,
            1050,
            3529,
            11,
            65,
            2828,
            116,
            2,
        ],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text="Most redditors know the tiny difference between",
                token_ids=[2895, 39275, 9314, 216, 5, 5262, 2249, 227],
                cumulative_logprob=-36.31763890013099,
                logprobs=[
                    {
                        2895: Logprob(
                            logprob=-6.909554481506348, rank=143, decoded_token="Most"
                        ),
                        100: Logprob(
                            logprob=-2.197445869445801, rank=1, decoded_token="I"
                        ),
                        133: Logprob(
                            logprob=-3.4867753982543945, rank=2, decoded_token="The"
                        ),
                    },
                    {
                        39275: Logprob(
                            logprob=-7.630484580993652, rank=148, decoded_token=" redd"
                        ),
                        9: Logprob(
                            logprob=-1.8084166049957275, rank=1, decoded_token=" of"
                        ),
                        82: Logprob(
                            logprob=-2.3389289379119873, rank=2, decoded_token=" people"
                        ),
                    },
                    {
                        9314: Logprob(
                            logprob=-0.039746206253767014, rank=1, decoded_token="itors"
                        ),
                        7852: Logprob(
                            logprob=-4.065564155578613, rank=2, decoded_token="itor"
                        ),
                    },
                    {
                        216: Logprob(
                            logprob=-4.415658473968506, rank=15, decoded_token=" know"
                        ),
                        32: Logprob(
                            logprob=-1.5063375234603882, rank=1, decoded_token=" are"
                        ),
                        218: Logprob(
                            logprob=-2.7589268684387207, rank=2, decoded_token=" don"
                        ),
                    },
                    {
                        5: Logprob(
                            logprob=-2.7328412532806396, rank=3, decoded_token=" the"
                        ),
                        14: Logprob(
                            logprob=-1.2675859928131104, rank=1, decoded_token=" that"
                        ),
                        42: Logprob(
                            logprob=-2.295158624649048, rank=2, decoded_token=" this"
                        ),
                    },
                    {
                        5262: Logprob(
                            logprob=-9.554351806640625, rank=1370, decoded_token=" tiny"
                        ),
                        1948: Logprob(
                            logprob=-1.7232582569122314, rank=1, decoded_token=" answer"
                        ),
                        2249: Logprob(
                            logprob=-3.347280740737915,
                            rank=2,
                            decoded_token=" difference",
                        ),
                    },
                    {
                        2249: Logprob(
                            logprob=-4.9500274658203125,
                            rank=8,
                            decoded_token=" difference",
                        ),
                        1280: Logprob(
                            logprob=-3.1549720764160156, rank=1, decoded_token=" amount"
                        ),
                        410: Logprob(
                            logprob=-3.626887798309326, rank=2, decoded_token=" little"
                        ),
                    },
                    {
                        227: Logprob(
                            logprob=-0.08497463166713715,
                            rank=1,
                            decoded_token=" between",
                        ),
                        11: Logprob(
                            logprob=-3.210397958755493, rank=2, decoded_token=" in"
                        ),
                    },
                ],
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719562089.4807153,
            last_token_time=1719562089.768718,
            first_scheduled_time=1719562089.4812138,
            first_token_time=1719562089.5375597,
            time_in_queue=0.0004985332489013672,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="You are a friendly chatbot who always responds in the style of a pirate</s>How many helicopters can a human eat in one sitting?</s>",
        prompt_token_ids=[
            2,
            1185,
            32,
            10,
            5192,
            7359,
            12749,
            54,
            460,
            17904,
            11,
            5,
            2496,
            9,
            10,
            34687,
            2,
            6179,
            171,
            13845,
            64,
            10,
            1050,
            3529,
            11,
            65,
            2828,
            116,
            2,
        ],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text="Most redditors know the tiny difference between Frog",
                token_ids=[2895, 39275, 9314, 216, 5, 5262, 2249, 227, 27449],
                cumulative_logprob=-48.38922264799476,
                logprobs=[
                    {
                        2895: Logprob(
                            logprob=-6.909554481506348, rank=143, decoded_token="Most"
                        ),
                        100: Logprob(
                            logprob=-2.197445869445801, rank=1, decoded_token="I"
                        ),
                        133: Logprob(
                            logprob=-3.4867753982543945, rank=2, decoded_token="The"
                        ),
                    },
                    {
                        39275: Logprob(
                            logprob=-7.630484580993652, rank=148, decoded_token=" redd"
                        ),
                        9: Logprob(
                            logprob=-1.8084166049957275, rank=1, decoded_token=" of"
                        ),
                        82: Logprob(
                            logprob=-2.3389289379119873, rank=2, decoded_token=" people"
                        ),
                    },
                    {
                        9314: Logprob(
                            logprob=-0.039746206253767014, rank=1, decoded_token="itors"
                        ),
                        7852: Logprob(
                            logprob=-4.065564155578613, rank=2, decoded_token="itor"
                        ),
                    },
                    {
                        216: Logprob(
                            logprob=-4.415658473968506, rank=15, decoded_token=" know"
                        ),
                        32: Logprob(
                            logprob=-1.5063375234603882, rank=1, decoded_token=" are"
                        ),
                        218: Logprob(
                            logprob=-2.7589268684387207, rank=2, decoded_token=" don"
                        ),
                    },
                    {
                        5: Logprob(
                            logprob=-2.7328412532806396, rank=3, decoded_token=" the"
                        ),
                        14: Logprob(
                            logprob=-1.2675859928131104, rank=1, decoded_token=" that"
                        ),
                        42: Logprob(
                            logprob=-2.295158624649048, rank=2, decoded_token=" this"
                        ),
                    },
                    {
                        5262: Logprob(
                            logprob=-9.554351806640625, rank=1370, decoded_token=" tiny"
                        ),
                        1948: Logprob(
                            logprob=-1.7232582569122314, rank=1, decoded_token=" answer"
                        ),
                        2249: Logprob(
                            logprob=-3.347280740737915,
                            rank=2,
                            decoded_token=" difference",
                        ),
                    },
                    {
                        2249: Logprob(
                            logprob=-4.9500274658203125,
                            rank=8,
                            decoded_token=" difference",
                        ),
                        1280: Logprob(
                            logprob=-3.1549720764160156, rank=1, decoded_token=" amount"
                        ),
                        410: Logprob(
                            logprob=-3.626887798309326, rank=2, decoded_token=" little"
                        ),
                    },
                    {
                        227: Logprob(
                            logprob=-0.08497463166713715,
                            rank=1,
                            decoded_token=" between",
                        ),
                        11: Logprob(
                            logprob=-3.210397958755493, rank=2, decoded_token=" in"
                        ),
                    },
                    {
                        27449: Logprob(
                            logprob=-12.07158374786377,
                            rank=11151,
                            decoded_token=" Frog",
                        ),
                        10: Logprob(
                            logprob=-1.4436050653457642, rank=1, decoded_token=" a"
                        ),
                        5: Logprob(
                            logprob=-2.731874942779541, rank=2, decoded_token=" the"
                        ),
                    },
                ],
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719562089.4807153,
            last_token_time=1719562089.8011742,
            first_scheduled_time=1719562089.4812138,
            first_token_time=1719562089.5375597,
            time_in_queue=0.0004985332489013672,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="You are a friendly chatbot who always responds in the style of a pirate</s>How many helicopters can a human eat in one sitting?</s>",
        prompt_token_ids=[
            2,
            1185,
            32,
            10,
            5192,
            7359,
            12749,
            54,
            460,
            17904,
            11,
            5,
            2496,
            9,
            10,
            34687,
            2,
            6179,
            171,
            13845,
            64,
            10,
            1050,
            3529,
            11,
            65,
            2828,
            116,
            2,
        ],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text="Most redditors know the tiny difference between Frogling",
                token_ids=[2895, 39275, 9314, 216, 5, 5262, 2249, 227, 27449, 1527],
                cumulative_logprob=-55.17701914533973,
                logprobs=[
                    {
                        2895: Logprob(
                            logprob=-6.909554481506348, rank=143, decoded_token="Most"
                        ),
                        100: Logprob(
                            logprob=-2.197445869445801, rank=1, decoded_token="I"
                        ),
                        133: Logprob(
                            logprob=-3.4867753982543945, rank=2, decoded_token="The"
                        ),
                    },
                    {
                        39275: Logprob(
                            logprob=-7.630484580993652, rank=148, decoded_token=" redd"
                        ),
                        9: Logprob(
                            logprob=-1.8084166049957275, rank=1, decoded_token=" of"
                        ),
                        82: Logprob(
                            logprob=-2.3389289379119873, rank=2, decoded_token=" people"
                        ),
                    },
                    {
                        9314: Logprob(
                            logprob=-0.039746206253767014, rank=1, decoded_token="itors"
                        ),
                        7852: Logprob(
                            logprob=-4.065564155578613, rank=2, decoded_token="itor"
                        ),
                    },
                    {
                        216: Logprob(
                            logprob=-4.415658473968506, rank=15, decoded_token=" know"
                        ),
                        32: Logprob(
                            logprob=-1.5063375234603882, rank=1, decoded_token=" are"
                        ),
                        218: Logprob(
                            logprob=-2.7589268684387207, rank=2, decoded_token=" don"
                        ),
                    },
                    {
                        5: Logprob(
                            logprob=-2.7328412532806396, rank=3, decoded_token=" the"
                        ),
                        14: Logprob(
                            logprob=-1.2675859928131104, rank=1, decoded_token=" that"
                        ),
                        42: Logprob(
                            logprob=-2.295158624649048, rank=2, decoded_token=" this"
                        ),
                    },
                    {
                        5262: Logprob(
                            logprob=-9.554351806640625, rank=1370, decoded_token=" tiny"
                        ),
                        1948: Logprob(
                            logprob=-1.7232582569122314, rank=1, decoded_token=" answer"
                        ),
                        2249: Logprob(
                            logprob=-3.347280740737915,
                            rank=2,
                            decoded_token=" difference",
                        ),
                    },
                    {
                        2249: Logprob(
                            logprob=-4.9500274658203125,
                            rank=8,
                            decoded_token=" difference",
                        ),
                        1280: Logprob(
                            logprob=-3.1549720764160156, rank=1, decoded_token=" amount"
                        ),
                        410: Logprob(
                            logprob=-3.626887798309326, rank=2, decoded_token=" little"
                        ),
                    },
                    {
                        227: Logprob(
                            logprob=-0.08497463166713715,
                            rank=1,
                            decoded_token=" between",
                        ),
                        11: Logprob(
                            logprob=-3.210397958755493, rank=2, decoded_token=" in"
                        ),
                    },
                    {
                        27449: Logprob(
                            logprob=-12.07158374786377,
                            rank=11151,
                            decoded_token=" Frog",
                        ),
                        10: Logprob(
                            logprob=-1.4436050653457642, rank=1, decoded_token=" a"
                        ),
                        5: Logprob(
                            logprob=-2.731874942779541, rank=2, decoded_token=" the"
                        ),
                    },
                    {
                        1527: Logprob(
                            logprob=-6.787796497344971, rank=69, decoded_token="ling"
                        ),
                        8: Logprob(
                            logprob=-1.6513729095458984, rank=1, decoded_token=" and"
                        ),
                        29: Logprob(
                            logprob=-1.7453670501708984, rank=2, decoded_token="s"
                        ),
                    },
                ],
                finish_reason="length",
                stop_reason=None,
            )
        ],
        finished=True,
        metrics=RequestMetrics(
            arrival_time=1719562089.4807153,
            last_token_time=1719562089.83397,
            first_scheduled_time=1719562089.4812138,
            first_token_time=1719562089.5375597,
            time_in_queue=0.0004985332489013672,
            finished_time=1719566665.8268993,
        ),
        lora_request=None,
    ),
]

opt_chat_cmpl_chunks_with_logit_bias = [
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="You are a friendly chatbot who always responds in the style of a pirate</s>How many helicopters can a human eat in one sitting?</s>",
        prompt_token_ids=[
            2,
            1185,
            32,
            10,
            5192,
            7359,
            12749,
            54,
            460,
            17904,
            11,
            5,
            2496,
            9,
            10,
            34687,
            2,
            6179,
            171,
            13845,
            64,
            10,
            1050,
            3529,
            11,
            65,
            2828,
            116,
            2,
        ],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text=" Frog",
                token_ids=[27449],
                cumulative_logprob=0.0,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719660998.9662814,
            last_token_time=1719660999.0457416,
            first_scheduled_time=1719660998.9670365,
            first_token_time=1719660999.0454147,
            time_in_queue=0.0007550716400146484,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="You are a friendly chatbot who always responds in the style of a pirate</s>How many helicopters can a human eat in one sitting?</s>",
        prompt_token_ids=[
            2,
            1185,
            32,
            10,
            5192,
            7359,
            12749,
            54,
            460,
            17904,
            11,
            5,
            2496,
            9,
            10,
            34687,
            2,
            6179,
            171,
            13845,
            64,
            10,
            1050,
            3529,
            11,
            65,
            2828,
            116,
            2,
        ],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text=" Frog Frog",
                token_ids=[27449, 27449],
                cumulative_logprob=0.0,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719660998.9662814,
            last_token_time=1719660999.0881808,
            first_scheduled_time=1719660998.9670365,
            first_token_time=1719660999.0454147,
            time_in_queue=0.0007550716400146484,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="You are a friendly chatbot who always responds in the style of a pirate</s>How many helicopters can a human eat in one sitting?</s>",
        prompt_token_ids=[
            2,
            1185,
            32,
            10,
            5192,
            7359,
            12749,
            54,
            460,
            17904,
            11,
            5,
            2496,
            9,
            10,
            34687,
            2,
            6179,
            171,
            13845,
            64,
            10,
            1050,
            3529,
            11,
            65,
            2828,
            116,
            2,
        ],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text=" Frog Frog Frog",
                token_ids=[27449, 27449, 27449],
                cumulative_logprob=0.0,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719660998.9662814,
            last_token_time=1719660999.1315076,
            first_scheduled_time=1719660998.9670365,
            first_token_time=1719660999.0454147,
            time_in_queue=0.0007550716400146484,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="You are a friendly chatbot who always responds in the style of a pirate</s>How many helicopters can a human eat in one sitting?</s>",
        prompt_token_ids=[
            2,
            1185,
            32,
            10,
            5192,
            7359,
            12749,
            54,
            460,
            17904,
            11,
            5,
            2496,
            9,
            10,
            34687,
            2,
            6179,
            171,
            13845,
            64,
            10,
            1050,
            3529,
            11,
            65,
            2828,
            116,
            2,
        ],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text=" Frog Frog Frog Frog",
                token_ids=[27449, 27449, 27449, 27449],
                cumulative_logprob=0.0,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719660998.9662814,
            last_token_time=1719660999.171964,
            first_scheduled_time=1719660998.9670365,
            first_token_time=1719660999.0454147,
            time_in_queue=0.0007550716400146484,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="You are a friendly chatbot who always responds in the style of a pirate</s>How many helicopters can a human eat in one sitting?</s>",
        prompt_token_ids=[
            2,
            1185,
            32,
            10,
            5192,
            7359,
            12749,
            54,
            460,
            17904,
            11,
            5,
            2496,
            9,
            10,
            34687,
            2,
            6179,
            171,
            13845,
            64,
            10,
            1050,
            3529,
            11,
            65,
            2828,
            116,
            2,
        ],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text=" Frog Frog Frog Frog Frog",
                token_ids=[27449, 27449, 27449, 27449, 27449],
                cumulative_logprob=0.0,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719660998.9662814,
            last_token_time=1719660999.2129953,
            first_scheduled_time=1719660998.9670365,
            first_token_time=1719660999.0454147,
            time_in_queue=0.0007550716400146484,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="You are a friendly chatbot who always responds in the style of a pirate</s>How many helicopters can a human eat in one sitting?</s>",
        prompt_token_ids=[
            2,
            1185,
            32,
            10,
            5192,
            7359,
            12749,
            54,
            460,
            17904,
            11,
            5,
            2496,
            9,
            10,
            34687,
            2,
            6179,
            171,
            13845,
            64,
            10,
            1050,
            3529,
            11,
            65,
            2828,
            116,
            2,
        ],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text=" Frog Frog Frog Frog Frog Frog",
                token_ids=[27449, 27449, 27449, 27449, 27449, 27449],
                cumulative_logprob=0.0,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719660998.9662814,
            last_token_time=1719660999.2545228,
            first_scheduled_time=1719660998.9670365,
            first_token_time=1719660999.0454147,
            time_in_queue=0.0007550716400146484,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="You are a friendly chatbot who always responds in the style of a pirate</s>How many helicopters can a human eat in one sitting?</s>",
        prompt_token_ids=[
            2,
            1185,
            32,
            10,
            5192,
            7359,
            12749,
            54,
            460,
            17904,
            11,
            5,
            2496,
            9,
            10,
            34687,
            2,
            6179,
            171,
            13845,
            64,
            10,
            1050,
            3529,
            11,
            65,
            2828,
            116,
            2,
        ],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text=" Frog Frog Frog Frog Frog Frog Frog",
                token_ids=[27449, 27449, 27449, 27449, 27449, 27449, 27449],
                cumulative_logprob=0.0,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719660998.9662814,
            last_token_time=1719660999.2973335,
            first_scheduled_time=1719660998.9670365,
            first_token_time=1719660999.0454147,
            time_in_queue=0.0007550716400146484,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="You are a friendly chatbot who always responds in the style of a pirate</s>How many helicopters can a human eat in one sitting?</s>",
        prompt_token_ids=[
            2,
            1185,
            32,
            10,
            5192,
            7359,
            12749,
            54,
            460,
            17904,
            11,
            5,
            2496,
            9,
            10,
            34687,
            2,
            6179,
            171,
            13845,
            64,
            10,
            1050,
            3529,
            11,
            65,
            2828,
            116,
            2,
        ],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text=" Frog Frog Frog Frog Frog Frog Frog Frog",
                token_ids=[27449, 27449, 27449, 27449, 27449, 27449, 27449, 27449],
                cumulative_logprob=0.0,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719660998.9662814,
            last_token_time=1719660999.340904,
            first_scheduled_time=1719660998.9670365,
            first_token_time=1719660999.0454147,
            time_in_queue=0.0007550716400146484,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="You are a friendly chatbot who always responds in the style of a pirate</s>How many helicopters can a human eat in one sitting?</s>",
        prompt_token_ids=[
            2,
            1185,
            32,
            10,
            5192,
            7359,
            12749,
            54,
            460,
            17904,
            11,
            5,
            2496,
            9,
            10,
            34687,
            2,
            6179,
            171,
            13845,
            64,
            10,
            1050,
            3529,
            11,
            65,
            2828,
            116,
            2,
        ],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text=" Frog Frog Frog Frog Frog Frog Frog Frog Frog",
                token_ids=[
                    27449,
                    27449,
                    27449,
                    27449,
                    27449,
                    27449,
                    27449,
                    27449,
                    27449,
                ],
                cumulative_logprob=0.0,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719660998.9662814,
            last_token_time=1719660999.3833568,
            first_scheduled_time=1719660998.9670365,
            first_token_time=1719660999.0454147,
            time_in_queue=0.0007550716400146484,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="You are a friendly chatbot who always responds in the style of a pirate</s>How many helicopters can a human eat in one sitting?</s>",
        prompt_token_ids=[
            2,
            1185,
            32,
            10,
            5192,
            7359,
            12749,
            54,
            460,
            17904,
            11,
            5,
            2496,
            9,
            10,
            34687,
            2,
            6179,
            171,
            13845,
            64,
            10,
            1050,
            3529,
            11,
            65,
            2828,
            116,
            2,
        ],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text=" Frog Frog Frog Frog Frog Frog Frog Frog Frog Frog",
                token_ids=[
                    27449,
                    27449,
                    27449,
                    27449,
                    27449,
                    27449,
                    27449,
                    27449,
                    27449,
                    27449,
                ],
                cumulative_logprob=0.0,
                logprobs=None,
                finish_reason="length",
                stop_reason=None,
            )
        ],
        finished=True,
        metrics=RequestMetrics(
            arrival_time=1719660998.9662814,
            last_token_time=1719660999.4252603,
            first_scheduled_time=1719660998.9670365,
            first_token_time=1719660999.0454147,
            time_in_queue=0.0007550716400146484,
            finished_time=1719660999.4252543,
        ),
        lora_request=None,
    ),
]

# Completion mocks

opt_cmpl_chunks = [
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text="-",
                token_ids=[12],
                cumulative_logprob=-5.968788146972656,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719571494.726604,
            last_token_time=1719571494.7727408,
            first_scheduled_time=1719571494.7271025,
            first_token_time=1719571494.7725506,
            time_in_queue=0.0004985332489013672,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text="- Labrador",
                token_ids=[12, 26882],
                cumulative_logprob=-16.97801971435547,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719571494.726604,
            last_token_time=1719571494.80594,
            first_scheduled_time=1719571494.7271025,
            first_token_time=1719571494.7725506,
            time_in_queue=0.0004985332489013672,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text="- Labrador!",
                token_ids=[12, 26882, 328],
                cumulative_logprob=-20.131213903427124,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719571494.726604,
            last_token_time=1719571494.8386245,
            first_scheduled_time=1719571494.7271025,
            first_token_time=1719571494.7725506,
            time_in_queue=0.0004985332489013672,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text="- Labrador! He",
                token_ids=[12, 26882, 328, 91],
                cumulative_logprob=-21.5479416847229,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719571494.726604,
            last_token_time=1719571494.8719313,
            first_scheduled_time=1719571494.7271025,
            first_token_time=1719571494.7725506,
            time_in_queue=0.0004985332489013672,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text="- Labrador! He has",
                token_ids=[12, 26882, 328, 91, 34],
                cumulative_logprob=-24.31446599960327,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719571494.726604,
            last_token_time=1719571494.905585,
            first_scheduled_time=1719571494.7271025,
            first_token_time=1719571494.7725506,
            time_in_queue=0.0004985332489013672,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text="- Labrador! He has tiny",
                token_ids=[12, 26882, 328, 91, 34, 5262],
                cumulative_logprob=-31.254112720489502,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719571494.726604,
            last_token_time=1719571494.9397604,
            first_scheduled_time=1719571494.7271025,
            first_token_time=1719571494.7725506,
            time_in_queue=0.0004985332489013672,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text="- Labrador! He has tiny ears",
                token_ids=[12, 26882, 328, 91, 34, 5262, 12137],
                cumulative_logprob=-32.616105914115906,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719571494.726604,
            last_token_time=1719571494.9736176,
            first_scheduled_time=1719571494.7271025,
            first_token_time=1719571494.7725506,
            time_in_queue=0.0004985332489013672,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text="- Labrador! He has tiny ears with",
                token_ids=[12, 26882, 328, 91, 34, 5262, 12137, 19],
                cumulative_logprob=-37.23606622219086,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719571494.726604,
            last_token_time=1719571495.0074778,
            first_scheduled_time=1719571494.7271025,
            first_token_time=1719571494.7725506,
            time_in_queue=0.0004985332489013672,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text="- Labrador! He has tiny ears with fluffy",
                token_ids=[12, 26882, 328, 91, 34, 5262, 12137, 19, 33564],
                cumulative_logprob=-42.86094415187836,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719571494.726604,
            last_token_time=1719571495.0404558,
            first_scheduled_time=1719571494.7271025,
            first_token_time=1719571494.7725506,
            time_in_queue=0.0004985332489013672,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text="- Labrador! He has tiny ears with fluffy white",
                token_ids=[12, 26882, 328, 91, 34, 5262, 12137, 19, 33564, 1104],
                cumulative_logprob=-45.076220870018005,
                logprobs=None,
                finish_reason="length",
                stop_reason=None,
            )
        ],
        finished=True,
        metrics=RequestMetrics(
            arrival_time=1719571494.726604,
            last_token_time=1719571495.0741613,
            first_scheduled_time=1719571494.7271025,
            first_token_time=1719571494.7725506,
            time_in_queue=0.0004985332489013672,
            finished_time=1719571495.0741584,
        ),
        lora_request=None,
    ),
]

opt_cmpl_chunks_with_logprobs = [
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text="-",
                token_ids=[12],
                cumulative_logprob=-5.968788146972656,
                logprobs=[
                    {
                        12: Logprob(
                            logprob=-5.968788146972656, rank=22, decoded_token="-"
                        ),
                        4: Logprob(
                            logprob=-1.4537553787231445, rank=1, decoded_token="."
                        ),
                        6: Logprob(
                            logprob=-1.8416948318481445, rank=2, decoded_token=","
                        ),
                    }
                ],
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719573407.7665198,
            last_token_time=1719573407.8125467,
            first_scheduled_time=1719573407.7670705,
            first_token_time=1719573407.8123348,
            time_in_queue=0.0005507469177246094,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text="- Labrador",
                token_ids=[12, 26882],
                cumulative_logprob=-16.97801971435547,
                logprobs=[
                    {
                        12: Logprob(
                            logprob=-5.968788146972656, rank=22, decoded_token="-"
                        ),
                        4: Logprob(
                            logprob=-1.4537553787231445, rank=1, decoded_token="."
                        ),
                        6: Logprob(
                            logprob=-1.8416948318481445, rank=2, decoded_token=","
                        ),
                    },
                    {
                        26882: Logprob(
                            logprob=-11.009231567382812,
                            rank=3032,
                            decoded_token=" Labrador",
                        ),
                        38: Logprob(
                            logprob=-1.754422903060913, rank=1, decoded_token=" I"
                        ),
                        79: Logprob(
                            logprob=-3.075488328933716, rank=2, decoded_token=" she"
                        ),
                    },
                ],
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719573407.7665198,
            last_token_time=1719573407.8460371,
            first_scheduled_time=1719573407.7670705,
            first_token_time=1719573407.8123348,
            time_in_queue=0.0005507469177246094,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text="- Labrador!",
                token_ids=[12, 26882, 328],
                cumulative_logprob=-20.131213903427124,
                logprobs=[
                    {
                        12: Logprob(
                            logprob=-5.968788146972656, rank=22, decoded_token="-"
                        ),
                        4: Logprob(
                            logprob=-1.4537553787231445, rank=1, decoded_token="."
                        ),
                        6: Logprob(
                            logprob=-1.8416948318481445, rank=2, decoded_token=","
                        ),
                    },
                    {
                        26882: Logprob(
                            logprob=-11.009231567382812,
                            rank=3032,
                            decoded_token=" Labrador",
                        ),
                        38: Logprob(
                            logprob=-1.754422903060913, rank=1, decoded_token=" I"
                        ),
                        79: Logprob(
                            logprob=-3.075488328933716, rank=2, decoded_token=" she"
                        ),
                    },
                    {
                        328: Logprob(
                            logprob=-3.1531941890716553, rank=5, decoded_token="!"
                        ),
                        3344: Logprob(
                            logprob=-1.0394361019134521, rank=1, decoded_token=" mix"
                        ),
                        4: Logprob(
                            logprob=-2.1872146129608154, rank=2, decoded_token="."
                        ),
                    },
                ],
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719573407.7665198,
            last_token_time=1719573407.8782697,
            first_scheduled_time=1719573407.7670705,
            first_token_time=1719573407.8123348,
            time_in_queue=0.0005507469177246094,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text="- Labrador! He",
                token_ids=[12, 26882, 328, 91],
                cumulative_logprob=-21.5479416847229,
                logprobs=[
                    {
                        12: Logprob(
                            logprob=-5.968788146972656, rank=22, decoded_token="-"
                        ),
                        4: Logprob(
                            logprob=-1.4537553787231445, rank=1, decoded_token="."
                        ),
                        6: Logprob(
                            logprob=-1.8416948318481445, rank=2, decoded_token=","
                        ),
                    },
                    {
                        26882: Logprob(
                            logprob=-11.009231567382812,
                            rank=3032,
                            decoded_token=" Labrador",
                        ),
                        38: Logprob(
                            logprob=-1.754422903060913, rank=1, decoded_token=" I"
                        ),
                        79: Logprob(
                            logprob=-3.075488328933716, rank=2, decoded_token=" she"
                        ),
                    },
                    {
                        328: Logprob(
                            logprob=-3.1531941890716553, rank=5, decoded_token="!"
                        ),
                        3344: Logprob(
                            logprob=-1.0394361019134521, rank=1, decoded_token=" mix"
                        ),
                        4: Logprob(
                            logprob=-2.1872146129608154, rank=2, decoded_token="."
                        ),
                    },
                    {
                        91: Logprob(
                            logprob=-1.4167277812957764, rank=1, decoded_token=" He"
                        ),
                        50118: Logprob(
                            logprob=-2.0672662258148193, rank=2, decoded_token="\n"
                        ),
                    },
                ],
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719573407.7665198,
            last_token_time=1719573407.9108856,
            first_scheduled_time=1719573407.7670705,
            first_token_time=1719573407.8123348,
            time_in_queue=0.0005507469177246094,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text="- Labrador! He has",
                token_ids=[12, 26882, 328, 91, 34],
                cumulative_logprob=-24.31446599960327,
                logprobs=[
                    {
                        12: Logprob(
                            logprob=-5.968788146972656, rank=22, decoded_token="-"
                        ),
                        4: Logprob(
                            logprob=-1.4537553787231445, rank=1, decoded_token="."
                        ),
                        6: Logprob(
                            logprob=-1.8416948318481445, rank=2, decoded_token=","
                        ),
                    },
                    {
                        26882: Logprob(
                            logprob=-11.009231567382812,
                            rank=3032,
                            decoded_token=" Labrador",
                        ),
                        38: Logprob(
                            logprob=-1.754422903060913, rank=1, decoded_token=" I"
                        ),
                        79: Logprob(
                            logprob=-3.075488328933716, rank=2, decoded_token=" she"
                        ),
                    },
                    {
                        328: Logprob(
                            logprob=-3.1531941890716553, rank=5, decoded_token="!"
                        ),
                        3344: Logprob(
                            logprob=-1.0394361019134521, rank=1, decoded_token=" mix"
                        ),
                        4: Logprob(
                            logprob=-2.1872146129608154, rank=2, decoded_token="."
                        ),
                    },
                    {
                        91: Logprob(
                            logprob=-1.4167277812957764, rank=1, decoded_token=" He"
                        ),
                        50118: Logprob(
                            logprob=-2.0672662258148193, rank=2, decoded_token="\n"
                        ),
                    },
                    {
                        34: Logprob(
                            logprob=-2.766524314880371, rank=3, decoded_token=" has"
                        ),
                        18: Logprob(
                            logprob=-1.0847474336624146, rank=1, decoded_token="'s"
                        ),
                        16: Logprob(
                            logprob=-1.547521710395813, rank=2, decoded_token=" is"
                        ),
                    },
                ],
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719573407.7665198,
            last_token_time=1719573407.9432268,
            first_scheduled_time=1719573407.7670705,
            first_token_time=1719573407.8123348,
            time_in_queue=0.0005507469177246094,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text="- Labrador! He has tiny",
                token_ids=[12, 26882, 328, 91, 34, 5262],
                cumulative_logprob=-31.254112720489502,
                logprobs=[
                    {
                        12: Logprob(
                            logprob=-5.968788146972656, rank=22, decoded_token="-"
                        ),
                        4: Logprob(
                            logprob=-1.4537553787231445, rank=1, decoded_token="."
                        ),
                        6: Logprob(
                            logprob=-1.8416948318481445, rank=2, decoded_token=","
                        ),
                    },
                    {
                        26882: Logprob(
                            logprob=-11.009231567382812,
                            rank=3032,
                            decoded_token=" Labrador",
                        ),
                        38: Logprob(
                            logprob=-1.754422903060913, rank=1, decoded_token=" I"
                        ),
                        79: Logprob(
                            logprob=-3.075488328933716, rank=2, decoded_token=" she"
                        ),
                    },
                    {
                        328: Logprob(
                            logprob=-3.1531941890716553, rank=5, decoded_token="!"
                        ),
                        3344: Logprob(
                            logprob=-1.0394361019134521, rank=1, decoded_token=" mix"
                        ),
                        4: Logprob(
                            logprob=-2.1872146129608154, rank=2, decoded_token="."
                        ),
                    },
                    {
                        91: Logprob(
                            logprob=-1.4167277812957764, rank=1, decoded_token=" He"
                        ),
                        50118: Logprob(
                            logprob=-2.0672662258148193, rank=2, decoded_token="\n"
                        ),
                    },
                    {
                        34: Logprob(
                            logprob=-2.766524314880371, rank=3, decoded_token=" has"
                        ),
                        18: Logprob(
                            logprob=-1.0847474336624146, rank=1, decoded_token="'s"
                        ),
                        16: Logprob(
                            logprob=-1.547521710395813, rank=2, decoded_token=" is"
                        ),
                    },
                    {
                        5262: Logprob(
                            logprob=-6.9396467208862305, rank=90, decoded_token=" tiny"
                        ),
                        10: Logprob(
                            logprob=-1.3877270221710205, rank=1, decoded_token=" a"
                        ),
                        57: Logprob(
                            logprob=-2.3109371662139893, rank=2, decoded_token=" been"
                        ),
                    },
                ],
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719573407.7665198,
            last_token_time=1719573407.9753387,
            first_scheduled_time=1719573407.7670705,
            first_token_time=1719573407.8123348,
            time_in_queue=0.0005507469177246094,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text="- Labrador! He has tiny ears",
                token_ids=[12, 26882, 328, 91, 34, 5262, 12137],
                cumulative_logprob=-32.616105914115906,
                logprobs=[
                    {
                        12: Logprob(
                            logprob=-5.968788146972656, rank=22, decoded_token="-"
                        ),
                        4: Logprob(
                            logprob=-1.4537553787231445, rank=1, decoded_token="."
                        ),
                        6: Logprob(
                            logprob=-1.8416948318481445, rank=2, decoded_token=","
                        ),
                    },
                    {
                        26882: Logprob(
                            logprob=-11.009231567382812,
                            rank=3032,
                            decoded_token=" Labrador",
                        ),
                        38: Logprob(
                            logprob=-1.754422903060913, rank=1, decoded_token=" I"
                        ),
                        79: Logprob(
                            logprob=-3.075488328933716, rank=2, decoded_token=" she"
                        ),
                    },
                    {
                        328: Logprob(
                            logprob=-3.1531941890716553, rank=5, decoded_token="!"
                        ),
                        3344: Logprob(
                            logprob=-1.0394361019134521, rank=1, decoded_token=" mix"
                        ),
                        4: Logprob(
                            logprob=-2.1872146129608154, rank=2, decoded_token="."
                        ),
                    },
                    {
                        91: Logprob(
                            logprob=-1.4167277812957764, rank=1, decoded_token=" He"
                        ),
                        50118: Logprob(
                            logprob=-2.0672662258148193, rank=2, decoded_token="\n"
                        ),
                    },
                    {
                        34: Logprob(
                            logprob=-2.766524314880371, rank=3, decoded_token=" has"
                        ),
                        18: Logprob(
                            logprob=-1.0847474336624146, rank=1, decoded_token="'s"
                        ),
                        16: Logprob(
                            logprob=-1.547521710395813, rank=2, decoded_token=" is"
                        ),
                    },
                    {
                        5262: Logprob(
                            logprob=-6.9396467208862305, rank=90, decoded_token=" tiny"
                        ),
                        10: Logprob(
                            logprob=-1.3877270221710205, rank=1, decoded_token=" a"
                        ),
                        57: Logprob(
                            logprob=-2.3109371662139893, rank=2, decoded_token=" been"
                        ),
                    },
                    {
                        12137: Logprob(
                            logprob=-1.3619931936264038, rank=1, decoded_token=" ears"
                        ),
                        40844: Logprob(
                            logprob=-2.2743258476257324, rank=2, decoded_token=" paws"
                        ),
                    },
                ],
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719573407.7665198,
            last_token_time=1719573408.0083814,
            first_scheduled_time=1719573407.7670705,
            first_token_time=1719573407.8123348,
            time_in_queue=0.0005507469177246094,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text="- Labrador! He has tiny ears with",
                token_ids=[12, 26882, 328, 91, 34, 5262, 12137, 19],
                cumulative_logprob=-37.23606622219086,
                logprobs=[
                    {
                        12: Logprob(
                            logprob=-5.968788146972656, rank=22, decoded_token="-"
                        ),
                        4: Logprob(
                            logprob=-1.4537553787231445, rank=1, decoded_token="."
                        ),
                        6: Logprob(
                            logprob=-1.8416948318481445, rank=2, decoded_token=","
                        ),
                    },
                    {
                        26882: Logprob(
                            logprob=-11.009231567382812,
                            rank=3032,
                            decoded_token=" Labrador",
                        ),
                        38: Logprob(
                            logprob=-1.754422903060913, rank=1, decoded_token=" I"
                        ),
                        79: Logprob(
                            logprob=-3.075488328933716, rank=2, decoded_token=" she"
                        ),
                    },
                    {
                        328: Logprob(
                            logprob=-3.1531941890716553, rank=5, decoded_token="!"
                        ),
                        3344: Logprob(
                            logprob=-1.0394361019134521, rank=1, decoded_token=" mix"
                        ),
                        4: Logprob(
                            logprob=-2.1872146129608154, rank=2, decoded_token="."
                        ),
                    },
                    {
                        91: Logprob(
                            logprob=-1.4167277812957764, rank=1, decoded_token=" He"
                        ),
                        50118: Logprob(
                            logprob=-2.0672662258148193, rank=2, decoded_token="\n"
                        ),
                    },
                    {
                        34: Logprob(
                            logprob=-2.766524314880371, rank=3, decoded_token=" has"
                        ),
                        18: Logprob(
                            logprob=-1.0847474336624146, rank=1, decoded_token="'s"
                        ),
                        16: Logprob(
                            logprob=-1.547521710395813, rank=2, decoded_token=" is"
                        ),
                    },
                    {
                        5262: Logprob(
                            logprob=-6.9396467208862305, rank=90, decoded_token=" tiny"
                        ),
                        10: Logprob(
                            logprob=-1.3877270221710205, rank=1, decoded_token=" a"
                        ),
                        57: Logprob(
                            logprob=-2.3109371662139893, rank=2, decoded_token=" been"
                        ),
                    },
                    {
                        12137: Logprob(
                            logprob=-1.3619931936264038, rank=1, decoded_token=" ears"
                        ),
                        40844: Logprob(
                            logprob=-2.2743258476257324, rank=2, decoded_token=" paws"
                        ),
                    },
                    {
                        19: Logprob(
                            logprob=-4.619960308074951, rank=10, decoded_token=" with"
                        ),
                        8: Logprob(
                            logprob=-0.805719792842865, rank=1, decoded_token=" and"
                        ),
                        6: Logprob(
                            logprob=-1.6155686378479004, rank=2, decoded_token=","
                        ),
                    },
                ],
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719573407.7665198,
            last_token_time=1719573408.0417943,
            first_scheduled_time=1719573407.7670705,
            first_token_time=1719573407.8123348,
            time_in_queue=0.0005507469177246094,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text="- Labrador! He has tiny ears with fluffy",
                token_ids=[12, 26882, 328, 91, 34, 5262, 12137, 19, 33564],
                cumulative_logprob=-42.86094415187836,
                logprobs=[
                    {
                        12: Logprob(
                            logprob=-5.968788146972656, rank=22, decoded_token="-"
                        ),
                        4: Logprob(
                            logprob=-1.4537553787231445, rank=1, decoded_token="."
                        ),
                        6: Logprob(
                            logprob=-1.8416948318481445, rank=2, decoded_token=","
                        ),
                    },
                    {
                        26882: Logprob(
                            logprob=-11.009231567382812,
                            rank=3032,
                            decoded_token=" Labrador",
                        ),
                        38: Logprob(
                            logprob=-1.754422903060913, rank=1, decoded_token=" I"
                        ),
                        79: Logprob(
                            logprob=-3.075488328933716, rank=2, decoded_token=" she"
                        ),
                    },
                    {
                        328: Logprob(
                            logprob=-3.1531941890716553, rank=5, decoded_token="!"
                        ),
                        3344: Logprob(
                            logprob=-1.0394361019134521, rank=1, decoded_token=" mix"
                        ),
                        4: Logprob(
                            logprob=-2.1872146129608154, rank=2, decoded_token="."
                        ),
                    },
                    {
                        91: Logprob(
                            logprob=-1.4167277812957764, rank=1, decoded_token=" He"
                        ),
                        50118: Logprob(
                            logprob=-2.0672662258148193, rank=2, decoded_token="\n"
                        ),
                    },
                    {
                        34: Logprob(
                            logprob=-2.766524314880371, rank=3, decoded_token=" has"
                        ),
                        18: Logprob(
                            logprob=-1.0847474336624146, rank=1, decoded_token="'s"
                        ),
                        16: Logprob(
                            logprob=-1.547521710395813, rank=2, decoded_token=" is"
                        ),
                    },
                    {
                        5262: Logprob(
                            logprob=-6.9396467208862305, rank=90, decoded_token=" tiny"
                        ),
                        10: Logprob(
                            logprob=-1.3877270221710205, rank=1, decoded_token=" a"
                        ),
                        57: Logprob(
                            logprob=-2.3109371662139893, rank=2, decoded_token=" been"
                        ),
                    },
                    {
                        12137: Logprob(
                            logprob=-1.3619931936264038, rank=1, decoded_token=" ears"
                        ),
                        40844: Logprob(
                            logprob=-2.2743258476257324, rank=2, decoded_token=" paws"
                        ),
                    },
                    {
                        19: Logprob(
                            logprob=-4.619960308074951, rank=10, decoded_token=" with"
                        ),
                        8: Logprob(
                            logprob=-0.805719792842865, rank=1, decoded_token=" and"
                        ),
                        6: Logprob(
                            logprob=-1.6155686378479004, rank=2, decoded_token=","
                        ),
                    },
                    {
                        33564: Logprob(
                            logprob=-5.6248779296875, rank=38, decoded_token=" fluffy"
                        ),
                        10: Logprob(
                            logprob=-1.4977400302886963, rank=1, decoded_token=" a"
                        ),
                        5262: Logprob(
                            logprob=-3.006150484085083, rank=2, decoded_token=" tiny"
                        ),
                    },
                ],
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719573407.7665198,
            last_token_time=1719573408.0741389,
            first_scheduled_time=1719573407.7670705,
            first_token_time=1719573407.8123348,
            time_in_queue=0.0005507469177246094,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text="- Labrador! He has tiny ears with fluffy white",
                token_ids=[12, 26882, 328, 91, 34, 5262, 12137, 19, 33564, 1104],
                cumulative_logprob=-45.076220870018005,
                logprobs=[
                    {
                        12: Logprob(
                            logprob=-5.968788146972656, rank=22, decoded_token="-"
                        ),
                        4: Logprob(
                            logprob=-1.4537553787231445, rank=1, decoded_token="."
                        ),
                        6: Logprob(
                            logprob=-1.8416948318481445, rank=2, decoded_token=","
                        ),
                    },
                    {
                        26882: Logprob(
                            logprob=-11.009231567382812,
                            rank=3032,
                            decoded_token=" Labrador",
                        ),
                        38: Logprob(
                            logprob=-1.754422903060913, rank=1, decoded_token=" I"
                        ),
                        79: Logprob(
                            logprob=-3.075488328933716, rank=2, decoded_token=" she"
                        ),
                    },
                    {
                        328: Logprob(
                            logprob=-3.1531941890716553, rank=5, decoded_token="!"
                        ),
                        3344: Logprob(
                            logprob=-1.0394361019134521, rank=1, decoded_token=" mix"
                        ),
                        4: Logprob(
                            logprob=-2.1872146129608154, rank=2, decoded_token="."
                        ),
                    },
                    {
                        91: Logprob(
                            logprob=-1.4167277812957764, rank=1, decoded_token=" He"
                        ),
                        50118: Logprob(
                            logprob=-2.0672662258148193, rank=2, decoded_token="\n"
                        ),
                    },
                    {
                        34: Logprob(
                            logprob=-2.766524314880371, rank=3, decoded_token=" has"
                        ),
                        18: Logprob(
                            logprob=-1.0847474336624146, rank=1, decoded_token="'s"
                        ),
                        16: Logprob(
                            logprob=-1.547521710395813, rank=2, decoded_token=" is"
                        ),
                    },
                    {
                        5262: Logprob(
                            logprob=-6.9396467208862305, rank=90, decoded_token=" tiny"
                        ),
                        10: Logprob(
                            logprob=-1.3877270221710205, rank=1, decoded_token=" a"
                        ),
                        57: Logprob(
                            logprob=-2.3109371662139893, rank=2, decoded_token=" been"
                        ),
                    },
                    {
                        12137: Logprob(
                            logprob=-1.3619931936264038, rank=1, decoded_token=" ears"
                        ),
                        40844: Logprob(
                            logprob=-2.2743258476257324, rank=2, decoded_token=" paws"
                        ),
                    },
                    {
                        19: Logprob(
                            logprob=-4.619960308074951, rank=10, decoded_token=" with"
                        ),
                        8: Logprob(
                            logprob=-0.805719792842865, rank=1, decoded_token=" and"
                        ),
                        6: Logprob(
                            logprob=-1.6155686378479004, rank=2, decoded_token=","
                        ),
                    },
                    {
                        33564: Logprob(
                            logprob=-5.6248779296875, rank=38, decoded_token=" fluffy"
                        ),
                        10: Logprob(
                            logprob=-1.4977400302886963, rank=1, decoded_token=" a"
                        ),
                        5262: Logprob(
                            logprob=-3.006150484085083, rank=2, decoded_token=" tiny"
                        ),
                    },
                    {
                        1104: Logprob(
                            logprob=-2.2152767181396484, rank=2, decoded_token=" white"
                        ),
                        15503: Logprob(
                            logprob=-1.9012728929519653, rank=1, decoded_token=" fur"
                        ),
                    },
                ],
                finish_reason="length",
                stop_reason=None,
            )
        ],
        finished=True,
        metrics=RequestMetrics(
            arrival_time=1719573407.7665198,
            last_token_time=1719573408.1068144,
            first_scheduled_time=1719573407.7670705,
            first_token_time=1719573407.8123348,
            time_in_queue=0.0005507469177246094,
            finished_time=1719573408.1068113,
        ),
        lora_request=None,
    ),
]

opt_cmpl_chunks_with_two_prompts = [
    [
        RequestOutput(
            request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
            prompt="Hi, I love my cat",
            prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
            prompt_logprobs=None,
            outputs=[
                CompletionOutput(
                    index=0,
                    text="-",
                    token_ids=[12],
                    cumulative_logprob=-5.968789577484131,
                    logprobs=None,
                    finish_reason=None,
                    stop_reason=None,
                )
            ],
            finished=False,
            metrics=RequestMetrics(
                arrival_time=1719584168.5240715,
                last_token_time=1719584168.5717702,
                first_scheduled_time=1719584168.5248196,
                first_token_time=1719584168.5715034,
                time_in_queue=0.0007481575012207031,
                finished_time=None,
            ),
            lora_request=None,
        ),
        RequestOutput(
            request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
            prompt="Hi, I love my cat",
            prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
            prompt_logprobs=None,
            outputs=[
                CompletionOutput(
                    index=0,
                    text="- Labrador",
                    token_ids=[12, 26882],
                    cumulative_logprob=-16.978020191192627,
                    logprobs=None,
                    finish_reason=None,
                    stop_reason=None,
                )
            ],
            finished=False,
            metrics=RequestMetrics(
                arrival_time=1719584168.5240715,
                last_token_time=1719584168.6119769,
                first_scheduled_time=1719584168.5248196,
                first_token_time=1719584168.5715034,
                time_in_queue=0.0007481575012207031,
                finished_time=None,
            ),
            lora_request=None,
        ),
        RequestOutput(
            request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
            prompt="Hi, I love my cat",
            prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
            prompt_logprobs=None,
            outputs=[
                CompletionOutput(
                    index=0,
                    text="- Labrador!",
                    token_ids=[12, 26882, 328],
                    cumulative_logprob=-20.13121271133423,
                    logprobs=None,
                    finish_reason=None,
                    stop_reason=None,
                )
            ],
            finished=False,
            metrics=RequestMetrics(
                arrival_time=1719584168.5240715,
                last_token_time=1719584168.6512127,
                first_scheduled_time=1719584168.5248196,
                first_token_time=1719584168.5715034,
                time_in_queue=0.0007481575012207031,
                finished_time=None,
            ),
            lora_request=None,
        ),
        RequestOutput(
            request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
            prompt="Hi, I love my cat",
            prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
            prompt_logprobs=None,
            outputs=[
                CompletionOutput(
                    index=0,
                    text="- Labrador! He",
                    token_ids=[12, 26882, 328, 91],
                    cumulative_logprob=-21.54794156551361,
                    logprobs=None,
                    finish_reason=None,
                    stop_reason=None,
                )
            ],
            finished=False,
            metrics=RequestMetrics(
                arrival_time=1719584168.5240715,
                last_token_time=1719584168.6903815,
                first_scheduled_time=1719584168.5248196,
                first_token_time=1719584168.5715034,
                time_in_queue=0.0007481575012207031,
                finished_time=None,
            ),
            lora_request=None,
        ),
        RequestOutput(
            request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
            prompt="Hi, I love my cat",
            prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
            prompt_logprobs=None,
            outputs=[
                CompletionOutput(
                    index=0,
                    text="- Labrador! He has",
                    token_ids=[12, 26882, 328, 91, 34],
                    cumulative_logprob=-24.31446635723114,
                    logprobs=None,
                    finish_reason=None,
                    stop_reason=None,
                )
            ],
            finished=False,
            metrics=RequestMetrics(
                arrival_time=1719584168.5240715,
                last_token_time=1719584168.729316,
                first_scheduled_time=1719584168.5248196,
                first_token_time=1719584168.5715034,
                time_in_queue=0.0007481575012207031,
                finished_time=None,
            ),
            lora_request=None,
        ),
        RequestOutput(
            request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
            prompt="Hi, I love my cat",
            prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
            prompt_logprobs=None,
            outputs=[
                CompletionOutput(
                    index=0,
                    text="- Labrador! He has tiny",
                    token_ids=[12, 26882, 328, 91, 34, 5262],
                    cumulative_logprob=-31.254114031791687,
                    logprobs=None,
                    finish_reason=None,
                    stop_reason=None,
                )
            ],
            finished=False,
            metrics=RequestMetrics(
                arrival_time=1719584168.5240715,
                last_token_time=1719584168.7692664,
                first_scheduled_time=1719584168.5248196,
                first_token_time=1719584168.5715034,
                time_in_queue=0.0007481575012207031,
                finished_time=None,
            ),
            lora_request=None,
        ),
        RequestOutput(
            request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
            prompt="Hi, I love my cat",
            prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
            prompt_logprobs=None,
            outputs=[
                CompletionOutput(
                    index=0,
                    text="- Labrador! He has tiny ears",
                    token_ids=[12, 26882, 328, 91, 34, 5262, 12137],
                    cumulative_logprob=-32.61610519886017,
                    logprobs=None,
                    finish_reason=None,
                    stop_reason=None,
                )
            ],
            finished=False,
            metrics=RequestMetrics(
                arrival_time=1719584168.5240715,
                last_token_time=1719584168.8083556,
                first_scheduled_time=1719584168.5248196,
                first_token_time=1719584168.5715034,
                time_in_queue=0.0007481575012207031,
                finished_time=None,
            ),
            lora_request=None,
        ),
        RequestOutput(
            request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
            prompt="Hi, I love my cat",
            prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
            prompt_logprobs=None,
            outputs=[
                CompletionOutput(
                    index=0,
                    text="- Labrador! He has tiny ears with",
                    token_ids=[12, 26882, 328, 91, 34, 5262, 12137, 19],
                    cumulative_logprob=-37.23606598377228,
                    logprobs=None,
                    finish_reason=None,
                    stop_reason=None,
                )
            ],
            finished=False,
            metrics=RequestMetrics(
                arrival_time=1719584168.5240715,
                last_token_time=1719584168.8476195,
                first_scheduled_time=1719584168.5248196,
                first_token_time=1719584168.5715034,
                time_in_queue=0.0007481575012207031,
                finished_time=None,
            ),
            lora_request=None,
        ),
        RequestOutput(
            request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
            prompt="Hi, I love my cat",
            prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
            prompt_logprobs=None,
            outputs=[
                CompletionOutput(
                    index=0,
                    text="- Labrador! He has tiny ears with fluffy",
                    token_ids=[12, 26882, 328, 91, 34, 5262, 12137, 19, 33564],
                    cumulative_logprob=-42.860944867134094,
                    logprobs=None,
                    finish_reason=None,
                    stop_reason=None,
                )
            ],
            finished=False,
            metrics=RequestMetrics(
                arrival_time=1719584168.5240715,
                last_token_time=1719584168.8882418,
                first_scheduled_time=1719584168.5248196,
                first_token_time=1719584168.5715034,
                time_in_queue=0.0007481575012207031,
                finished_time=None,
            ),
            lora_request=None,
        ),
        RequestOutput(
            request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
            prompt="Hi, I love my cat",
            prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
            prompt_logprobs=None,
            outputs=[
                CompletionOutput(
                    index=0,
                    text="- Labrador! He has tiny ears with fluffy white",
                    token_ids=[12, 26882, 328, 91, 34, 5262, 12137, 19, 33564, 1104],
                    cumulative_logprob=-45.076220631599426,
                    logprobs=None,
                    finish_reason="length",
                    stop_reason=None,
                )
            ],
            finished=True,
            metrics=RequestMetrics(
                arrival_time=1719584168.5240715,
                last_token_time=1719584168.9272976,
                first_scheduled_time=1719584168.5248196,
                first_token_time=1719584168.5715034,
                time_in_queue=0.0007481575012207031,
                finished_time=1719584168.9272888,
            ),
            lora_request=None,
        ),
    ],
    [
        RequestOutput(
            request_id="cmpl-d771287a234c44498e345f0a429d6691-1",
            prompt="The sky is blue",
            prompt_token_ids=[2, 133, 6360, 16, 2440],
            prompt_logprobs=None,
            outputs=[
                CompletionOutput(
                    index=0,
                    text=" and",
                    token_ids=[8],
                    cumulative_logprob=-2.4368906021118164,
                    logprobs=None,
                    finish_reason=None,
                    stop_reason=None,
                )
            ],
            finished=False,
            metrics=RequestMetrics(
                arrival_time=1719584168.524312,
                last_token_time=1719584168.5717702,
                first_scheduled_time=1719584168.5248196,
                first_token_time=1719584168.5715034,
                time_in_queue=0.0005075931549072266,
                finished_time=None,
            ),
            lora_request=None,
        ),
        RequestOutput(
            request_id="cmpl-d771287a234c44498e345f0a429d6691-1",
            prompt="The sky is blue",
            prompt_token_ids=[2, 133, 6360, 16, 2440],
            prompt_logprobs=None,
            outputs=[
                CompletionOutput(
                    index=0,
                    text=" and no",
                    token_ids=[8, 117],
                    cumulative_logprob=-7.690991401672363,
                    logprobs=None,
                    finish_reason=None,
                    stop_reason=None,
                )
            ],
            finished=False,
            metrics=RequestMetrics(
                arrival_time=1719584168.524312,
                last_token_time=1719584168.6119769,
                first_scheduled_time=1719584168.5248196,
                first_token_time=1719584168.5715034,
                time_in_queue=0.0005075931549072266,
                finished_time=None,
            ),
            lora_request=None,
        ),
        RequestOutput(
            request_id="cmpl-d771287a234c44498e345f0a429d6691-1",
            prompt="The sky is blue",
            prompt_token_ids=[2, 133, 6360, 16, 2440],
            prompt_logprobs=None,
            outputs=[
                CompletionOutput(
                    index=0,
                    text=" and no one",
                    token_ids=[8, 117, 65],
                    cumulative_logprob=-8.11336663365364,
                    logprobs=None,
                    finish_reason=None,
                    stop_reason=None,
                )
            ],
            finished=False,
            metrics=RequestMetrics(
                arrival_time=1719584168.524312,
                last_token_time=1719584168.6512127,
                first_scheduled_time=1719584168.5248196,
                first_token_time=1719584168.5715034,
                time_in_queue=0.0005075931549072266,
                finished_time=None,
            ),
            lora_request=None,
        ),
        RequestOutput(
            request_id="cmpl-d771287a234c44498e345f0a429d6691-1",
            prompt="The sky is blue",
            prompt_token_ids=[2, 133, 6360, 16, 2440],
            prompt_logprobs=None,
            outputs=[
                CompletionOutput(
                    index=0,
                    text=" and no one is",
                    token_ids=[8, 117, 65, 16],
                    cumulative_logprob=-9.278029590845108,
                    logprobs=None,
                    finish_reason=None,
                    stop_reason=None,
                )
            ],
            finished=False,
            metrics=RequestMetrics(
                arrival_time=1719584168.524312,
                last_token_time=1719584168.6903815,
                first_scheduled_time=1719584168.5248196,
                first_token_time=1719584168.5715034,
                time_in_queue=0.0005075931549072266,
                finished_time=None,
            ),
            lora_request=None,
        ),
        RequestOutput(
            request_id="cmpl-d771287a234c44498e345f0a429d6691-1",
            prompt="The sky is blue",
            prompt_token_ids=[2, 133, 6360, 16, 2440],
            prompt_logprobs=None,
            outputs=[
                CompletionOutput(
                    index=0,
                    text=" and no one is going",
                    token_ids=[8, 117, 65, 16, 164],
                    cumulative_logprob=-12.09887209534645,
                    logprobs=None,
                    finish_reason=None,
                    stop_reason=None,
                )
            ],
            finished=False,
            metrics=RequestMetrics(
                arrival_time=1719584168.524312,
                last_token_time=1719584168.729316,
                first_scheduled_time=1719584168.5248196,
                first_token_time=1719584168.5715034,
                time_in_queue=0.0005075931549072266,
                finished_time=None,
            ),
            lora_request=None,
        ),
        RequestOutput(
            request_id="cmpl-d771287a234c44498e345f0a429d6691-1",
            prompt="The sky is blue",
            prompt_token_ids=[2, 133, 6360, 16, 2440],
            prompt_logprobs=None,
            outputs=[
                CompletionOutput(
                    index=0,
                    text=" and no one is going to",
                    token_ids=[8, 117, 65, 16, 164, 7],
                    cumulative_logprob=-12.18747579306364,
                    logprobs=None,
                    finish_reason=None,
                    stop_reason=None,
                )
            ],
            finished=False,
            metrics=RequestMetrics(
                arrival_time=1719584168.524312,
                last_token_time=1719584168.7692664,
                first_scheduled_time=1719584168.5248196,
                first_token_time=1719584168.5715034,
                time_in_queue=0.0005075931549072266,
                finished_time=None,
            ),
            lora_request=None,
        ),
        RequestOutput(
            request_id="cmpl-d771287a234c44498e345f0a429d6691-1",
            prompt="The sky is blue",
            prompt_token_ids=[2, 133, 6360, 16, 2440],
            prompt_logprobs=None,
            outputs=[
                CompletionOutput(
                    index=0,
                    text=" and no one is going to notice",
                    token_ids=[8, 117, 65, 16, 164, 7, 3120],
                    cumulative_logprob=-14.225699536502361,
                    logprobs=None,
                    finish_reason=None,
                    stop_reason=None,
                )
            ],
            finished=False,
            metrics=RequestMetrics(
                arrival_time=1719584168.524312,
                last_token_time=1719584168.8083556,
                first_scheduled_time=1719584168.5248196,
                first_token_time=1719584168.5715034,
                time_in_queue=0.0005075931549072266,
                finished_time=None,
            ),
            lora_request=None,
        ),
        RequestOutput(
            request_id="cmpl-d771287a234c44498e345f0a429d6691-1",
            prompt="The sky is blue",
            prompt_token_ids=[2, 133, 6360, 16, 2440],
            prompt_logprobs=None,
            outputs=[
                CompletionOutput(
                    index=0,
                    text=" and no one is going to notice.",
                    token_ids=[8, 117, 65, 16, 164, 7, 3120, 4],
                    cumulative_logprob=-15.519690982997417,
                    logprobs=None,
                    finish_reason=None,
                    stop_reason=None,
                )
            ],
            finished=False,
            metrics=RequestMetrics(
                arrival_time=1719584168.524312,
                last_token_time=1719584168.8476195,
                first_scheduled_time=1719584168.5248196,
                first_token_time=1719584168.5715034,
                time_in_queue=0.0005075931549072266,
                finished_time=None,
            ),
            lora_request=None,
        ),
        RequestOutput(
            request_id="cmpl-d771287a234c44498e345f0a429d6691-1",
            prompt="The sky is blue",
            prompt_token_ids=[2, 133, 6360, 16, 2440],
            prompt_logprobs=None,
            outputs=[
                CompletionOutput(
                    index=0,
                    text=" and no one is going to notice. You",
                    token_ids=[8, 117, 65, 16, 164, 7, 3120, 4, 370],
                    cumulative_logprob=-20.042794220149517,
                    logprobs=None,
                    finish_reason=None,
                    stop_reason=None,
                )
            ],
            finished=False,
            metrics=RequestMetrics(
                arrival_time=1719584168.524312,
                last_token_time=1719584168.8882418,
                first_scheduled_time=1719584168.5248196,
                first_token_time=1719584168.5715034,
                time_in_queue=0.0005075931549072266,
                finished_time=None,
            ),
            lora_request=None,
        ),
        RequestOutput(
            request_id="cmpl-d771287a234c44498e345f0a429d6691-1",
            prompt="The sky is blue",
            prompt_token_ids=[2, 133, 6360, 16, 2440],
            prompt_logprobs=None,
            outputs=[
                CompletionOutput(
                    index=0,
                    text=" and no one is going to notice. You don",
                    token_ids=[8, 117, 65, 16, 164, 7, 3120, 4, 370, 218],
                    cumulative_logprob=-23.34256123751402,
                    logprobs=None,
                    finish_reason="length",
                    stop_reason=None,
                )
            ],
            finished=True,
            metrics=RequestMetrics(
                arrival_time=1719584168.524312,
                last_token_time=1719584168.9272976,
                first_scheduled_time=1719584168.5248196,
                first_token_time=1719584168.5715034,
                time_in_queue=0.0005075931549072266,
                finished_time=1719584168.9272954,
            ),
            lora_request=None,
        ),
    ],
]

opt_cmpl_chunks_with_two_prompts_log_probs = [
    [
        RequestOutput(
            request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
            prompt="Hi, I love my cat",
            prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
            prompt_logprobs=None,
            outputs=[
                CompletionOutput(
                    index=0,
                    text="-",
                    token_ids=[12],
                    cumulative_logprob=-5.968789577484131,
                    logprobs=[
                        {
                            12: Logprob(
                                logprob=-5.968789577484131, rank=22, decoded_token="-"
                            ),
                            4: Logprob(
                                logprob=-1.4537543058395386, rank=1, decoded_token="."
                            ),
                            6: Logprob(
                                logprob=-1.8416975736618042, rank=2, decoded_token=","
                            ),
                        }
                    ],
                    finish_reason=None,
                    stop_reason=None,
                )
            ],
            finished=False,
            metrics=RequestMetrics(
                arrival_time=1719589967.687916,
                last_token_time=1719589967.7971866,
                first_scheduled_time=1719589967.6917963,
                first_token_time=1719589967.796725,
                time_in_queue=0.0038802623748779297,
                finished_time=None,
            ),
            lora_request=None,
        ),
        RequestOutput(
            request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
            prompt="Hi, I love my cat",
            prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
            prompt_logprobs=None,
            outputs=[
                CompletionOutput(
                    index=0,
                    text="- Labrador",
                    token_ids=[12, 26882],
                    cumulative_logprob=-16.978020191192627,
                    logprobs=[
                        {
                            12: Logprob(
                                logprob=-5.968789577484131, rank=22, decoded_token="-"
                            ),
                            4: Logprob(
                                logprob=-1.4537543058395386, rank=1, decoded_token="."
                            ),
                            6: Logprob(
                                logprob=-1.8416975736618042, rank=2, decoded_token=","
                            ),
                        },
                        {
                            26882: Logprob(
                                logprob=-11.009230613708496,
                                rank=3032,
                                decoded_token=" Labrador",
                            ),
                            38: Logprob(
                                logprob=-1.7544232606887817, rank=1, decoded_token=" I"
                            ),
                            79: Logprob(
                                logprob=-3.0754880905151367,
                                rank=2,
                                decoded_token=" she",
                            ),
                        },
                    ],
                    finish_reason=None,
                    stop_reason=None,
                )
            ],
            finished=False,
            metrics=RequestMetrics(
                arrival_time=1719589967.687916,
                last_token_time=1719589967.8479855,
                first_scheduled_time=1719589967.6917963,
                first_token_time=1719589967.796725,
                time_in_queue=0.0038802623748779297,
                finished_time=None,
            ),
            lora_request=None,
        ),
        RequestOutput(
            request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
            prompt="Hi, I love my cat",
            prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
            prompt_logprobs=None,
            outputs=[
                CompletionOutput(
                    index=0,
                    text="- Labrador!",
                    token_ids=[12, 26882, 328],
                    cumulative_logprob=-20.13121271133423,
                    logprobs=[
                        {
                            12: Logprob(
                                logprob=-5.968789577484131, rank=22, decoded_token="-"
                            ),
                            4: Logprob(
                                logprob=-1.4537543058395386, rank=1, decoded_token="."
                            ),
                            6: Logprob(
                                logprob=-1.8416975736618042, rank=2, decoded_token=","
                            ),
                        },
                        {
                            26882: Logprob(
                                logprob=-11.009230613708496,
                                rank=3032,
                                decoded_token=" Labrador",
                            ),
                            38: Logprob(
                                logprob=-1.7544232606887817, rank=1, decoded_token=" I"
                            ),
                            79: Logprob(
                                logprob=-3.0754880905151367,
                                rank=2,
                                decoded_token=" she",
                            ),
                        },
                        {
                            328: Logprob(
                                logprob=-3.1531925201416016, rank=5, decoded_token="!"
                            ),
                            3344: Logprob(
                                logprob=-1.0394372940063477,
                                rank=1,
                                decoded_token=" mix",
                            ),
                            4: Logprob(
                                logprob=-2.187213897705078, rank=2, decoded_token="."
                            ),
                        },
                    ],
                    finish_reason=None,
                    stop_reason=None,
                )
            ],
            finished=False,
            metrics=RequestMetrics(
                arrival_time=1719589967.687916,
                last_token_time=1719589967.8971055,
                first_scheduled_time=1719589967.6917963,
                first_token_time=1719589967.796725,
                time_in_queue=0.0038802623748779297,
                finished_time=None,
            ),
            lora_request=None,
        ),
        RequestOutput(
            request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
            prompt="Hi, I love my cat",
            prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
            prompt_logprobs=None,
            outputs=[
                CompletionOutput(
                    index=0,
                    text="- Labrador! He",
                    token_ids=[12, 26882, 328, 91],
                    cumulative_logprob=-21.54794156551361,
                    logprobs=[
                        {
                            12: Logprob(
                                logprob=-5.968789577484131, rank=22, decoded_token="-"
                            ),
                            4: Logprob(
                                logprob=-1.4537543058395386, rank=1, decoded_token="."
                            ),
                            6: Logprob(
                                logprob=-1.8416975736618042, rank=2, decoded_token=","
                            ),
                        },
                        {
                            26882: Logprob(
                                logprob=-11.009230613708496,
                                rank=3032,
                                decoded_token=" Labrador",
                            ),
                            38: Logprob(
                                logprob=-1.7544232606887817, rank=1, decoded_token=" I"
                            ),
                            79: Logprob(
                                logprob=-3.0754880905151367,
                                rank=2,
                                decoded_token=" she",
                            ),
                        },
                        {
                            328: Logprob(
                                logprob=-3.1531925201416016, rank=5, decoded_token="!"
                            ),
                            3344: Logprob(
                                logprob=-1.0394372940063477,
                                rank=1,
                                decoded_token=" mix",
                            ),
                            4: Logprob(
                                logprob=-2.187213897705078, rank=2, decoded_token="."
                            ),
                        },
                        {
                            91: Logprob(
                                logprob=-1.4167288541793823, rank=1, decoded_token=" He"
                            ),
                            50118: Logprob(
                                logprob=-2.067265510559082, rank=2, decoded_token="\n"
                            ),
                        },
                    ],
                    finish_reason=None,
                    stop_reason=None,
                )
            ],
            finished=False,
            metrics=RequestMetrics(
                arrival_time=1719589967.687916,
                last_token_time=1719589967.9417195,
                first_scheduled_time=1719589967.6917963,
                first_token_time=1719589967.796725,
                time_in_queue=0.0038802623748779297,
                finished_time=None,
            ),
            lora_request=None,
        ),
        RequestOutput(
            request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
            prompt="Hi, I love my cat",
            prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
            prompt_logprobs=None,
            outputs=[
                CompletionOutput(
                    index=0,
                    text="- Labrador! He has",
                    token_ids=[12, 26882, 328, 91, 34],
                    cumulative_logprob=-24.31446635723114,
                    logprobs=[
                        {
                            12: Logprob(
                                logprob=-5.968789577484131, rank=22, decoded_token="-"
                            ),
                            4: Logprob(
                                logprob=-1.4537543058395386, rank=1, decoded_token="."
                            ),
                            6: Logprob(
                                logprob=-1.8416975736618042, rank=2, decoded_token=","
                            ),
                        },
                        {
                            26882: Logprob(
                                logprob=-11.009230613708496,
                                rank=3032,
                                decoded_token=" Labrador",
                            ),
                            38: Logprob(
                                logprob=-1.7544232606887817, rank=1, decoded_token=" I"
                            ),
                            79: Logprob(
                                logprob=-3.0754880905151367,
                                rank=2,
                                decoded_token=" she",
                            ),
                        },
                        {
                            328: Logprob(
                                logprob=-3.1531925201416016, rank=5, decoded_token="!"
                            ),
                            3344: Logprob(
                                logprob=-1.0394372940063477,
                                rank=1,
                                decoded_token=" mix",
                            ),
                            4: Logprob(
                                logprob=-2.187213897705078, rank=2, decoded_token="."
                            ),
                        },
                        {
                            91: Logprob(
                                logprob=-1.4167288541793823, rank=1, decoded_token=" He"
                            ),
                            50118: Logprob(
                                logprob=-2.067265510559082, rank=2, decoded_token="\n"
                            ),
                        },
                        {
                            34: Logprob(
                                logprob=-2.7665247917175293,
                                rank=3,
                                decoded_token=" has",
                            ),
                            18: Logprob(
                                logprob=-1.0847479104995728, rank=1, decoded_token="'s"
                            ),
                            16: Logprob(
                                logprob=-1.5475212335586548, rank=2, decoded_token=" is"
                            ),
                        },
                    ],
                    finish_reason=None,
                    stop_reason=None,
                )
            ],
            finished=False,
            metrics=RequestMetrics(
                arrival_time=1719589967.687916,
                last_token_time=1719589967.9892497,
                first_scheduled_time=1719589967.6917963,
                first_token_time=1719589967.796725,
                time_in_queue=0.0038802623748779297,
                finished_time=None,
            ),
            lora_request=None,
        ),
        RequestOutput(
            request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
            prompt="Hi, I love my cat",
            prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
            prompt_logprobs=None,
            outputs=[
                CompletionOutput(
                    index=0,
                    text="- Labrador! He has tiny",
                    token_ids=[12, 26882, 328, 91, 34, 5262],
                    cumulative_logprob=-31.254114031791687,
                    logprobs=[
                        {
                            12: Logprob(
                                logprob=-5.968789577484131, rank=22, decoded_token="-"
                            ),
                            4: Logprob(
                                logprob=-1.4537543058395386, rank=1, decoded_token="."
                            ),
                            6: Logprob(
                                logprob=-1.8416975736618042, rank=2, decoded_token=","
                            ),
                        },
                        {
                            26882: Logprob(
                                logprob=-11.009230613708496,
                                rank=3032,
                                decoded_token=" Labrador",
                            ),
                            38: Logprob(
                                logprob=-1.7544232606887817, rank=1, decoded_token=" I"
                            ),
                            79: Logprob(
                                logprob=-3.0754880905151367,
                                rank=2,
                                decoded_token=" she",
                            ),
                        },
                        {
                            328: Logprob(
                                logprob=-3.1531925201416016, rank=5, decoded_token="!"
                            ),
                            3344: Logprob(
                                logprob=-1.0394372940063477,
                                rank=1,
                                decoded_token=" mix",
                            ),
                            4: Logprob(
                                logprob=-2.187213897705078, rank=2, decoded_token="."
                            ),
                        },
                        {
                            91: Logprob(
                                logprob=-1.4167288541793823, rank=1, decoded_token=" He"
                            ),
                            50118: Logprob(
                                logprob=-2.067265510559082, rank=2, decoded_token="\n"
                            ),
                        },
                        {
                            34: Logprob(
                                logprob=-2.7665247917175293,
                                rank=3,
                                decoded_token=" has",
                            ),
                            18: Logprob(
                                logprob=-1.0847479104995728, rank=1, decoded_token="'s"
                            ),
                            16: Logprob(
                                logprob=-1.5475212335586548, rank=2, decoded_token=" is"
                            ),
                        },
                        {
                            5262: Logprob(
                                logprob=-6.939647674560547,
                                rank=90,
                                decoded_token=" tiny",
                            ),
                            10: Logprob(
                                logprob=-1.3877274990081787, rank=1, decoded_token=" a"
                            ),
                            57: Logprob(
                                logprob=-2.3109357357025146,
                                rank=2,
                                decoded_token=" been",
                            ),
                        },
                    ],
                    finish_reason=None,
                    stop_reason=None,
                )
            ],
            finished=False,
            metrics=RequestMetrics(
                arrival_time=1719589967.687916,
                last_token_time=1719589968.0347881,
                first_scheduled_time=1719589967.6917963,
                first_token_time=1719589967.796725,
                time_in_queue=0.0038802623748779297,
                finished_time=None,
            ),
            lora_request=None,
        ),
        RequestOutput(
            request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
            prompt="Hi, I love my cat",
            prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
            prompt_logprobs=None,
            outputs=[
                CompletionOutput(
                    index=0,
                    text="- Labrador! He has tiny ears",
                    token_ids=[12, 26882, 328, 91, 34, 5262, 12137],
                    cumulative_logprob=-32.61610519886017,
                    logprobs=[
                        {
                            12: Logprob(
                                logprob=-5.968789577484131, rank=22, decoded_token="-"
                            ),
                            4: Logprob(
                                logprob=-1.4537543058395386, rank=1, decoded_token="."
                            ),
                            6: Logprob(
                                logprob=-1.8416975736618042, rank=2, decoded_token=","
                            ),
                        },
                        {
                            26882: Logprob(
                                logprob=-11.009230613708496,
                                rank=3032,
                                decoded_token=" Labrador",
                            ),
                            38: Logprob(
                                logprob=-1.7544232606887817, rank=1, decoded_token=" I"
                            ),
                            79: Logprob(
                                logprob=-3.0754880905151367,
                                rank=2,
                                decoded_token=" she",
                            ),
                        },
                        {
                            328: Logprob(
                                logprob=-3.1531925201416016, rank=5, decoded_token="!"
                            ),
                            3344: Logprob(
                                logprob=-1.0394372940063477,
                                rank=1,
                                decoded_token=" mix",
                            ),
                            4: Logprob(
                                logprob=-2.187213897705078, rank=2, decoded_token="."
                            ),
                        },
                        {
                            91: Logprob(
                                logprob=-1.4167288541793823, rank=1, decoded_token=" He"
                            ),
                            50118: Logprob(
                                logprob=-2.067265510559082, rank=2, decoded_token="\n"
                            ),
                        },
                        {
                            34: Logprob(
                                logprob=-2.7665247917175293,
                                rank=3,
                                decoded_token=" has",
                            ),
                            18: Logprob(
                                logprob=-1.0847479104995728, rank=1, decoded_token="'s"
                            ),
                            16: Logprob(
                                logprob=-1.5475212335586548, rank=2, decoded_token=" is"
                            ),
                        },
                        {
                            5262: Logprob(
                                logprob=-6.939647674560547,
                                rank=90,
                                decoded_token=" tiny",
                            ),
                            10: Logprob(
                                logprob=-1.3877274990081787, rank=1, decoded_token=" a"
                            ),
                            57: Logprob(
                                logprob=-2.3109357357025146,
                                rank=2,
                                decoded_token=" been",
                            ),
                        },
                        {
                            12137: Logprob(
                                logprob=-1.3619911670684814,
                                rank=1,
                                decoded_token=" ears",
                            ),
                            40844: Logprob(
                                logprob=-2.2743265628814697,
                                rank=2,
                                decoded_token=" paws",
                            ),
                        },
                    ],
                    finish_reason=None,
                    stop_reason=None,
                )
            ],
            finished=False,
            metrics=RequestMetrics(
                arrival_time=1719589967.687916,
                last_token_time=1719589968.0797527,
                first_scheduled_time=1719589967.6917963,
                first_token_time=1719589967.796725,
                time_in_queue=0.0038802623748779297,
                finished_time=None,
            ),
            lora_request=None,
        ),
        RequestOutput(
            request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
            prompt="Hi, I love my cat",
            prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
            prompt_logprobs=None,
            outputs=[
                CompletionOutput(
                    index=0,
                    text="- Labrador! He has tiny ears with",
                    token_ids=[12, 26882, 328, 91, 34, 5262, 12137, 19],
                    cumulative_logprob=-37.23606598377228,
                    logprobs=[
                        {
                            12: Logprob(
                                logprob=-5.968789577484131, rank=22, decoded_token="-"
                            ),
                            4: Logprob(
                                logprob=-1.4537543058395386, rank=1, decoded_token="."
                            ),
                            6: Logprob(
                                logprob=-1.8416975736618042, rank=2, decoded_token=","
                            ),
                        },
                        {
                            26882: Logprob(
                                logprob=-11.009230613708496,
                                rank=3032,
                                decoded_token=" Labrador",
                            ),
                            38: Logprob(
                                logprob=-1.7544232606887817, rank=1, decoded_token=" I"
                            ),
                            79: Logprob(
                                logprob=-3.0754880905151367,
                                rank=2,
                                decoded_token=" she",
                            ),
                        },
                        {
                            328: Logprob(
                                logprob=-3.1531925201416016, rank=5, decoded_token="!"
                            ),
                            3344: Logprob(
                                logprob=-1.0394372940063477,
                                rank=1,
                                decoded_token=" mix",
                            ),
                            4: Logprob(
                                logprob=-2.187213897705078, rank=2, decoded_token="."
                            ),
                        },
                        {
                            91: Logprob(
                                logprob=-1.4167288541793823, rank=1, decoded_token=" He"
                            ),
                            50118: Logprob(
                                logprob=-2.067265510559082, rank=2, decoded_token="\n"
                            ),
                        },
                        {
                            34: Logprob(
                                logprob=-2.7665247917175293,
                                rank=3,
                                decoded_token=" has",
                            ),
                            18: Logprob(
                                logprob=-1.0847479104995728, rank=1, decoded_token="'s"
                            ),
                            16: Logprob(
                                logprob=-1.5475212335586548, rank=2, decoded_token=" is"
                            ),
                        },
                        {
                            5262: Logprob(
                                logprob=-6.939647674560547,
                                rank=90,
                                decoded_token=" tiny",
                            ),
                            10: Logprob(
                                logprob=-1.3877274990081787, rank=1, decoded_token=" a"
                            ),
                            57: Logprob(
                                logprob=-2.3109357357025146,
                                rank=2,
                                decoded_token=" been",
                            ),
                        },
                        {
                            12137: Logprob(
                                logprob=-1.3619911670684814,
                                rank=1,
                                decoded_token=" ears",
                            ),
                            40844: Logprob(
                                logprob=-2.2743265628814697,
                                rank=2,
                                decoded_token=" paws",
                            ),
                        },
                        {
                            19: Logprob(
                                logprob=-4.619960784912109,
                                rank=10,
                                decoded_token=" with",
                            ),
                            8: Logprob(
                                logprob=-0.8057191371917725,
                                rank=1,
                                decoded_token=" and",
                            ),
                            6: Logprob(
                                logprob=-1.615569829940796, rank=2, decoded_token=","
                            ),
                        },
                    ],
                    finish_reason=None,
                    stop_reason=None,
                )
            ],
            finished=False,
            metrics=RequestMetrics(
                arrival_time=1719589967.687916,
                last_token_time=1719589968.1252568,
                first_scheduled_time=1719589967.6917963,
                first_token_time=1719589967.796725,
                time_in_queue=0.0038802623748779297,
                finished_time=None,
            ),
            lora_request=None,
        ),
        RequestOutput(
            request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
            prompt="Hi, I love my cat",
            prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
            prompt_logprobs=None,
            outputs=[
                CompletionOutput(
                    index=0,
                    text="- Labrador! He has tiny ears with fluffy",
                    token_ids=[12, 26882, 328, 91, 34, 5262, 12137, 19, 33564],
                    cumulative_logprob=-42.860944867134094,
                    logprobs=[
                        {
                            12: Logprob(
                                logprob=-5.968789577484131, rank=22, decoded_token="-"
                            ),
                            4: Logprob(
                                logprob=-1.4537543058395386, rank=1, decoded_token="."
                            ),
                            6: Logprob(
                                logprob=-1.8416975736618042, rank=2, decoded_token=","
                            ),
                        },
                        {
                            26882: Logprob(
                                logprob=-11.009230613708496,
                                rank=3032,
                                decoded_token=" Labrador",
                            ),
                            38: Logprob(
                                logprob=-1.7544232606887817, rank=1, decoded_token=" I"
                            ),
                            79: Logprob(
                                logprob=-3.0754880905151367,
                                rank=2,
                                decoded_token=" she",
                            ),
                        },
                        {
                            328: Logprob(
                                logprob=-3.1531925201416016, rank=5, decoded_token="!"
                            ),
                            3344: Logprob(
                                logprob=-1.0394372940063477,
                                rank=1,
                                decoded_token=" mix",
                            ),
                            4: Logprob(
                                logprob=-2.187213897705078, rank=2, decoded_token="."
                            ),
                        },
                        {
                            91: Logprob(
                                logprob=-1.4167288541793823, rank=1, decoded_token=" He"
                            ),
                            50118: Logprob(
                                logprob=-2.067265510559082, rank=2, decoded_token="\n"
                            ),
                        },
                        {
                            34: Logprob(
                                logprob=-2.7665247917175293,
                                rank=3,
                                decoded_token=" has",
                            ),
                            18: Logprob(
                                logprob=-1.0847479104995728, rank=1, decoded_token="'s"
                            ),
                            16: Logprob(
                                logprob=-1.5475212335586548, rank=2, decoded_token=" is"
                            ),
                        },
                        {
                            5262: Logprob(
                                logprob=-6.939647674560547,
                                rank=90,
                                decoded_token=" tiny",
                            ),
                            10: Logprob(
                                logprob=-1.3877274990081787, rank=1, decoded_token=" a"
                            ),
                            57: Logprob(
                                logprob=-2.3109357357025146,
                                rank=2,
                                decoded_token=" been",
                            ),
                        },
                        {
                            12137: Logprob(
                                logprob=-1.3619911670684814,
                                rank=1,
                                decoded_token=" ears",
                            ),
                            40844: Logprob(
                                logprob=-2.2743265628814697,
                                rank=2,
                                decoded_token=" paws",
                            ),
                        },
                        {
                            19: Logprob(
                                logprob=-4.619960784912109,
                                rank=10,
                                decoded_token=" with",
                            ),
                            8: Logprob(
                                logprob=-0.8057191371917725,
                                rank=1,
                                decoded_token=" and",
                            ),
                            6: Logprob(
                                logprob=-1.615569829940796, rank=2, decoded_token=","
                            ),
                        },
                        {
                            33564: Logprob(
                                logprob=-5.624878883361816,
                                rank=38,
                                decoded_token=" fluffy",
                            ),
                            10: Logprob(
                                logprob=-1.4977388381958008, rank=1, decoded_token=" a"
                            ),
                            5262: Logprob(
                                logprob=-3.0061492919921875,
                                rank=2,
                                decoded_token=" tiny",
                            ),
                        },
                    ],
                    finish_reason=None,
                    stop_reason=None,
                )
            ],
            finished=False,
            metrics=RequestMetrics(
                arrival_time=1719589967.687916,
                last_token_time=1719589968.1702635,
                first_scheduled_time=1719589967.6917963,
                first_token_time=1719589967.796725,
                time_in_queue=0.0038802623748779297,
                finished_time=None,
            ),
            lora_request=None,
        ),
        RequestOutput(
            request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
            prompt="Hi, I love my cat",
            prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
            prompt_logprobs=None,
            outputs=[
                CompletionOutput(
                    index=0,
                    text="- Labrador! He has tiny ears with fluffy white",
                    token_ids=[12, 26882, 328, 91, 34, 5262, 12137, 19, 33564, 1104],
                    cumulative_logprob=-45.076220631599426,
                    logprobs=[
                        {
                            12: Logprob(
                                logprob=-5.968789577484131, rank=22, decoded_token="-"
                            ),
                            4: Logprob(
                                logprob=-1.4537543058395386, rank=1, decoded_token="."
                            ),
                            6: Logprob(
                                logprob=-1.8416975736618042, rank=2, decoded_token=","
                            ),
                        },
                        {
                            26882: Logprob(
                                logprob=-11.009230613708496,
                                rank=3032,
                                decoded_token=" Labrador",
                            ),
                            38: Logprob(
                                logprob=-1.7544232606887817, rank=1, decoded_token=" I"
                            ),
                            79: Logprob(
                                logprob=-3.0754880905151367,
                                rank=2,
                                decoded_token=" she",
                            ),
                        },
                        {
                            328: Logprob(
                                logprob=-3.1531925201416016, rank=5, decoded_token="!"
                            ),
                            3344: Logprob(
                                logprob=-1.0394372940063477,
                                rank=1,
                                decoded_token=" mix",
                            ),
                            4: Logprob(
                                logprob=-2.187213897705078, rank=2, decoded_token="."
                            ),
                        },
                        {
                            91: Logprob(
                                logprob=-1.4167288541793823, rank=1, decoded_token=" He"
                            ),
                            50118: Logprob(
                                logprob=-2.067265510559082, rank=2, decoded_token="\n"
                            ),
                        },
                        {
                            34: Logprob(
                                logprob=-2.7665247917175293,
                                rank=3,
                                decoded_token=" has",
                            ),
                            18: Logprob(
                                logprob=-1.0847479104995728, rank=1, decoded_token="'s"
                            ),
                            16: Logprob(
                                logprob=-1.5475212335586548, rank=2, decoded_token=" is"
                            ),
                        },
                        {
                            5262: Logprob(
                                logprob=-6.939647674560547,
                                rank=90,
                                decoded_token=" tiny",
                            ),
                            10: Logprob(
                                logprob=-1.3877274990081787, rank=1, decoded_token=" a"
                            ),
                            57: Logprob(
                                logprob=-2.3109357357025146,
                                rank=2,
                                decoded_token=" been",
                            ),
                        },
                        {
                            12137: Logprob(
                                logprob=-1.3619911670684814,
                                rank=1,
                                decoded_token=" ears",
                            ),
                            40844: Logprob(
                                logprob=-2.2743265628814697,
                                rank=2,
                                decoded_token=" paws",
                            ),
                        },
                        {
                            19: Logprob(
                                logprob=-4.619960784912109,
                                rank=10,
                                decoded_token=" with",
                            ),
                            8: Logprob(
                                logprob=-0.8057191371917725,
                                rank=1,
                                decoded_token=" and",
                            ),
                            6: Logprob(
                                logprob=-1.615569829940796, rank=2, decoded_token=","
                            ),
                        },
                        {
                            33564: Logprob(
                                logprob=-5.624878883361816,
                                rank=38,
                                decoded_token=" fluffy",
                            ),
                            10: Logprob(
                                logprob=-1.4977388381958008, rank=1, decoded_token=" a"
                            ),
                            5262: Logprob(
                                logprob=-3.0061492919921875,
                                rank=2,
                                decoded_token=" tiny",
                            ),
                        },
                        {
                            1104: Logprob(
                                logprob=-2.215275764465332,
                                rank=2,
                                decoded_token=" white",
                            ),
                            15503: Logprob(
                                logprob=-1.901274561882019, rank=1, decoded_token=" fur"
                            ),
                        },
                    ],
                    finish_reason="length",
                    stop_reason=None,
                )
            ],
            finished=True,
            metrics=RequestMetrics(
                arrival_time=1719589967.687916,
                last_token_time=1719589968.2152452,
                first_scheduled_time=1719589967.6917963,
                first_token_time=1719589967.796725,
                time_in_queue=0.0038802623748779297,
                finished_time=1719589968.215228,
            ),
            lora_request=None,
        ),
    ],
    [
        RequestOutput(
            request_id="cmpl-d771287a234c44498e345f0a429d6691-1",
            prompt="The sky is blue",
            prompt_token_ids=[2, 133, 6360, 16, 2440],
            prompt_logprobs=None,
            outputs=[
                CompletionOutput(
                    index=0,
                    text=" and",
                    token_ids=[8],
                    cumulative_logprob=-2.4368906021118164,
                    logprobs=[
                        {
                            8: Logprob(
                                logprob=-2.4368906021118164,
                                rank=4,
                                decoded_token=" and",
                            ),
                            6: Logprob(
                                logprob=-1.4933549165725708, rank=1, decoded_token=","
                            ),
                            4: Logprob(
                                logprob=-1.4948359727859497, rank=2, decoded_token="."
                            ),
                        }
                    ],
                    finish_reason=None,
                    stop_reason=None,
                )
            ],
            finished=False,
            metrics=RequestMetrics(
                arrival_time=1719589967.6889987,
                last_token_time=1719589967.7971866,
                first_scheduled_time=1719589967.6917963,
                first_token_time=1719589967.796725,
                time_in_queue=0.0027976036071777344,
                finished_time=None,
            ),
            lora_request=None,
        ),
        RequestOutput(
            request_id="cmpl-d771287a234c44498e345f0a429d6691-1",
            prompt="The sky is blue",
            prompt_token_ids=[2, 133, 6360, 16, 2440],
            prompt_logprobs=None,
            outputs=[
                CompletionOutput(
                    index=0,
                    text=" and no",
                    token_ids=[8, 117],
                    cumulative_logprob=-7.690991401672363,
                    logprobs=[
                        {
                            8: Logprob(
                                logprob=-2.4368906021118164,
                                rank=4,
                                decoded_token=" and",
                            ),
                            6: Logprob(
                                logprob=-1.4933549165725708, rank=1, decoded_token=","
                            ),
                            4: Logprob(
                                logprob=-1.4948359727859497, rank=2, decoded_token="."
                            ),
                        },
                        {
                            117: Logprob(
                                logprob=-5.254100799560547, rank=25, decoded_token=" no"
                            ),
                            5: Logprob(
                                logprob=-1.2720444202423096,
                                rank=1,
                                decoded_token=" the",
                            ),
                            38: Logprob(
                                logprob=-2.4218027591705322, rank=2, decoded_token=" I"
                            ),
                        },
                    ],
                    finish_reason=None,
                    stop_reason=None,
                )
            ],
            finished=False,
            metrics=RequestMetrics(
                arrival_time=1719589967.6889987,
                last_token_time=1719589967.8479855,
                first_scheduled_time=1719589967.6917963,
                first_token_time=1719589967.796725,
                time_in_queue=0.0027976036071777344,
                finished_time=None,
            ),
            lora_request=None,
        ),
        RequestOutput(
            request_id="cmpl-d771287a234c44498e345f0a429d6691-1",
            prompt="The sky is blue",
            prompt_token_ids=[2, 133, 6360, 16, 2440],
            prompt_logprobs=None,
            outputs=[
                CompletionOutput(
                    index=0,
                    text=" and no one",
                    token_ids=[8, 117, 65],
                    cumulative_logprob=-8.11336663365364,
                    logprobs=[
                        {
                            8: Logprob(
                                logprob=-2.4368906021118164,
                                rank=4,
                                decoded_token=" and",
                            ),
                            6: Logprob(
                                logprob=-1.4933549165725708, rank=1, decoded_token=","
                            ),
                            4: Logprob(
                                logprob=-1.4948359727859497, rank=2, decoded_token="."
                            ),
                        },
                        {
                            117: Logprob(
                                logprob=-5.254100799560547, rank=25, decoded_token=" no"
                            ),
                            5: Logprob(
                                logprob=-1.2720444202423096,
                                rank=1,
                                decoded_token=" the",
                            ),
                            38: Logprob(
                                logprob=-2.4218027591705322, rank=2, decoded_token=" I"
                            ),
                        },
                        {
                            65: Logprob(
                                logprob=-0.42237523198127747,
                                rank=1,
                                decoded_token=" one",
                            ),
                            10722: Logprob(
                                logprob=-4.17390775680542,
                                rank=2,
                                decoded_token=" clouds",
                            ),
                        },
                    ],
                    finish_reason=None,
                    stop_reason=None,
                )
            ],
            finished=False,
            metrics=RequestMetrics(
                arrival_time=1719589967.6889987,
                last_token_time=1719589967.8971055,
                first_scheduled_time=1719589967.6917963,
                first_token_time=1719589967.796725,
                time_in_queue=0.0027976036071777344,
                finished_time=None,
            ),
            lora_request=None,
        ),
        RequestOutput(
            request_id="cmpl-d771287a234c44498e345f0a429d6691-1",
            prompt="The sky is blue",
            prompt_token_ids=[2, 133, 6360, 16, 2440],
            prompt_logprobs=None,
            outputs=[
                CompletionOutput(
                    index=0,
                    text=" and no one is",
                    token_ids=[8, 117, 65, 16],
                    cumulative_logprob=-9.278029590845108,
                    logprobs=[
                        {
                            8: Logprob(
                                logprob=-2.4368906021118164,
                                rank=4,
                                decoded_token=" and",
                            ),
                            6: Logprob(
                                logprob=-1.4933549165725708, rank=1, decoded_token=","
                            ),
                            4: Logprob(
                                logprob=-1.4948359727859497, rank=2, decoded_token="."
                            ),
                        },
                        {
                            117: Logprob(
                                logprob=-5.254100799560547, rank=25, decoded_token=" no"
                            ),
                            5: Logprob(
                                logprob=-1.2720444202423096,
                                rank=1,
                                decoded_token=" the",
                            ),
                            38: Logprob(
                                logprob=-2.4218027591705322, rank=2, decoded_token=" I"
                            ),
                        },
                        {
                            65: Logprob(
                                logprob=-0.42237523198127747,
                                rank=1,
                                decoded_token=" one",
                            ),
                            10722: Logprob(
                                logprob=-4.17390775680542,
                                rank=2,
                                decoded_token=" clouds",
                            ),
                        },
                        {
                            16: Logprob(
                                logprob=-1.1646629571914673, rank=1, decoded_token=" is"
                            ),
                            64: Logprob(
                                logprob=-2.5355124473571777,
                                rank=2,
                                decoded_token=" can",
                            ),
                        },
                    ],
                    finish_reason=None,
                    stop_reason=None,
                )
            ],
            finished=False,
            metrics=RequestMetrics(
                arrival_time=1719589967.6889987,
                last_token_time=1719589967.9417195,
                first_scheduled_time=1719589967.6917963,
                first_token_time=1719589967.796725,
                time_in_queue=0.0027976036071777344,
                finished_time=None,
            ),
            lora_request=None,
        ),
        RequestOutput(
            request_id="cmpl-d771287a234c44498e345f0a429d6691-1",
            prompt="The sky is blue",
            prompt_token_ids=[2, 133, 6360, 16, 2440],
            prompt_logprobs=None,
            outputs=[
                CompletionOutput(
                    index=0,
                    text=" and no one is going",
                    token_ids=[8, 117, 65, 16, 164],
                    cumulative_logprob=-12.09887209534645,
                    logprobs=[
                        {
                            8: Logprob(
                                logprob=-2.4368906021118164,
                                rank=4,
                                decoded_token=" and",
                            ),
                            6: Logprob(
                                logprob=-1.4933549165725708, rank=1, decoded_token=","
                            ),
                            4: Logprob(
                                logprob=-1.4948359727859497, rank=2, decoded_token="."
                            ),
                        },
                        {
                            117: Logprob(
                                logprob=-5.254100799560547, rank=25, decoded_token=" no"
                            ),
                            5: Logprob(
                                logprob=-1.2720444202423096,
                                rank=1,
                                decoded_token=" the",
                            ),
                            38: Logprob(
                                logprob=-2.4218027591705322, rank=2, decoded_token=" I"
                            ),
                        },
                        {
                            65: Logprob(
                                logprob=-0.42237523198127747,
                                rank=1,
                                decoded_token=" one",
                            ),
                            10722: Logprob(
                                logprob=-4.17390775680542,
                                rank=2,
                                decoded_token=" clouds",
                            ),
                        },
                        {
                            16: Logprob(
                                logprob=-1.1646629571914673, rank=1, decoded_token=" is"
                            ),
                            64: Logprob(
                                logprob=-2.5355124473571777,
                                rank=2,
                                decoded_token=" can",
                            ),
                        },
                        {
                            164: Logprob(
                                logprob=-2.8208425045013428,
                                rank=4,
                                decoded_token=" going",
                            ),
                            2494: Logprob(
                                logprob=-2.0994670391082764,
                                rank=1,
                                decoded_token=" watching",
                            ),
                            546: Logprob(
                                logprob=-2.5881574153900146,
                                rank=2,
                                decoded_token=" looking",
                            ),
                        },
                    ],
                    finish_reason=None,
                    stop_reason=None,
                )
            ],
            finished=False,
            metrics=RequestMetrics(
                arrival_time=1719589967.6889987,
                last_token_time=1719589967.9892497,
                first_scheduled_time=1719589967.6917963,
                first_token_time=1719589967.796725,
                time_in_queue=0.0027976036071777344,
                finished_time=None,
            ),
            lora_request=None,
        ),
        RequestOutput(
            request_id="cmpl-d771287a234c44498e345f0a429d6691-1",
            prompt="The sky is blue",
            prompt_token_ids=[2, 133, 6360, 16, 2440],
            prompt_logprobs=None,
            outputs=[
                CompletionOutput(
                    index=0,
                    text=" and no one is going to",
                    token_ids=[8, 117, 65, 16, 164, 7],
                    cumulative_logprob=-12.18747579306364,
                    logprobs=[
                        {
                            8: Logprob(
                                logprob=-2.4368906021118164,
                                rank=4,
                                decoded_token=" and",
                            ),
                            6: Logprob(
                                logprob=-1.4933549165725708, rank=1, decoded_token=","
                            ),
                            4: Logprob(
                                logprob=-1.4948359727859497, rank=2, decoded_token="."
                            ),
                        },
                        {
                            117: Logprob(
                                logprob=-5.254100799560547, rank=25, decoded_token=" no"
                            ),
                            5: Logprob(
                                logprob=-1.2720444202423096,
                                rank=1,
                                decoded_token=" the",
                            ),
                            38: Logprob(
                                logprob=-2.4218027591705322, rank=2, decoded_token=" I"
                            ),
                        },
                        {
                            65: Logprob(
                                logprob=-0.42237523198127747,
                                rank=1,
                                decoded_token=" one",
                            ),
                            10722: Logprob(
                                logprob=-4.17390775680542,
                                rank=2,
                                decoded_token=" clouds",
                            ),
                        },
                        {
                            16: Logprob(
                                logprob=-1.1646629571914673, rank=1, decoded_token=" is"
                            ),
                            64: Logprob(
                                logprob=-2.5355124473571777,
                                rank=2,
                                decoded_token=" can",
                            ),
                        },
                        {
                            164: Logprob(
                                logprob=-2.8208425045013428,
                                rank=4,
                                decoded_token=" going",
                            ),
                            2494: Logprob(
                                logprob=-2.0994670391082764,
                                rank=1,
                                decoded_token=" watching",
                            ),
                            546: Logprob(
                                logprob=-2.5881574153900146,
                                rank=2,
                                decoded_token=" looking",
                            ),
                        },
                        {
                            7: Logprob(
                                logprob=-0.08860369771718979,
                                rank=1,
                                decoded_token=" to",
                            ),
                            4558: Logprob(
                                logprob=-3.895568609237671,
                                rank=2,
                                decoded_token=" anywhere",
                            ),
                        },
                    ],
                    finish_reason=None,
                    stop_reason=None,
                )
            ],
            finished=False,
            metrics=RequestMetrics(
                arrival_time=1719589967.6889987,
                last_token_time=1719589968.0347881,
                first_scheduled_time=1719589967.6917963,
                first_token_time=1719589967.796725,
                time_in_queue=0.0027976036071777344,
                finished_time=None,
            ),
            lora_request=None,
        ),
        RequestOutput(
            request_id="cmpl-d771287a234c44498e345f0a429d6691-1",
            prompt="The sky is blue",
            prompt_token_ids=[2, 133, 6360, 16, 2440],
            prompt_logprobs=None,
            outputs=[
                CompletionOutput(
                    index=0,
                    text=" and no one is going to notice",
                    token_ids=[8, 117, 65, 16, 164, 7, 3120],
                    cumulative_logprob=-14.225699536502361,
                    logprobs=[
                        {
                            8: Logprob(
                                logprob=-2.4368906021118164,
                                rank=4,
                                decoded_token=" and",
                            ),
                            6: Logprob(
                                logprob=-1.4933549165725708, rank=1, decoded_token=","
                            ),
                            4: Logprob(
                                logprob=-1.4948359727859497, rank=2, decoded_token="."
                            ),
                        },
                        {
                            117: Logprob(
                                logprob=-5.254100799560547, rank=25, decoded_token=" no"
                            ),
                            5: Logprob(
                                logprob=-1.2720444202423096,
                                rank=1,
                                decoded_token=" the",
                            ),
                            38: Logprob(
                                logprob=-2.4218027591705322, rank=2, decoded_token=" I"
                            ),
                        },
                        {
                            65: Logprob(
                                logprob=-0.42237523198127747,
                                rank=1,
                                decoded_token=" one",
                            ),
                            10722: Logprob(
                                logprob=-4.17390775680542,
                                rank=2,
                                decoded_token=" clouds",
                            ),
                        },
                        {
                            16: Logprob(
                                logprob=-1.1646629571914673, rank=1, decoded_token=" is"
                            ),
                            64: Logprob(
                                logprob=-2.5355124473571777,
                                rank=2,
                                decoded_token=" can",
                            ),
                        },
                        {
                            164: Logprob(
                                logprob=-2.8208425045013428,
                                rank=4,
                                decoded_token=" going",
                            ),
                            2494: Logprob(
                                logprob=-2.0994670391082764,
                                rank=1,
                                decoded_token=" watching",
                            ),
                            546: Logprob(
                                logprob=-2.5881574153900146,
                                rank=2,
                                decoded_token=" looking",
                            ),
                        },
                        {
                            7: Logprob(
                                logprob=-0.08860369771718979,
                                rank=1,
                                decoded_token=" to",
                            ),
                            4558: Logprob(
                                logprob=-3.895568609237671,
                                rank=2,
                                decoded_token=" anywhere",
                            ),
                        },
                        {
                            3120: Logprob(
                                logprob=-2.0382237434387207,
                                rank=1,
                                decoded_token=" notice",
                            ),
                            192: Logprob(
                                logprob=-2.475170612335205, rank=2, decoded_token=" see"
                            ),
                        },
                    ],
                    finish_reason=None,
                    stop_reason=None,
                )
            ],
            finished=False,
            metrics=RequestMetrics(
                arrival_time=1719589967.6889987,
                last_token_time=1719589968.0797527,
                first_scheduled_time=1719589967.6917963,
                first_token_time=1719589967.796725,
                time_in_queue=0.0027976036071777344,
                finished_time=None,
            ),
            lora_request=None,
        ),
        RequestOutput(
            request_id="cmpl-d771287a234c44498e345f0a429d6691-1",
            prompt="The sky is blue",
            prompt_token_ids=[2, 133, 6360, 16, 2440],
            prompt_logprobs=None,
            outputs=[
                CompletionOutput(
                    index=0,
                    text=" and no one is going to notice.",
                    token_ids=[8, 117, 65, 16, 164, 7, 3120, 4],
                    cumulative_logprob=-15.519690982997417,
                    logprobs=[
                        {
                            8: Logprob(
                                logprob=-2.4368906021118164,
                                rank=4,
                                decoded_token=" and",
                            ),
                            6: Logprob(
                                logprob=-1.4933549165725708, rank=1, decoded_token=","
                            ),
                            4: Logprob(
                                logprob=-1.4948359727859497, rank=2, decoded_token="."
                            ),
                        },
                        {
                            117: Logprob(
                                logprob=-5.254100799560547, rank=25, decoded_token=" no"
                            ),
                            5: Logprob(
                                logprob=-1.2720444202423096,
                                rank=1,
                                decoded_token=" the",
                            ),
                            38: Logprob(
                                logprob=-2.4218027591705322, rank=2, decoded_token=" I"
                            ),
                        },
                        {
                            65: Logprob(
                                logprob=-0.42237523198127747,
                                rank=1,
                                decoded_token=" one",
                            ),
                            10722: Logprob(
                                logprob=-4.17390775680542,
                                rank=2,
                                decoded_token=" clouds",
                            ),
                        },
                        {
                            16: Logprob(
                                logprob=-1.1646629571914673, rank=1, decoded_token=" is"
                            ),
                            64: Logprob(
                                logprob=-2.5355124473571777,
                                rank=2,
                                decoded_token=" can",
                            ),
                        },
                        {
                            164: Logprob(
                                logprob=-2.8208425045013428,
                                rank=4,
                                decoded_token=" going",
                            ),
                            2494: Logprob(
                                logprob=-2.0994670391082764,
                                rank=1,
                                decoded_token=" watching",
                            ),
                            546: Logprob(
                                logprob=-2.5881574153900146,
                                rank=2,
                                decoded_token=" looking",
                            ),
                        },
                        {
                            7: Logprob(
                                logprob=-0.08860369771718979,
                                rank=1,
                                decoded_token=" to",
                            ),
                            4558: Logprob(
                                logprob=-3.895568609237671,
                                rank=2,
                                decoded_token=" anywhere",
                            ),
                        },
                        {
                            3120: Logprob(
                                logprob=-2.0382237434387207,
                                rank=1,
                                decoded_token=" notice",
                            ),
                            192: Logprob(
                                logprob=-2.475170612335205, rank=2, decoded_token=" see"
                            ),
                        },
                        {
                            4: Logprob(
                                logprob=-1.2939914464950562, rank=1, decoded_token="."
                            ),
                            24: Logprob(
                                logprob=-1.670294165611267, rank=2, decoded_token=" it"
                            ),
                        },
                    ],
                    finish_reason=None,
                    stop_reason=None,
                )
            ],
            finished=False,
            metrics=RequestMetrics(
                arrival_time=1719589967.6889987,
                last_token_time=1719589968.1252568,
                first_scheduled_time=1719589967.6917963,
                first_token_time=1719589967.796725,
                time_in_queue=0.0027976036071777344,
                finished_time=None,
            ),
            lora_request=None,
        ),
        RequestOutput(
            request_id="cmpl-d771287a234c44498e345f0a429d6691-1",
            prompt="The sky is blue",
            prompt_token_ids=[2, 133, 6360, 16, 2440],
            prompt_logprobs=None,
            outputs=[
                CompletionOutput(
                    index=0,
                    text=" and no one is going to notice. You",
                    token_ids=[8, 117, 65, 16, 164, 7, 3120, 4, 370],
                    cumulative_logprob=-20.042794220149517,
                    logprobs=[
                        {
                            8: Logprob(
                                logprob=-2.4368906021118164,
                                rank=4,
                                decoded_token=" and",
                            ),
                            6: Logprob(
                                logprob=-1.4933549165725708, rank=1, decoded_token=","
                            ),
                            4: Logprob(
                                logprob=-1.4948359727859497, rank=2, decoded_token="."
                            ),
                        },
                        {
                            117: Logprob(
                                logprob=-5.254100799560547, rank=25, decoded_token=" no"
                            ),
                            5: Logprob(
                                logprob=-1.2720444202423096,
                                rank=1,
                                decoded_token=" the",
                            ),
                            38: Logprob(
                                logprob=-2.4218027591705322, rank=2, decoded_token=" I"
                            ),
                        },
                        {
                            65: Logprob(
                                logprob=-0.42237523198127747,
                                rank=1,
                                decoded_token=" one",
                            ),
                            10722: Logprob(
                                logprob=-4.17390775680542,
                                rank=2,
                                decoded_token=" clouds",
                            ),
                        },
                        {
                            16: Logprob(
                                logprob=-1.1646629571914673, rank=1, decoded_token=" is"
                            ),
                            64: Logprob(
                                logprob=-2.5355124473571777,
                                rank=2,
                                decoded_token=" can",
                            ),
                        },
                        {
                            164: Logprob(
                                logprob=-2.8208425045013428,
                                rank=4,
                                decoded_token=" going",
                            ),
                            2494: Logprob(
                                logprob=-2.0994670391082764,
                                rank=1,
                                decoded_token=" watching",
                            ),
                            546: Logprob(
                                logprob=-2.5881574153900146,
                                rank=2,
                                decoded_token=" looking",
                            ),
                        },
                        {
                            7: Logprob(
                                logprob=-0.08860369771718979,
                                rank=1,
                                decoded_token=" to",
                            ),
                            4558: Logprob(
                                logprob=-3.895568609237671,
                                rank=2,
                                decoded_token=" anywhere",
                            ),
                        },
                        {
                            3120: Logprob(
                                logprob=-2.0382237434387207,
                                rank=1,
                                decoded_token=" notice",
                            ),
                            192: Logprob(
                                logprob=-2.475170612335205, rank=2, decoded_token=" see"
                            ),
                        },
                        {
                            4: Logprob(
                                logprob=-1.2939914464950562, rank=1, decoded_token="."
                            ),
                            24: Logprob(
                                logprob=-1.670294165611267, rank=2, decoded_token=" it"
                            ),
                        },
                        {
                            370: Logprob(
                                logprob=-4.5231032371521, rank=6, decoded_token=" You"
                            ),
                            50118: Logprob(
                                logprob=-0.5480296015739441, rank=1, decoded_token="\n"
                            ),
                            1437: Logprob(
                                logprob=-2.24289870262146, rank=2, decoded_token=" "
                            ),
                        },
                    ],
                    finish_reason=None,
                    stop_reason=None,
                )
            ],
            finished=False,
            metrics=RequestMetrics(
                arrival_time=1719589967.6889987,
                last_token_time=1719589968.1702635,
                first_scheduled_time=1719589967.6917963,
                first_token_time=1719589967.796725,
                time_in_queue=0.0027976036071777344,
                finished_time=None,
            ),
            lora_request=None,
        ),
        RequestOutput(
            request_id="cmpl-d771287a234c44498e345f0a429d6691-1",
            prompt="The sky is blue",
            prompt_token_ids=[2, 133, 6360, 16, 2440],
            prompt_logprobs=None,
            outputs=[
                CompletionOutput(
                    index=0,
                    text=" and no one is going to notice. You don",
                    token_ids=[8, 117, 65, 16, 164, 7, 3120, 4, 370, 218],
                    cumulative_logprob=-23.34256123751402,
                    logprobs=[
                        {
                            8: Logprob(
                                logprob=-2.4368906021118164,
                                rank=4,
                                decoded_token=" and",
                            ),
                            6: Logprob(
                                logprob=-1.4933549165725708, rank=1, decoded_token=","
                            ),
                            4: Logprob(
                                logprob=-1.4948359727859497, rank=2, decoded_token="."
                            ),
                        },
                        {
                            117: Logprob(
                                logprob=-5.254100799560547, rank=25, decoded_token=" no"
                            ),
                            5: Logprob(
                                logprob=-1.2720444202423096,
                                rank=1,
                                decoded_token=" the",
                            ),
                            38: Logprob(
                                logprob=-2.4218027591705322, rank=2, decoded_token=" I"
                            ),
                        },
                        {
                            65: Logprob(
                                logprob=-0.42237523198127747,
                                rank=1,
                                decoded_token=" one",
                            ),
                            10722: Logprob(
                                logprob=-4.17390775680542,
                                rank=2,
                                decoded_token=" clouds",
                            ),
                        },
                        {
                            16: Logprob(
                                logprob=-1.1646629571914673, rank=1, decoded_token=" is"
                            ),
                            64: Logprob(
                                logprob=-2.5355124473571777,
                                rank=2,
                                decoded_token=" can",
                            ),
                        },
                        {
                            164: Logprob(
                                logprob=-2.8208425045013428,
                                rank=4,
                                decoded_token=" going",
                            ),
                            2494: Logprob(
                                logprob=-2.0994670391082764,
                                rank=1,
                                decoded_token=" watching",
                            ),
                            546: Logprob(
                                logprob=-2.5881574153900146,
                                rank=2,
                                decoded_token=" looking",
                            ),
                        },
                        {
                            7: Logprob(
                                logprob=-0.08860369771718979,
                                rank=1,
                                decoded_token=" to",
                            ),
                            4558: Logprob(
                                logprob=-3.895568609237671,
                                rank=2,
                                decoded_token=" anywhere",
                            ),
                        },
                        {
                            3120: Logprob(
                                logprob=-2.0382237434387207,
                                rank=1,
                                decoded_token=" notice",
                            ),
                            192: Logprob(
                                logprob=-2.475170612335205, rank=2, decoded_token=" see"
                            ),
                        },
                        {
                            4: Logprob(
                                logprob=-1.2939914464950562, rank=1, decoded_token="."
                            ),
                            24: Logprob(
                                logprob=-1.670294165611267, rank=2, decoded_token=" it"
                            ),
                        },
                        {
                            370: Logprob(
                                logprob=-4.5231032371521, rank=6, decoded_token=" You"
                            ),
                            50118: Logprob(
                                logprob=-0.5480296015739441, rank=1, decoded_token="\n"
                            ),
                            1437: Logprob(
                                logprob=-2.24289870262146, rank=2, decoded_token=" "
                            ),
                        },
                        {
                            218: Logprob(
                                logprob=-3.299767017364502, rank=6, decoded_token=" don"
                            ),
                            64: Logprob(
                                logprob=-2.143829822540283, rank=1, decoded_token=" can"
                            ),
                            214: Logprob(
                                logprob=-2.1697640419006348, rank=2, decoded_token="'re"
                            ),
                        },
                    ],
                    finish_reason="length",
                    stop_reason=None,
                )
            ],
            finished=True,
            metrics=RequestMetrics(
                arrival_time=1719589967.6889987,
                last_token_time=1719589968.2152452,
                first_scheduled_time=1719589967.6917963,
                first_token_time=1719589967.796725,
                time_in_queue=0.0027976036071777344,
                finished_time=1719589968.2152417,
            ),
            lora_request=None,
        ),
    ],
]

opt_cmpl_chunks_with_n_2 = [
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text=",",
                token_ids=[6],
                cumulative_logprob=-1.8416948318481445,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            ),
            CompletionOutput(
                index=2,
                text=" and",
                token_ids=[8],
                cumulative_logprob=-2.2421159744262695,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            ),
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719640772.0308266,
            last_token_time=1719640772.1047966,
            first_scheduled_time=1719640772.0334153,
            first_token_time=1719640772.1039727,
            time_in_queue=0.0025887489318847656,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text=", so",
                token_ids=[6, 98],
                cumulative_logprob=-5.305910110473633,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            ),
            CompletionOutput(
                index=2,
                text=" and myself",
                token_ids=[8, 2185],
                cumulative_logprob=-9.334444522857666,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            ),
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719640772.0308266,
            last_token_time=1719640772.1524968,
            first_scheduled_time=1719640772.0334153,
            first_token_time=1719640772.1039727,
            time_in_queue=0.0025887489318847656,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text=", so I",
                token_ids=[6, 98, 38],
                cumulative_logprob=-6.166063189506531,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            ),
            CompletionOutput(
                index=2,
                text=" and myself.",
                token_ids=[8, 2185, 4],
                cumulative_logprob=-11.079005599021912,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            ),
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719640772.0308266,
            last_token_time=1719640772.1984475,
            first_scheduled_time=1719640772.0334153,
            first_token_time=1719640772.1039727,
            time_in_queue=0.0025887489318847656,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text=", so I know",
                token_ids=[6, 98, 38, 216],
                cumulative_logprob=-10.287340998649597,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            ),
            CompletionOutput(
                index=2,
                text=" and myself. ",
                token_ids=[8, 2185, 4, 1437],
                cumulative_logprob=-13.231551051139832,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            ),
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719640772.0308266,
            last_token_time=1719640772.2463255,
            first_scheduled_time=1719640772.0334153,
            first_token_time=1719640772.1039727,
            time_in_queue=0.0025887489318847656,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text=", so I know how",
                token_ids=[6, 98, 38, 216, 141],
                cumulative_logprob=-12.755180716514587,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            ),
            CompletionOutput(
                index=2,
                text=" and myself.  Sometimes",
                token_ids=[8, 2185, 4, 1437, 7411],
                cumulative_logprob=-19.232725024223328,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            ),
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719640772.0308266,
            last_token_time=1719640772.2914917,
            first_scheduled_time=1719640772.0334153,
            first_token_time=1719640772.1039727,
            time_in_queue=0.0025887489318847656,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text=", so I know how much",
                token_ids=[6, 98, 38, 216, 141, 203],
                cumulative_logprob=-14.936209082603455,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            ),
            CompletionOutput(
                index=2,
                text=" and myself.  Sometimes I",
                token_ids=[8, 2185, 4, 1437, 7411, 38],
                cumulative_logprob=-19.991046369075775,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            ),
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719640772.0308266,
            last_token_time=1719640772.336993,
            first_scheduled_time=1719640772.0334153,
            first_token_time=1719640772.1039727,
            time_in_queue=0.0025887489318847656,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text=", so I know how much you",
                token_ids=[6, 98, 38, 216, 141, 203, 47],
                cumulative_logprob=-16.388848900794983,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            ),
            CompletionOutput(
                index=2,
                text=" and myself.  Sometimes I try",
                token_ids=[8, 2185, 4, 1437, 7411, 38, 860],
                cumulative_logprob=-23.83206480741501,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            ),
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719640772.0308266,
            last_token_time=1719640772.383404,
            first_scheduled_time=1719640772.0334153,
            first_token_time=1719640772.1039727,
            time_in_queue=0.0025887489318847656,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text=", so I know how much you guys",
                token_ids=[6, 98, 38, 216, 141, 203, 47, 1669],
                cumulative_logprob=-19.739151120185852,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            ),
            CompletionOutput(
                index=2,
                text=" and myself.  Sometimes I try to",
                token_ids=[8, 2185, 4, 1437, 7411, 38, 860, 7],
                cumulative_logprob=-23.989818647503853,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            ),
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719640772.0308266,
            last_token_time=1719640772.4289408,
            first_scheduled_time=1719640772.0334153,
            first_token_time=1719640772.1039727,
            time_in_queue=0.0025887489318847656,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text=", so I know how much you guys are",
                token_ids=[6, 98, 38, 216, 141, 203, 47, 1669, 32],
                cumulative_logprob=-22.317527890205383,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            ),
            CompletionOutput(
                index=2,
                text=" and myself.  Sometimes I try to pick",
                token_ids=[8, 2185, 4, 1437, 7411, 38, 860, 7, 1339],
                cumulative_logprob=-28.86552245914936,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            ),
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719640772.0308266,
            last_token_time=1719640772.4743848,
            first_scheduled_time=1719640772.0334153,
            first_token_time=1719640772.1039727,
            time_in_queue=0.0025887489318847656,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text=", so I know how much you guys are needing",
                token_ids=[6, 98, 38, 216, 141, 203, 47, 1669, 32, 12075],
                cumulative_logprob=-28.802258610725403,
                logprobs=None,
                finish_reason="length",
                stop_reason=None,
            ),
            CompletionOutput(
                index=2,
                text=" and myself.  Sometimes I try to pick my",
                token_ids=[8, 2185, 4, 1437, 7411, 38, 860, 7, 1339, 127],
                cumulative_logprob=-32.67578609287739,
                logprobs=None,
                finish_reason="length",
                stop_reason=None,
            ),
        ],
        finished=True,
        metrics=RequestMetrics(
            arrival_time=1719640772.0308266,
            last_token_time=1719640772.5209877,
            first_scheduled_time=1719640772.0334153,
            first_token_time=1719640772.1039727,
            time_in_queue=0.0025887489318847656,
            finished_time=1719640772.5209823,
        ),
        lora_request=None,
    ),
]

opt_cmpl_chunks_with_n_3 = [
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text=",",
                token_ids=[6],
                cumulative_logprob=-1.8416948318481445,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            ),
            CompletionOutput(
                index=2,
                text=" and",
                token_ids=[8],
                cumulative_logprob=-2.2421159744262695,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            ),
            CompletionOutput(
                index=1,
                text="-",
                token_ids=[12],
                cumulative_logprob=-5.968788146972656,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            ),
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719641806.1763275,
            last_token_time=1719641806.256669,
            first_scheduled_time=1719641806.1789184,
            first_token_time=1719641806.2558353,
            time_in_queue=0.0025908946990966797,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text=", so",
                token_ids=[6, 98],
                cumulative_logprob=-5.305910110473633,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            ),
            CompletionOutput(
                index=2,
                text=" and myself",
                token_ids=[8, 2185],
                cumulative_logprob=-9.334444522857666,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            ),
            CompletionOutput(
                index=1,
                text="-new",
                token_ids=[12, 4651],
                cumulative_logprob=-15.287845611572266,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            ),
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719641806.1763275,
            last_token_time=1719641806.3042936,
            first_scheduled_time=1719641806.1789184,
            first_token_time=1719641806.2558353,
            time_in_queue=0.0025908946990966797,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text=", so I",
                token_ids=[6, 98, 38],
                cumulative_logprob=-6.166063189506531,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            ),
            CompletionOutput(
                index=2,
                text=" and myself.",
                token_ids=[8, 2185, 4],
                cumulative_logprob=-11.079005599021912,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            ),
            CompletionOutput(
                index=1,
                text="-newbie",
                token_ids=[12, 4651, 12750],
                cumulative_logprob=-17.796749114990234,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            ),
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719641806.1763275,
            last_token_time=1719641806.3501604,
            first_scheduled_time=1719641806.1789184,
            first_token_time=1719641806.2558353,
            time_in_queue=0.0025908946990966797,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text=", so I know",
                token_ids=[6, 98, 38, 216],
                cumulative_logprob=-10.287340998649597,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            ),
            CompletionOutput(
                index=2,
                text=" and myself. ",
                token_ids=[8, 2185, 4, 1437],
                cumulative_logprob=-13.231551051139832,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            ),
            CompletionOutput(
                index=1,
                text="-newbie and",
                token_ids=[12, 4651, 12750, 8],
                cumulative_logprob=-20.89390254020691,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            ),
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719641806.1763275,
            last_token_time=1719641806.3966303,
            first_scheduled_time=1719641806.1789184,
            first_token_time=1719641806.2558353,
            time_in_queue=0.0025908946990966797,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text=", so I know how",
                token_ids=[6, 98, 38, 216, 141],
                cumulative_logprob=-12.755180716514587,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            ),
            CompletionOutput(
                index=2,
                text=" and myself.  Sometimes",
                token_ids=[8, 2185, 4, 1437, 7411],
                cumulative_logprob=-19.232725024223328,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            ),
            CompletionOutput(
                index=1,
                text="-newbie and don",
                token_ids=[12, 4651, 12750, 8, 218],
                cumulative_logprob=-25.38681435585022,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            ),
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719641806.1763275,
            last_token_time=1719641806.4434476,
            first_scheduled_time=1719641806.1789184,
            first_token_time=1719641806.2558353,
            time_in_queue=0.0025908946990966797,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text=", so I know how much",
                token_ids=[6, 98, 38, 216, 141, 203],
                cumulative_logprob=-14.936209082603455,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            ),
            CompletionOutput(
                index=2,
                text=" and myself.  Sometimes I",
                token_ids=[8, 2185, 4, 1437, 7411, 38],
                cumulative_logprob=-19.991046369075775,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            ),
            CompletionOutput(
                index=1,
                text="-newbie and don't",
                token_ids=[12, 4651, 12750, 8, 218, 75],
                cumulative_logprob=-25.490039318799973,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            ),
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719641806.1763275,
            last_token_time=1719641806.4952884,
            first_scheduled_time=1719641806.1789184,
            first_token_time=1719641806.2558353,
            time_in_queue=0.0025908946990966797,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text=", so I know how much you",
                token_ids=[6, 98, 38, 216, 141, 203, 47],
                cumulative_logprob=-16.388848900794983,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            ),
            CompletionOutput(
                index=2,
                text=" and myself.  Sometimes I try",
                token_ids=[8, 2185, 4, 1437, 7411, 38, 860],
                cumulative_logprob=-23.83206480741501,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            ),
            CompletionOutput(
                index=1,
                text="-newbie and don't generally",
                token_ids=[12, 4651, 12750, 8, 218, 75, 3489],
                cumulative_logprob=-33.25731226801872,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            ),
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719641806.1763275,
            last_token_time=1719641806.547046,
            first_scheduled_time=1719641806.1789184,
            first_token_time=1719641806.2558353,
            time_in_queue=0.0025908946990966797,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text=", so I know how much you guys",
                token_ids=[6, 98, 38, 216, 141, 203, 47, 1669],
                cumulative_logprob=-19.739151120185852,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            ),
            CompletionOutput(
                index=2,
                text=" and myself.  Sometimes I try to",
                token_ids=[8, 2185, 4, 1437, 7411, 38, 860, 7],
                cumulative_logprob=-23.989818647503853,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            ),
            CompletionOutput(
                index=1,
                text="-newbie and don't generally seek",
                token_ids=[12, 4651, 12750, 8, 218, 75, 3489, 2639],
                cumulative_logprob=-39.63435980677605,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            ),
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719641806.1763275,
            last_token_time=1719641806.5982373,
            first_scheduled_time=1719641806.1789184,
            first_token_time=1719641806.2558353,
            time_in_queue=0.0025908946990966797,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text=", so I know how much you guys are",
                token_ids=[6, 98, 38, 216, 141, 203, 47, 1669, 32],
                cumulative_logprob=-22.317527890205383,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            ),
            CompletionOutput(
                index=2,
                text=" and myself.  Sometimes I try to pick",
                token_ids=[8, 2185, 4, 1437, 7411, 38, 860, 7, 1339],
                cumulative_logprob=-28.86552245914936,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            ),
            CompletionOutput(
                index=1,
                text="-newbie and don't generally seek it",
                token_ids=[12, 4651, 12750, 8, 218, 75, 3489, 2639, 24],
                cumulative_logprob=-45.03968760371208,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            ),
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719641806.1763275,
            last_token_time=1719641806.6489444,
            first_scheduled_time=1719641806.1789184,
            first_token_time=1719641806.2558353,
            time_in_queue=0.0025908946990966797,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text=", so I know how much you guys are needing",
                token_ids=[6, 98, 38, 216, 141, 203, 47, 1669, 32, 12075],
                cumulative_logprob=-28.802258610725403,
                logprobs=None,
                finish_reason="length",
                stop_reason=None,
            ),
            CompletionOutput(
                index=2,
                text=" and myself.  Sometimes I try to pick my",
                token_ids=[8, 2185, 4, 1437, 7411, 38, 860, 7, 1339, 127],
                cumulative_logprob=-32.67578609287739,
                logprobs=None,
                finish_reason="length",
                stop_reason=None,
            ),
            CompletionOutput(
                index=1,
                text="-newbie and don't generally seek it out",
                token_ids=[12, 4651, 12750, 8, 218, 75, 3489, 2639, 24, 66],
                cumulative_logprob=-45.20532666146755,
                logprobs=None,
                finish_reason="length",
                stop_reason=None,
            ),
        ],
        finished=True,
        metrics=RequestMetrics(
            arrival_time=1719641806.1763275,
            last_token_time=1719641806.7002027,
            first_scheduled_time=1719641806.1789184,
            first_token_time=1719641806.2558353,
            time_in_queue=0.0025908946990966797,
            finished_time=1719641806.7001975,
        ),
        lora_request=None,
    ),
]

opt_cmpl_chunks_with_logit_bias = [
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text="-",
                token_ids=[12],
                cumulative_logprob=-5.96877384185791,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719659778.9400077,
            last_token_time=1719659779.0637863,
            first_scheduled_time=1719659778.9423137,
            first_token_time=1719659779.0626233,
            time_in_queue=0.0023059844970703125,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text="- Labrador",
                token_ids=[12, 26882],
                cumulative_logprob=-16.977975845336914,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719659778.9400077,
            last_token_time=1719659779.1451252,
            first_scheduled_time=1719659778.9423137,
            first_token_time=1719659779.0626233,
            time_in_queue=0.0023059844970703125,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text="- Labrador!",
                token_ids=[12, 26882, 328],
                cumulative_logprob=-20.131160736083984,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719659778.9400077,
            last_token_time=1719659779.222749,
            first_scheduled_time=1719659778.9423137,
            first_token_time=1719659779.0626233,
            time_in_queue=0.0023059844970703125,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text="- Labrador! He",
                token_ids=[12, 26882, 328, 91],
                cumulative_logprob=-21.547887325286865,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719659778.9400077,
            last_token_time=1719659779.2765543,
            first_scheduled_time=1719659778.9423137,
            first_token_time=1719659779.0626233,
            time_in_queue=0.0023059844970703125,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text="- Labrador! He has",
                token_ids=[12, 26882, 328, 91, 34],
                cumulative_logprob=-24.31441068649292,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719659778.9400077,
            last_token_time=1719659779.3302877,
            first_scheduled_time=1719659778.9423137,
            first_token_time=1719659779.0626233,
            time_in_queue=0.0023059844970703125,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text="- Labrador! He has tiny",
                token_ids=[12, 26882, 328, 91, 34, 5262],
                cumulative_logprob=-31.253928184509277,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719659778.9400077,
            last_token_time=1719659779.3823342,
            first_scheduled_time=1719659778.9423137,
            first_token_time=1719659779.0626233,
            time_in_queue=0.0023059844970703125,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text="- Labrador! He has tiny ears",
                token_ids=[12, 26882, 328, 91, 34, 5262, 12137],
                cumulative_logprob=-32.61497759819031,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719659778.9400077,
            last_token_time=1719659779.4363637,
            first_scheduled_time=1719659778.9423137,
            first_token_time=1719659779.0626233,
            time_in_queue=0.0023059844970703125,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text="- Labrador! He has tiny ears with",
                token_ids=[12, 26882, 328, 91, 34, 5262, 12137, 19],
                cumulative_logprob=-37.2349374294281,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719659778.9400077,
            last_token_time=1719659779.4887528,
            first_scheduled_time=1719659778.9423137,
            first_token_time=1719659779.0626233,
            time_in_queue=0.0023059844970703125,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text="- Labrador! He has tiny ears with red",
                token_ids=[12, 26882, 328, 91, 34, 5262, 12137, 19, 1275],
                cumulative_logprob=-41.17036414146423,
                logprobs=None,
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719659778.9400077,
            last_token_time=1719659779.541681,
            first_scheduled_time=1719659778.9423137,
            first_token_time=1719659779.0626233,
            time_in_queue=0.0023059844970703125,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=None,
        outputs=[
            CompletionOutput(
                index=0,
                text="- Labrador! He has tiny ears with red hair",
                token_ids=[12, 26882, 328, 91, 34, 5262, 12137, 19, 1275, 2549],
                cumulative_logprob=-43.83025312423706,
                logprobs=None,
                finish_reason="length",
                stop_reason=None,
            )
        ],
        finished=True,
        metrics=RequestMetrics(
            arrival_time=1719659778.9400077,
            last_token_time=1719659779.5935142,
            first_scheduled_time=1719659778.9423137,
            first_token_time=1719659779.0626233,
            time_in_queue=0.0023059844970703125,
            finished_time=1719659779.5935032,
        ),
        lora_request=None,
    ),
]

opt_cmpl_chunks_with_echo_logprobs = [
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=[
            None,
            {
                30086: Logprob(
                    logprob=-9.352765083312988, rank=369, decoded_token="Hi"
                ),
                100: Logprob(logprob=-1.4278708696365356, rank=1, decoded_token="I"),
                133: Logprob(logprob=-2.4365129470825195, rank=2, decoded_token="The"),
            },
            {
                6: Logprob(logprob=-1.4278249740600586, rank=1, decoded_token=","),
                328: Logprob(logprob=-1.934173583984375, rank=2, decoded_token="!"),
            },
            {
                38: Logprob(logprob=-0.976689338684082, rank=1, decoded_token=" I"),
                1437: Logprob(logprob=-2.723400115966797, rank=2, decoded_token=" "),
            },
            {
                657: Logprob(
                    logprob=-5.6148481369018555, rank=22, decoded_token=" love"
                ),
                437: Logprob(logprob=-1.015452265739441, rank=1, decoded_token="'m"),
                33: Logprob(logprob=-1.9374703168869019, rank=2, decoded_token=" have"),
            },
            {
                127: Logprob(logprob=-4.214991569519043, rank=9, decoded_token=" my"),
                110: Logprob(
                    logprob=-1.7619359493255615, rank=1, decoded_token=" your"
                ),
                5: Logprob(logprob=-1.999145269393921, rank=2, decoded_token=" the"),
            },
            {
                4758: Logprob(logprob=-4.99854040145874, rank=3, decoded_token=" cat"),
                92: Logprob(logprob=-3.4642574787139893, rank=1, decoded_token=" new"),
                793: Logprob(logprob=-4.73804235458374, rank=2, decoded_token=" old"),
            },
        ],
        outputs=[
            CompletionOutput(
                index=0,
                text="-",
                token_ids=[12],
                cumulative_logprob=-5.968787670135498,
                logprobs=[
                    {
                        12: Logprob(
                            logprob=-5.968787670135498, rank=22, decoded_token="-"
                        ),
                        4: Logprob(
                            logprob=-1.453755497932434, rank=1, decoded_token="."
                        ),
                        6: Logprob(
                            logprob=-1.841694951057434, rank=2, decoded_token=","
                        ),
                    }
                ],
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719815937.6041,
            last_token_time=1719815937.6572065,
            first_scheduled_time=1719815937.604701,
            first_token_time=1719815937.6568153,
            time_in_queue=0.0006010532379150391,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=[
            None,
            {
                30086: Logprob(
                    logprob=-9.352765083312988, rank=369, decoded_token="Hi"
                ),
                100: Logprob(logprob=-1.4278708696365356, rank=1, decoded_token="I"),
                133: Logprob(logprob=-2.4365129470825195, rank=2, decoded_token="The"),
            },
            {
                6: Logprob(logprob=-1.4278249740600586, rank=1, decoded_token=","),
                328: Logprob(logprob=-1.934173583984375, rank=2, decoded_token="!"),
            },
            {
                38: Logprob(logprob=-0.976689338684082, rank=1, decoded_token=" I"),
                1437: Logprob(logprob=-2.723400115966797, rank=2, decoded_token=" "),
            },
            {
                657: Logprob(
                    logprob=-5.6148481369018555, rank=22, decoded_token=" love"
                ),
                437: Logprob(logprob=-1.015452265739441, rank=1, decoded_token="'m"),
                33: Logprob(logprob=-1.9374703168869019, rank=2, decoded_token=" have"),
            },
            {
                127: Logprob(logprob=-4.214991569519043, rank=9, decoded_token=" my"),
                110: Logprob(
                    logprob=-1.7619359493255615, rank=1, decoded_token=" your"
                ),
                5: Logprob(logprob=-1.999145269393921, rank=2, decoded_token=" the"),
            },
            {
                4758: Logprob(logprob=-4.99854040145874, rank=3, decoded_token=" cat"),
                92: Logprob(logprob=-3.4642574787139893, rank=1, decoded_token=" new"),
                793: Logprob(logprob=-4.73804235458374, rank=2, decoded_token=" old"),
            },
        ],
        outputs=[
            CompletionOutput(
                index=0,
                text="- Labrador",
                token_ids=[12, 26882],
                cumulative_logprob=-16.97801923751831,
                logprobs=[
                    {
                        12: Logprob(
                            logprob=-5.968787670135498, rank=22, decoded_token="-"
                        ),
                        4: Logprob(
                            logprob=-1.453755497932434, rank=1, decoded_token="."
                        ),
                        6: Logprob(
                            logprob=-1.841694951057434, rank=2, decoded_token=","
                        ),
                    },
                    {
                        26882: Logprob(
                            logprob=-11.009231567382812,
                            rank=3032,
                            decoded_token=" Labrador",
                        ),
                        38: Logprob(
                            logprob=-1.754422903060913, rank=1, decoded_token=" I"
                        ),
                        79: Logprob(
                            logprob=-3.075488328933716, rank=2, decoded_token=" she"
                        ),
                    },
                ],
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719815937.6041,
            last_token_time=1719815937.6911924,
            first_scheduled_time=1719815937.604701,
            first_token_time=1719815937.6568153,
            time_in_queue=0.0006010532379150391,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=[
            None,
            {
                30086: Logprob(
                    logprob=-9.352765083312988, rank=369, decoded_token="Hi"
                ),
                100: Logprob(logprob=-1.4278708696365356, rank=1, decoded_token="I"),
                133: Logprob(logprob=-2.4365129470825195, rank=2, decoded_token="The"),
            },
            {
                6: Logprob(logprob=-1.4278249740600586, rank=1, decoded_token=","),
                328: Logprob(logprob=-1.934173583984375, rank=2, decoded_token="!"),
            },
            {
                38: Logprob(logprob=-0.976689338684082, rank=1, decoded_token=" I"),
                1437: Logprob(logprob=-2.723400115966797, rank=2, decoded_token=" "),
            },
            {
                657: Logprob(
                    logprob=-5.6148481369018555, rank=22, decoded_token=" love"
                ),
                437: Logprob(logprob=-1.015452265739441, rank=1, decoded_token="'m"),
                33: Logprob(logprob=-1.9374703168869019, rank=2, decoded_token=" have"),
            },
            {
                127: Logprob(logprob=-4.214991569519043, rank=9, decoded_token=" my"),
                110: Logprob(
                    logprob=-1.7619359493255615, rank=1, decoded_token=" your"
                ),
                5: Logprob(logprob=-1.999145269393921, rank=2, decoded_token=" the"),
            },
            {
                4758: Logprob(logprob=-4.99854040145874, rank=3, decoded_token=" cat"),
                92: Logprob(logprob=-3.4642574787139893, rank=1, decoded_token=" new"),
                793: Logprob(logprob=-4.73804235458374, rank=2, decoded_token=" old"),
            },
        ],
        outputs=[
            CompletionOutput(
                index=0,
                text="- Labrador!",
                token_ids=[12, 26882, 328],
                cumulative_logprob=-20.131213426589966,
                logprobs=[
                    {
                        12: Logprob(
                            logprob=-5.968787670135498, rank=22, decoded_token="-"
                        ),
                        4: Logprob(
                            logprob=-1.453755497932434, rank=1, decoded_token="."
                        ),
                        6: Logprob(
                            logprob=-1.841694951057434, rank=2, decoded_token=","
                        ),
                    },
                    {
                        26882: Logprob(
                            logprob=-11.009231567382812,
                            rank=3032,
                            decoded_token=" Labrador",
                        ),
                        38: Logprob(
                            logprob=-1.754422903060913, rank=1, decoded_token=" I"
                        ),
                        79: Logprob(
                            logprob=-3.075488328933716, rank=2, decoded_token=" she"
                        ),
                    },
                    {
                        328: Logprob(
                            logprob=-3.1531941890716553, rank=5, decoded_token="!"
                        ),
                        3344: Logprob(
                            logprob=-1.0394361019134521, rank=1, decoded_token=" mix"
                        ),
                        4: Logprob(
                            logprob=-2.1872146129608154, rank=2, decoded_token="."
                        ),
                    },
                ],
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719815937.6041,
            last_token_time=1719815937.7240052,
            first_scheduled_time=1719815937.604701,
            first_token_time=1719815937.6568153,
            time_in_queue=0.0006010532379150391,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=[
            None,
            {
                30086: Logprob(
                    logprob=-9.352765083312988, rank=369, decoded_token="Hi"
                ),
                100: Logprob(logprob=-1.4278708696365356, rank=1, decoded_token="I"),
                133: Logprob(logprob=-2.4365129470825195, rank=2, decoded_token="The"),
            },
            {
                6: Logprob(logprob=-1.4278249740600586, rank=1, decoded_token=","),
                328: Logprob(logprob=-1.934173583984375, rank=2, decoded_token="!"),
            },
            {
                38: Logprob(logprob=-0.976689338684082, rank=1, decoded_token=" I"),
                1437: Logprob(logprob=-2.723400115966797, rank=2, decoded_token=" "),
            },
            {
                657: Logprob(
                    logprob=-5.6148481369018555, rank=22, decoded_token=" love"
                ),
                437: Logprob(logprob=-1.015452265739441, rank=1, decoded_token="'m"),
                33: Logprob(logprob=-1.9374703168869019, rank=2, decoded_token=" have"),
            },
            {
                127: Logprob(logprob=-4.214991569519043, rank=9, decoded_token=" my"),
                110: Logprob(
                    logprob=-1.7619359493255615, rank=1, decoded_token=" your"
                ),
                5: Logprob(logprob=-1.999145269393921, rank=2, decoded_token=" the"),
            },
            {
                4758: Logprob(logprob=-4.99854040145874, rank=3, decoded_token=" cat"),
                92: Logprob(logprob=-3.4642574787139893, rank=1, decoded_token=" new"),
                793: Logprob(logprob=-4.73804235458374, rank=2, decoded_token=" old"),
            },
        ],
        outputs=[
            CompletionOutput(
                index=0,
                text="- Labrador! He",
                token_ids=[12, 26882, 328, 91],
                cumulative_logprob=-21.547941207885742,
                logprobs=[
                    {
                        12: Logprob(
                            logprob=-5.968787670135498, rank=22, decoded_token="-"
                        ),
                        4: Logprob(
                            logprob=-1.453755497932434, rank=1, decoded_token="."
                        ),
                        6: Logprob(
                            logprob=-1.841694951057434, rank=2, decoded_token=","
                        ),
                    },
                    {
                        26882: Logprob(
                            logprob=-11.009231567382812,
                            rank=3032,
                            decoded_token=" Labrador",
                        ),
                        38: Logprob(
                            logprob=-1.754422903060913, rank=1, decoded_token=" I"
                        ),
                        79: Logprob(
                            logprob=-3.075488328933716, rank=2, decoded_token=" she"
                        ),
                    },
                    {
                        328: Logprob(
                            logprob=-3.1531941890716553, rank=5, decoded_token="!"
                        ),
                        3344: Logprob(
                            logprob=-1.0394361019134521, rank=1, decoded_token=" mix"
                        ),
                        4: Logprob(
                            logprob=-2.1872146129608154, rank=2, decoded_token="."
                        ),
                    },
                    {
                        91: Logprob(
                            logprob=-1.4167277812957764, rank=1, decoded_token=" He"
                        ),
                        50118: Logprob(
                            logprob=-2.0672662258148193, rank=2, decoded_token="\n"
                        ),
                    },
                ],
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719815937.6041,
            last_token_time=1719815937.7567687,
            first_scheduled_time=1719815937.604701,
            first_token_time=1719815937.6568153,
            time_in_queue=0.0006010532379150391,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=[
            None,
            {
                30086: Logprob(
                    logprob=-9.352765083312988, rank=369, decoded_token="Hi"
                ),
                100: Logprob(logprob=-1.4278708696365356, rank=1, decoded_token="I"),
                133: Logprob(logprob=-2.4365129470825195, rank=2, decoded_token="The"),
            },
            {
                6: Logprob(logprob=-1.4278249740600586, rank=1, decoded_token=","),
                328: Logprob(logprob=-1.934173583984375, rank=2, decoded_token="!"),
            },
            {
                38: Logprob(logprob=-0.976689338684082, rank=1, decoded_token=" I"),
                1437: Logprob(logprob=-2.723400115966797, rank=2, decoded_token=" "),
            },
            {
                657: Logprob(
                    logprob=-5.6148481369018555, rank=22, decoded_token=" love"
                ),
                437: Logprob(logprob=-1.015452265739441, rank=1, decoded_token="'m"),
                33: Logprob(logprob=-1.9374703168869019, rank=2, decoded_token=" have"),
            },
            {
                127: Logprob(logprob=-4.214991569519043, rank=9, decoded_token=" my"),
                110: Logprob(
                    logprob=-1.7619359493255615, rank=1, decoded_token=" your"
                ),
                5: Logprob(logprob=-1.999145269393921, rank=2, decoded_token=" the"),
            },
            {
                4758: Logprob(logprob=-4.99854040145874, rank=3, decoded_token=" cat"),
                92: Logprob(logprob=-3.4642574787139893, rank=1, decoded_token=" new"),
                793: Logprob(logprob=-4.73804235458374, rank=2, decoded_token=" old"),
            },
        ],
        outputs=[
            CompletionOutput(
                index=0,
                text="- Labrador! He has",
                token_ids=[12, 26882, 328, 91, 34],
                cumulative_logprob=-24.314465522766113,
                logprobs=[
                    {
                        12: Logprob(
                            logprob=-5.968787670135498, rank=22, decoded_token="-"
                        ),
                        4: Logprob(
                            logprob=-1.453755497932434, rank=1, decoded_token="."
                        ),
                        6: Logprob(
                            logprob=-1.841694951057434, rank=2, decoded_token=","
                        ),
                    },
                    {
                        26882: Logprob(
                            logprob=-11.009231567382812,
                            rank=3032,
                            decoded_token=" Labrador",
                        ),
                        38: Logprob(
                            logprob=-1.754422903060913, rank=1, decoded_token=" I"
                        ),
                        79: Logprob(
                            logprob=-3.075488328933716, rank=2, decoded_token=" she"
                        ),
                    },
                    {
                        328: Logprob(
                            logprob=-3.1531941890716553, rank=5, decoded_token="!"
                        ),
                        3344: Logprob(
                            logprob=-1.0394361019134521, rank=1, decoded_token=" mix"
                        ),
                        4: Logprob(
                            logprob=-2.1872146129608154, rank=2, decoded_token="."
                        ),
                    },
                    {
                        91: Logprob(
                            logprob=-1.4167277812957764, rank=1, decoded_token=" He"
                        ),
                        50118: Logprob(
                            logprob=-2.0672662258148193, rank=2, decoded_token="\n"
                        ),
                    },
                    {
                        34: Logprob(
                            logprob=-2.766524314880371, rank=3, decoded_token=" has"
                        ),
                        18: Logprob(
                            logprob=-1.0847474336624146, rank=1, decoded_token="'s"
                        ),
                        16: Logprob(
                            logprob=-1.547521710395813, rank=2, decoded_token=" is"
                        ),
                    },
                ],
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719815937.6041,
            last_token_time=1719815937.7901855,
            first_scheduled_time=1719815937.604701,
            first_token_time=1719815937.6568153,
            time_in_queue=0.0006010532379150391,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=[
            None,
            {
                30086: Logprob(
                    logprob=-9.352765083312988, rank=369, decoded_token="Hi"
                ),
                100: Logprob(logprob=-1.4278708696365356, rank=1, decoded_token="I"),
                133: Logprob(logprob=-2.4365129470825195, rank=2, decoded_token="The"),
            },
            {
                6: Logprob(logprob=-1.4278249740600586, rank=1, decoded_token=","),
                328: Logprob(logprob=-1.934173583984375, rank=2, decoded_token="!"),
            },
            {
                38: Logprob(logprob=-0.976689338684082, rank=1, decoded_token=" I"),
                1437: Logprob(logprob=-2.723400115966797, rank=2, decoded_token=" "),
            },
            {
                657: Logprob(
                    logprob=-5.6148481369018555, rank=22, decoded_token=" love"
                ),
                437: Logprob(logprob=-1.015452265739441, rank=1, decoded_token="'m"),
                33: Logprob(logprob=-1.9374703168869019, rank=2, decoded_token=" have"),
            },
            {
                127: Logprob(logprob=-4.214991569519043, rank=9, decoded_token=" my"),
                110: Logprob(
                    logprob=-1.7619359493255615, rank=1, decoded_token=" your"
                ),
                5: Logprob(logprob=-1.999145269393921, rank=2, decoded_token=" the"),
            },
            {
                4758: Logprob(logprob=-4.99854040145874, rank=3, decoded_token=" cat"),
                92: Logprob(logprob=-3.4642574787139893, rank=1, decoded_token=" new"),
                793: Logprob(logprob=-4.73804235458374, rank=2, decoded_token=" old"),
            },
        ],
        outputs=[
            CompletionOutput(
                index=0,
                text="- Labrador! He has tiny",
                token_ids=[12, 26882, 328, 91, 34, 5262],
                cumulative_logprob=-31.254112243652344,
                logprobs=[
                    {
                        12: Logprob(
                            logprob=-5.968787670135498, rank=22, decoded_token="-"
                        ),
                        4: Logprob(
                            logprob=-1.453755497932434, rank=1, decoded_token="."
                        ),
                        6: Logprob(
                            logprob=-1.841694951057434, rank=2, decoded_token=","
                        ),
                    },
                    {
                        26882: Logprob(
                            logprob=-11.009231567382812,
                            rank=3032,
                            decoded_token=" Labrador",
                        ),
                        38: Logprob(
                            logprob=-1.754422903060913, rank=1, decoded_token=" I"
                        ),
                        79: Logprob(
                            logprob=-3.075488328933716, rank=2, decoded_token=" she"
                        ),
                    },
                    {
                        328: Logprob(
                            logprob=-3.1531941890716553, rank=5, decoded_token="!"
                        ),
                        3344: Logprob(
                            logprob=-1.0394361019134521, rank=1, decoded_token=" mix"
                        ),
                        4: Logprob(
                            logprob=-2.1872146129608154, rank=2, decoded_token="."
                        ),
                    },
                    {
                        91: Logprob(
                            logprob=-1.4167277812957764, rank=1, decoded_token=" He"
                        ),
                        50118: Logprob(
                            logprob=-2.0672662258148193, rank=2, decoded_token="\n"
                        ),
                    },
                    {
                        34: Logprob(
                            logprob=-2.766524314880371, rank=3, decoded_token=" has"
                        ),
                        18: Logprob(
                            logprob=-1.0847474336624146, rank=1, decoded_token="'s"
                        ),
                        16: Logprob(
                            logprob=-1.547521710395813, rank=2, decoded_token=" is"
                        ),
                    },
                    {
                        5262: Logprob(
                            logprob=-6.9396467208862305, rank=90, decoded_token=" tiny"
                        ),
                        10: Logprob(
                            logprob=-1.3877270221710205, rank=1, decoded_token=" a"
                        ),
                        57: Logprob(
                            logprob=-2.3109371662139893, rank=2, decoded_token=" been"
                        ),
                    },
                ],
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719815937.6041,
            last_token_time=1719815937.8231184,
            first_scheduled_time=1719815937.604701,
            first_token_time=1719815937.6568153,
            time_in_queue=0.0006010532379150391,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=[
            None,
            {
                30086: Logprob(
                    logprob=-9.352765083312988, rank=369, decoded_token="Hi"
                ),
                100: Logprob(logprob=-1.4278708696365356, rank=1, decoded_token="I"),
                133: Logprob(logprob=-2.4365129470825195, rank=2, decoded_token="The"),
            },
            {
                6: Logprob(logprob=-1.4278249740600586, rank=1, decoded_token=","),
                328: Logprob(logprob=-1.934173583984375, rank=2, decoded_token="!"),
            },
            {
                38: Logprob(logprob=-0.976689338684082, rank=1, decoded_token=" I"),
                1437: Logprob(logprob=-2.723400115966797, rank=2, decoded_token=" "),
            },
            {
                657: Logprob(
                    logprob=-5.6148481369018555, rank=22, decoded_token=" love"
                ),
                437: Logprob(logprob=-1.015452265739441, rank=1, decoded_token="'m"),
                33: Logprob(logprob=-1.9374703168869019, rank=2, decoded_token=" have"),
            },
            {
                127: Logprob(logprob=-4.214991569519043, rank=9, decoded_token=" my"),
                110: Logprob(
                    logprob=-1.7619359493255615, rank=1, decoded_token=" your"
                ),
                5: Logprob(logprob=-1.999145269393921, rank=2, decoded_token=" the"),
            },
            {
                4758: Logprob(logprob=-4.99854040145874, rank=3, decoded_token=" cat"),
                92: Logprob(logprob=-3.4642574787139893, rank=1, decoded_token=" new"),
                793: Logprob(logprob=-4.73804235458374, rank=2, decoded_token=" old"),
            },
        ],
        outputs=[
            CompletionOutput(
                index=0,
                text="- Labrador! He has tiny ears",
                token_ids=[12, 26882, 328, 91, 34, 5262, 12137],
                cumulative_logprob=-32.61610543727875,
                logprobs=[
                    {
                        12: Logprob(
                            logprob=-5.968787670135498, rank=22, decoded_token="-"
                        ),
                        4: Logprob(
                            logprob=-1.453755497932434, rank=1, decoded_token="."
                        ),
                        6: Logprob(
                            logprob=-1.841694951057434, rank=2, decoded_token=","
                        ),
                    },
                    {
                        26882: Logprob(
                            logprob=-11.009231567382812,
                            rank=3032,
                            decoded_token=" Labrador",
                        ),
                        38: Logprob(
                            logprob=-1.754422903060913, rank=1, decoded_token=" I"
                        ),
                        79: Logprob(
                            logprob=-3.075488328933716, rank=2, decoded_token=" she"
                        ),
                    },
                    {
                        328: Logprob(
                            logprob=-3.1531941890716553, rank=5, decoded_token="!"
                        ),
                        3344: Logprob(
                            logprob=-1.0394361019134521, rank=1, decoded_token=" mix"
                        ),
                        4: Logprob(
                            logprob=-2.1872146129608154, rank=2, decoded_token="."
                        ),
                    },
                    {
                        91: Logprob(
                            logprob=-1.4167277812957764, rank=1, decoded_token=" He"
                        ),
                        50118: Logprob(
                            logprob=-2.0672662258148193, rank=2, decoded_token="\n"
                        ),
                    },
                    {
                        34: Logprob(
                            logprob=-2.766524314880371, rank=3, decoded_token=" has"
                        ),
                        18: Logprob(
                            logprob=-1.0847474336624146, rank=1, decoded_token="'s"
                        ),
                        16: Logprob(
                            logprob=-1.547521710395813, rank=2, decoded_token=" is"
                        ),
                    },
                    {
                        5262: Logprob(
                            logprob=-6.9396467208862305, rank=90, decoded_token=" tiny"
                        ),
                        10: Logprob(
                            logprob=-1.3877270221710205, rank=1, decoded_token=" a"
                        ),
                        57: Logprob(
                            logprob=-2.3109371662139893, rank=2, decoded_token=" been"
                        ),
                    },
                    {
                        12137: Logprob(
                            logprob=-1.3619931936264038, rank=1, decoded_token=" ears"
                        ),
                        40844: Logprob(
                            logprob=-2.2743258476257324, rank=2, decoded_token=" paws"
                        ),
                    },
                ],
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719815937.6041,
            last_token_time=1719815937.8558471,
            first_scheduled_time=1719815937.604701,
            first_token_time=1719815937.6568153,
            time_in_queue=0.0006010532379150391,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=[
            None,
            {
                30086: Logprob(
                    logprob=-9.352765083312988, rank=369, decoded_token="Hi"
                ),
                100: Logprob(logprob=-1.4278708696365356, rank=1, decoded_token="I"),
                133: Logprob(logprob=-2.4365129470825195, rank=2, decoded_token="The"),
            },
            {
                6: Logprob(logprob=-1.4278249740600586, rank=1, decoded_token=","),
                328: Logprob(logprob=-1.934173583984375, rank=2, decoded_token="!"),
            },
            {
                38: Logprob(logprob=-0.976689338684082, rank=1, decoded_token=" I"),
                1437: Logprob(logprob=-2.723400115966797, rank=2, decoded_token=" "),
            },
            {
                657: Logprob(
                    logprob=-5.6148481369018555, rank=22, decoded_token=" love"
                ),
                437: Logprob(logprob=-1.015452265739441, rank=1, decoded_token="'m"),
                33: Logprob(logprob=-1.9374703168869019, rank=2, decoded_token=" have"),
            },
            {
                127: Logprob(logprob=-4.214991569519043, rank=9, decoded_token=" my"),
                110: Logprob(
                    logprob=-1.7619359493255615, rank=1, decoded_token=" your"
                ),
                5: Logprob(logprob=-1.999145269393921, rank=2, decoded_token=" the"),
            },
            {
                4758: Logprob(logprob=-4.99854040145874, rank=3, decoded_token=" cat"),
                92: Logprob(logprob=-3.4642574787139893, rank=1, decoded_token=" new"),
                793: Logprob(logprob=-4.73804235458374, rank=2, decoded_token=" old"),
            },
        ],
        outputs=[
            CompletionOutput(
                index=0,
                text="- Labrador! He has tiny ears with",
                token_ids=[12, 26882, 328, 91, 34, 5262, 12137, 19],
                cumulative_logprob=-37.2360657453537,
                logprobs=[
                    {
                        12: Logprob(
                            logprob=-5.968787670135498, rank=22, decoded_token="-"
                        ),
                        4: Logprob(
                            logprob=-1.453755497932434, rank=1, decoded_token="."
                        ),
                        6: Logprob(
                            logprob=-1.841694951057434, rank=2, decoded_token=","
                        ),
                    },
                    {
                        26882: Logprob(
                            logprob=-11.009231567382812,
                            rank=3032,
                            decoded_token=" Labrador",
                        ),
                        38: Logprob(
                            logprob=-1.754422903060913, rank=1, decoded_token=" I"
                        ),
                        79: Logprob(
                            logprob=-3.075488328933716, rank=2, decoded_token=" she"
                        ),
                    },
                    {
                        328: Logprob(
                            logprob=-3.1531941890716553, rank=5, decoded_token="!"
                        ),
                        3344: Logprob(
                            logprob=-1.0394361019134521, rank=1, decoded_token=" mix"
                        ),
                        4: Logprob(
                            logprob=-2.1872146129608154, rank=2, decoded_token="."
                        ),
                    },
                    {
                        91: Logprob(
                            logprob=-1.4167277812957764, rank=1, decoded_token=" He"
                        ),
                        50118: Logprob(
                            logprob=-2.0672662258148193, rank=2, decoded_token="\n"
                        ),
                    },
                    {
                        34: Logprob(
                            logprob=-2.766524314880371, rank=3, decoded_token=" has"
                        ),
                        18: Logprob(
                            logprob=-1.0847474336624146, rank=1, decoded_token="'s"
                        ),
                        16: Logprob(
                            logprob=-1.547521710395813, rank=2, decoded_token=" is"
                        ),
                    },
                    {
                        5262: Logprob(
                            logprob=-6.9396467208862305, rank=90, decoded_token=" tiny"
                        ),
                        10: Logprob(
                            logprob=-1.3877270221710205, rank=1, decoded_token=" a"
                        ),
                        57: Logprob(
                            logprob=-2.3109371662139893, rank=2, decoded_token=" been"
                        ),
                    },
                    {
                        12137: Logprob(
                            logprob=-1.3619931936264038, rank=1, decoded_token=" ears"
                        ),
                        40844: Logprob(
                            logprob=-2.2743258476257324, rank=2, decoded_token=" paws"
                        ),
                    },
                    {
                        19: Logprob(
                            logprob=-4.619960308074951, rank=10, decoded_token=" with"
                        ),
                        8: Logprob(
                            logprob=-0.805719792842865, rank=1, decoded_token=" and"
                        ),
                        6: Logprob(
                            logprob=-1.6155686378479004, rank=2, decoded_token=","
                        ),
                    },
                ],
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719815937.6041,
            last_token_time=1719815937.8884614,
            first_scheduled_time=1719815937.604701,
            first_token_time=1719815937.6568153,
            time_in_queue=0.0006010532379150391,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=[
            None,
            {
                30086: Logprob(
                    logprob=-9.352765083312988, rank=369, decoded_token="Hi"
                ),
                100: Logprob(logprob=-1.4278708696365356, rank=1, decoded_token="I"),
                133: Logprob(logprob=-2.4365129470825195, rank=2, decoded_token="The"),
            },
            {
                6: Logprob(logprob=-1.4278249740600586, rank=1, decoded_token=","),
                328: Logprob(logprob=-1.934173583984375, rank=2, decoded_token="!"),
            },
            {
                38: Logprob(logprob=-0.976689338684082, rank=1, decoded_token=" I"),
                1437: Logprob(logprob=-2.723400115966797, rank=2, decoded_token=" "),
            },
            {
                657: Logprob(
                    logprob=-5.6148481369018555, rank=22, decoded_token=" love"
                ),
                437: Logprob(logprob=-1.015452265739441, rank=1, decoded_token="'m"),
                33: Logprob(logprob=-1.9374703168869019, rank=2, decoded_token=" have"),
            },
            {
                127: Logprob(logprob=-4.214991569519043, rank=9, decoded_token=" my"),
                110: Logprob(
                    logprob=-1.7619359493255615, rank=1, decoded_token=" your"
                ),
                5: Logprob(logprob=-1.999145269393921, rank=2, decoded_token=" the"),
            },
            {
                4758: Logprob(logprob=-4.99854040145874, rank=3, decoded_token=" cat"),
                92: Logprob(logprob=-3.4642574787139893, rank=1, decoded_token=" new"),
                793: Logprob(logprob=-4.73804235458374, rank=2, decoded_token=" old"),
            },
        ],
        outputs=[
            CompletionOutput(
                index=0,
                text="- Labrador! He has tiny ears with fluffy",
                token_ids=[12, 26882, 328, 91, 34, 5262, 12137, 19, 33564],
                cumulative_logprob=-42.8609436750412,
                logprobs=[
                    {
                        12: Logprob(
                            logprob=-5.968787670135498, rank=22, decoded_token="-"
                        ),
                        4: Logprob(
                            logprob=-1.453755497932434, rank=1, decoded_token="."
                        ),
                        6: Logprob(
                            logprob=-1.841694951057434, rank=2, decoded_token=","
                        ),
                    },
                    {
                        26882: Logprob(
                            logprob=-11.009231567382812,
                            rank=3032,
                            decoded_token=" Labrador",
                        ),
                        38: Logprob(
                            logprob=-1.754422903060913, rank=1, decoded_token=" I"
                        ),
                        79: Logprob(
                            logprob=-3.075488328933716, rank=2, decoded_token=" she"
                        ),
                    },
                    {
                        328: Logprob(
                            logprob=-3.1531941890716553, rank=5, decoded_token="!"
                        ),
                        3344: Logprob(
                            logprob=-1.0394361019134521, rank=1, decoded_token=" mix"
                        ),
                        4: Logprob(
                            logprob=-2.1872146129608154, rank=2, decoded_token="."
                        ),
                    },
                    {
                        91: Logprob(
                            logprob=-1.4167277812957764, rank=1, decoded_token=" He"
                        ),
                        50118: Logprob(
                            logprob=-2.0672662258148193, rank=2, decoded_token="\n"
                        ),
                    },
                    {
                        34: Logprob(
                            logprob=-2.766524314880371, rank=3, decoded_token=" has"
                        ),
                        18: Logprob(
                            logprob=-1.0847474336624146, rank=1, decoded_token="'s"
                        ),
                        16: Logprob(
                            logprob=-1.547521710395813, rank=2, decoded_token=" is"
                        ),
                    },
                    {
                        5262: Logprob(
                            logprob=-6.9396467208862305, rank=90, decoded_token=" tiny"
                        ),
                        10: Logprob(
                            logprob=-1.3877270221710205, rank=1, decoded_token=" a"
                        ),
                        57: Logprob(
                            logprob=-2.3109371662139893, rank=2, decoded_token=" been"
                        ),
                    },
                    {
                        12137: Logprob(
                            logprob=-1.3619931936264038, rank=1, decoded_token=" ears"
                        ),
                        40844: Logprob(
                            logprob=-2.2743258476257324, rank=2, decoded_token=" paws"
                        ),
                    },
                    {
                        19: Logprob(
                            logprob=-4.619960308074951, rank=10, decoded_token=" with"
                        ),
                        8: Logprob(
                            logprob=-0.805719792842865, rank=1, decoded_token=" and"
                        ),
                        6: Logprob(
                            logprob=-1.6155686378479004, rank=2, decoded_token=","
                        ),
                    },
                    {
                        33564: Logprob(
                            logprob=-5.6248779296875, rank=38, decoded_token=" fluffy"
                        ),
                        10: Logprob(
                            logprob=-1.4977400302886963, rank=1, decoded_token=" a"
                        ),
                        5262: Logprob(
                            logprob=-3.006150484085083, rank=2, decoded_token=" tiny"
                        ),
                    },
                ],
                finish_reason=None,
                stop_reason=None,
            )
        ],
        finished=False,
        metrics=RequestMetrics(
            arrival_time=1719815937.6041,
            last_token_time=1719815937.9210107,
            first_scheduled_time=1719815937.604701,
            first_token_time=1719815937.6568153,
            time_in_queue=0.0006010532379150391,
            finished_time=None,
        ),
        lora_request=None,
    ),
    RequestOutput(
        request_id="cmpl-d771287a234c44498e345f0a429d6691-0",
        prompt="Hi, I love my cat",
        prompt_token_ids=[2, 30086, 6, 38, 657, 127, 4758],
        prompt_logprobs=[
            None,
            {
                30086: Logprob(
                    logprob=-9.352765083312988, rank=369, decoded_token="Hi"
                ),
                100: Logprob(logprob=-1.4278708696365356, rank=1, decoded_token="I"),
                133: Logprob(logprob=-2.4365129470825195, rank=2, decoded_token="The"),
            },
            {
                6: Logprob(logprob=-1.4278249740600586, rank=1, decoded_token=","),
                328: Logprob(logprob=-1.934173583984375, rank=2, decoded_token="!"),
            },
            {
                38: Logprob(logprob=-0.976689338684082, rank=1, decoded_token=" I"),
                1437: Logprob(logprob=-2.723400115966797, rank=2, decoded_token=" "),
            },
            {
                657: Logprob(
                    logprob=-5.6148481369018555, rank=22, decoded_token=" love"
                ),
                437: Logprob(logprob=-1.015452265739441, rank=1, decoded_token="'m"),
                33: Logprob(logprob=-1.9374703168869019, rank=2, decoded_token=" have"),
            },
            {
                127: Logprob(logprob=-4.214991569519043, rank=9, decoded_token=" my"),
                110: Logprob(
                    logprob=-1.7619359493255615, rank=1, decoded_token=" your"
                ),
                5: Logprob(logprob=-1.999145269393921, rank=2, decoded_token=" the"),
            },
            {
                4758: Logprob(logprob=-4.99854040145874, rank=3, decoded_token=" cat"),
                92: Logprob(logprob=-3.4642574787139893, rank=1, decoded_token=" new"),
                793: Logprob(logprob=-4.73804235458374, rank=2, decoded_token=" old"),
            },
        ],
        outputs=[
            CompletionOutput(
                index=0,
                text="- Labrador! He has tiny ears with fluffy white",
                token_ids=[12, 26882, 328, 91, 34, 5262, 12137, 19, 33564, 1104],
                cumulative_logprob=-45.07622039318085,
                logprobs=[
                    {
                        12: Logprob(
                            logprob=-5.968787670135498, rank=22, decoded_token="-"
                        ),
                        4: Logprob(
                            logprob=-1.453755497932434, rank=1, decoded_token="."
                        ),
                        6: Logprob(
                            logprob=-1.841694951057434, rank=2, decoded_token=","
                        ),
                    },
                    {
                        26882: Logprob(
                            logprob=-11.009231567382812,
                            rank=3032,
                            decoded_token=" Labrador",
                        ),
                        38: Logprob(
                            logprob=-1.754422903060913, rank=1, decoded_token=" I"
                        ),
                        79: Logprob(
                            logprob=-3.075488328933716, rank=2, decoded_token=" she"
                        ),
                    },
                    {
                        328: Logprob(
                            logprob=-3.1531941890716553, rank=5, decoded_token="!"
                        ),
                        3344: Logprob(
                            logprob=-1.0394361019134521, rank=1, decoded_token=" mix"
                        ),
                        4: Logprob(
                            logprob=-2.1872146129608154, rank=2, decoded_token="."
                        ),
                    },
                    {
                        91: Logprob(
                            logprob=-1.4167277812957764, rank=1, decoded_token=" He"
                        ),
                        50118: Logprob(
                            logprob=-2.0672662258148193, rank=2, decoded_token="\n"
                        ),
                    },
                    {
                        34: Logprob(
                            logprob=-2.766524314880371, rank=3, decoded_token=" has"
                        ),
                        18: Logprob(
                            logprob=-1.0847474336624146, rank=1, decoded_token="'s"
                        ),
                        16: Logprob(
                            logprob=-1.547521710395813, rank=2, decoded_token=" is"
                        ),
                    },
                    {
                        5262: Logprob(
                            logprob=-6.9396467208862305, rank=90, decoded_token=" tiny"
                        ),
                        10: Logprob(
                            logprob=-1.3877270221710205, rank=1, decoded_token=" a"
                        ),
                        57: Logprob(
                            logprob=-2.3109371662139893, rank=2, decoded_token=" been"
                        ),
                    },
                    {
                        12137: Logprob(
                            logprob=-1.3619931936264038, rank=1, decoded_token=" ears"
                        ),
                        40844: Logprob(
                            logprob=-2.2743258476257324, rank=2, decoded_token=" paws"
                        ),
                    },
                    {
                        19: Logprob(
                            logprob=-4.619960308074951, rank=10, decoded_token=" with"
                        ),
                        8: Logprob(
                            logprob=-0.805719792842865, rank=1, decoded_token=" and"
                        ),
                        6: Logprob(
                            logprob=-1.6155686378479004, rank=2, decoded_token=","
                        ),
                    },
                    {
                        33564: Logprob(
                            logprob=-5.6248779296875, rank=38, decoded_token=" fluffy"
                        ),
                        10: Logprob(
                            logprob=-1.4977400302886963, rank=1, decoded_token=" a"
                        ),
                        5262: Logprob(
                            logprob=-3.006150484085083, rank=2, decoded_token=" tiny"
                        ),
                    },
                    {
                        1104: Logprob(
                            logprob=-2.2152767181396484, rank=2, decoded_token=" white"
                        ),
                        15503: Logprob(
                            logprob=-1.9012728929519653, rank=1, decoded_token=" fur"
                        ),
                    },
                ],
                finish_reason="length",
                stop_reason=None,
            )
        ],
        finished=True,
        metrics=RequestMetrics(
            arrival_time=1719815937.6041,
            last_token_time=1719815937.9535863,
            first_scheduled_time=1719815937.604701,
            first_token_time=1719815937.6568153,
            time_in_queue=0.0006010532379150391,
            finished_time=1719815937.9535837,
        ),
        lora_request=None,
    ),
]

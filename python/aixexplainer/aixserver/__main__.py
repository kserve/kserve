# Copyright 2021 The KServe Authors.
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

import sys
import kserve
from aixserver.parse import parseArgs
from aixserver.explainer import AIXModel, ExplainerMethod
import asyncio

if __name__ == "__main__":
    args, extra = parseArgs(sys.argv[1:])
    model = AIXModel(
        name=args.model_name,
        predictor_host=args.predictor_host,
        method=ExplainerMethod(args.command),
        config=extra
    )
    model.load()
    asyncio.run(kserve.ModelServer().start([model]))

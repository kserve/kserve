/* Copyright 2016 The TensorFlow Authors. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
vcyou may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
==============================================================================*/

#ifndef TENSORFLOW_CORE_KERNELS_I_REMOTE_FUSED_GRAPH_OPS_DEFINITIONS_H_
#define TENSORFLOW_CORE_KERNELS_I_REMOTE_FUSED_GRAPH_OPS_DEFINITIONS_H_

#include "tensorflow/core/framework/types.h"
#include "tensorflow/core/platform/macros.h"

namespace tensorflow {

// IRemoteFusedGraphOpsDefinitions is an interface class which provides
// APIs to provide information about op types supported by SOC.
// TODO(satok): Provide ways to transfer graph definitions into SOC
class IRemoteFusedGraphOpsDefinitions {
 public:
  // op id which is not supported by SOC
  static constexpr int INVALID_OP_ID = -1;

  IRemoteFusedGraphOpsDefinitions() = default;
  virtual ~IRemoteFusedGraphOpsDefinitions() = default;
  // Return total ops count supported by SOC
  virtual int GetTotalOpsCount() const = 0;
  // Return op id for given string op name
  virtual int GetOpIdFor(const string& op_type,
                         const DataTypeVector& dt) const = 0;

 private:
  TF_DISALLOW_COPY_AND_ASSIGN(IRemoteFusedGraphOpsDefinitions);
};

}  // namespace tensorflow

#endif  // TENSORFLOW_CORE_KERNELS_I_REMOTE_FUSED_GRAPH_OPS_DEFINITIONS_H_

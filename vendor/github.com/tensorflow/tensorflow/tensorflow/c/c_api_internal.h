/* Copyright 2015 The TensorFlow Authors. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
==============================================================================*/

#ifndef TENSORFLOW_C_C_API_INTERNAL_H_
#define TENSORFLOW_C_C_API_INTERNAL_H_

#include "tensorflow/c/c_api.h"

#include <list>
#include <set>
#include <string>
#include <unordered_map>
#include <vector>

#ifndef __ANDROID__
#include "tensorflow/core/distributed_runtime/server_lib.h"
#include "tensorflow/core/framework/op_gen_lib.h"
#endif
#include "tensorflow/core/common_runtime/shape_refiner.h"
#include "tensorflow/core/framework/tensor.h"
#include "tensorflow/core/framework/tensor_shape.h"
#include "tensorflow/core/graph/graph.h"
#include "tensorflow/core/graph/graph_constructor.h"
#include "tensorflow/core/graph/node_builder.h"
#include "tensorflow/core/lib/core/status.h"
#include "tensorflow/core/platform/mutex.h"
#include "tensorflow/core/platform/types.h"
#include "tensorflow/core/public/session.h"

namespace tensorflow {
class Device;
class DeviceMgr;
}  // namespace tensorflow

// Internal structures used by the C API. These are likely to change and should
// not be depended on.

struct TF_Status {
  tensorflow::Status status;
};

struct TF_Tensor {
  ~TF_Tensor();

  TF_DataType dtype;
  tensorflow::TensorShape shape;
  tensorflow::TensorBuffer* buffer;
};

struct TF_SessionOptions {
  tensorflow::SessionOptions options;
};

struct TF_DeprecatedSession {
  tensorflow::Session* session;
};

struct TF_Library {
  void* lib_handle;
  TF_Buffer op_list;
};

struct TF_Graph {
  TF_Graph();

  tensorflow::mutex mu;
  tensorflow::Graph graph GUARDED_BY(mu);

  // Runs shape inference.
  tensorflow::ShapeRefiner refiner GUARDED_BY(mu);

  // Maps from name of an operation to the Node* in 'graph'.
  std::unordered_map<tensorflow::string, tensorflow::Node*> name_map
      GUARDED_BY(mu);

  // The keys of this map are all the active sessions using this graph. Each
  // value records whether the graph has been mutated since the corresponding
  // session has been run (this is detected in RecordMutation function). If the
  // string is empty, no mutation has occurred. Otherwise the string is a
  // description of the mutation suitable for returning to the user.
  //
  // Sessions are added to this map in TF_NewSession, and removed in
  // TF_DeleteSession.
  // TF_Graph may only / must be deleted when
  //   sessions.size() == 0 && delete_requested == true
  //
  // TODO(b/74949947): mutations currently trigger a warning instead of a bad
  // status, this should be reverted when possible.
  tensorflow::gtl::FlatMap<TF_Session*, tensorflow::string> sessions
      GUARDED_BY(mu);
  bool delete_requested GUARDED_BY(mu);  // set true by TF_DeleteGraph

  // Used to link graphs contained in TF_WhileParams to the parent graph that
  // will eventually contain the full while loop.
  TF_Graph* parent;
  TF_Output* parent_inputs;
};

struct TF_OperationDescription {
  TF_OperationDescription(TF_Graph* g, const char* op_type,
                          const char* node_name)
      : node_builder(node_name, op_type, g->graph.op_registry()), graph(g) {}

  tensorflow::NodeBuilder node_builder;
  TF_Graph* graph;
  std::set<tensorflow::string> colocation_constraints;
};

struct TF_Operation {
  tensorflow::Node node;
};

struct TF_Session {
  TF_Session(tensorflow::Session* s, TF_Graph* g);

  tensorflow::Session* session;
  TF_Graph* const graph;

  tensorflow::mutex mu ACQUIRED_AFTER(TF_Graph::mu);
  int last_num_graph_nodes;

  // If true, TF_SessionRun and similar methods will call
  // ExtendSessionGraphHelper before running the graph (this is the default
  // public behavior). Can be set to false if the caller needs to call
  // ExtendSessionGraphHelper manually.
  std::atomic<bool> extend_before_run;
};

struct TF_ImportGraphDefOptions {
  tensorflow::ImportGraphDefOptions opts;

  // Backing memory for TensorId fields in opts.
  // TODO(skyewm): it'd be better if ImportGraphDefOptions owned this.
  std::list<tensorflow::string> tensor_id_data;
};

struct TF_ImportGraphDefResults {
  std::vector<TF_Output> return_tensors;
  std::vector<TF_Operation*> return_nodes;
  std::vector<const char*> missing_unused_key_names;
  std::vector<int> missing_unused_key_indexes;

  // Backing memory for missing_unused_key_names values.
  std::list<tensorflow::string> missing_unused_key_names_data;
};

struct TF_DeviceList {
  std::vector<tensorflow::DeviceAttributes> response;
};

struct TF_Function {
  tensorflow::FunctionDef fdef;
};

struct TF_ApiDefMap {
  explicit TF_ApiDefMap(const tensorflow::OpList& op_list)
      :
#ifndef __ANDROID__
        api_def_map(op_list),
#endif
        update_docs_called(false) {
  }

#ifndef __ANDROID__
  tensorflow::ApiDefMap api_def_map GUARDED_BY(lock);
#endif
  bool update_docs_called GUARDED_BY(lock);
  tensorflow::mutex lock;
};

#ifndef __ANDROID__
struct TF_Server {
  TF_Server(std::unique_ptr<tensorflow::ServerInterface> server);

  const tensorflow::string target;
  std::unique_ptr<tensorflow::ServerInterface> server;
};
#endif

namespace tensorflow {

class TensorCApi {
 public:
  static TensorBuffer* Buffer(const Tensor& tensor) { return tensor.buf_; }
  static Tensor MakeTensor(TF_DataType type, const TensorShape& shape,
                           TensorBuffer* buf) {
    return Tensor(static_cast<DataType>(type), shape, buf);
  }
};

Status TF_TensorToTensor(const TF_Tensor* src, Tensor* dst);

TF_Tensor* TF_TensorFromTensor(const Tensor& src, TF_Status* status);

Status MessageToBuffer(const tensorflow::protobuf::Message& in, TF_Buffer* out);

// Set the shapes and types of the output's handle.
//
// The lengths of the arrays pointed to by `shapes`, `ranks`, and `types` must
// all be equal to `num_shapes_and_types`. If `ranks[i] != -1`, (i.e., if the
// rank is known), then it must be equal to the length of `shapes[i]`; if
// `ranks[i] == 1`, then `shapes[i]` may be nullptr.
//
// TODO(akshayka): Implement a corresponding getter method.
void TF_GraphSetOutputHandleShapesAndTypes(TF_Graph* graph, TF_Output output,
                                           int num_shapes_and_types,
                                           const int64_t** shapes,
                                           const int* ranks,
                                           const TF_DataType* types,
                                           TF_Status* status);

void RecordMutation(TF_Graph* graph, const TF_Operation& op,
                    const char* mutation_type)
    EXCLUSIVE_LOCKS_REQUIRED(graph->mu);

bool ExtendSessionGraphHelper(TF_Session* session, TF_Status* status)
    LOCKS_EXCLUDED(session->graph->mu, session->mu);

}  // end namespace tensorflow

#endif  // TENSORFLOW_C_C_API_INTERNAL_H_

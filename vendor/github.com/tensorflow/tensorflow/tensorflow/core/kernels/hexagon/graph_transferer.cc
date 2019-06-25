/* Copyright 2016 The TensorFlow Authors. All Rights Reserved.

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

#include "tensorflow/core/kernels/hexagon/graph_transferer.h"

#include <algorithm>
#include <cinttypes>

#include "tensorflow/core/framework/graph.pb.h"
#include "tensorflow/core/framework/graph_transfer_info.pb.h"
#include "tensorflow/core/framework/op.h"
#include "tensorflow/core/graph/algorithm.h"
#include "tensorflow/core/graph/graph_constructor.h"
#include "tensorflow/core/graph/node_builder.h"
#include "tensorflow/core/platform/env.h"
#include "tensorflow/core/platform/types.h"
#include "tensorflow/core/public/session.h"
#include "tensorflow/core/public/session_options.h"
#include "tensorflow/core/util/tensor_slice_writer.h"

namespace tensorflow {

// function alias
constexpr auto AddOutputTensorShapeTypeByTensorShapeMap =
    &RemoteFusedGraphExecuteUtils::AddOutputTensorShapeTypeByTensorShapeMap;

constexpr bool DBG_DUMP_VERIFICATION_STRING = false;
constexpr bool DBG_DUMP_PARAMS = false;

const char RESHAPE_NODE_TYPE_STRING[] = "Reshape";
const char SOURCE_NODE_NAME[] = "_SOURCE";
const char SINK_NODE_NAME[] = "_SINK";
const char INPUTS_NODE_PREFIX[] = "inputs_for_";
const char OUTPUTS_NODE_PREFIX[] = "outputs_for_";
const char DATA_NODE_PREFIX[] = "data_for_op_";
const char CONST_SHAPE_PREFIX[] = "const_shape_";
const char CONST_VAL_PREFIX[] = "const_val_";
const char CONST_TENSOR_PREFIX[] = "const_tensor_";
const char PADDING_ATTR_NAME[] = "padding";
const char STRIDES_ATTR_NAME[] = "strides";
const char KEEP_DIMS_ATTR_NAME[] = "keep_dims";
const char KSIZE_ATTR_NAME[] = "ksize";
const char NULL_OUTPUT_NAME[] = "NULL";
const char AGGREGATED_INPUT_NODE_NAME[] = "graph_transfer_aggregated_input";
const int PADDING_NA_ID = 0;  // VALID = 1, SAME = 2

// This is a temporary workaround to support android build
// where std::string is not supported even with c++11 option.
template <typename T>
static string ToString(T val) {
  std::stringstream stream;
  stream << val;
  return stream.str();
}

static Node* FindMutableNodeByName(const string& name, Graph* graph) {
  const TensorId tid = ParseTensorName(name);
  for (Node* node : graph->nodes()) {
    if (node != nullptr && node->name() == tid.first) {
      return node;
    }
  }
  return nullptr;
}

GraphTransferer::GraphTransferer() {
  graph_transfer_info_ = new GraphTransferInfo();
}

GraphTransferer::~GraphTransferer() { delete graph_transfer_info_; }

/**
 * graph loading functions
 * - LoadGraphFromProto
 * - LoadGraphFromProptoFile
 * These functions read a graph definition and store parameters
 * of node to transfer the graph to SOC.
 */
Status GraphTransferer::LoadGraphFromProto(
    const IRemoteFusedGraphOpsDefinitions& ops_definitions,
    const GraphDef& graph_def,
    const std::vector<std::pair<string, Tensor>>& input_node_info_list,
    const std::vector<string>& output_node_names,
    const bool shape_inference_for_unknown_shape) {
  Graph graph(OpRegistry::Global());
  ShapeRefiner shape_refiner(graph.versions(), graph.op_registry());
  Status status = ImportGraphDef({}, graph_def, &graph, &shape_refiner);
  if (!status.ok()) {
    return status;
  }

  if (shape_inference_for_unknown_shape) {
    status = RemoteFusedGraphExecuteUtils::PropagateShapeInference(
        graph_def, input_node_info_list, &graph, &shape_refiner);
    if (!status.ok()) {
      return status;
    }
  }

  TF_RETURN_IF_ERROR(TransformGraphToAddAggregatedInputNode(
      input_node_info_list, &graph, &shape_refiner));

  std::unordered_multimap<string, const Node*> op_name_to_node_multimap(
      graph.num_nodes());
  for (const Node* const node : graph.nodes()) {
    if (node == nullptr) {
      continue;
    }
    CacheNode(*node);
  }

  for (const Node* const node : graph.nodes()) {
    if (node == nullptr) {
      continue;
    }
    VLOG(1) << "<Node> " << node->name();
    for (const Node* const input_node : node->in_nodes()) {
      const string& name = input_node->name();
      op_name_to_node_multimap.emplace(name, node);
      VLOG(1) << "Add dependency: " << name << " -> " << node->name();
    }
  }

  for (const Node* const node : graph.nodes()) {
    if (node == nullptr) {
      continue;
    }
    status = RegisterNodeIfAllInputsAreCached(
        ops_definitions, shape_refiner, *node, false, input_node_info_list,
        output_node_names);
    if (!status.ok()) {
      LOG(ERROR) << "Failed to transfer graph " << status;
      return status;
    }
  }

  SortParams(output_node_names);

  for (const std::pair<string, Tensor>& input_node_info :
       input_node_info_list) {
    GraphTransferGraphInputNodeInfo& graph_input_node_info =
        *graph_transfer_info_->add_graph_input_node_info();
    graph_input_node_info.set_name(input_node_info.first);
    graph_input_node_info.set_dtype(input_node_info.second.dtype());
    for (const int64 dim : ToTensorShapeArray(input_node_info.second.shape())) {
      graph_input_node_info.add_shape(dim);
    }
  }

  for (const string& output_node_name : output_node_names) {
    const TensorId tid = ParseTensorName(output_node_name);
    const string node_name(tid.first);
    const int port = tid.second;
    const int node_id = node_name_to_id_cache_map_.at(node_name);
    const Node* node = node_name_cache_list_.at(node_id);
    CHECK_NOTNULL(node);

    GraphTransferGraphOutputNodeInfo& graph_output_node_info =
        *graph_transfer_info_->add_graph_output_node_info();
    graph_output_node_info.set_name(strings::StrCat(node_name, ":", port));

    // Get output tensor shape type
    std::vector<DataType> data_types;
    std::vector<TensorShape> shapes;
    status = RemoteFusedGraphExecuteUtils::GetOutputTensorShapeType(
        node->attrs(), &data_types, &shapes);
    if (status.ok()) {
      CHECK(data_types.size() > port);
      graph_output_node_info.set_dtype(data_types.at(port));
      for (const int64 dim : ToTensorShapeArray(shapes.at(port))) {
        graph_output_node_info.add_shape(dim);
      }
    }
  }

  ClearCache();
  if (DBG_DUMP_PARAMS) {
    DumpNodeTransferParams();
  }
  if (DBG_DUMP_VERIFICATION_STRING) {
    DumpVerificationStringOfNodeTransferParams();
  }
  return Status();
}

Status GraphTransferer::LoadGraphFromProtoFile(
    const IRemoteFusedGraphOpsDefinitions& ops_definitions,
    const string& graph_def_path,
    const std::vector<std::pair<string, Tensor>>& input_node_info_list,
    const std::vector<string>& output_node_names, const bool is_text_proto,
    const bool shape_inference_for_unknown_shape,
    const bool dry_run_for_unknown_shape) {
  GraphDef graph_def;
  string output;
  Status status;
  VLOG(1) << "Parse file " << graph_def_path;
  if (is_text_proto) {
    status = ReadFileToString(Env::Default(), graph_def_path, &output);
    if (!protobuf::TextFormat::ParseFromString(output, &graph_def)) {
      return errors::InvalidArgument("Cannot parse proto string.");
    }
  } else {
    status = ReadBinaryProto(Env::Default(), graph_def_path, &graph_def);
  }
  if (!status.ok()) {
    VLOG(1) << "Failed to load graph " << status;
    return status;
  }
  if (dry_run_for_unknown_shape) {
    VLOG(1) << "Dry run graph to obtain shape of nodes";
    RemoteFusedGraphExecuteUtils::TensorShapeMap tensor_shape_map;
    status = RemoteFusedGraphExecuteUtils::DryRunInferenceForAllNode(
        graph_def, input_node_info_list, true, &tensor_shape_map);
    if (!status.ok()) {
      return status;
    }
    for (NodeDef& node_def : *graph_def.mutable_node()) {
      TF_CHECK_OK(AddOutputTensorShapeTypeByTensorShapeMap(tensor_shape_map,
                                                           &node_def));
    }
  }
  VLOG(1) << "Load graph with output tensors";
  return LoadGraphFromProto(ops_definitions, graph_def, input_node_info_list,
                            output_node_names,
                            shape_inference_for_unknown_shape);
}

void GraphTransferer::SortParams(const std::vector<string>& output_node_names) {
  // TODO(satok): optimize complexity
  std::unordered_map<int, GraphTransferNodeInputInfo*> input_map;
  for (GraphTransferNodeInputInfo& input :
       *graph_transfer_info_->mutable_node_input_info()) {
    input_map.emplace(input.node_id(), &input);
  }

  // Setup dependency map placeholder
  std::vector<int> output_node_ids;
  std::unordered_map<int, std::unordered_set<int>> dependency_map;
  for (const GraphTransferNodeInfo& params :
       graph_transfer_info_->node_info()) {
    const int node_id = params.node_id();
    for (const string& output_node_name : output_node_names) {
      if (params.name() == output_node_name) {
        output_node_ids.emplace_back(node_id);
      }
    }

    dependency_map.emplace(std::piecewise_construct, std::make_tuple(node_id),
                           std::make_tuple());
    if (params.input_count() == 0) {
      continue;
    }
    CHECK_EQ(input_map.count(node_id), 1);
    for (const GraphTransferNodeInput& node_input :
         input_map.at(node_id)->node_input()) {
      dependency_map.at(node_id).emplace(node_input.node_id());
    }
  }

  // Create dependency map traversed from output nodes
  std::unordered_set<int> completed;
  for (int output_node_id : output_node_ids) {
    FillDependencyRec(output_node_id, dependency_map, completed);
  }

  std::sort(graph_transfer_info_->mutable_node_info()->begin(),
            graph_transfer_info_->mutable_node_info()->end(),
            TransferParamsComparator(dependency_map));
}

void GraphTransferer::EnableStrictCheckMode(const bool enable) {
  strict_check_mode_ = enable;
}

void GraphTransferer::SetSerializedGraphTransferInfo(
    const string& serialized_proto) {
  graph_transfer_info_->ParseFromString(serialized_proto);
}

const GraphTransferInfo& GraphTransferer::GetGraphTransferInfo() const {
  return *graph_transfer_info_;
}

GraphTransferInfo& GraphTransferer::GetMutableGraphTransferInfo() {
  return *graph_transfer_info_;
}

void GraphTransferer::CacheNode(const Node& node) {
  if (node_name_to_id_cache_map_.count(node.name()) > 0) {
    return;
  }
  node_name_cache_list_.emplace_back(&node);
  const int node_id = node_name_cache_list_.size() - 1;
  bool emplace_succeeded = false;
  std::tie(std::ignore, emplace_succeeded) =
      node_name_to_id_cache_map_.emplace(node.name(), node_id);
  CHECK(emplace_succeeded);
}

bool GraphTransferer::AreAllInputsCached(const Node& node) const {
  for (const Node* const input_node : node.in_nodes()) {
    if (node_name_to_id_cache_map_.count(input_node->name()) <= 0) {
      VLOG(1) << "input_node " << input_node->name() << " of " << node.name()
              << " is not cached yet.";
      return false;
    }
  }
  return true;
}

Status GraphTransferer::TransformGraphToAddAggregatedInputNode(
    const std::vector<std::pair<string, Tensor>>& input_node_info_list,
    Graph* graph, ShapeRefiner* shape_refiner) {
  // Transform a remote fused graph to add an aggregated input node which takes
  // all inputs of the remote graph.
  DataTypeVector input_data_types;
  std::vector<DataType> data_types;
  std::vector<TensorShape> shapes;
  std::vector<string> input_nodes;
  for (int i = 0; i < input_node_info_list.size(); ++i) {
    Node* node = FindMutableNodeByName(input_node_info_list.at(i).first, graph);
    CHECK_NOTNULL(node);
    input_nodes.emplace_back(node->name());
    input_data_types.emplace_back(input_node_info_list.at(i).second.dtype());
    data_types.emplace_back(input_node_info_list.at(i).second.dtype());
    shapes.emplace_back(input_node_info_list.at(i).second.shape());
  }

  NodeDef input_node_def;
  auto builder =
      NodeBuilder(AGGREGATED_INPUT_NODE_NAME, "RemoteFusedGraphExecute")
          .Input(std::vector<NodeBuilder::NodeOut>{})
          .Attr("Tinputs", DataTypeVector{})
          .Attr("Toutputs", input_data_types)
          .Attr("serialized_remote_fused_graph_execute_info", "")
          .Attr(RemoteFusedGraphExecuteUtils::ATTR_OUTPUT_DATA_TYPES,
                data_types)
          .Attr(RemoteFusedGraphExecuteUtils::ATTR_OUTPUT_SHAPES, shapes);

  Node* input_node;
  TF_RETURN_IF_ERROR(builder.Finalize(graph, &input_node));
  CHECK_NOTNULL(input_node);

  bool refined;
  TF_RETURN_IF_ERROR(
      shape_refiner->UpdateNode(input_node, false /* relax */, &refined));

  shape_inference::InferenceContext* context =
      shape_refiner->GetContext(input_node);
  for (int i = 0; i < input_node_info_list.size(); ++i) {
    shape_inference::ShapeHandle handle;
    TF_RETURN_IF_ERROR(context->MakeShapeFromTensorShape(
        input_node_info_list.at(i).second.shape(), &handle));
    TF_RETURN_IF_ERROR(shape_refiner->SetShape(input_node, i, handle));
  }

  // Cache the aggregate input node first as it's consumed first.
  CacheNode(*input_node);

  std::vector<Node*> original_input_nodes(input_nodes.size());

  for (int i = 0; i < input_nodes.size(); ++i) {
    const string& node_name = input_nodes.at(i);
    Node* original_input_node = FindMutableNodeByName(node_name, graph);
    CHECK_NOTNULL(original_input_node);
    CHECK_EQ(1, original_input_node->num_outputs());  // replaced by identity.
    Node* created_node;
    TF_RETURN_IF_ERROR(RemoteFusedGraphExecuteUtils::BuildIdentityOpNode(
        node_name, AGGREGATED_INPUT_NODE_NAME, i, data_types.at(i), graph,
        &created_node));
    CHECK_NOTNULL(created_node);
    std::vector<DataType> data_types;
    std::vector<TensorShape> shapes;
    Status status = RemoteFusedGraphExecuteUtils::GetOutputTensorShapeType(
        original_input_node->attrs(), &data_types, &shapes);
    if (status.ok()) {
      created_node->AddAttr(
          RemoteFusedGraphExecuteUtils::ATTR_OUTPUT_DATA_TYPES, data_types);
      created_node->AddAttr(RemoteFusedGraphExecuteUtils::ATTR_OUTPUT_SHAPES,
                            shapes);
    }
    for (const Edge* out_edge : original_input_node->out_edges()) {
      Node* dst = out_edge->dst();
      int dst_port = out_edge->dst_input();
      // Unused edge will be removed when removing node.
      graph->AddEdge(created_node, 0, dst, dst_port);
    }
    original_input_nodes[i] = original_input_node;

    TF_RETURN_IF_ERROR(
        shape_refiner->UpdateNode(created_node, false /* relax */, &refined));

    shape_inference::InferenceContext* context =
        shape_refiner->GetContext(created_node);
    CHECK_NOTNULL(context);

    // Cache replaced input node next to the aggregated input node.
    CacheNode(*created_node);
  }

  // Remove original input nodes after adding new input nodes to avoid
  // reusing same pointer in Graph.
  for (Node* original_input_node : original_input_nodes) {
    graph->RemoveNode(original_input_node);
  }

  return Status::OK();
}

Status GraphTransferer::RegisterNode(
    const IRemoteFusedGraphOpsDefinitions& ops_definitions,
    const ShapeRefiner& shape_refiner, const Node& node,
    const std::vector<std::pair<string, Tensor>>& input_node_info_list,
    const std::vector<string>& output_node_names) {
  VLOG(1) << "Register node: " << node.name() << ", " << std::hex
          << node_name_to_id_cache_map_.at(node.name());
  if (node.name() == SOURCE_NODE_NAME || node.name() == SINK_NODE_NAME) {
    // Just ignore sink and source
    return Status::OK();
  } else if (node.name() == AGGREGATED_INPUT_NODE_NAME) {
    RegisterInputNode(ops_definitions, shape_refiner, node);
    return Status::OK();
  } else if (node.IsConstant()) {
    RegisterConstantNode(shape_refiner, node);
  } else if (IsPadNode(node)) {
    RegisterPadNode(ops_definitions, shape_refiner, node);
  } else if (HasPaddingAndStrides(node)) {
    RegisterNodeWithPaddingAndStrides(ops_definitions, shape_refiner, node);
  } else if (NeedsToAddRank(node)) {
    RegisterNodeWithRank(ops_definitions, shape_refiner, node);
  } else if (IsNodeFlattenReshape(node, shape_refiner)) {
    RegisterFlattenNode(ops_definitions, shape_refiner, node);
  } else if (ops_definitions.GetOpIdFor(node.type_string(), {}) !=
             IRemoteFusedGraphOpsDefinitions::INVALID_OP_ID) {
    // TODO(satok): Set correct data type if it's given.
    RegisterGenericNode(ops_definitions, shape_refiner, node);
  } else {
    return errors::InvalidArgument(node.type_string() +
                                   " has not been implemented yet.");
  }

  return Status::OK();
}

void GraphTransferer::RegisterConstantNode(const ShapeRefiner& shape_refiner,
                                           const Node& node) {
  VLOG(1) << "Register constant node: " << node.name();
  CHECK_EQ(node_name_to_id_cache_map_.count(node.name()), 1);
  const int id = node_name_to_id_cache_map_[node.name()];
  const int output_node_size = node.num_outputs();
  CHECK_EQ(output_node_size, 1);
  // TODO(satok): support multiple outputs?
  const int output_index = 0;
  const DataType dt = node.output_type(output_index);
  const size_t max_bytes_per_data = DataTypeSize(dt);
  CHECK_GT(max_bytes_per_data, 0)
      << "dt = " << dt << ", " + DataTypeString(dt) << ", "
      << max_bytes_per_data << ", " << static_cast<int>(DataTypeSize(dt))
      << ",,,,,,,";
  shape_inference::InferenceContext* context = shape_refiner.GetContext(&node);
  shape_inference::ShapeHandle shape_handle = context->output(output_index);
  const shape_inference::DimensionHandle num_elements_dim =
      context->NumElements(shape_handle);
  std::array<int64, SHAPE_ARRAY_SIZE> shape_array;
  int data_size;
  // Shape of constant node must be known
  CHECK(context->ValueKnown(num_elements_dim));
  const int64 num_output_elements = context->Value(num_elements_dim);
  data_size = max_bytes_per_data * num_output_elements;
  shape_array = BuildShapeArray(shape_handle, context);

  GraphTransferConstNodeInfo& const_node_info =
      *graph_transfer_info_->add_const_node_info();
  const_node_info.set_name(node.name());
  const_node_info.set_node_id(id);
  // TODO(satok): Make this generic. Never assume rank is 4.
  CHECK_EQ(4, SHAPE_ARRAY_SIZE);
  const_node_info.add_shape(shape_array[0]);
  const_node_info.add_shape(shape_array[1]);
  const_node_info.add_shape(shape_array[2]);
  const_node_info.add_shape(shape_array[3]);
  const TensorProto* proto = nullptr;
  TF_CHECK_OK(GetNodeAttr(node.attrs(), "value", &proto));
  Tensor const_tensor;
  TF_CHECK_OK(MakeTensorFromProto(*proto, &const_tensor));

  const_node_info.set_dtype(const_tensor.dtype());
  if (data_size > 0) {
    const_node_info.set_data(const_tensor.tensor_data().data(), data_size);
  }
}

int GraphTransferer::RegisterConstantShape(const std::vector<int>& shape) {
  VLOG(1) << "Cache constant shape.";
  // TODO(satok): Handle non-4dim strides
  CHECK_EQ(shape.size(), 4);
  const string shape_name = CONST_SHAPE_PREFIX + ToString(shape.at(0)) + 'x' +
                            ToString(shape.at(1)) + 'x' +
                            ToString(shape.at(2)) + 'x' + ToString(shape.at(3));
  if (node_name_to_id_cache_map_.count(shape_name) <= 0) {
    node_name_cache_list_.emplace_back(nullptr);
    const int id = node_name_cache_list_.size() - 1;
    node_name_to_id_cache_map_.emplace(shape_name, id);
    GraphTransferConstNodeInfo& const_node_info =
        *graph_transfer_info_->add_const_node_info();
    const_node_info.set_name(shape_name);
    const_node_info.set_node_id(id);
    // TODO(satok): Make this generic. Never assume rank is 5.
    const_node_info.add_shape(static_cast<int64>(shape[0]));
    const_node_info.add_shape(static_cast<int64>(shape[1]));
    const_node_info.add_shape(static_cast<int64>(shape[2]));
    const_node_info.add_shape(static_cast<int64>(shape[3]));
  }
  return node_name_to_id_cache_map_[shape_name];
}

int GraphTransferer::RegisterConstTensor(const Tensor& tensor,
                                         const string& suffix) {
  VLOG(1) << "Cache const tensor.";
  const int dims = tensor.shape().dims();
  CHECK(dims <= 4);
  const string node_name = strings::StrCat(CONST_TENSOR_PREFIX, "_", suffix);
  if (node_name_to_id_cache_map_.count(node_name) <= 0) {
    node_name_cache_list_.emplace_back(nullptr);
    const int id = node_name_cache_list_.size() - 1;
    node_name_to_id_cache_map_.emplace(node_name, id);
    GraphTransferConstNodeInfo& const_node_info =
        *graph_transfer_info_->add_const_node_info();
    const_node_info.set_name(node_name);
    const_node_info.set_node_id(id);
    CHECK_EQ(4, SHAPE_ARRAY_SIZE);
    for (int i = 0; i < SHAPE_ARRAY_SIZE; ++i) {
      if (i < SHAPE_ARRAY_SIZE - dims) {
        const_node_info.add_shape(1);
      } else {
        const_node_info.add_shape(
            tensor.shape().dim_size(i - (SHAPE_ARRAY_SIZE - dims)));
      }
    }
    const_node_info.set_dtype(tensor.dtype());
    const_node_info.set_data(tensor.tensor_data().data(),
                             tensor.tensor_data().size());
  }
  return node_name_to_id_cache_map_[node_name];
}

int GraphTransferer::RegisterConstScalar(const DataType dt, const int val,
                                         const int dst_id,
                                         const int dst_input_count) {
  VLOG(1) << "Cache const.";
  const string val_name =
      CONST_VAL_PREFIX + ToString(dst_id) + '_' + ToString(dst_input_count);
  if (node_name_to_id_cache_map_.count(val_name) <= 0) {
    node_name_cache_list_.emplace_back(nullptr);
    const int id = node_name_cache_list_.size() - 1;
    node_name_to_id_cache_map_.emplace(val_name, id);
    GraphTransferConstNodeInfo& const_node_info =
        *graph_transfer_info_->add_const_node_info();
    const_node_info.set_name(val_name);
    const_node_info.set_node_id(id);
    // TODO(satok): Do not assume rank is 4 here.
    const_node_info.add_shape(static_cast<int64>(1));
    const_node_info.add_shape(static_cast<int64>(1));
    const_node_info.add_shape(static_cast<int64>(1));
    const_node_info.add_shape(static_cast<int64>(1));
    const_node_info.set_data(&val, DataTypeSize(dt));
  }
  return node_name_to_id_cache_map_[val_name];
}

bool GraphTransferer::HasPaddingAndStrides(const Node& node) {
  auto attrs = node.attrs();
  return attrs.Find(PADDING_ATTR_NAME) != nullptr &&
         attrs.Find(STRIDES_ATTR_NAME) != nullptr;
}

bool GraphTransferer::NeedsToAddRank(const Node& node) {
  const StringPiece op_type(node.type_string());
  if (op_type == "Transpose" || op_type == "ExpandDims") {
    return true;
  }
  return false;
}

bool GraphTransferer::IsPadNode(const Node& node) {
  const StringPiece op_type(node.type_string());
  if (op_type == "Pad") {
    return true;
  }
  return false;
}

bool GraphTransferer::IsNodeFlattenReshape(const Node& node,
                                           const ShapeRefiner& shape_refiner) {
  // Check if node is reshape op
  if (node.type_string() != RESHAPE_NODE_TYPE_STRING) {
    return false;
  }

  shape_inference::InferenceContext* context = shape_refiner.GetContext(&node);
  // Check if output count is valid
  if (context->num_outputs() != 1) {
    return false;
  }

  shape_inference::ShapeHandle shape_handle = context->output(0);
  std::array<int64, SHAPE_ARRAY_SIZE> shape_array;
  const shape_inference::DimensionHandle dim_handle =
      context->NumElements(shape_handle);

  // Obtain shape of output of node
  if (context->ValueKnown(dim_handle)) {
    shape_array = BuildShapeArray(shape_handle, context);
  } else {
    std::vector<TensorShape> shapes;
    TF_CHECK_OK(RemoteFusedGraphExecuteUtils::GetOutputTensorShapeType(
        node.attrs(), nullptr, &shapes));

    // Number of outputs should be 1 for reshape node.
    CHECK_EQ(1, shapes.size());
    shape_array = ToTensorShapeArray(shapes.at(0));
  }

  // check if reshape op just does flatten
  if (shape_array[0] == 1 && shape_array[1] == 1 && shape_array[2] == 1) {
    return true;
  } else {
    return false;
  }
}

void GraphTransferer::RegisterNodeWithPaddingAndStrides(
    const IRemoteFusedGraphOpsDefinitions& ops_definitions,
    const ShapeRefiner& shape_refiner, const Node& node) {
  CHECK_EQ(node_name_to_id_cache_map_.count(node.name()), 1);
  const int id = node_name_to_id_cache_map_[node.name()];
  shape_inference::InferenceContext* context = shape_refiner.GetContext(&node);
  CHECK(node.attrs().Find(PADDING_ATTR_NAME));
  // TODO(satok): Use context->GetAttr(...) instead?
  Padding padding;
  TF_CHECK_OK(context->GetAttr(PADDING_ATTR_NAME, &padding));
  CHECK(node.attrs().Find(STRIDES_ATTR_NAME));
  std::vector<int32> strides;
  TF_CHECK_OK(context->GetAttr(STRIDES_ATTR_NAME, &strides));
  const int stride_id = RegisterConstantShape(strides);
  std::vector<int> extra_inputs{stride_id};
  if (node.attrs().Find(KSIZE_ATTR_NAME)) {
    std::vector<int32> kernel_sizes;
    TF_CHECK_OK(context->GetAttr(KSIZE_ATTR_NAME, &kernel_sizes));
    const int ksize_id = RegisterConstantShape(kernel_sizes);
    extra_inputs.insert(extra_inputs.begin(), ksize_id);
  }
  // TODO(satok): Set correct data type if it's given.
  const int op_type_id = ops_definitions.GetOpIdFor(node.type_string(), {});
  CHECK(op_type_id >= 0 && op_type_id < ops_definitions.GetTotalOpsCount())
      << "Op " << node.type_string() << " not found in map(id = " << op_type_id
      << ")";
  // Safety check of padding id
  CHECK(padding == Padding::VALID ? 1 : 2);
  AppendNodeParamsWithIoParams(
      shape_refiner, node, node.name(), id, node.type_string(), op_type_id,
      static_cast<int>(padding), node.num_inputs(), extra_inputs,
      node.num_outputs(), true /* append_input */, true /* append_output */);
}

void GraphTransferer::RegisterNodeWithRank(
    const IRemoteFusedGraphOpsDefinitions& ops_definitions,
    const ShapeRefiner& shape_refiner, const Node& node) {
  CHECK_EQ(node_name_to_id_cache_map_.count(node.name()), 1);
  const int id = node_name_to_id_cache_map_[node.name()];
  shape_inference::InferenceContext* context = shape_refiner.GetContext(&node);
  const Node* input0_node;
  TF_CHECK_OK(node.input_node(0, &input0_node));
  CHECK_NOTNULL(input0_node);
  std::vector<TensorShape> shapes;
  Status status = RemoteFusedGraphExecuteUtils::GetOutputTensorShapeType(
      input0_node->attrs(), nullptr, &shapes);
  CHECK_EQ(1, shapes.size()) << "Output size should be 1.";
  const int const_val_id =
      RegisterConstScalar(DT_INT32, shapes.at(0).dims(), id, node.num_inputs());
  std::vector<int> extra_inputs{const_val_id};
  // TODO(satok): Set correct data type if it's given.
  const int op_type_id = ops_definitions.GetOpIdFor(node.type_string(), {});
  CHECK(op_type_id >= 0 && op_type_id < ops_definitions.GetTotalOpsCount())
      << "Op " << node.type_string() << " not found in map(id = " << op_type_id
      << ")";
  bool keep_dims = false;
  int padding_id = PADDING_NA_ID;
  if (context->GetAttr(KEEP_DIMS_ATTR_NAME, &keep_dims).ok()) {
    padding_id = keep_dims ? Padding::SAME : Padding::VALID;
  }

  AppendNodeParamsWithIoParams(
      shape_refiner, node, node.name(), id, node.type_string(), op_type_id,
      padding_id, node.num_inputs(), extra_inputs, node.num_outputs(),
      true /* append_input */, true /* append_output */);
}

void GraphTransferer::RegisterPadNode(
    const IRemoteFusedGraphOpsDefinitions& ops_definitions,
    const ShapeRefiner& shape_refiner, const Node& node) {
  static constexpr int PAD_WIDTH = 4;
  static constexpr int PAD_HEIGHT = 2;
  VLOG(1) << "Register generic node: " << node.name();
  CHECK_EQ(node_name_to_id_cache_map_.count(node.name()), 1);
  const int id = node_name_to_id_cache_map_[node.name()];

  // TODO(satok): Set correct data type if it's given.
  const int op_type_id = ops_definitions.GetOpIdFor(node.type_string(), {});
  CHECK(op_type_id >= 0 && op_type_id < ops_definitions.GetTotalOpsCount());

  CHECK_EQ(2, node.num_inputs());

  GraphTransferNodeInputInfo& node_input_info =
      *graph_transfer_info_->add_node_input_info();
  node_input_info.set_node_id(id);

  AddNodeInputByInputIndex(node, 0, &node_input_info);

  const Edge* edge = nullptr;
  TF_CHECK_OK(node.input_edge(1, &edge));
  const Node* input_node = edge->src();
  CHECK_NOTNULL(input_node);
  CHECK(input_node->IsConstant());

  const TensorProto* tensor_proto = nullptr;
  TF_CHECK_OK(GetNodeAttr(input_node->attrs(), "value", &tensor_proto));
  CHECK_NOTNULL(tensor_proto);
  Tensor const_tensor;
  TF_CHECK_OK(MakeTensorFromProto(*tensor_proto, &const_tensor));
  CHECK_EQ(2, const_tensor.shape().dims());
  CHECK_EQ(PAD_HEIGHT, const_tensor.shape().dim_size(1));
  if (const_tensor.shape().dim_size(0) == PAD_WIDTH) {
    AddNodeInputByInputIndex(node, 1, &node_input_info);
  } else if (const_tensor.shape().dim_size(0) < PAD_WIDTH) {
    const int width = const_tensor.shape().dim_size(0);
    const TensorProto* proto = nullptr;
    TF_CHECK_OK(GetNodeAttr(input_node->attrs(), "value", &proto));
    Tensor const_tensor;
    TF_CHECK_OK(MakeTensorFromProto(*proto, &const_tensor));
    CHECK_EQ(DT_INT32, const_tensor.dtype());
    // reshape tensor input to be rank 4.
    // TODO(satok): Never assume rank is 4.
    Tensor new_const_tensor(const_tensor.dtype(), TensorShape{4, 2});
    for (int i = 0; i < PAD_HEIGHT; ++i) {
      for (int j = 0; j < PAD_WIDTH; ++j) {
        if (j < PAD_WIDTH - width) {
          new_const_tensor.matrix<int32>()(j, i) = 0;
        } else {
          new_const_tensor.matrix<int32>()(j, i) =
              const_tensor.matrix<int32>()(j - (PAD_WIDTH - width), i);
        }
      }
    }

    const int id = RegisterConstTensor(
        new_const_tensor,
        strings::StrCat(input_node->name(), "_", node.name(), "_1"));

    GraphTransferNodeInput& node_input = *node_input_info.add_node_input();
    node_input.set_node_id(id);
    node_input.set_output_port(0);
  } else {
    LOG(FATAL);
  }

  AppendNodeParamsWithIoParams(
      shape_refiner, node, node.name(), id, node.type_string(), op_type_id,
      PADDING_NA_ID, node.num_inputs(), {}, node.num_outputs(),
      false /* append_input */, true /* append_output */);
}

void GraphTransferer::RegisterInputNode(
    const IRemoteFusedGraphOpsDefinitions& ops_definitions,
    const ShapeRefiner& shape_refiner, const Node& node) {
  const string op_type = node.type_string();
  VLOG(1) << "Register input node: " << node.name() << ", " << op_type;
  CHECK_EQ(node_name_to_id_cache_map_.count(node.name()), 1);
  const int id = node_name_to_id_cache_map_[node.name()];
  // TODO(satok): Set correct data type if it's given.
  const int op_type_id = ops_definitions.GetOpIdFor("INPUT", {});
  CHECK(op_type_id >= 0 && op_type_id < ops_definitions.GetTotalOpsCount())
      << "Op" << node.name() << ", " << op_type << " is not supported,"
      << op_type_id;
  AppendNodeParamsWithIoParams(
      shape_refiner, node, node.name(), id, node.type_string(), op_type_id,
      PADDING_NA_ID, node.num_inputs(), {}, node.num_outputs(),
      true /* append_input */, true /* append_output */);
}

void GraphTransferer::RegisterFlattenNode(
    const IRemoteFusedGraphOpsDefinitions& ops_definitions,
    const ShapeRefiner& shape_refiner, const Node& node) {
  VLOG(1) << "Register flatten node: " << node.name();
  CHECK_EQ(node_name_to_id_cache_map_.count(node.name()), 1);
  const int id = node_name_to_id_cache_map_[node.name()];
  // TODO(satok): Remove dependency to specific type
  const string op_type = "FLATTEN";
  // TODO(satok): Set correct data type if it's given.
  const int op_type_id = ops_definitions.GetOpIdFor(op_type, {});
  CHECK(op_type_id >= 0 && op_type_id < ops_definitions.GetTotalOpsCount());

  AppendNodeParamsWithIoParams(
      shape_refiner, node, node.name(), id, node.type_string(), op_type_id,
      PADDING_NA_ID, node.num_inputs(), {}, node.num_outputs(),
      true /* append_input */, true /* append_output */);
}

void GraphTransferer::RegisterGenericNode(
    const IRemoteFusedGraphOpsDefinitions& ops_definitions,
    const ShapeRefiner& shape_refiner, const Node& node) {
  VLOG(1) << "Register generic node: " << node.name();
  CHECK_EQ(node_name_to_id_cache_map_.count(node.name()), 1);
  const int id = node_name_to_id_cache_map_[node.name()];
  // TODO(satok): Set correct data type if it's given.
  const int op_type_id = ops_definitions.GetOpIdFor(node.type_string(), {});
  CHECK(op_type_id >= 0 && op_type_id < ops_definitions.GetTotalOpsCount());

  AppendNodeParamsWithIoParams(
      shape_refiner, node, node.name(), id, node.type_string(), op_type_id,
      PADDING_NA_ID, node.num_inputs(), {}, node.num_outputs(),
      true /* append_input */, true /* append_output */);
}

// TODO(satok): Remove this function.
// TODO(satok): Remove only_register_const_node.
Status GraphTransferer::RegisterNodeIfAllInputsAreCached(
    const IRemoteFusedGraphOpsDefinitions& ops_definitions,
    const ShapeRefiner& shape_refiner, const Node& node,
    const bool only_register_const_node,
    const std::vector<std::pair<string, Tensor>>& input_node_info_list,
    const std::vector<string>& output_node_names) {
  if (only_register_const_node && !node.IsConstant()) {
    return Status();
  }
  CHECK(AreAllInputsCached(node));
  return RegisterNode(ops_definitions, shape_refiner, node,
                      input_node_info_list, output_node_names);
}

// CAVEAT: Append inputs and outputs params accordingly
void GraphTransferer::AppendNodeParams(const string& name, const int id,
                                       const string& type, const int type_id,
                                       const int padding, const int inputs_size,
                                       const std::vector<int>& extra_inputs,
                                       const int outputs_size) {
  GraphTransferNodeInfo& node_info = *graph_transfer_info_->add_node_info();
  node_info.set_name(name);
  node_info.set_node_id(id);
  node_info.set_type_name(type);
  node_info.set_soc_op_id(type_id);
  node_info.set_padding_id(padding);
  node_info.set_input_count(inputs_size +
                            static_cast<int>(extra_inputs.size()));
  node_info.set_output_count(static_cast<int>(outputs_size));
}

void GraphTransferer::AddNodeInputByInputIndex(
    const Node& node, const int idx,
    GraphTransferNodeInputInfo* node_input_info) {
  const Edge* edge = nullptr;
  TF_CHECK_OK(node.input_edge(idx, &edge));
  const Node* input_node = edge->src();
  CHECK_NOTNULL(input_node);
  const int port = edge->src_output();

  const std::string& op_name = input_node->name();
  CHECK_GT(node_name_to_id_cache_map_.count(op_name), 0) << op_name;
  const int src_id = node_name_to_id_cache_map_[op_name];
  GraphTransferNodeInput& node_input = *node_input_info->add_node_input();
  node_input.set_node_id(src_id);
  node_input.set_output_port(port);
}

void GraphTransferer::AppendNodeInputParams(
    const int id, const Node& node, const std::vector<int>& extra_inputs) {
  VLOG(1) << "Append input params: " << node.name() << ", " << node.num_inputs()
          << ", " << extra_inputs.size();
  GraphTransferNodeInputInfo& node_input_info =
      *graph_transfer_info_->add_node_input_info();
  node_input_info.set_node_id(id);
  for (int i = 0; i < node.num_inputs(); ++i) {
    AddNodeInputByInputIndex(node, i, &node_input_info);
  }
  for (const int extra_input : extra_inputs) {
    GraphTransferNodeInput& node_input = *node_input_info.add_node_input();
    node_input.set_node_id(extra_input);
    node_input.set_output_port(0);
  }
}

void GraphTransferer::AppendNodeOutputParams(const ShapeRefiner& shape_refiner,
                                             const int id, const Node& node) {
  VLOG(1) << "Append output params: " << node.name() << ", "
          << node.num_outputs();
  GraphTransferNodeOutputInfo& node_output_info =
      *graph_transfer_info_->add_node_output_info();
  node_output_info.set_node_id(id);

  std::vector<DataType> data_types;
  std::vector<TensorShape> shapes;
  Status status = RemoteFusedGraphExecuteUtils::GetOutputTensorShapeType(
      node.attrs(), &data_types, &shapes);

  for (int i = 0; i < node.num_outputs(); ++i) {
    int data_size = -1;
    const int output_index = i;
    const DataType dt = node.output_type(output_index);
    const size_t max_bytes_per_data = DataTypeSize(dt);

    shape_inference::InferenceContext* context =
        shape_refiner.GetContext(&node);

    if (context != nullptr && context->ValueKnown(context->NumElements(
                                  context->output(output_index)))) {
      const shape_inference::DimensionHandle num_elements_dim =
          context->NumElements(context->output(output_index));
      const int64 num_output_elements = context->Value(num_elements_dim);
      data_size = max_bytes_per_data * num_output_elements;
      if (status.ok()) {
        TF_CHECK_OK(status);
        CHECK_EQ(shapes.at(i).num_elements(), num_output_elements);
      }
    } else {
      TF_CHECK_OK(status);
      // Use attribute attached to node
      data_size = max_bytes_per_data * shapes.at(i).num_elements();
    }
    CHECK_GE(data_size, 0);
    node_output_info.add_max_byte_size(data_size);
  }
}

void GraphTransferer::AppendNodeParamsWithIoParams(
    const ShapeRefiner& shape_refiner, const Node& node, const string& name,
    const int id, const string& type, const int type_id, const int padding,
    const int inputs_size, const std::vector<int>& extra_inputs,
    const int outputs_size, const bool append_input_params,
    const bool append_output_params) {
  VLOG(1) << "Append node with io params: " << node.name();
  if (append_input_params) {
    AppendNodeInputParams(id, node, extra_inputs);
  }
  if (append_output_params) {
    AppendNodeOutputParams(shape_refiner, id, node);
  }
  AppendNodeParams(name, id, type, type_id, padding, inputs_size, extra_inputs,
                   outputs_size);
}

/* static */ std::array<int64, GraphTransferer::SHAPE_ARRAY_SIZE>
GraphTransferer::BuildShapeArray(
    const shape_inference::ShapeHandle& shape_handle,
    shape_inference::InferenceContext* context) {
  switch (context->Rank(shape_handle)) {
    case 0:
      return std::array<int64, SHAPE_ARRAY_SIZE>{{1, 1, 1, 1}};
    case 1:
      return std::array<int64, SHAPE_ARRAY_SIZE>{
          {1, 1, 1, context->Value(context->Dim(shape_handle, 0))}};
    case 2:
      return std::array<int64, SHAPE_ARRAY_SIZE>{
          {1, 1, context->Value(context->Dim(shape_handle, 0)),
           context->Value(context->Dim(shape_handle, 1))}};
    case 3:
      return std::array<int64, SHAPE_ARRAY_SIZE>{
          {1, context->Value(context->Dim(shape_handle, 0)),
           context->Value(context->Dim(shape_handle, 1)),
           context->Value(context->Dim(shape_handle, 2))}};
    case 4:
      return std::array<int64, SHAPE_ARRAY_SIZE>{
          {context->Value(context->Dim(shape_handle, 0)),
           context->Value(context->Dim(shape_handle, 1)),
           context->Value(context->Dim(shape_handle, 2)),
           context->Value(context->Dim(shape_handle, 3))}};
    default:
      // TODO(satok): Support more ranks?
      LOG(FATAL);
      return std::array<int64, SHAPE_ARRAY_SIZE>();
  }
}

/* static */ std::array<int64, GraphTransferer::SHAPE_ARRAY_SIZE>
GraphTransferer::ToTensorShapeArray(const TensorShape& shape) {
  switch (shape.dims()) {
    case 0:
      return std::array<int64, SHAPE_ARRAY_SIZE>{{1, 1, 1, 1}};
    case 1:
      return std::array<int64, SHAPE_ARRAY_SIZE>{{1, 1, 1, shape.dim_size(0)}};
    case 2:
      return std::array<int64, SHAPE_ARRAY_SIZE>{
          {1, 1, shape.dim_size(0), shape.dim_size(1)}};
    case 3:
      return std::array<int64, SHAPE_ARRAY_SIZE>{
          {1, shape.dim_size(0), shape.dim_size(1), shape.dim_size(2)}};
    case 4:
      return std::array<int64, SHAPE_ARRAY_SIZE>{
          {shape.dim_size(0), shape.dim_size(1), shape.dim_size(2),
           shape.dim_size(3)}};
    default:
      // TODO(satok): Support more ranks?
      LOG(FATAL);
      return std::array<int64, SHAPE_ARRAY_SIZE>();
  }
}

/* static */ string GraphTransferer::ToPaddingDebugString(const int padding) {
  switch (padding) {
    case 0:
      return "NN_PAD_NA";
    case Padding::VALID:
      return "NN_PAD_VALID";
    case Padding::SAME:
      return "NN_PAD_SAME";
    default:
      LOG(FATAL);
      return "";
  }
}

GraphTransferer::TransferParamsComparator::TransferParamsComparator(
    const std::unordered_map<int, std::unordered_set<int>>& dep_map)
    : dependency_map_(dep_map) {}

bool GraphTransferer::TransferParamsComparator::operator()(
    const GraphTransferNodeInfo& obj0, const GraphTransferNodeInfo& obj1) {
  const int node_id0 = obj0.node_id();
  const int node_id1 = obj1.node_id();
  bool obj0_uses_obj1 = false;
  if (dependency_map_.count(node_id0) > 0) {
    obj0_uses_obj1 = dependency_map_.at(node_id0).count(node_id1) > 0;
  }
  bool obj1_uses_obj0 = false;
  if (dependency_map_.count(node_id1) > 0) {
    obj1_uses_obj0 = dependency_map_.at(node_id1).count(node_id0) > 0;
  }
  CHECK(!obj0_uses_obj1 || !obj1_uses_obj0);
  if (obj0_uses_obj1) {
    return false;
  } else if (obj1_uses_obj0) {
    return true;
  }
  // If there is no dependency between two nodes, it expects that
  // the execution order follows node id order.
  return node_id0 < node_id1;
}

/* static */ void GraphTransferer::FillDependencyRec(
    const int node_id,
    std::unordered_map<int, std::unordered_set<int>>& dep_map,
    std::unordered_set<int>& completed) {
  if (dep_map.count(node_id) == 0 || dep_map.at(node_id).empty() ||
      completed.count(node_id) == 1) {
    return;
  }
  CHECK_EQ(dep_map.count(node_id), 1);

  // Complete children's dependency map
  for (int child_node_id : dep_map.at(node_id)) {
    CHECK(child_node_id != node_id);
    if (completed.count(child_node_id) != 0) {
      continue;
    }
    FillDependencyRec(child_node_id, dep_map, completed);
  }

  // Find additional depending ids
  std::vector<int> depending_ids;
  for (int child_node_id : dep_map.at(node_id)) {
    if (dep_map.count(child_node_id) == 0) {
      continue;
    }
    for (int depending_id : dep_map.at(child_node_id)) {
      depending_ids.emplace_back(depending_id);
    }
  }

  // Insert additional depending ids
  for (int depending_id : depending_ids) {
    if (dep_map.at(node_id).count(depending_id) == 0) {
      dep_map.at(node_id).emplace(depending_id);
    }
  }

  // DP: Record completed node id
  completed.emplace(node_id);
}

/* static */ Status GraphTransferer::MakeTensorFromProto(
    const TensorProto& tensor_proto, Tensor* tensor) {
  if (tensor_proto.dtype() > 0 && tensor_proto.dtype() <= DataType_MAX) {
    Tensor parsed(tensor_proto.dtype());
    if (parsed.FromProto(cpu_allocator(), tensor_proto)) {
      *tensor = parsed;
      return Status::OK();
    }
  }
  return errors::InvalidArgument("Cannot parse tensor from proto: ",
                                 tensor_proto.DebugString());
}

void GraphTransferer::ClearCache() {
  node_name_cache_list_.clear();
  node_name_to_id_cache_map_.clear();
}

void GraphTransferer::DumpNodeTransferParams() const {
  LOG(INFO) << "*** Const Nodes ***";
  for (const GraphTransferConstNodeInfo& params :
       graph_transfer_info_->const_node_info()) {
    // TODO(satok): Stop assuming shape size is 4.
    CHECK_EQ(params.shape_size(), 4);
    LOG(INFO) << "[ " << params.node_id() << " \"" << params.name()
              << "\" (Const)";
    LOG(INFO) << "  shape: " << params.shape(0) << params.shape(1)
              << params.shape(2) << params.shape(3);
    LOG(INFO) << "  data_name: "
              << (params.data().length() <= 0
                      ? ""
                      : DATA_NODE_PREFIX + ToString(params.node_id()));
    LOG(INFO) << "  data_size: " << params.data().length() << " bytes"
              << " ]";
  }
  LOG(INFO) << "******\n";
  LOG(INFO) << "*** Op Nodes ***";
  for (const GraphTransferNodeInfo& params :
       graph_transfer_info_->node_info()) {
    LOG(INFO) << "[ " << params.node_id() << " \"" << params.name();
    LOG(INFO) << "  type: " << params.type_name();
    LOG(INFO) << "  padding: " << ToPaddingDebugString(params.padding_id());
    LOG(INFO) << "  inputs: " << INPUTS_NODE_PREFIX + ToString(params.node_id())
              << ", size = " << params.input_count();
    LOG(INFO) << "  outputs: "
              << (params.output_count() <= 0
                      ? NULL_OUTPUT_NAME
                      : (OUTPUTS_NODE_PREFIX + ToString(params.node_id())))
              << ", size = " << params.output_count() << " ]";
  }
  LOG(INFO) << "******\n";
  LOG(INFO) << "*** Node input params ***";
  for (const GraphTransferNodeInputInfo& params :
       graph_transfer_info_->node_input_info()) {
    LOG(INFO) << "[ " << params.node_id() << " ]";
    for (const GraphTransferNodeInput& node_input : params.node_input()) {
      LOG(INFO) << "    src node id = " << node_input.node_id()
                << ", output port = " << node_input.output_port();
    }
  }
  LOG(INFO) << "******\n";
  LOG(INFO) << "*** Node output params ***";
  for (const GraphTransferNodeOutputInfo& params :
       graph_transfer_info_->node_output_info()) {
    LOG(INFO) << "[ " << params.node_id() << " ]";
    for (const int max_size : params.max_byte_size()) {
      LOG(INFO) << "    max_size = " << max_size;
    }
  }
  LOG(INFO) << "******\n";
}

void GraphTransferer::DumpVerificationStringOfNodeTransferParams() const {
  for (const GraphTransferConstNodeInfo& params :
       graph_transfer_info_->const_node_info()) {
    std::stringstream sstream;
    // TODO(satok): Stop assuming shape size is 4.
    CHECK_EQ(params.shape_size(), 4);
    sstream << "---(CONST) [" << std::hex << params.node_id() << std::dec << ","
            << params.shape(0) << "," << params.shape(1) << ","
            << params.shape(2) << "," << params.shape(3) << ","
            << (params.data().length() <= 0
                    ? ""
                    : DATA_NODE_PREFIX + ToString(params.node_id()))
            << "," << params.data().length() << "," << params.name() << "]";
    LOG(INFO) << sstream.str();
  }
  LOG(INFO) << "Const node count = "
            << graph_transfer_info_->const_node_info_size();
  for (const GraphTransferNodeInfo& params :
       graph_transfer_info_->node_info()) {
    std::stringstream sstream;
    sstream << "---(OP) [" << params.name().c_str() << "," << std::hex
            << params.node_id() << std::dec << "," << params.soc_op_id() << ","
            << ToPaddingDebugString(params.padding_id()) << ","
            << INPUTS_NODE_PREFIX + ToString(params.node_id()) << ","
            << params.input_count() << ","
            << (params.output_count() <= 0
                    ? NULL_OUTPUT_NAME
                    : (OUTPUTS_NODE_PREFIX + ToString(params.node_id())))
            << "," << params.output_count() << "," << params.type_name() << "]";
    LOG(INFO) << sstream.str();
  }
  LOG(INFO) << "Op node count = " << graph_transfer_info_->node_info_size();
  for (const GraphTransferNodeInputInfo& params :
       graph_transfer_info_->node_input_info()) {
    std::stringstream sstream;
    sstream << "---(INPUT) [" << std::hex << params.node_id() << std::dec;
    for (const GraphTransferNodeInput& node_input : params.node_input()) {
      sstream << "," << std::hex << node_input.node_id() << std::dec << ","
              << node_input.output_port();
    }
    sstream << "]";
    LOG(INFO) << sstream.str();
  }
  LOG(INFO) << "Input params count = "
            << graph_transfer_info_->node_input_info_size();
  for (const GraphTransferNodeOutputInfo& params :
       graph_transfer_info_->node_output_info()) {
    std::stringstream sstream;
    sstream << "---(OUTPUT) [" << std::hex << params.node_id() << std::dec;
    for (const int max_size : params.max_byte_size()) {
      sstream << "," << max_size;
    }
    sstream << "]";
    LOG(INFO) << sstream.str();
  }
  LOG(INFO) << "Output params count = "
            << graph_transfer_info_->node_output_info_size();
}

}  // namespace tensorflow

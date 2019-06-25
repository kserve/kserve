/* Copyright 2018 The TensorFlow Authors. All Rights Reserved.

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
#include "tensorflow/core/grappler/optimizers/scoped_allocator_optimizer.h"

#include "tensorflow/core/common_runtime/scoped_allocator.h"
#include "tensorflow/core/common_runtime/scoped_allocator_mgr.h"
#include "tensorflow/core/framework/graph.pb.h"
#include "tensorflow/core/framework/node_def_builder.h"
#include "tensorflow/core/framework/node_def_util.h"
#include "tensorflow/core/graph/graph.h"
#include "tensorflow/core/grappler/costs/graph_properties.h"
#include "tensorflow/core/grappler/grappler_item.h"
#include "tensorflow/core/grappler/op_types.h"
#include "tensorflow/core/grappler/utils/frame.h"
#include "tensorflow/core/lib/gtl/inlined_vector.h"

// Like TF_RETURN_IF_ERROR, but also logs a WARNING.
#define LOG_WARNING_AND_RETURN_IF_ERROR(...)            \
  do {                                                  \
    const ::tensorflow::Status _status = (__VA_ARGS__); \
    if (TF_PREDICT_FALSE(!_status.ok())) {              \
      LOG(WARNING) << "error: " << _status;             \
      return _status;                                   \
    }                                                   \
  } while (0)

namespace tensorflow {
namespace grappler {

namespace {
// Node names often have some kind of name_scope prefix, with slashes,
// and a _nn numeric suffix.  Returns true if the main part of the node_name
// matches op_name, i.e. it looks from the name like this node is
// of that op type.
bool HasOpName(const string& node_name, const string& op_name) {
  size_t begin = node_name.rfind("/");
  if (begin == string::npos) {
    begin = 0;
  } else {
    ++begin;
  }
  size_t end = node_name.rfind("_");
  if (end != string::npos) {
    size_t p = end + 1;
    while (p < node_name.size()) {
      if (!isdigit(node_name[p])) {
        end = node_name.size();
        break;
      }
      ++p;
    }
  } else {
    end = node_name.size();
  }
  return node_name.substr(begin, end - begin) == op_name;
}

// After shape inference has been done each op should be annotated
// with its output shape(s).  This function iterates over a collection
// of ops that are a potential application of a ScopedAllocator.  It
// verifies whether they all have the same output type and if so
// gathers a vector of their output shapes.  It returns an error if
// any of the ops doesn't have type or shape data, or if it has more
// than one output, of if the output type of all ops is not the same.
// If it returns OK then *type and *shapes should be correctly populated.
Status CheckTypesAndGetShapes(const GraphProperties& graph_properties,
                              const std::vector<NodeDef*>& ops, DataType* type,
                              std::vector<TensorShape>* shapes) {
  VLOG(1) << "CheckTypesAndGetShapes";
  *type = DT_INVALID;
  for (NodeDef* n : ops) {
    AttrSlice n_attrs = AttrSlice(*n);
    DataType dtype;
    LOG_WARNING_AND_RETURN_IF_ERROR(GetNodeAttr(n_attrs, "T", &dtype));
    VLOG(2) << "op " << n->name() << " has type " << dtype << " shapes.size() "
            << shapes->size();
    if (!graph_properties.HasOutputProperties(n->name())) {
      LOG(ERROR) << "Node " << n->DebugString() << " lacks output shape.";
      return errors::Internal("Node ", n->name(), " lacks output shape.");
    }
    const std::vector<OpInfo::TensorProperties>& prop_list =
        graph_properties.GetOutputProperties(n->name());
    if (prop_list.size() != 1) {
      return errors::Internal("Node ", n->name(),
                              " does not have exactly one output as expected "
                              "by ScopedAllocatorOptimizer");
    }
    const OpInfo::TensorProperties& props = prop_list[0];
    if (shapes->empty()) {
      *type = props.dtype();
    } else if (*type != props.dtype()) {
      return errors::Internal("Group ops don't all have same type");
    } else if (!TensorShape::IsValid(props.shape())) {
      return errors::Internal("Complete shape not known for ", n->name());
    }
    VLOG(2) << "Adding shape " << props.shape().DebugString();
    shapes->push_back(TensorShape(props.shape()));
  }
  return Status::OK();
}

// Describes an existing input edge in the graph.
struct InputDesc {
  NodeDef* from_node_def;
  int output_slot;
  NodeDef* to_node_def;
  InputDesc(NodeDef* f, int os, NodeDef* t)
      : from_node_def(f), output_slot(os), to_node_def(t) {}
};

// Populates *inputs with all of the non-control inputs of ops.
// Returns error if it fails to find exactly one input for each op,
// or if some input is not of type dtype.
Status GetInputs(NodeMap* node_map, const std::vector<NodeDef*>& ops,
                 DataType dtype, std::vector<InputDesc>* inputs) {
  VLOG(1) << "Getinputs";
  for (NodeDef* n : ops) {
    NodeDef* inode = nullptr;
    int position = 0;
    VLOG(2) << "for node " << n->name();
    for (const auto& input_name : n->input()) {
      if (!IsControlInput(input_name)) {
        if (inode) {
          return errors::Internal("Found more than one input for node ",
                                  n->name());
        }
        ParseNodeName(input_name, &position);
        inode = node_map->GetNode(input_name);
        CHECK(inode) << input_name;
        VLOG(2) << "inode " << inode->DebugString();
      }
    }
    AttrSlice inode_attrs = AttrSlice(*inode);
    DataType inode_dtype;
    LOG_WARNING_AND_RETURN_IF_ERROR(
        GetNodeAttr(inode_attrs, "T", &inode_dtype));
    if (inode_dtype != dtype) {
      return errors::Internal("ScopedAllocatorOptimizer expected input type ",
                              dtype, " but found ", inode_dtype);
    }
    // inputs->push_back(InputDesc(inode, position, n));
    inputs->emplace_back(inode, position, n);
  }
  return Status::OK();
}

// Remove the NodeDef nd from node_map and graph.  It must be the case
// that nd no longer has any input or output edges, though that is not
// checked.
void RemoveNode(NodeDef* nd, GraphDef* graph, NodeMap* node_map) {
  node_map->RemoveNode(nd->name());
  // TODO(tucker): The efficiency of this routine is poor.
  // Change to accumulate and do a bulk removal, maybe refactoring
  // some code from dependency_optimizer.
  protobuf::RepeatedPtrField<NodeDef>* nodes = graph->mutable_node();
  for (int i = 0; i < nodes->size(); ++i) {
    if (nd->name() == (*nodes)[i].name()) {
      nodes->SwapElements(i, nodes->size() - 1);
      nodes->RemoveLast();
      return;
    }
  }
  LOG(FATAL) << "Failed to find node " << nd->name() << " in graph";
}

// Removes a named edge from between two nodes.
Status RemoveEdge(const string& input_edge_name, const string& from_node_name,
                  NodeDef* to_node, NodeMap* node_map) {
  if (node_map) {
    node_map->RemoveOutput(from_node_name, to_node->name());
  }
  protobuf::RepeatedPtrField<string>* inputs = to_node->mutable_input();
  int edge_index = -1;
  for (edge_index = 0; edge_index < inputs->size(); ++edge_index) {
    VLOG(2) << " consider edge " << (*inputs)[edge_index];
    if ((*inputs)[edge_index] == input_edge_name) {
      break;
    }
  }
  if (edge_index >= inputs->size()) {
    return errors::Internal("Could not find input name ", input_edge_name,
                            " at node ", to_node->name());
  }
  inputs->DeleteSubrange(edge_index, 1);
  return Status::OK();
}
}  // namespace

void ScopedAllocatorOptimizer::ExtendNodeAttr(StringPiece name,
                                              const std::vector<int32>& values,
                                              NodeDef* node_def) {
  if (HasNodeAttr(*node_def, name)) {
    VLOG(2) << "extending";
    AttrValue* existing = &(*node_def->mutable_attr())[string(name)];
    for (int32 i : values) {
      existing->mutable_list()->add_i(i);
    }
  } else {
    VLOG(2) << "setting new attr value";
    AddNodeAttr(name, values, node_def);
  }
}

class UnaryElementwiseRewriter : public ScopedAllocatorOptimizer::Rewriter {
 public:
  ~UnaryElementwiseRewriter() override {}

  // Return non-OK if any input is already committed to a ScopedAllocator.
  Status CheckExistingScopedAllocator(const std::vector<InputDesc>& inputs) {
    for (const InputDesc& nd : inputs) {
      VLOG(2) << "get attrs for " << nd.from_node_def->name();
      AttrSlice n_attrs = AttrSlice(*nd.from_node_def);
      int sa_id;
      Status ss = GetNodeAttr(n_attrs, "sa_id", &sa_id);
      if (ss.ok()) {
        LOG(INFO) << "Abandoning PARewriter because input "
                  << nd.from_node_def->name() << " is already assigned "
                  << "to ScopedAllocator " << sa_id;
        return errors::Internal(
            "Abandoning PARewriter because input ", nd.from_node_def->name(),
            " is already assigned to ScopedAllocator ", sa_id);
      }
    }
    return Status::OK();
  }

  // Return non-OK if any input is a member of op_set.
  Status CheckInternalDataDependency(const std::set<string>& op_set,
                                     const std::vector<InputDesc>& inputs) {
    for (const InputDesc& nd : inputs) {
      if (op_set.find(nd.from_node_def->name()) != op_set.end()) {
        if (nd.output_slot != tensorflow::Graph::kControlSlot) {
          return errors::Internal("Data edge exists bewtween ",
                                  nd.from_node_def->name(),
                                  " and another "
                                  "node in the set");
        }
      }
    }
    return Status::OK();
  }

  // Remove all control edges between members of ops.
  void ClearInternalControlInputs(const std::set<string>& op_set,
                                  const std::vector<NodeDef*>& ops,
                                  NodeMap* node_map) {
    for (NodeDef* n : ops) {
      for (const auto& input_name : n->input()) {
        if (IsControlInput(input_name)) {
          int position = 0;
          string input_node_name = ParseNodeName(input_name, &position);
          CHECK_EQ(position, -1);
          if (op_set.find(input_node_name) != op_set.end()) {
            // This is an internal control edge.  Remove it.
            VLOG(1) << "Remove control output from " << input_node_name
                    << " via edge " << input_name << " to " << n->name();
            TF_CHECK_OK(RemoveEdge(input_name, input_node_name, n, node_map));
          }
        }
      }
    }
  }

  // Examine the input set of an op set, gathering their shapes and types
  // and checking whether there are any considerations that prevent use
  // of a single ScopedAllocator for all of those inputs.
  Status AnalyzeInputs(ScopedAllocatorOptimizer* sa_opti, NodeMap* node_map,
                       const std::vector<NodeDef*>& ops,
                       const std::set<string>& op_instance_names,
                       string* device_name, DataType* dtype,
                       std::vector<TensorShape>* input_shapes,
                       std::vector<InputDesc>* inputs, TensorShape* sa_shape) {
    CHECK(graph_properties_);
    LOG_WARNING_AND_RETURN_IF_ERROR(
        CheckTypesAndGetShapes(*graph_properties_, ops, dtype, input_shapes));
    LOG_WARNING_AND_RETURN_IF_ERROR(
        GetInputs(sa_opti->node_map(), ops, *dtype, inputs));
    LOG_WARNING_AND_RETURN_IF_ERROR(CheckExistingScopedAllocator(*inputs));
    LOG_WARNING_AND_RETURN_IF_ERROR(
        CheckInternalDataDependency(op_instance_names, *inputs));
    ClearInternalControlInputs(op_instance_names, ops, node_map);
    *device_name = ops[0]->device();
    CHECK(!device_name->empty());
    CHECK(!input_shapes->empty());
    CHECK_EQ(0, Allocator::kAllocatorAlignment % DataTypeSize(*dtype))
        << "ScopedAllocatorOptimizer only applies to types that evenly "
        << "divide kAllocatorAlignment";
    std::vector<ScopedAllocator::Field> sa_fields;
    // Calculate the field embedding boundaries and thereby the
    // required size of the backing tensor.
    int64 num_bytes = ScopedAllocatorMgr::PopulateFields(
        0 /*scope_id*/, *input_shapes, *dtype, &sa_fields);
    int64 num_elts = num_bytes / DataTypeSize(*dtype);
    VLOG(2) << "num_bytes " << num_bytes << " num_elts=" << num_elts;
    *sa_shape = TensorShape({num_elts});
    return Status::OK();
  }

  // Build the ScopedAllocator node that will be assigned to allocate
  // the output tensors of the input node set.
  Status ConstructScopedAllocatorNode(
      ScopedAllocatorOptimizer* sa_opti, GraphDef* graph, NodeMap* node_map,
      const std::vector<NodeDef*>& ops, const string& device_name,
      DataType dtype, int sa_id, const string& sa_name,
      const std::vector<TensorShape>& input_shapes,
      const std::vector<InputDesc>& inputs, const TensorShape& sa_shape) {
    VLOG(2) << "ConstructScopedAllocatorNode " << sa_name;
    NodeDefBuilder sa_builder(sa_name, "_ScopedAllocator");
    sa_builder.Device(device_name);
    sa_builder.Attr("sa_name", sa_name);
    sa_builder.Attr("T", dtype);
    sa_builder.Attr("id", sa_id);
    sa_builder.Attr("shapes", input_shapes);
    sa_builder.Attr("shape", sa_shape);
    sa_builder.Attr("expected_call_count", static_cast<int64>(ops.size()));
    NodeDef* sa_node = graph->add_node();
    LOG_WARNING_AND_RETURN_IF_ERROR(sa_builder.Finalize(sa_node));
    node_map->AddNode(sa_name, sa_node);

    // Add control edges from the ScopedAllocatorOp to all of the
    // input nodes and mark them for allocation from backing tensor.
    for (int i = 0; i < inputs.size(); ++i) {
      auto& nd = inputs[i];
      VLOG(2) << "To input " << i << ": " << nd.from_node_def->name()
              << " add control input "
              << "^" << sa_name;
      nd.from_node_def->add_input(strings::StrCat("^", sa_name));
      // This attribute says: allocate output_slot from
      // ScopedAllocator instance sa_id + 1 + i.
      ScopedAllocatorOptimizer::ExtendNodeAttr("_scoped_allocator",
                                               {nd.output_slot, sa_id + 1 + i},
                                               nd.from_node_def);
      node_map->AddOutput(sa_name, nd.from_node_def->name());
    }
    return Status::OK();
  }

  Status BuildSAConcatNode(GraphDef* graph, NodeMap* node_map,
                           const std::vector<NodeDef*>& ops,
                           const std::set<string>& op_instance_names,
                           const string& device_name, DataType dtype, int sa_id,
                           const string& sa_name, const string& sac_name,
                           const TensorShape& sa_shape,
                           std::vector<NodeDefBuilder::NodeOut>* sac_inputs) {
    VLOG(2) << "BuildSAConcatNode " << sac_name;
    std::set<string> sac_ctl_inputs;
    for (int i = 0; i < ops.size(); ++i) {
      NodeDef* old_op = ops[i];
      for (const string& old_op_input : old_op->input()) {
        int position = 0;
        string input_name = ParseNodeName(old_op_input, &position);
        if (position == -1) {
          // A control input: drop if from another member of the op set.
          if (op_instance_names.find(old_op_input) == op_instance_names.end()) {
            sac_ctl_inputs.insert(old_op_input);
          }
        } else {
          // TODO(tucker): remove redundant check.
          // A data input: illegal if from another member of the op set.
          if (op_instance_names.find(old_op_input) != op_instance_names.end()) {
            LOG(ERROR) << "Data edge between " << old_op_input << " and "
                       << old_op->name() << " cannot build ScopedAllocator.";
            return errors::Internal("Data edge between ", old_op_input, " and ",
                                    old_op->name(),
                                    " cannot build ScopedAllocator.");
          }
          sac_inputs->push_back(
              NodeDefBuilder::NodeOut(old_op_input, 0, dtype));
        }
        VLOG(3) << "from op " << i << ": " << old_op->name()
                << " sac_inputs append " << old_op_input;
      }
    }
    NodeDefBuilder sac_builder(sac_name, "_ScopedAllocatorConcat");
    VLOG(2) << "New sac_name " << sac_name << " shape "
            << sa_shape.DebugString();
    sac_builder.Device(device_name);
    sac_builder.Attr("sa_name", sa_name);
    sac_builder.Attr("id", sa_id);
    sac_builder.Attr("T", dtype);
    sac_builder.Attr("shape", sa_shape);
    sac_builder.Attr("N", static_cast<int>(sac_inputs->size()));
    sac_builder.Input(NodeDefBuilder::NodeOut(sa_name, 0, dtype));
    sac_builder.Input(*sac_inputs);
    NodeDef* sac_node = graph->add_node();
    LOG_WARNING_AND_RETURN_IF_ERROR(sac_builder.Finalize(sac_node));
    node_map->AddNode(sac_name, sac_node);
    node_map->AddOutput(sa_name, sac_name);

    // Attach the old control inputs to the new sac node.
    for (const string& ctl_input : sac_ctl_inputs) {
      sac_node->add_input(ctl_input);
    }
    return Status::OK();
  }

  Status BuildReplacementOp(GraphDef* graph, NodeMap* node_map,
                            const std::vector<NodeDef*>& ops,
                            const string& device_name, DataType dtype,
                            const string& op_name, const string& sac_name,
                            const string& sa_op_name) {
    VLOG(2) << "BuildReplacementOp " << sa_op_name;
    NodeDefBuilder op_builder(sa_op_name, op_name);
    op_builder.Device(device_name);

    // Transfer the Node Attr from the first replaced Node to the new
    // Node.  TODO(tucker): In principle we should verify that
    // the Attr are consistent and compatible across all op instances.
    // Unfortunately that will probably require op-specific tests, so
    // punt on that for the time being.
    AttrSlice first_slice(*ops[0]);
    for (auto& it : first_slice) {
      op_builder.Attr(it.first, it.second);
    }
    op_builder.Attr("_forward_input", {0, 0});
    op_builder.Input(sac_name, 0, dtype);
    NodeDef* sa_op_node = graph->add_node();
    LOG_WARNING_AND_RETURN_IF_ERROR(op_builder.Finalize(sa_op_node));
    node_map->AddNode(sa_op_name, sa_op_node);
    node_map->AddOutput(sac_name, sa_op_name);
    return Status::OK();
  }

  Status BuildSplitNode(GraphDef* graph, NodeMap* node_map,
                        const std::vector<NodeDef*>& ops,
                        const std::vector<TensorShape>& input_shapes,
                        const std::vector<NodeDefBuilder::NodeOut>& sac_inputs,
                        const string& device_name, DataType dtype,
                        const string& op_name, int sa_id,
                        const string& sas_name, const string& sa_name,
                        const string& sa_op_name) {
    VLOG(2) << "new ScopedAllocatorSplit " << sas_name;
    NodeDefBuilder sas_builder(sas_name, "_ScopedAllocatorSplit");
    sas_builder.Device(device_name);
    sas_builder.Attr("sa_name", sa_name);
    sas_builder.Attr("id", sa_id);
    sas_builder.Attr("T", dtype);
    sas_builder.Attr("shapes", input_shapes);
    std::vector<NodeDefBuilder::NodeOut> sas_inputs = sac_inputs;
    sas_builder.Attr("N", static_cast<int>(sas_inputs.size()));
    sas_builder.Input(NodeDefBuilder::NodeOut(sa_op_name, 0, dtype));
    sas_builder.Input(sas_inputs);
    NodeDef* sas_node = graph->add_node();
    LOG_WARNING_AND_RETURN_IF_ERROR(sas_builder.Finalize(sas_node));
    node_map->AddNode(sas_name, sas_node);
    node_map->AddOutput(sa_op_name, sas_name);
    return Status::OK();
  }

  // After the new ScopedAllocator and its corresponding Concat and
  // Split nodes have been built, and a new single Op instance
  // constructed, rewire the graph: Remove input edges to the old Op
  // nodes and replace the old Op node outputs with the corresponding
  // ScopedAllocatorSplit node outputs.  After this the old Op nodes
  // should no longer have any input or output edges and they can be
  // removed from the graph.
  Status RewireSubgraph(GraphDef* graph, NodeMap* node_map,
                        const std::vector<NodeDef*>& ops,
                        const std::set<string>& op_instance_names,
                        const string& op_name, const string& sas_name) {
    VLOG(2) << "RewireSubgraph";
    for (int op_idx = 0; op_idx < ops.size(); ++op_idx) {
      NodeDef* old_op = ops[op_idx];
      // Copy the output node set since we'll be modifying the version
      // maintained by NodeMap in the loop.
      std::set<NodeDef*> output_nodes = node_map->GetOutputs(old_op->name());
      VLOG(3) << "old_op " << old_op->name() << " had " << output_nodes.size()
              << " outputs.  Moving them to the PASplit node.";
      if (VLOG_IS_ON(2)) {
        for (NodeDef* n : output_nodes) {
          VLOG(3) << "    output: " << n->name();
        }
      }
      for (NodeDef* n : output_nodes) {
        VLOG(3) << "really checking old output " << n->name()
                << " for corresponding input.";
        if (op_instance_names.find(n->name()) != op_instance_names.end()) {
          // If this output node is a member of the ops set, it must have
          // been an internal control edge so drop it.
          VLOG(3) << "Dropping control output from " << old_op->name() << " to "
                  << n->name();
          // However, we may already have dropped it at the clear() below,
          // so if we fail to find it, that's okay.
          Status ignore = RemoveEdge(strings::StrCat("^", old_op->name()),
                                     old_op->name(), n, node_map);
          continue;
        }
        bool found = false;
        VLOG(3) << "about to iterate over " << n->input_size() << " inputs";
        for (int i = 0; i < n->input_size(); ++i) {
          VLOG(3) << "input " << n->input(i);
          int position = 0;
          string input_node = ParseNodeName(n->input(i), &position);
          if (input_node == old_op->name()) {
            found = true;
            VLOG(3) << "match pos=" << position;
            if (position == -1) {
              // It was a control edge
              *n->mutable_input(i) = strings::StrCat("^", sas_name);
            } else {
              CHECK_EQ(0, position)
                  << "name " << n->input(i) << " pos " << position;
              *n->mutable_input(i) = strings::StrCat(sas_name, ":", op_idx);
            }
            node_map->RemoveOutput(old_op->name(), n->name());
            node_map->AddOutput(sas_name, n->name());
            VLOG(3) << "breaking on success";
            break;
          } else {
            VLOG(3) << "other input " << n->input(i);
          }
        }
        // In general it's required that we found the output node's old
        // input and replaced it, but one exception is if the output node
        // is of the same type being coalesced and the edge is a control
        // input.  In that case it probably got eliminated in an earlier
        // pass.
        VLOG(3) << "before HasOp";
        if (!HasOpName(n->name(), op_name)) {
          CHECK(found) << "old_op " << old_op->name() << " node "
                       << " could not find input edge on " << n->DebugString()
                       << " to replace."
                       << " " << op_name << " not in " << n->name();
        }
        VLOG(3) << "bottom of for output_nodes";
      }
      VLOG(3) << "Clearing all inputs of " << old_op->name();
      node_map->RemoveInputs(old_op->name());
      old_op->clear_input();
      node_map->RemoveOutputs(old_op->name());
      VLOG(3) << "after clear: " << old_op->DebugString();
      // old_op should be dead, with no further inputs or outputs.
      // It needs to be removed altogether before the graph is generated,
      // but we need to leave it around until this Optimizer is done,
      // because there may be some
      // Remove.
      RemoveNode(old_op, graph, node_map);
    }
    return Status::OK();
  }

  // Given a collection of instances of op_name, presumed to be
  // logically parallel and operating on tensors of the same type,
  // replace them by a single instance.  First find the upstream Ops
  // generating their inputs. Create a new ScopedAllocatorOp that
  // outputs a single backing_tensor pre-arranged for sub-allocation
  // of all of those input tensors.  Then insert a new
  // ScopedAllocatorConcatOp below the upstream Ops to make explicit
  // the materialization of a concatenation of their outputs.  Put the
  // new op_name instance below the new concat op and follow with a
  // ScopedAllocatorSplitOp that restores the correct shape outputs
  // for the consumers of the old op_name instances.
  //
  // There must be no non-control edges between Nodes in 'ops'.
  // Control edges among these nodes will be dropped.
  Status Rewrite(ScopedAllocatorOptimizer* sa_opti, GraphDef* graph,
                 const string& op_name, const std::vector<NodeDef*>& ops,
                 bool* applied) override {
    if (VLOG_IS_ON(1)) {
      VLOG(1) << "Rewrite";
      string op_names;
      for (auto& nd : ops) {
        strings::StrAppend(&op_names, nd->name(), ", ");
      }
      VLOG(1) << "UnaryElementwiseRewriter::Rewrite " << op_name
              << " to: " << op_names;
    }
    NodeMap* node_map = sa_opti->node_map();

    // Make a set of the node names for faster membership testing.
    std::set<string> op_instance_names;
    for (auto& nd : ops) {
      op_instance_names.insert(nd->name());
      VLOG(2) << "op_instance_name " << nd->name();
    }
    DataType dtype;
    std::vector<TensorShape> input_shapes;
    std::vector<InputDesc> inputs;
    TensorShape sa_shape;
    string device_name;

    TF_RETURN_IF_ERROR(AnalyzeInputs(sa_opti, node_map, ops, op_instance_names,
                                     &device_name, &dtype, &input_shapes,
                                     &inputs, &sa_shape));

    int sa_id = sa_opti->NewScopedAllocatorId(input_shapes.size());
    string sa_name = strings::StrCat("scoped_allocator_", sa_id);
    TF_RETURN_IF_ERROR(ConstructScopedAllocatorNode(
        sa_opti, graph, node_map, ops, device_name, dtype, sa_id, sa_name,
        input_shapes, inputs, sa_shape));

    // TODO(tucker): Maybe add control edges to delay execution of the
    // ScopedAllocatorOp until just before first use in order to
    // conserve memory.  What would be correct?  Let I0...In be the
    // input nodes that are all going to alloc from SA.  If we make
    // SA wait until all of these are ready, that might be too slow.
    // It should probably wait until at least one is ready, but which
    // one?  Maybe just pick the first.
    // {
    //   auto& nd = inputs[0];
    //   std::vector<InputDesc> inputs_to_first;
    //   LOG_WARNING_AND_RETURN_IF_ERROR(GetInputs(sa_opti->node_map(),
    //   {nd.from_node_def},
    //                                dtype, &inputs_to_first));
    //   for (int i = 0; i < inputs_to_first.size(); ++i) {
    //     sa_node->add_input(
    //         strings::StrCat("^", inputs_to_first[i].from_node_def->name()));
    //   }
    // }

    // Build a ScopedAllocatorConcat below all of the input nodes.
    std::vector<NodeDefBuilder::NodeOut> sac_inputs;
    string sac_name = strings::StrCat("scoped_allocator_concat_", sa_id);
    TF_RETURN_IF_ERROR(BuildSAConcatNode(
        graph, node_map, ops, op_instance_names, device_name, dtype, sa_id,
        sa_name, sac_name, sa_shape, &sac_inputs));

    // Construct a new instance of the parallel op and insert it
    // immediately below the new ScopedAllocatorConcat.
    string sa_op_name = strings::StrCat(sa_name, "_", op_name);
    TF_RETURN_IF_ERROR(BuildReplacementOp(graph, node_map, ops, device_name,
                                          dtype, op_name, sac_name,
                                          sa_op_name));

    // Build a ScopedAllocatorSplit split below the new Op.
    string sas_name = strings::StrCat("scoped_allocator_split_", sa_id);
    TF_RETURN_IF_ERROR(BuildSplitNode(graph, node_map, ops, input_shapes,
                                      sac_inputs, device_name, dtype, op_name,
                                      sa_id, sas_name, sa_name, sa_op_name));

    // Rewire the graph.
    TF_RETURN_IF_ERROR(RewireSubgraph(graph, node_map, ops, op_instance_names,
                                      op_name, sas_name));

    *applied = true;
    return Status::OK();
  }
};

ScopedAllocatorOptimizer::ScopedAllocatorOptimizer(
    RewriterConfig::Toggle opt_level, const ScopedAllocatorOptions& opts)
    : opt_level_(opt_level) {
  VLOG(1) << "ScopedAllocatorOptimizer::ScopedAllocatorOptimizer";
  Rewriter* r = new UnaryElementwiseRewriter();
  to_delete_.push_back(r);
  if (opts.enable_op_size() == 0) {
    // Opts handled by default:
    for (const auto& op_name : {"CollectiveReduce"}) {
      op_name_set_.insert(op_name);
      rewriters_[op_name] = r;
    }
  } else {
    for (const auto& op_name : opts.enable_op()) {
      op_name_set_.insert(op_name);
      rewriters_[op_name] = r;
    }
  }
}

Status ScopedAllocatorOptimizer::Optimize(Cluster* /*cluster*/,
                                          const GrapplerItem& item,
                                          GraphDef* optimized_graph) {
  *optimized_graph = item.graph;
  // Nodes that cannot be removed from the graph without damaging correctness,
  // typically fetch nodes.
  nodes_to_preserve_ = item.NodesToPreserve();

  GraphProperties graph_properties(item);
  const bool assume_valid_feeds = opt_level_ == RewriterConfig::AGGRESSIVE;
  LOG_WARNING_AND_RETURN_IF_ERROR(
      graph_properties.InferStatically(assume_valid_feeds));
  node_map_.reset(new NodeMap(optimized_graph));

  LOG_WARNING_AND_RETURN_IF_ERROR(ScopedAllocatorOptimizer::ProcessGraphDef(
      optimized_graph, graph_properties));

  VLOG(1) << "ScopedAllocatorOptimizer::Optimize() done";
  return Status::OK();
}

ScopedAllocatorOptimizer::Rewriter* ScopedAllocatorOptimizer::GetRewriter(
    const string& op_name) {
  auto it = rewriters_.find(op_name);
  if (it != rewriters_.end()) {
    return it->second;
  }
  return nullptr;
}

int ScopedAllocatorOptimizer::NewScopedAllocatorId(int num_fields) {
  CHECK_GT(num_fields, 0);
  int id = next_sa_id_;
  next_sa_id_ += (num_fields + 1);
  CHECK_GT(next_sa_id_, 0);
  return id;
}

ScopedAllocatorOptimizer::~ScopedAllocatorOptimizer() {
  for (auto ptr : to_delete_) {
    delete ptr;
  }
}

void ScopedAllocatorOptimizer::FindOpOccurrences(GraphDef* graph,
                                                 const OpNameSet& op_names,
                                                 GraphOpOccurrences* occs) {
  VLOG(1) << "FindOpOccurrences ";
  for (const auto& it : op_names) {
    VLOG(1) << "search target " << it;
  }
  for (int ni = 0; ni < graph->node_size(); ++ni) {
    NodeDef* node = graph->mutable_node(ni);
    const string& op_name = node->op();
    if (op_names.find(op_name) != op_names.end()) {
      VLOG(1) << "found " << op_name << " on dev " << node->device();
      (*occs)[node->device()][op_name].push_back(node);
    }
  }
}

namespace {
struct OpNameOrder {
  bool operator()(const NodeDef* a, const NodeDef* b) {
    return a->name() <= b->name();
  }
};

class Tree {
 public:
  Tree(const string& edge, int depth) : edge_(edge), depth_(depth) {}
  ~Tree() {
    for (auto it : subtrees_) delete it.second;
  }

  Tree* GetSubTree(const string& edge) {
    auto it = subtrees_.find(edge);
    if (it != subtrees_.end()) {
      return it->second;
    }
    Tree* t = new Tree(edge, depth_ + 1);
    subtrees_[edge] = t;
    return t;
  }

  void InsertNode(NodeDef* n) { nodes_.push_back(n); }

  string edge_;
  int depth_;
  std::vector<NodeDef*> nodes_;
  std::unordered_map<string, Tree*> subtrees_;
};

// Applies a function to every Tree in DFS order.  Terminates early
// on any non-OK Status.
Status ApplyToAll(Tree* tree, const std::function<Status(Tree*)>& func) {
  Status s;
  for (auto it : tree->subtrees_) {
    s = ApplyToAll(it.second, func);
    if (!s.ok()) return s;
  }
  s = func(tree);
  return s;
}

Tree* ComputeScopeTree(const string& op_name,
                       const std::vector<NodeDef*>& node_vec) {
  Tree* root = new Tree("", 0);
  for (NodeDef* n : node_vec) {
    std::vector<string> pieces = str_util::Split(n->name(), "/");
    // last piece is node name proper.
    int depth = pieces.size() - 1;
    Tree* subtree = root;
    for (int i = 0; i < depth; ++i) {
      subtree = subtree->GetSubTree(pieces[i]);
    }
    subtree->InsertNode(n);
  }
  return root;
}

void PartitionByLoopStructure(const FrameView& frame_view,
                              std::vector<NodeDef*> nodes,
                              std::vector<std::vector<NodeDef*>>* loop_groups) {
  // It is assumed that two nodes with identical loop containment have
  // identical integer vectors. Represent those by 64 bit hashes.
  std::unordered_map<uint64, std::vector<NodeDef*>> loop_sets;
  for (NodeDef* nd : nodes) {
    uint64 hash = 0;
    const std::vector<int>& loop_ids = frame_view.Frames(*nd);
    for (int id : loop_ids) {
      hash = Hash64Combine(hash, static_cast<uint64>(id));
    }
    loop_sets[hash].push_back(nd);
  }
  for (auto it : loop_sets) {
    loop_groups->push_back(std::move(it.second));
  }
}

}  // namespace

Status ScopedAllocatorOptimizer::ProcessGraphDef(
    GraphDef* graph, const GraphProperties& graph_properties) {
  VLOG(1) << "ProcessGraphDef";
  Status status;
  GraphOpOccurrences occ;
  FindOpOccurrences(graph, op_name_set_, &occ);
  if (!occ.empty()) {
    FrameView frame_view;
    // TODO(ezhulenev): Pass a GraphView when this optimizer will be migrated
    // from NodeMap.
    LOG_WARNING_AND_RETURN_IF_ERROR(frame_view.InferFromGraph(*graph));

    for (auto& dt : occ) {
      VLOG(2) << "Processing device " << dt.first;
      const DevOpOccurrences& dev_occ = dt.second;
      for (auto& it : dev_occ) {
        string op_name = it.first;
        VLOG(1) << "Processing " << op_name << " set size " << it.second.size();
        Rewriter* rewriter = GetRewriter(op_name);
        if (!rewriter) {
          LOG(ERROR) << "Failed to find PARewriter for op_name " << op_name;
          continue;
        }
        rewriter->SetGraphProperties(graph_properties);
        std::unique_ptr<Tree> root(ComputeScopeTree(it.first, it.second));
        // Nodes with a common depth and root path are now grouped
        // in the same Tree struct.  Split those groups into subgroups that
        // share identical loop nesting.
        status = ApplyToAll(root.get(), [this, rewriter, graph, &frame_view,
                                         &op_name](Tree* t) {
          VLOG(2) << "applied to tree node " << t->edge_ << " at depth "
                  << t->depth_ << " of size " << t->nodes_.size();
          if (t->nodes_.size() > 1) {
            std::vector<std::vector<NodeDef*>> loop_groups;
            PartitionByLoopStructure(frame_view, t->nodes_, &loop_groups);
            for (auto& lg : loop_groups) {
              if (lg.size() > 1) {
                bool applied = false;
                Status s = OrderNodeSet(&lg);
                TF_RETURN_IF_ERROR(s);
                VLOG(1) << "Applying Rewriter for " << op_name;
                s = rewriter->Rewrite(this, graph, op_name, lg, &applied);
                LOG_WARNING_AND_RETURN_IF_ERROR(s);
              }
            }
          }
          return Status::OK();
        });
        if (!status.ok()) {
          break;
        }
      }
      if (!status.ok()) {
        break;
      }
    }
  }
  VLOG(1) << "ScopedAllocatorOptimizer returning " << status;
  if (!status.ok()) {
    LOG(ERROR) << "ScopedAllocatorOptimizer: " << status;
  }
  return status;
}

namespace {
struct InstanceKeyLess {
  bool operator()(const NodeDef* a, const NodeDef* b) const {
    AttrSlice a_attrs = AttrSlice(*a);
    AttrSlice b_attrs = AttrSlice(*b);
    int32 a_key = -1;
    int32 b_key = -1;
    Status s = GetNodeAttr(a_attrs, "instance_key", &a_key);
    CHECK(s.ok());
    s = GetNodeAttr(b_attrs, "instance_key", &b_key);
    CHECK(s.ok());
    return a_key < b_key;
  }
};

struct NameLess {
  bool operator()(const NodeDef* a, const NodeDef* b) const {
    return a->name() < b->name();
  }
};

bool IsCollectiveNode(const NodeDef& n) {
  AttrSlice attrs = AttrSlice(n);
  int key = -1;
  if (!IsCollective(n)) return false;
  Status s = GetNodeAttr(attrs, "instance_key", &key);
  if (s.ok() && key >= 0) {
    return true;
  }
  return false;
}
}  // namespace

Status ScopedAllocatorOptimizer::OrderNodeSet(
    std::vector<NodeDef*>* nodes) const {
  // Nodes should be identical type.  Default order is by name but for
  // collectives we order by increasing instance_key so each group gets
  // the same instance_key.
  if (nodes->size() <= 1) return Status::OK();
  if (IsCollectiveNode(*nodes->at(0))) {
    sort(nodes->begin(), nodes->end(), InstanceKeyLess());
  } else {
    sort(nodes->begin(), nodes->end(), NameLess());
  }
  return Status::OK();
}

}  // namespace grappler
}  // namespace tensorflow

#undef LOG_WARNING_AND_RETURN_IF_ERROR

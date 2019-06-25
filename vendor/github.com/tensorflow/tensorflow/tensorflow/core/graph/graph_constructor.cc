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

#include "tensorflow/core/graph/graph_constructor.h"

#include <algorithm>
#include <set>
#include <string>
#include <unordered_map>
#include <vector>

#include "tensorflow/core/common_runtime/shape_refiner.h"
#include "tensorflow/core/framework/function.h"
#include "tensorflow/core/framework/function.pb.h"
#include "tensorflow/core/framework/graph.pb.h"
#include "tensorflow/core/framework/node_def.pb.h"
#include "tensorflow/core/framework/node_def_util.h"
#include "tensorflow/core/framework/tensor_shape.pb.h"
#include "tensorflow/core/framework/types.h"
#include "tensorflow/core/framework/versions.h"
#include "tensorflow/core/framework/versions.pb.h"
#include "tensorflow/core/graph/algorithm.h"
#include "tensorflow/core/graph/graph.h"
#include "tensorflow/core/graph/tensor_id.h"
#include "tensorflow/core/lib/core/errors.h"
#include "tensorflow/core/lib/gtl/flatmap.h"
#include "tensorflow/core/lib/gtl/flatset.h"
#include "tensorflow/core/lib/gtl/inlined_vector.h"
#include "tensorflow/core/lib/strings/scanner.h"
#include "tensorflow/core/lib/strings/str_util.h"
#include "tensorflow/core/platform/logging.h"
#include "tensorflow/core/public/version.h"

namespace tensorflow {

namespace {
inline bool IsMerge(const NodeDef& node_def) {
  return node_def.op() == "Merge" || node_def.op() == "RefMerge";
}

inline bool IsNextIteration(const NodeDef& node_def) {
  return node_def.op() == "NextIteration" ||
         node_def.op() == "RefNextIteration";
}

bool IsValidNodeName(StringPiece s, bool allow_internal_ops) {
  using ::tensorflow::strings::Scanner;
  return Scanner(s)
      .One(allow_internal_ops ? Scanner::LETTER_DIGIT_DOT_UNDERSCORE
                              : Scanner::LETTER_DIGIT_DOT)
      .Any(Scanner::LETTER_DIGIT_DASH_DOT_SLASH_UNDERSCORE)
      .Eos()
      .GetResult();
}

class GraphConstructor {
 public:
  struct Options {
    Options(const GraphConstructorOptions& in)  // NOLINT(runtime/explicit)
        : allow_internal_ops(in.allow_internal_ops),
          expect_device_spec(in.expect_device_spec),
          importing(false),
          validate_colocation_constraints(false) {}
    Options(const ImportGraphDefOptions& in)  // NOLINT(runtime/explicit)
        : allow_internal_ops(false),
          expect_device_spec(false),
          prefix(in.prefix.empty() || str_util::EndsWith(in.prefix, "/")
                     ? in.prefix
                     : in.prefix + "/"),
          uniquify_names(in.uniquify_names),
          uniquify_prefix(in.uniquify_prefix),
          input_map(in.input_map.begin(), in.input_map.end()),
          skip_mapped_nodes(in.skip_mapped_nodes),
          control_dependencies(in.control_dependencies),
          return_tensors(in.return_tensors.begin(), in.return_tensors.end()),
          return_nodes(in.return_nodes),
          importing(true),
          validate_colocation_constraints(in.validate_colocation_constraints),
          validate_shape(in.validate_shape),
          default_device(in.default_device) {}

    bool allow_internal_ops;
    bool expect_device_spec;

    string prefix;
    bool uniquify_names;
    bool uniquify_prefix;
    std::map<TensorId, TensorId> input_map;
    bool skip_mapped_nodes;
    std::vector<string> control_dependencies;
    std::vector<TensorId> return_tensors;
    std::vector<string> return_nodes;

    // TODO(ashankar): This bool exists to separate out functionality required
    // to make ImportGraphDef a close equivalent of Python's import_graph_def
    // without affecting the behavior of ConvertGraphDefToGraph at the time
    // ImportGraphDef was added.
    //
    // That said, the functionality here (shape and op validation) seems
    // applicable to ConvertGraphDefToGraph as well, so make an attempt to
    // remove this.
    bool importing;
    bool validate_colocation_constraints;
    bool validate_shape = true;

    string default_device;
  };

  typedef gtl::ArraySlice<const NodeDef*> NodeDefSlice;

  // versions and library may be nullptr
  static Status Construct(
      const Options& opts, NodeDefSlice node_defs, const VersionDef* versions,
      const FunctionDefLibrary* library, Graph* g, ShapeRefiner* refiner,
      std::vector<std::pair<Node*, int>>* return_tensors,
      std::vector<Node*>* return_nodes,
      std::vector<SafeTensorId>* missing_unused_input_map_keys) {
    if (versions) {
      TF_RETURN_IF_ERROR(CheckVersions(*versions, TF_GRAPH_DEF_VERSION,
                                       TF_GRAPH_DEF_VERSION_MIN_PRODUCER,
                                       "GraphDef", "graph"));
    }
    GraphConstructor c(opts, node_defs, versions, library, g, refiner,
                       return_tensors, return_nodes,
                       missing_unused_input_map_keys);
    const Status s = c.TryImport();
    if (!s.ok()) c.Undo();
    return s;
  }

 private:
  GraphConstructor(const Options& opts, NodeDefSlice node_defs,
                   const VersionDef* versions,
                   const FunctionDefLibrary* library, Graph* g,
                   ShapeRefiner* refiner,
                   std::vector<std::pair<Node*, int>>* return_tensors,
                   std::vector<Node*>* return_nodes,
                   std::vector<SafeTensorId>* missing_unused_input_map_keys)
      : opts_(opts),
        node_defs_(node_defs),
        versions_(versions),
        library_(library),
        g_(g),
        original_versions_(g->versions()),
        prefix_(opts.prefix),
        refiner_(refiner),
        return_tensors_(return_tensors),
        return_nodes_(return_nodes),
        missing_unused_input_map_keys_(missing_unused_input_map_keys) {}

  Status TryImport() {
    TF_RETURN_IF_ERROR(EnsureNoNameCollisions());
    TF_RETURN_IF_ERROR(ValidateInputMapAndControlDependencies());
    TF_RETURN_IF_ERROR(BuildNodeIndex());
    TF_RETURN_IF_ERROR(InitFromEdges());
    TF_RETURN_IF_ERROR(Convert());
    TF_RETURN_IF_ERROR(AddBackEdges());
    TF_RETURN_IF_ERROR(UpdateVersionDef());
    TF_RETURN_IF_ERROR(PopulateReturnTensors());
    TF_RETURN_IF_ERROR(PopulateReturnNodes());
    TF_RETURN_IF_ERROR(PopulateMissingUnusedInputMapKeys());
    UpdateUniquifiedColocationNames();
    FixupSourceAndSinkEdges(g_);
    return Status::OK();
  }

  Status EnsureNoNameCollisions();
  Status ValidateInputMapAndControlDependencies();
  Status BuildNodeIndex();
  Status InitFromEdges();
  Status Convert();
  Status AddBackEdges();
  Status UpdateVersionDef();
  Status PopulateReturnTensors();
  Status PopulateReturnNodes();
  Status PopulateMissingUnusedInputMapKeys();

  void Undo();

  Status IsNodeFullyMapped(const NodeDef& node_def, bool* is_node_mapped);
  Status ValidateColocationConstraints(const NodeDef& node_def);
  Status MakeNode(const NodeDef& node_def, Node** node);
  Status MakeEdge(Node* src, int output_index, Node* dst, int input_index);
  Status ValidateShape(Node* node);
  Status ModifyNodeDefForImport(NodeDef* node_def);
  // Modifies node_def's inputs according to opts_.input_map.
  // input_already_exists is a pre-initialized vector of length
  // node_def->input_size(). This function will mark inputs that are remapped to
  // true.
  void RemapNodeDefInputs(NodeDef* node_def,
                          std::vector<bool>* input_already_exists);
  // input_already_exists is a pre-initialized vector of length
  // node_def->input_size(). This function will add and mark control inputs as
  // true.
  void AddControlDependencies(NodeDef* node_def,
                              std::vector<bool>* input_already_exists);
  void AddPrefixToNodeDef(const std::vector<bool>& input_already_exists,
                          NodeDef* node_def);

  // Modifies `node_def` if its name isn't unique, or if any of its inputs'
  // names have been uniquified. This must be called in topological order on all
  // nodes.
  void UniquifyNames(const std::vector<bool>& input_already_exists,
                     NodeDef* node_def);

  // Updates any constructed nodes' colocation group names if the name has been
  // updated by UniquifyNames. This is called after all the nodes have been
  // constructed so all the names have been uniquified if necessary.
  void UpdateUniquifiedColocationNames();

  // Returns true if `name` already exists in `g_` (either as a node name or
  // prefix).
  bool NameExistsInGraph(StringPiece name);

  // Returns true if `name` already exists in the GraphDef being imported
  // (either as a node name or prefix).
  bool NameExistsInGraphDef(StringPiece name);

  // Returns a unique version of `original_name`, or `original_name` if it's
  // already unique in the graph.
  string FindUniqueName(StringPiece original_name);

  // Decrement pending count for users of `processed` and add the ones that now
  // have all of their pending inputs satisfied to `ready_`.
  void UpdatePendingCountAndReady(int processed);

  // From constructor
  const Options opts_;
  const NodeDefSlice node_defs_;
  const VersionDef* versions_;
  const FunctionDefLibrary* library_;
  Graph* g_;
  const VersionDef original_versions_;

  // A copy of opts_.prefix, possibly uniquified.
  string prefix_;

  ShapeRefiner* refiner_;

  // May be null. Not owned.
  std::vector<std::pair<Node*, int>>* return_tensors_;

  // May be null. Not owned.
  std::vector<Node*>* return_nodes_;

  // May be null. Not owned.
  std::vector<SafeTensorId>* missing_unused_input_map_keys_;

  // Intermediate datastructure used to populate
  // `missing_unused_input_map_keys_`.
  std::set<TensorId> used_input_map_keys_;

  // Mapping from node name to the index within node_defs_.
  struct NodeInfo {
    explicit NodeInfo(int i) : gdef_index(i), node(nullptr) {}
    // std::unordered_map<> requires that we have a default constructor.
    NodeInfo() : NodeInfo(-1) {}
    int gdef_index;
    Node* node;  // nullptr until the NodeDef is converted to a Node.
  };
  gtl::FlatMap<StringPiece, NodeInfo, StringPieceHasher> gdef_nodes_;

  // Prefixes already used in the GraphDef being imported.
  gtl::FlatSet<StringPiece, StringPieceHasher> gdef_prefixes_;

  // Mapping from node name to the existing node in g_.
  gtl::FlatMap<StringPiece, Node*, StringPieceHasher> existing_nodes_;

  // Prefixes already used in the graph.
  gtl::FlatSet<StringPiece, StringPieceHasher> existing_prefixes_;

  // Imported node names that have been uniquified. The key is the original
  // name, the value is the new unique name.
  gtl::FlatMap<string, string> uniquified_names_;

  // Index of NodeDefs in node_defs_ with all inputs already converted. We use a
  // (sorted) set so nodes are created in the order defined in the GraphDef.
  std::set<int> ready_;

  // Mapping between index within node_defs_ and the number of inputs that
  // still need to be converted.
  std::vector<int> pending_count_;

  // Mapping between index within node_defs_ and the index within node_defs_ of
  // all nodes it outputs to.
  std::vector<gtl::InlinedVector<int, 4>> outputs_;

  // Used in the conversion from node_defs_ to g_ to represent the ith input
  // of a node.
  struct InputInfo {
    explicit InputInfo(const string& node_name, Node* n, int i)
        : name(node_name), node(n), index(i) {}
    // Use string instead of StringPiece so we don't have to manage lifetime
    string name;
    Node* node;
    int index;
  };

  // Used in the conversion from node_defs_ to g_ to represent an edge from
  // the node named 'name' to node 'n'.
  struct EdgeInfo {
    explicit EdgeInfo(const string& name, int i1, Node* n, int i2)
        : src_name(name), src_index(i1), dst_node(n), dst_index(i2) {}
    // Use string instead of StringPiece so we don't have to manage lifetime
    string src_name;
    int src_index;
    Node* dst_node;
    int dst_index;
  };
  std::vector<EdgeInfo> back_edges_;
};

void GraphConstructor::UpdatePendingCountAndReady(int processed) {
  // We didn't consider NextIteration->Merge edges when computing
  // pending_counts_ so we should not have to consider it here either.
  bool is_next_iteration = IsNextIteration(*node_defs_[processed]);
  for (size_t i = 0; i < outputs_[processed].size(); ++i) {
    const int output = outputs_[processed][i];
    bool is_next_iteration_to_merge_edge =
        is_next_iteration && IsMerge(*node_defs_[output]);
    if (!is_next_iteration_to_merge_edge) {
      int* current_pending_count = &pending_count_[output];
      CHECK_GT(*current_pending_count, 0);
      (*current_pending_count)--;
      if (*current_pending_count == 0) {
        ready_.insert(output);
      }
    }
  }
}

// This could be expensive but we don't expect to call it often, if at all (only
// if there are multiple nodes in g_ with the same name)
bool NodeNameInValues(const std::map<TensorId, TensorId>& input_map,
                      const StringPiece& node_name) {
  for (auto iter = input_map.begin(); iter != input_map.end(); ++iter) {
    if (iter->second.first == node_name) return true;
  }
  return false;
}

bool NodeNameInValues(const std::vector<string>& control_dependencies,
                      const StringPiece& node_name) {
  return std::find(control_dependencies.begin(), control_dependencies.end(),
                   node_name) != control_dependencies.end();
}

// Adds any prefixes of `node_name` (not including the full name itself) to
// `prefixes`.
void AddPrefixes(StringPiece node_name,
                 gtl::FlatSet<StringPiece, StringPieceHasher>* prefixes) {
  size_t idx = -1;
  while ((idx = node_name.find('/', idx + 1)) != StringPiece::npos) {
    prefixes->insert(node_name.substr(0, idx));
  }
}

Status GraphConstructor::EnsureNoNameCollisions() {
  existing_nodes_.reserve(g_->num_nodes());
  // Populate existing_nodes_ and existing_prefixes_.
  for (Node* n : g_->nodes()) {
    bool already_exists = !existing_nodes_.insert({n->name(), n}).second;
    if (already_exists) {
      if (NodeNameInValues(opts_.input_map, n->name())) {
        return errors::InvalidArgument(
            "cannot resolve input_map because multiple nodes exist with name '",
            n->name(), "'");
      }
      if (NodeNameInValues(opts_.control_dependencies, n->name())) {
        return errors::InvalidArgument(
            "cannot resolve control_dependencies because multiple nodes exist "
            "with name '",
            n->name(), "'");
      }
    }
    AddPrefixes(n->name(), &existing_prefixes_);
  }
  if (prefix_.empty() && opts_.importing && !opts_.uniquify_names) {
    for (const NodeDef* n : node_defs_) {
      const string& name = n->name();
      if (NameExistsInGraph(name)) {
        return errors::InvalidArgument("Node name '", name,
                                       "' already exists in the Graph");
      }
    }
  } else if (!prefix_.empty()) {
    StringPiece prefix_no_slash(prefix_);
    prefix_no_slash.remove_suffix(1);
    if (!IsValidNodeName(prefix_no_slash, false)) {
      return errors::InvalidArgument("Imported node name prefix '", prefix_,
                                     "' would lead to invalid node names");
    }
    if (NameExistsInGraph(prefix_no_slash) && opts_.uniquify_prefix) {
      prefix_ = strings::StrCat(FindUniqueName(prefix_no_slash), "/");
    }
  }
  return Status::OK();
}

Status GraphConstructor::ValidateInputMapAndControlDependencies() {
  for (const auto& mapping : opts_.input_map) {
    TensorId src = mapping.first;
    TensorId dst = mapping.second;
    if (existing_nodes_.count(dst.first) == 0) {
      return errors::InvalidArgument(
          "node '", dst.first, "' in input_map does not exist in graph ",
          "(input_map entry: ", src.ToString(), "->", dst.ToString(), ")");
    }
    if ((src.second == Graph::kControlSlot) !=
        (dst.second == Graph::kControlSlot)) {
      return errors::InvalidArgument("input_map entry ", src.ToString(), "->",
                                     dst.ToString(), " between ",
                                     "control edge and non-control edge");
    }
  }
  for (const string& node : opts_.control_dependencies) {
    if (existing_nodes_.count(node) == 0) {
      return errors::InvalidArgument(
          "node '", node,
          "' in control_dependencies does not exist in "
          "graph");
    }
  }
  return Status::OK();
}

Status GraphConstructor::BuildNodeIndex() {
  // Validate the node names and add them to gdef_nodes_ and gdef_prefixes_.
  for (int n = 0; n < node_defs_.size(); ++n) {
    const NodeDef& node_def = *node_defs_[n];
    if (!IsValidNodeName(node_def.name(), opts_.allow_internal_ops)) {
      return errors::InvalidArgument(
          "Node '", node_def.name(),
          "': Node name contains invalid characters");
    }
    if (!gdef_nodes_
             .insert(std::make_pair(StringPiece(node_def.name()), NodeInfo(n)))
             .second) {
      return errors::InvalidArgument("Node '", node_def.name(),
                                     "' is not unique");
    }
    // Validate the operation's type.
    if (node_def.op().empty()) {
      return errors::InvalidArgument("Node '", node_def.name(),
                                     "' does not specify an operation");
    }
    if (opts_.expect_device_spec && node_def.device().empty()) {
      return errors::InvalidArgument("Node '", node_def.name(),
                                     "' is missing a device specification");
    }
    // Validate control edges at end
    bool in_control_dependence = false;
    for (int i = 0; i < node_def.input_size(); ++i) {
      StringPiece input_name = node_def.input(i);
      if (!input_name.empty() && str_util::StartsWith(input_name, "^")) {
        in_control_dependence = true;
      } else if (in_control_dependence) {
        return errors::InvalidArgument(
            "Node '", node_def.name(),
            "': Control dependencies must come after regular dependencies");
      }
    }
    // Update gdef_prefixes_.
    AddPrefixes(node_def.name(), &gdef_prefixes_);
  }
  return Status::OK();
}

std::unordered_set<string> GetNextIterationNodes(
    const GraphConstructor::NodeDefSlice& node_defs) {
  std::unordered_set<string> next_iteration_nodes;

  for (int n = 0; n < node_defs.size(); ++n) {
    const NodeDef& node_def = *node_defs[n];
    if (IsNextIteration(node_def)) {
      next_iteration_nodes.insert(node_def.name());
    }
  }

  return next_iteration_nodes;
}

Status GraphConstructor::InitFromEdges() {
  const int num_nodes = node_defs_.size();
  pending_count_.reserve(num_nodes);
  outputs_.resize(num_nodes);
  std::unordered_set<string> next_iteration_nodes_ =
      GetNextIterationNodes(node_defs_);

  // Parse the inputs for each node.
  for (int n = 0; n < num_nodes; ++n) {
    const NodeDef& node_def = *node_defs_[n];
    int pending_count = node_def.input_size();
    if (IsMerge(node_def)) {
      // Cycles in the graph are only allowed for while loops. A while loop is
      // identified by an edge from a NextIteration node to a Merge node. For
      // such Merge nodes, only wait for one non-control input before
      // considering the node ready to process in Convert().
      int32 num_control_edges = 0;
      bool has_loop_back_edge = false;
      for (int i = 0; i < node_def.input_size(); ++i) {
        StringPiece input_name(node_def.input(i));
        if (str_util::StartsWith(input_name, "^")) {
          num_control_edges++;
        } else {
          TensorId id(ParseTensorName(input_name));
          if (next_iteration_nodes_.find(string(id.first)) !=
              next_iteration_nodes_.end()) {
            has_loop_back_edge = true;
          }
        }
      }
      if (has_loop_back_edge) {
        pending_count = num_control_edges + 1;
      }
    }
    for (int i = 0; i < node_def.input_size(); ++i) {
      StringPiece input_name = node_def.input(i);
      TensorId id(ParseTensorName(input_name));
      if (opts_.input_map.count(id) == 0) {
        // If an input is not mapped, then the input should appear in the graph
        // being imported.
        auto iter = gdef_nodes_.find(id.first);
        if (iter == gdef_nodes_.end()) {
          return errors::InvalidArgument("Node '", node_def.name(),
                                         "': Unknown input node '",
                                         node_def.input(i), "'");
        }
        outputs_[iter->second.gdef_index].push_back(n);
      } else {
        // This input is mapped to an existing edge. Therefore this input is
        // as good as being already processed.
        --pending_count;
        DCHECK_GE(pending_count, 0);
      }
    }
    if (pending_count == 0) {
      ready_.insert(n);
    }
    pending_count_.push_back(pending_count);
  }
  return Status::OK();
}

Status GraphConstructor::ValidateColocationConstraints(
    const NodeDef& node_def) {
  if (!opts_.validate_colocation_constraints || !opts_.importing)
    return Status::OK();
  const auto iter = node_def.attr().find(kColocationAttrName);
  if (iter == node_def.attr().end()) return Status::OK();
  for (const string& c : iter->second.list().s()) {
    StringPiece s(c);
    if (str_util::ConsumePrefix(&s, kColocationGroupPrefix) &&
        gdef_nodes_.find(s) == gdef_nodes_.end()) {
      return errors::InvalidArgument(
          "Node '", node_def.name(),
          "' expects to be colocated with unknown node '", s, "'");
    }
  }
  return Status::OK();
}

Status GraphConstructor::MakeNode(const NodeDef& node_def, Node** node) {
  // Add the node to the graph.
  Status status;
  *node = g_->AddNode(node_def, &status);
  if (!status.ok()) return status;
  if (opts_.expect_device_spec) {
    (*node)->set_assigned_device_name(node_def.device());
  }
  return Status::OK();
}

Status GraphConstructor::ValidateShape(Node* node) {
  if (!opts_.importing || !opts_.validate_shape) return Status::OK();
  TF_RETURN_IF_ERROR(refiner_->AddNode(node));
  // For nodes with the _output_shapes attribute, override the shape.
  std::vector<TensorShapeProto> shape_attrs;
  const char* kAttrName = "_output_shapes";
  if (!GetNodeAttr(node->attrs(), kAttrName, &shape_attrs).ok()) {
    // No _output_shapes attribute, the AddNode call above was sufficient.
    return Status::OK();
  }
  auto* ic = refiner_->GetContext(node);
  DCHECK(ic != nullptr)
      << "ShapeRefiner::AddNode() should have created the InferenceContext";
  if (shape_attrs.size() < node->num_outputs()) {
    return errors::InvalidArgument(
        "Node '", node->name(), "' has ", node->num_outputs(),
        " outputs but the ", kAttrName, " attribute specifies shapes for ",
        shape_attrs.size(), " outputs");
  }
  // NOTE(skyewm): we don't raise an error here because some users depend on
  // this behavior, even though it's unsafe.
  // TODO(b/74619486): raise an error.
  if (shape_attrs.size() > node->num_outputs()) {
    LOG(WARNING) << "Node '" << node->name() << "' has " << node->num_outputs()
                 << " outputs but the " << kAttrName
                 << " attribute specifies shapes for " << shape_attrs.size()
                 << " outputs. Output shapes may be inaccurate.";
  }
  for (int i = 0; i < node->num_outputs(); ++i) {
    const TensorShapeProto& p = shape_attrs[i];
    shape_inference::ShapeHandle h;
    Status s = ic->MakeShapeFromShapeProto(p, &h);
    if (!s.ok()) {
      return errors::InvalidArgument("Node '", node->name(), " has an invalid ",
                                     kAttrName, " attribute (shape #", i,
                                     " error:'", s.error_message(), "'");
    }
    s = refiner_->SetShape(node, i, h);
    if (!s.ok()) {
      // If the output shape is incompatible with what is inferred
      // by the graph for a very specific whitelist of ops, then we
      // ignore this output shape.  This can happen if there is a
      // bug in the shape function for some operation, and the
      // serialized graph def has the incorrect shape set when
      // running on a newer binary with the fixed shape function.
      // This is an escape hatch that allows us to correct shape
      // functions that are not critical to correct execution but
      // would cause graphs to fail if imported after correcting.
      //
      const string& op = node->type_string();
      const std::vector<string> whitelist = {
          // To be removed after 2017/03/08.
          "RandomShuffleQueue",
          "PaddingFIFOQueue",
          "FIFOQueue",
          "PriorityQueue",
          "QueueSize",
          "Stack",
          "Barrier",
          "BarrierReadySize",
          "BarrierIncompleteSize",
          "HashTable",
          "MutableHashTable",
          "MutableHashTableOfTensors",
          "Mutex",
          "CuckooTable",
          "IndexTable",
          "WholeFileReader",
          "TextLineReader",
          "FixedLengthRecordReader",
          "TFRecordReader",
          "IdentityReader",
          "RefSwitch",
          "RefEnter",
          "RefNextIteration",
          "RefMerge",
          "RefIdentity",
          "LMDBReader",
          // To be removed after 2017/04/24.
          "ConditionalAccumulator",
          "SparseConditionalAccumulator",
          "Table",
      };
      if (std::find(whitelist.begin(), whitelist.end(), op) ==
          whitelist.end()) {
        return errors::InvalidArgument(
            "Node '", node->name(), "' has an ", kAttrName,
            " attribute inconsistent with the GraphDef for output #", i, ": ",
            s.error_message());
      }
    }
  }
  node->ClearAttr(kAttrName);
  return Status::OK();
}

Status GraphConstructor::ModifyNodeDefForImport(NodeDef* node_def) {
  const OpDef* op_def;
  TF_RETURN_IF_ERROR(g_->op_registry()->LookUpOpDef(node_def->op(), &op_def));
  AddDefaultsToNodeDef(*op_def, node_def);
  TF_RETURN_IF_ERROR(ValidateNodeDef(*node_def, *op_def));
  if (versions_) {
    TF_RETURN_IF_ERROR(CheckOpDeprecation(*op_def, versions_->producer()));
  }
  return Status::OK();
}

void RemoveInputs(const std::vector<int>& inputs_to_remove, NodeDef* node_def,
                  std::vector<bool>* input_already_exists) {
  // Remove 'inputs_to_remove' from 'node_def'
  NodeDef copy;
  copy.mutable_input()->Reserve(node_def->input_size() -
                                inputs_to_remove.size());
  for (int i = 0, j = 0; i < node_def->input_size(); ++i) {
    if (j < inputs_to_remove.size() && i == inputs_to_remove[j]) {
      ++j;
    } else {
      copy.add_input()->swap(*node_def->mutable_input(i));
    }
  }
  node_def->mutable_input()->Swap(copy.mutable_input());
  // Remove 'inputs_to_remove' from 'input_already_exists'
  for (int idx : inputs_to_remove) {
    input_already_exists->erase(input_already_exists->begin() + idx);
  }
  DCHECK_EQ(input_already_exists->size(), node_def->input_size());
}

void GraphConstructor::RemapNodeDefInputs(
    NodeDef* node_def, std::vector<bool>* input_already_exists) {
  DCHECK_EQ(input_already_exists->size(), node_def->input_size());
  std::set<TensorId> control_inputs;
  std::vector<int> inputs_to_remove;

  for (int i = 0; i < node_def->input_size(); ++i) {
    auto iter = opts_.input_map.find(ParseTensorName(node_def->input(i)));
    if (iter == opts_.input_map.end()) continue;
    used_input_map_keys_.insert(iter->first);

    TensorId new_input = iter->second;
    if (new_input.second == Graph::kControlSlot) {
      // Check if we've already remapped a different input to new_input, and if
      // so remove this input.
      if (control_inputs.count(new_input) > 0) {
        inputs_to_remove.push_back(i);
        continue;
      }
      control_inputs.insert(new_input);
    }
    node_def->set_input(i, new_input.ToString());
    (*input_already_exists)[i] = true;
  }
  if (!inputs_to_remove.empty()) {
    RemoveInputs(inputs_to_remove, node_def, input_already_exists);
  }
}

void GraphConstructor::AddControlDependencies(
    NodeDef* node_def, std::vector<bool>* input_already_exists) {
  // To avoid adding redundant control dependencies to every imported node, skip
  // nodes that will inherit the dependencies from another imported node.
  bool inherits_deps = false;
  for (int i = 0; i < node_def->input_size(); ++i) {
    // Assume we won't inherit dependencies from remapped inputs that already
    // exist in the graph. Even if we're wrong, we'll only add redundant
    // dependencies.
    if ((*input_already_exists)[i]) continue;

    // If this input is a backedge, assume we won't inherit the dependencies.
    // TODO(skyewm): we have many redundant ParseTensorName calls. It could be
    // worth optimizing these.
    TensorId id(ParseTensorName(node_def->input(i)));
    auto iter = gdef_nodes_.find(id.first);
    DCHECK(iter != gdef_nodes_.end()) << id.first;
    if (iter->second.node == nullptr) {
      // Input hasn't been created yet, indicating it's a backedge.
      continue;
    }
    inherits_deps = true;
  }
  if (inherits_deps) return;

  // node_def either has no inputs or all remapped inputs, add the control
  // dependencies
  for (const string& control_dep : opts_.control_dependencies) {
    string input = TensorId(control_dep, Graph::kControlSlot).ToString();
    bool found = false;
    for (int i = node_def->input_size() - 1; i >= 0; --i) {
      const string& node_input = node_def->input(i);
      if (node_input[0] != '^') {
        // Control inputs are at the end. Break when we reach the non-control
        // inputs.
        break;
      }
      if (node_input == input) {
        // Control dependency already exists
        found = true;
        break;
      }
    }
    if (found) {
      continue;
    }
    node_def->add_input(input);
    input_already_exists->push_back(true);
  }
}

void GraphConstructor::AddPrefixToNodeDef(
    const std::vector<bool>& input_already_exists, NodeDef* node_def) {
  if (prefix_.empty()) return;
  node_def->set_name(strings::StrCat(prefix_, node_def->name()));
  // Update names of input nodes
  for (int i = 0; i < node_def->input_size(); ++i) {
    // Skip remapped inputs (which already exist in g_ and are not being
    // imported).
    if (input_already_exists[i]) continue;
    StringPiece input(node_def->input(i));
    if (str_util::ConsumePrefix(&input, "^")) {
      node_def->set_input(i, strings::StrCat("^", prefix_, input));
    } else {
      node_def->set_input(i, strings::StrCat(prefix_, input));
    }
  }
  // Update names of colocation groups
  if (node_def->attr().find(kColocationAttrName) != node_def->attr().end()) {
    auto* list =
        node_def->mutable_attr()->at(kColocationAttrName).mutable_list();
    for (int i = 0; i < list->s_size(); ++i) {
      StringPiece v(list->s(i));
      if (str_util::ConsumePrefix(&v, kColocationGroupPrefix)) {
        list->set_s(i, strings::StrCat(kColocationGroupPrefix, prefix_, v));
      }
    }
  }
}

void GraphConstructor::UniquifyNames(
    const std::vector<bool>& input_already_exists, NodeDef* node_def) {
  if (NameExistsInGraph(node_def->name())) {
    string old_name = node_def->name();
    node_def->set_name(FindUniqueName(node_def->name()));
    uniquified_names_[old_name] = node_def->name();
    // Note that we don't have to update gdef_nodes_ or gdef_prefixes_ with
    // `name` because we guarantee the original NodeDef names are unique,
    // meaning we won't generate this name again.
  }
  for (int i = 0; i < node_def->input_size(); ++i) {
    // Skip remapped inputs (which already exist in g_ and are not being
    // imported).
    if (input_already_exists[i]) continue;
    TensorId id = ParseTensorName(node_def->input(i));
    // We require that UniquifyNames() is called on all NodeDefs in topological
    // order. This guarantees that node_def's inputs will already be uniquified
    // if necessary.
    auto iter = uniquified_names_.find(string(id.first));
    if (iter == uniquified_names_.end()) continue;
    id.first = iter->second;
    node_def->set_input(i, id.ToString());
  }
}

void GraphConstructor::UpdateUniquifiedColocationNames() {
  for (const auto& pair : gdef_nodes_) {
    Node* node = pair.second.node;
    if (node == nullptr) continue;
    std::vector<string> coloc_values;
    Status status =
        GetNodeAttr(node->attrs(), kColocationAttrName, &coloc_values);
    if (!status.ok()) continue;
    bool updated = false;
    for (int i = 0; i < coloc_values.size(); ++i) {
      StringPiece val(coloc_values[i]);
      if (str_util::ConsumePrefix(&val, kColocationGroupPrefix)) {
        auto name_pair = uniquified_names_.find(string(val));
        if (name_pair == uniquified_names_.end()) continue;
        updated = true;
        coloc_values[i] =
            strings::StrCat(kColocationGroupPrefix, name_pair->second);
      }
    }
    if (updated) {
      node->AddAttr(kColocationAttrName, coloc_values);
    }
  }
}

bool GraphConstructor::NameExistsInGraph(StringPiece name) {
  if (existing_nodes_.find(name) != existing_nodes_.end()) return true;
  if (existing_prefixes_.find(name) != existing_prefixes_.end()) return true;
  return false;
}

bool GraphConstructor::NameExistsInGraphDef(StringPiece name) {
  if (gdef_nodes_.find(name) != gdef_nodes_.end()) return true;
  if (gdef_prefixes_.find(name) != gdef_prefixes_.end()) return true;
  return false;
}

string GraphConstructor::FindUniqueName(StringPiece original_name) {
  string name(original_name);
  int count = 0;
  // Check that any generated names don't collide with imported NodeDefs (as
  // well as nodes in g_).
  while (NameExistsInGraph(name) || (count > 0 && NameExistsInGraphDef(name))) {
    name = strings::StrCat(original_name, "_", ++count);
  }
  return name;
}

Status GraphConstructor::IsNodeFullyMapped(const NodeDef& node_def,
                                           bool* is_node_mapped) {
  const OpDef* op_def;
  TF_RETURN_IF_ERROR(g_->op_registry()->LookUpOpDef(node_def.op(), &op_def));
  for (int i = 0; i < op_def->output_arg_size(); ++i) {
    if (opts_.input_map.find({node_def.name(), i}) == opts_.input_map.end()) {
      *is_node_mapped = false;
      return Status::OK();
    }
  }
  *is_node_mapped = true;
  return Status::OK();
}

Status GraphConstructor::Convert() {
  // Import functions before adding nodes, since imported nodes may refer to
  // functions
  if (library_) {
    TF_RETURN_IF_ERROR(g_->AddFunctionLibrary(*library_));
  }

  std::vector<InputInfo> inputs;
  int processed = 0;

  std::vector<bool> input_already_exists;

  // Process the NodeDefs in topological order.
  // (InitFromEdges() sets this up by filling in ready_ with nodes that have no
  // inputs, pending_counts_ with the number of inputs for each node and
  // outputs_ with the outputs of each node).
  while (!ready_.empty()) {
    int o = *ready_.begin();
    ready_.erase(ready_.begin());
    ++processed;
    inputs.clear();
    bool has_data_back_edge = false;

    const NodeDef& original_node_def = *node_defs_[o];
    NodeDef imported_node_def;
    const NodeDef* node_def;

    // input_already_exists[i] is true iff the i-th input of the node we're
    // importing refers to a preexisting node in g_ (i.e. input[i] existed prior
    // to importing node_defs_).  Conversely, input_already_exists[i] is false
    // iff the input refers to a node in node_defs_.
    input_already_exists.clear();
    input_already_exists.resize(original_node_def.input_size(), false);

    if (opts_.importing) {
      if (opts_.skip_mapped_nodes) {
        bool is_node_mapped = false;
        TF_RETURN_IF_ERROR(
            IsNodeFullyMapped(original_node_def, &is_node_mapped));
        if (is_node_mapped) {
          // Skip this node after updating pending_count_ for outputs
          UpdatePendingCountAndReady(o);
          continue;
        }
      }

      // TODO(ashankar): The line below means an additional copy of the
      // NodeDef, which can be expensive if the NodeDef contains large tensors
      // in it. Might make sense to change the API for ImportGraphDef to take
      // a mutable GraphDef* and avoid the copying.
      imported_node_def = original_node_def;
      if (!opts_.input_map.empty()) {
        // Note that input_already_exists can shrink here
        RemapNodeDefInputs(&imported_node_def, &input_already_exists);
      }
      if (!opts_.control_dependencies.empty()) {
        // Note that input_already_exists can grow here
        AddControlDependencies(&imported_node_def, &input_already_exists);
      }
      if (!opts_.default_device.empty() && imported_node_def.device().empty()) {
        imported_node_def.set_device(opts_.default_device);
      }

      node_def = &imported_node_def;
    } else {
      node_def = &original_node_def;
    }

    DCHECK_EQ(node_def->input_size(), input_already_exists.size());
    TF_RETURN_IF_ERROR(ValidateColocationConstraints(*node_def));
    for (int i = 0; i < node_def->input_size(); ++i) {
      TensorId id(ParseTensorName(node_def->input(i)));
      Node* src_node;
      int src_index;

      if (!input_already_exists[i]) {
        // Locate input in newly-imported nodes
        auto iter = gdef_nodes_.find(id.first);
        DCHECK(iter != gdef_nodes_.end()) << id.first;
        src_node = iter->second.node;
        src_index = id.second;
        if (src_node == nullptr) has_data_back_edge = true;
      } else {
        // Input refers to preexistng node in graph
        auto iter = existing_nodes_.find(id.first);
        DCHECK(iter != existing_nodes_.end()) << id.first;
        src_node = iter->second;
        src_index = id.second;
      }

      if (src_node != nullptr && src_index >= src_node->num_outputs()) {
        return errors::InvalidArgument(
            "Node '", node_def->name(), "': Connecting to invalid output ",
            id.second, " of source node ", id.first, " which has ",
            src_node->num_outputs(), " outputs");
      }

      inputs.emplace_back(string(id.first), src_node, src_index);
    }

    if (has_data_back_edge && !IsMerge(*node_def)) {
      return errors::InvalidArgument(
          "Node '", node_def->name(),
          "' had a back edge, but only Merge nodes can have back edges.");
    }

    Node* node;
    if (opts_.importing) {
      if (!prefix_.empty()) {
        AddPrefixToNodeDef(input_already_exists, &imported_node_def);
      }
      // Note: no need to uniquify names if the prefix already guarantees
      // uniqueness
      if (opts_.uniquify_names && (prefix_.empty() || !opts_.uniquify_prefix)) {
        UniquifyNames(input_already_exists, &imported_node_def);
      }
      TF_RETURN_IF_ERROR(ModifyNodeDefForImport(&imported_node_def));
    }
    TF_RETURN_IF_ERROR(MakeNode(*node_def, &node));
    // Use original_node_def so name StringPiece remains valid
    gdef_nodes_[original_node_def.name()].node = node;

    // Add edges from inputs to *node to the graph.
    for (size_t i = 0; i < inputs.size(); ++i) {
      if (inputs[i].node == nullptr) {
        // Record this back edge, which will be added after all nodes
        // are created.
        back_edges_.emplace_back(inputs[i].name, inputs[i].index, node, i);
      } else if (inputs[i].index == Graph::kControlSlot) {
        g_->AddControlEdge(inputs[i].node, node);
      } else {
        TF_RETURN_IF_ERROR(MakeEdge(inputs[i].node, inputs[i].index, node, i));
      }
    }

    TF_RETURN_IF_ERROR(ValidateShape(node));

    // Update pending_count_ for outputs.
    UpdatePendingCountAndReady(o);
  }

  if (processed < node_defs_.size()) {
    LOG(WARNING) << "IN " << __func__ << " " << (node_defs_.size() - processed)
                 << " NODES IN A CYCLE";
    for (int64 i = 0; i < node_defs_.size(); i++) {
      if (pending_count_[i] != 0) {
        LOG(WARNING) << "PENDING: " << SummarizeNodeDef(*node_defs_[i])
                     << " WITH PENDING COUNT = " << pending_count_[i];
      }
    }
    return errors::InvalidArgument(node_defs_.size() - processed,
                                   " nodes in a cycle");
  }

  return Status::OK();
}

Status GraphConstructor::AddBackEdges() {
  // Add the back edges after all nodes are created.
  for (auto e : back_edges_) {
    Node* src_node = gdef_nodes_[e.src_name].node;
    if (e.src_index == Graph::kControlSlot) {
      g_->AddControlEdge(src_node, e.dst_node);
    } else {
      TF_RETURN_IF_ERROR(
          MakeEdge(src_node, e.src_index, e.dst_node, e.dst_index));
    }

    VLOG(2) << "Add back edge: " << src_node->name() << " -> "
            << e.dst_node->name();
  }
  return Status::OK();
}

Status GraphConstructor::UpdateVersionDef() {
  if (versions_ == nullptr) return Status::OK();

  if (!opts_.importing) {
    g_->set_versions(*versions_);
    return Status::OK();
  }
  VersionDef versions = g_->versions();
  versions.set_producer(std::min(versions.producer(), versions_->producer()));
  versions.set_min_consumer(
      std::max(versions.min_consumer(), versions_->min_consumer()));
  if (versions_->bad_consumers_size() > 0) {
    std::set<int> bad(versions.bad_consumers().begin(),
                      versions.bad_consumers().end());
    bad.insert(versions_->bad_consumers().begin(),
               versions_->bad_consumers().end());
    versions.clear_bad_consumers();
    for (int v : bad) {
      versions.add_bad_consumers(v);
    }
  }
  g_->set_versions(versions);
  return Status::OK();
}

Status GraphConstructor::PopulateReturnTensors() {
  if (opts_.return_tensors.empty()) return Status::OK();
  for (const TensorId& id : opts_.return_tensors) {
    auto iter = opts_.input_map.find(id);
    if (iter == opts_.input_map.end()) {
      // Locate id in imported nodes
      auto iter = gdef_nodes_.find(id.first);
      if (iter == gdef_nodes_.end()) {
        return errors::InvalidArgument("Requested return tensor '",
                                       id.ToString(),
                                       "' not found in graph def");
      }
      int num_outputs = iter->second.node->num_outputs();
      if ((id.second < 0 || id.second >= num_outputs) &&
          id.second != Graph::kControlSlot) {
        return errors::InvalidArgument("Invalid return output ", id.second,
                                       " of node '", id.first, "', which has ",
                                       num_outputs, " output(s)");
      }
      return_tensors_->push_back({iter->second.node, id.second});
    } else {
      // id was remapped to existing node
      TensorId remapped_id = iter->second;
      DCHECK_GT(existing_nodes_.count(remapped_id.first), 0);
      Node* node = existing_nodes_[remapped_id.first];
      return_tensors_->push_back({node, remapped_id.second});
    }
  }
  return Status::OK();
}

Status GraphConstructor::PopulateReturnNodes() {
  if (opts_.return_nodes.empty()) return Status::OK();
  for (StringPiece name : opts_.return_nodes) {
    auto iter = gdef_nodes_.find(name);
    if (iter == gdef_nodes_.end()) {
      return errors::InvalidArgument("Requested return node '", name,
                                     "' not found in graph def");
    }
    return_nodes_->push_back(iter->second.node);
  }
  return Status::OK();
}

Status GraphConstructor::PopulateMissingUnusedInputMapKeys() {
  if (missing_unused_input_map_keys_ == nullptr) return Status::OK();
  for (const auto& input_map_pair : opts_.input_map) {
    TensorId key = input_map_pair.first;
    if (used_input_map_keys_.count(key) > 0) continue;

    auto pair = gdef_nodes_.find(key.first);
    if (pair == gdef_nodes_.end()) {
      // key's node doesn't exist in GraphDef
      missing_unused_input_map_keys_->push_back(key);
      continue;
    }

    // Check that key's index is in bounds. Get the number of outputs from the
    // NodeDef, rather than the imported Node, since the Node may not exist if
    // opts_.skip_mapped_nodes is true.
    const NodeDef* node_def = node_defs_[pair->second.gdef_index];
    const OpDef* op_def;
    TF_RETURN_IF_ERROR(g_->op_registry()->LookUpOpDef(node_def->op(), &op_def));
    int num_outputs;
    TF_RETURN_IF_ERROR(NumOutputsForNode(*node_def, *op_def, &num_outputs));
    if (key.second >= num_outputs) {
      // key's index out of bounds
      missing_unused_input_map_keys_->push_back(key);
    }
  }
  return Status::OK();
}

void GraphConstructor::Undo() {
  for (const auto& iter : gdef_nodes_) {
    if (iter.second.node != nullptr) {
      g_->RemoveNode(iter.second.node);
    }
  }
  g_->set_versions(original_versions_);
}

Status GraphConstructor::MakeEdge(Node* src, int output_index, Node* dst,
                                  int input_index) {
  DataType src_out = src->output_type(output_index);
  DataType dst_in = dst->input_type(input_index);
  if (!TypesCompatible(dst_in, src_out)) {
    return errors::InvalidArgument(
        "Input ", input_index, " of node ", dst->name(), " was passed ",
        DataTypeString(src_out), " from ", src->name(), ":", output_index,
        " incompatible with expected ", DataTypeString(dst_in), ".");
  }
  g_->AddEdge(src, output_index, dst, input_index);
  return Status::OK();
}

}  // namespace

Status ConvertGraphDefToGraph(const GraphConstructorOptions& opts,
                              const GraphDef& gdef, Graph* g) {
  ShapeRefiner refiner(gdef.versions().producer(), g->op_registry());
  return GraphConstructor::Construct(
      opts, gdef.node(), &gdef.versions(), &gdef.library(), g, &refiner,
      /*return_tensors=*/nullptr, /*return_nodes=*/nullptr,
      /*missing_unused_input_map_keys=*/nullptr);
}

Status ConvertNodeDefsToGraph(const GraphConstructorOptions& opts,
                              gtl::ArraySlice<NodeDef> nodes, Graph* g) {
  ShapeRefiner refiner(TF_GRAPH_DEF_VERSION, g->op_registry());
  // TODO(irving): Copy will go away once NodeInfo exists
  std::vector<const NodeDef*> node_defs;
  for (const auto& n : nodes) {
    node_defs.push_back(&n);
  }
  return GraphConstructor::Construct(opts, node_defs, nullptr, nullptr, g,
                                     &refiner, /*return_tensors=*/nullptr,
                                     /*return_nodes=*/nullptr,
                                     /*missing_unused_input_map_keys=*/nullptr);
}

Status ImportGraphDef(const ImportGraphDefOptions& opts, const GraphDef& gdef,
                      Graph* g, ShapeRefiner* refiner,
                      ImportGraphDefResults* results) {
  if (!opts.return_tensors.empty()) {
    if (results == nullptr) {
      return errors::InvalidArgument(
          "results argument to ImportGraphDef() must be non-null if "
          "opts.return_tensors is non-empty");
    }
  }

  if (!opts.return_nodes.empty()) {
    if (opts.skip_mapped_nodes) {
      return errors::InvalidArgument(
          "Requesting return_nodes with skip_mapped_nodes set is not currently "
          "supported");
    }
    if (results == nullptr) {
      return errors::InvalidArgument(
          "results argument to ImportGraphDef() must be non-null if "
          "opts.return_nodes is non-empty");
    }
  }

  if (results != nullptr) {
    if (!results->return_tensors.empty() || !results->return_nodes.empty() ||
        !results->missing_unused_input_map_keys.empty()) {
      return errors::InvalidArgument(
          "All fields in results argument to ImportGraphDef() must be empty.");
    }
  }

  ShapeRefiner default_refiner(gdef.versions().producer(), g->op_registry());
  if (refiner == nullptr) {
    refiner = &default_refiner;
  } else {
    // Log a warning if we are importing a GraphDef at an older
    // producer version after already having added non-source/sink
    // nodes to the graph in the past.
    if (gdef.versions().producer() > 0 &&
        gdef.versions().producer() < refiner->graph_def_version() &&
        g->num_nodes() > 2) {
      LOG(WARNING) << "Importing a graph with a lower producer version "
                   << gdef.versions().producer()
                   << " into an existing graph with producer version "
                   << refiner->graph_def_version() << ". Shape inference will "
                   << "have run different parts of the graph with different "
                   << "producer versions.";
    }
  }

  // Set the graph def version of the refiner as the min of the
  // current value and the version from the graph we are about to
  // import.
  //
  // Note: to match Run() semantics, we should re-run shape inference
  // on the entire graph if the producer version has changed.  For now
  // we log the warning above.
  refiner->set_graph_def_version(
      std::min(refiner->graph_def_version(), gdef.versions().producer()));

  if (results == nullptr) {
    return GraphConstructor::Construct(opts, gdef.node(), &gdef.versions(),
                                       &gdef.library(), g, refiner, nullptr,
                                       nullptr, nullptr);
  } else {
    return GraphConstructor::Construct(
        opts, gdef.node(), &gdef.versions(), &gdef.library(), g, refiner,
        &results->return_tensors, &results->return_nodes,
        &results->missing_unused_input_map_keys);
  }
}

void CopyGraph(const Graph& src, Graph* dest) {
  for (Node* n : dest->nodes()) {
    CHECK(n->IsSource() || n->IsSink()) << "*dest must be empty";
  }

  // Copy GraphDef versions
  dest->set_versions(src.versions());

  // Copy the nodes
  std::unordered_map<const Node*, Node*>
      node_map;  // "Node in src" -> "Node in *dest"
  node_map[src.source_node()] = dest->source_node();
  node_map[src.sink_node()] = dest->sink_node();
  for (Node* n : src.op_nodes()) {
    node_map[n] = dest->CopyNode(n);
  }

  // Copy the edges
  for (const Edge* e : src.edges()) {
    Node* src_copy = node_map[e->src()];
    Node* dst_copy = node_map[e->dst()];
    dest->AddEdge(src_copy, e->src_output(), dst_copy, e->dst_input());
  }
}

}  // namespace tensorflow

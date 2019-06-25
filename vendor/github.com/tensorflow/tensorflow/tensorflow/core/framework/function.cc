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

#include "tensorflow/core/framework/function.h"

#include <map>
#include <unordered_map>
#include <utility>
#include <vector>

#include "absl/container/flat_hash_set.h"
#include "absl/strings/str_join.h"
#include "tensorflow/core/framework/common_shape_fns.h"
#include "tensorflow/core/framework/function.pb_text.h"
#include "tensorflow/core/framework/graph.pb.h"
#include "tensorflow/core/framework/node_def.pb.h"
#include "tensorflow/core/framework/node_def_util.h"
#include "tensorflow/core/framework/op.h"
#include "tensorflow/core/graph/graph.h"
#include "tensorflow/core/lib/core/errors.h"
#include "tensorflow/core/lib/gtl/inlined_vector.h"
#include "tensorflow/core/lib/gtl/map_util.h"
#include "tensorflow/core/lib/strings/str_util.h"
#include "tensorflow/core/util/device_name_utils.h"
#include "tensorflow/core/util/equal_graph_def.h"

namespace tensorflow {

// Extracts the actual type from "attr_values" based on its definition
// "arg_def".
//
// If "arg_def" is a N*T type, *is_type_list is set to false, and
// *dtypes is set to be a vector of size N and each element is T.
//
// If "arg_def" is a list(type), *is_type_list is set to true, and
// *dtypes is set to be a vector of types specified in attrs for
// arg_def.
//
// Otherwise (arg_def is a simple type T), *is_type_list is set to
// false, and *dtypes is set to a single element vector, whose only
// element is T.
Status ArgNumType(AttrSlice attrs, const OpDef::ArgDef& arg_def,
                  bool* is_type_list, DataTypeVector* dtypes) {
  dtypes->clear();
  if (!arg_def.type_list_attr().empty()) {
    const AttrValue* v = attrs.Find(arg_def.type_list_attr());
    if (v == nullptr) {
      return errors::NotFound("type attr not found: ",
                              arg_def.type_list_attr());
    }
    *is_type_list = true;
    for (int i = 0; i < v->list().type_size(); ++i) {
      dtypes->push_back(v->list().type(i));
    }
    return Status::OK();
  }

  *is_type_list = false;
  int num = 1;
  if (!arg_def.number_attr().empty()) {
    const AttrValue* v = attrs.Find(arg_def.number_attr());
    if (v == nullptr) {
      return errors::NotFound("type attr not found: ", arg_def.type_attr());
    }
    num = v->i();
  }

  DataType dtype;
  if (arg_def.type() != DT_INVALID) {
    dtype = arg_def.type();
  } else if (arg_def.type_attr().empty()) {
    dtype = DT_INVALID;
  } else {
    const AttrValue* v = attrs.Find(arg_def.type_attr());
    if (v == nullptr) {
      return errors::NotFound("type attr not found: ", arg_def.type_attr());
    }
    dtype = v->type();
  }
  dtypes->resize(num, dtype);
  return Status::OK();
}

namespace {

template <typename T>
void AddAttr(const string& name, const T& val, NodeDef* ndef) {
  SetAttrValue(val, &((*ndef->mutable_attr())[name]));
}

Status ValidateSignatureWithAttrs(const OpDef& sig, AttrSlice attr_values) {
  // attr_values should specify all attrs defined in fdef.
  for (const auto& a : sig.attr()) {
    const AttrValue* v = attr_values.Find(a.name());
    if (!v) {
      return errors::NotFound("Attr ", a.name(), " is not found from ",
                              SummarizeOpDef(sig));
    }
    Status status = AttrValueHasType(*v, a.type());
    if (!status.ok()) {
      errors::AppendToMessage(&status, "for attr '", a.name(), "'");
      return status;
    }
  }

// TODO(josh11b): Enable this code once it works with function gradients.
// Right now the C++ function gradient code assumes it can pass
// all the attrs of the function to the gradient, and any attrs that
// the gradient doesn't care about will be ignored.
#if 0
  if (attr_values.size() != sig.attr_size()) {
    for (const auto& a : attr_values) {
      // TODO(josh11b): Possibly should ignore attrs that start with "_" here?
      bool found = false;
      for (const auto& s : sig.attr()) {
        if (a.first == s.name()) {
          found = true;
          break;
        }
      }
      if (!found) {
        return errors::NotFound("Attr ", a.first, " is not found in ",
                                SummarizeOpDef(sig));
      }
    }
  }
#endif

  return Status::OK();
}

// A helper class for instantiating functions. This contains shared information
// like the resulting graph and node name index.
class FunctionInstantiationHelper {
 public:
  FunctionInstantiationHelper(GetFunctionSignature get_function,
                              InstantiationResult* result)
      : get_function_(std ::move(get_function)), result_(*result) {
    result_.nodes.clear();
  }

  // Builds index for nodes that can be used as node's input arguments.
  Status BuildInputArgIndex(const OpDef::ArgDef& arg_def, AttrSlice attr_values,
                            bool ints_on_device) {
    bool is_type_list;
    DataTypeVector dtypes;
    TF_RETURN_IF_ERROR(
        ArgNumType(attr_values, arg_def, &is_type_list, &dtypes));
    CHECK_GE(dtypes.size(), size_t{1});
    int arg_index = result_.nodes.size();
    TF_RETURN_IF_ERROR(
        AddItem(arg_def.name(), {true, arg_index, 0, is_type_list, dtypes}));
    // Creates dtypes.size() nodes in the graph.
    for (size_t i = 0; i < dtypes.size(); ++i) {
      TF_RETURN_IF_ERROR(AddItem(strings::StrCat(arg_def.name(), ":", i),
                                 {true, arg_index, 0, false, {dtypes[i]}}));
      DCHECK_EQ(arg_index, result_.nodes.size());
      string name = arg_def.name();
      if (dtypes.size() > 1) {
        strings::StrAppend(&name, "_", i);
      }
      NodeDef* gnode = AddNode(name);
      if (ints_on_device && dtypes[i] == DataType::DT_INT32) {
        gnode->set_op(FunctionLibraryDefinition::kDeviceArgOp);
      } else {
        gnode->set_op(FunctionLibraryDefinition::kArgOp);
      }
      AddAttr("T", dtypes[i], gnode);
      AddAttr("index", arg_index, gnode);
      result_.arg_types.push_back(dtypes[i]);
      ++arg_index;
    }
    return Status::OK();
  }

  Status BuildNodeOutputIndex(const NodeDef& node, AttrSlice attrs,
                              const int arg_index) {
    const OpDef* node_sig = nullptr;
    TF_RETURN_IF_ERROR(get_function_(node.op(), &node_sig));
    if (node_sig->output_arg_size() == 0) {
      return AddItem(node.name(), {false, arg_index, 0, false, {}});
    }
    const int num_retval = node_sig->output_arg_size();
    int start = 0;
    bool is_type_list;
    DataTypeVector dtypes;
    for (int i = 0; i < num_retval; ++i) {
      TF_RETURN_IF_ERROR(
          ArgNumType(attrs, node_sig->output_arg(i), &is_type_list, &dtypes));
      // Note that we rely on the backwards-compatibility test enforcing
      // that output_arg(*).name() doesn't change here.
      const string base_name =
          strings::StrCat(node.name(), ":", node_sig->output_arg(i).name());
      TF_RETURN_IF_ERROR(
          AddItem(base_name, {false, arg_index, start, is_type_list, dtypes}));
      for (int j = 0; j < static_cast<int>(dtypes.size()); ++j) {
        TF_RETURN_IF_ERROR(
            AddItem(strings::StrCat(base_name, ":", j),
                    {false, arg_index, start + j, false, {dtypes[j]}}));
      }
      start += dtypes.size();
    }
    return Status::OK();
  }

  Status InstantiateNode(const NodeDef& fnode, AttrSlice attrs) {
    const OpDef* fnode_sig = nullptr;
    TF_CHECK_OK(get_function_(fnode.op(), &fnode_sig));
    NodeDef* gnode = AddNode(fnode.name());
    gnode->set_op(fnode.op());
    gnode->set_device(fnode.device());
    int gnode_idx = nodes_.size() - 1;

    // Input
    const int num_args = fnode_sig->input_arg_size();
    bool is_type_list;  // ignored
    DataTypeVector dtypes;
    int fnode_arg_index = 0;
    for (int i = 0; i < num_args; ++i) {
      TF_RETURN_IF_ERROR(
          ArgNumType(attrs, fnode_sig->input_arg(i), &is_type_list, &dtypes));
      // Consume inputs (indexed by fnode_arg_index) until we have
      // matched each element of dtypes (indexed by j).
      for (size_t j = 0; j < dtypes.size(); ++fnode_arg_index) {
        if (fnode_arg_index >= fnode.input_size()) {
          // Should never happen if we computed dtypes correctly.
          return errors::InvalidArgument(
              "Attempt to access beyond input size: ", fnode_arg_index,
              " >= ", fnode.input_size());
        }
        // Look up the next input.
        const string& input_name = fnode.input(fnode_arg_index);
        const auto* item = GetItemOrNull(input_name);
        if (item == nullptr) {
          return errors::InvalidArgument(
              "input ", input_name,
              " is not found: ", FormatNodeDefForError(fnode));
        }
        if (item->dtypes.size() > dtypes.size() - j) {
          return errors::InvalidArgument("Input ", input_name, " too long for ",
                                         fnode_sig->input_arg(i).name());
        }
        // Match up all the elements of this input (indexed by k) with
        // elements of dtypes (advancing j).
        for (int k = 0; k < item->dtypes.size(); ++k, ++j) {
          if (item->dtypes[k] != dtypes[j]) {
            return errors::InvalidArgument(
                "input ", fnode_sig->input_arg(i).name(), "[", j,
                "] expected type ", DataTypeString(dtypes[j]),
                " != ", DataTypeString(item->dtypes[k]), ", the type of ",
                input_name, "[", k, "]");
          }
          if (item->is_func_arg) {
            AddInput(gnode_idx, item->nid + k, 0);
          } else {
            AddInput(gnode_idx, item->nid, item->idx + k);
          }
        }
      }
    }

    // Control deps.
    for (int i = fnode_arg_index; i < fnode.input_size(); ++i) {
      const string& input = fnode.input(i);
      if (input.empty() || input[0] != '^') {
        return errors::InvalidArgument("Expected input[", i, "] == '", input,
                                       "' to be a control input.");
      }
      int nid = -1;
      const string node_name = input.substr(1);
      const string node_colon = node_name + ":";
      const string node_colon_bound = node_name + ";";
      // index_ is a map sorted lexicographically, so the key we are looking for
      // must lie in the range [node_name, node_colon_bound).
      auto it = index_.lower_bound(node_name);
      while (it != index_.end() && it->first <= node_colon_bound) {
        if (it->first == node_name ||
            tensorflow::str_util::StartsWith(it->first, node_colon)) {
          nid = it->second.nid;
          break;
        }
        ++it;
      }
      if (nid == -1) {
        return errors::InvalidArgument("input[", i, "] == '", input,
                                       "', is not found.");
      }
      AddDep(gnode_idx, nid);
    }

    // Attrs.
    for (const auto& p : attrs) {
      (*gnode->mutable_attr())[p.first] = p.second;
    }

    return Status::OK();
  }

  Status AddReturnNode(
      const OpDef::ArgDef& ret_def, AttrSlice attrs,
      const ::tensorflow::protobuf::Map<string, string>& ret_map,
      bool ints_on_device, int* ret_index) {
    auto ret_iter = ret_map.find(ret_def.name());
    if (ret_iter == ret_map.end()) {
      return errors::InvalidArgument("Return ", ret_def.name(), " missing.");
    }
    bool is_type_list;
    DataTypeVector dtypes;
    TF_RETURN_IF_ERROR(ArgNumType(attrs, ret_def, &is_type_list, &dtypes));
    CHECK_GE(dtypes.size(), size_t{1});
    const auto* item = GetItemOrNull(ret_iter->second);
    if (item == nullptr) {
      return errors::InvalidArgument("Return ", ret_def.name(), " -> ",
                                     ret_iter->second, " is not found.");
    }
    if (dtypes != item->dtypes) {
      return errors::InvalidArgument("Invalid ret types ", ret_def.name(),
                                     " : ", DataTypeVectorString(dtypes),
                                     " vs. ",
                                     DataTypeVectorString(item->dtypes));
    }
    for (size_t i = 0; i < dtypes.size(); ++i) {
      string name = strings::StrCat(ret_def.name(), "_RetVal");
      if (dtypes.size() > 1) {
        strings::StrAppend(&name, "_", i);
      }
      NodeDef* gnode = AddNode(name);
      if (ints_on_device && dtypes[i] == DataType::DT_INT32) {
        gnode->set_op(FunctionLibraryDefinition::kDeviceRetOp);
      } else {
        gnode->set_op(FunctionLibraryDefinition::kRetOp);
      }
      AddInput(nodes_.size() - 1, item->nid, item->idx + i);
      AddAttr("T", dtypes[i], gnode);
      AddAttr("index", (*ret_index)++, gnode);
      result_.ret_types.push_back(dtypes[i]);
    }
    return Status::OK();
  }

  // Adds the actual node inputs to the result graph by converting indexes to
  // the node names.
  void AddNodeInputs() {
    for (int i = 0; i < result_.nodes.size(); i++) {
      NodeInfo& node_info = nodes_[i];
      for (const auto& p : node_info.data_inputs) {
        result_.nodes[i].add_input(Name(p.first, p.second));
      }
      for (int index : node_info.control_inputs) {
        result_.nodes[i].add_input(Dep(index));
      }
    }
  }

 private:
  // This is used to build a small index for all names that can be used as a
  // node's input arguments.
  //
  // If is_func_arg is true, the name is a function's argument.  In
  // this case, the produced graph def has node[nid:nid + dtype.size()].
  //
  // Otherwise, the name is a function body's node return value.  In
  // this case, the produced graph def has one node node[nid] and
  // the node's output index [idx ... idx + num) corresponds to the
  // named outputs.
  //
  // In all cases, "dtype" specifies the data type.
  struct NameInfoItem {
    bool is_func_arg;
    int nid;
    int idx;
    bool is_type_list;
    DataTypeVector dtypes;
  };

  // Adds an item into the input name index.
  Status AddItem(const string& name, const NameInfoItem& item) {
    if (!index_.insert({name, item}).second) {
      return errors::InvalidArgument(
          strings::StrCat("Duplicated ", item.is_func_arg ? "arg" : "ret",
                          " name: "),
          name);
    }
    return Status::OK();
  }

  const NameInfoItem* GetItemOrNull(const string& name) const {
    return gtl::FindOrNull(index_, name);
  }

  string Dep(int node_index) const {
    return strings::StrCat("^", Name(node_index));
  }

  string Name(int node_index) const {
    CHECK_LT(node_index, nodes_.size());
    return nodes_[node_index].name;
  }

  string Name(int node_index, int output_index) const {
    if (output_index == 0) {
      return Name(node_index);
    } else {
      return strings::StrCat(Name(node_index), ":", output_index);
    }
  }

  NodeDef* AddNode(const string& name) {
    result_.nodes.emplace_back();
    NodeDef* gnode = &result_.nodes.back();
    gnode->set_name(name);
    nodes_.push_back({name, {}, {}});
    CHECK_EQ(result_.nodes.size(), nodes_.size());
    return gnode;
  }

  void AddInput(int node_index, int output_node, int output_index) {
    CHECK_LT(node_index, nodes_.size());
    nodes_[node_index].data_inputs.push_back(
        std::make_pair(output_node, output_index));
  }

  void AddDep(int node_index, int dep_index) {
    CHECK_LT(node_index, nodes_.size());
    nodes_[node_index].control_inputs.push_back(dep_index);
  }

  GetFunctionSignature get_function_;
  InstantiationResult& result_;
  // A small index for all names that can be used as a node's input arguments.
  std::map<string, NameInfoItem> index_;
  // This contains information about a node in the new graph including the node
  // names and input nodes' indexes.
  struct NodeInfo {
    string name;
    // Data inputs where <n, k> means arg k of node n.
    std::vector<std::pair<int, int>> data_inputs;
    // Control inputs (dependencies).
    std::vector<int> control_inputs;
  };
  // nodes_[i] is the information about result_.nodes[i].
  std::vector<NodeInfo> nodes_;
};

// Various helpers Print(proto) to print relevant protos to ascii.
string Print(const OpDef::ArgDef& arg) {
  string out;
  strings::StrAppend(&out, arg.name(), ":");
  if (arg.is_ref()) strings::StrAppend(&out, "Ref(");
  if (!arg.number_attr().empty()) {
    strings::StrAppend(&out, arg.number_attr(), "*");
  }
  if (arg.type() != DT_INVALID) {
    strings::StrAppend(&out, DataTypeString(arg.type()));
  } else {
    strings::StrAppend(&out, arg.type_attr());
  }
  if (arg.is_ref()) strings::StrAppend(&out, ")");
  return out;
}

// TODO(josh11b): Merge this with SummarizeAttrValue().
string Print(const AttrValue& attr_value) {
  if (attr_value.value_case() == AttrValue::kType) {
    return DataTypeString(attr_value.type());
  } else if ((attr_value.value_case() == AttrValue::kList) &&
             (attr_value.list().type_size() > 0)) {
    string ret = "{";
    for (int i = 0; i < attr_value.list().type_size(); ++i) {
      if (i > 0) strings::StrAppend(&ret, ", ");
      strings::StrAppend(&ret, DataTypeString(attr_value.list().type(i)));
    }
    strings::StrAppend(&ret, "}");
    return ret;
  } else if (attr_value.value_case() == AttrValue::kFunc) {
    if (attr_value.func().attr_size() == 0) {
      return attr_value.func().name();
    }
    std::vector<string> entries;
    for (auto p : attr_value.func().attr()) {
      entries.push_back(strings::StrCat(p.first, "=", Print(p.second)));
    }
    std::sort(entries.begin(), entries.end());
    return strings::StrCat(attr_value.func().name(), "[",
                           str_util::Join(entries, ", "), "]");
  }
  return SummarizeAttrValue(attr_value);
}

// TODO(josh11b): Merge this with SummarizeNodeDef().
string Print(const NodeDef& n) {
  string out;
  strings::StrAppend(&out, n.name(), " = ", n.op());
  if (n.attr_size() > 0) {
    std::vector<string> entries;
    for (auto& a : n.attr()) {
      entries.push_back(strings::StrCat(a.first, "=", Print(a.second)));
    }
    std::sort(entries.begin(), entries.end());
    // Add a short device string at the end of all attributes.
    if (!n.device().empty()) {
      DeviceNameUtils::ParsedName parsed;
      if (DeviceNameUtils::ParseFullName(n.device(), &parsed)) {
        entries.push_back(
            strings::StrCat("device=", parsed.type, ":", parsed.id));
      } else {
        entries.push_back("device=<FAILED_TO_PARSE>");
      }
    }
    strings::StrAppend(&out, "[", str_util::Join(entries, ", "), "]");
  }
  strings::StrAppend(&out, "(");
  std::vector<StringPiece> dat;
  std::vector<string> dep;
  for (StringPiece s : n.input()) {
    if (str_util::ConsumePrefix(&s, "^")) {
      dep.emplace_back(s);
    } else {
      dat.push_back(s);
    }
  }
  strings::StrAppend(&out, str_util::Join(dat, ", "), ")");
  if (!dep.empty()) {
    strings::StrAppend(&out, " @ ", str_util::Join(dep, ", "));
  }
  return out;
}

string Print(const FunctionDef& fdef) {
  string out;
  const OpDef& sig = fdef.signature();
  strings::StrAppend(&out, "\n", sig.name());
  if (sig.attr_size() > 0) {
    strings::StrAppend(&out, "[");
    for (int i = 0; i < sig.attr_size(); ++i) {
      const auto& a = sig.attr(i);
      if (i > 0) strings::StrAppend(&out, ", ");
      if (a.type() == "type") {
        strings::StrAppend(&out, a.name(), ":", Print(a.allowed_values()));
      } else {
        strings::StrAppend(&out, a.name(), ":", a.type());
      }
    }
    strings::StrAppend(&out, "]");
  }
  strings::StrAppend(&out, "(");
  for (int i = 0; i < sig.input_arg_size(); ++i) {
    if (i > 0) strings::StrAppend(&out, ", ");
    strings::StrAppend(&out, Print(sig.input_arg(i)));
  }
  strings::StrAppend(&out, ") -> (");
  for (int i = 0; i < sig.output_arg_size(); ++i) {
    if (i > 0) strings::StrAppend(&out, ", ");
    strings::StrAppend(&out, Print(sig.output_arg(i)));
  }
  strings::StrAppend(&out, ") {\n");
  for (const auto& n : fdef.node_def()) {
    strings::StrAppend(&out, "  ", Print(n), "\n");
  }
  for (const auto& r : fdef.ret()) {
    strings::StrAppend(&out, "  return ", r.first, " = ", r.second, "\n");
  }
  strings::StrAppend(&out, "}\n");
  return out;
}

string Print(gtl::ArraySlice<const NodeDef*> nodes) {
  std::vector<const NodeDef*> arg;
  std::vector<const NodeDef*> ret;
  std::vector<const NodeDef*> body;
  for (const NodeDef* n : nodes) {
    if (n->op() == FunctionLibraryDefinition::kArgOp ||
        n->op() == FunctionLibraryDefinition::kDeviceArgOp) {
      arg.push_back(n);
    } else if (n->op() == FunctionLibraryDefinition::kRetOp ||
               n->op() == FunctionLibraryDefinition::kDeviceRetOp) {
      ret.push_back(n);
    } else {
      body.push_back(n);
    }
  }
  auto comp = [](const NodeDef* x, const NodeDef* y) {
    int xi;
    TF_CHECK_OK(GetNodeAttr(*x, "index", &xi));
    int yi;
    TF_CHECK_OK(GetNodeAttr(*y, "index", &yi));
    return xi < yi;
  };
  std::sort(arg.begin(), arg.end(), comp);
  std::sort(ret.begin(), ret.end(), comp);
  string out;
  strings::StrAppend(&out, "\n(");
  auto get_type_and_device = [](const NodeDef& n) {
    DataType dt;
    if (!GetNodeAttr(n, "T", &dt).ok()) {
      dt = DT_INVALID;
    }
    if (!n.device().empty()) {
      DeviceNameUtils::ParsedName parsed;
      if (DeviceNameUtils::ParseFullName(n.device(), &parsed)) {
        return strings::StrCat(DataTypeString(dt), "@", parsed.type, ":",
                               parsed.id);
      } else {
        return strings::StrCat(DataTypeString(dt), "@",
                               "<FAILED_TO_PARSE_DEVICE>");
      }
    }
    return DataTypeString(dt);
  };
  for (size_t i = 0; i < arg.size(); ++i) {
    const NodeDef* n = arg[i];
    if (i > 0) strings::StrAppend(&out, ", ");
    CHECK_GE(n->attr_size(), 2);
    strings::StrAppend(&out, n->name(), ":", get_type_and_device(*n));
  }
  strings::StrAppend(&out, ") -> (");
  for (size_t i = 0; i < ret.size(); ++i) {
    const NodeDef* n = ret[i];
    if (i > 0) strings::StrAppend(&out, ", ");
    CHECK_LE(2, n->attr_size());

    // The _RetVal op should have a unique non-control input. We assert that
    // here and add it to the output.
    bool found_non_control_input = false;
    for (const string& input : n->input()) {
      if (!input.empty() && input[0] != '^') {
        DCHECK_EQ(found_non_control_input, false)
            << "RetVal node has more than one non-control input: "
            << absl::StrJoin(n->input(), ", ");
        strings::StrAppend(&out, n->input(0), ":", get_type_and_device(*n));
        found_non_control_input = true;
      }
    }
    DCHECK_EQ(found_non_control_input, true)
        << "RetVal did not have any non-control inputs: "
        << absl::StrJoin(n->input(), ", ");
  }
  strings::StrAppend(&out, ") {\n");
  for (size_t i = 0; i < body.size(); ++i) {
    strings::StrAppend(&out, "  ", Print(*body[i]), "\n");
  }
  strings::StrAppend(&out, "}\n");
  return out;
}

Status AddDefaultAttrs(const string& op,
                       const GetFunctionSignature& get_function,
                       AttrValueMap* attrs) {
  const OpDef* op_def = nullptr;
  TF_RETURN_IF_ERROR(get_function(op, &op_def));
  AttrSlice attr_slice(attrs);
  for (const auto& attr_def : op_def->attr()) {
    if (attr_def.has_default_value() && !attr_slice.Find(attr_def.name())) {
      if (!attrs->insert({attr_def.name(), attr_def.default_value()}).second) {
        return errors::Internal("Somehow duplicated: ", attr_def.name());
      }
    }
  }
  return Status::OK();
}

}  // end namespace

// TODO(shikharagarwal): Transmit original node names correctly in file.
Status InstantiateFunction(const FunctionDef& fdef, AttrSlice attr_values,
                           GetFunctionSignature get_function,
                           InstantiationResult* result) {
  VLOG(3) << "Instantiation Function: " << Print(fdef);

  const OpDef& sig = fdef.signature();
  TF_RETURN_IF_ERROR(ValidateSignatureWithAttrs(sig, attr_values));

  bool ints_on_device = fdef.attr().count("experimental_ints_on_device") != 0 &&
                        fdef.attr().at("experimental_ints_on_device").b();

  FunctionInstantiationHelper helper(get_function, result);
  Status s;
  for (const OpDef::ArgDef& arg_def : sig.input_arg()) {
    s = helper.BuildInputArgIndex(arg_def, attr_values, ints_on_device);
    if (!s.ok()) {
      errors::AppendToMessage(&s, "In ", Print(arg_def));
      return s;
    }
  }

  auto substitute = [attr_values](StringPiece name, AttrValue* val) {
    if (const AttrValue* v = attr_values.Find(name)) {
      *val = *v;
      return true;
    }
    return false;
  };

  // Makes a copy of all attrs in fdef and substitutes placeholders.
  // After this step, every attr is bound to a concrete value.
  std::vector<AttrValueMap> node_attrs;
  node_attrs.resize(fdef.node_def_size());
  for (int i = 0; i < fdef.node_def_size(); ++i) {
    for (auto attr : fdef.node_def(i).attr()) {
      if (!SubstitutePlaceholders(substitute, &attr.second)) {
        return errors::InvalidArgument("Failed to bind all placeholders in ",
                                       SummarizeAttrValue(attr.second));
      }
      if (!node_attrs[i].insert(attr).second) {
        return errors::Internal("Somehow duplicated: ", attr.first);
      }
    }
    TF_RETURN_IF_ERROR(
        AddDefaultAttrs(fdef.node_def(i).op(), get_function, &node_attrs[i]));
  }

  for (int i = 0; i < fdef.node_def_size(); ++i) {
    s = helper.BuildNodeOutputIndex(fdef.node_def(i), AttrSlice(&node_attrs[i]),
                                    result->nodes.size() + i);
    if (!s.ok()) {
      errors::AppendToMessage(&s, "In ",
                              FormatNodeDefForError(fdef.node_def(i)));
      return s;
    }
  }
  // Emits one node for each fdef.node_def.
  for (int i = 0; i < fdef.node_def_size(); ++i) {
    s = helper.InstantiateNode(fdef.node_def(i), AttrSlice(&node_attrs[i]));
    if (!s.ok()) {
      errors::AppendToMessage(&s, "In ",
                              FormatNodeDefForError(fdef.node_def(i)));
      return s;
    }
  }

  // Emits nodes for the function's return values.
  int ret_index = 0;
  for (const OpDef::ArgDef& ret_def : sig.output_arg()) {
    s = helper.AddReturnNode(ret_def, attr_values, fdef.ret(), ints_on_device,
                             &ret_index);
    if (!s.ok()) {
      errors::AppendToMessage(&s, "In function output ", Print(ret_def));
      return s;
    }
  }

  // Adds the actual node inputs using the input indexes.
  helper.AddNodeInputs();

  return Status::OK();
}

string DebugString(const FunctionDef& func_def) { return Print(func_def); }

string DebugString(const GraphDef& instantiated_func_def) {
  std::vector<const NodeDef*> ptrs;
  for (const NodeDef& n : instantiated_func_def.node()) {
    ptrs.push_back(&n);
  }
  return Print(ptrs);
}

string DebugString(gtl::ArraySlice<NodeDef> instantiated_func_nodes) {
  std::vector<const NodeDef*> ptrs;
  for (const NodeDef& n : instantiated_func_nodes) {
    ptrs.push_back(&n);
  }
  return Print(ptrs);
}

string DebugStringWhole(const GraphDef& gdef) {
  string ret;
  for (const auto& fdef : gdef.library().function()) {
    strings::StrAppend(&ret, Print(fdef));
  }
  strings::StrAppend(&ret, "\n");
  for (const auto& ndef : gdef.node()) {
    strings::StrAppend(&ret, Print(ndef), "\n");
  }
  return ret;
}

namespace {

// Returns the name -> attr mapping of fdef's attrs that have a value set. In
// Python, it's possible to access unset attrs, which returns a default value
// and adds an unset attr to the map.
std::map<string, AttrValue> GetSetAttrs(const FunctionDef& fdef) {
  std::map<string, AttrValue> set_attrs;
  for (auto pair : fdef.attr()) {
    if (pair.second.value_case() != AttrValue::VALUE_NOT_SET) {
      set_attrs[pair.first] = pair.second;
    }
  }
  return set_attrs;
}

}  // end namespace

bool FunctionDefsEqual(const FunctionDef& f1, const FunctionDef& f2) {
  if (!OpDefEqual(f1.signature(), f2.signature())) return false;

  std::map<string, AttrValue> f1_attrs = GetSetAttrs(f1);
  std::map<string, AttrValue> f2_attrs = GetSetAttrs(f2);
  if (f1_attrs.size() != f2_attrs.size()) return false;
  for (auto iter1 : f1_attrs) {
    auto iter2 = f2_attrs.find(iter1.first);
    if (iter2 == f2_attrs.end()) return false;
    if (!AreAttrValuesEqual(iter1.second, iter2->second)) return false;
  }

  if (!EqualRepeatedNodeDef(f1.node_def(), f2.node_def(), nullptr)) {
    return false;
  }

  std::map<string, string> ret1(f1.ret().begin(), f1.ret().end());
  std::map<string, string> ret2(f2.ret().begin(), f2.ret().end());
  if (ret1 != ret2) return false;

  return true;
}

uint64 FunctionDefHash(const FunctionDef& fdef) {
  // signature
  uint64 h = OpDefHash(fdef.signature());

  // attrs
  std::map<string, AttrValue> attrs = GetSetAttrs(fdef);
  for (const auto& p : attrs) {
    h = Hash64(p.first.data(), p.first.size(), h);
    h = Hash64Combine(AttrValueHash(p.second), h);
  }

  // node defs
  h = Hash64Combine(RepeatedNodeDefHash(fdef.node_def()), h);

  // output names
  std::map<string, string> ret(fdef.ret().begin(), fdef.ret().end());
  for (const auto& p : ret) {
    h = Hash64(p.first.data(), p.first.size(), h);
    h = Hash64(p.second.data(), p.second.size(), h);
  }

  return h;
}

static constexpr const char* const kExecutorAttr = "_executor";

/* static */
string FunctionLibraryRuntime::ExecutorType(const InstantiateOptions& options,
                                            AttrSlice attrs) {
  if (!options.executor_type.empty()) {
    return options.executor_type;
  } else if (const AttrValue* executor_attr = attrs.Find(kExecutorAttr)) {
    return executor_attr->s();
  } else {
    return string();
  }
}

string Canonicalize(const string& funcname, AttrSlice attrs,
                    const FunctionLibraryRuntime::InstantiateOptions& options) {
  std::vector<string> entries;
  entries.reserve(options.target.empty() ? attrs.size() : (attrs.size() + 1));
  for (auto p : attrs) {
    if (p.first != kExecutorAttr) {
      entries.push_back(strings::StrCat(p.first, "=", Print(p.second)));
    }
  }
  if (!options.target.empty()) {
    entries.push_back(
        strings::StrCat("_target", "=", str_util::CEscape(options.target)));
  }
  if (options.overlay_lib) {
    entries.push_back(strings::StrCat(
        "_overlay_lib", "=", reinterpret_cast<uintptr_t>(options.overlay_lib)));
  }
  if (!options.state_handle.empty()) {
    entries.push_back(
        strings::StrCat("_state_handle", "=", options.state_handle));
  }
  string executor_type = FunctionLibraryRuntime::ExecutorType(options, attrs);
  if (!executor_type.empty()) {
    entries.push_back(strings::StrCat(kExecutorAttr, "=", executor_type));
  }
  std::sort(entries.begin(), entries.end());
  return strings::StrCat(funcname, "[", str_util::Join(entries, ","), "]");
}

FunctionCallFrame::FunctionCallFrame(DataTypeSlice arg_types,
                                     DataTypeSlice ret_types)
    : arg_types_(arg_types.begin(), arg_types.end()),
      ret_types_(ret_types.begin(), ret_types.end()) {
  args_.resize(arg_types_.size());
  rets_.resize(ret_types_.size());
}

FunctionCallFrame::~FunctionCallFrame() {}

Status FunctionCallFrame::SetArgs(gtl::ArraySlice<Tensor> args) {
  // Input type checks.
  if (args.size() != arg_types_.size()) {
    return errors::InvalidArgument("Expects ", arg_types_.size(),
                                   " arguments, but ", args.size(),
                                   " is provided");
  }
  for (size_t i = 0; i < args.size(); ++i) {
    if (arg_types_[i] != args[i].dtype()) {
      return errors::InvalidArgument(
          "Expects arg[", i, "] to be ", DataTypeString(arg_types_[i]), " but ",
          DataTypeString(args[i].dtype()), " is provided");
    }
    args_[i] = args[i];
  }
  return Status::OK();
}

Status FunctionCallFrame::GetRetvals(std::vector<Tensor>* rets) const {
  rets->clear();
  rets->reserve(rets_.size());
  for (size_t i = 0; i < rets_.size(); ++i) {
    const auto& item = rets_[i];
    if (item.has_val) {
      rets->push_back(item.val);
    } else {
      return errors::Internal("Retval[", i, "] does not have value");
    }
  }
  return Status::OK();
}

Status FunctionCallFrame::ConsumeRetvals(std::vector<Tensor>* rets,
                                         bool allow_dead_tensors) {
  rets->clear();
  rets->reserve(rets_.size());
  for (size_t i = 0; i < rets_.size(); ++i) {
    if (rets_[i].has_val) {
      rets->emplace_back(std::move(rets_[i].val));
    } else if (allow_dead_tensors) {
      rets->emplace_back();
    } else {
      return errors::Internal("Retval[", i, "] does not have value");
    }
  }
  return Status::OK();
}

Status FunctionCallFrame::GetArg(int index, Tensor* val) const {
  if (index < 0 || static_cast<size_t>(index) >= args_.size()) {
    return errors::InvalidArgument("GetArg ", index, " is not within [0, ",
                                   args_.size(), ")");
  }
  *val = args_[index];
  return Status::OK();
}

Status FunctionCallFrame::SetRetval(int index, const Tensor& val) {
  if (index < 0 || static_cast<size_t>(index) >= rets_.size()) {
    return errors::InvalidArgument("SetRetval ", index, " is not within [0, ",
                                   rets_.size(), ")");
  }
  if (val.dtype() != ret_types_[index]) {
    return errors::InvalidArgument(
        "Expects ret[", index, "] to be ", DataTypeString(ret_types_[index]),
        ", but ", DataTypeString(val.dtype()), " is provided.");
  }
  Retval* item = &rets_[index];
  if (!item->has_val) {
    item->has_val = true;
    item->val = val;
  } else {
    return errors::Internal("Retval[", index, "] has already been set.");
  }
  return Status::OK();
}

FunctionLibraryDefinition::FunctionDefAndOpRegistration::
    FunctionDefAndOpRegistration(const FunctionDef& fdef_in)
    : fdef(fdef_in),
      // Exact shape inference for functions is handled by ShapeRefiner.
      // Here we pass a dummy shape inference function for legacy code paths.
      op_registration_data(fdef.signature(), shape_inference::UnknownShape,
                           true /* is_function */) {}

FunctionLibraryDefinition::FunctionLibraryDefinition(
    const FunctionLibraryDefinition& other)
    : default_registry_(other.default_registry_) {
  tf_shared_lock l(other.mu_);
  for (const auto& it : other.function_defs_) {
    TF_CHECK_OK(AddFunctionDef(it.second->fdef));
  }
  func_grad_ = other.func_grad_;
}

FunctionLibraryDefinition::FunctionLibraryDefinition(
    const OpRegistryInterface* default_registry,
    const FunctionDefLibrary& def_lib)
    : default_registry_(default_registry),
      function_defs_(def_lib.function_size()) {
  for (const auto& fdef : def_lib.function()) {
    // The latter function definition wins.
    auto& ptr = function_defs_[fdef.signature().name()];
    ptr.reset(new FunctionDefAndOpRegistration(fdef));
  }
  for (const auto& grad : def_lib.gradient()) {
    func_grad_[grad.function_name()] = grad.gradient_func();
  }
}

FunctionLibraryDefinition::~FunctionLibraryDefinition() {}

bool FunctionLibraryDefinition::Contains(const string& func) const {
  tf_shared_lock l(mu_);
  return function_defs_.find(func) != function_defs_.end();
}

const FunctionDef* FunctionLibraryDefinition::Find(const string& func) const {
  tf_shared_lock l(mu_);
  return FindHelper(func);
}

const FunctionDef* FunctionLibraryDefinition::FindHelper(
    const string& func) const {
  auto iter = function_defs_.find(func);
  if (iter == function_defs_.end()) {
    return nullptr;
  } else {
    return &iter->second->fdef;
  }
}

Status FunctionLibraryDefinition::AddFunctionDef(const FunctionDef& fdef) {
  mutex_lock l(mu_);
  bool added;
  return AddFunctionDefHelper(fdef, &added);
}

Status FunctionLibraryDefinition::AddFunctionDefHelper(const FunctionDef& fdef,
                                                       bool* added) {
  *added = false;
  std::unique_ptr<FunctionDefAndOpRegistration>* entry =
      &function_defs_[fdef.signature().name()];
  if (*entry != nullptr) {
    if (!FunctionDefsEqual((*entry)->fdef, fdef)) {
      return errors::InvalidArgument(
          "Cannot add function '", fdef.signature().name(),
          "' because a different function with the same name already "
          "exists.");
    }
    // Ignore duplicate FunctionDefs
    return Status::OK();
  }
  const OpDef* op_def;
  if (default_registry_->LookUpOpDef(fdef.signature().name(), &op_def).ok()) {
    return errors::InvalidArgument(
        "Cannot add function '", fdef.signature().name(),
        "' because an op with the same name already exists.");
  }
  entry->reset(new FunctionDefAndOpRegistration(fdef));
  *added = true;
  return Status::OK();
}

Status FunctionLibraryDefinition::AddGradientDef(const GradientDef& grad) {
  mutex_lock l(mu_);
  bool added;
  return AddGradientDefHelper(grad, &added);
}

Status FunctionLibraryDefinition::AddGradientDefHelper(const GradientDef& grad,
                                                       bool* added) {
  *added = false;
  string* entry = &func_grad_[grad.function_name()];
  if (!entry->empty()) {
    if (*entry != grad.gradient_func()) {
      return errors::InvalidArgument(
          "Cannot assign gradient function '", grad.gradient_func(), "' to '",
          grad.function_name(), "' because it already has gradient function ",
          "'", *entry, "'");
    }
    // Ignore duplicate GradientDefs
    return Status::OK();
  }
  *entry = grad.gradient_func();
  *added = true;
  return Status::OK();
}

Status FunctionLibraryDefinition::AddLibrary(
    const FunctionLibraryDefinition& other) {
  // Clone `other` to ensure thread-safety (grabbing `other`'s lock for
  // the duration of the function could lead to deadlock).
  FunctionLibraryDefinition clone(other);
  mutex_lock l(mu_);
  // Remember the funcs and grads that we added successfully so that
  // we can roll them back on error.
  std::vector<string> funcs;
  std::vector<string> funcs_with_grads;
  Status s;
  bool added;
  for (auto iter : clone.function_defs_) {
    s = AddFunctionDefHelper(iter.second->fdef, &added);
    if (!s.ok()) {
      Remove(funcs, funcs_with_grads);
      return s;
    }
    if (added) {
      funcs.push_back(iter.second->fdef.signature().name());
    }
  }
  for (auto iter : clone.func_grad_) {
    GradientDef grad;
    grad.set_function_name(iter.first);
    grad.set_gradient_func(iter.second);
    s = AddGradientDefHelper(grad, &added);
    if (!s.ok()) {
      Remove(funcs, funcs_with_grads);
      return s;
    }
    if (added) {
      funcs_with_grads.push_back(grad.function_name());
    }
  }
  return Status::OK();
}

Status FunctionLibraryDefinition::AddLibrary(
    const FunctionDefLibrary& lib_def) {
  // Remember the funcs and grads that we added successfully so that
  // we can roll them back on error.
  mutex_lock l(mu_);
  std::vector<string> funcs;
  std::vector<string> funcs_with_grads;
  Status s;
  bool added;
  for (const FunctionDef& fdef : lib_def.function()) {
    s = AddFunctionDefHelper(fdef, &added);
    if (!s.ok()) {
      Remove(funcs, funcs_with_grads);
      return s;
    }
    if (added) {
      funcs.push_back(fdef.signature().name());
    }
  }
  for (const GradientDef& grad : lib_def.gradient()) {
    s = AddGradientDefHelper(grad, &added);
    if (!s.ok()) {
      Remove(funcs, funcs_with_grads);
      return s;
    }
    if (added) {
      funcs_with_grads.push_back(grad.function_name());
    }
  }
  return Status::OK();
}

Status FunctionLibraryDefinition::ReplaceFunction(const string& func,
                                                  const FunctionDef& fdef) {
  mutex_lock l(mu_);
  bool added;
  TF_RETURN_IF_ERROR(RemoveFunctionHelper(func));
  TF_RETURN_IF_ERROR(AddFunctionDefHelper(fdef, &added));
  return Status::OK();
}

Status FunctionLibraryDefinition::ReplaceGradient(const GradientDef& grad) {
  mutex_lock l(mu_);
  bool added;
  TF_RETURN_IF_ERROR(RemoveGradient(grad.function_name()));
  TF_RETURN_IF_ERROR(AddGradientDefHelper(grad, &added));
  return Status::OK();
}

Status FunctionLibraryDefinition::RemoveFunction(const string& func) {
  mutex_lock l(mu_);
  TF_RETURN_IF_ERROR(RemoveFunctionHelper(func));
  return Status::OK();
}

Status FunctionLibraryDefinition::RemoveFunctionHelper(const string& func) {
  const auto& i = function_defs_.find(func);
  if (i == function_defs_.end()) {
    return errors::InvalidArgument("Tried to remove non-existent function ",
                                   func);
  }
  function_defs_.erase(i);
  return Status::OK();
}

Status FunctionLibraryDefinition::RemoveGradient(const string& func) {
  const auto& i = func_grad_.find(func);
  if (i == func_grad_.end()) {
    return errors::InvalidArgument("Tried to remove non-existent gradient ",
                                   func);
  }
  func_grad_.erase(i);
  return Status::OK();
}

void FunctionLibraryDefinition::Remove(
    const std::vector<string>& funcs,
    const std::vector<string>& funcs_with_grads) {
  for (const string& f : funcs) {
    Status s = RemoveFunctionHelper(f);
    DCHECK(s.ok());
  }
  for (const string& f : funcs_with_grads) {
    Status s = RemoveGradient(f);
    DCHECK(s.ok());
  }
}

string FunctionLibraryDefinition::FindGradient(const string& func) const {
  tf_shared_lock l(mu_);
  return gtl::FindWithDefault(func_grad_, func, "");
}

string FunctionLibraryDefinition::FindGradientHelper(const string& func) const {
  return gtl::FindWithDefault(func_grad_, func, "");
}

Status FunctionLibraryDefinition::LookUp(
    const string& op, const OpRegistrationData** op_reg_data) const {
  tf_shared_lock l(mu_);
  auto iter = function_defs_.find(op);
  if (iter != function_defs_.end()) {
    *op_reg_data = &iter->second->op_registration_data;
    return Status::OK();
  }
  return default_registry_->LookUp(op, op_reg_data);
}

string FunctionLibraryDefinition::UniqueFunctionName(StringPiece prefix) const {
  tf_shared_lock l(mu_);
  int index = 0;
  string name = strings::StrCat(prefix, index);
  while (function_defs_.find(name) != function_defs_.end()) {
    ++index;
    name = strings::StrCat(prefix, index);
  }
  return name;
}

const FunctionDef* FunctionLibraryDefinition::GetAttrImpl(
    const NodeDef& ndef) const {
  if (ndef.op() != kGradientOp) {
    // If 'ndef' calls a function and the function's def has the attr,
    // returns it.
    return Find(ndef.op());
  }

  // If ndef is SymbolicGradient[f=Foo], we use Foo's gradient or
  // Foo's attributes.
  const NameAttrList* forward_func_attrs;
  if (!GetNodeAttr(ndef, kFuncAttr, &forward_func_attrs).ok()) {
    return nullptr;
  }
  const string& func_name = forward_func_attrs->name();
  {
    tf_shared_lock l(mu_);
    const string& grad_name = FindGradientHelper(func_name);
    // If 'func' has a user-defined gradient function, uses the grad
    // function's attrs to see if noinline is specified. Otherwise,
    // uses func's attrs.
    if (!grad_name.empty()) {
      return FindHelper(grad_name);
    }
    return FindHelper(func_name);
  }
}

std::vector<string> FunctionLibraryDefinition::ListFunctionNames() const {
  std::vector<string> function_names;
  tf_shared_lock l(mu_);
  function_names.reserve(function_defs_.size());
  for (const auto& it : function_defs_) {
    function_names.emplace_back(it.first);
  }
  return function_names;
}

FunctionDefLibrary FunctionLibraryDefinition::ToProto() const {
  FunctionDefLibrary lib;
  tf_shared_lock l(mu_);
  for (const auto& f : function_defs_) {
    *lib.add_function() = f.second->fdef;
  }
  for (const auto& g : func_grad_) {
    GradientDef* gd = lib.add_gradient();
    gd->set_function_name(g.first);
    gd->set_gradient_func(g.second);
  }
  return lib;
}

template <typename T>
Status FunctionLibraryDefinition::GetAttr(const NodeDef& ndef,
                                          const string& attr, T* value) const {
  const FunctionDef* fdef = GetAttrImpl(ndef);
  if (fdef && GetNodeAttr(AttrSlice(&fdef->attr()), attr, value).ok()) {
    return Status::OK();
  }
  return errors::InvalidArgument("Attr ", attr, " is not defined.");
}

template <typename T>
Status FunctionLibraryDefinition::GetAttr(const Node& node, const string& attr,
                                          T* value) const {
  return GetAttr(node.def(), attr, value);
}

#define GET_ATTR(T)                                                            \
  template Status FunctionLibraryDefinition::GetAttr(const Node&,              \
                                                     const string&, T*) const; \
  template Status FunctionLibraryDefinition::GetAttr(const NodeDef&,           \
                                                     const string&, T*) const;
GET_ATTR(string)
GET_ATTR(bool)
#undef GET_ATTR

namespace {

constexpr char kExperimentalApiImplements[] = "experimental_api_implements";

absl::flat_hash_set<string> ReachableFunctions(
    const FunctionLibraryDefinition& flib,
    const protobuf::RepeatedPtrField<NodeDef>& nodes) {
  // Functions that are reachable from the graph.
  absl::flat_hash_set<string> reachable_funcs;

  // For any functions, if it has attribute "experimental_api_implements" =
  // "some_interface" and it is reachable, then it means any other
  // function with same attribute name and value could also be potentially
  // reachable, eg via experimental_implementation_selector swapping the
  // nodedef.
  absl::flat_hash_set<string> reachable_api_interface;

  // Functions might be reachable from the nested function calls, so we keep a
  // queue of functions that we have to check.
  gtl::InlinedVector<const FunctionDef*, 4> func_queue;

  // Add reachable and not already processed functions to the functions queue.
  const auto add_to_func_queue = [&](const string& func_name) {
    const FunctionDef* func = flib.Find(func_name);
    if (func && reachable_funcs.find(func_name) == reachable_funcs.end()) {
      func_queue.push_back(func);
    }
  };

  // Add all the functions that are reachable from the given node to the queue.
  const auto process_node = [&](const NodeDef& node) {
    // Node itself can be a call to the function.
    add_to_func_queue(node.op());

    // Or node can have an attribute referencing a function.
    for (const auto& attr : node.attr()) {
      const auto& attr_value = attr.second;

      // 1. AttrValue.func
      if (attr_value.has_func()) {
        add_to_func_queue(attr_value.func().name());
      }

      // 2. AttrValue.ListValue.func
      if (attr_value.has_list()) {
        for (const auto& func : attr_value.list().func()) {
          add_to_func_queue(func.name());
        }
      }
    }
  };

  // Add all functions that are directly called from the optimized graph.
  std::for_each(nodes.begin(), nodes.end(), process_node);

  // Process all reachable functions.
  while (!func_queue.empty()) {
    const FunctionDef* func = func_queue.back();
    func_queue.pop_back();

    const string& func_name = func->signature().name();
    reachable_funcs.insert(func_name);

    const auto attr_it = func->attr().find(kExperimentalApiImplements);
    if (attr_it != func->attr().end()) {
      reachable_api_interface.insert(attr_it->second.s());
    }

    // Find all the functions called from the function body.
    const auto& func_body = func->node_def();
    std::for_each(func_body.begin(), func_body.end(), process_node);

    // Check if the function has a registered gradient.
    const string grad_func_name = flib.FindGradient(func_name);
    if (!grad_func_name.empty()) add_to_func_queue(grad_func_name);
  }

  for (const auto& func_name : flib.ListFunctionNames()) {
    const auto& func_def = flib.Find(func_name);
    const auto attr_it = func_def->attr().find(kExperimentalApiImplements);
    if (attr_it != func_def->attr().end()) {
      if (reachable_api_interface.contains(attr_it->second.s())) {
        reachable_funcs.insert(func_name);
      }
    }
  }

  return reachable_funcs;
}

FunctionLibraryDefinition ReachableFunctionLibraryDefinition(
    const FunctionLibraryDefinition& flib,
    const protobuf::RepeatedPtrField<NodeDef>& nodes) {
  absl::flat_hash_set<string> reachable_funcs = ReachableFunctions(flib, nodes);

  FunctionLibraryDefinition reachable_flib(flib.default_registry(),
                                           FunctionDefLibrary());

  for (const string& func_name : reachable_funcs) {
    const FunctionDef* func = flib.Find(func_name);
    DCHECK_NE(func, nullptr);
    // That should never fail, because we copy functions from valid flib and use
    // the same default registry.
    const Status added = reachable_flib.AddFunctionDef(*func);
    DCHECK(added.ok());

    const string grad_func_name = flib.FindGradient(func_name);
    if (!grad_func_name.empty()) {
      GradientDef grad;
      grad.set_function_name(func_name);
      grad.set_gradient_func(grad_func_name);
      // It can only fail if function already has a gradient function.
      const Status added_grad = reachable_flib.AddGradientDef(grad);
      DCHECK(added_grad.ok());
    }
  }

  return reachable_flib;
}

}  // namespace

FunctionLibraryDefinition FunctionLibraryDefinition::ReachableDefinitions(
    const GraphDef& graph) const {
  return ReachableFunctionLibraryDefinition(*this, graph.node());
}

FunctionLibraryDefinition FunctionLibraryDefinition::ReachableDefinitions(
    const FunctionDef& func) const {
  return ReachableFunctionLibraryDefinition(*this, func.node_def());
}

void FunctionDefHelper::AttrValueWrapper::InitFromString(StringPiece val) {
  if (val.size() >= 2 && val[0] == '$') {
    proto.set_placeholder(val.data() + 1, val.size() - 1);
  } else {
    SetAttrValue(val, &proto);
  }
}

FunctionDefHelper::AttrValueWrapper FunctionDefHelper::FunctionRef(
    const string& name,
    gtl::ArraySlice<std::pair<string, AttrValueWrapper>> attrs) {
  AttrValueWrapper ret;
  ret.proto.mutable_func()->set_name(name);
  for (const auto& a : attrs) {
    ret.proto.mutable_func()->mutable_attr()->insert({a.first, a.second.proto});
  }
  return ret;
}

NodeDef FunctionDefHelper::Node::ToNodeDef() const {
  NodeDef n;
  n.set_op(this->op);
  n.set_name(this->ret[0]);
  for (const auto& a : this->attr) {
    n.mutable_attr()->insert({a.first, a.second.proto});
  }
  for (const string& a : this->arg) {
    n.add_input(a);
  }
  for (const string& d : this->dep) {
    n.add_input(strings::StrCat("^", d));
  }
  return n;
}

/* static */
FunctionDef FunctionDefHelper::Create(
    const string& function_name, gtl::ArraySlice<string> in_def,
    gtl::ArraySlice<string> out_def, gtl::ArraySlice<string> attr_def,
    gtl::ArraySlice<Node> node_def,
    gtl::ArraySlice<std::pair<string, string>> ret_def) {
  FunctionDef fdef;

  // Signature
  OpDefBuilder b(function_name);
  for (const auto& i : in_def) b.Input(i);
  for (const auto& o : out_def) b.Output(o);
  for (const auto& a : attr_def) b.Attr(a);

  OpRegistrationData op_reg_data;
  TF_CHECK_OK(b.Finalize(&op_reg_data));
  fdef.mutable_signature()->Swap(&op_reg_data.op_def);

  // Function body
  for (const auto& n : node_def) {
    *(fdef.add_node_def()) = n.ToNodeDef();
  }

  // Returns
  for (const auto& r : ret_def) {
    fdef.mutable_ret()->insert({r.first, r.second});
  }

  auto* op_def_registry = OpRegistry::Global();
  // Check if any op is stateful.
  for (const auto& n : node_def) {
    const OpDef* op_def = nullptr;
    auto status = op_def_registry->LookUpOpDef(n.op, &op_def);
    // Lookup can fail if e.g. we are calling a function that was not yet
    // defined.  If it happens, conservatively assume the op is stateful.
    if (!status.ok() || op_def->is_stateful()) {
      fdef.mutable_signature()->set_is_stateful(true);
    }
  }
  return fdef;
}

/* static */
FunctionDef FunctionDefHelper::Define(const string& name,
                                      gtl::ArraySlice<string> arg_def,
                                      gtl::ArraySlice<string> ret_def,
                                      gtl::ArraySlice<string> attr_def,
                                      gtl::ArraySlice<Node> node_def) {
  FunctionDef fdef;
  OpDefBuilder b(name);
  for (const auto& a : arg_def) b.Input(a);
  for (const auto& r : ret_def) b.Output(r);
  for (const auto& a : attr_def) b.Attr(a);

  OpRegistrationData op_reg_data;
  TF_CHECK_OK(b.Finalize(&op_reg_data));
  fdef.mutable_signature()->Swap(&op_reg_data.op_def);

  // Mapping from legacy output names to NodeDef outputs.
  std::unordered_map<string, string> ret_index;
  for (const auto& a : fdef.signature().input_arg()) {
    ret_index[a.name()] = a.name();
  }

  // For looking up OpDefs
  auto* op_def_registry = OpRegistry::Global();

  // Function body
  for (const auto& src : node_def) {
    NodeDef* n = fdef.add_node_def();
    n->set_op(src.op);
    n->set_name(src.ret[0]);
    for (const auto& a : src.attr) {
      n->mutable_attr()->insert({a.first, a.second.proto});
    }
    for (const string& a : src.arg) {
      const auto iter = ret_index.find(a);
      CHECK(iter != ret_index.end())
          << "Node input '" << a << "' in '" << src.ret[0] << "' of " << name;
      n->add_input(iter->second);
    }
    for (const string& d : src.dep) {
      n->add_input(strings::StrCat("^", d));
    }

    // Add the outputs of this node to ret_index.
    const OpDef* op_def = nullptr;
    TF_CHECK_OK(op_def_registry->LookUpOpDef(n->op(), &op_def)) << n->op();
    CHECK(op_def != nullptr) << n->op();
    NameRangeMap output_names;
    TF_CHECK_OK(NameRangesForNode(*n, *op_def, nullptr, &output_names));
    for (const auto& o : output_names) {
      CHECK_LE(o.second.second, src.ret.size())
          << "Missing ret for output '" << o.first << "' in '" << src.ret[0]
          << "' of " << name;
      for (int i = o.second.first; i < o.second.second; ++i) {
        ret_index[src.ret[i]] =
            strings::StrCat(src.ret[0], ":", o.first, ":", i - o.second.first);
      }
    }
    if (op_def->is_stateful()) fdef.mutable_signature()->set_is_stateful(true);
  }

  // Returns
  for (const auto& r : fdef.signature().output_arg()) {
    const auto iter = ret_index.find(r.name());
    CHECK(iter != ret_index.end()) << "Return '" << r.name() << "' in " << name;
    fdef.mutable_ret()->insert({r.name(), iter->second});
  }
  return fdef;
}

FunctionDef FunctionDefHelper::Define(gtl::ArraySlice<string> arg_def,
                                      gtl::ArraySlice<string> ret_def,
                                      gtl::ArraySlice<string> attr_def,
                                      gtl::ArraySlice<Node> node_def) {
  return Define("_", arg_def, ret_def, attr_def, node_def);
}

namespace gradient {

typedef std::unordered_map<string, Creator> OpGradFactory;

OpGradFactory* GetOpGradFactory() {
  static OpGradFactory* factory = new OpGradFactory;
  return factory;
}

bool RegisterOp(const string& op, Creator func) {
  CHECK(GetOpGradFactory()->insert({op, func}).second)
      << "Duplicated gradient for " << op;
  return true;
}

Status GetOpGradientCreator(const string& op, Creator* creator) {
  auto fac = GetOpGradFactory();
  auto iter = fac->find(op);
  if (iter == fac->end()) {
    return errors::NotFound("No gradient defined for op: ", op);
  }
  *creator = iter->second;
  return Status::OK();
}

}  // end namespace gradient

}  // end namespace tensorflow

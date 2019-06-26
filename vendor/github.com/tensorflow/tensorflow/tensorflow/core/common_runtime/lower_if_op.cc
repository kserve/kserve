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

#include "tensorflow/core/common_runtime/lower_if_op.h"

#include "tensorflow/core/common_runtime/function.h"
#include "tensorflow/core/framework/node_def_builder.h"
#include "tensorflow/core/graph/graph.h"
#include "tensorflow/core/graph/node_builder.h"

namespace tensorflow {

namespace {

using NodeOut = NodeBuilder::NodeOut;

// Convenience builder to make it easy to construct a conditional with a single
// function call in the then and else branch. This first converts the if node
// into switches (for inputs) and merges (for outputs) around a function call
// per branch, then inlines the function calls.
class CondBuilder {
 public:
  enum Branch { kElseBranch = 0, kThenBranch = 1 };

  // Create a CondBuilder to create the lowered form of `if_op` with then and
  // else functions named `then_fn_name` and `else_fn_name` respectively in the
  // `graph`. The functions should be available in `flib`.
  CondBuilder(Node* if_op, const string& then_fn_name,
              const string& else_fn_name, const FunctionLibraryDefinition& flib,
              Graph* graph);

  // Constructs the basic conditional control flow using switch and merge nodes.
  Status CreatePivotNodes();

  // Adds the inputs from the if node to the merge nodes of the lowered if.
  Status AddInputs();

  // Adds the outputs from the if node to the merge nodes of the lowered if.
  // Note: no inputs can be added once outputs are added as the then and else
  // nodes are finalized while adding outputs.
  Status AddOutputs();

  // Builds an identity node with the same outputs as If.
  Status BuildLoweredIfOutput();

  // Inline call nodes for then and else.
  Status InlineCallNodes();

 private:
  // Returns unique name containing the name of the If op being rewritten
  // (name_), infix and a suffix to ensure it is unique within the graph.
  string NewName(const string& infix);

  // Adds input to both the then and else nodes from src:src_output.
  Status AddInput(Node* src, int src_output);

  // The merged outputs of the then and else nodes.
  std::vector<NodeOut> outputs_;

  // The node that dominates all execution of the then and else body nodes.
  Node* control_predecessor_;
  // The original If op.
  Node* if_op_;
  // The identity node with the same outputs as the original If op.
  Node* lowered_if_output_;
  // The predicate of the conditional.
  OutputTensor pred_;
  // Node corresponding to pivot_f branch of predicate switch which is
  // the pivot node that dominates all nodes in the false/else branch.
  Node* pivot_f_;
  // Node corresponding to pivot_t branch of predicate switch which is
  // the pivot node that dominates all nodes in the true/then branch.
  Node* pivot_t_;
  Node* then_call_node_;
  Node* else_call_node_;
  Graph* graph_;
  const FunctionLibraryDefinition& flib_;
  string name_;

  NodeDebugInfo debug_info_;
  NodeBuilder then_call_builder_;
  NodeBuilder else_call_builder_;
};

CondBuilder::CondBuilder(Node* if_op, const string& then_fn_name,
                         const string& else_fn_name,
                         const FunctionLibraryDefinition& flib, Graph* graph)
    : if_op_(if_op),
      graph_(graph),
      flib_(flib),
      name_(if_op->name()),
      debug_info_(*if_op_),
      then_call_builder_(NewName("then"), then_fn_name, graph->op_registry(),
                         &debug_info_),
      else_call_builder_(NewName("else"), else_fn_name, graph->op_registry(),
                         &debug_info_) {
  TF_CHECK_OK(if_op_->input_tensor(0, &pred_));
  then_call_builder_.Device(if_op_->requested_device());
  else_call_builder_.Device(if_op_->requested_device());
}

Status CondBuilder::CreatePivotNodes() {
  // Construct the basic cond body (consisting of feeding in the predicate to
  // create pivot nodes).
  Node* switch_pred;
  TF_RETURN_IF_ERROR(NodeBuilder(NewName("switch_pred"), "Switch",
                                 graph_->op_registry(), &debug_info_)
                         .Input(NodeOut(pred_))
                         .Input(NodeOut(pred_))
                         .Device(if_op_->requested_device())
                         .Finalize(graph_, &switch_pred));
  control_predecessor_ = switch_pred;
  TF_RETURN_IF_ERROR(NodeBuilder(NewName("pivot_f"), "Identity",
                                 graph_->op_registry(), &debug_info_)
                         .Input(switch_pred, kElseBranch)
                         .Device(if_op_->requested_device())
                         .Finalize(graph_, &pivot_f_));
  TF_RETURN_IF_ERROR(NodeBuilder(NewName("pivot_t"), "Identity",
                                 graph_->op_registry(), &debug_info_)
                         .Input(switch_pred, kThenBranch)
                         .Device(if_op_->requested_device())
                         .Finalize(graph_, &pivot_t_));
  return Status::OK();
}

string CondBuilder::NewName(const string& infix) {
  return graph_->NewName(strings::StrCat(name_, "/", infix));
}

Status CondBuilder::AddInput(Node* src, int src_output) {
  Node* input;
  NodeDebugInfo debug_info(*src);
  TF_RETURN_IF_ERROR(NodeBuilder(NewName(src->name()), "Switch",
                                 graph_->op_registry(), &debug_info)
                         .Input(src, src_output)
                         .Input(pred_)
                         .Device(if_op_->requested_device())
                         .Finalize(graph_, &input));
  then_call_builder_.Input(input, kThenBranch);
  else_call_builder_.Input(input, kElseBranch);
  return Status::OK();
}

Status CondBuilder::AddInputs() {
  // Add input data edges.
  std::vector<const Edge*> edges;
  TF_RETURN_IF_ERROR(if_op_->input_edges(&edges));
  // Start at index 1 as the first input is the predicate.
  for (int i = 1; i < edges.size(); ++i) {
    const Edge* e = edges[i];
    TF_RETURN_IF_ERROR(AddInput(e->src(), e->src_output()));
  }
  // Add input control edges.
  for (const Edge* e : if_op_->in_edges()) {
    if (e->IsControlEdge()) {
      graph_->AddControlEdge(e->src(), control_predecessor_);
    }
  }
  return Status::OK();
}

Status CondBuilder::AddOutputs() {
  // Construct the then and else nodes.
  TF_RETURN_IF_ERROR(then_call_builder_.Finalize(graph_, &then_call_node_));
  graph_->AddControlEdge(pivot_t_, then_call_node_);
  TF_RETURN_IF_ERROR(else_call_builder_.Finalize(graph_, &else_call_node_));
  graph_->AddControlEdge(pivot_f_, else_call_node_);

  // Merge the outputs from the two branches.
  std::vector<Node*> merges(then_call_node_->num_outputs());
  outputs_.resize(merges.size());
  for (int i = 0; i < then_call_node_->num_outputs(); ++i) {
    TF_RETURN_IF_ERROR(
        NodeBuilder(graph_->NewName("merge"), "Merge", graph_->op_registry(),
                    &debug_info_)
            .Input({NodeOut(then_call_node_, i), NodeOut(else_call_node_, i)})
            .Device(if_op_->requested_device())
            .Finalize(graph_, &merges[i]));
    outputs_[i] = NodeOut(merges[i], 0);
  }

  TF_RETURN_IF_ERROR(BuildLoweredIfOutput());

  // Add outputs.
  for (const Edge* e : if_op_->out_edges()) {
    if (e->IsControlEdge()) {
      graph_->AddControlEdge(lowered_if_output_, e->dst());
    } else {
      // Feed the outputs directly from the merge nodes so that downstream ops
      // can start before all the outputs have been computed.
      graph_->AddEdge(merges[e->src_output()], 0, e->dst(), e->dst_input());
    }
  }
  return Status::OK();
}

Status InlineCallInGraph(Node* n, const FunctionLibraryDefinition& flib,
                         Graph* g) {
  const FunctionDef* fdef = flib.Find(n->type_string());
  CHECK(fdef != nullptr);
  FunctionBody* fbody;
  TF_RETURN_IF_ERROR(
      FunctionDefToBodyHelper(*fdef, n->attrs(), &flib,
                              [&flib](const string& op, const OpDef** sig) {
                                return flib.LookUpOpDef(op, sig);
                              },
                              &fbody));
  // TODO(jpienaar): Improve this interface to make the need to delete it
  // explicit.
  InlineFunctionBody(g->flib_def(), g, n, fbody, false);
  delete fbody;
  return Status::OK();
}

Status CondBuilder::BuildLoweredIfOutput() {
  // Build the identity node output.
  NodeBuilder ib(name_, "IdentityN");
  ib.Input(outputs_).Device(if_op_->requested_device());
  return ib.Finalize(graph_, &lowered_if_output_);
}

Status CondBuilder::InlineCallNodes() {
  TF_RETURN_IF_ERROR(InlineCallInGraph(then_call_node_, flib_, graph_));
  TF_RETURN_IF_ERROR(InlineCallInGraph(else_call_node_, flib_, graph_));
  return Status::OK();
}

}  // namespace

Status RewriteIfNode(Node* n, Graph* g, const FunctionLibraryDefinition& flib) {
  const AttrValue* then_attr = n->attrs().Find("then_branch");
  if (then_attr == nullptr) {
    return errors::InvalidArgument("Then branch function missing");
  }
  const AttrValue* else_attr = n->attrs().Find("else_branch");
  if (else_attr == nullptr) {
    return errors::InvalidArgument("Else branch function missing");
  }

  CondBuilder cb(n, then_attr->func().name(), else_attr->func().name(), flib,
                 g);
  TF_RETURN_IF_ERROR(cb.CreatePivotNodes());
  TF_RETURN_IF_ERROR(cb.AddInputs());
  TF_RETURN_IF_ERROR(cb.AddOutputs());
  TF_RETURN_IF_ERROR(cb.InlineCallNodes());
  g->RemoveNode(n);

  return Status::OK();
}

}  // namespace tensorflow

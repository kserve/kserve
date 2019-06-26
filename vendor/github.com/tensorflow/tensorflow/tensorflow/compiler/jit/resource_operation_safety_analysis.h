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

#ifndef TENSORFLOW_COMPILER_JIT_RESOURCE_OPERATION_SAFETY_ANALYSIS_H_
#define TENSORFLOW_COMPILER_JIT_RESOURCE_OPERATION_SAFETY_ANALYSIS_H_

#include "tensorflow/compiler/jit/graphcycles/graphcycles.h"
#include "tensorflow/core/framework/function.h"
#include "tensorflow/core/graph/graph.h"

namespace tensorflow {
// An XLA cluster hoists all resource reads to be beginning of the cluster
// execution and all the resource writes to the end.  This means it cannot
// enforce arbitrary ordering dependencies (via control or data edges) between
// resource operations.  Since all resource reads happen before all resource
// writes, edges constraining resource reads to happen before resource writes
// are fine, but all other kinds of edges are problematic.  This analysis
// returns the set of pairs of resource operations that cannot be put in the
// same cluster because XLA cannot respect the dependencies between them in the
// TensorFlow program.
//
// The restrictions are not transitive: it is fine to put A and C in the same
// cluster even if the returned set contains (A,B) and (B,C).
//
// In other words, if these pairs are seen as edges in an undirected graph of
// the nodes in `g` then auto-clustering is at least as constrained as the graph
// coloring problem on this graph.
//
//
// For instance if we auto-cluster all operations in this TensorFlow graph:
//
//         ReadVariablepOp0  ->  ReadVariableOp1
//                                      |
//                                      v
//                              AssignVariableOp0  ->  AssignVariableOp1
//
// we will lose the ReadVariablepOp0 -> ReadVariableOp1 and the
// AssignVariableOp0 -> AssignVariableOp1 dependencies.  I.e. it is possible for
// XlaLaunchOp to issue ReadVariableOp1 before ReadVariablepOp0 since it reads
// all the resource variables when the cluster starts executing without any
// particular ordering between them; same holds for the AssignVariableOp0 ->
// AssignVariableOp1 edge.  The ReadVariableOp1 -> AssignVariableOp0 edge will
// be respected by XlaLaunchOp though because all reads happen before all
// writes.
//
//
// NB!  The result computed by this analysis assumes that we don't auto-cluster
// back-edges (i.e. the edges from NextIteration to Merge).
//
// NB!  The result computed by this analysis assumes that we don't auto-cluster
// functional control flow nodes containing resource operations.
//
// If `resource_ops_to_ignore` is set then nodes for which it returns true are
// ignored (we pretend these nodes are not resource operations).
Status ComputeIncompatibleResourceOperationPairs(
    const Graph& g, const FunctionLibraryDefinition* flib_def,
    const std::function<Status(const Node&, bool*)>& resource_ops_to_ignore,
    std::vector<std::pair<int, int>>* result);
}  // namespace tensorflow

#endif  // TENSORFLOW_COMPILER_JIT_RESOURCE_OPERATION_SAFETY_ANALYSIS_H_

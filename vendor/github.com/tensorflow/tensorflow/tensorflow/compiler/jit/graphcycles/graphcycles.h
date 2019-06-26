/* Copyright 2017 The TensorFlow Authors. All Rights Reserved.

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

#ifndef TENSORFLOW_COMPILER_JIT_GRAPHCYCLES_GRAPHCYCLES_H_
#define TENSORFLOW_COMPILER_JIT_GRAPHCYCLES_GRAPHCYCLES_H_

// GraphCycles detects the introduction of a cycle into a directed
// graph that is being built up incrementally.
//
// Nodes are identified by small integers.  It is not possible to
// record multiple edges with the same (source, destination) pair;
// requests to add an edge where one already exists are silently
// ignored.
//
// It is also not possible to introduce a cycle; an attempt to insert
// an edge that would introduce a cycle fails and returns false.
//
// GraphCycles uses no internal locking; calls into it should be
// serialized externally.

// Performance considerations:
//   Works well on sparse graphs, poorly on dense graphs.
//   Extra information is maintained incrementally to detect cycles quickly.
//   InsertEdge() is very fast when the edge already exists, and reasonably fast
//   otherwise.
//   FindPath() is linear in the size of the graph.
// The current implementation uses O(|V|+|E|) space.

#include <unordered_set>

#include "tensorflow/core/platform/macros.h"
#include "tensorflow/core/platform/types.h"

namespace tensorflow {

// NOTE!!!
// For now a copy of this is forked to net/plaque. If you
// find a bug or add a feature, please inform the owners of the
// net/plaque copy in case it should be integrated.
// NOTE!!!
class GraphCycles {
 public:
  GraphCycles();
  ~GraphCycles();

  // Allocate an unused node id and return it.
  // The new node has a null pointer for its node data.
  // All node identifiers passed to other routines in this interface
  // must have been allocated by NewNode() and not yet deallocated
  // by RemoveNode().
  int32 NewNode();

  // Remove "node" from the graph, deleting all edges to and from it.
  // After this call the identifier "node" it may no longer be used
  // as an argument to any routine until it has been reallocated with
  // NewNode().
  void RemoveNode(int32 node);

  // Attempt to insert an edge from source_node to dest_node.  If the
  // edge would introduce a cycle, return false without making any
  // changes. Otherwise add the edge and return true.
  bool InsertEdge(int32 source_node, int32 dest_node);

  // Remove any edge that exists from source_node to dest_node.
  void RemoveEdge(int32 source_node, int32 dest_node);

  // Return whether there is an edge directly from source_node to dest_node.
  bool HasEdge(int32 source_node, int32 dest_node) const;

  // Contracts the edge from 'a' to node 'b', merging nodes 'a' and 'b'. 'b' is
  // removed from the graph, and edges to/from 'b' are replaced with edges
  // to/from 'a'. If contracting the edge would create a cycle, does nothing
  // and returns false.
  bool ContractEdge(int32 a, int32 b);

  // Return true if can contract edge, otherwise return false.
  bool CanContractEdge(int32 a, int32 b);

  // Return whether dest_node is reachable from source_node
  // by following edges.
  bool IsReachable(int32 source_node, int32 dest_node) const;

  // A faster non-thread-safe version of IsReachable.
  bool IsReachableNonConst(int32 source_node, int32 dest_node);

  // Return or set the node data for a node.  This data is unused
  // by the implementation.
  void *GetNodeData(int32 node) const;
  void SetNodeData(int32 node, void *data);

  // Find a path from "source" to "dest".  If such a path exists, place the
  // node IDs of the nodes on the path in the array path[], and return the
  // number of nodes on the path.  If the path is longer than max_path_len
  // nodes, only the first max_path_len nodes are placed in path[].  The client
  // should compare the return value with max_path_len" to see when this
  // occurs.  If no path exists, return 0.  Any valid path stored in path[]
  // will start with "source" and end with "dest".  There is no guarantee that
  // the path is the shortest, but no node will appear twice in the path,
  // except the source and destination node if they are identical; therefore,
  // the return value is at most one greater than the number of nodes in the
  // graph.
  int FindPath(int32 source, int32 dest, int max_path_len, int32 path[]) const;

  // Check internal invariants. Crashes on failure, returns true on success.
  // Expensive: should only be called from graphcycles_test.cc.
  bool CheckInvariants() const;

  std::unordered_set<int32> Successors(int32 node);
  std::unordered_set<int32> Predecessors(int32 node);

  // ----------------------------------------------------
  struct Rep;

 private:
  Rep *rep_;  // opaque representation
  TF_DISALLOW_COPY_AND_ASSIGN(GraphCycles);
};

}  // namespace tensorflow
#endif  // TENSORFLOW_COMPILER_JIT_GRAPHCYCLES_GRAPHCYCLES_H_

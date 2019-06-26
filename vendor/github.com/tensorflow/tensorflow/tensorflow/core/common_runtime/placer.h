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

#ifndef TENSORFLOW_CORE_COMMON_RUNTIME_PLACER_H_
#define TENSORFLOW_CORE_COMMON_RUNTIME_PLACER_H_

#include <string>
#include <unordered_map>

#include "tensorflow/core/common_runtime/device_set.h"
#include "tensorflow/core/graph/graph.h"
#include "tensorflow/core/lib/core/status.h"
#include "tensorflow/core/platform/macros.h"
#include "tensorflow/core/platform/types.h"
#include "tensorflow/core/public/session_options.h"
#include "tensorflow/core/util/device_name_utils.h"

namespace tensorflow {

// A placement algorithm that assigns the nodes of the given Graph to
// devices the given DeviceSet, respecting the following constraints:
//
// 1. Existing device assignments remain unchanged.
// 2. Requested (partial or complete) device specifications given by device name
//    for each node are granted.
// 3. Nodes connected by edges of a reference type are colocated on
//    the same device.
// 4. Given nodes "A" and "B", if node "B" has a colocation group
//    "@loc:A", nodes "A" and "B" will be colocated on the same device.
//
// The implementation builds a constraint graph with the same set of
// nodes, and edges that represent colocation constraints between
// nodes.  Each connected component in the resulting constraint graph
// is then assigned to a set of valid devices.
//
// Run() will finally assign the device to each node given the list of
// possible devices.
//
// TODO(mrry): "Soft" constraints, such as "place node 'x' as close as
// possible to node 'y' while respecting the other constraints"?
// TODO(mrry): Create a common interface for this and the other
// placement algorithms so that they may be injected into the graph
// builder.
class Placer {
 public:
  // A map from graph node names to numerical IDs (in a Graph object).
  typedef std::unordered_map<string, int> NodeNameToIdMap;

  // Creates an instance of the Placer algorithm for the given
  // Graph "graph" (nodes in which may or may not be assigned) on the
  // given DeviceSet "devices".
  //
  // If non-null, default_device is used where possible as a placement for nodes
  // which do not have a device specified, ahead of other devices which would
  // otherwise be higher priority.
  //
  // The "graph", "devices", and "default_device" pointer arguments are borrowed
  // by this Placer, and must outlive it.
  Placer(Graph* graph, const DeviceSet* devices, const SessionOptions* options,
         const Device* default_device);

  Placer(Graph* graph, const DeviceSet* devices);

  ~Placer();

  // Assigns each node in this Placer's graph to a device in its
  // set of devices.
  //
  // This method is not thread-safe.
  // Run() may be invoked at most once.
  Status Run();

 private:
  // Returns true if the device type of 'candidate_device_name' is
  // found in 'devices'.
  bool CanAssignToDevice(const string& candidate_device_name,
                         const std::vector<Device*>& devices) const;

  // Assigns 'node's devices to 'assigned_device', and logs the
  // placement if the SessionOptions entry in 'options_' requests it.
  void AssignAndLog(int assigned_device, Node* node) const;
  void LogDeviceAssignment(const Node* node) const;

  Graph* const graph_;              // Not owned.
  const DeviceSet* const devices_;  // Not owned.
  const SessionOptions* options_;   // Not owned.
  const bool log_device_placement_;
  const Device* default_device_;  // Not owned.

  TF_DISALLOW_COPY_AND_ASSIGN(Placer);
};

}  // namespace tensorflow

#endif  // TENSORFLOW_CORE_COMMON_RUNTIME_PLACER_H_

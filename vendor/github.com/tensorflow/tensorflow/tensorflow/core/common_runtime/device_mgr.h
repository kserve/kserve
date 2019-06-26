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

#ifndef TENSORFLOW_CORE_COMMON_RUNTIME_DEVICE_MGR_H_
#define TENSORFLOW_CORE_COMMON_RUNTIME_DEVICE_MGR_H_

#include <memory>
#include <string>
#include <unordered_map>
#include <unordered_set>
#include <vector>

#include "tensorflow/core/common_runtime/device.h"
#include "tensorflow/core/lib/core/arena.h"
#include "tensorflow/core/lib/core/status.h"
#include "tensorflow/core/lib/core/stringpiece.h"
#include "tensorflow/core/lib/gtl/inlined_vector.h"
#include "tensorflow/core/platform/macros.h"

namespace tensorflow {

class DeviceAttributes;

class DeviceMgr {
 public:
  // Constructs a DeviceMgr from a list of devices.
  // TODO(zhifengc): Other initialization information.
  explicit DeviceMgr(std::vector<std::unique_ptr<Device>> devices);

  // Constructs a DeviceMgr managing a single device.
  explicit DeviceMgr(std::unique_ptr<Device> device);

  // Returns attributes of all devices.
  void ListDeviceAttributes(std::vector<DeviceAttributes>* devices) const;

  // Returns raw pointers to the underlying devices.
  std::vector<Device*> ListDevices() const;

  // Returns a string listing all devices.
  string DebugString() const;

  // Returns a string of all the device mapping.
  string DeviceMappingString() const;

  // Assigns *device with pointer to Device of the given name.
  // Accepts either a full device name, or just the replica-local suffix.
  Status LookupDevice(StringPiece name, Device** device) const;

  // Clears given containers of all devices if 'container' is
  // non-empty. Otherwise, clears default containers of all devices.
  void ClearContainers(gtl::ArraySlice<string> containers) const;

  int NumDeviceType(const string& type) const;

 private:
  const std::vector<std::unique_ptr<Device>> devices_;

  StringPiece CopyToBackingStore(StringPiece s);

  std::unordered_map<StringPiece, Device*, StringPieceHasher> device_map_;
  core::Arena name_backing_store_;  // Storage for keys in device_map_
  std::unordered_map<string, int> device_type_counts_;

  TF_DISALLOW_COPY_AND_ASSIGN(DeviceMgr);
};

}  // namespace tensorflow

#endif  // TENSORFLOW_CORE_COMMON_RUNTIME_DEVICE_MGR_H_

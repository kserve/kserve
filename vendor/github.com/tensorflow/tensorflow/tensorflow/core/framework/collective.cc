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
#include "tensorflow/core/framework/collective.h"

#include "tensorflow/core/framework/op_kernel.h"
#include "tensorflow/core/lib/core/errors.h"
#include "tensorflow/core/lib/hash/hash.h"
#include "tensorflow/core/lib/strings/strcat.h"

namespace tensorflow {

namespace {
// A RegistrationInfo object stores a collective implementation registration
// details.  `factory` is used to create instances of the collective
// implementation.
struct RegistrationInfo {
  // This constructor also creates, and stores in `param_resolver_instance`,
  // what is effectively a static instance of the collective implementation.
  // During param resolution of collective ops we return this static instance.
  // The actual op execution gets a fresh instance using `factory`.
  RegistrationInfo(const string& n, CollectiveRegistry::Factory f)
      : name(n),
        factory(std::move(f)),
        param_resolver_instance(this->factory()) {}
  string name;
  CollectiveRegistry::Factory factory;
  CollectiveImplementationInterface* param_resolver_instance;
};

std::vector<RegistrationInfo>* MutableCollectiveRegistry() {
  static std::vector<RegistrationInfo>* registry =
      new std::vector<RegistrationInfo>;
  return registry;
}
}  // namespace

string CollGroupParams::ToString() const {
  return strings::StrCat("CollGroupParams {group_key=", group_key,
                         " group_size=", group_size,
                         " device_type=", device_type.type_string(),
                         " num_tasks=", num_tasks, "}");
}

CollInstanceParams& CollInstanceParams::operator=(
    const CollInstanceParams& other) {
  if (this != &other) {
    instance_key = other.instance_key;
    type = other.type;
    data_type = other.data_type;
    shape = other.shape;
    device_names.clear();
    device_names.assign(other.device_names.begin(), other.device_names.end());
    task_names.assign(other.task_names.begin(), other.task_names.end());
    same_num_devices_per_task = other.same_num_devices_per_task;
    gpu_ring_order = other.gpu_ring_order;
    impl_details.subdiv_offsets.assign(
        other.impl_details.subdiv_offsets.begin(),
        other.impl_details.subdiv_offsets.end());
    impl_details.subdiv_permutations.clear();
    for (auto p : other.impl_details.subdiv_permutations) {
      impl_details.subdiv_permutations.push_back(
          std::vector<int>(p.begin(), p.end()));
    }
    impl_details.subdiv_source_rank.assign(
        other.impl_details.subdiv_source_rank.begin(),
        other.impl_details.subdiv_source_rank.end());
  }
  return *this;
}

string CollInstanceParams::ToString() const {
  string v = strings::StrCat("CollInstanceParams { instance_key=", instance_key,
                             " type=", type, " data_type=", data_type,
                             " shape=", shape.DebugString(), " devices {");
  for (const auto& d : device_names) {
    strings::StrAppend(&v, d, ",");
  }
  strings::StrAppend(&v, "} task_names={");
  for (const auto& n : task_names) {
    strings::StrAppend(&v, n, ", ");
  }
  strings::StrAppend(&v, "}, subdiv_offsets={");
  for (const auto& d : impl_details.subdiv_offsets) {
    strings::StrAppend(&v, d, ",");
  }
  strings::StrAppend(&v, "}, subdiv_perms={");
  for (const auto& p : impl_details.subdiv_permutations) {
    strings::StrAppend(&v, "{");
    for (const auto& i : p) {
      strings::StrAppend(&v, i, ",");
    }
    strings::StrAppend(&v, "}");  // one subdiv
  }
  if (!impl_details.subdiv_source_rank.empty()) {
    strings::StrAppend(&v, " subdiv_source_rank={");
    for (const auto& r : impl_details.subdiv_source_rank) {
      strings::StrAppend(&v, r, ",");
    }
    strings::StrAppend(&v, "}");
  }
  strings::StrAppend(&v, "}");  // all subdivs
  return v;
}

string CollTaskParams::ToString() const {
  string v = strings::StrCat("CollTaskParams {is_local={");
  for (const auto& b : is_local) {
    strings::StrAppend(&v, static_cast<int>(b), ",");
  }
  strings::StrAppend(&v, "}}");
  return v;
}

string CollectiveParams::ToString() const {
  string v = strings::StrCat("CollectiveParams ", name, " {", group.ToString());
  strings::StrAppend(&v, " ", instance.ToString());
  strings::StrAppend(&v, " ", task.ToString());
  strings::StrAppend(&v, " default_rank=", default_rank,
                     " is_source=", is_source, " source_rank=", source_rank,
                     " subdiv_rank={");
  for (const auto& r : subdiv_rank) {
    strings::StrAppend(&v, r, ",");
  }
  strings::StrAppend(&v, "}}");
  return v;
}

/*static*/ OpKernelContext::Params* CollectiveExecutor::CtxParams(
    OpKernelContext* ctx) {
  return ctx->params_;
}

CollectiveContext::CollectiveContext(CollectiveExecutor* col_exec,
                                     const DeviceMgr* dev_mgr,
                                     OpKernelContext* ctx,
                                     OpKernelContext::Params* op_params,
                                     const CollectiveParams& col_params,
                                     const string& exec_key, int64 step_id,
                                     const Tensor* input, Tensor* output)
    : col_exec(col_exec),
      dev_mgr(dev_mgr),
      op_ctx(ctx),
      op_params(op_params),
      col_params(col_params),
      exec_key(exec_key),
      step_id(step_id),
      input(input),
      output(output),
      device(nullptr),
      device_name(col_params.instance.device_names[col_params.default_rank]) {}

/*static*/
int64 CollectiveExecutor::kInvalidId = -1;

/*static*/
Status CollectiveRegistry::Lookup(
    const string& collective_name,
    CollectiveImplementationInterface** implementation) {
  return LookupHelper(collective_name, implementation, false);
}

/*static*/
Status CollectiveRegistry::LookupParamResolverInstance(
    const string& collective_name,
    CollectiveImplementationInterface** implementation) {
  return LookupHelper(collective_name, implementation, true);
}

/*static*/
void CollectiveRegistry::GetAll(
    std::vector<CollectiveImplementationInterface*>* implementations) {
  std::vector<RegistrationInfo>* registry = MutableCollectiveRegistry();
  for (const RegistrationInfo& reg_info : *registry)
    implementations->emplace_back(reg_info.factory());
}

/*static*/
Status CollectiveRegistry::Register(const string& collective_name,
                                    Factory factory) {
  std::vector<RegistrationInfo>* registry = MutableCollectiveRegistry();
  for (const RegistrationInfo& reg_info : *registry) {
    if (reg_info.name == collective_name)
      return errors::Internal("Already registered collective ",
                              collective_name);
  }
  registry->emplace_back(collective_name, std::move(factory));
  return Status::OK();
}

/*static*/
Status CollectiveRegistry::LookupHelper(
    const string& collective_name,
    CollectiveImplementationInterface** implementation, bool param_resolver) {
  std::vector<RegistrationInfo>* registry = MutableCollectiveRegistry();
  for (const RegistrationInfo& reg_info : *registry) {
    if (reg_info.name == collective_name) {
      if (param_resolver) {
        *implementation = reg_info.param_resolver_instance;
      } else {
        *implementation = reg_info.factory();
      }
      return Status::OK();
    }
  }
  return errors::Internal(
      "CollectiveRegistry::Lookup did not find collective implementation ",
      collective_name);
}

}  // namespace tensorflow

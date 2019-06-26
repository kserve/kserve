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
#ifndef TENSORFLOW_CORE_DISTRIBUTED_RUNTIME_COLLECTIVE_PARAM_RESOLVER_DISTRIBUTED_H_
#define TENSORFLOW_CORE_DISTRIBUTED_RUNTIME_COLLECTIVE_PARAM_RESOLVER_DISTRIBUTED_H_

#include "tensorflow/core/common_runtime/collective_param_resolver_local.h"

namespace tensorflow {
class ConfigProto;
class WorkerCacheInterface;
class DeviceResolverDistributed;
class DeviceMgr;

class CollectiveParamResolverDistributed : public CollectiveParamResolverLocal {
 public:
  CollectiveParamResolverDistributed(const ConfigProto& config,
                                     const DeviceMgr* dev_mgr,
                                     DeviceResolverDistributed* dev_resolver,
                                     WorkerCacheInterface* worker_cache,
                                     const string& task_name);

  void CompleteParamsAsync(const string& device, CollectiveParams* cp,
                           CancellationManager* cancel_mgr,
                           const StatusCallback& done) override;

  void CompleteGroupAsync(const CompleteGroupRequest* request,
                          CompleteGroupResponse* response,
                          CancellationManager* cancel_mgr,
                          const StatusCallback& done) override;

  void CompleteInstanceAsync(const CompleteInstanceRequest* request,
                             CompleteInstanceResponse* response,
                             CancellationManager* cancel_mgr,
                             const StatusCallback& done) override;

 protected:
  // Returns true iff there's an entry for this group_key in the
  // local group_table_.
  bool GroupIsCached(int32 group_key) LOCKS_EXCLUDED(group_mu_);

  // Updates group_table_ with contents of resp.
  Status UpdateGroupCache(const CompleteGroupResponse& resp)
      LOCKS_EXCLUDED(group_mu_);

  // Finds the GroupRec that corresponds to cp->group_key and also
  // populates cp->group from that GroupRec.
  //
  // Semantics are like those of CompleteGroupLocal but will make a
  // remote call to the group leader if necessary.
  void CompleteGroupDistributed(const string& device, CollectiveParams* cp,
                                CancellationManager* cancel_mgr,
                                const GroupRecCallback& done);

  // Returns true iff there's an entry for this instance_key in the
  // local instance_table_.
  bool InstanceIsCached(int32 instance_key) LOCKS_EXCLUDED(instance_mu_);

  // Updates instance_table_ with contents of resp.
  void UpdateInstanceCache(const GroupRec* gr, CollectiveParams* cp,
                           const CompleteInstanceResponse& resp,
                           const StatusCallback& done)
      LOCKS_EXCLUDED(instance_mu_, gr->mu, group_mu_);

  // Finish populating *cp.  Semantics are like those of
  // CompleteInstanceLocal but will make a remote call to the group
  // leader if necessary.
  void CompleteInstanceDistributed(const string& device, const GroupRec* gr,
                                   CollectiveParams* cp,
                                   CancellationManager* cancel_mgr,
                                   const StatusCallback& done)
      LOCKS_EXCLUDED(instance_mu_, gr->mu, group_mu_);

  WorkerCacheInterface* worker_cache_;  // Not owned
  const string group_leader_;
};

}  // namespace tensorflow
#endif  // TENSORFLOW_CORE_DISTRIBUTED_RUNTIME_COLLECTIVE_PARAM_RESOLVER_DISTRIBUTED_H_

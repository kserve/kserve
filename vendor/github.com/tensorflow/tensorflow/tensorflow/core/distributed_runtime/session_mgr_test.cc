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

#include "tensorflow/core/distributed_runtime/session_mgr.h"

#include "tensorflow/core/distributed_runtime/rpc/rpc_rendezvous_mgr.h"
#include "tensorflow/core/distributed_runtime/worker_env.h"
#include "tensorflow/core/lib/core/status_test_util.h"
#include "tensorflow/core/platform/test.h"
#include "tensorflow/core/protobuf/cluster.pb.h"

namespace tensorflow {

class FakeDevice : public Device {
 private:
  explicit FakeDevice(const DeviceAttributes& device_attributes)
      : Device(nullptr, device_attributes) {}

 public:
  Status Sync() override { return errors::Unimplemented("FakeDevice::Sync()"); }

  Allocator* GetAllocator(AllocatorAttributes attr) override { return nullptr; }

  static std::unique_ptr<Device> MakeCPU(const string& name) {
    DeviceAttributes device_attributes;
    device_attributes.set_name(name);
    device_attributes.set_device_type(DeviceType("FakeCPU").type());
    return std::unique_ptr<Device>(new FakeDevice(device_attributes));
  }
};

class SessionMgrTest : public ::testing::Test {
 protected:
  SessionMgrTest()
      : mgr_(&env_, "/job:mnist/replica:0/task:0",
             std::unique_ptr<WorkerCacheInterface>(), factory_) {
    device_mgr_ = absl::make_unique<DeviceMgr>(
        FakeDevice::MakeCPU("/job:mnist/replica:0/task:0/device:fakecpu:0"));
    env_.local_devices = device_mgr_->ListDevices();
    env_.device_mgr = device_mgr_.get();
  }

  std::unique_ptr<DeviceMgr> device_mgr_;
  WorkerEnv env_;
  SessionMgr::WorkerCacheFactory factory_ =
      [](const ServerDef& server_def, WorkerCacheInterface** worker_cache) {
        *worker_cache = nullptr;  // Set to null to make debugging easier.
        return Status::OK();
      };
  SessionMgr mgr_;
};

TEST_F(SessionMgrTest, CreateSessionSimple) {
  ServerDef server_def;
  server_def.set_job_name("worker");
  server_def.set_task_index(3);

  string session_handle = "test_session_handle";
  TF_EXPECT_OK(mgr_.CreateSession(session_handle, server_def, true));
  std::shared_ptr<WorkerSession> session;
  TF_EXPECT_OK(mgr_.WorkerSessionForSession(session_handle, &session));
  EXPECT_NE(nullptr, session) << "Session for " << session_handle << "was null";
  EXPECT_NE(mgr_.LegacySession(), session);
  TF_EXPECT_OK(mgr_.DeleteSession(session_handle));
}

TEST_F(SessionMgrTest, CreateSessionClusterDefWorkerName) {
  ServerDef server_def;
  server_def.set_job_name("worker");
  server_def.set_task_index(3);
  auto job = server_def.mutable_cluster()->add_job();
  job->set_name("worker");
  job->mutable_tasks()->insert({3, "localhost:3333"});

  string session_handle = "test_session_handle";
  TF_EXPECT_OK(mgr_.CreateSession(session_handle, server_def, true));
  std::shared_ptr<WorkerSession> session;
  TF_EXPECT_OK(mgr_.WorkerSessionForSession(session_handle, &session));
  EXPECT_NE(nullptr, session) << "Session for " << session_handle << "was null";
  EXPECT_EQ("/job:worker/replica:0/task:3", session->worker_name);
  TF_EXPECT_OK(mgr_.DeleteSession(session_handle));
}

TEST_F(SessionMgrTest, CreateSessionDefaultWorkerName) {
  ServerDef server_def;
  string session_handle = "test_session_handle";
  TF_EXPECT_OK(mgr_.CreateSession(session_handle, server_def, true));
  std::shared_ptr<WorkerSession> session;
  TF_EXPECT_OK(mgr_.WorkerSessionForSession(session_handle, &session));
  EXPECT_NE(nullptr, session) << "Session for " << session_handle << "was null";
  EXPECT_EQ("/job:mnist/replica:0/task:0", session->worker_name);
  TF_EXPECT_OK(mgr_.DeleteSession(session_handle));
}

TEST_F(SessionMgrTest, CreateSessionIsolateSessionState) {
  ServerDef server_def;
  server_def.set_job_name("worker");
  server_def.set_task_index(3);

  TF_EXPECT_OK(mgr_.CreateSession("handle_1", server_def, false));
  std::shared_ptr<WorkerSession> session_1;
  TF_EXPECT_OK(mgr_.WorkerSessionForSession("handle_1", &session_1));
  std::vector<Device*> devices_1 = session_1->device_mgr()->ListDevices();
  EXPECT_EQ(1, devices_1.size());

  TF_EXPECT_OK(mgr_.CreateSession("handle_2", server_def, false));
  std::shared_ptr<WorkerSession> session_2;
  TF_EXPECT_OK(mgr_.WorkerSessionForSession("handle_2", &session_2));
  std::vector<Device*> devices_2 = session_2->device_mgr()->ListDevices();
  EXPECT_EQ(1, devices_2.size());

  TF_EXPECT_OK(mgr_.CreateSession("handle_3", server_def, true));
  std::shared_ptr<WorkerSession> session_3;
  TF_EXPECT_OK(mgr_.WorkerSessionForSession("handle_3", &session_3));
  std::vector<Device*> devices_3 = session_3->device_mgr()->ListDevices();
  EXPECT_EQ(1, devices_3.size());

  TF_EXPECT_OK(mgr_.CreateSession("handle_4", server_def, true));
  std::shared_ptr<WorkerSession> session_4;
  TF_EXPECT_OK(mgr_.WorkerSessionForSession("handle_4", &session_4));
  std::vector<Device*> devices_4 = session_4->device_mgr()->ListDevices();
  EXPECT_EQ(1, devices_4.size());

  EXPECT_EQ(devices_1[0]->resource_manager(), devices_2[0]->resource_manager());
  EXPECT_NE(devices_1[0]->resource_manager(), devices_3[0]->resource_manager());
  EXPECT_NE(devices_1[0]->resource_manager(), devices_4[0]->resource_manager());
  EXPECT_NE(devices_3[0]->resource_manager(), devices_4[0]->resource_manager());
}

TEST_F(SessionMgrTest, LegacySession) {
  ServerDef server_def;
  string session_handle = "";
  std::shared_ptr<WorkerSession> session;
  TF_EXPECT_OK(mgr_.WorkerSessionForSession(session_handle, &session));
  EXPECT_EQ(mgr_.LegacySession(), session);

  TF_EXPECT_OK(mgr_.DeleteSession(session_handle));
}

TEST_F(SessionMgrTest, UnknownSessionHandle) {
  ServerDef server_def;
  string session_handle = "unknown_session_handle";
  std::shared_ptr<WorkerSession> session;
  Status s = mgr_.WorkerSessionForSession(session_handle, &session);
  EXPECT_TRUE(errors::IsAborted(s));
  EXPECT_TRUE(
      str_util::StrContains(s.error_message(), "Session handle is not found"));
}

TEST_F(SessionMgrTest, WorkerNameFromServerDef) {
  ServerDef server_def;
  server_def.set_job_name("worker");
  server_def.set_task_index(3);
  string worker_name = SessionMgr::WorkerNameFromServerDef(server_def);
  EXPECT_EQ("/job:worker/replica:0/task:3", worker_name);
}

TEST_F(SessionMgrTest, DeleteLegacySession) {
  TF_EXPECT_OK(mgr_.DeleteSession(""));
}

}  // namespace tensorflow

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

#include "tensorflow/contrib/ignite/kernels/igfs/igfs_client.h"

namespace tensorflow {

IGFSClient::IGFSClient(const string &host, int port, const string &fs_name,
                       const string &user_name)
    : fs_name_(fs_name),
      user_name_(user_name),
      client_(ExtendedTCPClient(host, port, true)) {
  client_.Connect();
}

IGFSClient::~IGFSClient() { client_.Disconnect(); }

Status IGFSClient::SendRequestGetResponse(const Request &request,
                                          Response *response) {
  TF_RETURN_IF_ERROR(request.Write(&client_));
  client_.reset();

  if (response != nullptr) {
    TF_RETURN_IF_ERROR(response->Read(&client_));
    client_.reset();
  }

  return Status::OK();
}

}  // namespace tensorflow

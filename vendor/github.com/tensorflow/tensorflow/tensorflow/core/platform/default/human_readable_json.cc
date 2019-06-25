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

#include "tensorflow/core/platform/human_readable_json.h"
#include "tensorflow/core/lib/core/errors.h"
#include "tensorflow/core/lib/core/status.h"
#include "tensorflow/core/lib/strings/strcat.h"

namespace tensorflow {

Status ProtoToHumanReadableJson(const protobuf::Message& proto,
                                string* result) {
#ifdef TENSORFLOW_LITE_PROTOS
  *result = "[human readable output not available on Android]";
  return Status::OK();
#else
  result->clear();

  auto status = protobuf::util::MessageToJsonString(proto, result);
  if (!status.ok()) {
    // Convert error_msg google::protobuf::StringPiece to
    // tensorflow::StringPiece.
    auto error_msg = status.error_message();
    return errors::Internal(
        strings::StrCat("Could not convert proto to JSON string: ",
                        StringPiece(error_msg.data(), error_msg.length())));
  }
  return Status::OK();
#endif
}

Status HumanReadableJsonToProto(const string& str, protobuf::Message* proto) {
#ifdef TENSORFLOW_LITE_PROTOS
  return errors::Internal("Cannot parse JSON protos on Android");
#else
  proto->Clear();
  auto status = google::protobuf::util::JsonStringToMessage(str, proto);
  if (!status.ok()) {
    // Convert error_msg google::protobuf::StringPiece to
    // tensorflow::StringPiece.
    auto error_msg = status.error_message();
    return errors::Internal(
        strings::StrCat("Could not convert JSON string to proto: ",
                        StringPiece(error_msg.data(), error_msg.length())));
  }
  return Status::OK();
#endif
}

}  // namespace tensorflow

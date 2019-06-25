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

#include "tensorflow/core/platform/logger.h"

#include "tensorflow/core/platform/logging.h"

namespace tensorflow {

Logger* Logger::Singleton() {
  class DefaultLogger : public Logger {
   private:
    void DoLogProto(google::protobuf::Any* proto) override {
      VLOG(2) << proto->ShortDebugString();
    }
    void DoFlush() override {}
  };
  static Logger* instance = new DefaultLogger();
  return instance;
}

}  // namespace tensorflow

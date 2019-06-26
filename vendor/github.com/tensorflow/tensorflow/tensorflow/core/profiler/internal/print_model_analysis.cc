/* Copyright 2016 The TensorFlow Authors All Rights Reserved.

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

#include "tensorflow/core/profiler/internal/print_model_analysis.h"

#include <stdio.h>
#include <memory>
#include <utility>

#include "tensorflow/c/checkpoint_reader.h"
#include "tensorflow/core/framework/graph.pb.h"
#include "tensorflow/core/lib/core/errors.h"
#include "tensorflow/core/profiler/internal/advisor/tfprof_advisor.h"
#include "tensorflow/core/profiler/internal/tfprof_stats.h"
#include "tensorflow/core/profiler/tfprof_log.pb.h"
#include "tensorflow/core/profiler/tfprof_options.h"
#include "tensorflow/core/profiler/tfprof_options.pb.h"
#include "tensorflow/core/profiler/tfprof_output.pb.h"
#include "tensorflow/core/protobuf/config.pb.h"

namespace tensorflow {
namespace tfprof {
namespace {
TFStats* tf_stat = nullptr;

string RunProfile(const string& command, const string& options,
                  TFStats* tf_stats) {
  if (command == kCmds[4]) {
    AdvisorOptionsProto option_pb;
    if (!option_pb.ParseFromString(options)) {
      fprintf(stderr, "Cannot parse AdvisorOptionsProto\n");
      return "";
    }
    tf_stats->BuildAllViews();
    return Advisor(tf_stats).Advise(option_pb).SerializeAsString();
  } else {
    tf_stats->BuildView(command);
  }

  Options opts;
  tensorflow::Status s = Options::FromProtoStr(options, &opts);
  if (!s.ok()) {
    fprintf(stderr, "%s\n", s.ToString().c_str());
    return "";
  }

  if (opts.output_type == kOutput[1]) {
    printf("\n=========================Options=============================\n");
    printf("%s", opts.ToString().c_str());
    printf("\n==================Model Analysis Report======================\n");
    string ret = "";
    if (command == kCmds[2] || command == kCmds[3]) {
      ret = tf_stats->ShowMultiGraphNode(command, opts).SerializeAsString();
    } else if (command == kCmds[0] || command == kCmds[1]) {
      ret = tf_stats->ShowGraphNode(command, opts).SerializeAsString();
    } else {
      fprintf(stderr, "Unknown command: %s\n", command.c_str());
    }
    printf("\n======================End of Report==========================\n");
    fflush(stdout);
    return ret;
  }
  if (command == kCmds[2] || command == kCmds[3]) {
    return tf_stats->ShowMultiGraphNode(command, opts).SerializeAsString();
  } else if (command == kCmds[0] || command == kCmds[1]) {
    return tf_stats->ShowGraphNode(command, opts).SerializeAsString();
  } else {
    fprintf(stderr, "Unknown command: %s\n", command.c_str());
    return "";
  }
}
}  // namespace

bool NewProfiler(const string* graph, const string* op_log) {
  std::unique_ptr<GraphDef> graph_ptr(new GraphDef());
  if (graph && !graph->empty()) {
    if (!graph_ptr->ParseFromString(*graph)) {
      if (!protobuf::TextFormat::ParseFromString(*graph, graph_ptr.get())) {
        fprintf(stderr, "Failed to parse graph\n");
        return false;
      }
    }
  }

  std::unique_ptr<OpLogProto> op_log_ptr;
  if (op_log && !op_log->empty()) {
    op_log_ptr.reset(new OpLogProto());
    if (!op_log_ptr->ParseFromString(*op_log)) {
      fprintf(stderr, "Failed to parse OpLogProto.\n");
      return false;
    }
  }
  tf_stat = new TFStats(std::move(graph_ptr), nullptr, std::move(op_log_ptr),
                        nullptr);
  return true;
}

void ProfilerFromFile(const string* filename) {
  CHECK(!tf_stat) << "Currently only 1 living tfprof profiler is allowed";
  CHECK(filename) << "Missing profile filename to init profiler from file";
  tf_stat = new TFStats(*filename, nullptr);
}

void DeleteProfiler() {
  if (tf_stat) {
    delete tf_stat;
    tf_stat = nullptr;
  }
}

double AddStep(int64 step, const string* graph, const string* run_meta,
               const string* op_log) {
  CHECK(tf_stat);

  if (graph && !graph->empty()) {
    std::unique_ptr<GraphDef> graph_ptr(new GraphDef());
    if (!graph_ptr->ParseFromString(*graph)) {
      if (!protobuf::TextFormat::ParseFromString(*graph, graph_ptr.get())) {
        fprintf(stderr, "Failed to parse graph\n");
      }
    }
    tf_stat->AddGraph(std::move(graph_ptr));
  }

  CHECK(run_meta && !run_meta->empty());
  // TODO(xpan): Better error handling.
  std::unique_ptr<RunMetadata> run_meta_ptr(new RunMetadata());
  run_meta_ptr->ParseFromString(*run_meta);
  tf_stat->AddRunMeta(step, std::move(run_meta_ptr));

  if (op_log && !op_log->empty()) {
    std::unique_ptr<OpLogProto> op_log_ptr;
    op_log_ptr.reset(new OpLogProto());
    op_log_ptr->ParseFromString(*op_log);
    tf_stat->AddOpLogProto(std::move(op_log_ptr));
  }
  return tf_stat->run_coverage();
}

string Profile(const string* command, const string* options) {
  CHECK(tf_stat);
  CHECK(command) << "command mustn't be null";
  CHECK(options) << "options mustn't be null";
  return RunProfile(*command, *options, tf_stat);
}

string SerializeToString() {
  CHECK(tf_stat);
  string content;
  tf_stat->SerializeToString(&content);
  return content;
}

void WriteProfile(const string* filename) {
  CHECK(tf_stat);
  CHECK(filename) << "empty file name when asking to write profile.";
  tf_stat->WriteProfile(*filename);
}

string PrintModelAnalysis(const string* graph, const string* run_meta,
                          const string* op_log, const string* command,
                          const string* options) {
  CHECK(command) << "command mustn't be null";
  CHECK(options) << "options mustn't be null";
  std::unique_ptr<GraphDef> graph_ptr(new GraphDef());
  if (graph && !graph->empty()) {
    graph_ptr->ParseFromString(*graph);
  }

  std::unique_ptr<RunMetadata> run_meta_ptr;
  if (run_meta && !run_meta->empty()) {
    run_meta_ptr.reset(new RunMetadata());
    run_meta_ptr->ParseFromString(*run_meta);
  }

  std::unique_ptr<OpLogProto> op_log_ptr;
  if (op_log && !op_log->empty()) {
    op_log_ptr.reset(new OpLogProto());
    op_log_ptr->ParseFromString(*op_log);
  }

  // TODO(xpan): Maybe need to init the checkpoint reader?
  std::unique_ptr<checkpoint::CheckpointReader> ckpt_reader;

  TFStats tf_stats(std::move(graph_ptr), std::move(run_meta_ptr),
                   std::move(op_log_ptr), std::move(ckpt_reader));

  return RunProfile(*command, *options, &tf_stats);
}

}  // namespace tfprof
}  // namespace tensorflow

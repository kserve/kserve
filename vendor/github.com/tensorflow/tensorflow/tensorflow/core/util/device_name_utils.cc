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

#include "tensorflow/core/util/device_name_utils.h"

#include "tensorflow/core/lib/core/errors.h"
#include "tensorflow/core/lib/strings/str_util.h"
#include "tensorflow/core/lib/strings/strcat.h"
#include "tensorflow/core/platform/logging.h"

namespace tensorflow {

static bool IsAlpha(char c) {
  return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z');
}

static bool IsAlphaNum(char c) { return IsAlpha(c) || (c >= '0' && c <= '9'); }

// Returns true iff "in" is a valid job name.
static bool IsJobName(StringPiece in) {
  if (in.empty()) return false;
  if (!IsAlpha(in[0])) return false;
  for (size_t i = 1; i < in.size(); ++i) {
    if (!(IsAlphaNum(in[i]) || in[i] == '_')) return false;
  }
  return true;
}

// Returns true and fills in "*job" iff "*in" starts with a job name.
static bool ConsumeJobName(StringPiece* in, string* job) {
  if (in->empty()) return false;
  if (!IsAlpha((*in)[0])) return false;
  size_t i = 1;
  for (; i < in->size(); ++i) {
    const char c = (*in)[i];
    if (c == '/') break;
    if (!(IsAlphaNum(c) || c == '_')) {
      return false;
    }
  }
  job->assign(in->data(), i);
  in->remove_prefix(i);
  return true;
}

// Returns true and fills in "*device_type" iff "*in" starts with a device type
// name.
static bool ConsumeDeviceType(StringPiece* in, string* device_type) {
  if (in->empty()) return false;
  if (!IsAlpha((*in)[0])) return false;
  size_t i = 1;
  for (; i < in->size(); ++i) {
    const char c = (*in)[i];
    if (c == '/' || c == ':') break;
    if (!(IsAlphaNum(c) || c == '_')) {
      return false;
    }
  }
  device_type->assign(in->data(), i);
  in->remove_prefix(i);
  return true;
}

// Returns true and fills in "*val" iff "*in" starts with a decimal
// number.
static bool ConsumeNumber(StringPiece* in, int* val) {
  uint64 tmp;
  if (str_util::ConsumeLeadingDigits(in, &tmp)) {
    *val = tmp;
    return true;
  } else {
    return false;
  }
}

// Returns a fully qualified device name given the parameters.
static string DeviceName(const string& job, int replica, int task,
                         const string& device_prefix, const string& device_type,
                         int id) {
  CHECK(IsJobName(job)) << job;
  CHECK_LE(0, replica);
  CHECK_LE(0, task);
  CHECK(!device_type.empty());
  CHECK_LE(0, id);
  return strings::StrCat("/job:", job, "/replica:", replica, "/task:", task,
                         device_prefix, device_type, ":", id);
}

/* static */
string DeviceNameUtils::FullName(const string& job, int replica, int task,
                                 const string& type, int id) {
  return DeviceName(job, replica, task, "/device:", type, id);
}

namespace {
string LegacyName(const string& job, int replica, int task, const string& type,
                  int id) {
  return DeviceName(job, replica, task, "/", str_util::Lowercase(type), id);
}
}  // anonymous namespace

bool DeviceNameUtils::ParseFullName(StringPiece fullname, ParsedName* p) {
  p->Clear();
  if (fullname == "/") {
    return true;
  }
  while (!fullname.empty()) {
    bool progress = false;
    if (str_util::ConsumePrefix(&fullname, "/job:")) {
      p->has_job = !str_util::ConsumePrefix(&fullname, "*");
      if (p->has_job && !ConsumeJobName(&fullname, &p->job)) {
        return false;
      }
      progress = true;
    }
    if (str_util::ConsumePrefix(&fullname, "/replica:")) {
      p->has_replica = !str_util::ConsumePrefix(&fullname, "*");
      if (p->has_replica && !ConsumeNumber(&fullname, &p->replica)) {
        return false;
      }
      progress = true;
    }
    if (str_util::ConsumePrefix(&fullname, "/task:")) {
      p->has_task = !str_util::ConsumePrefix(&fullname, "*");
      if (p->has_task && !ConsumeNumber(&fullname, &p->task)) {
        return false;
      }
      progress = true;
    }
    if (str_util::ConsumePrefix(&fullname, "/device:")) {
      p->has_type = !str_util::ConsumePrefix(&fullname, "*");
      if (p->has_type && !ConsumeDeviceType(&fullname, &p->type)) {
        return false;
      }
      if (!str_util::ConsumePrefix(&fullname, ":")) {
        p->has_id = false;
      } else {
        p->has_id = !str_util::ConsumePrefix(&fullname, "*");
        if (p->has_id && !ConsumeNumber(&fullname, &p->id)) {
          return false;
        }
      }
      progress = true;
    }

    // Handle legacy naming convention for cpu and gpu.
    if (str_util::ConsumePrefix(&fullname, "/cpu:") ||
        str_util::ConsumePrefix(&fullname, "/CPU:")) {
      p->has_type = true;
      p->type = "CPU";  // Treat '/cpu:..' as uppercase '/device:CPU:...'
      p->has_id = !str_util::ConsumePrefix(&fullname, "*");
      if (p->has_id && !ConsumeNumber(&fullname, &p->id)) {
        return false;
      }
      progress = true;
    }
    if (str_util::ConsumePrefix(&fullname, "/gpu:") ||
        str_util::ConsumePrefix(&fullname, "/GPU:")) {
      p->has_type = true;
      p->type = "GPU";  // Treat '/gpu:..' as uppercase '/device:GPU:...'
      p->has_id = !str_util::ConsumePrefix(&fullname, "*");
      if (p->has_id && !ConsumeNumber(&fullname, &p->id)) {
        return false;
      }
      progress = true;
    }

    if (!progress) {
      return false;
    }
  }
  return true;
}

namespace {

void CompleteName(const DeviceNameUtils::ParsedName& parsed_basename,
                  DeviceNameUtils::ParsedName* parsed_name) {
  if (!parsed_name->has_job) {
    parsed_name->job = parsed_basename.job;
    parsed_name->has_job = true;
  }
  if (!parsed_name->has_replica) {
    parsed_name->replica = parsed_basename.replica;
    parsed_name->has_replica = true;
  }
  if (!parsed_name->has_task) {
    parsed_name->task = parsed_basename.task;
    parsed_name->has_task = true;
  }
  if (!parsed_name->has_type) {
    parsed_name->type = parsed_basename.type;
    parsed_name->has_type = true;
  }
  if (!parsed_name->has_id) {
    parsed_name->id = parsed_basename.id;
    parsed_name->has_id = true;
  }
}

}  // namespace

/* static */
Status DeviceNameUtils::CanonicalizeDeviceName(StringPiece fullname,
                                               StringPiece basename,
                                               string* canonical_name) {
  *canonical_name = "";
  ParsedName parsed_basename;
  if (!ParseFullName(basename, &parsed_basename)) {
    return errors::InvalidArgument("Could not parse basename: ", basename,
                                   " into a device specification.");
  }
  if (!(parsed_basename.has_job && parsed_basename.has_replica &&
        parsed_basename.has_task && parsed_basename.has_type &&
        parsed_basename.has_id)) {
    return errors::InvalidArgument("Basename: ", basename,
                                   " should be fully "
                                   "specified.");
  }
  ParsedName parsed_name;
  if (ParseLocalName(fullname, &parsed_name)) {
    CompleteName(parsed_basename, &parsed_name);
    *canonical_name = ParsedNameToString(parsed_name);
    return Status::OK();
  }
  if (ParseFullName(fullname, &parsed_name)) {
    CompleteName(parsed_basename, &parsed_name);
    *canonical_name = ParsedNameToString(parsed_name);
    return Status::OK();
  }
  return errors::InvalidArgument("Could not parse ", fullname,
                                 " into a device "
                                 "specification.");
}

/* static */
string DeviceNameUtils::ParsedNameToString(const ParsedName& pn) {
  string buf;
  if (pn.has_job) strings::StrAppend(&buf, "/job:", pn.job);
  if (pn.has_replica) strings::StrAppend(&buf, "/replica:", pn.replica);
  if (pn.has_task) strings::StrAppend(&buf, "/task:", pn.task);
  if (pn.has_type) {
    strings::StrAppend(&buf, "/device:", pn.type, ":");
    if (pn.has_id) {
      strings::StrAppend(&buf, pn.id);
    } else {
      strings::StrAppend(&buf, "*");
    }
  }
  return buf;
}

/* static */
bool DeviceNameUtils::IsSpecification(const ParsedName& less_specific,
                                      const ParsedName& more_specific) {
  if (less_specific.has_job &&
      (!more_specific.has_job || (less_specific.job != more_specific.job))) {
    return false;
  }
  if (less_specific.has_replica &&
      (!more_specific.has_replica ||
       (less_specific.replica != more_specific.replica))) {
    return false;
  }
  if (less_specific.has_task &&
      (!more_specific.has_task || (less_specific.task != more_specific.task))) {
    return false;
  }
  if (less_specific.has_type &&
      (!more_specific.has_type || (less_specific.type != more_specific.type))) {
    return false;
  }
  if (less_specific.has_id &&
      (!more_specific.has_id || (less_specific.id != more_specific.id))) {
    return false;
  }
  return true;
}

/* static */
bool DeviceNameUtils::IsCompleteSpecification(const ParsedName& pattern,
                                              const ParsedName& name) {
  CHECK(name.has_job && name.has_replica && name.has_task && name.has_type &&
        name.has_id);

  if (pattern.has_job && (pattern.job != name.job)) return false;
  if (pattern.has_replica && (pattern.replica != name.replica)) return false;
  if (pattern.has_task && (pattern.task != name.task)) return false;
  if (pattern.has_type && (pattern.type != name.type)) return false;
  if (pattern.has_id && (pattern.id != name.id)) return false;
  return true;
}

/* static */
Status DeviceNameUtils::MergeDevNames(ParsedName* target,
                                      const ParsedName& other,
                                      bool allow_soft_placement) {
  if (other.has_job) {
    if (target->has_job && target->job != other.job) {
      return errors::InvalidArgument(
          "Cannot merge devices with incompatible jobs: '",
          ParsedNameToString(*target), "' and '", ParsedNameToString(other),
          "'");
    } else {
      target->has_job = other.has_job;
      target->job = other.job;
    }
  }

  if (other.has_replica) {
    if (target->has_replica && target->replica != other.replica) {
      return errors::InvalidArgument(
          "Cannot merge devices with incompatible replicas: '",
          ParsedNameToString(*target), "' and '", ParsedNameToString(other),
          "'");
    } else {
      target->has_replica = other.has_replica;
      target->replica = other.replica;
    }
  }

  if (other.has_task) {
    if (target->has_task && target->task != other.task) {
      return errors::InvalidArgument(
          "Cannot merge devices with incompatible tasks: '",
          ParsedNameToString(*target), "' and '", ParsedNameToString(other),
          "'");
    } else {
      target->has_task = other.has_task;
      target->task = other.task;
    }
  }

  if (other.has_type) {
    if (target->has_type && target->type != other.type) {
      if (!allow_soft_placement) {
        return errors::InvalidArgument(
            "Cannot merge devices with incompatible types: '",
            ParsedNameToString(*target), "' and '", ParsedNameToString(other),
            "'");
      } else {
        target->has_id = false;
        target->has_type = false;
        return Status::OK();
      }
    } else {
      target->has_type = other.has_type;
      target->type = other.type;
    }
  }

  if (other.has_id) {
    if (target->has_id && target->id != other.id) {
      if (!allow_soft_placement) {
        return errors::InvalidArgument(
            "Cannot merge devices with incompatible ids: '",
            ParsedNameToString(*target), "' and '", ParsedNameToString(other),
            "'");
      } else {
        target->has_id = false;
        return Status::OK();
      }
    } else {
      target->has_id = other.has_id;
      target->id = other.id;
    }
  }

  return Status::OK();
}

/* static */
bool DeviceNameUtils::IsSameAddressSpace(const ParsedName& a,
                                         const ParsedName& b) {
  return (a.has_job && b.has_job && (a.job == b.job)) &&
         (a.has_replica && b.has_replica && (a.replica == b.replica)) &&
         (a.has_task && b.has_task && (a.task == b.task));
}

/* static */
bool DeviceNameUtils::IsSameAddressSpace(StringPiece src, StringPiece dst) {
  ParsedName x;
  ParsedName y;
  return ParseFullName(src, &x) && ParseFullName(dst, &y) &&
         IsSameAddressSpace(x, y);
}

/* static */
string DeviceNameUtils::LocalName(StringPiece type, int id) {
  return strings::StrCat("/device:", type, ":", id);
}

namespace {
// Returns the legacy local device name given its "type" and "id" (which is
// '/device:type:id').
string LegacyLocalName(StringPiece type, int id) {
  return strings::StrCat(type, ":", id);
}
}  // anonymous namespace

/* static */
string DeviceNameUtils::LocalName(StringPiece fullname) {
  ParsedName x;
  CHECK(ParseFullName(fullname, &x)) << fullname;
  return LocalName(x.type, x.id);
}

/* static */
bool DeviceNameUtils::ParseLocalName(StringPiece name, ParsedName* p) {
  if (!ConsumeDeviceType(&name, &p->type)) {
    return false;
  }
  p->has_type = true;
  if (!str_util::ConsumePrefix(&name, ":")) {
    return false;
  }
  if (!ConsumeNumber(&name, &p->id)) {
    return false;
  }
  p->has_id = true;
  return name.empty();
}

/* static */
bool DeviceNameUtils::SplitDeviceName(StringPiece name, string* task,
                                      string* device) {
  ParsedName pn;
  if (ParseFullName(name, &pn) && pn.has_type && pn.has_id) {
    task->clear();
    task->reserve(
        (pn.has_job ? (5 + pn.job.size()) : 0) +
        (pn.has_replica ? (9 + 4 /*estimated UB for # replica digits*/) : 0) +
        (pn.has_task ? (6 + 4 /*estimated UB for # task digits*/) : 0));
    if (pn.has_job) {
      strings::StrAppend(task, "/job:", pn.job);
    }
    if (pn.has_replica) {
      strings::StrAppend(task, "/replica:", pn.replica);
    }
    if (pn.has_task) {
      strings::StrAppend(task, "/task:", pn.task);
    }
    device->clear();
    strings::StrAppend(device, pn.type, ":", pn.id);
    return true;
  }
  return false;
}

std::vector<string> DeviceNameUtils::GetNamesForDeviceMappings(
    const ParsedName& pn) {
  if (pn.has_job && pn.has_replica && pn.has_task && pn.has_type && pn.has_id) {
    return {
        DeviceNameUtils::FullName(pn.job, pn.replica, pn.task, pn.type, pn.id),
        LegacyName(pn.job, pn.replica, pn.task, pn.type, pn.id)};
  } else {
    return {};
  }
}

std::vector<string> DeviceNameUtils::GetLocalNamesForDeviceMappings(
    const ParsedName& pn) {
  if (pn.has_type && pn.has_id) {
    return {DeviceNameUtils::LocalName(pn.type, pn.id),
            LegacyLocalName(pn.type, pn.id)};
  } else {
    return {};
  }
}

/*static*/ Status DeviceNameUtils::DeviceNameToCpuDeviceName(
    const string& device_name, string* host_device_name) {
  DeviceNameUtils::ParsedName device;
  if (!DeviceNameUtils::ParseFullName(device_name, &device)) {
    return errors::Internal("Could not parse device name ", device_name);
  }
  device.type = "CPU";
  device.id = 0;
  *host_device_name = DeviceNameUtils::ParsedNameToString(device);
  return Status::OK();
}

}  // namespace tensorflow

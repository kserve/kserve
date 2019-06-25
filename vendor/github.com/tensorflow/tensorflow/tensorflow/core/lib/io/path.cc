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

#include "tensorflow/core/lib/io/path.h"

#include <errno.h>
#include <fcntl.h>
#include <stdlib.h>
#include <sys/stat.h>
#include <sys/types.h>
#if !defined(PLATFORM_WINDOWS)
#include <unistd.h>
#endif

#include <vector>

#include "tensorflow/core/lib/strings/scanner.h"
#include "tensorflow/core/lib/strings/strcat.h"
#include "tensorflow/core/platform/logging.h"
#include "tensorflow/core/platform/mutex.h"

namespace tensorflow {
namespace io {
namespace internal {

string JoinPathImpl(std::initializer_list<StringPiece> paths) {
  string result;

  for (StringPiece path : paths) {
    if (path.empty()) continue;

    if (result.empty()) {
      result = string(path);
      continue;
    }

    if (result[result.size() - 1] == '/') {
      if (IsAbsolutePath(path)) {
        strings::StrAppend(&result, path.substr(1));
      } else {
        strings::StrAppend(&result, path);
      }
    } else {
      if (IsAbsolutePath(path)) {
        strings::StrAppend(&result, path);
      } else {
        strings::StrAppend(&result, "/", path);
      }
    }
  }

  return result;
}

// Return the parts of the URI, split on the final "/" in the path. If there is
// no "/" in the path, the first part of the output is the scheme and host, and
// the second is the path. If the only "/" in the path is the first character,
// it is included in the first part of the output.
std::pair<StringPiece, StringPiece> SplitPath(StringPiece uri) {
  StringPiece scheme, host, path;
  ParseURI(uri, &scheme, &host, &path);

  auto pos = path.rfind('/');
#ifdef PLATFORM_WINDOWS
  if (pos == StringPiece::npos) pos = path.rfind('\\');
#endif
  // Handle the case with no '/' in 'path'.
  if (pos == StringPiece::npos)
    return std::make_pair(StringPiece(uri.begin(), host.end() - uri.begin()),
                          path);

  // Handle the case with a single leading '/' in 'path'.
  if (pos == 0)
    return std::make_pair(
        StringPiece(uri.begin(), path.begin() + 1 - uri.begin()),
        StringPiece(path.data() + 1, path.size() - 1));

  return std::make_pair(
      StringPiece(uri.begin(), path.begin() + pos - uri.begin()),
      StringPiece(path.data() + pos + 1, path.size() - (pos + 1)));
}

// Return the parts of the basename of path, split on the final ".".
// If there is no "." in the basename or "." is the final character in the
// basename, the second value will be empty.
std::pair<StringPiece, StringPiece> SplitBasename(StringPiece path) {
  path = Basename(path);

  auto pos = path.rfind('.');
  if (pos == StringPiece::npos)
    return std::make_pair(path, StringPiece(path.data() + path.size(), 0));
  return std::make_pair(
      StringPiece(path.data(), pos),
      StringPiece(path.data() + pos + 1, path.size() - (pos + 1)));
}
}  // namespace internal

bool IsAbsolutePath(StringPiece path) {
  return !path.empty() && path[0] == '/';
}

StringPiece Dirname(StringPiece path) {
  return internal::SplitPath(path).first;
}

StringPiece Basename(StringPiece path) {
  return internal::SplitPath(path).second;
}

StringPiece Extension(StringPiece path) {
  return internal::SplitBasename(path).second;
}

string CleanPath(StringPiece unclean_path) {
  string path(unclean_path);
  const char* src = path.c_str();
  string::iterator dst = path.begin();

  // Check for absolute path and determine initial backtrack limit.
  const bool is_absolute_path = *src == '/';
  if (is_absolute_path) {
    *dst++ = *src++;
    while (*src == '/') ++src;
  }
  string::const_iterator backtrack_limit = dst;

  // Process all parts
  while (*src) {
    bool parsed = false;

    if (src[0] == '.') {
      //  1dot ".<whateverisnext>", check for END or SEP.
      if (src[1] == '/' || !src[1]) {
        if (*++src) {
          ++src;
        }
        parsed = true;
      } else if (src[1] == '.' && (src[2] == '/' || !src[2])) {
        // 2dot END or SEP (".." | "../<whateverisnext>").
        src += 2;
        if (dst != backtrack_limit) {
          // We can backtrack the previous part
          for (--dst; dst != backtrack_limit && dst[-1] != '/'; --dst) {
            // Empty.
          }
        } else if (!is_absolute_path) {
          // Failed to backtrack and we can't skip it either. Rewind and copy.
          src -= 2;
          *dst++ = *src++;
          *dst++ = *src++;
          if (*src) {
            *dst++ = *src;
          }
          // We can never backtrack over a copied "../" part so set new limit.
          backtrack_limit = dst;
        }
        if (*src) {
          ++src;
        }
        parsed = true;
      }
    }

    // If not parsed, copy entire part until the next SEP or EOS.
    if (!parsed) {
      while (*src && *src != '/') {
        *dst++ = *src++;
      }
      if (*src) {
        *dst++ = *src++;
      }
    }

    // Skip consecutive SEP occurrences
    while (*src == '/') {
      ++src;
    }
  }

  // Calculate and check the length of the cleaned path.
  string::difference_type path_length = dst - path.begin();
  if (path_length != 0) {
    // Remove trailing '/' except if it is root path ("/" ==> path_length := 1)
    if (path_length > 1 && path[path_length - 1] == '/') {
      --path_length;
    }
    path.resize(path_length);
  } else {
    // The cleaned path is empty; assign "." as per the spec.
    path.assign(1, '.');
  }
  return path;
}

void ParseURI(StringPiece remaining, StringPiece* scheme, StringPiece* host,
              StringPiece* path) {
  // 0. Parse scheme
  // Make sure scheme matches [a-zA-Z][0-9a-zA-Z.]*
  // TODO(keveman): Allow "+" and "-" in the scheme.
  // Keep URI pattern in tensorboard/backend/server.py updated accordingly
  if (!strings::Scanner(remaining)
           .One(strings::Scanner::LETTER)
           .Many(strings::Scanner::LETTER_DIGIT_DOT)
           .StopCapture()
           .OneLiteral("://")
           .GetResult(&remaining, scheme)) {
    // If there's no scheme, assume the entire string is a path.
    *scheme = StringPiece(remaining.begin(), 0);
    *host = StringPiece(remaining.begin(), 0);
    *path = remaining;
    return;
  }

  // 1. Parse host
  if (!strings::Scanner(remaining).ScanUntil('/').GetResult(&remaining, host)) {
    // No path, so the rest of the URI is the host.
    *host = remaining;
    *path = StringPiece(remaining.end(), 0);
    return;
  }

  // 2. The rest is the path
  *path = remaining;
}

string CreateURI(StringPiece scheme, StringPiece host, StringPiece path) {
  if (scheme.empty()) {
    return string(path);
  }
  return strings::StrCat(scheme, "://", host, path);
}

// Returns a unique number every time it is called.
int64 UniqueId() {
  static mutex mu(LINKER_INITIALIZED);
  static int64 id = 0;
  mutex_lock l(mu);
  return ++id;
}

string GetTempFilename(const string& extension) {
#if defined(PLATFORM_WINDOWS) || defined(__ANDROID__)
  LOG(FATAL) << "GetTempFilename is not implemented in this platform.";
#else
  for (const char* dir : std::vector<const char*>(
           {getenv("TEST_TMPDIR"), getenv("TMPDIR"), getenv("TMP"), "/tmp"})) {
    if (!dir || !dir[0]) {
      continue;
    }
    struct stat statbuf;
    if (!stat(dir, &statbuf) && S_ISDIR(statbuf.st_mode)) {
      // UniqueId is added here because mkstemps is not as thread safe as it
      // looks. https://github.com/tensorflow/tensorflow/issues/5804 shows
      // the problem.
      string tmp_filepath;
      int fd;
      if (extension.length()) {
        tmp_filepath = io::JoinPath(
            dir, strings::StrCat("tmp_file_tensorflow_", UniqueId(), "_XXXXXX.",
                                 extension));
        fd = mkstemps(&tmp_filepath[0], extension.length() + 1);
      } else {
        tmp_filepath = io::JoinPath(
            dir,
            strings::StrCat("tmp_file_tensorflow_", UniqueId(), "_XXXXXX"));
        fd = mkstemp(&tmp_filepath[0]);
      }
      if (fd < 0) {
        LOG(FATAL) << "Failed to create temp file.";
      } else {
        close(fd);
        return tmp_filepath;
      }
    }
  }
  LOG(FATAL) << "No temp directory found.";
#endif
}

}  // namespace io
}  // namespace tensorflow

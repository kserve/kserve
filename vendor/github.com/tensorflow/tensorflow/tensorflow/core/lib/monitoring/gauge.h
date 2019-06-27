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

#ifndef TENSORFLOW_CORE_LIB_MONITORING_GAUGE_H_
#define TENSORFLOW_CORE_LIB_MONITORING_GAUGE_H_

// We replace this implementation with a null implementation for mobile
// platforms.
#include "tensorflow/core/platform/platform.h"
#ifdef IS_MOBILE_PLATFORM
#include "tensorflow/core/lib/monitoring/mobile_gauge.h"
#else

#include <array>
#include <atomic>
#include <map>

#include "tensorflow/core/lib/monitoring/collection_registry.h"
#include "tensorflow/core/lib/monitoring/metric_def.h"
#include "tensorflow/core/platform/macros.h"
#include "tensorflow/core/platform/mutex.h"
#include "tensorflow/core/platform/thread_annotations.h"
#include "tensorflow/core/platform/types.h"

namespace tensorflow {
namespace monitoring {

// GaugeCell stores each value of a gauge.
//
// A cell can be passed off to a module which may repeatedly update it without
// needing further map-indexing computations. This improves both encapsulation
// (separate modules can own a cell each, without needing to know about the map
// to which both cells belong) and performance (since map indexing and
// associated locking are both avoided).
//
// This class is thread-safe.
template <typename T>
class GaugeCell {
 public:
  explicit GaugeCell(const T& value) : value_(value) {}
  ~GaugeCell() {}

  // Atomically sets the value.
  void Set(const T& value) LOCKS_EXCLUDED(mu_);

  // Retrieves the current value.
  T value() const LOCKS_EXCLUDED(mu_);

 private:
  T value_ GUARDED_BY(mu_);
  mutable mutex mu_;

  TF_DISALLOW_COPY_AND_ASSIGN(GaugeCell);
};

// Explicit specialization of GaugeCell<int64>. Compared to the primary
// template, it uses atomic values as opposed to mutex. This class is
// thread-safe.
template <>
class GaugeCell<int64> {
 public:
  explicit GaugeCell(int64 value) : value_(value) {}
  ~GaugeCell() {}

  // Atomically sets the value.
  void Set(int64 value);

  // Retrieves the current value.
  int64 value() const;

 private:
  std::atomic<int64> value_;

  TF_DISALLOW_COPY_AND_ASSIGN(GaugeCell);
};

// Explicit specialization of GaugeCell<bool>. Compared to the primary
// template, it uses atomic values as opposed to mutex. This class is
// thread-safe.
template <>
class GaugeCell<bool> {
 public:
  explicit GaugeCell(bool value) : value_(value) {}
  ~GaugeCell() {}

  // Atomically sets the value.
  void Set(bool value);

  // Retrieves the current value.
  bool value() const;

 private:
  std::atomic<bool> value_;

  TF_DISALLOW_COPY_AND_ASSIGN(GaugeCell);
};

// A stateful class for updating a gauge-like metric. Allowed ValueType are
// int64, string and bool.
//
// This class encapsulates a set of values (or a single value for a label-less
// metric). Each value is identified by a tuple of labels. The class allows the
// user to set each value.
//
// Gauge allocates storage and maintains a cell for each value. You can
// retrieve an individual cell using a label-tuple and update it separately.
// This improves performance since operations related to retrieval, like
// map-indexing and locking, are avoided.
//
// This class is thread-safe.
template <typename ValueType, int NumLabels>
class Gauge {
 public:
  ~Gauge() {
    // Deleted here, before the metric_def is destroyed.
    registration_handle_.reset();
  }

  // Creates the metric based on the metric-definition arguments.
  //
  // Example:
  //
  // auto* string_gauge_with_label = Gauge<string,1>::New(
  //   "/tensorflow/string_gauge_with_label",
  //   "String gauge with one label.", "MyLabelName");
  //
  // auto* integer_gauge = Gauge<int64, 0>::New("/tensorflow/integer_gauge",
  //   "Integer gauge")
  //
  // auto* bool_gauge = Gauge<bool, 0>::New("/tensorflow/bool_gauge",
  //   "Bool gauge")
  template <typename... MetricDefArgs>
  static Gauge* New(MetricDefArgs&&... metric_def_args);

  // Retrieves the cell for the specified labels, creating it on demand if not
  // already present.
  template <typename... Labels>
  GaugeCell<ValueType>* GetCell(const Labels&... labels) LOCKS_EXCLUDED(mu_);

 private:
  explicit Gauge(
      const MetricDef<MetricKind::kGauge, ValueType, NumLabels>& metric_def)
      : metric_def_(metric_def),
        registration_handle_(CollectionRegistry::Default()->Register(
            &metric_def_, [&](MetricCollectorGetter getter) {
              auto metric_collector = getter.Get(&metric_def_);

              mutex_lock l(mu_);
              for (const auto& cell : cells_) {
                metric_collector.CollectValue(cell.first, cell.second.value());
              }
            })) {}

  mutable mutex mu_;

  // The metric definition. This will be used to identify the metric when we
  // register it for collection.
  const MetricDef<MetricKind::kGauge, ValueType, NumLabels> metric_def_;

  std::unique_ptr<CollectionRegistry::RegistrationHandle> registration_handle_;

  using LabelArray = std::array<string, NumLabels>;
  std::map<LabelArray, GaugeCell<ValueType> > cells_ GUARDED_BY(mu_);

  TF_DISALLOW_COPY_AND_ASSIGN(Gauge);
};

////
//  Implementation details follow. API readers may skip.
////
template <typename T>
void GaugeCell<T>::Set(const T& value) {
  mutex_lock l(mu_);
  value_ = value;
}

template <typename T>
T GaugeCell<T>::value() const {
  mutex_lock l(mu_);
  return value_;
}

inline void GaugeCell<int64>::Set(int64 value) { value_ = value; }

inline int64 GaugeCell<int64>::value() const { return value_; }

inline void GaugeCell<bool>::Set(bool value) { value_ = value; }

inline bool GaugeCell<bool>::value() const { return value_; }

template <typename ValueType, int NumLabels>
template <typename... MetricDefArgs>
Gauge<ValueType, NumLabels>* Gauge<ValueType, NumLabels>::New(
    MetricDefArgs&&... metric_def_args) {
  static_assert(std::is_same<ValueType, int64>::value ||
                    std::is_same<ValueType, string>::value ||
                    std::is_same<ValueType, bool>::value,
                "Gauge only allows int64 and string types.");
  return new Gauge<ValueType, NumLabels>(
      MetricDef<MetricKind::kGauge, ValueType, NumLabels>(
          std::forward<MetricDefArgs>(metric_def_args)...));
}

template <typename ValueType, int NumLabels>
template <typename... Labels>
GaugeCell<ValueType>* Gauge<ValueType, NumLabels>::GetCell(
    const Labels&... labels) LOCKS_EXCLUDED(mu_) {
  // Provides a more informative error message than the one during array
  // construction below.
  static_assert(
      sizeof...(Labels) == NumLabels,
      "Mismatch between Gauge<ValueType, NumLabels> and number of labels "
      "provided in GetCell(...).");

  const LabelArray& label_array = {{labels...}};
  mutex_lock l(mu_);
  const auto found_it = cells_.find(label_array);
  if (found_it != cells_.end()) {
    return &(found_it->second);
  }
  return &(cells_
               .emplace(std::piecewise_construct,
                        std::forward_as_tuple(label_array),
                        std::forward_as_tuple(ValueType()))
               .first->second);
}

}  // namespace monitoring
}  // namespace tensorflow

#endif  // IS_MOBILE_PLATFORM
#endif  // TENSORFLOW_CORE_LIB_MONITORING_GAUGE_H_

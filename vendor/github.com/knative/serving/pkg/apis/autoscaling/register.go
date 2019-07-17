/*
Copyright 2018 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package autoscaling

import "time"

const (
	InternalGroupName = "autoscaling.internal.knative.dev"

	GroupName = "autoscaling.knative.dev"

	// ClassAnnotationKey is the annotation for the explicit class of autoscaler
	// that a particular resource has opted into. For example,
	//   autoscaling.knative.dev/class: foo
	// This uses a different domain because unlike the resource, it is user-facing.
	ClassAnnotationKey = GroupName + "/class"
	// KPA is Knative Horizontal Pod Autoscaler
	KPA = "kpa.autoscaling.knative.dev"
	// HPA is Kubernetes Horizontal Pod Autoscaler
	HPA = "hpa.autoscaling.knative.dev"

	// MinScaleAnnotationKey is the annotation to specify the minimum number of Pods
	// the PodAutoscaler should provision. For example,
	//   autoscaling.knative.dev/minScale: "1"
	MinScaleAnnotationKey = GroupName + "/minScale"
	// MaxScaleAnnotationKey is the annotation to specify the maximum number of Pods
	// the PodAutoscaler should provision. For example,
	//   autoscaling.knative.dev/maxScale: "10"
	MaxScaleAnnotationKey = GroupName + "/maxScale"

	// MetricAnnotationKey is the annotation to specify what metric the PodAutoscaler
	// should be scaled on. For example,
	//   autoscaling.knative.dev/metric: cpu
	MetricAnnotationKey = GroupName + "/metric"
	// Concurrency is the number of requests in-flight at any given time.
	Concurrency = "concurrency"
	// CPU is the amount of the requested cpu actually being consumed by the Pod.
	CPU = "cpu"

	// TargetAnnotationKey is the annotation to specify what metric value the
	// PodAutoscaler should attempt to maintain. For example,
	//   autoscaling.knative.dev/metric: cpu
	//   autoscaling.knative.dev/target: "75"   # target 75% cpu utilization
	TargetAnnotationKey = GroupName + "/target"
	// TargetMin is the minimum allowable target. Values less than
	// zero don't make sense.
	TargetMin = 1

	// WindowAnnotationKey is the annotation to specify the time
	// interval over which to calculate the average metric.  Larger
	// values result in more smoothing. For example,
	//   autoscaling.knative.dev/metric: concurrency
	//   autoscaling.knative.dev/window: "2m"
	// Only the kpa.autoscaling.knative.dev class autoscaler supports
	// the window annotation.
	WindowAnnotationKey = GroupName + "/window"
	// WindowMin is the minimum allowable stable autoscaling
	// window. KPA-class autoscalers calculate the desired replica
	// count every 2 seconds (tick-interval in config-autoscaler) so
	// the closer the window gets to that value, the more likely data
	// points will be missed entirely by the panic window which is
	// smaller than the stable window. Anything less than 6 second
	// isn't going to work well.
	WindowMin = 6 * time.Second

	// PanicWindowPercentageAnnotationKey is the annotation to
	// specify the time interval over which to calculate the average
	// metric during a spike. Where a spike is defined as the metric
	// reaching panic level within the panic window (e.g. panic
	// mode). Lower values make panic mode more sensitive. Note:
	// Panic threshold can be overridden with the
	// PanicThresholdPercentageAnnotationKey. For example,
	//   autoscaling.knative.dev/panicWindowPercentage: "5.0"
	//   autoscaling.knative.dev/panicThresholdPercentage: "150.0"
	// Only the kpa.autoscaling.knative.dev class autoscaler supports
	// the panicWindowPercentage annotation.
	// Panic window is specified as a percentag to maintain the
	// autoscaler's algorithm behavior when only the stable window is
	// specified. The panic window will change along with the stable
	// window at the default percentage.
	PanicWindowPercentageAnnotationKey = GroupName + "/panicWindowPercentage"
	// PanicWindowPercentageMin is the minimum allowable panic window
	// percentage. The autoscaler calculates desired replicas every 2
	// seconds (tick-interval in config-autoscaler), so a panic
	// window less than 2 seconds will be missing data points. One
	// percent is a very small ratio and would require a stable
	// window of at least 3.4 minutes. Anything less doesn't make
	// sense.
	PanicWindowPercentageMin = 1.0
	// PanicWindowPercentageMax is the maximum allowable panic window
	// percentage. The KPA autoscaler's panic feature allows the
	// autoscaler to be more responsive over a smaller time scale
	// when necessary. So the panic window cannot be larger than the
	// stable window.
	PanicWindowPercentageMax = 100.0

	// PanicThresholdPercentageAnnotationKey is the annotation to specify
	// the level at what level panic mode will engage when reached within
	// in the panic window. The level is defined as a percentage of
	// the metric target. Lower values make panic mode more
	// sensitive. For example,
	//   autoscaling.knative.dev/panicWindowPercentage: "5.0"
	//   autoscaling.knative.dev/panicThresholdPercentage: "150.0"
	// Only the kpa.autoscaling.knative.dev class autoscaler supports
	// the panicThresholdPercentage annotation
	PanicThresholdPercentageAnnotationKey = GroupName + "/panicThresholdPercentage"
	// PanicThresholdPercentageMin is the minimum allowable panic
	// threshold percentage. The KPA autoscaler's panic feature
	// allows the autoscaler to be more responsive over a smaller
	// time scale when necessary. To prevent flapping, during panic
	// mode the autoscaler never decreases the number of replicas. If
	// the panic threshold was as small as the stable target, the
	// autoscaler would always be panicking and the autoscaler would
	// never scale down. One hundred and ten percent is about the
	// smallest useful value.
	PanicThresholdPercentageMin = 110.0

	// KPALabelKey is the label key attached to a K8s Service to hint to the KPA
	// which services/endpoints should trigger reconciles.
	KPALabelKey = GroupName + "/kpa"
)

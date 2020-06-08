module github.com/kubeflow/kfserving

go 1.13

require (
	cloud.google.com/go v0.47.0 // indirect
	contrib.go.opencensus.io/exporter/stackdriver v0.12.9-0.20191108183826-59d068f8d8ff // indirect
	github.com/aws/aws-sdk-go v1.28.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cloudevents/sdk-go v0.9.2
	github.com/docker/distribution v2.7.1+incompatible // indirect
	github.com/emicklei/go-restful v2.11.0+incompatible // indirect
	github.com/fatih/color v1.9.0 // indirect
	github.com/getkin/kin-openapi v0.2.0
	github.com/go-logr/logr v0.1.0
	github.com/go-logr/zapr v0.1.1 // indirect
	github.com/go-openapi/spec v0.19.4
	github.com/gogo/protobuf v1.3.1
	github.com/golang/groupcache v0.0.0-20191002201903-404acd9df4cc // indirect
	github.com/golang/protobuf v1.3.2
	github.com/google/go-cmp v0.3.1
	github.com/google/go-containerregistry v0.0.0-20190910142231-b02d448a3705 // indirect
	github.com/google/uuid v1.1.1
	github.com/imdario/mergo v0.3.8 // indirect
	github.com/mattbaird/jsonpatch v0.0.0-20171005235357-81af80346b1a // indirect
	github.com/natefinch/lumberjack v2.0.0+incompatible // indirect
	github.com/onsi/gomega v1.8.1
	github.com/opencontainers/go-digest v1.0.0-rc1 // indirect
	github.com/pkg/errors v0.8.1
	github.com/prometheus/common v0.7.0 // indirect
	github.com/prometheus/procfs v0.0.5 // indirect
	github.com/spf13/cobra v0.0.5
	github.com/tensorflow/tensorflow v1.13.1 // indirect
	go.uber.org/multierr v1.2.0 // indirect
	go.uber.org/zap v1.11.0 // indirect
	golang.org/x/lint v0.0.0-20200302205851-738671d3881b // indirect
	golang.org/x/net v0.0.0-20200226121028-0de0cce0169b
	golang.org/x/time v0.0.0-20191023065245-6d3f0bb11be5 // indirect
	golang.org/x/tools v0.0.0-20200606014950-c42cb6316fb6 // indirect
	gomodules.xyz/jsonpatch v2.0.1+incompatible // indirect
	google.golang.org/api v0.15.0 // indirect
	google.golang.org/genproto v0.0.0-20200108215221-bd8f9a0ef82f // indirect
	google.golang.org/grpc v1.26.0 // indirect
	gopkg.in/yaml.v1 v1.0.0-20140924161607-9f9df34309c0 // indirect
	gopkg.in/yaml.v3 v3.0.0-20191120175047-4206685974f2 // indirect
	istio.io/api v0.0.0-20191115173247-e1a1952e5b81
	istio.io/client-go v0.0.0-20191120150049-26c62a04cdbc
	istio.io/gogo-genproto v0.0.0-20191029161641-f7d19ec0141d // indirect
	k8s.io/api v0.17.2
	k8s.io/apimachinery v0.17.2
	k8s.io/client-go v0.17.2
	k8s.io/gengo v0.0.0-20200114144118-36b2048a9120 // indirect
	k8s.io/klog v1.0.0
	k8s.io/kube-openapi v0.0.0-20200410145947-61e04a5be9a6
	knative.dev/pkg v0.0.0-20191217184203-cf220a867b3d
	knative.dev/serving v0.11.0
	sigs.k8s.io/controller-runtime v0.5.0
	sigs.k8s.io/controller-tools v0.2.5 // indirect
	sigs.k8s.io/testing_frameworks v0.1.2 // indirect
	sigs.k8s.io/yaml v1.2.0 // indirect
)

//replace gopkg.in/fsnotify.v1 v1.4.7 => github.com/fsnotify/fsnotify v1.4.7

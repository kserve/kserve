module github.com/kubeflow/kfserving

go 1.13

require (
	cloud.google.com/go v0.47.0 // indirect
	contrib.go.opencensus.io/exporter/stackdriver v0.12.9-0.20191108183826-59d068f8d8ff // indirect
	github.com/astaxie/beego v1.12.1
	github.com/aws/aws-sdk-go v1.28.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cloudevents/sdk-go v1.2.0
	github.com/emicklei/go-restful v2.11.0+incompatible // indirect
	github.com/getkin/kin-openapi v0.2.0
	github.com/go-logr/logr v0.1.0
	github.com/go-logr/zapr v0.1.1 // indirect
	github.com/go-openapi/spec v0.19.4
	github.com/gogo/protobuf v1.3.1
	github.com/golang/groupcache v0.0.0-20191002201903-404acd9df4cc // indirect
	github.com/golang/protobuf v1.4.1
	github.com/google/go-cmp v0.5.0
	github.com/google/go-containerregistry v0.0.0-20190910142231-b02d448a3705 // indirect
	github.com/google/uuid v1.1.1
	github.com/imdario/mergo v0.3.8 // indirect
	github.com/mattbaird/jsonpatch v0.0.0-20171005235357-81af80346b1a // indirect
	github.com/onsi/ginkgo v1.11.0 // indirect
	github.com/onsi/gomega v1.8.1
	github.com/pkg/errors v0.8.1
	github.com/prometheus/common v0.7.0 // indirect
	github.com/prometheus/procfs v0.0.5 // indirect
	github.com/satori/go.uuid v1.2.0
	github.com/shiena/ansicolor v0.0.0-20151119151921-a422bbe96644 // indirect
	github.com/spf13/cobra v0.0.5
	go.uber.org/multierr v1.2.0 // indirect
	go.uber.org/zap v1.11.0 // indirect
	golang.org/x/net v0.0.0-20200226121028-0de0cce0169b
	golang.org/x/sys v0.0.0-20191026070338-33540a1f6037 // indirect
	golang.org/x/time v0.0.0-20191023065245-6d3f0bb11be5 // indirect
	google.golang.org/grpc v1.27.0
	google.golang.org/protobuf v1.25.0
	istio.io/api v0.0.0-20191115173247-e1a1952e5b81
	istio.io/client-go v0.0.0-20191120150049-26c62a04cdbc
	istio.io/gogo-genproto v0.0.0-20191029161641-f7d19ec0141d // indirect
	k8s.io/api v0.17.2
	k8s.io/apiextensions-apiserver v0.17.2 // indirect
	k8s.io/apimachinery v0.17.2
	k8s.io/client-go v0.17.2
	k8s.io/klog v1.0.0
	k8s.io/kube-openapi v0.0.0-20200410145947-61e04a5be9a6
	knative.dev/pkg v0.0.0-20191217184203-cf220a867b3d
	knative.dev/serving v0.11.0
	sigs.k8s.io/controller-runtime v0.4.0
	sigs.k8s.io/yaml v1.2.0 // indirect
)

//replace gopkg.in/fsnotify.v1 v1.4.7 => github.com/fsnotify/fsnotify v1.4.7

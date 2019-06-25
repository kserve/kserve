VENDOR_DIR := $(GOPATH)/src/github.com/kubeflow/kfserving/vendor
TF_PROTO_SRC := $(VENDOR_DIR)/github.com/tensorflow/tensorflow
TF_PROTO_OUT := generated
TEMP_DIR:=$(shell mktemp -d /tmp/tf2openapi.XXX)

# Generate code
generate:
ifndef GOPATH
	$(error GOPATH not defined, please define GOPATH. Run "go help gopath" to learn more about GOPATH)
endif

ifeq (,$(findstring $(GOPATH)/bin, $(PATH)))
	$(error GOPATH bin not in path, please add it to PATH)
endif
	# compile TensorFlow protos
	protoc -I $(TF_PROTO_SRC) --go_out=$(TEMP_DIR) $(TF_PROTO_SRC)/tensorflow/core/lib/core/error_codes.proto
	protoc -I $(TF_PROTO_SRC) --go_out=$(TEMP_DIR) $(TF_PROTO_SRC)/tensorflow/core/framework/*.proto
	protoc -I $(TF_PROTO_SRC) --go_out=$(TEMP_DIR) $(TF_PROTO_SRC)/tensorflow/core/protobuf/saver.proto
	protoc -I $(TF_PROTO_SRC) --go_out=$(TEMP_DIR) $(TF_PROTO_SRC)/tensorflow/core/protobuf/meta_graph.proto
	protoc -I $(TF_PROTO_SRC) --go_out=$(TEMP_DIR) $(TF_PROTO_SRC)/tensorflow/core/protobuf/saved_model.proto

	# move compiled protos out of temp dir
	if [ ! -d "./$(TF_PROTO_OUT)" ]; then \
		mkdir $(TF_PROTO_OUT); \
	fi
	for dir in lib framework protobuf; do \
		echo "$$dir"; \
		if [ ! -d "./$(TF_PROTO_OUT)/$$dir" ]; then \
                	mkdir $(TF_PROTO_OUT)/$$dir; \
                fi; \
                mv  $(TEMP_DIR)/github.com/tensorflow/tensorflow/tensorflow/go/core/$$dir/* ./$(TF_PROTO_OUT)/$$dir; \
	done

	# update meta_graph.pb.go to import generated package
	perl -pi -e 's+github.com/tensorflow/tensorflow/tensorflow/go/core/framework+github.com/kubeflow/kfserving/tools/tf2openapi/$(TF_PROTO_OUT)/framework+g' \
	./generated/protobuf/meta_graph.pb.go

	# clean up
	rm -rf $(TEMP_DIR)

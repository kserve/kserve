// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.28.1
// 	protoc        v3.14.0
// source: tensorflow/core/framework/variable.proto

package framework

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

// Protocol buffer representing a Variable.
type VariableDef struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Name of the variable tensor.
	VariableName string `protobuf:"bytes,1,opt,name=variable_name,json=variableName,proto3" json:"variable_name,omitempty"`
	// Name of the tensor holding the variable's initial value.
	InitialValueName string `protobuf:"bytes,6,opt,name=initial_value_name,json=initialValueName,proto3" json:"initial_value_name,omitempty"`
	// Name of the initializer op.
	InitializerName string `protobuf:"bytes,2,opt,name=initializer_name,json=initializerName,proto3" json:"initializer_name,omitempty"`
	// Name of the snapshot tensor.
	SnapshotName string `protobuf:"bytes,3,opt,name=snapshot_name,json=snapshotName,proto3" json:"snapshot_name,omitempty"`
	// Support for saving variables as slices of a larger variable.
	SaveSliceInfoDef *SaveSliceInfoDef `protobuf:"bytes,4,opt,name=save_slice_info_def,json=saveSliceInfoDef,proto3" json:"save_slice_info_def,omitempty"`
	// Whether to represent this as a ResourceVariable.
	IsResource bool `protobuf:"varint,5,opt,name=is_resource,json=isResource,proto3" json:"is_resource,omitempty"`
	// Whether this variable should be trained.
	Trainable bool `protobuf:"varint,7,opt,name=trainable,proto3" json:"trainable,omitempty"`
}

func (x *VariableDef) Reset() {
	*x = VariableDef{}
	if protoimpl.UnsafeEnabled {
		mi := &file_tensorflow_core_framework_variable_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *VariableDef) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*VariableDef) ProtoMessage() {}

func (x *VariableDef) ProtoReflect() protoreflect.Message {
	mi := &file_tensorflow_core_framework_variable_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use VariableDef.ProtoReflect.Descriptor instead.
func (*VariableDef) Descriptor() ([]byte, []int) {
	return file_tensorflow_core_framework_variable_proto_rawDescGZIP(), []int{0}
}

func (x *VariableDef) GetVariableName() string {
	if x != nil {
		return x.VariableName
	}
	return ""
}

func (x *VariableDef) GetInitialValueName() string {
	if x != nil {
		return x.InitialValueName
	}
	return ""
}

func (x *VariableDef) GetInitializerName() string {
	if x != nil {
		return x.InitializerName
	}
	return ""
}

func (x *VariableDef) GetSnapshotName() string {
	if x != nil {
		return x.SnapshotName
	}
	return ""
}

func (x *VariableDef) GetSaveSliceInfoDef() *SaveSliceInfoDef {
	if x != nil {
		return x.SaveSliceInfoDef
	}
	return nil
}

func (x *VariableDef) GetIsResource() bool {
	if x != nil {
		return x.IsResource
	}
	return false
}

func (x *VariableDef) GetTrainable() bool {
	if x != nil {
		return x.Trainable
	}
	return false
}

type SaveSliceInfoDef struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Name of the full variable of which this is a slice.
	FullName string `protobuf:"bytes,1,opt,name=full_name,json=fullName,proto3" json:"full_name,omitempty"`
	// Shape of the full variable.
	FullShape []int64 `protobuf:"varint,2,rep,packed,name=full_shape,json=fullShape,proto3" json:"full_shape,omitempty"`
	// Offset of this variable into the full variable.
	VarOffset []int64 `protobuf:"varint,3,rep,packed,name=var_offset,json=varOffset,proto3" json:"var_offset,omitempty"`
	// Shape of this variable.
	VarShape []int64 `protobuf:"varint,4,rep,packed,name=var_shape,json=varShape,proto3" json:"var_shape,omitempty"`
}

func (x *SaveSliceInfoDef) Reset() {
	*x = SaveSliceInfoDef{}
	if protoimpl.UnsafeEnabled {
		mi := &file_tensorflow_core_framework_variable_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *SaveSliceInfoDef) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SaveSliceInfoDef) ProtoMessage() {}

func (x *SaveSliceInfoDef) ProtoReflect() protoreflect.Message {
	mi := &file_tensorflow_core_framework_variable_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SaveSliceInfoDef.ProtoReflect.Descriptor instead.
func (*SaveSliceInfoDef) Descriptor() ([]byte, []int) {
	return file_tensorflow_core_framework_variable_proto_rawDescGZIP(), []int{1}
}

func (x *SaveSliceInfoDef) GetFullName() string {
	if x != nil {
		return x.FullName
	}
	return ""
}

func (x *SaveSliceInfoDef) GetFullShape() []int64 {
	if x != nil {
		return x.FullShape
	}
	return nil
}

func (x *SaveSliceInfoDef) GetVarOffset() []int64 {
	if x != nil {
		return x.VarOffset
	}
	return nil
}

func (x *SaveSliceInfoDef) GetVarShape() []int64 {
	if x != nil {
		return x.VarShape
	}
	return nil
}

var File_tensorflow_core_framework_variable_proto protoreflect.FileDescriptor

var file_tensorflow_core_framework_variable_proto_rawDesc = []byte{
	0x0a, 0x28, 0x74, 0x65, 0x6e, 0x73, 0x6f, 0x72, 0x66, 0x6c, 0x6f, 0x77, 0x2f, 0x63, 0x6f, 0x72,
	0x65, 0x2f, 0x66, 0x72, 0x61, 0x6d, 0x65, 0x77, 0x6f, 0x72, 0x6b, 0x2f, 0x76, 0x61, 0x72, 0x69,
	0x61, 0x62, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x0a, 0x74, 0x65, 0x6e, 0x73,
	0x6f, 0x72, 0x66, 0x6c, 0x6f, 0x77, 0x22, 0xbc, 0x02, 0x0a, 0x0b, 0x56, 0x61, 0x72, 0x69, 0x61,
	0x62, 0x6c, 0x65, 0x44, 0x65, 0x66, 0x12, 0x23, 0x0a, 0x0d, 0x76, 0x61, 0x72, 0x69, 0x61, 0x62,
	0x6c, 0x65, 0x5f, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0c, 0x76,
	0x61, 0x72, 0x69, 0x61, 0x62, 0x6c, 0x65, 0x4e, 0x61, 0x6d, 0x65, 0x12, 0x2c, 0x0a, 0x12, 0x69,
	0x6e, 0x69, 0x74, 0x69, 0x61, 0x6c, 0x5f, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x5f, 0x6e, 0x61, 0x6d,
	0x65, 0x18, 0x06, 0x20, 0x01, 0x28, 0x09, 0x52, 0x10, 0x69, 0x6e, 0x69, 0x74, 0x69, 0x61, 0x6c,
	0x56, 0x61, 0x6c, 0x75, 0x65, 0x4e, 0x61, 0x6d, 0x65, 0x12, 0x29, 0x0a, 0x10, 0x69, 0x6e, 0x69,
	0x74, 0x69, 0x61, 0x6c, 0x69, 0x7a, 0x65, 0x72, 0x5f, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x02, 0x20,
	0x01, 0x28, 0x09, 0x52, 0x0f, 0x69, 0x6e, 0x69, 0x74, 0x69, 0x61, 0x6c, 0x69, 0x7a, 0x65, 0x72,
	0x4e, 0x61, 0x6d, 0x65, 0x12, 0x23, 0x0a, 0x0d, 0x73, 0x6e, 0x61, 0x70, 0x73, 0x68, 0x6f, 0x74,
	0x5f, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0c, 0x73, 0x6e, 0x61,
	0x70, 0x73, 0x68, 0x6f, 0x74, 0x4e, 0x61, 0x6d, 0x65, 0x12, 0x4b, 0x0a, 0x13, 0x73, 0x61, 0x76,
	0x65, 0x5f, 0x73, 0x6c, 0x69, 0x63, 0x65, 0x5f, 0x69, 0x6e, 0x66, 0x6f, 0x5f, 0x64, 0x65, 0x66,
	0x18, 0x04, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1c, 0x2e, 0x74, 0x65, 0x6e, 0x73, 0x6f, 0x72, 0x66,
	0x6c, 0x6f, 0x77, 0x2e, 0x53, 0x61, 0x76, 0x65, 0x53, 0x6c, 0x69, 0x63, 0x65, 0x49, 0x6e, 0x66,
	0x6f, 0x44, 0x65, 0x66, 0x52, 0x10, 0x73, 0x61, 0x76, 0x65, 0x53, 0x6c, 0x69, 0x63, 0x65, 0x49,
	0x6e, 0x66, 0x6f, 0x44, 0x65, 0x66, 0x12, 0x1f, 0x0a, 0x0b, 0x69, 0x73, 0x5f, 0x72, 0x65, 0x73,
	0x6f, 0x75, 0x72, 0x63, 0x65, 0x18, 0x05, 0x20, 0x01, 0x28, 0x08, 0x52, 0x0a, 0x69, 0x73, 0x52,
	0x65, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x12, 0x1c, 0x0a, 0x09, 0x74, 0x72, 0x61, 0x69, 0x6e,
	0x61, 0x62, 0x6c, 0x65, 0x18, 0x07, 0x20, 0x01, 0x28, 0x08, 0x52, 0x09, 0x74, 0x72, 0x61, 0x69,
	0x6e, 0x61, 0x62, 0x6c, 0x65, 0x22, 0x8a, 0x01, 0x0a, 0x10, 0x53, 0x61, 0x76, 0x65, 0x53, 0x6c,
	0x69, 0x63, 0x65, 0x49, 0x6e, 0x66, 0x6f, 0x44, 0x65, 0x66, 0x12, 0x1b, 0x0a, 0x09, 0x66, 0x75,
	0x6c, 0x6c, 0x5f, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x08, 0x66,
	0x75, 0x6c, 0x6c, 0x4e, 0x61, 0x6d, 0x65, 0x12, 0x1d, 0x0a, 0x0a, 0x66, 0x75, 0x6c, 0x6c, 0x5f,
	0x73, 0x68, 0x61, 0x70, 0x65, 0x18, 0x02, 0x20, 0x03, 0x28, 0x03, 0x52, 0x09, 0x66, 0x75, 0x6c,
	0x6c, 0x53, 0x68, 0x61, 0x70, 0x65, 0x12, 0x1d, 0x0a, 0x0a, 0x76, 0x61, 0x72, 0x5f, 0x6f, 0x66,
	0x66, 0x73, 0x65, 0x74, 0x18, 0x03, 0x20, 0x03, 0x28, 0x03, 0x52, 0x09, 0x76, 0x61, 0x72, 0x4f,
	0x66, 0x66, 0x73, 0x65, 0x74, 0x12, 0x1b, 0x0a, 0x09, 0x76, 0x61, 0x72, 0x5f, 0x73, 0x68, 0x61,
	0x70, 0x65, 0x18, 0x04, 0x20, 0x03, 0x28, 0x03, 0x52, 0x08, 0x76, 0x61, 0x72, 0x53, 0x68, 0x61,
	0x70, 0x65, 0x42, 0x6e, 0x0a, 0x18, 0x6f, 0x72, 0x67, 0x2e, 0x74, 0x65, 0x6e, 0x73, 0x6f, 0x72,
	0x66, 0x6c, 0x6f, 0x77, 0x2e, 0x66, 0x72, 0x61, 0x6d, 0x65, 0x77, 0x6f, 0x72, 0x6b, 0x42, 0x0e,
	0x56, 0x61, 0x72, 0x69, 0x61, 0x62, 0x6c, 0x65, 0x50, 0x72, 0x6f, 0x74, 0x6f, 0x73, 0x50, 0x01,
	0x5a, 0x3d, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x74, 0x65, 0x6e,
	0x73, 0x6f, 0x72, 0x66, 0x6c, 0x6f, 0x77, 0x2f, 0x74, 0x65, 0x6e, 0x73, 0x6f, 0x72, 0x66, 0x6c,
	0x6f, 0x77, 0x2f, 0x74, 0x65, 0x6e, 0x73, 0x6f, 0x72, 0x66, 0x6c, 0x6f, 0x77, 0x2f, 0x67, 0x6f,
	0x2f, 0x63, 0x6f, 0x72, 0x65, 0x2f, 0x66, 0x72, 0x61, 0x6d, 0x65, 0x77, 0x6f, 0x72, 0x6b, 0xf8,
	0x01, 0x01, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_tensorflow_core_framework_variable_proto_rawDescOnce sync.Once
	file_tensorflow_core_framework_variable_proto_rawDescData = file_tensorflow_core_framework_variable_proto_rawDesc
)

func file_tensorflow_core_framework_variable_proto_rawDescGZIP() []byte {
	file_tensorflow_core_framework_variable_proto_rawDescOnce.Do(func() {
		file_tensorflow_core_framework_variable_proto_rawDescData = protoimpl.X.CompressGZIP(file_tensorflow_core_framework_variable_proto_rawDescData)
	})
	return file_tensorflow_core_framework_variable_proto_rawDescData
}

var file_tensorflow_core_framework_variable_proto_msgTypes = make([]protoimpl.MessageInfo, 2)
var file_tensorflow_core_framework_variable_proto_goTypes = []interface{}{
	(*VariableDef)(nil),      // 0: tensorflow.VariableDef
	(*SaveSliceInfoDef)(nil), // 1: tensorflow.SaveSliceInfoDef
}
var file_tensorflow_core_framework_variable_proto_depIdxs = []int32{
	1, // 0: tensorflow.VariableDef.save_slice_info_def:type_name -> tensorflow.SaveSliceInfoDef
	1, // [1:1] is the sub-list for method output_type
	1, // [1:1] is the sub-list for method input_type
	1, // [1:1] is the sub-list for extension type_name
	1, // [1:1] is the sub-list for extension extendee
	0, // [0:1] is the sub-list for field type_name
}

func init() { file_tensorflow_core_framework_variable_proto_init() }
func file_tensorflow_core_framework_variable_proto_init() {
	if File_tensorflow_core_framework_variable_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_tensorflow_core_framework_variable_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*VariableDef); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_tensorflow_core_framework_variable_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*SaveSliceInfoDef); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_tensorflow_core_framework_variable_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   2,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_tensorflow_core_framework_variable_proto_goTypes,
		DependencyIndexes: file_tensorflow_core_framework_variable_proto_depIdxs,
		MessageInfos:      file_tensorflow_core_framework_variable_proto_msgTypes,
	}.Build()
	File_tensorflow_core_framework_variable_proto = out.File
	file_tensorflow_core_framework_variable_proto_rawDesc = nil
	file_tensorflow_core_framework_variable_proto_goTypes = nil
	file_tensorflow_core_framework_variable_proto_depIdxs = nil
}

// Code generated by protoc-gen-go. DO NOT EDIT.
// source: tensorflow/core/framework/remote_fused_graph_execute_info.proto

package framework

import (
	fmt "fmt"
	proto "github.com/golang/protobuf/proto"
	math "math"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion3 // please upgrade the proto package

// Protocol buffer representing a handle to a tensorflow resource. Handles are
// not valid across executions, but can be serialized back and forth from within
// a single run.
type RemoteFusedGraphExecuteInfo struct {
	// Definition of remote graph
	RemoteGraph *GraphDef `protobuf:"bytes,1,opt,name=remote_graph,json=remoteGraph,proto3" json:"remote_graph,omitempty"`
	// Remote fused graph input node name
	GraphInputNodeName []string `protobuf:"bytes,2,rep,name=graph_input_node_name,json=graphInputNodeName,proto3" json:"graph_input_node_name,omitempty"`
	// Remote fused graph output node name
	GraphOutputNodeName []string `protobuf:"bytes,3,rep,name=graph_output_node_name,json=graphOutputNodeName,proto3" json:"graph_output_node_name,omitempty"`
	// Executor's name
	ExecutorName string `protobuf:"bytes,4,opt,name=executor_name,json=executorName,proto3" json:"executor_name,omitempty"`
	// Optional: Parameters given to the inference-inferencelogger
	SerializedExecutorParameters []byte `protobuf:"bytes,5,opt,name=serialized_executor_parameters,json=serializedExecutorParameters,proto3" json:"serialized_executor_parameters,omitempty"`
	// Optional: Default graph input tensor shape used to allocate memory
	// before executing op
	DefaultGraphInputTensorShape []*RemoteFusedGraphExecuteInfo_TensorShapeTypeProto `protobuf:"bytes,6,rep,name=default_graph_input_tensor_shape,json=defaultGraphInputTensorShape,proto3" json:"default_graph_input_tensor_shape,omitempty"`
	// Optional: Default graph input tensor shape used to allocate memory
	// before executing op
	// TODO(satok): Remote output tensor shape once shape information is stored
	// in NodeDef
	DefaultGraphOutputTensorShape []*RemoteFusedGraphExecuteInfo_TensorShapeTypeProto `protobuf:"bytes,7,rep,name=default_graph_output_tensor_shape,json=defaultGraphOutputTensorShape,proto3" json:"default_graph_output_tensor_shape,omitempty"`
	XXX_NoUnkeyedLiteral          struct{}                                            `json:"-"`
	XXX_unrecognized              []byte                                              `json:"-"`
	XXX_sizecache                 int32                                               `json:"-"`
}

func (m *RemoteFusedGraphExecuteInfo) Reset()         { *m = RemoteFusedGraphExecuteInfo{} }
func (m *RemoteFusedGraphExecuteInfo) String() string { return proto.CompactTextString(m) }
func (*RemoteFusedGraphExecuteInfo) ProtoMessage()    {}
func (*RemoteFusedGraphExecuteInfo) Descriptor() ([]byte, []int) {
	return fileDescriptor_c15f13da5b37f691, []int{0}
}

func (m *RemoteFusedGraphExecuteInfo) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_RemoteFusedGraphExecuteInfo.Unmarshal(m, b)
}
func (m *RemoteFusedGraphExecuteInfo) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_RemoteFusedGraphExecuteInfo.Marshal(b, m, deterministic)
}
func (m *RemoteFusedGraphExecuteInfo) XXX_Merge(src proto.Message) {
	xxx_messageInfo_RemoteFusedGraphExecuteInfo.Merge(m, src)
}
func (m *RemoteFusedGraphExecuteInfo) XXX_Size() int {
	return xxx_messageInfo_RemoteFusedGraphExecuteInfo.Size(m)
}
func (m *RemoteFusedGraphExecuteInfo) XXX_DiscardUnknown() {
	xxx_messageInfo_RemoteFusedGraphExecuteInfo.DiscardUnknown(m)
}

var xxx_messageInfo_RemoteFusedGraphExecuteInfo proto.InternalMessageInfo

func (m *RemoteFusedGraphExecuteInfo) GetRemoteGraph() *GraphDef {
	if m != nil {
		return m.RemoteGraph
	}
	return nil
}

func (m *RemoteFusedGraphExecuteInfo) GetGraphInputNodeName() []string {
	if m != nil {
		return m.GraphInputNodeName
	}
	return nil
}

func (m *RemoteFusedGraphExecuteInfo) GetGraphOutputNodeName() []string {
	if m != nil {
		return m.GraphOutputNodeName
	}
	return nil
}

func (m *RemoteFusedGraphExecuteInfo) GetExecutorName() string {
	if m != nil {
		return m.ExecutorName
	}
	return ""
}

func (m *RemoteFusedGraphExecuteInfo) GetSerializedExecutorParameters() []byte {
	if m != nil {
		return m.SerializedExecutorParameters
	}
	return nil
}

func (m *RemoteFusedGraphExecuteInfo) GetDefaultGraphInputTensorShape() []*RemoteFusedGraphExecuteInfo_TensorShapeTypeProto {
	if m != nil {
		return m.DefaultGraphInputTensorShape
	}
	return nil
}

func (m *RemoteFusedGraphExecuteInfo) GetDefaultGraphOutputTensorShape() []*RemoteFusedGraphExecuteInfo_TensorShapeTypeProto {
	if m != nil {
		return m.DefaultGraphOutputTensorShape
	}
	return nil
}

type RemoteFusedGraphExecuteInfo_TensorShapeTypeProto struct {
	Dtype                DataType          `protobuf:"varint,1,opt,name=dtype,proto3,enum=tensorflow.DataType" json:"dtype,omitempty"`
	Shape                *TensorShapeProto `protobuf:"bytes,2,opt,name=shape,proto3" json:"shape,omitempty"`
	XXX_NoUnkeyedLiteral struct{}          `json:"-"`
	XXX_unrecognized     []byte            `json:"-"`
	XXX_sizecache        int32             `json:"-"`
}

func (m *RemoteFusedGraphExecuteInfo_TensorShapeTypeProto) Reset() {
	*m = RemoteFusedGraphExecuteInfo_TensorShapeTypeProto{}
}
func (m *RemoteFusedGraphExecuteInfo_TensorShapeTypeProto) String() string {
	return proto.CompactTextString(m)
}
func (*RemoteFusedGraphExecuteInfo_TensorShapeTypeProto) ProtoMessage() {}
func (*RemoteFusedGraphExecuteInfo_TensorShapeTypeProto) Descriptor() ([]byte, []int) {
	return fileDescriptor_c15f13da5b37f691, []int{0, 0}
}

func (m *RemoteFusedGraphExecuteInfo_TensorShapeTypeProto) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_RemoteFusedGraphExecuteInfo_TensorShapeTypeProto.Unmarshal(m, b)
}
func (m *RemoteFusedGraphExecuteInfo_TensorShapeTypeProto) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_RemoteFusedGraphExecuteInfo_TensorShapeTypeProto.Marshal(b, m, deterministic)
}
func (m *RemoteFusedGraphExecuteInfo_TensorShapeTypeProto) XXX_Merge(src proto.Message) {
	xxx_messageInfo_RemoteFusedGraphExecuteInfo_TensorShapeTypeProto.Merge(m, src)
}
func (m *RemoteFusedGraphExecuteInfo_TensorShapeTypeProto) XXX_Size() int {
	return xxx_messageInfo_RemoteFusedGraphExecuteInfo_TensorShapeTypeProto.Size(m)
}
func (m *RemoteFusedGraphExecuteInfo_TensorShapeTypeProto) XXX_DiscardUnknown() {
	xxx_messageInfo_RemoteFusedGraphExecuteInfo_TensorShapeTypeProto.DiscardUnknown(m)
}

var xxx_messageInfo_RemoteFusedGraphExecuteInfo_TensorShapeTypeProto proto.InternalMessageInfo

func (m *RemoteFusedGraphExecuteInfo_TensorShapeTypeProto) GetDtype() DataType {
	if m != nil {
		return m.Dtype
	}
	return DataType_DT_INVALID
}

func (m *RemoteFusedGraphExecuteInfo_TensorShapeTypeProto) GetShape() *TensorShapeProto {
	if m != nil {
		return m.Shape
	}
	return nil
}

func init() {
	proto.RegisterType((*RemoteFusedGraphExecuteInfo)(nil), "tensorflow.RemoteFusedGraphExecuteInfo")
	proto.RegisterType((*RemoteFusedGraphExecuteInfo_TensorShapeTypeProto)(nil), "tensorflow.RemoteFusedGraphExecuteInfo.TensorShapeTypeProto")
}

func init() {
	proto.RegisterFile("tensorflow/core/framework/remote_fused_graph_execute_info.proto", fileDescriptor_c15f13da5b37f691)
}

var fileDescriptor_c15f13da5b37f691 = []byte{
	// 457 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0xac, 0x93, 0xc1, 0x6f, 0xd3, 0x30,
	0x14, 0xc6, 0xe5, 0x95, 0x0e, 0xcd, 0x2d, 0x1c, 0xcc, 0x40, 0x51, 0x29, 0x28, 0x80, 0x90, 0x22,
	0x84, 0x12, 0xd1, 0x1d, 0xb8, 0x80, 0x90, 0xa6, 0x8e, 0x69, 0x97, 0x51, 0x85, 0x9d, 0xb8, 0x58,
	0x5e, 0xf3, 0x92, 0x46, 0x34, 0x79, 0x91, 0xe3, 0x30, 0xc6, 0x89, 0x03, 0xe2, 0xff, 0xe1, 0xbf,
	0xe3, 0x88, 0x6c, 0x87, 0xd4, 0x41, 0x6b, 0x4f, 0xdc, 0x92, 0xbc, 0xdf, 0xf7, 0xfc, 0xbd, 0x2f,
	0xcf, 0xf4, 0x9d, 0x82, 0xb2, 0x46, 0x99, 0xae, 0xf1, 0x2a, 0x5a, 0xa2, 0x84, 0x28, 0x95, 0xa2,
	0x80, 0x2b, 0x94, 0x9f, 0x23, 0x09, 0x05, 0x2a, 0xe0, 0x69, 0x53, 0x43, 0xc2, 0x33, 0x29, 0xaa,
	0x15, 0x87, 0xaf, 0xb0, 0x6c, 0x14, 0xf0, 0xbc, 0x4c, 0x31, 0xac, 0x24, 0x2a, 0x64, 0x74, 0xd3,
	0x60, 0xf2, 0x7c, 0x7b, 0x33, 0xa3, 0xb7, 0x92, 0xc9, 0xcb, 0xed, 0x98, 0xad, 0xf0, 0x7a, 0x25,
	0x2a, 0x68, 0xe9, 0x1d, 0x4d, 0xd5, 0x75, 0x05, 0xb5, 0xc5, 0x9e, 0xfe, 0x1a, 0xd2, 0x87, 0xb1,
	0x71, 0xfc, 0x5e, 0x1b, 0x3e, 0xd5, 0xe7, 0x9d, 0x58, 0xbb, 0x67, 0x65, 0x8a, 0xec, 0x35, 0x1d,
	0xb7, 0x03, 0x19, 0x2b, 0x1e, 0xf1, 0x49, 0x30, 0x9a, 0x1d, 0x86, 0x9b, 0xee, 0xa1, 0xd1, 0xcc,
	0x21, 0x8d, 0x47, 0x96, 0x34, 0xef, 0xec, 0x15, 0xbd, 0x6f, 0x87, 0xcf, 0xcb, 0xaa, 0x51, 0xbc,
	0xc4, 0x04, 0x78, 0x29, 0x0a, 0xf0, 0xf6, 0xfc, 0x41, 0x70, 0x10, 0x33, 0x53, 0x3c, 0xd3, 0xb5,
	0x73, 0x4c, 0xe0, 0x5c, 0x14, 0xc0, 0x8e, 0xe8, 0x03, 0x2b, 0xc1, 0x46, 0xf5, 0x35, 0x03, 0xa3,
	0xb9, 0x67, 0xaa, 0x1f, 0x4c, 0xb1, 0x13, 0x3d, 0xa3, 0x77, 0x6c, 0xbc, 0x28, 0x2d, 0x7b, 0xcb,
	0x27, 0xc1, 0x41, 0x3c, 0xfe, 0xfb, 0xd1, 0x40, 0x73, 0xfa, 0xb8, 0x06, 0x99, 0x8b, 0x75, 0xfe,
	0x0d, 0x12, 0xde, 0xf1, 0x95, 0xd0, 0x99, 0x28, 0x90, 0xb5, 0x37, 0xf4, 0x49, 0x30, 0x8e, 0xa7,
	0x1b, 0xea, 0xa4, 0x85, 0x16, 0x1d, 0xc3, 0x7e, 0x10, 0xea, 0x27, 0x90, 0x8a, 0x66, 0xad, 0xb8,
	0x3b, 0x9b, 0x9b, 0xbe, 0xb7, 0xef, 0x0f, 0x82, 0xd1, 0xec, 0x8d, 0x1b, 0xd0, 0x8e, 0x7c, 0xc3,
	0x0b, 0x83, 0x7d, 0xd4, 0xd2, 0x8b, 0xeb, 0x0a, 0x16, 0xfa, 0xa7, 0xc4, 0xd3, 0xf6, 0x94, 0xd3,
	0x2e, 0x23, 0x07, 0x63, 0x3f, 0x09, 0x7d, 0xd2, 0xb7, 0xd1, 0xe6, 0xd5, 0xf3, 0x71, 0xfb, 0x3f,
	0xf8, 0x78, 0xe4, 0xfa, 0xb0, 0xb9, 0x3b, 0xdc, 0xe4, 0x0b, 0x3d, 0xbc, 0x49, 0xc6, 0x5e, 0xd0,
	0x61, 0xa2, 0x77, 0xcc, 0x2c, 0xcb, 0xdd, 0xfe, 0xb2, 0xcc, 0x85, 0x12, 0x9a, 0x8c, 0x2d, 0xc2,
	0x66, 0x74, 0x68, 0xfd, 0xee, 0x99, 0xc5, 0x9a, 0xba, 0xac, 0xd3, 0xdc, 0xfa, 0xb1, 0xe8, 0xf1,
	0x77, 0x42, 0x3d, 0x94, 0x99, 0x8b, 0x76, 0xcb, 0x7d, 0xec, 0xef, 0x98, 0xd2, 0x74, 0x59, 0x90,
	0x4f, 0x6f, 0xb3, 0x5c, 0xad, 0x9a, 0xcb, 0x70, 0x89, 0x45, 0xe4, 0x5c, 0x93, 0x9b, 0x1f, 0x33,
	0xfc, 0xe7, 0xfe, 0xfc, 0x26, 0xe4, 0x72, 0xdf, 0xdc, 0x9e, 0xa3, 0x3f, 0x01, 0x00, 0x00, 0xff,
	0xff, 0x41, 0x7e, 0x4c, 0x93, 0x08, 0x04, 0x00, 0x00,
}

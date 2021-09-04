// +build grpchelloworld
package main

import (
	"log"
	"net"

	pb "github.com/kserve/kserve/docs/samples/v1alpha2/custom/grpc-server/proto"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

const (
	PORT = ":8080"
)

type server struct{}

func (s *server) SayHello(ctx context.Context, in *pb.HelloRequest) (*pb.HelloReply, error) {
	log.Println("request: ", in.Name)
	return &pb.HelloReply{Message: "Hello " + in.Name}, nil
}

func (s *server) SendSomething(ctx context.Context, in *pb.HelloRequest) (*pb.HelloReply, error) {
	log.Println("request: ", in.Name)
	return &pb.HelloReply{Message: "Hello " + in.Name}, nil
}
func main() {
	lis, err := net.Listen("tcp", PORT)

	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	s := grpc.NewServer()
	pb.RegisterKServeGRPCServer(s, &server{})
	log.Println("server startup...")
	s.Serve(lis)
}

// +build grpchelloworld

package main

import (
	"crypto/tls"
	"flag"
	"log"
	"time"

	pb "github.com/kserve/kserve/docs/samples/v1alpha2/custom/grpc-server/proto"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

var (
	serverAddr         = flag.String("server_addr", "127.0.0.1:8080", "The server address in the format of host:port")
	serverHostOverride = flag.String("server_host_override", "", "")
	insecure           = flag.Bool("insecure", false, "Set to true to skip SSL validation")
	skipVerify         = flag.Bool("skip_verify", false, "Set to true to skip server hostname verification in SSL validation")
)

func main() {
	flag.Parse()

	var opts []grpc.DialOption
	if *serverHostOverride != "" {
		opts = append(opts, grpc.WithAuthority(*serverHostOverride))
	}
	if *insecure {
		opts = append(opts, grpc.WithInsecure())
	} else {
		cred := credentials.NewTLS(&tls.Config{
			InsecureSkipVerify: *skipVerify,
		})
		opts = append(opts, grpc.WithTransportCredentials(cred))
	}
	conn, err := grpc.Dial(*serverAddr, opts...)
	if err != nil {
		log.Fatalf("fail to dial: %v", err)
	}
	defer conn.Close()

	client := pb.NewKServeGRPCClient(conn)

	SayHello(client, "world")
	SendSomething(client, "KServe")
}

func SayHello(client pb.KServeGRPCClient, msg string) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	rep, err := client.SayHello(ctx, &pb.HelloRequest{Name: msg})
	if err != nil {
		log.Fatalf("%v.SyaHello failed %v: ", client, err)
	}
	log.Printf("SyaHello got %v\n", rep.GetMessage())
}

func SendSomething(client pb.KServeGRPCClient, msg string) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	rep, err := client.SendSomething(ctx, &pb.HelloRequest{Name: msg})
	if err != nil {
		log.Fatalf("%v.SendSomething failed %v: ", client, err)
	}
	log.Printf("SendSomething got %v\n", rep.Message)
}

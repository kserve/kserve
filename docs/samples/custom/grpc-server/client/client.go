// +build grpcping

package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"log"
	"time"

	pb "github.com/knative/docs/docs/serving/samples/grpc-ping-go/proto"
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

	client := pb.NewPingServiceClient(conn)

	ping(client, "hello")
	pingStream(client, "hello")
}

func ping(client pb.PingServiceClient, msg string) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	rep, err := client.Ping(ctx, &pb.Request{Msg: msg})
	if err != nil {
		log.Fatalf("%v.Ping failed %v: ", client, err)
	}
	log.Printf("Ping got %v\n", rep.GetMsg())
}

func pingStream(client pb.PingServiceClient, msg string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stream, err := client.PingStream(ctx)
	if err != nil {
		log.Fatalf("%v.(_) = _, %v", client, err)
	}

	waitc := make(chan struct{})
	go func() {
		for {
			in, err := stream.Recv()
			if err == io.EOF {
				// read done.
				close(waitc)
				return
			}
			if err != nil {
				log.Fatalf("Failed to receive a response : %v", err)
			}
			log.Printf("Got %s", in.GetMsg())
		}
	}()

	i := 0
	for i < 20 {
		if err := stream.Send(&pb.Request{Msg: fmt.Sprintf("%s-%d", msg, i)}); err != nil {
			log.Fatalf("Failed to send a ping: %v", err)
		}
		i++
	}
	stream.CloseSend()
	<-waitc

}

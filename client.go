package main

import (
	"context"
	"fmt"
	"log"
	"time"

	pb "github.com/prkbuilds/raft-kv/proto/raftpb"
	"google.golang.org/grpc"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, "localhost:50051", grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	client := pb.NewRaftClient(conn)

	for {
		log.Println("Calling StartCommand RPC...")
		resp, err := client.StartCommand(context.Background(), &pb.StartCommandRequest{Command: "PUT x 42"})
		if err != nil {
			log.Printf("Error calling StartCommand: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}

		log.Println("StartCommand RPC returned")

		if resp.IsLeader {
			fmt.Printf("Command accepted. Index: %d, Term: %d\n", resp.Index, resp.Term)
		} else {
			fmt.Println("This node is not the leader.")
		}

		break
	}
}

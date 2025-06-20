package main

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	pb "q4/proto"

	"google.golang.org/grpc"
)

var q4Nodes = []string{
	"q4_node1:50052",
	"q4_node2:50052",
	"q4_node3:50052",
	"q4_node4:50052",
	"q4_node5:50052",
}

var operations = []string{
	"SET x = 10",
	"SET y = 20",
	"INCR x",
	"GET x",
	"INCR y",
	"GET y",
}

func main() {
	fmt.Println("Client is waiting for Q3 leader election and Q4 init...")
	time.Sleep(30 * time.Second)

	for {
		op := operations[rand.Intn(len(operations))]
		fmt.Printf("\n[CLIENT] Sending operation: %s\n", op)

		success := false

		for !success {
			for _, addr := range q4Nodes {
				fmt.Printf("[CLIENT] Trying node %s...\n", addr)

				conn, err := grpc.Dial(addr, grpc.WithInsecure())
				if err != nil {
					fmt.Printf("[CLIENT] Failed to connect to %s: %v\n", addr, err)
					continue
				}
				client := pb.NewRaftServiceClient(conn)

				ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
				resp, err := client.ClientOperation(ctx, &pb.ClientRequest{Operation: op})
				cancel()
				conn.Close()

				if err != nil {
					fmt.Printf("[CLIENT] Node %s RPC error: %v\n", addr, err)
					continue
				}

				fmt.Printf("[CLIENT] Response from node %s: %s\n", addr, resp.Msg)

				if resp.Msg == "No leader available" || resp.Msg == "Leader unreachable" || resp.Msg == "Commit timeout" {
					fmt.Println("[CLIENT] Will retry after short delay...")
					time.Sleep(3 * time.Second)
					break
				}

				success = true
				break
			}
		}

		time.Sleep(5 * time.Second)
	}
}

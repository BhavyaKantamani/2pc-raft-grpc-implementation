package main

import (
	"context"
	"log"
	"net"
	"os"
	"time"

	pb "twopcapp/proto"
	"google.golang.org/grpc"
)

const Divider = "------------------------------------------------"

type ParticipantServer struct {
	pb.UnimplementedDecisionServiceServer
	pb.UnimplementedGlobalDecisionServiceServer
	nodeID   string
	lastVote bool
}

func (s *ParticipantServer) ReportVote(ctx context.Context, req *pb.VoteReport) (*pb.Ack, error) {
	log.Printf("[%s] Phase Decision of Node %s received ReportVote from Node %s: Commit=%v",
		time.Now().Format(time.RFC3339Nano), s.nodeID, req.NodeId, req.Commit)
	s.lastVote = req.Commit
	return &pb.Ack{Message: "Vote recorded"}, nil
}

func (s *ParticipantServer) GlobalDecision(ctx context.Context, req *pb.DecisionMessage) (*pb.Ack, error) {
	outcome := "ABORTED"
	if req.GlobalCommit {
		outcome = "COMMITTED"
	}

	log.Printf("\n%s\n[%s] Phase Decision of Node %s received GlobalDecision for Tx %s => %s\n%s",
		Divider,
		time.Now().Format(time.RFC3339Nano), s.nodeID, req.TransactionId, outcome,
		Divider,
	)

	// Log the local action taken based on the global decision
	if req.GlobalCommit {
		log.Printf("[Participant %s] Locally COMMIT the transaction %s", s.nodeID, req.TransactionId)
	} else {
		log.Printf("[Participant %s] Locally ABORT the transaction %s", s.nodeID, req.TransactionId)
	}

	return &pb.Ack{Message: "Decision applied"}, nil
}

func main() {
	log.SetFlags(0)
	log.SetOutput(os.Stdout)

	nodeID := os.Getenv("NODE_ID")
	if nodeID == "" {
		log.Fatal("NODE_ID environment variable not set")
	}

	port := ":60051"
	lis, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	server := &ParticipantServer{nodeID: nodeID}
	pb.RegisterDecisionServiceServer(grpcServer, server)
	pb.RegisterGlobalDecisionServiceServer(grpcServer, server)

	log.Printf("[%s] Phase Decision of Node %s listening on %s...",
		time.Now().Format(time.RFC3339Nano), nodeID, port)

	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}

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
type ParticipantServer struct {
	pb.UnimplementedDecisionServiceServer
	pb.UnimplementedGlobalDecisionServiceServer
	nodeID   string
	lastVote bool
}
func (s *ParticipantServer) ReportVote(ctx context.Context, req *pb.VoteReport) (*pb.Ack, error) {
	log.Printf("Phase Voting of Node %s sends RPC ReportVote to Phase Decision of Node %s", req.NodeId, s.nodeID)
	log.Printf("[PHASE VOTE] Node %s received ReportVote from %s: Commit=%v", s.nodeID, req.NodeId, req.Commit)
	s.lastVote = req.Commit
	return &pb.Ack{Message: "Vote recorded"}, nil
}
func (s *ParticipantServer) GlobalDecision(ctx context.Context, req *pb.DecisionMessage) (*pb.Ack, error) {
	outcome := "ABORTED"
	if req.GlobalCommit {
		outcome = "COMMITTED"
	}
	log.Printf("Phase Decision of Node coordinator sends RPC GlobalDecision to Phase Decision of Node %s", s.nodeID)
	log.Printf("[PHASE DECISION] Node %s received GlobalDecision for Tx %s => %s", s.nodeID, req.TransactionId, outcome)
	return &pb.Ack{Message: "Decision applied"}, nil
}
func (s *ParticipantServer) StartDecisionPhase(ctx context.Context, req *pb.DecisionRequest) (*pb.Ack, error) {
	log.Printf("Phase Voting of Node coordinator sends RPC StartDecisionPhase to Phase Decision of Node %s", s.nodeID)
	globalCommit := true
	for _, vote := range req.Votes {
		if !vote.Commit {
			globalCommit = false
			break
		}
	}
	log.Printf("[COORDINATOR] Global decision for %s: %v", req.TransactionId, globalCommit)
	for _, vote := range req.Votes {
		host, _, err := net.SplitHostPort(vote.NodeId)
		if err != nil {
			log.Printf("[COORDINATOR ERROR] Invalid node ID format: %s", vote.NodeId)
			continue
		}
		address := host + ":60051"
		conn, err := grpc.Dial(address, grpc.WithInsecure())
		if err != nil {
			log.Printf("[COORDINATOR ERROR] Could not reach %s: %v", address, err)
			continue
		}
		client := pb.NewGlobalDecisionServiceClient(conn)
		_, err = client.GlobalDecision(context.Background(), &pb.DecisionMessage{
			TransactionId: req.TransactionId,
			GlobalCommit:  globalCommit,
		})
		if err != nil {
			log.Printf("[COORDINATOR ERROR] Sending GlobalDecision to %s failed: %v", address, err)
		} else {
			log.Printf("[COORDINATOR] Decision sent to %s: %s", address, map[bool]string{true: "COMMIT", false: "ABORT"}[globalCommit])
		}
		conn.Close()
	}
	return &pb.Ack{Message: "Global decision made"}, nil
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
	log.Printf("[%s] Phase Decision of Node %s listening on %s...", time.Now().Format(time.RFC3339Nano), nodeID, port)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}

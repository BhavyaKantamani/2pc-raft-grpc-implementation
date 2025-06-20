package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	pb "q4/proto"

	"google.golang.org/grpc"
)

type Server struct {
	pb.UnimplementedRaftServiceServer
	nodeID        int32
	leaderID      int32
	leaderAddress string
	isLeader      bool
	currentTerm   int32
	peers         []string

	mu          sync.Mutex
	logs        []string
	commitIndex int
	nextIndex   int
	pendingOps  map[string]*PendingOp
}

type PendingOp struct {
	operation string
	idx       int
	tm        int32
	acks      int
	done      chan bool
}

func (s *Server) NotifyLeader(ctx context.Context, req *pb.NotifyLeaderRequest) (*pb.Ack, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.leaderID = req.LeaderId
	s.leaderAddress = req.LeaderAddress
	s.isLeader = s.nodeID == req.LeaderId
	s.currentTerm++ // just increment for simulation

	fmt.Printf("RECV: Node %d received NotifyLeader from Node %d (Leader is %d at %s) [New Term: %d]\n",
		s.nodeID, req.LeaderId, s.leaderID, s.leaderAddress, s.currentTerm)

	return &pb.Ack{Msg: "OK"}, nil
}

func (s *Server) ClientOperation(ctx context.Context, req *pb.ClientRequest) (*pb.Ack, error) {
	s.mu.Lock()
	isLeader := s.isLeader
	leaderAddr := s.leaderAddress
	leaderID := s.leaderID
	s.mu.Unlock()

	fmt.Printf("RECV: Node %d received ClientOperation from client\n", s.nodeID)

	if !isLeader {
		if leaderAddr == "" || leaderID == 0 {
			fmt.Printf("INFO: Node %d has no leader info, cannot forward\n", s.nodeID)
			return &pb.Ack{Msg: "No leader available"}, nil
		}

		fmt.Printf("SEND: Node %d forwards ClientOperation to Leader Node %d at %s\n", s.nodeID, leaderID, leaderAddr)
		conn, err := grpc.Dial(leaderAddr, grpc.WithInsecure())
		if err != nil {
			return &pb.Ack{Msg: "Leader unreachable"}, nil
		}
		defer conn.Close()

		client := pb.NewRaftServiceClient(conn)
		return client.ClientOperation(context.Background(), req)
	}

	s.mu.Lock()
	s.nextIndex++
	idx := s.nextIndex
	term := s.currentTerm
	entry := fmt.Sprintf("%s (term: %d, idx: %d)", req.Operation, term, idx)
	s.logs = append(s.logs, entry)

	pending := &PendingOp{
		operation: req.Operation,
		idx:       idx,
		tm:        term,
		acks:      1,
		done:      make(chan bool, 1),
	}
	if s.pendingOps == nil {
		s.pendingOps = make(map[string]*PendingOp)
	}
	s.pendingOps[entry] = pending
	s.mu.Unlock()

	fmt.Printf("INFO: Leader Node %d accepted ClientOperation: %s\n", s.nodeID, req.Operation)
	go s.broadcastAppendEntries(entry)

	select {
	case <-pending.done:
		return &pb.Ack{Msg: "Operation committed"}, nil
	case <-time.After(5 * time.Second):
		return &pb.Ack{Msg: "Commit timeout"}, nil
	}
}

func (s *Server) broadcastAppendEntries(entry string) {
	s.mu.Lock()
	logCopy := append([]string{}, s.logs...)
	commitIdx := int32(s.commitIndex)
	term := s.currentTerm
	s.mu.Unlock()

	var wg sync.WaitGroup
	for _, p := range s.peers {
		if p == fmt.Sprintf("q4_node%d:50052", s.nodeID) {
			continue
		}
		wg.Add(1)
		go func(addr string) {
			defer wg.Done()
			conn, err := grpc.Dial(addr, grpc.WithInsecure())
			if err != nil {
				return
			}
			defer conn.Close()

			client := pb.NewRaftServiceClient(conn)
			_, err = client.AppendEntries(context.Background(), &pb.AppendEntriesRequest{
				Term:        term,
				LeaderId:    s.nodeID,
				Entries:     logCopy,
				CommitIndex: commitIdx,
			})
			if err == nil {
				s.mu.Lock()
				if op, exists := s.pendingOps[entry]; exists {
					op.acks++
					if op.acks >= 3 && s.commitIndex < op.idx {
						fmt.Printf("COMMIT: Node %d committed operation: %s\n", s.nodeID, op.operation)
						s.commitIndex = op.idx
						op.done <- true
						delete(s.pendingOps, entry)
					}
				}
				s.mu.Unlock()
			}
		}(p)
	}
	wg.Wait()
}

func (s *Server) AppendEntries(ctx context.Context, req *pb.AppendEntriesRequest) (*pb.AppendEntriesResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.logs = req.Entries
	for i := s.commitIndex; i < int(req.CommitIndex); i++ {
		if i < len(s.logs) {
			fmt.Printf("COMMIT: Node %d committed operation: %s\n", s.nodeID, s.logs[i])
			s.commitIndex++
		}
	}
	return &pb.AppendEntriesResponse{Success: true}, nil
}

func main() {
	time.Sleep(5 * time.Second)

	id := os.Getenv("NODE_ID")
	nodeID, _ := strconv.Atoi(id)

	var peers []string
	for i := 1; i <= 5; i++ {
		peers = append(peers, fmt.Sprintf("q4_node%d:50052", i))
	}

	server := &Server{
		nodeID:      int32(nodeID),
		currentTerm: 0,
		peers:       peers,
		pendingOps:  make(map[string]*PendingOp),
	}

	go func() {
		for {
			time.Sleep(2 * time.Second)
			server.mu.Lock()
			if server.isLeader {
				server.mu.Unlock()
				server.broadcastAppendEntries("")
			} else {
				server.mu.Unlock()
			}
		}
	}()

	lis, err := net.Listen("tcp", ":50052")
	if err != nil {
		log.Fatalf("ERROR: Failed to listen: %v", err)
	}
	log.Printf("INFO: Q4 Node %d started at :50052", nodeID)
	grpcServer := grpc.NewServer()
	pb.RegisterRaftServiceServer(grpcServer, server)
	grpcServer.Serve(lis)
}

import grpc
import threading
import time
import sys
import random
import raft_pb2
import raft_pb2_grpc
from concurrent import futures

NODE_ID = int(sys.argv[1])
PEERS = [i for i in range(1, 6) if i != NODE_ID]

STATE = 'follower'
current_term = 0
voted_for = None
votes_received = 0
election_timer = None
lock = threading.Lock()
printed_timeout_once = False  # ✅ ensures timeout is printed only once

def reset_election_timer():
    global election_timer, printed_timeout_once
    if election_timer:
        election_timer.cancel()
    timeout = random.uniform(1.5, 3.0)
    if not printed_timeout_once:
        print(f"[INFO] Node {NODE_ID} sets election timeout to {timeout:.2f}s")
        printed_timeout_once = True
    election_timer = threading.Timer(timeout, start_election)
    election_timer.start()

def start_election():
    global STATE, current_term, voted_for, votes_received
    with lock:
        if STATE == 'leader':
            return
        STATE = 'candidate'
        current_term += 1
        voted_for = NODE_ID
        votes_received = 1
        print(f"[ELECTION] Node {NODE_ID} starts election for term {current_term}, voted for self")

    for peer in PEERS:
        addr = f"q3_node{peer}:50051"
        try:
            channel = grpc.insecure_channel(addr)
            stub = raft_pb2_grpc.RaftServiceStub(channel)
            print(f"[SEND] Node {NODE_ID} sends RPC RequestVote to Node {peer}")
            response = stub.RequestVote(raft_pb2.RequestVoteRequest(term=current_term, candidate_id=NODE_ID))
            if response.vote_granted:
                with lock:
                    votes_received += 1
                    if STATE == 'candidate' and votes_received > len(PEERS) // 2:
                        become_leader()
        except Exception as e:
            print(f"[ERROR] Node {NODE_ID} failed to contact Node {peer}: {e}")

def become_leader():
    global STATE
    STATE = 'leader'
    print(f"[LEADER] Node {NODE_ID} becomes leader for term {current_term}")
    notify_q4_leader()
    start_heartbeat()

def notify_q4_leader():
    for i in range(1, 6):
        try:
            addr = f"q4_node{i}:50052"
            print(f"[SEND] Node {NODE_ID} sends RPC NotifyLeader to Q4 Node {i} at {addr}")
            channel = grpc.insecure_channel(addr)
            stub = raft_pb2_grpc.RaftServiceStub(channel)
            stub.NotifyLeader(raft_pb2.NotifyLeaderRequest(
                leader_id=NODE_ID,
                leader_address=f"q4_node{NODE_ID}:50052"
            ))
        except Exception as e:
            print(f"[ERROR] NotifyLeader to q4_node{i} failed: {e}")

def start_heartbeat():
    def send_heartbeats():
        while STATE == 'leader':
            for peer in PEERS:
                try:
                    addr = f"q3_node{peer}:50051"
                    channel = grpc.insecure_channel(addr)
                    stub = raft_pb2_grpc.RaftServiceStub(channel)
                    stub.AppendEntries(raft_pb2.AppendEntriesRequest(
                        term=current_term, leader_id=NODE_ID
                    ))
                    print(f"[SEND] Node {NODE_ID} sends RPC AppendEntries to Node {peer}")
                except Exception as e:
                    print(f"[ERROR] Failed to send AppendEntries to Node {peer}: {e}")
            time.sleep(1)

    threading.Thread(target=send_heartbeats, daemon=True).start()

class RaftServicer(raft_pb2_grpc.RaftServiceServicer):
    def RequestVote(self, request, context):
        global voted_for, current_term, STATE
        print(f"[RECV] Node {NODE_ID} runs RPC RequestVote from Node {request.candidate_id}")
        with lock:
            if request.term > current_term:
                current_term = request.term
                STATE = 'follower'
                voted_for = None
            if request.term >= current_term and (voted_for is None or voted_for == request.candidate_id):
                current_term = request.term
                voted_for = request.candidate_id
                reset_election_timer()
                return raft_pb2.RequestVoteResponse(vote_granted=True, term=current_term)
        return raft_pb2.RequestVoteResponse(vote_granted=False, term=current_term)

    def AppendEntries(self, request, context):
        global current_term, STATE, voted_for
        print(f"[RECV] Node {NODE_ID} runs RPC AppendEntries from Leader {request.leader_id}")
        with lock:
            if request.term >= current_term:
                current_term = request.term
                STATE = 'follower'
                voted_for = None
            reset_election_timer()
        return raft_pb2.AppendEntriesResponse(success=True, term=current_term)

    def NotifyLeader(self, request, context):
        return raft_pb2.Ack(msg="Q3 doesn't process this")

def serve():
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))
    raft_pb2_grpc.add_RaftServiceServicer_to_server(RaftServicer(), server)
    server.add_insecure_port('[::]:50051')
    server.start()
    print(f"[INFO] Node {NODE_ID} gRPC server started at port 50051")
    reset_election_timer()
    server.wait_for_termination()

if __name__ == "__main__":
    serve()


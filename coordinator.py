import grpc
import time
import uuid
import sys
import threading
from concurrent import futures
import twopc_pb2
import twopc_pb2_grpc

import builtins
print = lambda *args, **kwargs: builtins.print(*args, flush=True, **kwargs)
SEPARATOR = "\n" + "-" * 60 + "\n"
print("[COORDINATOR] Starting process...")

PARTICIPANTS = ["participant1:50051", "participant2:50051", "participant3:50051", "participant4:50051"]
DECISION_COORDINATOR_ADDR = "localhost:60051"

class Coordinator(twopc_pb2_grpc.CoordinatorServiceServicer):
    def RequestVote(self, request, context):
        transaction_id = request.transaction_id
        print(f"[COORDINATOR] Requesting votes for Transaction {transaction_id}")
        votes = []
        for node in PARTICIPANTS:
            try:
                with grpc.insecure_channel(node) as channel:
                    stub = twopc_pb2_grpc.VotingServiceStub(channel)
                    response = stub.Vote(twopc_pb2.VoteRequest(transaction_id=transaction_id, node_id=node))
                    votes.append(response.commit)
                    print(f"[COORDINATOR] Received vote from {node}: {'Commit' if response.commit else 'Abort'}")
            except Exception as e:
                print(f"[COORDINATOR WARNING] Failed to connect to {node}: {e}")
        return twopc_pb2.VoteResponse(transaction_id=transaction_id, node_id="Coordinator", commit=all(votes))

def serve():
    print("[COORDINATOR] Initializing gRPC server...")
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))
    twopc_pb2_grpc.add_CoordinatorServiceServicer_to_server(Coordinator(), server)
    server.add_insecure_port("[::]:50050")
    server.start()
    print("[COORDINATOR] gRPC server is running on port 50050.")
    try:
        while True:
            time.sleep(1)
    except KeyboardInterrupt:
        print("[COORDINATOR] Shutting down service...")
        server.stop(0)
    except Exception as e:
        print(f"[COORDINATOR WARNING] {e}", file=sys.stderr)

if __name__ == "__main__":
    print("[COORDINATOR] Executing serve() function...")
    server_thread = threading.Thread(target=serve, daemon=True)
    server_thread.start()
    while True:
        time.sleep(5)
        transaction_id = str(uuid.uuid4())
        print(SEPARATOR)
        print(f"[COORDINATOR] Sending test VoteRequest for Transaction {transaction_id}...")
        vote_reports = []
        for node in PARTICIPANTS:
            try:
                with grpc.insecure_channel(node) as channel:
                    stub = twopc_pb2_grpc.VotingServiceStub(channel)
                    response = stub.Vote(twopc_pb2.VoteRequest(transaction_id=transaction_id, node_id=node))
                    vote_reports.append(twopc_pb2.VoteReport(
                        transaction_id=transaction_id,
                        node_id=node,
                        commit=response.commit
                    ))
                    print(f"[COORDINATOR] Received vote from {node}: {'Commit' if response.commit else 'Abort'}")
            except Exception as e:
                print(f"[COORDINATOR WARNING] Could not reach {node}: {e}")
        time.sleep(1.0)
        print("[COORDINATOR] Voting phase completed.")
        decision_request = twopc_pb2.DecisionRequest(
            transaction_id=transaction_id,
            votes=vote_reports
        )
        try:
            with grpc.insecure_channel(DECISION_COORDINATOR_ADDR) as channel:
                stub = twopc_pb2_grpc.DecisionServiceStub(channel)
                print("Phase Voting of Node coordinator sends RPC StartDecisionPhase to Phase Decision of Node coordinator")
                ack = stub.StartDecisionPhase(decision_request)
                time.sleep(0.5)
                global_commit = all(v.commit for v in vote_reports)
                decision_text = "COMMITTED" if global_commit else "ABORTED"
                print(f"[COORDINATOR] Global decision for {transaction_id}: {decision_text}")
                print(f"[COORDINATOR] Got ack from Go coordinator: {ack.message}")
        except Exception as e:
            print(f"[COORDINATOR WARNING] Failed to reach Go decision coordinator: {e}")
        time.sleep(1.5)

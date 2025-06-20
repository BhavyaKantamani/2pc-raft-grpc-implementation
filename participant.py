import grpc
import time
import random
import sys
import os
from concurrent import futures

import twopc_pb2
import twopc_pb2_grpc

SEPARATOR = "-" * 60

import builtins
print = lambda *args, **kwargs: builtins.print(*args, flush=True, **kwargs)

class VotingParticipant(twopc_pb2_grpc.VotingServiceServicer):
    def Vote(self, request, context):
        node_id = os.getenv("NODE_ID", "unknown")
        print(f"Phase Voting of Node {node_id} receives RPC Vote from Phase Voting of Node coordinator")

        commit_decision = random.choice([True, False])
        decision_str = "Commit" if commit_decision else "Abort"

        print(f"[{node_id}] Voting {decision_str} for Transaction {request.transaction_id}")
        print(SEPARATOR)

        return twopc_pb2.VoteResponse(
            transaction_id=request.transaction_id,
            node_id=node_id,
            commit=commit_decision
        )
def serve():
    port = os.getenv("PORT", "50051")
    node_id = os.getenv("NODE_ID", "unknown")

    print(f"[{node_id}] Phase Voting of Node {node_id} listening on port {port}...")

    server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))
    twopc_pb2_grpc.add_VotingServiceServicer_to_server(VotingParticipant(), server)
    server.add_insecure_port(f"[::]:{port}")
    server.start()
    try:
        while True:
            time.sleep(1)
    except KeyboardInterrupt:
        print(f"[{node_id}] Shutting down service...")
        server.stop(0)
    except Exception as e:
        print(f"[{node_id} ERROR] {e}", file=sys.stderr)
if __name__ == "__main__":
    serve()

import sys
import os

# Add the current directory to sys.path so that generated pb2/pb2_grpc files
# can resolve top-level imports of other pb2 modules.
sys.path.insert(0, os.path.abspath(os.path.dirname(__file__)))

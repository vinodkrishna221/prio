import os
import sys
import json
import pytest
import grpc
import concurrent.futures
from unittest.mock import patch, MagicMock

# Add project root and protos directory to sys.path for test run execution
sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "..")))
sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "../protos")))

import triage_pb2
import triage_pb2_grpc
import scheduler_pb2
import scheduler_pb2_grpc
from main import TriageServiceHandler, SchedulerServiceHandler

@pytest.fixture(scope="module")
def grpc_server():
    server = grpc.server(concurrent.futures.ThreadPoolExecutor(max_workers=1))
    triage_pb2_grpc.add_TriageServiceServicer_to_server(TriageServiceHandler(), server)
    scheduler_pb2_grpc.add_SchedulerServiceServicer_to_server(SchedulerServiceHandler(), server)
    # Bind to port 0 for an ephemeral port
    port = server.add_insecure_port("[::]:0")
    server.start()
    yield f"localhost:{port}"
    server.stop(0)


def test_grpc_triage_service(grpc_server) -> None:
    # Mock Gemini model response to return HIGH effort
    mock_response = MagicMock()
    mock_response.text = json.dumps({
        "triage_priority_score": 90,
        "urgency_level": "CRITICAL",
        "action_type": "GMAIL_DRAFT",
        "draft_payload_json": '{"to": "boss@company.com"}',
        "friction_saved_minutes": "15",
        "cognitive_effort": "HIGH"
    })
    
    with patch("agents.triage_agent.GenerativeModel") as mock_model_cls:
        mock_model_cls.return_value.generate_content.return_value = mock_response
        
        with grpc.insecure_channel(grpc_server) as channel:
            stub = triage_pb2_grpc.TriageServiceStub(channel)
            
            request = triage_pb2.ProcessTriageRequest(
                user_id="u1",
                email_id="e1",
                subject="Help needed",
                sender="friend@test.com",
                body_content="Can you review this code?",
                received_timestamp=1719219600,
                user_context=triage_pb2.UserContext(
                    energy_score=3,  # low energy -> demote priority from 90 to 70
                    current_location="Home",
                    active_task_tags=["help"]
                )
            )
            
            response = stub.ProcessTriage(request)
            
            assert response.triage_priority_score == 70
            assert response.urgency_level == "QUIET"
            assert response.action_type == "GMAIL_DRAFT"
            assert response.task_id == "e1"


def test_grpc_scheduler_service(grpc_server) -> None:
    # 1719219600 corresponds to Monday June 24, 2024 at 9:00 AM UTC
    # We mock current time to that
    with patch("time.time", return_value=1719219600):
        with grpc.insecure_channel(grpc_server) as channel:
            stub = scheduler_pb2_grpc.SchedulerServiceStub(channel)
            
            request = scheduler_pb2.MatchScheduleRequest(
                user_id="u1",
                user_energy_score=5,
                busy_slots=[
                    scheduler_pb2.CalendarEvent(
                        event_id="b1",
                        start_time=1719223200,
                        end_time=1719226800,
                        is_tentative=False
                    )
                ],
                task_pool=[
                    scheduler_pb2.TaskItem(
                        task_id="t1",
                        title="Short task",
                        estimated_duration_minutes=30,
                        priority=50,
                        hard_deadline=1719252000
                    )
                ]
            )
            
            response = stub.MatchSchedule(request)
            
            assert response.user_id == "u1"
            assert len(response.allocations) == 1
            assert response.allocations[0].task_id == "t1"
            assert response.allocations[0].start_time == 1719219600
            assert response.allocations[0].end_time == 1719219600 + 1800

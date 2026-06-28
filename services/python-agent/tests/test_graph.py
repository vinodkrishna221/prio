import os
import sys
import json
import pytest
from unittest.mock import MagicMock, patch

# Add project root and protos directory to sys.path for test run execution
sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "..")))
sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "../protos")))

from agents.graph import triage_graph, scheduler_graph
from agents.state import TriageState, SchedulerState
from agents.triage_agent import triage_agent_node, get_genai_client

def test_triage_graph_traversal() -> None:
    """
    Tests that the Triage Graph successfully runs triage_agent and biometric_agent.
    """
    inputs: TriageState = {
        "user_id": "user123",
        "email_id": "email456",
        "subject": "Important Meeting",
        "sender": "boss@company.com",
        "body_content": "We need to discuss the quarterly goals tomorrow morning.",
        "received_timestamp": 1719320000,
        "energy_score": 3,  # Low energy -> should trigger demotion of HIGH effort task
        "current_location": "Office",
        "active_task_tags": ["work"],
        "triage_priority_score": 0,
        "urgency_level": "",
        "action_type": "",
        "draft_payload_json": "",
        "friction_saved_minutes": "",
        "cognitive_effort": "",
        "task_id": ""
    }
    
    # Mock Gemini model response to return HIGH effort
    mock_client = MagicMock()
    mock_response = MagicMock()
    mock_response.text = json.dumps({
        "triage_priority_score": 90,
        "urgency_level": "CRITICAL",
        "action_type": "GMAIL_DRAFT",
        "draft_payload_json": '{"to": "boss@company.com"}',
        "friction_saved_minutes": "15",
        "cognitive_effort": "HIGH"
    })
    mock_client.models.generate_content.return_value = mock_response
    
    with patch("agents.triage_agent.get_genai_client", return_value=mock_client):
        # Run graph
        result = triage_graph.invoke(inputs)
        
        # Verify initial Gemini parsing was called
        mock_client.models.generate_content.assert_called_once()
        
        # Verify BiometricAgent demoted the HIGH effort task since energy is 3 (<= 4)
        # 90 priority - 20 = 70. Urgency CRITICAL -> QUIET.
        assert result["triage_priority_score"] == 70
        assert result["urgency_level"] == "QUIET"
        assert result["cognitive_effort"] == "HIGH"
        assert result["action_type"] == "GMAIL_DRAFT"
        assert result["task_id"] == "email456"


def test_scheduler_graph_traversal() -> None:
    """
    Tests that the Scheduler Graph successfully runs scheduler_agent and biometric_agent.
    """
    inputs: SchedulerState = {
        "user_id": "user123",
        "busy_slots": [
            {
                "event_id": "event1",
                "start_time": 1719223200,
                "end_time": 1719226800,
                "is_tentative": False
            }
        ],
        "task_pool": [
            {
                "task_id": "task1",
                "title": "Debug backend issue",
                "estimated_duration_minutes": 30,
                "priority": 80,
                "hard_deadline": 1719252000,  # Monday 6:00 PM UTC
                "cognitive_effort": "HIGH"
            }
        ],
        "user_energy_score": 8,  # High energy
        "allocations": []
    }
    
    # Mock system time to monday 9:00 AM UTC
    with patch("time.time", return_value=1719219600):
        result = scheduler_graph.invoke(inputs)
        
        # Verify we got allocations
        allocations = result["allocations"]
        assert len(allocations) > 0
        
        # Verify the HIGH effort task got scheduled in the earliest morning peak hours if possible
        # Earliest free gap starts at 9:00 AM (1719219600) to 10:00 AM (1719223200)
        # So task1 (HIGH effort) should be at 1719219600
        assert allocations[0]["task_id"] == "task1"
        assert allocations[0]["start_time"] == 1719219600
        assert allocations[0]["end_time"] == 1719219600 + 30 * 60


def test_get_genai_client_success() -> None:
    import agents.triage_agent
    agents.triage_agent._client = None
    with patch("agents.triage_agent.genai.Client") as mock_client_cls, \
         patch.dict(os.environ, {"GCP_PROJECT": "test-project", "GCP_LOCATION": "test-loc"}):
        
        client = get_genai_client()
        
        mock_client_cls.assert_called_once_with(vertexai=True, project="test-project", location="test-loc")
        assert client == mock_client_cls.return_value


def test_get_genai_client_failure() -> None:
    import agents.triage_agent
    agents.triage_agent._client = None
    with patch.dict(os.environ, {}, clear=True):
        with pytest.raises(ValueError, match="GCP_PROJECT or PROJECT_ID environment variable is not set"):
            get_genai_client()


def test_triage_agent_fallback_on_exception() -> None:
    inputs: TriageState = {
        "user_id": "u1",
        "email_id": "e1",
        "subject": "Hi",
        "sender": "sender@test.com",
        "body_content": "Hello",
        "received_timestamp": 123,
        "energy_score": 5,
        "current_location": "",
        "active_task_tags": [],
        "triage_priority_score": 0,
        "urgency_level": "",
        "action_type": "",
        "draft_payload_json": "",
        "friction_saved_minutes": "",
        "cognitive_effort": "",
        "task_id": ""
    }
    
    with patch("agents.triage_agent.get_genai_client") as mock_get_client:
        # Force generate_content to raise an error
        mock_get_client.return_value.models.generate_content.side_effect = Exception("Vertex AI error")
        
        result = triage_agent_node(inputs)
        
        # Verify fallback values
        assert result["triage_priority_score"] == 50
        assert result["urgency_level"] == "AMBIENT"
        assert result["action_type"] == "GMAIL_DRAFT"
        assert result["friction_saved_minutes"] == "2"
        assert result["cognitive_effort"] == "MEDIUM"
        assert result["task_id"] == "e1"

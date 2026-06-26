import os
import sys
import time
from unittest.mock import patch

# Add project root and protos directory to sys.path for test run execution
sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "..")))
sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "../protos")))

from agents.biometric_agent import biometric_agent_node, biometric_scheduler_node, classify_cognitive_effort
from agents.state import TriageState, SchedulerState

def test_classify_cognitive_effort() -> None:
    assert classify_cognitive_effort("Design new api", 30) == "HIGH"
    assert classify_cognitive_effort("Implement database migration", 20) == "HIGH"
    assert classify_cognitive_effort("Write unit tests", 75) == "HIGH"
    assert classify_cognitive_effort("Pay water bill", 15) == "LOW"
    assert classify_cognitive_effort("Send follow-up email", 10) == "LOW"
    assert classify_cognitive_effort("Refactor code", 10) == "HIGH"
    assert classify_cognitive_effort("Do laundry", 45) == "MEDIUM"


def test_biometric_triage_demotion() -> None:
    # Energy <= 4 and HIGH effort -> priority should decrease by 20, urgency demote
    state: TriageState = {
        "user_id": "u1",
        "email_id": "e1",
        "subject": "Plan launch event",
        "sender": "partner@event.com",
        "body_content": "We need to plan the event launch checklist.",
        "received_timestamp": 12345,
        "energy_score": 3,  # Low energy
        "current_location": "Home",
        "active_task_tags": [],
        "triage_priority_score": 90,
        "urgency_level": "CRITICAL",
        "action_type": "CALENDAR_BOOKING",
        "draft_payload_json": "{}",
        "friction_saved_minutes": "30",
        "cognitive_effort": "HIGH",
        "task_id": "e1"
    }
    
    res = biometric_agent_node(state)
    assert res["triage_priority_score"] == 70  # 90 - 20
    assert res["urgency_level"] == "QUIET"    # CRITICAL -> QUIET


def test_biometric_triage_promotion() -> None:
    # Energy >= 7 and HIGH effort -> priority increase by 15, urgency promote
    state: TriageState = {
        "user_id": "u1",
        "email_id": "e1",
        "subject": "Plan launch event",
        "sender": "partner@event.com",
        "body_content": "We need to plan the event launch checklist.",
        "received_timestamp": 12345,
        "energy_score": 8,  # High energy
        "current_location": "Home",
        "active_task_tags": [],
        "triage_priority_score": 60,
        "urgency_level": "QUIET",
        "action_type": "CALENDAR_BOOKING",
        "draft_payload_json": "{}",
        "friction_saved_minutes": "30",
        "cognitive_effort": "HIGH",
        "task_id": "e1"
    }
    
    res = biometric_agent_node(state)
    assert res["triage_priority_score"] == 75  # 60 + 15
    assert res["urgency_level"] == "CRITICAL" # QUIET -> CRITICAL


def test_biometric_scheduler_low_energy_demotion() -> None:
    # Low energy -> HIGH effort task scheduled in next 24h should be swapped with LOW effort task scheduled after 24h
    now = 100000
    state: SchedulerState = {
        "user_id": "u1",
        "busy_slots": [],
        "task_pool": [
            {
                "task_id": "task_high",
                "title": "Design microservice architecture",
                "estimated_duration_minutes": 60,
                "priority": 90,
                "hard_deadline": 200000,
                "cognitive_effort": "HIGH"
            },
            {
                "task_id": "task_low",
                "title": "Pay gas bill",
                "estimated_duration_minutes": 15,
                "priority": 50,
                "hard_deadline": 200000,
                "cognitive_effort": "LOW"
            }
        ],
        "user_energy_score": 2,  # Low energy
        "allocations": [
            # High effort task in near term (next 24h)
            {
                "task_id": "task_high",
                "start_time": now + 3600,  # +1h
                "end_time": now + 7200,
                "allocation_type": "FOCUS_BLOCK",
                "create_ghost_block": False
            },
            # Low effort task in far term (after 24h)
            {
                "task_id": "task_low",
                "start_time": now + 30 * 3600,  # +30h
                "end_time": now + 30 * 3600 + 900,
                "allocation_type": "MICRO_GAP",
                "create_ghost_block": False
            }
        ]
    }
    
    with patch("time.time", return_value=now):
        res = biometric_scheduler_node(state)
        allocs = res["allocations"]
        
        # Verify tasks have swapped slots!
        # Slot 1 (+1h) should now belong to task_low
        assert allocs[0]["task_id"] == "task_low"
        # Slot 2 (+30h) should now belong to task_high
        assert allocs[1]["task_id"] == "task_high"

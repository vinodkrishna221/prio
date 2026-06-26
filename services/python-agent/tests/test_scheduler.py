import os
import sys
import time
from unittest.mock import patch

# Add project root and protos directory to sys.path for test run execution
sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "..")))
sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "../protos")))

from agents.scheduler_agent import scheduler_agent_node, get_working_windows, get_latest_working_window_before
from agents.state import SchedulerState

def test_get_working_windows() -> None:
    # 1719219600 corresponds to Monday June 24, 2024 at 9:00 AM UTC
    windows = get_working_windows(1719219600, max_days=3)
    assert len(windows) == 3
    # First window should be Monday 9:00 AM to 6:00 PM (1719219600 to 1719252000)
    assert windows[0] == (1719219600, 1719252000)


def test_get_latest_working_window_before() -> None:
    # Deadline: Monday June 24, 2024 at 5:00 PM UTC (1719248400)
    # Duration: 30 minutes (1800 seconds)
    # Should schedule 4:30 PM to 5:00 PM
    start, end = get_latest_working_window_before(1719248400, 1800)
    assert end == 1719248400
    assert start == 1719248400 - 1800
    
    # Deadline: Monday June 24, 2024 at 10:00 PM UTC (1719266400 - outside working hours)
    # Should schedule ending at 6:00 PM UTC (1719252000)
    start, end = get_latest_working_window_before(1719266400, 1800)
    assert end == 1719252000
    assert start == 1719252000 - 1800


def test_greedy_scheduler_success() -> None:
    # 1719219600 = Monday 9:00 AM UTC
    state: SchedulerState = {
        "user_id": "user123",
        "busy_slots": [
            # Busy 11:00 AM - 1:00 PM UTC (7200 seconds)
            {
                "event_id": "b1",
                "start_time": 1719226800,
                "end_time": 1719234000,
                "is_tentative": False
            }
        ],
        "task_pool": [
            {
                "task_id": "t1",
                "title": "Email clients",
                "estimated_duration_minutes": 30,
                "priority": 50,
                "hard_deadline": 1719252000,  # Monday 6 PM UTC
                "cognitive_effort": "LOW"
            },
            {
                "task_id": "t2",
                "title": "Write report",
                "estimated_duration_minutes": 60,
                "priority": 60,
                "hard_deadline": 1719252000,
                "cognitive_effort": "HIGH"
            }
        ],
        "user_energy_score": 5,
        "allocations": []
    }
    
    with patch("time.time", return_value=1719219600):
        res = scheduler_agent_node(state)
        allocs = res["allocations"]
        
        # We sorted by deadline and priority. t2 (priority 60) scheduled first, then t1 (priority 50).
        # Both should fit in the morning gap (9:00 AM to 11:00 AM)
        assert len(allocs) == 2
        
        # t2 (duration 60m) gets earliest: 9:00 - 10:00 UTC
        assert allocs[0]["task_id"] == "t2"
        assert allocs[0]["start_time"] == 1719219600
        assert allocs[0]["end_time"] == 1719219600 + 3600
        
        # t1 (duration 30m) gets next: 10:00 - 10:30 UTC
        assert allocs[1]["task_id"] == "t1"
        assert allocs[1]["start_time"] == 1719219600 + 3600
        assert allocs[1]["end_time"] == 1719219600 + 3600 + 1800


def test_ghost_block_allocation() -> None:
    # 1719219600 = Monday 9:00 AM UTC
    # Calendar is fully booked (busy 9:00 AM to 6:00 PM)
    state: SchedulerState = {
        "user_id": "user123",
        "busy_slots": [
            {
                "event_id": "b1",
                "start_time": 1719219600,
                "end_time": 1719252000,
                "is_tentative": False
            }
        ],
        "task_pool": [
            # High priority task (priority 80 >= 70) should create a Ghost Block
            {
                "task_id": "high_prio",
                "title": "Fix critical bug",
                "estimated_duration_minutes": 60,
                "priority": 85,
                "hard_deadline": 1719252000,
                "cognitive_effort": "HIGH"
            },
            # Low priority task (priority 40 < 70) should NOT get scheduled (skipped)
            {
                "task_id": "low_prio",
                "title": "Clean desk",
                "estimated_duration_minutes": 30,
                "priority": 40,
                "hard_deadline": 1719252000,
                "cognitive_effort": "LOW"
            }
        ],
        "user_energy_score": 5,
        "allocations": []
    }
    
    with patch("time.time", return_value=1719219600):
        res = scheduler_agent_node(state)
        allocs = res["allocations"]
        
        # Only high_prio should be allocated (as a ghost block)
        assert len(allocs) == 1
        assert allocs[0]["task_id"] == "high_prio"
        assert allocs[0]["create_ghost_block"] is True
        assert allocs[0]["start_time"] == 1719252000 - 3600
        assert allocs[0]["end_time"] == 1719252000

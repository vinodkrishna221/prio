from typing import TypedDict, List, Optional

class TriageState(TypedDict):
    user_id: str
    email_id: str
    subject: str
    sender: str
    body_content: str
    received_timestamp: int
    energy_score: int
    current_location: str
    active_task_tags: List[str]
    
    # Outputs generated during execution
    triage_priority_score: int
    urgency_level: str  # "AMBIENT", "QUIET", "CRITICAL"
    action_type: str    # "GMAIL_DRAFT", "CALENDAR_BOOKING", "BILL_PAY"
    draft_payload_json: str
    friction_saved_minutes: str
    cognitive_effort: str # "HIGH", "MEDIUM", "LOW"
    task_id: str


class CalendarEventDict(TypedDict):
    event_id: str
    start_time: int
    end_time: int
    is_tentative: bool


class TaskItemDict(TypedDict):
    task_id: str
    title: str
    estimated_duration_minutes: int
    priority: int
    hard_deadline: int
    cognitive_effort: Optional[str]


class ScheduledAllocationDict(TypedDict):
    task_id: str
    start_time: int
    end_time: int
    allocation_type: str  # "FOCUS_BLOCK", "MICRO_GAP"
    create_ghost_block: bool


class SchedulerState(TypedDict):
    user_id: str
    busy_slots: List[CalendarEventDict]
    task_pool: List[TaskItemDict]
    user_energy_score: int
    allocations: List[ScheduledAllocationDict]

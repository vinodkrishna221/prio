import time
import structlog
from datetime import datetime, timezone
from typing import Dict, List, Any
from agents.state import TriageState, SchedulerState, ScheduledAllocationDict, TaskItemDict

logger = structlog.get_logger()


def classify_cognitive_effort(title: str, duration_mins: int) -> str:
    """
    Heuristically classifies a task's cognitive effort as HIGH, MEDIUM, or LOW
    based on its title keywords and estimated duration.
    """
    title_lower = title.lower()
    
    # Keyword matches
    high_keywords = ["design", "implement", "debug", "write", "code", "plan", "analyze", "review", "refactor"]
    low_keywords = ["pay", "send", "call", "email", "draft", "check", "clean", "buy", "approve"]
    
    if any(kw in title_lower for kw in high_keywords) or duration_mins > 60:
        return "HIGH"
    elif any(kw in title_lower for kw in low_keywords) or duration_mins <= 30:
        return "LOW"
    else:
        return "MEDIUM"


def biometric_agent_node(state: TriageState) -> TriageState:
    """
    Adjusts task priority/urgency based on current user energy level in the Triage Graph.
    """
    energy = state.get("energy_score", 5)
    effort = state.get("cognitive_effort", "MEDIUM")
    priority = state.get("triage_priority_score", 50)
    urgency = state.get("urgency_level", "AMBIENT")
    
    original_priority = priority
    original_urgency = urgency
    
    # Demotion logic (Low energy <= 4, High cognitive effort)
    if energy <= 4 and effort == "HIGH":
        priority = max(1, priority - 20)
        if urgency == "CRITICAL":
            urgency = "QUIET"
        elif urgency == "QUIET":
            urgency = "AMBIENT"
            
    # Promotion logic (High energy >= 7, High cognitive effort)
    elif energy >= 7 and effort == "HIGH":
        priority = min(100, priority + 15)
        if urgency == "AMBIENT":
            urgency = "QUIET"
        elif urgency == "QUIET":
            urgency = "CRITICAL"
            
    state["triage_priority_score"] = priority
    state["urgency_level"] = urgency
    
    logger.info(
        "biometric_triage_adjusted",
        energy=energy,
        effort=effort,
        original_priority=original_priority,
        adjusted_priority=priority,
        original_urgency=original_urgency,
        adjusted_urgency=urgency
    )
    
    return state


def biometric_scheduler_node(state: SchedulerState) -> SchedulerState:
    """
    Adjusts scheduler allocations based on biometric energy levels.
    """
    energy = state.get("user_energy_score", 5)
    allocations = list(state.get("allocations", []))
    tasks = state.get("task_pool", [])
    
    if not allocations or not tasks:
        return state
        
    now = int(time.time())
    
    # Create helper dictionary for task properties
    task_map: Dict[str, TaskItemDict] = {t["task_id"]: t for t in tasks}
    
    # Resolve and store cognitive effort for each task in the map
    task_efforts: Dict[str, str] = {}
    for task_id, task in task_map.items():
        effort = task.get("cognitive_effort")
        if not effort:
            effort = classify_cognitive_effort(task.get("title", ""), task.get("estimated_duration_minutes", 15))
        task_efforts[task_id] = effort

    # Helper function to check if a slot falls in peak focus hours (e.g. morning 9 AM to 12 PM UTC)
    def is_peak_focus_block(alloc: ScheduledAllocationDict) -> bool:
        start_dt = datetime.fromtimestamp(alloc["start_time"], tz=timezone.utc)
        is_morning = 9 <= start_dt.hour < 12
        is_long = (alloc["end_time"] - alloc["start_time"]) >= 3600  # >= 1 hour
        return is_morning or is_long

    # 1. Demotion logic: Low energy (<= 4) -> shift High effort tasks out of short-term (next 24 hours)
    if energy <= 4:
        logger.info("biometric_scheduler.low_energy_demotion_triggered", energy=energy)
        # Separate allocations into next 24h and after 24h
        threshold_24h = now + 24 * 3600
        
        near_allocs: List[ScheduledAllocationDict] = []
        far_allocs: List[ScheduledAllocationDict] = []
        
        for alloc in allocations:
            if alloc["start_time"] < threshold_24h:
                near_allocs.append(alloc)
            else:
                far_allocs.append(alloc)
                
        # Find High effort tasks in near_allocs and try to swap with Low/Medium effort tasks in far_allocs
        for near_idx, near_alloc in enumerate(near_allocs):
            near_task_id = near_alloc["task_id"]
            if task_efforts.get(near_task_id) == "HIGH":
                # Find a swap target in far_allocs that is LOW/MEDIUM effort
                for far_idx, far_alloc in enumerate(far_allocs):
                    far_task_id = far_alloc["task_id"]
                    if task_efforts.get(far_task_id) in ["LOW", "MEDIUM"]:
                        # Swap their times/types
                        logger.info("biometric_scheduler.swap_low_energy", near_task=near_task_id, far_task=far_task_id)
                        
                        near_alloc["task_id"], far_alloc["task_id"] = far_task_id, near_task_id
                        break
                        
        # Recombine allocations
        state["allocations"] = near_allocs + far_allocs

    # 2. Promotion logic: High energy (>= 7) -> promote High effort tasks to peak focus blocks
    elif energy >= 7:
        logger.info("biometric_scheduler.high_energy_promotion_triggered", energy=energy)
        # Attempt to swap High-effort tasks in non-peak blocks with Low-effort tasks in peak focus blocks
        for i, alloc_i in enumerate(allocations):
            task_i = alloc_i["task_id"]
            effort_i = task_efforts.get(task_i, "MEDIUM")
            
            # If task i is High-effort but NOT in a peak focus block
            if effort_i == "HIGH" and not is_peak_focus_block(alloc_i):
                # Search for another task j that is Low-effort but scheduled in a peak focus block
                for j, alloc_j in enumerate(allocations):
                    task_j = alloc_j["task_id"]
                    effort_j = task_efforts.get(task_j, "MEDIUM")
                    
                    if effort_j == "LOW" and is_peak_focus_block(alloc_j):
                        logger.info("biometric_scheduler.swap_high_energy", non_peak_high=task_i, peak_low=task_j)
                        # Swap their times/types
                        alloc_i["task_id"], alloc_j["task_id"] = task_j, task_i
                        break
                        
    return state

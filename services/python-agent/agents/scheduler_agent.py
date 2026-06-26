import time
import structlog
from datetime import datetime, timezone, timedelta
from typing import List, Tuple, Dict, Any, Optional
from agents.state import SchedulerState, ScheduledAllocationDict, CalendarEventDict, TaskItemDict

logger = structlog.get_logger()


def get_working_windows(now_ts: int, max_days: int = 14) -> List[Tuple[int, int]]:
    """
    Generates working hour windows (9:00 AM to 6:00 PM UTC) for the next max_days.
    Clamps windows starting before now_ts to now_ts.
    """
    now_dt = datetime.fromtimestamp(now_ts, tz=timezone.utc)
    working_windows = []
    
    # Generate windows starting from the current day
    for i in range(max_days):
        day_dt = now_dt + timedelta(days=i)
        
        # Working hours: 9:00 AM to 6:00 PM (18:00) UTC
        w_start_dt = day_dt.replace(hour=9, minute=0, second=0, microsecond=0)
        w_end_dt = day_dt.replace(hour=18, minute=0, second=0, microsecond=0)
        
        w_start = int(w_start_dt.timestamp())
        w_end = int(w_end_dt.timestamp())
        
        if w_end <= now_ts:
            continue
            
        if w_start < now_ts:
            w_start = now_ts
            
        if w_start < w_end:
            working_windows.append((w_start, w_end))
            
    return working_windows


def subtract_busy_slots(working_windows: List[Tuple[int, int]], busy_slots: List[CalendarEventDict]) -> List[Tuple[int, int]]:
    """
    Subtracts busy slots from the list of working windows to find free gaps.
    """
    busy_intervals = [(slot["start_time"], slot["end_time"]) for slot in busy_slots]
    busy_intervals.sort()
    
    free_gaps = []
    for w_start, w_end in working_windows:
        current_start = w_start
        for b_start, b_end in busy_intervals:
            if b_end <= current_start:
                continue
            if b_start >= w_end:
                break
            
            if b_start > current_start:
                free_gaps.append((current_start, b_start))
            current_start = max(current_start, b_end)
            if current_start >= w_end:
                break
                
        if current_start < w_end:
            free_gaps.append((current_start, w_end))
            
    return free_gaps


def get_latest_working_window_before(deadline: int, duration_seconds: int) -> Tuple[int, int]:
    """
    Finds the latest working hours segment ending at or before deadline.
    If the deadline is in the middle of a working window, the block ends at deadline.
    If the deadline is after working hours, the block ends at 6:00 PM (18:00) of that day.
    """
    dl_dt = datetime.fromtimestamp(deadline, tz=timezone.utc)
    
    # Working hours for deadline day
    w_start_dt = dl_dt.replace(hour=9, minute=0, second=0, microsecond=0)
    w_end_dt = dl_dt.replace(hour=18, minute=0, second=0, microsecond=0)
    
    w_start = int(w_start_dt.timestamp())
    w_end = int(w_end_dt.timestamp())
    
    if deadline >= w_end:
        # Deadline is after working hours on this day. Schedule ending at 6:00 PM.
        end_time = w_end
        start_time = max(w_start, w_end - duration_seconds)
        return start_time, end_time
    elif deadline > w_start:
        # Deadline falls during working hours on this day. Schedule ending at deadline.
        end_time = deadline
        start_time = max(w_start, deadline - duration_seconds)
        return start_time, end_time
    else:
        # Deadline is before working hours on this day. Look at previous day's working window.
        prev_day = dl_dt - timedelta(days=1)
        pw_start_dt = prev_day.replace(hour=9, minute=0, second=0, microsecond=0)
        pw_end_dt = prev_day.replace(hour=18, minute=0, second=0, microsecond=0)
        
        pw_start = int(pw_start_dt.timestamp())
        pw_end = int(pw_end_dt.timestamp())
        
        end_time = pw_end
        start_time = max(pw_start, pw_end - duration_seconds)
        return start_time, end_time


def scheduler_agent_node(state: SchedulerState) -> SchedulerState:
    """
    Greedy task scheduler that maps tasks to calendar gaps.
    """
    now = int(time.time())
    
    # Sort tasks by hard_deadline (ascending) and priority (descending)
    tasks = list(state.get("task_pool", []))
    # Using stable sort or composite key
    tasks.sort(key=lambda t: (t.get("hard_deadline", 0), -t.get("priority", 0)))
    
    # Generate free gaps >= 15 minutes
    working_windows = get_working_windows(now)
    free_gaps = subtract_busy_slots(working_windows, state.get("busy_slots", []))
    free_gaps = [gap for gap in free_gaps if (gap[1] - gap[0]) >= 15 * 60]
    
    allocations: List[ScheduledAllocationDict] = []
    
    for task in tasks:
        task_id = task.get("task_id", "")
        duration_mins = task.get("estimated_duration_minutes", 15)
        duration_secs = duration_mins * 60
        deadline = task.get("hard_deadline", 0)
        priority = task.get("priority", 0)
        
        allocated = False
        
        # Try to find the earliest fitting free gap before the deadline
        for idx, (gap_start, gap_end) in enumerate(free_gaps):
            # The gap must start before the deadline
            if gap_start >= deadline:
                break
                
            # Max time we can allocate inside this gap is the minimum of gap_end and deadline
            max_alloc_end = min(gap_end, deadline)
            if (max_alloc_end - gap_start) >= duration_secs:
                start_time = gap_start
                end_time = gap_start + duration_secs
                
                # Allocation type
                alloc_type = "FOCUS_BLOCK" if duration_mins >= 60 else "MICRO_GAP"
                
                allocations.append({
                    "task_id": task_id,
                    "start_time": start_time,
                    "end_time": end_time,
                    "allocation_type": alloc_type,
                    "create_ghost_block": False
                })
                
                # Update free gaps list
                remaining_start = end_time
                if (gap_end - remaining_start) >= 15 * 60:
                    free_gaps[idx] = (remaining_start, gap_end)
                else:
                    free_gaps.pop(idx)
                
                allocated = True
                break
                
        if not allocated:
            # High priority tasks (priority >= 70) get a Ghost Block right before deadline
            if priority >= 70:
                logger.info("scheduler.ghost_block_created", task_id=task_id, deadline=deadline)
                start_time, end_time = get_latest_working_window_before(deadline, duration_secs)
                allocations.append({
                    "task_id": task_id,
                    "start_time": start_time,
                    "end_time": end_time,
                    "allocation_type": "FOCUS_BLOCK",
                    "create_ghost_block": True
                })
            else:
                logger.info("scheduler.task_skipped_no_gap", task_id=task_id, priority=priority)
                
    state["allocations"] = allocations
    return state

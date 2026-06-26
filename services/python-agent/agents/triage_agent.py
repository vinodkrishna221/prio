import os
import json
import structlog
from typing import Any, Dict
import vertexai
from vertexai.generative_models import GenerativeModel, GenerationConfig
from agents.state import TriageState

logger = structlog.get_logger()

# Schema matching the triage output format
TRIAGE_SCHEMA: Dict[str, Any] = {
    "type": "OBJECT",
    "properties": {
        "triage_priority_score": {
            "type": "INTEGER",
            "description": "Task priority score from 1 (lowest) to 100 (highest)."
        },
        "urgency_level": {
            "type": "STRING",
            "enum": ["AMBIENT", "QUIET", "CRITICAL"],
            "description": "Urgency based on task deadlines: AMBIENT, QUIET, or CRITICAL."
        },
        "action_type": {
            "type": "STRING",
            "enum": ["GMAIL_DRAFT", "CALENDAR_BOOKING", "BILL_PAY"],
            "description": "Best action path: GMAIL_DRAFT, CALENDAR_BOOKING, or BILL_PAY."
        },
        "draft_payload_json": {
            "type": "STRING",
            "description": "A serialized JSON string containing context-specific details. E.g., for GMAIL_DRAFT: {'to': '...', 'subject': '...', 'body': '...'}, for CALENDAR_BOOKING: {'title': '...', 'start_time': 0, 'duration': 30}, for BILL_PAY: {'vendor': '...', 'amount': 100.0, 'due_date': '...'}"
        },
        "friction_saved_minutes": {
            "type": "STRING",
            "description": "Estimated minutes saved by this automation, e.g. '15'."
        },
        "cognitive_effort": {
            "type": "STRING",
            "enum": ["HIGH", "MEDIUM", "LOW"],
            "description": "Cognitive load/effort required for user to complete the task."
        }
    },
    "required": [
        "triage_priority_score",
        "urgency_level",
        "action_type",
        "draft_payload_json",
        "friction_saved_minutes",
        "cognitive_effort"
    ]
}

_vertex_ai_initialized = False

def init_vertex_ai() -> None:
    global _vertex_ai_initialized
    if _vertex_ai_initialized:
        return
    project = os.environ.get("GCP_PROJECT") or os.environ.get("PROJECT_ID")
    location = os.environ.get("GCP_LOCATION", "us-central1")
    if project:
        try:
            vertexai.init(project=project, location=location)
            _vertex_ai_initialized = True
            logger.info("vertex_ai_initialized", project=project, location=location)
        except Exception as e:
            logger.error("vertex_ai_init_failed", error=str(e))
    else:
        logger.warning("vertex_ai_skipped_missing_project_id")


def triage_agent_node(state: TriageState) -> TriageState:
    """
    Node that runs the Vertex AI Gemini model to parse inbound email tasks
    and formulate initial triage, urgency, action, and payload drafts.
    """
    init_vertex_ai()
    
    prompt = f"""
    Analyze the following email to prioritize it, draft an automated action response, and determine task constraints:
    
    - Sender: {state.get('sender', '')}
    - Subject: {state.get('subject', '')}
    - Received Timestamp: {state.get('received_timestamp', 0)}
    - Email Body:
    {state.get('body_content', '')}
    
    - User Context:
      * Current Energy Score: {state.get('energy_score', 5)}
      * Current Location: {state.get('current_location', '')}
      * Active Task Tags: {state.get('active_task_tags', [])}
    
    Respond in JSON matching the requested schema.
    """
    
    model_name = os.environ.get("GEMINI_MODEL", "gemini-2.0-flash")
    
    try:
        model = GenerativeModel(model_name)
        config = GenerationConfig(
            response_mime_type="application/json",
            response_schema=TRIAGE_SCHEMA
        )
        
        response = model.generate_content(prompt, generation_config=config)
        result = json.loads(response.text)
        
        state["triage_priority_score"] = int(result.get("triage_priority_score", 50))
        state["urgency_level"] = str(result.get("urgency_level", "AMBIENT"))
        state["action_type"] = str(result.get("action_type", "GMAIL_DRAFT"))
        state["draft_payload_json"] = str(result.get("draft_payload_json", "{}"))
        state["friction_saved_minutes"] = str(result.get("friction_saved_minutes", "0"))
        state["cognitive_effort"] = str(result.get("cognitive_effort", "MEDIUM"))
        
        logger.info(
            "triage_agent.completed",
            priority=state["triage_priority_score"],
            urgency=state["urgency_level"],
            effort=state["cognitive_effort"]
        )
    except Exception as e:
        logger.error("triage_agent.failed", error=str(e))
        # Fallback values if API call fails
        state["triage_priority_score"] = 50
        state["urgency_level"] = "AMBIENT"
        state["action_type"] = "GMAIL_DRAFT"
        state["draft_payload_json"] = json.dumps({
            "to": state.get("sender", ""),
            "subject": f"Re: {state.get('subject', '')}",
            "body": "Thank you for your email. I will look into it."
        })
        state["friction_saved_minutes"] = "2"
        state["cognitive_effort"] = "MEDIUM"
        
    # Make sure task_id is populated (if not already there)
    if not state.get("task_id"):
        # Default task_id to email_id or a generic task identifier
        state["task_id"] = state.get("email_id", "task_unknown")
        
    return state

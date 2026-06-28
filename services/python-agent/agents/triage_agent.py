import os
import json
import structlog
from typing import Any, Dict
import google.genai as genai
from google.genai import types
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
            "description": (
                "A serialized JSON string containing context-specific details for the chosen action_type. "
                "For GMAIL_DRAFT: {\"to\": \"sender@example.com\", \"subject\": \"Re: ...\", \"body\": \"...\"}. "
                "For CALENDAR_BOOKING: {\"title\": \"Meeting title\", "
                "\"timeSlot\": \"3:15 PM - 3:45 PM\", "
                "\"date\": \"Today|Tomorrow|Jun 30\", "
                "\"location\": \"Google Meet\", "
                "\"description\": \"The meeting purpose or agenda extracted from the email body\", "
                "\"attendees\": [\"participant1@example.com\", \"participant2@example.com\"]}"
                " — attendees must be a JSON array of email addresses of all people invited to the meeting, "
                "extracted from the email body, CC fields, or any email addresses mentioned. "
                "description must summarize what the meeting is for, taken from the email body content. "
                "For BILL_PAY: {\"vendor\": \"...\", \"amount\": 100.0, \"due_date\": \"...\"}"
            )
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

_client: genai.Client | None = None


def get_genai_client() -> genai.Client:
    """Returns a singleton google-genai Client configured for Vertex AI."""
    global _client
    if _client is not None:
        return _client

    project = os.environ.get("GCP_PROJECT") or os.environ.get("PROJECT_ID")
    # Default to 'global' — gemini-2.5-flash is only available on the global/multi-region
    # endpoint (Enterprise/Global tier), not on regional endpoints like us-central1.
    # See: https://cloud.google.com/vertex-ai/generative-ai/docs/learn/locations
    location = os.environ.get("GCP_LOCATION", "global")

    if not project:
        logger.warning("vertex_ai_skipped_missing_project_id")
        raise ValueError("GCP_PROJECT or PROJECT_ID environment variable is not set")

    # gemini-2.5-flash is only available on the global/enterprise endpoint.
    # enterprise=True routes to the correct multi-region global endpoint.
    # Reset the singleton so a stale bad client is never reused.
    _client = genai.Client(vertexai=True, project=project, location=location)
    logger.info("vertex_ai_initialized", project=project, location=location)
    return _client


def triage_agent_node(state: TriageState) -> TriageState:
    """
    Node that runs the Vertex AI Gemini model to parse inbound email tasks
    and formulate initial triage, urgency, action, and payload drafts.
    Uses the google-genai SDK (v2+) with vertexai=True for Workload Identity auth.
    """
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

    Instructions for draft_payload_json:
    - If action_type is GMAIL_DRAFT: populate {{"to", "subject", "body"}} with a polished reply draft.
    - If action_type is CALENDAR_BOOKING:
        * title: a concise meeting title derived from the subject or email body.
        * timeSlot: suggest a realistic time slot string like "3:15 PM - 3:45 PM" based on context.
        * date: "Today", "Tomorrow", or a date like "Jun 30" if mentioned in the email.
        * location: extract a meeting link or location if present, otherwise default to "Google Meet".
        * description: summarize the meeting purpose and agenda from the email body (2-3 sentences max).
        * attendees: extract ALL email addresses of participants mentioned in the email body, CC list,
          or any other context. Include the sender's email. Return as a JSON array of strings.
    - If action_type is BILL_PAY: populate {{"vendor", "amount", "due_date"}}.

    Respond in JSON matching the requested schema.
    """

    # gemini-3.5-flash is only on global/enterprise; regional endpoints do NOT support it.
    model_name = os.environ.get("GEMINI_MODEL", "gemini-3.5-flash")

    try:
        client = get_genai_client()

        response = client.models.generate_content(
            model=model_name,
            contents=prompt,
            config=types.GenerateContentConfig(
                response_mime_type="application/json",
                response_schema=TRIAGE_SCHEMA,
            ),
        )

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
            action=state["action_type"],
            effort=state["cognitive_effort"],
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
            "body": "Thank you for your email. I will look into it.",
        })
        state["friction_saved_minutes"] = "2"
        state["cognitive_effort"] = "MEDIUM"

    # Make sure task_id is populated (if not already there)
    if not state.get("task_id"):
        state["task_id"] = state.get("email_id", "task_unknown")

    return state


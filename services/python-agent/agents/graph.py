from langgraph.graph import StateGraph, END
from agents.state import TriageState, SchedulerState
from agents.triage_agent import triage_agent_node
from agents.scheduler_agent import scheduler_agent_node
from agents.biometric_agent import biometric_agent_node, biometric_scheduler_node

# Build Triage Graph: TriageAgent -> BiometricAgent -> END
triage_workflow = StateGraph(TriageState)
triage_workflow.add_node("triage_agent", triage_agent_node)
triage_workflow.add_node("biometric_agent", biometric_agent_node)

triage_workflow.set_entry_point("triage_agent")
triage_workflow.add_edge("triage_agent", "biometric_agent")
triage_workflow.add_edge("biometric_agent", END)

triage_graph = triage_workflow.compile()


# Build Scheduler Graph: SchedulerAgent -> BiometricAgent -> END
scheduler_workflow = StateGraph(SchedulerState)
scheduler_workflow.add_node("scheduler_agent", scheduler_agent_node)
scheduler_workflow.add_node("biometric_agent", biometric_scheduler_node)

scheduler_workflow.set_entry_point("scheduler_agent")
scheduler_workflow.add_edge("scheduler_agent", "biometric_agent")
scheduler_workflow.add_edge("biometric_agent", END)

scheduler_graph = scheduler_workflow.compile()

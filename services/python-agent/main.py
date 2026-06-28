import os
import sys
import time
import signal
import concurrent.futures
from typing import Any
import structlog
import grpc
from grpc_health.v1 import health
from grpc_health.v1 import health_pb2
from grpc_health.v1 import health_pb2_grpc

# Add generated protos to path
sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "protos")))

import protos.triage_pb2 as triage_pb2
import protos.triage_pb2_grpc as triage_pb2_grpc
import protos.scheduler_pb2 as scheduler_pb2
import protos.scheduler_pb2_grpc as scheduler_pb2_grpc
import protos.genome_pb2 as genome_pb2
import protos.genome_pb2_grpc as genome_pb2_grpc

from agents.graph import triage_graph, scheduler_graph
from agents.genome_analyst import analyze_weekly_genome

logger = structlog.get_logger()


class TriageServiceHandler(triage_pb2_grpc.TriageServiceServicer):
    def ProcessTriage(
        self,
        request: triage_pb2.ProcessTriageRequest,
        context: grpc.ServicerContext
    ) -> triage_pb2.ProcessTriageResponse:
        logger.info("grpc.process_triage.received", user_id=request.user_id, email_id=request.email_id)
        try:
            inputs = {
                "user_id": request.user_id,
                "email_id": request.email_id,
                "subject": request.subject,
                "sender": request.sender,
                "body_content": request.body_content,
                "received_timestamp": request.received_timestamp,
                "energy_score": request.user_context.energy_score,
                "current_location": request.user_context.current_location,
                "active_task_tags": list(request.user_context.active_task_tags),
            }
            
            # Execute the LangGraph workflow
            outputs = triage_graph.invoke(inputs)
            
            logger.info("grpc.process_triage.success", user_id=request.user_id)
            return triage_pb2.ProcessTriageResponse(
                task_id=outputs.get("task_id", ""),
                triage_priority_score=outputs.get("triage_priority_score", 50),
                urgency_level=outputs.get("urgency_level", "AMBIENT"),
                action_type=outputs.get("action_type", "GMAIL_DRAFT"),
                draft_payload_json=outputs.get("draft_payload_json", "{}"),
                friction_saved_minutes=outputs.get("friction_saved_minutes", "0")
            )
        except Exception as e:
            logger.error("grpc.process_triage.failed", error=str(e), user_id=request.user_id)
            context.set_details(f"Triage execution failed: {str(e)}")
            context.set_code(grpc.StatusCode.INTERNAL)
            return triage_pb2.ProcessTriageResponse()


class SchedulerServiceHandler(scheduler_pb2_grpc.SchedulerServiceServicer):
    def MatchSchedule(
        self,
        request: scheduler_pb2.MatchScheduleRequest,
        context: grpc.ServicerContext
    ) -> scheduler_pb2.MatchScheduleResponse:
        logger.info("grpc.match_schedule.received", user_id=request.user_id)
        try:
            busy_slots = []
            for slot in request.busy_slots:
                busy_slots.append({
                    "event_id": slot.event_id,
                    "start_time": slot.start_time,
                    "end_time": slot.end_time,
                    "is_tentative": slot.is_tentative
                })
                
            task_pool = []
            for task in request.task_pool:
                task_pool.append({
                    "task_id": task.task_id,
                    "title": task.title,
                    "estimated_duration_minutes": task.estimated_duration_minutes,
                    "priority": task.priority,
                    "hard_deadline": task.hard_deadline,
                    "cognitive_effort": None
                })
                
            inputs = {
                "user_id": request.user_id,
                "busy_slots": busy_slots,
                "task_pool": task_pool,
                "user_energy_score": request.user_energy_score,
                "allocations": []
            }
            
            # Execute the LangGraph workflow
            outputs = scheduler_graph.invoke(inputs)
            
            allocations_proto = []
            for alloc in outputs.get("allocations", []):
                allocations_proto.append(scheduler_pb2.ScheduledAllocation(
                    task_id=alloc["task_id"],
                    start_time=alloc["start_time"],
                    end_time=alloc["end_time"],
                    allocation_type=alloc["allocation_type"],
                    create_ghost_block=alloc["create_ghost_block"]
                ))
                
            logger.info("grpc.match_schedule.success", user_id=request.user_id, allocations_count=len(allocations_proto))
            return scheduler_pb2.MatchScheduleResponse(
                user_id=outputs.get("user_id", ""),
                allocations=allocations_proto
            )
        except Exception as e:
            logger.error("grpc.match_schedule.failed", error=str(e), user_id=request.user_id)
            context.set_details(f"Schedule matching failed: {str(e)}")
            context.set_code(grpc.StatusCode.INTERNAL)
            return scheduler_pb2.MatchScheduleResponse()


class GenomeServiceHandler(genome_pb2_grpc.GenomeServiceServicer):
    def GenerateGenome(
        self,
        request: genome_pb2.GenerateGenomeRequest,
        context: grpc.ServicerContext
    ) -> genome_pb2.GenerateGenomeResponse:
        logger.info("grpc.generate_genome.received", user_id=request.user_id)
        try:
            tasks = []
            for t in request.tasks:
                tasks.append({
                    "task_id": t.task_id,
                    "title": t.title,
                    "source": t.source,
                    "status": t.status,
                    "priority_score": t.priority_score,
                    "duration_minutes": t.duration_minutes,
                    "due_at": t.due_at,
                    "saves_minutes": t.saves_minutes,
                })

            schedules = []
            for s in request.schedules:
                schedules.append({
                    "schedule_id": s.schedule_id,
                    "task_id": s.task_id,
                    "start_time": s.start_time,
                    "end_time": s.end_time,
                    "allocation_type": s.allocation_type,
                    "status": s.status,
                })

            biometrics = []
            for b in request.biometric_logs:
                biometrics.append({
                    "log_date": b.log_date,
                    "sleep_duration_hours": b.sleep_duration_hours,
                    "resting_heart_rate": b.resting_heart_rate,
                    "step_count": b.step_count,
                    "computed_energy_score": b.computed_energy_score,
                })

            result = analyze_weekly_genome(tasks, schedules, biometrics)

            insights_proto = []
            for insight in result.get("insights", []):
                insights_proto.append(genome_pb2.InsightCard(
                    category=insight["category"],
                    title=insight["title"],
                    description=insight["description"],
                    impact=insight["impact"]
                ))

            logger.info("grpc.generate_genome.success", user_id=request.user_id)
            return genome_pb2.GenerateGenomeResponse(
                deadline_risk_score=result.get("deadline_risk_score", 50),
                peak_hours=result.get("peak_hours", []),
                insights=insights_proto,
                scheduling_preferences_json=result.get("scheduling_preferences_json", "{}")
            )
        except Exception as e:
            logger.error("grpc.generate_genome.failed", error=str(e), user_id=request.user_id)
            context.set_details(f"Genome generation failed: {str(e)}")
            context.set_code(grpc.StatusCode.INTERNAL)
            return genome_pb2.GenerateGenomeResponse()


def serve() -> None:
    port = os.environ.get("PORT", "50051")
    server = grpc.server(concurrent.futures.ThreadPoolExecutor(max_workers=10))
    
    # Register our services
    triage_pb2_grpc.add_TriageServiceServicer_to_server(TriageServiceHandler(), server)
    scheduler_pb2_grpc.add_SchedulerServiceServicer_to_server(SchedulerServiceHandler(), server)
    genome_pb2_grpc.add_GenomeServiceServicer_to_server(GenomeServiceHandler(), server)
    
    # Register standard health check servicer
    health_servicer = health.HealthServicer()
    health_pb2_grpc.add_HealthServicer_to_server(health_servicer, server)
    
    # Mark status as SERVING
    health_servicer.set("", health_pb2.HealthCheckResponse.SERVING)
    health_servicer.set("triage.v1.TriageService", health_pb2.HealthCheckResponse.SERVING)
    health_servicer.set("scheduler.v1.SchedulerService", health_pb2.HealthCheckResponse.SERVING)
    health_servicer.set("genome.v1.GenomeService", health_pb2.HealthCheckResponse.SERVING)
    
    server.add_insecure_port(f"[::]:{port}")
    logger.info("grpc_server.starting", port=port)
    server.start()
    
    def graceful_shutdown(signum: Any, frame: Any) -> None:
        logger.info("grpc_server.graceful_shutdown_started")
        server.stop(5)
        logger.info("grpc_server.shutdown_complete")
        sys.exit(0)
        
    signal.signal(signal.SIGINT, graceful_shutdown)
    signal.signal(signal.SIGTERM, graceful_shutdown)
    
    # Keep main thread alive
    try:
        while True:
            time.sleep(3600)
    except KeyboardInterrupt:
        server.stop(0)


if __name__ == "__main__":
    # Configure structured json logging
    structlog.configure(
        processors=[
            structlog.processors.TimeStamper(fmt="iso"),
            structlog.processors.JSONRenderer(),
        ]
    )
    serve()

import os
import sys
import json
import pytest
import grpc
import concurrent.futures
from unittest.mock import patch, MagicMock

# Add project root and protos directory to sys.path
sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "..")))
sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "../protos")))

import genome_pb2
import genome_pb2_grpc
from main import GenomeServiceHandler


@pytest.fixture(scope="module")
def grpc_server():
    server = grpc.server(concurrent.futures.ThreadPoolExecutor(max_workers=1))
    genome_pb2_grpc.add_GenomeServiceServicer_to_server(GenomeServiceHandler(), server)
    port = server.add_insecure_port("[::]:0")
    server.start()
    yield f"localhost:{port}"
    server.stop(0)


def test_grpc_generate_genome_service(grpc_server) -> None:
    mock_client = MagicMock()
    mock_response = MagicMock()
    mock_response.text = json.dumps({
        "deadline_risk_score": 15,
        "peak_hours": ["Tuesday 9-11 AM"],
        "insights": [
            {
                "category": "ENERGY",
                "title": "Sleep Boosts Output",
                "description": "Your completion speed increases after 8h sleep.",
                "impact": "1.2x productivity"
            }
        ],
        "scheduling_preferences_json": '{"focusBlockTimeOfDay": "morning"}'
    })
    mock_client.models.generate_content.return_value = mock_response

    # Patch get_genai_client in the genome_analyst module
    with patch("agents.genome_analyst.get_genai_client", return_value=mock_client):
        with grpc.insecure_channel(grpc_server) as channel:
            stub = genome_pb2_grpc.GenomeServiceStub(channel)

            request = genome_pb2.GenerateGenomeRequest(
                user_id="u1",
                tasks=[
                    genome_pb2.HistoricalTask(
                        task_id="t1",
                        title="Acme Proposal",
                        source="GMAIL",
                        status="COMPLETED",
                        priority_score=85,
                        duration_minutes=30,
                        due_at=1719252000000,
                        saves_minutes=15
                    )
                ],
                schedules=[
                    genome_pb2.HistoricalSchedule(
                        schedule_id="s1",
                        task_id="t1",
                        start_time=1719219600000,
                        end_time=1719221400000,
                        allocation_type="MICRO_GAP",
                        status="COMMITTED"
                    )
                ],
                biometric_logs=[
                    genome_pb2.BiometricLog(
                        log_date="2024-06-24",
                        sleep_duration_hours=8.2,
                        resting_heart_rate=65,
                        step_count=8200,
                        computed_energy_score=8
                    )
                ]
            )

            response = stub.GenerateGenome(request)

            assert response.deadline_risk_score == 15
            assert "Tuesday 9-11 AM" in response.peak_hours
            assert len(response.insights) == 1
            assert response.insights[0].category == "ENERGY"
            assert response.insights[0].title == "Sleep Boosts Output"
            assert "focusBlockTimeOfDay" in response.scheduling_preferences_json
            mock_client.models.generate_content.assert_called_once()

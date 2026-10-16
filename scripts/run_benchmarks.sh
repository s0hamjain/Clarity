#!/usr/bin/env bash
set -eo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export PATH="/opt/homebrew/bin:/usr/local/bin:$PATH"

echo "================================================================================"
echo " CLARITY SYSTEM & SRE BENCHMARK SUITE RUNNER"
echo "================================================================================"
echo "Host: $(uname -s) $(uname -m) | Cores: $(sysctl -n hw.ncpu 2>/dev/null || nproc)"
echo "Go: $(go version 2>/dev/null || echo 'go not in PATH')"
echo "Python: $("$REPO_ROOT/agent/.venv/bin/python" --version 2>/dev/null || echo 'python not in venv')"
echo "Date: $(date -u)"
echo "================================================================================"

echo ""
echo ">>> [1/4] Running Go Pipeline Benchmarks (Latency, Queue Saturation, Precheck Overhead)..."
cd "$REPO_ROOT/server"
go test -v -run='TestPipelineThroughputAndLatency|TestPipelineStageBreakdown|TestQueueSaturationBoundary|TestPrecheckOverheadBenchmark' ./internal/jobs

echo ""
echo ">>> [2/4] Running Go Resume XYZ Metric Verification Tests..."
go test -v -run='TestXYZ' ./internal/jobs

echo ""
echo ">>> [3/4] Running Go HTTP Polling & Security Boundary Benchmarks..."
go test -v -run='TestConcurrentPollingLoad|TestConnectionStabilityUnderDropout|TestWorkDirBoundaryIsolationFuzzing' ./internal/api

echo ""
echo ">>> [4/4] Running Python Resume Metrics & SRE Benchmarks..."
cd "$REPO_ROOT/agent"
.venv/bin/pytest -v tests/test_resume_metrics.py -s
.venv/bin/python experiments/sre_benchmark_suite.py

echo ""
echo "================================================================================"
echo " ALL BENCHMARK & RELIABILITY SUITES EXECUTED SUCCESSFULLY"
echo "================================================================================"

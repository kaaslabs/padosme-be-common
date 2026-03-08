#!/bin/bash

# Color codes for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "========================================="
echo "Padosme Observability Stack Verification"
echo "========================================="
echo ""

# Function to check if a service is healthy
check_service() {
    local service_name=$1
    local url=$2
    local description=$3

    echo -n "Checking $description... "

    if curl -f -s -o /dev/null "$url"; then
        echo -e "${GREEN}✓ OK${NC}"
        return 0
    else
        echo -e "${RED}✗ FAILED${NC}"
        return 1
    fi
}

# Function to check if docker container is running
check_container() {
    local container_name=$1
    local description=$2

    echo -n "Checking $description... "

    if docker ps --format '{{.Names}}' | grep -q "^${container_name}$"; then
        echo -e "${GREEN}✓ Running${NC}"
        return 0
    else
        echo -e "${RED}✗ Not Running${NC}"
        return 1
    fi
}

echo "1. Checking Docker Containers"
echo "------------------------------"
check_container "padosme-otel-collector" "OpenTelemetry Collector"
check_container "padosme-prometheus" "Prometheus"
check_container "padosme-grafana" "Grafana"
check_container "padosme-jaeger" "Jaeger"
echo ""

echo "2. Checking Service Endpoints"
echo "------------------------------"
check_service "Prometheus" "http://localhost:9090/-/healthy" "Prometheus Health"
check_service "Grafana" "http://localhost:3000/api/health" "Grafana Health"
check_service "Jaeger" "http://localhost:16686/" "Jaeger UI"
check_service "OTLP gRPC" "http://localhost:4317" "OTLP gRPC Endpoint" || true
echo ""

echo "3. Checking Prometheus Targets"
echo "--------------------------------"
echo "Visit http://localhost:9090/targets to see scrape targets"
echo ""

echo "4. Checking Metrics Export"
echo "---------------------------"
echo -n "Checking if OTLP collector is exporting metrics... "
if curl -s http://localhost:8889/metrics | grep -q "padosme"; then
    echo -e "${GREEN}✓ Metrics found${NC}"
else
    echo -e "${YELLOW}⚠ No metrics yet (this is normal if services aren't instrumented yet)${NC}"
fi
echo ""

echo "5. Access URLs"
echo "--------------"
echo "Grafana:    http://localhost:3000 (admin/kaaslabs123)"
echo "Prometheus: http://localhost:9090"
echo "Jaeger:     http://localhost:16686"
echo "OTLP gRPC:  localhost:4317"
echo "OTLP HTTP:  localhost:4318"
echo ""

echo "========================================="
echo "Verification Complete!"
echo "========================================="
echo ""
echo "Next steps:"
echo "1. Log in to Grafana at http://localhost:3000"
echo "2. Check the 'Padosme Services Overview' dashboard"
echo "3. Instrument your services to send metrics to localhost:4317"
echo "4. See OBSERVABILITY.md for detailed instrumentation guide"

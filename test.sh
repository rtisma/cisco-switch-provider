#!/bin/bash

set -e

echo "╔══════════════════════════════════════════════════════════════════════════════╗"
echo "║                    Cisco Switch Provider Test Suite                         ║"
echo "╚══════════════════════════════════════════════════════════════════════════════╝"
echo ""

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Test results
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0

run_test() {
    local test_name=$1
    local test_cmd=$2

    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    echo -n "Running: $test_name ... "

    if eval "$test_cmd" > /tmp/test_output_$$.log 2>&1; then
        echo -e "${GREEN}PASS${NC}"
        PASSED_TESTS=$((PASSED_TESTS + 1))
    else
        echo -e "${RED}FAIL${NC}"
        FAILED_TESTS=$((FAILED_TESTS + 1))
        echo "  Error details:"
        cat /tmp/test_output_$$.log | sed 's/^/    /'
    fi
    rm -f /tmp/test_output_$$.log
}

echo "═══════════════════════════════════════════════════════════════════════════════"
echo "1. Build Tests"
echo "═══════════════════════════════════════════════════════════════════════════════"
echo ""

run_test "Go modules tidy" "go mod tidy"
run_test "Go modules verify" "go mod verify"
run_test "Build provider binary" "go build -o terraform-provider-cisco"

echo ""
echo "═══════════════════════════════════════════════════════════════════════════════"
echo "2. Unit Tests"
echo "═══════════════════════════════════════════════════════════════════════════════"
echo ""

run_test "Parser tests" "go test -v ./internal/provider/client -run TestParse"
run_test "Error detection tests" "go test -v ./internal/provider/client -run TestIsErrorOutput"
run_test "VLAN resource tests" "go test -v ./internal/provider/resources -run TestVlan"

echo ""
echo "═══════════════════════════════════════════════════════════════════════════════"
echo "3. Integration Tests (with Mock Switch)"
echo "═══════════════════════════════════════════════════════════════════════════════"
echo ""

run_test "Client connection tests" "go test -v ./internal/provider/client -run TestClientConnect"
run_test "VLAN lifecycle tests" "go test -v ./internal/provider/client -run TestVLANLifecycle"
run_test "Interface lifecycle tests" "go test -v ./internal/provider/client -run TestInterfaceLifecycle"
run_test "SVI lifecycle tests" "go test -v ./internal/provider/client -run TestSVILifecycle"
run_test "Concurrent command tests" "go test -v ./internal/provider/client -run TestConcurrentCommands"

echo ""
echo "═══════════════════════════════════════════════════════════════════════════════"
echo "4. Safety Tests"
echo "═══════════════════════════════════════════════════════════════════════════════"
echo ""

run_test "Invalid command handling" "go test -v ./internal/provider/client -run TestInvalidCommands"

echo ""
echo "═══════════════════════════════════════════════════════════════════════════════"
echo "5. Code Quality"
echo "═══════════════════════════════════════════════════════════════════════════════"
echo ""

run_test "Go fmt check" "test -z \$(gofmt -l .)"
run_test "Go vet" "go vet ./..."

echo ""
echo "═══════════════════════════════════════════════════════════════════════════════"
echo "Test Summary"
echo "═══════════════════════════════════════════════════════════════════════════════"
echo ""
echo "Total Tests:  $TOTAL_TESTS"
echo -e "Passed:       ${GREEN}$PASSED_TESTS${NC}"
echo -e "Failed:       ${RED}$FAILED_TESTS${NC}"
echo ""

if [ $FAILED_TESTS -eq 0 ]; then
    echo -e "${GREEN}✓ All tests passed!${NC}"
    echo ""
    echo "The provider is safe to use. You can now:"
    echo "  1. Install it:  make install"
    echo "  2. Test with real hardware (carefully!)"
    echo "  3. Start with read-only operations (show commands)"
    echo ""
    exit 0
else
    echo -e "${RED}✗ Some tests failed!${NC}"
    echo ""
    echo "DO NOT use this provider on production hardware until all tests pass!"
    echo ""
    exit 1
fi

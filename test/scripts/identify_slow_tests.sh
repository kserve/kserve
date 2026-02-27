#!/bin/bash

echo "🔍 Running tests with detailed output to identify specific slow tests..."

# Run with full verbose output to map timings to test names
go test -v ./pkg/controller/v1beta1/inferenceservice -run TestV1beta1APIs \
  -ginkgo.v 2>&1 | tee detailed_test_output.log

echo -e "\n📊 Analyzing output for slow tests..."

# Extract test names and their approximate timing
echo "Looking for tests that took >30 seconds..."
grep -A1 -B1 "70\..*seconds\|50\..*seconds\|40\..*seconds\|30\..*seconds\|25\..*seconds" detailed_test_output.log

echo -e "\nFull log saved to: detailed_test_output.log"
echo "Search the log file for the specific test descriptions near the timing markers."
#!/usr/bin/env bash
# E2E tool calling tests for KServe LLMInferenceService
# Tests Chat Completions and Anthropic Messages API tool-calling support
# for any tool-calling-capable model deployed via LLMInferenceService.
#
# Usage:
#   MODEL=Qwen/Qwen3-Coder-30B-A3B-Instruct-FP8 LLMISVC_NAME=qwen3-coder-tool-calling ./run-tests.sh
#   ./run-tests.sh http://localhost:8080/my-namespace/my-llmisvc
#
# Environment variables:
#   MODEL          - Model name (required unless endpoint URL is the only concern)
#   LLMISVC_NAME   - Name of the LLMInferenceService resource (required when not passing a URL)
#   NAMESPACE      - Kubernetes namespace (default: tool-calling-test)
#   VERBOSE        - Set to "true" for full response output
set -euo pipefail

MODEL="${MODEL:-}"
NAMESPACE="${NAMESPACE:-tool-calling-test}"
LLMISVC_NAME="${LLMISVC_NAME:-}"

if [ -n "${1:-}" ]; then
    BASE_URL="$1"
else
    if [ -z "$LLMISVC_NAME" ]; then
        echo "ERROR: LLMISVC_NAME must be set, or pass the base URL as an argument."
        echo "  Example: LLMISVC_NAME=qwen3-coder-tool-calling MODEL=Qwen/Qwen3-Coder-30B-A3B-Instruct-FP8 $0"
        echo "  Example: $0 http://localhost:8080/my-namespace/my-llmisvc"
        exit 1
    fi
    GATEWAY_IP=$(kubectl get gateway kserve-ingress-gateway -n kserve -o jsonpath='{.status.addresses[0].value}' 2>/dev/null || true)
    if [ -z "$GATEWAY_IP" ]; then
        echo "ERROR: Could not determine Gateway IP. Pass the base URL as an argument."
        exit 1
    fi
    BASE_URL="http://${GATEWAY_IP}/${NAMESPACE}/${LLMISVC_NAME}"
fi

if [ -z "$MODEL" ]; then
    echo "ERROR: MODEL must be set."
    echo "  Example: MODEL=Qwen/Qwen3-Coder-30B-A3B-Instruct-FP8 $0"
    exit 1
fi

echo "Using endpoint: ${BASE_URL}"
echo "Model: ${MODEL}"
VERBOSE="${VERBOSE:-false}"
PASS=0
FAIL=0
RESULTS=()

run_test() {
    local name="$1"
    local cmd="$2"
    local validate="$3"

    echo ""
    echo "============================================"
    echo "TEST: ${name}"
    echo "============================================"

    local response
    response=$(eval "$cmd" 2>&1) || true

    if echo "$response" | eval "$validate" > /dev/null 2>&1; then
        echo "PASS"
        PASS=$((PASS + 1))
        RESULTS+=("PASS: ${name}")
    else
        echo "FAIL"
        echo "Response:"
        echo "$response" | head -50
        FAIL=$((FAIL + 1))
        RESULTS+=("FAIL: ${name}")
    fi

    if [ "$VERBOSE" = "true" ]; then
        echo ""
        echo "--- Full Response ---"
        echo "$response" | python3 -m json.tool 2>/dev/null || echo "$response"
        echo "--- End Response ---"
    fi
}

# ============================================
# Step 1.2: Chat Completions Tool Calling (non-streaming)
# ============================================
run_test "Chat Completions - tool calling (non-streaming)" \
    "curl -s -X POST ${BASE_URL}/v1/chat/completions \
        -H 'Content-Type: application/json' \
        -d '{
            \"model\": \"${MODEL}\",
            \"temperature\": 0,
            \"messages\": [
                {\"role\": \"system\", \"content\": \"You are a helpful assistant with access to tools. Use the get_weather tool when asked about weather.\"},
                {\"role\": \"user\", \"content\": \"What is the weather in San Francisco?\"}
            ],
            \"tools\": [{
                \"type\": \"function\",
                \"function\": {
                    \"name\": \"get_weather\",
                    \"description\": \"Get the current weather in a given location\",
                    \"parameters\": {
                        \"type\": \"object\",
                        \"properties\": {
                            \"location\": {\"type\": \"string\", \"description\": \"City and state\"},
                            \"unit\": {\"type\": \"string\", \"enum\": [\"celsius\", \"fahrenheit\"]}
                        },
                        \"required\": [\"location\"]
                    }
                }
            }],
            \"tool_choice\": \"auto\",
            \"parallel_tool_calls\": true
        }'" \
    "jq -e '.choices[0].message.tool_calls[0].function.name == \"get_weather\"'"

# ============================================
# Step 1.3: Chat Completions Tool Calling (streaming)
# ============================================
run_test "Chat Completions - tool calling (streaming)" \
    "curl -s -X POST ${BASE_URL}/v1/chat/completions \
        -H 'Content-Type: application/json' \
        -d '{
            \"model\": \"${MODEL}\",
            \"temperature\": 0,
            \"stream\": true,
            \"messages\": [
                {\"role\": \"system\", \"content\": \"You are a helpful assistant with access to tools. Use the get_weather tool when asked about weather.\"},
                {\"role\": \"user\", \"content\": \"What is the weather in San Francisco?\"}
            ],
            \"tools\": [{
                \"type\": \"function\",
                \"function\": {
                    \"name\": \"get_weather\",
                    \"description\": \"Get the current weather in a given location\",
                    \"parameters\": {
                        \"type\": \"object\",
                        \"properties\": {
                            \"location\": {\"type\": \"string\", \"description\": \"City and state\"},
                            \"unit\": {\"type\": \"string\", \"enum\": [\"celsius\", \"fahrenheit\"]}
                        },
                        \"required\": [\"location\"]
                    }
                }
            }],
            \"tool_choice\": \"auto\"
        }'" \
    "grep '^data: ' | grep -v '\\[DONE\\]' | sed 's/^data: //' | jq -s 'map(select(.choices[0].delta.tool_calls != null)) | length > 0 and (map(.choices[0].delta.tool_calls[0].function.arguments // empty) | join(\"\") | fromjson | .location != null)'"

# ============================================
# Step 1.4: Anthropic Messages API Tool Calling (non-streaming)
# ============================================
run_test "Messages API - tool calling (non-streaming)" \
    "curl -s -X POST ${BASE_URL}/v1/messages \
        -H 'Content-Type: application/json' \
        -H 'x-api-key: dummy' \
        -H 'anthropic-version: 2023-06-01' \
        -d '{
            \"model\": \"${MODEL}\",
            \"temperature\": 0,
            \"max_tokens\": 1024,
            \"system\": \"You are a helpful assistant with access to tools. Use the get_weather tool when asked about weather.\",
            \"messages\": [
                {\"role\": \"user\", \"content\": \"What is the weather in San Francisco?\"}
            ],
            \"tools\": [{
                \"name\": \"get_weather\",
                \"description\": \"Get the current weather in a given location\",
                \"input_schema\": {
                    \"type\": \"object\",
                    \"properties\": {
                        \"location\": {\"type\": \"string\", \"description\": \"City and state\"},
                        \"unit\": {\"type\": \"string\", \"enum\": [\"celsius\", \"fahrenheit\"]}
                    },
                    \"required\": [\"location\"]
                }
            }],
            \"tool_choice\": {\"type\": \"auto\"}
        }'" \
    "jq -e '.content[] | select(.type == \"tool_use\") | .name == \"get_weather\"'"

# ============================================
# Step 1.5: Anthropic Messages API Tool Calling (streaming)
# ============================================
run_test "Messages API - tool calling (streaming)" \
    "curl -s -X POST ${BASE_URL}/v1/messages \
        -H 'Content-Type: application/json' \
        -H 'x-api-key: dummy' \
        -H 'anthropic-version: 2023-06-01' \
        -d '{
            \"model\": \"${MODEL}\",
            \"temperature\": 0,
            \"max_tokens\": 1024,
            \"stream\": true,
            \"system\": \"You are a helpful assistant with access to tools. Use the get_weather tool when asked about weather.\",
            \"messages\": [
                {\"role\": \"user\", \"content\": \"What is the weather in San Francisco?\"}
            ],
            \"tools\": [{
                \"name\": \"get_weather\",
                \"description\": \"Get the current weather in a given location\",
                \"input_schema\": {
                    \"type\": \"object\",
                    \"properties\": {
                        \"location\": {\"type\": \"string\", \"description\": \"City and state\"},
                        \"unit\": {\"type\": \"string\", \"enum\": [\"celsius\", \"fahrenheit\"]}
                    },
                    \"required\": [\"location\"]
                }
            }],
            \"tool_choice\": {\"type\": \"auto\"}
        }'" \
    "grep '^data: ' | grep -v '\\[DONE\\]' | sed 's/^data: //' | jq -s 'map(select(.type == \"content_block_start\" and .content_block.type == \"tool_use\")) | length > 0'"

# ============================================
# Step 1.6a: Chat Completions Multi-turn Tool Use
# ============================================
run_test "Chat Completions - multi-turn tool use" \
    "curl -s -X POST ${BASE_URL}/v1/chat/completions \
        -H 'Content-Type: application/json' \
        -d '{
            \"model\": \"${MODEL}\",
            \"temperature\": 0,
            \"messages\": [
                {\"role\": \"system\", \"content\": \"You are a helpful assistant with access to tools.\"},
                {\"role\": \"user\", \"content\": \"What is the weather in San Francisco?\"},
                {\"role\": \"assistant\", \"content\": null, \"tool_calls\": [
                    {\"id\": \"call_123\", \"type\": \"function\", \"function\": {\"name\": \"get_weather\", \"arguments\": \"{\\\"location\\\": \\\"San Francisco, CA\\\"}\"}}
                ]},
                {\"role\": \"tool\", \"tool_call_id\": \"call_123\", \"content\": \"{\\\"temperature\\\": 62, \\\"unit\\\": \\\"fahrenheit\\\", \\\"condition\\\": \\\"foggy\\\"}\"}
            ],
            \"tools\": [{
                \"type\": \"function\",
                \"function\": {
                    \"name\": \"get_weather\",
                    \"description\": \"Get the current weather in a given location\",
                    \"parameters\": {
                        \"type\": \"object\",
                        \"properties\": {
                            \"location\": {\"type\": \"string\"},
                            \"unit\": {\"type\": \"string\", \"enum\": [\"celsius\", \"fahrenheit\"]}
                        },
                        \"required\": [\"location\"]
                    }
                }
            }]
        }'" \
    "jq -e '.choices[0].message.content != null and (.choices[0].message.tool_calls | length == 0 or .choices[0].message.tool_calls == null)'"

# ============================================
# Step 1.6b: Anthropic Messages Multi-turn Tool Use
# ============================================
run_test "Messages API - multi-turn tool use" \
    "curl -s -X POST ${BASE_URL}/v1/messages \
        -H 'Content-Type: application/json' \
        -H 'x-api-key: dummy' \
        -H 'anthropic-version: 2023-06-01' \
        -d '{
            \"model\": \"${MODEL}\",
            \"temperature\": 0,
            \"max_tokens\": 1024,
            \"messages\": [
                {\"role\": \"user\", \"content\": \"What is the weather in San Francisco?\"},
                {\"role\": \"assistant\", \"content\": [
                    {\"type\": \"tool_use\", \"id\": \"toolu_123\", \"name\": \"get_weather\", \"input\": {\"location\": \"San Francisco, CA\"}}
                ]},
                {\"role\": \"user\", \"content\": [
                    {\"type\": \"tool_result\", \"tool_use_id\": \"toolu_123\", \"content\": \"{\\\"temperature\\\": 62, \\\"unit\\\": \\\"fahrenheit\\\", \\\"condition\\\": \\\"foggy\\\"}\"}
                ]}
            ],
            \"tools\": [{
                \"name\": \"get_weather\",
                \"description\": \"Get the current weather in a given location\",
                \"input_schema\": {
                    \"type\": \"object\",
                    \"properties\": {
                        \"location\": {\"type\": \"string\"},
                        \"unit\": {\"type\": \"string\", \"enum\": [\"celsius\", \"fahrenheit\"]}
                    },
                    \"required\": [\"location\"]
                }
            }]
        }'" \
    "jq -e '(.content | map(select(.type == \"text\")) | length > 0) and .stop_reason == \"end_turn\"'"

# ============================================
# Step 1.6c: Chat Completions Multiple Tools
# ============================================
run_test "Chat Completions - multiple tools" \
    "curl -s -X POST ${BASE_URL}/v1/chat/completions \
        -H 'Content-Type: application/json' \
        -d '{
            \"model\": \"${MODEL}\",
            \"temperature\": 0,
            \"messages\": [
                {\"role\": \"system\", \"content\": \"You are a helpful assistant with access to tools. Use the appropriate tool for each part of the request. Call multiple tools in parallel when needed.\"},
                {\"role\": \"user\", \"content\": \"What is the weather in San Francisco and what time is it in Tokyo?\"}
            ],
            \"tools\": [
                {
                    \"type\": \"function\",
                    \"function\": {
                        \"name\": \"get_weather\",
                        \"description\": \"Get the current weather in a given location\",
                        \"parameters\": {
                            \"type\": \"object\",
                            \"properties\": {
                                \"location\": {\"type\": \"string\", \"description\": \"City and state or city and country\"}
                            },
                            \"required\": [\"location\"]
                        }
                    }
                },
                {
                    \"type\": \"function\",
                    \"function\": {
                        \"name\": \"get_time\",
                        \"description\": \"Get the current time in a given timezone or city\",
                        \"parameters\": {
                            \"type\": \"object\",
                            \"properties\": {
                                \"location\": {\"type\": \"string\", \"description\": \"City or timezone name\"}
                            },
                            \"required\": [\"location\"]
                        }
                    }
                }
            ],
            \"tool_choice\": \"auto\",
            \"parallel_tool_calls\": true
        }'" \
    "jq -e '.choices[0].message.tool_calls | length >= 1 and all(.[]; .function.name == \"get_weather\" or .function.name == \"get_time\")'"

# ============================================
# Step 1.6d: Messages API Multiple Tools
# ============================================
run_test "Messages API - multiple tools" \
    "curl -s -X POST ${BASE_URL}/v1/messages \
        -H 'Content-Type: application/json' \
        -H 'x-api-key: dummy' \
        -H 'anthropic-version: 2023-06-01' \
        -d '{
            \"model\": \"${MODEL}\",
            \"temperature\": 0,
            \"max_tokens\": 1024,
            \"system\": \"You are a helpful assistant with access to tools. Use the appropriate tool for each part of the request. Call multiple tools when needed.\",
            \"messages\": [
                {\"role\": \"user\", \"content\": \"What is the weather in San Francisco and what time is it in Tokyo?\"}
            ],
            \"tools\": [
                {
                    \"name\": \"get_weather\",
                    \"description\": \"Get the current weather in a given location\",
                    \"input_schema\": {
                        \"type\": \"object\",
                        \"properties\": {
                            \"location\": {\"type\": \"string\", \"description\": \"City and state or city and country\"}
                        },
                        \"required\": [\"location\"]
                    }
                },
                {
                    \"name\": \"get_time\",
                    \"description\": \"Get the current time in a given timezone or city\",
                    \"input_schema\": {
                        \"type\": \"object\",
                        \"properties\": {
                            \"location\": {\"type\": \"string\", \"description\": \"City or timezone name\"}
                        },
                        \"required\": [\"location\"]
                    }
                }
            ],
            \"tool_choice\": {\"type\": \"auto\"}
        }'" \
    "jq -e '[.content[] | select(.type == \"tool_use\")] | length >= 1 and all(.[]; .name == \"get_weather\" or .name == \"get_time\")'"

# ============================================
# Step 1.6e: Chat Completions - 5 tools, use all
# ============================================
run_test "Chat Completions - 5 tools, use all" \
    "curl -s -X POST ${BASE_URL}/v1/chat/completions \
        -H 'Content-Type: application/json' \
        -d '{
            \"model\": \"${MODEL}\",
            \"temperature\": 0,
            \"messages\": [
                {\"role\": \"system\", \"content\": \"You are a travel planning assistant. You MUST call ALL relevant tools in parallel to answer the user. For this request, call get_weather for the destination, get_time for the destination, search_flights, get_exchange_rate, and translate_phrase.\"},
                {\"role\": \"user\", \"content\": \"I want to travel from New York to Tokyo. What is the weather and time there, find me flights, what is the USD to JPY rate, and how do I say hello in Japanese?\"}
            ],
            \"tools\": [
                {\"type\": \"function\", \"function\": {\"name\": \"get_weather\", \"description\": \"Get weather for a location\", \"parameters\": {\"type\": \"object\", \"properties\": {\"location\": {\"type\": \"string\"}}, \"required\": [\"location\"]}}},
                {\"type\": \"function\", \"function\": {\"name\": \"get_time\", \"description\": \"Get current time in a city\", \"parameters\": {\"type\": \"object\", \"properties\": {\"city\": {\"type\": \"string\"}}, \"required\": [\"city\"]}}},
                {\"type\": \"function\", \"function\": {\"name\": \"search_flights\", \"description\": \"Search for flights between cities\", \"parameters\": {\"type\": \"object\", \"properties\": {\"origin\": {\"type\": \"string\"}, \"destination\": {\"type\": \"string\"}, \"date\": {\"type\": \"string\", \"description\": \"YYYY-MM-DD\"}}, \"required\": [\"origin\", \"destination\"]}}},
                {\"type\": \"function\", \"function\": {\"name\": \"get_exchange_rate\", \"description\": \"Get exchange rate between currencies\", \"parameters\": {\"type\": \"object\", \"properties\": {\"from_currency\": {\"type\": \"string\"}, \"to_currency\": {\"type\": \"string\"}}, \"required\": [\"from_currency\", \"to_currency\"]}}},
                {\"type\": \"function\", \"function\": {\"name\": \"translate_phrase\", \"description\": \"Translate a phrase to another language\", \"parameters\": {\"type\": \"object\", \"properties\": {\"phrase\": {\"type\": \"string\"}, \"target_language\": {\"type\": \"string\"}}, \"required\": [\"phrase\", \"target_language\"]}}}
            ],
            \"tool_choice\": \"auto\",
            \"parallel_tool_calls\": true
        }'" \
    "jq -e '.choices[0].message.tool_calls | length >= 5 and ([.[].function.name] | sort == [\"get_exchange_rate\", \"get_time\", \"get_weather\", \"search_flights\", \"translate_phrase\"])'"

# ============================================
# Step 1.6f: Messages API - 5 tools, use all
# ============================================
run_test "Messages API - 5 tools, use all" \
    "curl -s -X POST ${BASE_URL}/v1/messages \
        -H 'Content-Type: application/json' \
        -H 'x-api-key: dummy' \
        -H 'anthropic-version: 2023-06-01' \
        -d '{
            \"model\": \"${MODEL}\",
            \"temperature\": 0,
            \"max_tokens\": 2048,
            \"system\": \"You are a travel planning assistant. You MUST call ALL relevant tools to answer the user. For this request, call get_weather for the destination, get_time for the destination, search_flights, get_exchange_rate, and translate_phrase.\",
            \"messages\": [
                {\"role\": \"user\", \"content\": \"I want to travel from New York to Tokyo. What is the weather and time there, find me flights, what is the USD to JPY rate, and how do I say hello in Japanese?\"}
            ],
            \"tools\": [
                {\"name\": \"get_weather\", \"description\": \"Get weather for a location\", \"input_schema\": {\"type\": \"object\", \"properties\": {\"location\": {\"type\": \"string\"}}, \"required\": [\"location\"]}},
                {\"name\": \"get_time\", \"description\": \"Get current time in a city\", \"input_schema\": {\"type\": \"object\", \"properties\": {\"city\": {\"type\": \"string\"}}, \"required\": [\"city\"]}},
                {\"name\": \"search_flights\", \"description\": \"Search for flights between cities\", \"input_schema\": {\"type\": \"object\", \"properties\": {\"origin\": {\"type\": \"string\"}, \"destination\": {\"type\": \"string\"}, \"date\": {\"type\": \"string\", \"description\": \"YYYY-MM-DD\"}}, \"required\": [\"origin\", \"destination\"]}},
                {\"name\": \"get_exchange_rate\", \"description\": \"Get exchange rate between currencies\", \"input_schema\": {\"type\": \"object\", \"properties\": {\"from_currency\": {\"type\": \"string\"}, \"to_currency\": {\"type\": \"string\"}}, \"required\": [\"from_currency\", \"to_currency\"]}},
                {\"name\": \"translate_phrase\", \"description\": \"Translate a phrase to another language\", \"input_schema\": {\"type\": \"object\", \"properties\": {\"phrase\": {\"type\": \"string\"}, \"target_language\": {\"type\": \"string\"}}, \"required\": [\"phrase\", \"target_language\"]}}
            ],
            \"tool_choice\": {\"type\": \"auto\"}
        }'" \
    "jq -e '[.content[] | select(.type == \"tool_use\")] | length >= 5 and ([.[].name] | sort == [\"get_exchange_rate\", \"get_time\", \"get_weather\", \"search_flights\", \"translate_phrase\"])'"

# ============================================
# Step 1.6g: Chat Completions - tool_choice forces specific function
# ============================================
run_test "Chat Completions - tool_choice specific function" \
    "curl -s -X POST ${BASE_URL}/v1/chat/completions \
        -H 'Content-Type: application/json' \
        -d '{
            \"model\": \"${MODEL}\",
            \"temperature\": 0,
            \"messages\": [
                {\"role\": \"user\", \"content\": \"Tell me a joke\"}
            ],
            \"tools\": [
                {\"type\": \"function\", \"function\": {\"name\": \"get_weather\", \"description\": \"Get weather for a location\", \"parameters\": {\"type\": \"object\", \"properties\": {\"location\": {\"type\": \"string\"}}, \"required\": [\"location\"]}}},
                {\"type\": \"function\", \"function\": {\"name\": \"get_time\", \"description\": \"Get current time\", \"parameters\": {\"type\": \"object\", \"properties\": {\"city\": {\"type\": \"string\"}}, \"required\": [\"city\"]}}}
            ],
            \"tool_choice\": {\"type\": \"function\", \"function\": {\"name\": \"get_weather\"}}
        }'" \
    "jq -e '.choices[0].message.tool_calls[0].function.name == \"get_weather\"'"

# ============================================
# Step 1.6h: Messages API - tool_choice forces specific tool
# ============================================
run_test "Messages API - tool_choice specific tool" \
    "curl -s -X POST ${BASE_URL}/v1/messages \
        -H 'Content-Type: application/json' \
        -H 'x-api-key: dummy' \
        -H 'anthropic-version: 2023-06-01' \
        -d '{
            \"model\": \"${MODEL}\",
            \"temperature\": 0,
            \"max_tokens\": 1024,
            \"messages\": [
                {\"role\": \"user\", \"content\": \"Tell me a joke\"}
            ],
            \"tools\": [
                {\"name\": \"get_weather\", \"description\": \"Get weather for a location\", \"input_schema\": {\"type\": \"object\", \"properties\": {\"location\": {\"type\": \"string\"}}, \"required\": [\"location\"]}},
                {\"name\": \"get_time\", \"description\": \"Get current time\", \"input_schema\": {\"type\": \"object\", \"properties\": {\"city\": {\"type\": \"string\"}}, \"required\": [\"city\"]}}
            ],
            \"tool_choice\": {\"type\": \"tool\", \"name\": \"get_weather\"}
        }'" \
    "jq -e '[.content[] | select(.type == \"tool_use\")] | length >= 1 and .[0].name == \"get_weather\"'"

# ============================================
# Step 1.6i: Chat Completions - complex nested parameters
# ============================================
run_test "Chat Completions - complex nested parameters" \
    "curl -s -X POST ${BASE_URL}/v1/chat/completions \
        -H 'Content-Type: application/json' \
        -d '{
            \"model\": \"${MODEL}\",
            \"temperature\": 0,
            \"messages\": [
                {\"role\": \"user\", \"content\": \"Create a meeting titled Team Standup for 2025-07-15T09:00:00 with alice@example.com and bob@example.com, set it as high priority with a 30 minute reminder.\"}
            ],
            \"tools\": [{
                \"type\": \"function\",
                \"function\": {
                    \"name\": \"create_calendar_event\",
                    \"description\": \"Create a calendar event with attendees and settings\",
                    \"parameters\": {
                        \"type\": \"object\",
                        \"properties\": {
                            \"title\": {\"type\": \"string\"},
                            \"datetime\": {\"type\": \"string\", \"description\": \"ISO 8601 datetime\"},
                            \"attendees\": {
                                \"type\": \"array\",
                                \"items\": {
                                    \"type\": \"object\",
                                    \"properties\": {
                                        \"email\": {\"type\": \"string\"},
                                        \"role\": {\"type\": \"string\", \"enum\": [\"required\", \"optional\"]}
                                    },
                                    \"required\": [\"email\"]
                                }
                            },
                            \"settings\": {
                                \"type\": \"object\",
                                \"properties\": {
                                    \"priority\": {\"type\": \"string\", \"enum\": [\"low\", \"medium\", \"high\"]},
                                    \"reminders\": {
                                        \"type\": \"array\",
                                        \"items\": {\"type\": \"integer\", \"description\": \"Minutes before event\"}
                                    }
                                }
                            }
                        },
                        \"required\": [\"title\", \"datetime\", \"attendees\"]
                    }
                }
            }],
            \"tool_choice\": \"auto\"
        }'" \
    "jq -e '.choices[0].message.tool_calls[0] | .function.name == \"create_calendar_event\" and (.function.arguments | fromjson | .attendees | length >= 2)'"

# ============================================
# Step 1.6j: Chat Completions - tool with no parameters
# ============================================
run_test "Chat Completions - tool with no parameters" \
    "curl -s -X POST ${BASE_URL}/v1/chat/completions \
        -H 'Content-Type: application/json' \
        -d '{
            \"model\": \"${MODEL}\",
            \"temperature\": 0,
            \"messages\": [
                {\"role\": \"user\", \"content\": \"What is the current server status?\"}
            ],
            \"tools\": [{
                \"type\": \"function\",
                \"function\": {
                    \"name\": \"get_server_status\",
                    \"description\": \"Returns the current server health status. Takes no arguments.\",
                    \"parameters\": {\"type\": \"object\", \"properties\": {}}
                }
            }],
            \"tool_choice\": \"auto\"
        }'" \
    "jq -e '.choices[0].message.tool_calls[0].function.name == \"get_server_status\"'"

# ============================================
# Step 1.6k: Chat Completions - multiple calls to same tool
# ============================================
run_test "Chat Completions - multiple calls same tool, different args" \
    "curl -s -X POST ${BASE_URL}/v1/chat/completions \
        -H 'Content-Type: application/json' \
        -d '{
            \"model\": \"${MODEL}\",
            \"temperature\": 0,
            \"messages\": [
                {\"role\": \"system\", \"content\": \"You are a helpful assistant. When asked about weather in multiple cities, make a separate get_weather call for EACH city.\"},
                {\"role\": \"user\", \"content\": \"What is the weather in San Francisco, New York, and London?\"}
            ],
            \"tools\": [{
                \"type\": \"function\",
                \"function\": {
                    \"name\": \"get_weather\",
                    \"description\": \"Get weather for a location\",
                    \"parameters\": {\"type\": \"object\", \"properties\": {\"location\": {\"type\": \"string\"}}, \"required\": [\"location\"]}
                }
            }],
            \"tool_choice\": \"auto\",
            \"parallel_tool_calls\": true
        }'" \
    "jq -e '.choices[0].message.tool_calls | length >= 1 and all(.[]; .function.name == \"get_weather\")'"

# ============================================
# Step 1.6l: Chat Completions - verify all tools visible to model
# ============================================
run_test "Chat Completions - verify all tool definitions reach model" \
    "curl -s -X POST ${BASE_URL}/v1/chat/completions \
        -H 'Content-Type: application/json' \
        -d '{
            \"model\": \"${MODEL}\",
            \"temperature\": 0,
            \"messages\": [
                {\"role\": \"user\", \"content\": \"List the exact names of all tools available to you, one per line. Do not explain them.\"}
            ],
            \"tools\": [
                {\"type\": \"function\", \"function\": {\"name\": \"tool_alpha\", \"description\": \"First tool\", \"parameters\": {\"type\": \"object\", \"properties\": {}}}},
                {\"type\": \"function\", \"function\": {\"name\": \"tool_beta\", \"description\": \"Second tool\", \"parameters\": {\"type\": \"object\", \"properties\": {}}}},
                {\"type\": \"function\", \"function\": {\"name\": \"tool_gamma\", \"description\": \"Third tool\", \"parameters\": {\"type\": \"object\", \"properties\": {}}}},
                {\"type\": \"function\", \"function\": {\"name\": \"tool_delta\", \"description\": \"Fourth tool\", \"parameters\": {\"type\": \"object\", \"properties\": {}}}},
                {\"type\": \"function\", \"function\": {\"name\": \"tool_epsilon\", \"description\": \"Fifth tool\", \"parameters\": {\"type\": \"object\", \"properties\": {}}}}
            ],
            \"max_tokens\": 100
        }'" \
    "grep -o 'tool_alpha\|tool_beta\|tool_gamma\|tool_delta\|tool_epsilon' | sort -u | wc -l | xargs test 5 -eq"

# ============================================
# Step 1.6m: Messages API - verify all tools visible to model
# ============================================
run_test "Messages API - verify all tool definitions reach model" \
    "curl -s -X POST ${BASE_URL}/v1/messages \
        -H 'Content-Type: application/json' \
        -H 'x-api-key: dummy' \
        -H 'anthropic-version: 2023-06-01' \
        -d '{
            \"model\": \"${MODEL}\",
            \"temperature\": 0,
            \"max_tokens\": 100,
            \"messages\": [
                {\"role\": \"user\", \"content\": \"List the exact names of all tools available to you, one per line. Do not explain them.\"}
            ],
            \"tools\": [
                {\"name\": \"tool_alpha\", \"description\": \"First tool\", \"input_schema\": {\"type\": \"object\", \"properties\": {}}},
                {\"name\": \"tool_beta\", \"description\": \"Second tool\", \"input_schema\": {\"type\": \"object\", \"properties\": {}}},
                {\"name\": \"tool_gamma\", \"description\": \"Third tool\", \"input_schema\": {\"type\": \"object\", \"properties\": {}}},
                {\"name\": \"tool_delta\", \"description\": \"Fourth tool\", \"input_schema\": {\"type\": \"object\", \"properties\": {}}},
                {\"name\": \"tool_epsilon\", \"description\": \"Fifth tool\", \"input_schema\": {\"type\": \"object\", \"properties\": {}}}
            ]
        }'" \
    "grep -o 'tool_alpha\|tool_beta\|tool_gamma\|tool_delta\|tool_epsilon' | sort -u | wc -l | xargs test 5 -eq"

# ============================================
# Step 4.1a: Regression - Chat Completions without tools
# ============================================
run_test "Regression - Chat Completions without tools" \
    "curl -s -X POST ${BASE_URL}/v1/chat/completions \
        -H 'Content-Type: application/json' \
        -d '{
            \"model\": \"${MODEL}\",
            \"temperature\": 0,
            \"messages\": [
                {\"role\": \"user\", \"content\": \"Explain how a simple agent loop works in 3 sentences.\"}
            ]
        }'" \
    "jq -e '.choices[0].message.content != null'"

# ============================================
# Step 4.1b: Regression - Messages API without tools
# ============================================
run_test "Regression - Messages API without tools" \
    "curl -s -X POST ${BASE_URL}/v1/messages \
        -H 'Content-Type: application/json' \
        -H 'x-api-key: dummy' \
        -H 'anthropic-version: 2023-06-01' \
        -d '{
            \"model\": \"${MODEL}\",
            \"temperature\": 0,
            \"max_tokens\": 1024,
            \"messages\": [
                {\"role\": \"user\", \"content\": \"Explain how a simple agent loop works in 3 sentences.\"}
            ]
        }'" \
    "jq -e '.content | map(select(.type == \"text\")) | length > 0'"

# ============================================
# Stack passthrough verification
# ============================================
echo ""
echo "============================================"
echo "STACK PASSTHROUGH VERIFICATION"
echo "============================================"

if [ -z "$LLMISVC_NAME" ]; then
    echo "(Skipping stack passthrough -- LLMISVC_NAME not set)"
else
    WORKLOAD_PODS=$(kubectl get pods -n "${NAMESPACE}" -l app.kubernetes.io/name="${LLMISVC_NAME}" -o name 2>/dev/null || true)
    EPP_POD=$(kubectl get pods -n "${NAMESPACE}" -l app.kubernetes.io/name="${LLMISVC_NAME}",app.kubernetes.io/component=llminferenceservice-scheduler -o name 2>/dev/null | head -1)

    echo ""
    echo "--- Workload pod logs ---"
    if [ -n "$WORKLOAD_PODS" ]; then
        for POD in $WORKLOAD_PODS; do
            echo "  [$POD]"
            kubectl logs -n "${NAMESPACE}" "${POD}" --all-containers --tail=100 2>&1 \
                | grep -i "tool\|auto_tool\|chat/completions\|/v1/messages\|get_weather\|error\|fail" \
                | tail -20 || echo "  (no tool-related logs)"
        done
    else
        echo "  (no workload pods found)"
    fi

    echo ""
    echo "--- EPP logs ---"
    if [ -n "$EPP_POD" ]; then
        kubectl logs -n "${NAMESPACE}" "${EPP_POD}" -c main --tail=100 2>&1 \
            | grep -i "tool\|message\|chat/completions\|/v1/messages\|request" \
            | tail -20 || echo "  (no tool-related EPP logs)"
    else
        echo "  (no EPP pod found)"
    fi
fi

# ============================================
# Summary
# ============================================
echo ""
echo "============================================"
echo "SUMMARY"
echo "============================================"
for r in "${RESULTS[@]}"; do
    echo "  $r"
done
echo ""
echo "Total: $((PASS + FAIL))  Passed: ${PASS}  Failed: ${FAIL}"
echo "============================================"

if [ "$FAIL" -gt 0 ]; then
    exit 1
fi

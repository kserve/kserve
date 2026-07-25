# Tool Calling Test Requests

Test requests for validating tool calling passthrough on both Chat Completions and Anthropic Messages API surfaces through KServe LLMInferenceService.

## Setup

Set these environment variables to point at your deployed LLMInferenceService endpoint and the model it serves.

```bash
export BASE_URL="<your-llminferenceservice-endpoint>"   # e.g. http://localhost:8080
export MODEL="<your-model-name>"                         # e.g. the model identifier used at deploy time
```

---

## 1. Chat Completions -- Tool Calling (non-streaming)

Sends a request with a tool definition and expects the model to return a tool call.

```bash
curl -s -X POST ${BASE_URL}/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "'${MODEL}'",
    "messages": [
      {
        "role": "system",
        "content": "You are a helpful assistant with access to tools. Use the get_weather tool when asked about weather."
      },
      {
        "role": "user",
        "content": "What is the weather in San Francisco?"
      }
    ],
    "tools": [
      {
        "type": "function",
        "function": {
          "name": "get_weather",
          "description": "Get the current weather in a given location",
          "parameters": {
            "type": "object",
            "properties": {
              "location": {
                "type": "string",
                "description": "City and state"
              },
              "unit": {
                "type": "string",
                "enum": ["celsius", "fahrenheit"]
              }
            },
            "required": ["location"]
          }
        }
      }
    ],
    "tool_choice": "auto",
    "parallel_tool_calls": true
  }' | jq
```

**Expected:** `choices[].message.tool_calls` array with `function.name == "get_weather"` and `function.arguments` containing `"location"`.

---

## 2. Chat Completions -- Tool Calling (streaming)

Same request with `"stream": true`.

```bash
curl -s -X POST ${BASE_URL}/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "'${MODEL}'",
    "stream": true,
    "messages": [
      {
        "role": "system",
        "content": "You are a helpful assistant with access to tools. Use the get_weather tool when asked about weather."
      },
      {
        "role": "user",
        "content": "What is the weather in San Francisco?"
      }
    ],
    "tools": [
      {
        "type": "function",
        "function": {
          "name": "get_weather",
          "description": "Get the current weather in a given location",
          "parameters": {
            "type": "object",
            "properties": {
              "location": {
                "type": "string",
                "description": "City and state"
              },
              "unit": {
                "type": "string",
                "enum": ["celsius", "fahrenheit"]
              }
            },
            "required": ["location"]
          }
        }
      }
    ],
    "tool_choice": "auto"
  }'
```

**Expected:** SSE stream with `choices[].delta.tool_calls` chunks. Concatenated `function.arguments` fragments form valid JSON containing `"location"`.

---

## 3. Messages API -- Tool Calling (non-streaming)

Anthropic Messages format with `input_schema` instead of `parameters`, `tool_choice` as object.

```bash
curl -s -X POST ${BASE_URL}/v1/messages \
  -H 'Content-Type: application/json' \
  -H 'x-api-key: dummy' \
  -H 'anthropic-version: 2023-06-01' \
  -d '{
    "model": "'${MODEL}'",
    "max_tokens": 1024,
    "system": "You are a helpful assistant with access to tools. Use the get_weather tool when asked about weather.",
    "messages": [
      {
        "role": "user",
        "content": "What is the weather in San Francisco?"
      }
    ],
    "tools": [
      {
        "name": "get_weather",
        "description": "Get the current weather in a given location",
        "input_schema": {
          "type": "object",
          "properties": {
            "location": {
              "type": "string",
              "description": "City and state"
            },
            "unit": {
              "type": "string",
              "enum": ["celsius", "fahrenheit"]
            }
          },
          "required": ["location"]
        }
      }
    ],
    "tool_choice": {"type": "auto"}
  }' | jq
```

**Expected:** `content` array with a `tool_use` block containing `name: "get_weather"` and `input` with `"location"`. `stop_reason: "tool_use"`.

---

## 4. Messages API -- Tool Calling (streaming)

Same request with `"stream": true`.

```bash
curl -s -X POST ${BASE_URL}/v1/messages \
  -H 'Content-Type: application/json' \
  -H 'x-api-key: dummy' \
  -H 'anthropic-version: 2023-06-01' \
  -d '{
    "model": "'${MODEL}'",
    "max_tokens": 1024,
    "stream": true,
    "system": "You are a helpful assistant with access to tools. Use the get_weather tool when asked about weather.",
    "messages": [
      {
        "role": "user",
        "content": "What is the weather in San Francisco?"
      }
    ],
    "tools": [
      {
        "name": "get_weather",
        "description": "Get the current weather in a given location",
        "input_schema": {
          "type": "object",
          "properties": {
            "location": {
              "type": "string",
              "description": "City and state"
            },
            "unit": {
              "type": "string",
              "enum": ["celsius", "fahrenheit"]
            }
          },
          "required": ["location"]
        }
      }
    ],
    "tool_choice": {"type": "auto"}
  }'
```

**Expected:** SSE stream with `content_block_start` event containing `type: "tool_use"`. `content_block_delta` events with `partial_json` fragments that assemble into valid JSON with `"location"`.

---

## 5. Chat Completions -- Multi-turn Tool Use

Simulates a complete tool use loop: user asks, model calls tool, tool returns result, model responds with text.

```bash
curl -s -X POST ${BASE_URL}/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "'${MODEL}'",
    "messages": [
      {
        "role": "system",
        "content": "You are a helpful assistant with access to tools."
      },
      {
        "role": "user",
        "content": "What is the weather in San Francisco?"
      },
      {
        "role": "assistant",
        "content": null,
        "tool_calls": [
          {
            "id": "call_123",
            "type": "function",
            "function": {
              "name": "get_weather",
              "arguments": "{\"location\": \"San Francisco, CA\"}"
            }
          }
        ]
      },
      {
        "role": "tool",
        "tool_call_id": "call_123",
        "content": "{\"temperature\": 62, \"unit\": \"fahrenheit\", \"condition\": \"foggy\"}"
      }
    ],
    "tools": [
      {
        "type": "function",
        "function": {
          "name": "get_weather",
          "description": "Get the current weather in a given location",
          "parameters": {
            "type": "object",
            "properties": {
              "location": {"type": "string"},
              "unit": {"type": "string", "enum": ["celsius", "fahrenheit"]}
            },
            "required": ["location"]
          }
        }
      }
    ]
  }' | jq
```

**Expected:** `choices[].message.content` contains natural language referencing the weather data (62F, foggy). No additional tool calls.

---

## 6. Messages API -- Multi-turn Tool Use

Same flow in Anthropic format: `tool_use` in assistant message, `tool_result` in user message.

```bash
curl -s -X POST ${BASE_URL}/v1/messages \
  -H 'Content-Type: application/json' \
  -H 'x-api-key: dummy' \
  -H 'anthropic-version: 2023-06-01' \
  -d '{
    "model": "'${MODEL}'",
    "max_tokens": 1024,
    "messages": [
      {
        "role": "user",
        "content": "What is the weather in San Francisco?"
      },
      {
        "role": "assistant",
        "content": [
          {
            "type": "tool_use",
            "id": "toolu_123",
            "name": "get_weather",
            "input": {"location": "San Francisco, CA"}
          }
        ]
      },
      {
        "role": "user",
        "content": [
          {
            "type": "tool_result",
            "tool_use_id": "toolu_123",
            "content": "{\"temperature\": 62, \"unit\": \"fahrenheit\", \"condition\": \"foggy\"}"
          }
        ]
      }
    ],
    "tools": [
      {
        "name": "get_weather",
        "description": "Get the current weather in a given location",
        "input_schema": {
          "type": "object",
          "properties": {
            "location": {"type": "string"},
            "unit": {"type": "string", "enum": ["celsius", "fahrenheit"]}
          },
          "required": ["location"]
        }
      }
    ]
  }' | jq
```

**Expected:** `content` array with a `text` block referencing the weather data. `stop_reason: "end_turn"`.

---

## 7. Chat Completions -- Multiple Tools (2 definitions)

Provides two tool definitions (`get_weather` and `get_time`) and a prompt that requires both. Verifies the model can select from multiple available tools.

```bash
curl -s -X POST ${BASE_URL}/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "'${MODEL}'",
    "messages": [
      {
        "role": "system",
        "content": "You are a helpful assistant with access to tools. Use the appropriate tool for each part of the request. Call multiple tools in parallel when needed."
      },
      {
        "role": "user",
        "content": "What is the weather in San Francisco and what time is it in Tokyo?"
      }
    ],
    "tools": [
      {
        "type": "function",
        "function": {
          "name": "get_weather",
          "description": "Get the current weather in a given location",
          "parameters": {
            "type": "object",
            "properties": {
              "location": {
                "type": "string",
                "description": "City and state or city and country"
              }
            },
            "required": ["location"]
          }
        }
      },
      {
        "type": "function",
        "function": {
          "name": "get_time",
          "description": "Get the current time in a given timezone or city",
          "parameters": {
            "type": "object",
            "properties": {
              "location": {
                "type": "string",
                "description": "City or timezone name"
              }
            },
            "required": ["location"]
          }
        }
      }
    ],
    "tool_choice": "auto",
    "parallel_tool_calls": true
  }' | jq
```

**Expected:** `choices[].message.tool_calls` with at least one call. Each call's `function.name` is either `"get_weather"` or `"get_time"`.

---

## 8. Messages API -- Multiple Tools (2 definitions)

Same two-tool scenario in Anthropic Messages format.

```bash
curl -s -X POST ${BASE_URL}/v1/messages \
  -H 'Content-Type: application/json' \
  -H 'x-api-key: dummy' \
  -H 'anthropic-version: 2023-06-01' \
  -d '{
    "model": "'${MODEL}'",
    "max_tokens": 1024,
    "system": "You are a helpful assistant with access to tools. Use the appropriate tool for each part of the request. Call multiple tools when needed.",
    "messages": [
      {
        "role": "user",
        "content": "What is the weather in San Francisco and what time is it in Tokyo?"
      }
    ],
    "tools": [
      {
        "name": "get_weather",
        "description": "Get the current weather in a given location",
        "input_schema": {
          "type": "object",
          "properties": {
            "location": {
              "type": "string",
              "description": "City and state or city and country"
            }
          },
          "required": ["location"]
        }
      },
      {
        "name": "get_time",
        "description": "Get the current time in a given timezone or city",
        "input_schema": {
          "type": "object",
          "properties": {
            "location": {
              "type": "string",
              "description": "City or timezone name"
            }
          },
          "required": ["location"]
        }
      }
    ],
    "tool_choice": {"type": "auto"}
  }' | jq
```

**Expected:** `content` array with at least one `tool_use` block. Each block's `name` is either `"get_weather"` or `"get_time"`.

---

## 9. Chat Completions -- 5 Tools

Provides five tool definitions and a prompt designed to exercise all of them in parallel. Validates that the serving stack passes a larger tool set through correctly.

```bash
curl -s -X POST ${BASE_URL}/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "'${MODEL}'",
    "messages": [
      {
        "role": "system",
        "content": "You are a travel planning assistant. You MUST call ALL relevant tools in parallel to answer the user. For this request, call get_weather for the destination, get_time for the destination, search_flights, get_exchange_rate, and translate_phrase."
      },
      {
        "role": "user",
        "content": "I want to travel from New York to Tokyo. What is the weather and time there, find me flights, what is the USD to JPY rate, and how do I say hello in Japanese?"
      }
    ],
    "tools": [
      {
        "type": "function",
        "function": {
          "name": "get_weather",
          "description": "Get weather for a location",
          "parameters": {
            "type": "object",
            "properties": {
              "location": {"type": "string"}
            },
            "required": ["location"]
          }
        }
      },
      {
        "type": "function",
        "function": {
          "name": "get_time",
          "description": "Get current time in a city",
          "parameters": {
            "type": "object",
            "properties": {
              "city": {"type": "string"}
            },
            "required": ["city"]
          }
        }
      },
      {
        "type": "function",
        "function": {
          "name": "search_flights",
          "description": "Search for flights between cities",
          "parameters": {
            "type": "object",
            "properties": {
              "origin": {"type": "string"},
              "destination": {"type": "string"},
              "date": {"type": "string", "description": "YYYY-MM-DD"}
            },
            "required": ["origin", "destination"]
          }
        }
      },
      {
        "type": "function",
        "function": {
          "name": "get_exchange_rate",
          "description": "Get exchange rate between currencies",
          "parameters": {
            "type": "object",
            "properties": {
              "from_currency": {"type": "string"},
              "to_currency": {"type": "string"}
            },
            "required": ["from_currency", "to_currency"]
          }
        }
      },
      {
        "type": "function",
        "function": {
          "name": "translate_phrase",
          "description": "Translate a phrase to another language",
          "parameters": {
            "type": "object",
            "properties": {
              "phrase": {"type": "string"},
              "target_language": {"type": "string"}
            },
            "required": ["phrase", "target_language"]
          }
        }
      }
    ],
    "tool_choice": "auto",
    "parallel_tool_calls": true
  }' | jq
```

**Expected:** `choices[].message.tool_calls` with 5 entries. The set of `function.name` values, sorted, equals `["get_exchange_rate", "get_time", "get_weather", "search_flights", "translate_phrase"]`.

---

## 10. Messages API -- 5 Tools

Same five-tool scenario in Anthropic Messages format.

```bash
curl -s -X POST ${BASE_URL}/v1/messages \
  -H 'Content-Type: application/json' \
  -H 'x-api-key: dummy' \
  -H 'anthropic-version: 2023-06-01' \
  -d '{
    "model": "'${MODEL}'",
    "max_tokens": 2048,
    "system": "You are a travel planning assistant. You MUST call ALL relevant tools to answer the user. For this request, call get_weather for the destination, get_time for the destination, search_flights, get_exchange_rate, and translate_phrase.",
    "messages": [
      {
        "role": "user",
        "content": "I want to travel from New York to Tokyo. What is the weather and time there, find me flights, what is the USD to JPY rate, and how do I say hello in Japanese?"
      }
    ],
    "tools": [
      {
        "name": "get_weather",
        "description": "Get weather for a location",
        "input_schema": {
          "type": "object",
          "properties": {
            "location": {"type": "string"}
          },
          "required": ["location"]
        }
      },
      {
        "name": "get_time",
        "description": "Get current time in a city",
        "input_schema": {
          "type": "object",
          "properties": {
            "city": {"type": "string"}
          },
          "required": ["city"]
        }
      },
      {
        "name": "search_flights",
        "description": "Search for flights between cities",
        "input_schema": {
          "type": "object",
          "properties": {
            "origin": {"type": "string"},
            "destination": {"type": "string"},
            "date": {"type": "string", "description": "YYYY-MM-DD"}
          },
          "required": ["origin", "destination"]
        }
      },
      {
        "name": "get_exchange_rate",
        "description": "Get exchange rate between currencies",
        "input_schema": {
          "type": "object",
          "properties": {
            "from_currency": {"type": "string"},
            "to_currency": {"type": "string"}
          },
          "required": ["from_currency", "to_currency"]
        }
      },
      {
        "name": "translate_phrase",
        "description": "Translate a phrase to another language",
        "input_schema": {
          "type": "object",
          "properties": {
            "phrase": {"type": "string"},
            "target_language": {"type": "string"}
          },
          "required": ["phrase", "target_language"]
        }
      }
    ],
    "tool_choice": {"type": "auto"}
  }' | jq
```

**Expected:** `content` array with 5 `tool_use` blocks. The sorted set of `name` values equals `["get_exchange_rate", "get_time", "get_weather", "search_flights", "translate_phrase"]`.

---

## 11. Chat Completions -- tool_choice Specific Function

Sends a prompt unrelated to the tools but forces the model to call `get_weather` via `tool_choice`. Validates that `tool_choice` with a specific function name is honored.

```bash
curl -s -X POST ${BASE_URL}/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "'${MODEL}'",
    "messages": [
      {
        "role": "user",
        "content": "Tell me a joke"
      }
    ],
    "tools": [
      {
        "type": "function",
        "function": {
          "name": "get_weather",
          "description": "Get weather for a location",
          "parameters": {
            "type": "object",
            "properties": {
              "location": {"type": "string"}
            },
            "required": ["location"]
          }
        }
      },
      {
        "type": "function",
        "function": {
          "name": "get_time",
          "description": "Get current time",
          "parameters": {
            "type": "object",
            "properties": {
              "city": {"type": "string"}
            },
            "required": ["city"]
          }
        }
      }
    ],
    "tool_choice": {"type": "function", "function": {"name": "get_weather"}}
  }' | jq
```

**Expected:** `choices[].message.tool_calls[0].function.name == "get_weather"` regardless of the prompt content.

---

## 12. Messages API -- tool_choice Specific Tool

Same forced-tool scenario in Anthropic Messages format.

```bash
curl -s -X POST ${BASE_URL}/v1/messages \
  -H 'Content-Type: application/json' \
  -H 'x-api-key: dummy' \
  -H 'anthropic-version: 2023-06-01' \
  -d '{
    "model": "'${MODEL}'",
    "max_tokens": 1024,
    "messages": [
      {
        "role": "user",
        "content": "Tell me a joke"
      }
    ],
    "tools": [
      {
        "name": "get_weather",
        "description": "Get weather for a location",
        "input_schema": {
          "type": "object",
          "properties": {
            "location": {"type": "string"}
          },
          "required": ["location"]
        }
      },
      {
        "name": "get_time",
        "description": "Get current time",
        "input_schema": {
          "type": "object",
          "properties": {
            "city": {"type": "string"}
          },
          "required": ["city"]
        }
      }
    ],
    "tool_choice": {"type": "tool", "name": "get_weather"}
  }' | jq
```

**Expected:** `content` array with at least one `tool_use` block where `name == "get_weather"`.

---

## 13. Chat Completions -- Complex Nested Parameters

Sends a tool with deeply nested parameter schemas (arrays of objects, nested objects) and verifies the model populates them correctly.

```bash
curl -s -X POST ${BASE_URL}/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "'${MODEL}'",
    "messages": [
      {
        "role": "user",
        "content": "Create a meeting titled Team Standup for 2025-07-15T09:00:00 with alice@example.com and bob@example.com, set it as high priority with a 30 minute reminder."
      }
    ],
    "tools": [
      {
        "type": "function",
        "function": {
          "name": "create_calendar_event",
          "description": "Create a calendar event with attendees and settings",
          "parameters": {
            "type": "object",
            "properties": {
              "title": {"type": "string"},
              "datetime": {
                "type": "string",
                "description": "ISO 8601 datetime"
              },
              "attendees": {
                "type": "array",
                "items": {
                  "type": "object",
                  "properties": {
                    "email": {"type": "string"},
                    "role": {
                      "type": "string",
                      "enum": ["required", "optional"]
                    }
                  },
                  "required": ["email"]
                }
              },
              "settings": {
                "type": "object",
                "properties": {
                  "priority": {
                    "type": "string",
                    "enum": ["low", "medium", "high"]
                  },
                  "reminders": {
                    "type": "array",
                    "items": {
                      "type": "integer",
                      "description": "Minutes before event"
                    }
                  }
                }
              }
            },
            "required": ["title", "datetime", "attendees"]
          }
        }
      }
    ],
    "tool_choice": "auto"
  }' | jq
```

**Expected:** `choices[].message.tool_calls[0].function.name == "create_calendar_event"`. The parsed `function.arguments` contains an `attendees` array with at least 2 entries.

---

## 14. Chat Completions -- Tool with No Parameters

Provides a tool that takes no arguments. Verifies the model can call it without generating spurious parameters.

```bash
curl -s -X POST ${BASE_URL}/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "'${MODEL}'",
    "messages": [
      {
        "role": "user",
        "content": "What is the current server status?"
      }
    ],
    "tools": [
      {
        "type": "function",
        "function": {
          "name": "get_server_status",
          "description": "Returns the current server health status. Takes no arguments.",
          "parameters": {
            "type": "object",
            "properties": {}
          }
        }
      }
    ],
    "tool_choice": "auto"
  }' | jq
```

**Expected:** `choices[].message.tool_calls[0].function.name == "get_server_status"`.

---

## 15. Chat Completions -- Multiple Calls to Same Tool

Asks about weather in three cities with a single tool definition. Verifies the model issues multiple parallel calls to the same function with different arguments.

```bash
curl -s -X POST ${BASE_URL}/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "'${MODEL}'",
    "messages": [
      {
        "role": "system",
        "content": "You are a helpful assistant. When asked about weather in multiple cities, make a separate get_weather call for EACH city."
      },
      {
        "role": "user",
        "content": "What is the weather in San Francisco, New York, and London?"
      }
    ],
    "tools": [
      {
        "type": "function",
        "function": {
          "name": "get_weather",
          "description": "Get weather for a location",
          "parameters": {
            "type": "object",
            "properties": {
              "location": {"type": "string"}
            },
            "required": ["location"]
          }
        }
      }
    ],
    "tool_choice": "auto",
    "parallel_tool_calls": true
  }' | jq
```

**Expected:** `choices[].message.tool_calls` with at least one entry. Every entry has `function.name == "get_weather"`.

---

## 16. Chat Completions -- Verify All Tool Definitions Reach Model

Registers five distinctly named tools and asks the model to list them. Confirms every definition is visible to the model through the serving stack.

```bash
curl -s -X POST ${BASE_URL}/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "'${MODEL}'",
    "messages": [
      {
        "role": "user",
        "content": "List the exact names of all tools available to you, one per line. Do not explain them."
      }
    ],
    "tools": [
      {
        "type": "function",
        "function": {
          "name": "tool_alpha",
          "description": "First tool",
          "parameters": {"type": "object", "properties": {}}
        }
      },
      {
        "type": "function",
        "function": {
          "name": "tool_beta",
          "description": "Second tool",
          "parameters": {"type": "object", "properties": {}}
        }
      },
      {
        "type": "function",
        "function": {
          "name": "tool_gamma",
          "description": "Third tool",
          "parameters": {"type": "object", "properties": {}}
        }
      },
      {
        "type": "function",
        "function": {
          "name": "tool_delta",
          "description": "Fourth tool",
          "parameters": {"type": "object", "properties": {}}
        }
      },
      {
        "type": "function",
        "function": {
          "name": "tool_epsilon",
          "description": "Fifth tool",
          "parameters": {"type": "object", "properties": {}}
        }
      }
    ],
    "max_tokens": 100
  }' | jq
```

**Expected:** `choices[].message.content` mentions all five names: `tool_alpha`, `tool_beta`, `tool_gamma`, `tool_delta`, `tool_epsilon`.

---

## 17. Messages API -- Verify All Tool Definitions Reach Model

Same five-tool visibility test in Anthropic Messages format.

```bash
curl -s -X POST ${BASE_URL}/v1/messages \
  -H 'Content-Type: application/json' \
  -H 'x-api-key: dummy' \
  -H 'anthropic-version: 2023-06-01' \
  -d '{
    "model": "'${MODEL}'",
    "max_tokens": 100,
    "messages": [
      {
        "role": "user",
        "content": "List the exact names of all tools available to you, one per line. Do not explain them."
      }
    ],
    "tools": [
      {
        "name": "tool_alpha",
        "description": "First tool",
        "input_schema": {"type": "object", "properties": {}}
      },
      {
        "name": "tool_beta",
        "description": "Second tool",
        "input_schema": {"type": "object", "properties": {}}
      },
      {
        "name": "tool_gamma",
        "description": "Third tool",
        "input_schema": {"type": "object", "properties": {}}
      },
      {
        "name": "tool_delta",
        "description": "Fourth tool",
        "input_schema": {"type": "object", "properties": {}}
      },
      {
        "name": "tool_epsilon",
        "description": "Fifth tool",
        "input_schema": {"type": "object", "properties": {}}
      }
    ]
  }' | jq
```

**Expected:** `content` text mentions all five names: `tool_alpha`, `tool_beta`, `tool_gamma`, `tool_delta`, `tool_epsilon`.

---

## 18. Chat Completions -- Regression (no tools)

Verifies non-tool-calling requests still work.

```bash
curl -s -X POST ${BASE_URL}/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "'${MODEL}'",
    "messages": [
      {
        "role": "user",
        "content": "Explain how a simple agent loop works in 3 sentences."
      }
    ]
  }' | jq
```

**Expected:** `choices[].message.content` with a text response.

---

## 19. Messages API -- Regression (no tools)

```bash
curl -s -X POST ${BASE_URL}/v1/messages \
  -H 'Content-Type: application/json' \
  -H 'x-api-key: dummy' \
  -H 'anthropic-version: 2023-06-01' \
  -d '{
    "model": "'${MODEL}'",
    "max_tokens": 1024,
    "messages": [
      {
        "role": "user",
        "content": "Explain how a simple agent loop works in 3 sentences."
      }
    ]
  }' | jq
```

**Expected:** `content` array with a `text` block. `stop_reason: "end_turn"`.

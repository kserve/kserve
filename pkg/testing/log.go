/*
Copyright 2025 The KServe Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package testing

import (
	"fmt"
	"slices"
	"strings"

	"github.com/go-logr/logr"
)

// testingT is a subset of the testing.T interface.
// This is used to allow for easy testing of the logger itself.
type testingT interface {
	Log(args ...interface{})
	Helper()
}

// NewTestLogger returns a logr.Logger that prints to the given testing.T.
// This is useful for logging in tests, as the output will be buffered and
// printed only if the test fails.
func NewTestLogger(t testingT) logr.Logger {
	return logr.New(&testingLogSink{t: t})
}

// testingLogSink is a logr.LogSink that prints to a testing.T.
type testingLogSink struct {
	t      testingT
	name   string
	values []interface{}
}

// ensure testingLogSink implements logr.LogSink
var _ logr.LogSink = &testingLogSink{}

// Init is called by logr.New to initialize the sink.
// We don't need any special initialization, so this is a no-op.
func (l *testingLogSink) Init(info logr.RuntimeInfo) {}

// Enabled returns true if the log level is enabled.
// For testing, we generally want to see all logs, so this always returns true.
func (l *testingLogSink) Enabled(level int) bool {
	return true
}

// Info logs a non-error message with the given key/value pairs.
func (l *testingLogSink) Info(level int, msg string, keysAndValues ...interface{}) {
	l.t.Helper()
	args := l.formatArgs(msg, keysAndValues)
	l.t.Log(args...)
}

// Error logs an error message with the given key/value pairs.
func (l *testingLogSink) Error(err error, msg string, keysAndValues ...interface{}) {
	l.t.Helper()
	args := l.formatArgs(msg, keysAndValues)
	// Prepend the error to the log arguments
	allArgs := append([]interface{}{"ERROR", err}, args...)
	l.t.Log(allArgs...)
}

// WithValues returns a new LogSink with additional key/value pairs.
func (l *testingLogSink) WithValues(keysAndValues ...interface{}) logr.LogSink {
	newSink := l.clone()
	newSink.values = append(newSink.values, keysAndValues...)
	return newSink
}

// WithName returns a new LogSink with an additional name component.
func (l *testingLogSink) WithName(name string) logr.LogSink {
	newSink := l.clone()
	if len(l.name) > 0 {
		newSink.name = l.name + "." + name
	} else {
		newSink.name = name
	}
	return newSink
}

// formatArgs formats the log message and key/value pairs into a slice of interfaces for t.Log.
func (l *testingLogSink) formatArgs(msg string, keysAndValues []interface{}) []interface{} {
	l.t.Helper()

	// Start with the logger name if it exists
	var namePart string
	if l.name != "" {
		namePart = "[" + l.name + "] "
	}

	// Combine the static values from WithValues with the call-site values
	allValues := slices.Clone(l.values)
	allValues = append(allValues, keysAndValues...)

	// Format the key-value pairs
	var kvParts []string
	for i := 0; i < len(allValues); i += 2 {
		key := allValues[i]
		var val interface{} = "(no-value)"
		if i+1 < len(allValues) {
			val = allValues[i+1]
		}
		kvParts = append(kvParts, fmt.Sprintf("%s=%+v", key, val))
	}

	// Combine all parts into a single log line
	return []interface{}{
		fmt.Sprintf("%s%s (%s)", namePart, msg, strings.Join(kvParts, " ")),
	}
}

// clone creates a deep copy of the sink.
func (l *testingLogSink) clone() *testingLogSink {
	newSink := &testingLogSink{
		t:      l.t,
		name:   l.name,
		values: make([]interface{}, len(l.values)),
	}
	copy(newSink.values, l.values)
	return newSink
}

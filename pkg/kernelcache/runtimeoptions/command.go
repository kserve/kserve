/*
Copyright 2026 The KServe Authors.

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

package runtimeoptions

import (
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	corev1 "k8s.io/api/core/v1"
)

var simpleEnvironmentReference = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}|\$([A-Za-z_][A-Za-z0-9_]*)`)

func resolveCommand(result map[string]string, command []string, env []corev1.EnvVar, index specIndex) {
	if len(command) == 0 {
		return
	}
	environment := literalEnvironment(env)
	if script, ok := shellScript(command); ok {
		tokens, ok := extractShellCommand(script, environment)
		if !ok {
			return
		}
		parseOptionTokens(result, tokens, index, true)
		return
	}
	parseOptionTokens(result, command, index, false)
}

func literalEnvironment(env []corev1.EnvVar) map[string]string {
	values := make(map[string]string, len(env))
	for _, variable := range env {
		if variable.ValueFrom == nil {
			values[variable.Name] = variable.Value
		}
	}
	return values
}

func shellScript(command []string) (string, bool) {
	if len(command) < 3 {
		return "", false
	}
	name := filepath.Base(command[0])
	if name != "sh" && name != "bash" && name != "dash" && name != "zsh" {
		return "", false
	}
	for index := 1; index < len(command)-1; index++ {
		argument := command[index]
		if isShellCommandFlag(argument) {
			return command[index+1], true
		}
	}
	return "", false
}

func isShellCommandFlag(argument string) bool {
	if argument == "-c" {
		return true
	}
	return strings.HasPrefix(argument, "-") &&
		!strings.HasPrefix(argument, "--") &&
		strings.ContainsRune(argument, 'c')
}

func extractShellCommand(script string, environment map[string]string) ([]string, bool) {
	execIndex := strings.LastIndex(script, "exec ")
	if execIndex >= 0 {
		prefix := strings.TrimSpace(script[:execIndex])
		wrapperQuote := byte(0)
		if strings.HasSuffix(prefix, "\"") {
			wrapperQuote = '"'
		} else if strings.HasSuffix(prefix, "'") {
			wrapperQuote = '\''
		}
		script = script[execIndex+len("exec "):]
		script = strings.TrimSpace(script)
		if wrapperQuote != 0 && strings.HasSuffix(script, string(wrapperQuote)) {
			script = strings.TrimSpace(strings.TrimSuffix(script, string(wrapperQuote)))
		}
	}
	script = strings.TrimSpace(script)
	script = expandSimpleEnvironmentReferences(script, environment)
	return tokenizeShellWords(script)
}

func expandSimpleEnvironmentReferences(script string, environment map[string]string) string {
	var expanded strings.Builder
	inSingleQuote := false
	escaped := false
	for position := 0; position < len(script); {
		character := script[position]
		if escaped {
			expanded.WriteByte(character)
			escaped = false
			position++
			continue
		}
		if character == '\\' && !inSingleQuote {
			expanded.WriteByte(character)
			escaped = true
			position++
			continue
		}
		if character == '\'' {
			inSingleQuote = !inSingleQuote
			expanded.WriteByte(character)
			position++
			continue
		}
		if character == '$' && !inSingleQuote {
			reference := simpleEnvironmentReference.FindString(script[position:])
			if reference != "" {
				matches := simpleEnvironmentReference.FindStringSubmatch(reference)
				name := matches[1]
				if name == "" {
					name = matches[2]
				}
				if value, ok := environment[name]; ok {
					expanded.WriteString(value)
				} else {
					expanded.WriteString(reference)
				}
				position += len(reference)
				continue
			}
		}
		expanded.WriteByte(character)
		position++
	}
	return expanded.String()
}

func tokenizeShellWords(value string) ([]string, bool) {
	var tokens []string
	var current strings.Builder
	inSingleQuote := false
	inDoubleQuote := false
	escaped := false
	started := false
	flush := func() {
		if started {
			tokens = append(tokens, current.String())
			current.Reset()
			started = false
		}
	}

	for index := range len(value) {
		character := value[index]
		if escaped {
			if character != '\n' {
				current.WriteByte(character)
				started = true
			}
			escaped = false
			continue
		}
		if character == '\\' && !inSingleQuote {
			escaped = true
			continue
		}
		if inSingleQuote {
			if character == '\'' {
				inSingleQuote = false
			} else {
				current.WriteByte(character)
				started = true
			}
			continue
		}
		if inDoubleQuote {
			if character == '"' {
				inDoubleQuote = false
			} else {
				current.WriteByte(character)
				started = true
			}
			continue
		}

		switch {
		case character == '\'':
			inSingleQuote = true
			started = true
		case character == '"':
			inDoubleQuote = true
			started = true
		case unicode.IsSpace(rune(character)):
			flush()
		case strings.ContainsRune("|;&<>", rune(character)):
			return nil, false
		case character == '`' || (character == '$' && index+1 < len(value) && value[index+1] == '('):
			return nil, false
		default:
			current.WriteByte(character)
			started = true
		}
	}

	if escaped || inSingleQuote || inDoubleQuote {
		return nil, false
	}
	flush()
	return tokens, true
}

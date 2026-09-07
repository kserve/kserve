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

package logformat

import (
	"path"
	"runtime"
	"strconv"
	"strings"

	logging "github.com/sirupsen/logrus"
)

/*
Default is the default formatter for our logs
*/
var Default = &logging.TextFormatter{
	FullTimestamp:    true,
	TimestampFormat:  "2006-01-02 15:04:05",
	ForceColors:      true,
	CallerPrettyfier: func(frame *runtime.Frame) (string, string) { return "", "" },
}

/*
Debug is the debug formatter for our logs, it prints additional data to aid with debugging
*/
var Debug = &logging.TextFormatter{
	FullTimestamp:   true,
	TimestampFormat: "2006-01-02 15:04:05",
	ForceColors:     true,
	CallerPrettyfier: func(frame *runtime.Frame) (string, string) {
		s := strings.Split(frame.Function, ".")
		funcName := "[" + s[len(s)-1] + "]"
		fileName := " [" + path.Base(frame.File) + ":" + strconv.Itoa(frame.Line) + "]"
		return funcName, fileName
	},
}

func ConfigureLogging(logLevel string) error {
	if logLevel != "" {
		logging.Infof("Setting log level: %s", logLevel)
		_level, err := logging.ParseLevel(logLevel)
		if err != nil {
			logging.Errorf("Error setting log level: %v", err)
			return err
		}
		logging.SetLevel(_level)

		if logLevel == "debug" {
			logging.Infof("Switching to debug log format")
			logging.SetFormatter(Debug)
		}
	}

	return nil
}

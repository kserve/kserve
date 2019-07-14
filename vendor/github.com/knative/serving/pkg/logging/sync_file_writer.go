/*
Copyright 2019 The Knative Authors

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

package logging

import (
	"io"
	"os"
	"sync"
)

var _ io.Writer = (*syncFileWriter)(nil)

type syncFileWriter struct {
	file *os.File
	mux  sync.Mutex
}

// NewSyncFileWriter returns an io.Writer that is backed by an os.File
// and that synchronizes the writes to the file.
// This is suitable for use with non-threadsafe writers, e.g. os.Stdout.
func NewSyncFileWriter(file *os.File) io.Writer {
	return &syncFileWriter{file, sync.Mutex{}}
}

// Write writes len(b) bytes to the file.
func (w *syncFileWriter) Write(b []byte) (n int, err error) {
	w.mux.Lock()
	defer w.mux.Unlock()
	return w.file.Write(b)
}

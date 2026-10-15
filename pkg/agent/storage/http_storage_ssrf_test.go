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

package storage

import (
	"net/http"
	"net/url"
	"testing"
)

func TestHTTPSDownloaderRejectsInternalTarget(t *testing.T) {
	rawURI := "http://127.0.0.1/model"
	parsedURI, err := url.Parse(rawURI)
	if err != nil {
		t.Fatal(err)
	}
	downloader := HTTPSDownloader{StorageUri: rawURI, Uri: parsedURI}
	if err := downloader.Download(http.Client{}); err == nil {
		t.Fatal("expected loopback storage URI to be rejected")
	}
}

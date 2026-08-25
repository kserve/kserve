/*
Copyright 2021 The KServe Authors.

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

package mocks

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	gstorage "cloud.google.com/go/storage"
	"github.com/googleapis/google-cloud-go-testing/storage/stiface"
	"google.golang.org/api/iterator"
)

type mockGCSClient struct {
	stiface.Client
	buckets map[string]*mockBucket
}

type mockBucket struct {
	attrs   *gstorage.BucketAttrs
	objects map[string]*gstorage.ObjectAttrs
}

func NewMockClient() stiface.Client {
	return &mockGCSClient{buckets: map[string]*mockBucket{}}
}

func (c *mockGCSClient) Bucket(name string) stiface.BucketHandle {
	return mockBucketHandle{c: c, name: name}
}

type mockBucketHandle struct {
	stiface.BucketHandle
	c    *mockGCSClient
	name string
}

func (b mockBucketHandle) Create(_ context.Context, _ string, attrs *gstorage.BucketAttrs) error {
	if _, ok := b.c.buckets[b.name]; ok {
		return fmt.Errorf("bucket %q already exists", b.name)
	}
	if attrs == nil {
		attrs = &gstorage.BucketAttrs{}
	}
	attrs.Name = b.name
	b.c.buckets[b.name] = &mockBucket{attrs: attrs, objects: map[string]*gstorage.ObjectAttrs{}}
	return nil
}

func (b mockBucketHandle) Objects(ctx context.Context, query *gstorage.Query) stiface.ObjectIterator {
	var items []*gstorage.ObjectAttrs
	objs := b.c.buckets[b.name].objects
	for key, element := range objs {
		if strings.Contains(key, query.Prefix) {
			items = append(items, element)
		}
	}
	return &mockObjectIterator{b: b, items: items}
}

type mockObjectIterator struct {
	stiface.ObjectIterator
	b     mockBucketHandle
	items []*gstorage.ObjectAttrs
}

func (i *mockObjectIterator) Next() (*gstorage.ObjectAttrs, error) {
	if len(i.items) == 0 {
		return nil, iterator.Done
	}
	item := i.items[0]
	i.items = i.items[1:]
	return item, nil
}

func (b mockBucketHandle) Object(name string) stiface.ObjectHandle {
	return mockObjectHandle{c: b.c, bucketName: b.name, name: name}
}

type mockObjectHandle struct {
	stiface.ObjectHandle
	c          *mockGCSClient
	bucketName string
	name       string
}

func (o mockObjectHandle) Attrs(context.Context) (*gstorage.ObjectAttrs, error) {
	bkt, ok := o.c.buckets[o.bucketName]
	if !ok {
		return nil, fmt.Errorf("bucket %q not found", o.bucketName)
	}
	contents, ok := bkt.objects[o.name]
	if !ok {
		return nil, gstorage.ErrObjectNotExist
	}
	return contents, nil
}

func (o mockObjectHandle) NewReader(context.Context) (stiface.Reader, error) {
	bkt, ok := o.c.buckets[o.bucketName]
	if !ok {
		return nil, fmt.Errorf("bucket %q not found", o.bucketName)
	}
	contents, ok := bkt.objects[o.name]
	if !ok {
		return nil, fmt.Errorf("object %q not found in bucket %q", o.name, o.bucketName)
	}
	return mockReader{r: bytes.NewReader(contents.MD5)}, nil
}

func (o mockObjectHandle) NewWriter(context.Context) stiface.Writer {
	attrs := &gstorage.ObjectAttrs{
		Bucket: o.bucketName,
		Name:   o.name,
		MD5:    nil,
	}
	o.c.buckets[o.bucketName].objects[o.name] = attrs
	return &mockWriter{o: o, obj: attrs}
}

type mockReader struct {
	stiface.Reader
	r *bytes.Reader
}

func (r mockReader) Read(buf []byte) (int, error) {
	return r.r.Read(buf)
}

func (r mockReader) Close() error {
	return nil
}

type mockWriter struct {
	stiface.Writer
	o   mockObjectHandle
	buf bytes.Buffer
	obj *gstorage.ObjectAttrs
}

func (w *mockWriter) Write(data []byte) (int, error) {
	int, err := w.buf.Write(data)
	w.obj.MD5 = data
	return int, err
}

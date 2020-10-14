package test

import (
	"bytes"
	"cloud.google.com/go/storage"
	"context"
	"fmt"
	"github.com/kubeflow/kfserving/pkg/agent/test/mockapi"
	"github.com/kubeflow/kfserving/pkg/agent/kfstorage"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"google.golang.org/api/iterator"
	"io/ioutil"
	logger "log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type mockGCSClient struct {
	mockapi.Client
	buckets map[string]*mockBucket
}

type mockBucket struct {
	attrs   *storage.BucketAttrs
	objects map[string]*storage.ObjectAttrs
}

func newMockClient() mockapi.Client {
	return &mockGCSClient{buckets: map[string]*mockBucket{}}
}

func (c *mockGCSClient) Bucket(name string) mockapi.BucketHandle {
	return mockBucketHandle{c: c, name: name}
}

type mockBucketHandle struct {
	mockapi.BucketHandle
	c    *mockGCSClient
	name string
}

func (b mockBucketHandle) Create(_ context.Context, _ string, attrs *storage.BucketAttrs) error {
	if _, ok := b.c.buckets[b.name]; ok {
		return fmt.Errorf("bucket %q already exists", b.name)
	}
	if attrs == nil {
		attrs = &storage.BucketAttrs{}
	}
	attrs.Name = b.name
	b.c.buckets[b.name] = &mockBucket{attrs: attrs, objects: map[string]*storage.ObjectAttrs{}}
	return nil
}

func (b mockBucketHandle) Objects(ctx context.Context, query *storage.Query) mockapi.ObjectIterator {
	var items []*storage.ObjectAttrs
	objs := b.c.buckets[b.name].objects
	for key, element := range objs {
		if strings.Contains(key, query.Prefix){
			items = append(items, element)
		}
	}
	return &mockObjectIterator{b: b, items: items}
}

type mockObjectIterator struct {
	mockapi.ObjectIterator
	b       mockBucketHandle
	items   []*storage.ObjectAttrs
}

func (i *mockObjectIterator) Next() (*storage.ObjectAttrs, error) {
	if len(i.items) == 0 {
		return nil, iterator.Done
	}
	item := i.items[0]
	i.items = i.items[1:]
	return item, nil
}

func (b mockBucketHandle) Object(name string) mockapi.ObjectHandle {
	return mockObjectHandle{c: b.c, bucketName: b.name, name: name}
}

type mockObjectHandle struct {
	mockapi.ObjectHandle
	c          *mockGCSClient
	bucketName string
	name       string
}

func (o mockObjectHandle) NewReader(context.Context) (mockapi.Reader, error) {
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

func (o mockObjectHandle) NewWriter(context.Context) mockapi.Writer {
	attrs := &storage.ObjectAttrs{
		Bucket: o.bucketName,
		Name:   o.name,
		MD5:    nil,
	}
	o.c.buckets[o.bucketName].objects[o.name] = attrs
	return &mockWriter{o: o, obj: attrs}
}

type mockReader struct {
	mockapi.Reader
	r *bytes.Reader
}

func (r mockReader) Read(buf []byte) (int, error) {
	return r.r.Read(buf)
}

func (r mockReader) Close() error {
	return nil
}

type mockWriter struct {
	mockapi.Writer
	o     mockObjectHandle
	buf   bytes.Buffer
	obj   *storage.ObjectAttrs
}

func (w *mockWriter) Write(data []byte) (int, error) {
	int, err := w.buf.Write(data)
	w.obj.MD5 = data
	return int, err
}

func TestGCSMockDownload(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "GCS Provider Test Suite")
}

var _ = Describe("GCS Provider", func() {
	var modelDir string
	BeforeEach(func() {
		dir, err := ioutil.TempDir("", "example")
		if err != nil {
			logger.Fatal(err)
		}
		modelDir = dir
		logger.Printf("Creating temp dir %v\n", modelDir)
	})
	AfterEach(func() {
		os.RemoveAll(modelDir)
		logger.Printf("Deleted temp dir %v\n", modelDir)
	})
	Describe("Use GCS Downloader", func() {
		Context("Download Mocked Model", func() {
			It("should download test model and write contents", func() {
				defer GinkgoRecover()

				logger.Printf("Creating mock GCS Client")
				ctx := context.Background()
				client := newMockClient()
				cl := kfstorage.GCSProvider {
					Client: client,
				}

				logger.Printf("Populating mocked bucket with test model")
				modelName := "model1"
				modelStorageURI := "gs://testBucket/testModel1"
				bkt := client.Bucket("testBucket")
				if err := bkt.Create(ctx, "test", nil); err != nil {
					Fail("Error creating bucket.")
				}
				const modelContents = "Model Contents"
				w := bkt.Object("testModel1").NewWriter(ctx)
				if _, err := fmt.Fprint(w, modelContents); err != nil {
					Fail("Failed to write contents.")
				}
				err := cl.DownloadModel(modelDir, modelName, modelStorageURI)
				Expect(err).To(BeNil())

				testFile := filepath.Join(modelDir, "model1/testModel1")
				dat, err := ioutil.ReadFile(testFile)
				Expect(err).To(BeNil())
				Expect(string(dat)).To(Equal(modelContents))
			})
		})
	})
})

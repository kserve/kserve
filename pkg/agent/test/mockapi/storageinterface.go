// storageinterface includes gcs related interfaces, which can be used to wrap
// a type which implements the given methods. This is useful for testing, where mocking
// needs to be done for the gcs client.

// This class was referenced from googleapis/google-cloud-go-testing

package mockapi

import (
	"context"
	"io"
	"cloud.google.com/go/iam"
	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
)

type Client interface {
	Bucket(name string) BucketHandle
	Buckets(ctx context.Context, projectID string) BucketIterator
	Close() error
	CreateHMACKey(ctx context.Context, projectID, serviceAccountEmail string, opts ...storage.HMACKeyOption) (*storage.HMACKey, error)
	HMACKeyHandle(projectID, accessID string) *storage.HMACKeyHandle
	ListHMACKeys(ctx context.Context, projectID string, opts ...storage.HMACKeyOption) *storage.HMACKeysIterator
	ServiceAccount(ctx context.Context, projectID string) (string, error)

	embedToIncludeNewMethods()
}

type ObjectHandle interface {
	ACL() ACLHandle
	Generation(int64) ObjectHandle
	If(storage.Conditions) ObjectHandle
	Key([]byte) ObjectHandle
	ReadCompressed(bool) ObjectHandle
	Attrs(context.Context) (*storage.ObjectAttrs, error)
	Update(context.Context, storage.ObjectAttrsToUpdate) (*storage.ObjectAttrs, error)
	NewReader(context.Context) (Reader, error)
	NewRangeReader(context.Context, int64, int64) (Reader, error)
	NewWriter(context.Context) Writer
	Delete(context.Context) error
	CopierFrom(ObjectHandle) Copier
	ComposerFrom(...ObjectHandle) Composer

	embedToIncludeNewMethods()
}

type BucketHandle interface {
	Create(context.Context, string, *storage.BucketAttrs) error
	Delete(context.Context) error
	DefaultObjectACL() ACLHandle
	Object(string) ObjectHandle
	Attrs(context.Context) (*storage.BucketAttrs, error)
	Update(context.Context, storage.BucketAttrsToUpdate) (*storage.BucketAttrs, error)
	If(storage.BucketConditions) BucketHandle
	Objects(context.Context, *storage.Query) ObjectIterator
	ACL() ACLHandle
	IAM() *iam.Handle
	UserProject(projectID string) BucketHandle
	Notifications(context.Context) (map[string]*storage.Notification, error)
	AddNotification(context.Context, *storage.Notification) (*storage.Notification, error)
	DeleteNotification(context.Context, string) error
	LockRetentionPolicy(context.Context) error

	embedToIncludeNewMethods()
}

type ObjectIterator interface {
	Next() (*storage.ObjectAttrs, error)
	PageInfo() *iterator.PageInfo

	embedToIncludeNewMethods()
}

type BucketIterator interface {
	SetPrefix(string)
	Next() (*storage.BucketAttrs, error)
	PageInfo() *iterator.PageInfo

	embedToIncludeNewMethods()
}

type ACLHandle interface {
	Delete(context.Context, storage.ACLEntity) error
	Set(context.Context, storage.ACLEntity, storage.ACLRole) error
	List(context.Context) ([]storage.ACLRule, error)

	embedToIncludeNewMethods()
}

type Reader interface {
	io.ReadCloser
	Size() int64
	Remain() int64
	ContentType() string
	ContentEncoding() string
	CacheControl() string

	embedToIncludeNewMethods()
}

type Writer interface {
	io.WriteCloser
	ObjectAttrs() *storage.ObjectAttrs
	SetChunkSize(int)
	SetProgressFunc(func(int64))
	SetCRC32C(uint32) // Sets both CRC32C and SendCRC32C.
	CloseWithError(err error) error
	Attrs() *storage.ObjectAttrs

	embedToIncludeNewMethods()
}

type Copier interface {
	ObjectAttrs() *storage.ObjectAttrs
	SetRewriteToken(string)
	SetProgressFunc(func(uint64, uint64))
	SetDestinationKMSKeyName(string)
	Run(context.Context) (*storage.ObjectAttrs, error)

	embedToIncludeNewMethods()
}

type Composer interface {
	ObjectAttrs() *storage.ObjectAttrs
	Run(context.Context) (*storage.ObjectAttrs, error)

	embedToIncludeNewMethods()
}
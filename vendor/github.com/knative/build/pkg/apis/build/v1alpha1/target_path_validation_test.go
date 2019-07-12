package v1alpha1

import (
	"testing"

	"github.com/knative/pkg/apis"
)

func TestValidateTargetPaths(t *testing.T) {
	for _, c := range []struct {
		desc    string
		paths   []string
		wantErr *apis.FieldError // if "", expect success.
	}{{
		desc: "same parents dir with different ",
		paths: []string{
			"a/b/c/d",
			"a/d",
		},
	}, {
		desc: "paths with overlap",
		paths: []string{
			"a/b/c/d",
			"a/b",
		},
		wantErr: &apis.FieldError{
			Message: "Overlapping Target Paths",
			Paths:   []string{"targetPath"},
		},
	}, {
		desc: "paths with overlap in different order",
		paths: []string{
			"a/b",
			"a/b/c",
		},
		wantErr: &apis.FieldError{
			Message: "Overlapping Target Paths",
			Paths:   []string{"targetPath"},
		},
	}, {
		desc: "paths with leaf node overlap",
		paths: []string{
			"a/d",
			"a/b/c/d",
		},
	}, {
		desc: "paths with no overlap",
		paths: []string{
			"e/f",
			"l/k",
			"a/b/c/d",
		},
	}, {
		desc: "paths with same length and different leaf node",
		paths: []string{
			"a/b/d",
			"a/b/c",
		},
	}, {
		desc: "same paths",
		paths: []string{
			"a/b/c",
			"a/b/c",
		},
		wantErr: &apis.FieldError{
			Message: "Overlapping Target Paths",
			Paths:   []string{"targetPath"},
		},
	}, {
		desc: "paths with same length and different leaf node",
		paths: []string{
			"github.com/foo/bar",
			"github.com/foo/frobber",
		},
	}, {
		desc: "paths with overlap",
		paths: []string{
			"github.com/foo/bar",
			"github.com/foo/",
		},
		wantErr: &apis.FieldError{
			Message: "Overlapping Target Paths",
			Paths:   []string{"targetPath"},
		},
	}, {
		desc: "paths with overlap in different order",
		paths: []string{
			"github.com/foo",
			"github.com/foo/bar",
		},
		wantErr: &apis.FieldError{
			Message: "Overlapping Target Paths",
			Paths:   []string{"targetPath"},
		},
	}, {
		desc: "paths with different leaf",
		paths: []string{
			"github.com/foobar",
			"github.com/foo",
			"github.com/bar",
		},
	}, {
		desc: "longer paths with different leaf node",
		paths: []string{
			"/go/src/github.com/knative/build",
			"/go/src/github.com/knative/serving",
			"/go/src/github.com/knative/build-pipeline",
		},
	}, {
		desc: "paths that start with /",
		paths: []string{
			"/dir/a",
			"/dir/a/b",
		},
		wantErr: &apis.FieldError{
			Message: "Overlapping Target Paths",
			Paths:   []string{"targetPath"},
		},
	}, {
		desc: "paths with different parent node",
		paths: []string{
			"a/b/c/d",
			"d/e",
			"b/c",
		},
	}, {
		desc: "multiple paths",
		paths: []string{
			"a/b/c/d",
			"a/d",
			"a/d",
		},
		wantErr: &apis.FieldError{
			Message: "Overlapping Target Paths",
			Paths:   []string{"targetPath"},
		},
	}, {
		desc: "paths starts with combination of / and no /",
		paths: []string{
			"/a/b/d",
			"a/e",
		},
	}, {
		desc: "paths with repeating nodes",
		paths: []string{
			"/a/a/b/d",
			"a/a",
		},
		wantErr: &apis.FieldError{
			Message: "Overlapping Target Paths",
			Paths:   []string{"targetPath"},
		},
	}, {
		desc: "paths with repeating nodes",
		paths: []string{
			"/a/a/b/d",
			"a/d",
		},
	}} {
		t.Run(c.desc, func(t *testing.T) {
			pathtree := pathTree{
				nodeMap: map[string]map[string]string{},
			}
			var verr *apis.FieldError
			for _, path := range c.paths {
				if verr = insertNode(path, pathtree); verr != nil {
					break
				}
			}
			if verr.Error() != c.wantErr.Error() {
				t.Errorf("validateTargetPaths(%s); got %#v, want %q", c.desc, verr, c.wantErr)
			}
		})
	}
}

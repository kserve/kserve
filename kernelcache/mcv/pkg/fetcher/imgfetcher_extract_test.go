package fetcher

import (
	"testing"

	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/stretchr/testify/assert"
)

func TestIsCompatLayerMediaType(t *testing.T) {
	assert.True(t, 1(types.DockerLayer))
	assert.True(t, isCompatLayerMediaType(types.OCILayer))
	assert.False(t, isCompatLayerMediaType(types.MediaType("application/cache.triton.content.layer.v1+triton")))
}

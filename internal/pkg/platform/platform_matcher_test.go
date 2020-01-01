package platform

import (
	"testing"

	"github.com/containers/image/v5/types"
	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
)

func TestWantedPlatformsCompatibility(t *testing.T) {
	ctx := &types.SystemContext{
		ArchitectureChoice: "arm",
		OSChoice:           "linux",
		VariantChoice:      "v6",
	}
	platforms, err := WantedPlatforms(ctx)
	assert.Nil(t, err)
	assert.Equal(t, len(platforms), 2)
	assert.Equal(t, platforms[0], imgspecv1.Platform{
		OS:           ctx.OSChoice,
		Architecture: ctx.ArchitectureChoice,
		Variant:      "v6",
	})
	assert.Equal(t, platforms[1], imgspecv1.Platform{
		OS:           ctx.OSChoice,
		Architecture: ctx.ArchitectureChoice,
		Variant:      "v5",
	})
}

func TestWantedPlatformsCustom(t *testing.T) {
	ctx := &types.SystemContext{
		ArchitectureChoice: "armel",
		OSChoice:           "freeBSD",
		VariantChoice:      "custom",
	}
	platforms, err := WantedPlatforms(ctx)
	assert.Nil(t, err)
	assert.Equal(t, len(platforms), 1)
	assert.Equal(t, platforms[0], imgspecv1.Platform{
		OS:           ctx.OSChoice,
		Architecture: ctx.ArchitectureChoice,
		Variant:      ctx.VariantChoice,
	})
}

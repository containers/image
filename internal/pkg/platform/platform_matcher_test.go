package platform

import (
	"fmt"
	"testing"

	"github.com/containers/image/v5/types"
	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
)

func TestWantedPlatforms(t *testing.T) {
	for _, c := range []struct {
		ctx      types.SystemContext
		expected []imgspecv1.Platform
	}{
		{ // X86_64 does not have variants
			types.SystemContext{ArchitectureChoice: "amd64", OSChoice: "linux"},
			[]imgspecv1.Platform{
				{OS: "linux", Architecture: "amd64", Variant: ""},
			},
		},
		{ // ARM with variant
			types.SystemContext{ArchitectureChoice: "arm", OSChoice: "linux", VariantChoice: "v6"},
			[]imgspecv1.Platform{
				{OS: "linux", Architecture: "arm", Variant: "v6"},
				{OS: "linux", Architecture: "arm", Variant: "v5"},
				{OS: "linux", Architecture: "arm", Variant: ""},
			},
		},
		{ // ARM without variant
			types.SystemContext{ArchitectureChoice: "arm", OSChoice: "linux"},
			[]imgspecv1.Platform{
				{OS: "linux", Architecture: "arm", Variant: ""},
				{OS: "linux", Architecture: "arm", Variant: "v8"},
				{OS: "linux", Architecture: "arm", Variant: "v7"},
				{OS: "linux", Architecture: "arm", Variant: "v6"},
				{OS: "linux", Architecture: "arm", Variant: "v5"},
			},
		},
		{ // ARM64 has a base variant
			types.SystemContext{ArchitectureChoice: "arm64", OSChoice: "linux"},
			[]imgspecv1.Platform{
				{OS: "linux", Architecture: "arm64", Variant: ""},
				{OS: "linux", Architecture: "arm64", Variant: "v8"},
			},
		},
		{ // Custom (completely unrecognized data)
			types.SystemContext{ArchitectureChoice: "armel", OSChoice: "freeBSD", VariantChoice: "custom"},
			[]imgspecv1.Platform{
				{OS: "freeBSD", Architecture: "armel", Variant: "custom"},
				{OS: "freeBSD", Architecture: "armel", Variant: ""},
			},
		},
	} {
		testName := fmt.Sprintf("%q/%q/%q", c.ctx.ArchitectureChoice, c.ctx.OSChoice, c.ctx.VariantChoice)
		platforms, err := WantedPlatforms(&c.ctx)
		assert.Nil(t, err, testName)
		assert.Equal(t, c.expected, platforms, testName)
	}
}

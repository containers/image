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
		{ // ARM
			types.SystemContext{ArchitectureChoice: "arm", OSChoice: "linux", VariantChoice: "v6"},
			[]imgspecv1.Platform{
				{OS: "linux", Architecture: "arm", Variant: "v6"},
				{OS: "linux", Architecture: "arm", Variant: "v5"},
			},
		},
		{ // Custom (completely unrecognized data)
			types.SystemContext{ArchitectureChoice: "armel", OSChoice: "freeBSD", VariantChoice: "custom"},
			[]imgspecv1.Platform{
				{OS: "freeBSD", Architecture: "armel", Variant: "custom"},
			},
		},
	} {
		testName := fmt.Sprintf("%q/%q/%q", c.ctx.ArchitectureChoice, c.ctx.OSChoice, c.ctx.VariantChoice)
		platforms, err := WantedPlatforms(&c.ctx)
		assert.Nil(t, err, testName)
		assert.Equal(t, c.expected, platforms, testName)
	}
}

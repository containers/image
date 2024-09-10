package layout

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestList(t *testing.T) {

	for _, test := range []struct {
		path    string
		num     int
		digests []string
		names   map[int]string
	}{
		{
			path: "fixtures/two_images_manifest",
			num:  2,
			digests: []string{
				"sha256:e692418e4cbaf90ca69d05a66403747baa33ee08806650b51fab815ad7fc331f",
				"sha256:5b0bcabd1ed22e9fb1310cf6c2dec7cdef19f0ad69efa1f392e94a4333501270",
			},
			names: map[int]string{0: "", 1: ""},
		},
		{
			path: "fixtures/manifest",
			num:  1,
			digests: []string{
				"sha256:84afb6189c4d69f2d040c5f1dc4e0a16fed9b539ce9cfb4ac2526ae4e0576cc0",
			},
			names: map[int]string{0: "v0.1.1"},
		},
		{
			path: "fixtures/name_lookups",
			num:  4,
			digests: []string{
				"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
				"sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
				"sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
			},
			names: map[int]string{0: "a", 1: "b", 2: "invalid-mime", 3: "invalid-mime"},
		},
	} {
		results, err := List(test.path)
		require.NoError(t, err)
		require.NotNil(t, results)
		require.Len(t, results, test.num)
		for i, res := range results {
			ociRef, ok := res.Reference.(ociReference)
			require.True(t, ok)
			require.Equal(t, test.digests[i], res.ManifestDescriptor.Digest.String())
			require.Equal(t, test.names[i], ociRef.image)
			if test.names[i] != "" {
				require.True(t, strings.HasSuffix(res.Reference.StringWithinTransport(), ":"+test.names[i]))
				require.Equal(t, -1, ociRef.sourceIndex)
			} else {
				require.Equal(t, i, ociRef.sourceIndex)
			}
			_, err := ParseReference(fmt.Sprintf("%s:@%d", test.path, i))
			require.NoError(t, err)
		}
	}

	_, err := List("fixtures/i_do_not_exist")
	require.Error(t, err)
}

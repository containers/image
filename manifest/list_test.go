package manifest

import (
	"io/ioutil"
	"path/filepath"
	"testing"

	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func isOCI1Index(i interface{}) bool {
	switch i.(type) {
	case *OCI1Index:
		return true
	}
	return false
}

func isSchema2List(i interface{}) bool {
	switch i.(type) {
	case *Schema2List:
		return true
	}
	return false
}

func cloneOCI1Index(i interface{}) List {
	if impl, ok := i.(*OCI1Index); ok {
		return OCI1IndexClone(impl)
	}
	return nil
}

func cloneSchema2List(i interface{}) List {
	if impl, ok := i.(*Schema2List); ok {
		return Schema2ListClone(impl)
	}
	return nil
}

func pare(m List) {
	if impl, ok := m.(*OCI1Index); ok {
		impl.Annotations = nil
	}
	if impl, ok := m.(*Schema2List); ok {
		for i := range impl.Manifests {
			impl.Manifests[i].Platform.Features = nil
		}
	}
	return
}

func TestParseLists(t *testing.T) {
	cases := []struct {
		path      string
		mimeType  string
		checkType (func(interface{}) bool)
		clone     (func(interface{}) List)
	}{
		{"ociv1.image.index.json", imgspecv1.MediaTypeImageIndex, isOCI1Index, cloneOCI1Index},
		{"v2list.manifest.json", DockerV2ListMediaType, isSchema2List, cloneSchema2List},
	}
	for _, c := range cases {
		manifest, err := ioutil.ReadFile(filepath.Join("fixtures", c.path))
		require.NoError(t, err, "error reading file %q", filepath.Join("fixtures", c.path))
		assert.Equal(t, GuessMIMEType(manifest), c.mimeType)

		_, err = FromBlob(manifest, c.mimeType)
		require.Error(t, err, "manifest list %q should not parse as single images", c.path)

		m, err := ListFromBlob(manifest, c.mimeType)
		require.NoError(t, err, "manifest list %q  should parse as list types", c.path)
		assert.True(t, c.checkType(m), "manifest %q is not of the expected implementation type", c.path)
		pare(m)

		clone := c.clone(m)
		assert.Equal(t, clone, m, "manifest %q is missing some fields after being cloned", c.path)

		index, err := m.ToOCI1Index()
		require.NoError(t, err, "error converting %q to an OCI1Index", c.path)

		list, err := m.ToSchema2List()
		require.NoError(t, err, "error converting %q to an Schema2List", c.path)

		index2, err := list.ToOCI1Index()
		assert.Equal(t, index, index2, "index %q lost data in conversion", c.path)

		list2, err := index.ToSchema2List()
		assert.Equal(t, list, list2, "list %q lost data in conversion", c.path)
	}
}

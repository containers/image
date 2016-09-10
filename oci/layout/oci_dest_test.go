package layout

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readerFromFunc allows implementing Reader by any function, e.g. a closure.
type readerFromFunc func([]byte) (int, error)

func (fn readerFromFunc) Read(p []byte) (int, error) {
	return fn(p)
}

// TestPutBlobDigestFailure simulates behavior on digest verification failure.
func TestPutBlobDigestFailure(t *testing.T) {
	const digestErrorString = "Simulated digest error"
	const blobDigest = "sha256:e692418e4cbaf90ca69d05a66403747baa33ee08806650b51fab815ad7fc331f"

	ref, tmpDir := refToTempOCI(t)
	defer os.RemoveAll(tmpDir)
	dirRef, ok := ref.(ociReference)
	require.True(t, ok)
	blobPath, err := dirRef.blobPath(blobDigest)
	assert.NoError(t, err)

	firstRead := true
	reader := readerFromFunc(func(p []byte) (int, error) {
		_, err := os.Lstat(blobPath)
		require.Error(t, err)
		require.True(t, os.IsNotExist(err))
		if firstRead {
			if len(p) > 0 {
				firstRead = false
			}
			for i := 0; i < len(p); i++ {
				p[i] = 0xAA
			}
			return len(p), nil
		}
		return 0, fmt.Errorf(digestErrorString)
	})

	dest, err := ref.NewImageDestination(nil)
	require.NoError(t, err)
	defer dest.Close()
	_, _, err = dest.PutBlob(reader, blobDigest, -1)
	assert.Error(t, err)
	assert.Contains(t, digestErrorString, err.Error())
	err = dest.Commit()
	assert.NoError(t, err)

	_, err = os.Lstat(blobPath)
	require.Error(t, err)
	require.True(t, os.IsNotExist(err))
}

func TestPutManifestUnrecognizedMediaType(t *testing.T) {
	ref, tmpDir := refToTempOCI(t)
	defer os.RemoveAll(tmpDir)
	dirRef, ok := ref.(ociReference)
	require.True(t, ok)

	ociDest, err := dirRef.NewImageDestination(nil)
	require.NoError(t, err)

	m := `{"name":"puerapuliae/busybox","tag":"latest","architecture":"amd64","fsLayers":[{"blobSum":"sha256:04f18047a28f8dea4a3b3872a2ad345cbb6f0eae28d99a60d3df844d6eaae571"},{"blobSum":"sha256:04f18047a28f8dea4a3b3872a2ad345cbb6f0eae28d99a60d3df844d6eaae571"}],"history":[{"v1Compatibility":"{\"id\":\"b46e47334e74d687019107dbec32559dd598db58fe90d2a0c5473bda8b59829d\",\"comment\":\"Imported from -\",\"created\":\"2015-07-03T07:56:02.57018886Z\",\"container_config\":{\"Hostname\":\"\",\"Domainname\":\"\",\"User\":\"\",\"Memory\":0,\"MemorySwap\":0,\"CpuShares\":0,\"Cpuset\":\"\",\"AttachStdin\":false,\"AttachStdout\":false,\"AttachStderr\":false,\"PortSpecs\":null,\"ExposedPorts\":null,\"Tty\":false,\"OpenStdin\":false,\"StdinOnce\":false,\"Env\":null,\"Cmd\":null,\"Image\":\"\",\"Volumes\":null,\"WorkingDir\":\"\",\"Entrypoint\":null,\"NetworkDisabled\":false,\"MacAddress\":\"\",\"OnBuild\":null,\"Labels\":null},\"docker_version\":\"1.6.2\",\"config\":{\"Hostname\":\"\",\"Domainname\":\"\",\"User\":\"\",\"Memory\":0,\"MemorySwap\":0,\"CpuShares\":0,\"Cpuset\":\"\",\"AttachStdin\":false,\"AttachStdout\":false,\"AttachStderr\":false,\"PortSpecs\":null,\"ExposedPorts\":null,\"Tty\":false,\"OpenStdin\":false,\"StdinOnce\":false,\"Env\":null,\"Cmd\":null,\"Image\":\"\",\"Volumes\":null,\"WorkingDir\":\"\",\"Entrypoint\":null,\"NetworkDisabled\":false,\"MacAddress\":\"\",\"OnBuild\":null,\"Labels\":null},\"architecture\":\"amd64\",\"os\":\"linux\",\"Size\":9356886}\n"},{"v1Compatibility":"{\"id\":\"b46e47334e74d687019107dbec32559dd598db58fe90d2a0c5473bda8b59829d\",\"comment\":\"Imported from -\",\"created\":\"2015-07-03T07:56:02.57018886Z\",\"container_config\":{\"Hostname\":\"\",\"Domainname\":\"\",\"User\":\"\",\"Memory\":0,\"MemorySwap\":0,\"CpuShares\":0,\"Cpuset\":\"\",\"AttachStdin\":false,\"AttachStdout\":false,\"AttachStderr\":false,\"PortSpecs\":null,\"ExposedPorts\":null,\"Tty\":false,\"OpenStdin\":false,\"StdinOnce\":false,\"Env\":null,\"Cmd\":null,\"Image\":\"\",\"Volumes\":null,\"WorkingDir\":\"\",\"Entrypoint\":null,\"NetworkDisabled\":false,\"MacAddress\":\"\",\"OnBuild\":null,\"Labels\":null},\"docker_version\":\"1.6.2\",\"config\":{\"Hostname\":\"\",\"Domainname\":\"\",\"User\":\"\",\"Memory\":0,\"MemorySwap\":0,\"CpuShares\":0,\"Cpuset\":\"\",\"AttachStdin\":false,\"AttachStdout\":false,\"AttachStderr\":false,\"PortSpecs\":null,\"ExposedPorts\":null,\"Tty\":false,\"OpenStdin\":false,\"StdinOnce\":false,\"Env\":null,\"Cmd\":null,\"Image\":\"\",\"Volumes\":null,\"WorkingDir\":\"\",\"Entrypoint\":null,\"NetworkDisabled\":false,\"MacAddress\":\"\",\"OnBuild\":null,\"Labels\":null},\"architecture\":\"amd64\",\"os\":\"linux\",\"Size\":9356886}\n"}],"schemaVersion":1,"signatures":[{"header":{"jwk":{"crv":"P-256","kid":"SVJ4:Q6G3:SXTN:H6LT:7PXH:DHUZ:SGTB:5TMV:YPIV:UPHY:MRHO:PN6V","kty":"EC","x":"qrSsA2UAKEFlDhLk12zoWpnHgYcTNfEOWGZU46pzhfk","y":"RtD_vGFtagPlheiunLvZL02LOssnu7DqShuBwc6Ml44"},"alg":"ES256"},"signature":"YzfU_rKQLWqG74uilltTiV3O92lfEjaG5wJkVt_dCtjH_C5AeghfQttnbtceJOyiaU7xP2yEnjdultutsxkQKQ","protected":"eyJmb3JtYXRMZW5ndGgiOjI4NDgsImZvcm1hdFRhaWwiOiJDbjAiLCJ0aW1lIjoiMjAxNi0wOS0xMFQwODoyMDowOFoifQ"}]}`

	err = ociDest.PutManifest([]byte(m))
	require.Error(t, err)
	assert.Contains(t, `Unrecognized manifest media type: "application/vnd.docker.distribution.manifest.v1+prettyjws"`, err.Error())
}

package manifest

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/containers/image/v5/pkg/compression"
	"github.com/containers/image/v5/types"
	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// DiffID values corresponding to layers of schema2-to-schema1-by-docker.json
var schema1FixtureLayerDiffIDs = []digest.Digest{
	"sha256:142a601d97936307e75220c35dde0348971a9584c21e7cb42e1f7004005432ab",
	"sha256:90fcc66ad3be9f1757f954b750deb37032f208428aa12599fcb02182b9065a9c",
	"sha256:5a8624bb7e76d1e6829f9c64c43185e02bc07f97a2189eb048609a8914e72c56",
	"sha256:d349ff6b3afc6a2800054768c82bfbf4289c9aa5da55c1290f802943dcd4d1e9",
	"sha256:8c064bb1f60e84fa8cc6079b6d2e76e0423389fd6aeb7e497dfdae5e05b2b25b",
}

func manifestSchema1FromFixture(t *testing.T, fixture string) *Schema1 {
	manifest, err := os.ReadFile(filepath.Join("fixtures", fixture))
	require.NoError(t, err)

	m, err := Schema1FromManifest(manifest)
	require.NoError(t, err)
	return m
}

func TestSchema1FromManifest(t *testing.T) {
	validManifest, err := os.ReadFile(filepath.Join("fixtures", "schema2-to-schema1-by-docker.json"))
	require.NoError(t, err)

	// Invalid manifest version is rejected
	m, err := Schema1FromManifest(validManifest)
	require.NoError(t, err)
	m.SchemaVersion = 2
	manifest, err := m.Serialize()
	require.NoError(t, err)
	_, err = Schema1FromManifest(manifest)
	assert.Error(t, err)

	parser := func(m []byte) error {
		_, err := Schema1FromManifest(m)
		return err
	}
	// Schema mismatch is rejected
	testManifestFixturesAreRejected(t, parser, []string{
		"v2s2.manifest.json", "v2list.manifest.json",
		"ociv1.manifest.json", "ociv1.image.index.json",
	})
	// Extra fields are rejected
	testValidManifestWithExtraFieldsIsRejected(t, parser, validManifest, []string{"config", "layers", "manifests"})
}

func TestSchema1Initialize(t *testing.T) {
	// Test this indirectly via Schema1FromComponents; otherwise we would have to break the API and create an instance manually.

	// FIXME: this should eventually share a fixture with the other parsing tests.
	fsLayers := []Schema1FSLayers{
		{BlobSum: "sha256:e623934bca8d1a74f51014256445937714481e49343a31bda2bc5f534748184d"},
		{BlobSum: "sha256:62e48e39dc5b30b75a97f05bccc66efbae6058b860ee20a5c9a184b9d5e25788"},
		{BlobSum: "sha256:9e92df2aea7dc0baf5f1f8d509678d6a6306de27ad06513f8e218371938c07a6"},
		{BlobSum: "sha256:f576d102e09b9eef0e305aaef705d2d43a11bebc3fd5810a761624bd5e11997e"},
		{BlobSum: "sha256:4aa565ad8b7a87248163ce7dba1dd3894821aac97e846b932ff6b8ef9a8a508a"},
		{BlobSum: "sha256:9cadd93b16ff2a0c51ac967ea2abfadfac50cfa3af8b5bf983d89b8f8647f3e4"},
	}
	history := []Schema1History{
		{V1Compatibility: "{\"architecture\":\"amd64\",\"config\":{\"Hostname\":\"9428cdea83ba\",\"Domainname\":\"\",\"User\":\"nova\",\"AttachStdin\":false,\"AttachStdout\":false,\"AttachStderr\":false,\"Tty\":false,\"OpenStdin\":false,\"StdinOnce\":false,\"Env\":[\"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin\",\"container=oci\",\"KOLLA_BASE_DISTRO=rhel\",\"KOLLA_INSTALL_TYPE=binary\",\"KOLLA_INSTALL_METATYPE=rhos\",\"PS1=$(tput bold)($(printenv KOLLA_SERVICE_NAME))$(tput sgr0)[$(id -un)@$(hostname -s) $(pwd)]$ \"],\"Cmd\":[\"kolla_start\"],\"Healthcheck\":{\"Test\":[\"CMD-SHELL\",\"/openstack/healthcheck\"]},\"ArgsEscaped\":true,\"Image\":\"3bf9afe371220b1eb1c57bec39b5a99ba976c36c92d964a1c014584f95f51e33\",\"Volumes\":null,\"WorkingDir\":\"\",\"Entrypoint\":null,\"OnBuild\":[],\"Labels\":{\"Kolla-SHA\":\"5.0.0-39-g6f1b947b\",\"architecture\":\"x86_64\",\"authoritative-source-url\":\"registry.access.redhat.com\",\"build-date\":\"2018-01-25T00:32:27.807261\",\"com.redhat.build-host\":\"ip-10-29-120-186.ec2.internal\",\"com.redhat.component\":\"openstack-nova-api-docker\",\"description\":\"Red Hat OpenStack Platform 12.0 nova-api\",\"distribution-scope\":\"public\",\"io.k8s.description\":\"Red Hat OpenStack Platform 12.0 nova-api\",\"io.k8s.display-name\":\"Red Hat OpenStack Platform 12.0 nova-api\",\"io.openshift.tags\":\"rhosp osp openstack osp-12.0\",\"kolla_version\":\"stable/pike\",\"name\":\"rhosp12/openstack-nova-api\",\"release\":\"20180124.1\",\"summary\":\"Red Hat OpenStack Platform 12.0 nova-api\",\"tripleo-common_version\":\"7.6.3-23-g4891cfe\",\"url\":\"https://access.redhat.com/containers/#/registry.access.redhat.com/rhosp12/openstack-nova-api/images/12.0-20180124.1\",\"vcs-ref\":\"9b31243b7b448eb2fc3b6e2c96935b948f806e98\",\"vcs-type\":\"git\",\"vendor\":\"Red Hat, Inc.\",\"version\":\"12.0\",\"version-release\":\"12.0-20180124.1\"}},\"container_config\":{\"Hostname\":\"9428cdea83ba\",\"Domainname\":\"\",\"User\":\"nova\",\"AttachStdin\":false,\"AttachStdout\":false,\"AttachStderr\":false,\"Tty\":false,\"OpenStdin\":false,\"StdinOnce\":false,\"Env\":[\"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin\",\"container=oci\",\"KOLLA_BASE_DISTRO=rhel\",\"KOLLA_INSTALL_TYPE=binary\",\"KOLLA_INSTALL_METATYPE=rhos\",\"PS1=$(tput bold)($(printenv KOLLA_SERVICE_NAME))$(tput sgr0)[$(id -un)@$(hostname -s) $(pwd)]$ \"],\"Cmd\":[\"/bin/sh\",\"-c\",\"#(nop) \",\"USER [nova]\"],\"Healthcheck\":{\"Test\":[\"CMD-SHELL\",\"/openstack/healthcheck\"]},\"ArgsEscaped\":true,\"Image\":\"sha256:274ce4dcbeb09fa173a5d50203ae5cec28f456d1b8b59477b47a42bd74d068bf\",\"Volumes\":null,\"WorkingDir\":\"\",\"Entrypoint\":null,\"OnBuild\":[],\"Labels\":{\"Kolla-SHA\":\"5.0.0-39-g6f1b947b\",\"architecture\":\"x86_64\",\"authoritative-source-url\":\"registry.access.redhat.com\",\"build-date\":\"2018-01-25T00:32:27.807261\",\"com.redhat.build-host\":\"ip-10-29-120-186.ec2.internal\",\"com.redhat.component\":\"openstack-nova-api-docker\",\"description\":\"Red Hat OpenStack Platform 12.0 nova-api\",\"distribution-scope\":\"public\",\"io.k8s.description\":\"Red Hat OpenStack Platform 12.0 nova-api\",\"io.k8s.display-name\":\"Red Hat OpenStack Platform 12.0 nova-api\",\"io.openshift.tags\":\"rhosp osp openstack osp-12.0\",\"kolla_version\":\"stable/pike\",\"name\":\"rhosp12/openstack-nova-api\",\"release\":\"20180124.1\",\"summary\":\"Red Hat OpenStack Platform 12.0 nova-api\",\"tripleo-common_version\":\"7.6.3-23-g4891cfe\",\"url\":\"https://access.redhat.com/containers/#/registry.access.redhat.com/rhosp12/openstack-nova-api/images/12.0-20180124.1\",\"vcs-ref\":\"9b31243b7b448eb2fc3b6e2c96935b948f806e98\",\"vcs-type\":\"git\",\"vendor\":\"Red Hat, Inc.\",\"version\":\"12.0\",\"version-release\":\"12.0-20180124.1\"}},\"created\":\"2018-01-25T00:37:48.268558Z\",\"docker_version\":\"1.12.6\",\"id\":\"486cbbaf6c6f7d890f9368c86eda3f4ebe3ae982b75098037eb3c3cc6f0e0cdf\",\"os\":\"linux\",\"parent\":\"20d0c9c79f9fee83c4094993335b9b321112f13eef60ed9ec1599c7593dccf20\"}"},
		{V1Compatibility: "{\"id\":\"20d0c9c79f9fee83c4094993335b9b321112f13eef60ed9ec1599c7593dccf20\",\"parent\":\"47a1014db2116c312736e11adcc236fb77d0ad32457f959cbaec0c3fc9ab1caa\",\"created\":\"2018-01-24T23:08:25.300741Z\",\"container_config\":{\"Cmd\":[\"/bin/sh -c rm -f '/etc/yum.repos.d/rhel-7.4.repo' '/etc/yum.repos.d/rhos-optools-12.0.repo' '/etc/yum.repos.d/rhos-12.0-container-yum-need_images.repo'\"]}}"},
		{V1Compatibility: "{\"id\":\"47a1014db2116c312736e11adcc236fb77d0ad32457f959cbaec0c3fc9ab1caa\",\"parent\":\"cec66cab6c92a5f7b50ef407b80b83840a0d089b9896257609fd01de3a595824\",\"created\":\"2018-01-24T22:00:57.807862Z\",\"container_config\":{\"Cmd\":[\"/bin/sh -c rm -f '/etc/yum.repos.d/rhel-7.4.repo' '/etc/yum.repos.d/rhos-optools-12.0.repo' '/etc/yum.repos.d/rhos-12.0-container-yum-need_images.repo'\"]}}"},
		{V1Compatibility: "{\"id\":\"cec66cab6c92a5f7b50ef407b80b83840a0d089b9896257609fd01de3a595824\",\"parent\":\"0e7730eccb3d014b33147b745d771bc0e38a967fd932133a6f5325a3c84282e2\",\"created\":\"2018-01-24T21:40:32.494686Z\",\"container_config\":{\"Cmd\":[\"/bin/sh -c rm -f '/etc/yum.repos.d/rhel-7.4.repo' '/etc/yum.repos.d/rhos-optools-12.0.repo' '/etc/yum.repos.d/rhos-12.0-container-yum-need_images.repo'\"]}}"},
		{V1Compatibility: "{\"id\":\"0e7730eccb3d014b33147b745d771bc0e38a967fd932133a6f5325a3c84282e2\",\"parent\":\"3e49094c0233214ab73f8e5c204af8a14cfc6f0403384553c17fbac2e9d38345\",\"created\":\"2017-11-21T16:49:37.292899Z\",\"container_config\":{\"Cmd\":[\"/bin/sh -c rm -f '/etc/yum.repos.d/compose-rpms-1.repo'\"]},\"author\":\"Red Hat, Inc.\"}"},
		{V1Compatibility: "{\"id\":\"3e49094c0233214ab73f8e5c204af8a14cfc6f0403384553c17fbac2e9d38345\",\"comment\":\"Imported from -\",\"created\":\"2017-11-21T16:47:27.755341705Z\",\"container_config\":{\"Cmd\":[\"\"]}}"},
	}

	// Valid input
	m, err := Schema1FromComponents(nil, fsLayers, history, "amd64")
	assert.NoError(t, err)
	assert.Equal(t, []Schema1V1Compatibility{
		{
			ID:      "486cbbaf6c6f7d890f9368c86eda3f4ebe3ae982b75098037eb3c3cc6f0e0cdf",
			Parent:  "20d0c9c79f9fee83c4094993335b9b321112f13eef60ed9ec1599c7593dccf20",
			Created: time.Date(2018, 1, 25, 0, 37, 48, 268558000, time.UTC),
			ContainerConfig: schema1V1CompatibilityContainerConfig{
				Cmd: []string{"/bin/sh", "-c", "#(nop) ", "USER [nova]"},
			},
			ThrowAway: false,
		},
		{
			ID:      "20d0c9c79f9fee83c4094993335b9b321112f13eef60ed9ec1599c7593dccf20",
			Parent:  "47a1014db2116c312736e11adcc236fb77d0ad32457f959cbaec0c3fc9ab1caa",
			Created: time.Date(2018, 1, 24, 23, 8, 25, 300741000, time.UTC),
			ContainerConfig: schema1V1CompatibilityContainerConfig{
				Cmd: []string{"/bin/sh -c rm -f '/etc/yum.repos.d/rhel-7.4.repo' '/etc/yum.repos.d/rhos-optools-12.0.repo' '/etc/yum.repos.d/rhos-12.0-container-yum-need_images.repo'"},
			},
			ThrowAway: false,
		},
		{
			ID:      "47a1014db2116c312736e11adcc236fb77d0ad32457f959cbaec0c3fc9ab1caa",
			Parent:  "cec66cab6c92a5f7b50ef407b80b83840a0d089b9896257609fd01de3a595824",
			Created: time.Date(2018, 1, 24, 22, 0, 57, 807862000, time.UTC),
			ContainerConfig: schema1V1CompatibilityContainerConfig{
				Cmd: []string{"/bin/sh -c rm -f '/etc/yum.repos.d/rhel-7.4.repo' '/etc/yum.repos.d/rhos-optools-12.0.repo' '/etc/yum.repos.d/rhos-12.0-container-yum-need_images.repo'"},
			},
			ThrowAway: false,
		},
		{
			ID:      "cec66cab6c92a5f7b50ef407b80b83840a0d089b9896257609fd01de3a595824",
			Parent:  "0e7730eccb3d014b33147b745d771bc0e38a967fd932133a6f5325a3c84282e2",
			Created: time.Date(2018, 1, 24, 21, 40, 32, 494686000, time.UTC),
			ContainerConfig: schema1V1CompatibilityContainerConfig{
				Cmd: []string{"/bin/sh -c rm -f '/etc/yum.repos.d/rhel-7.4.repo' '/etc/yum.repos.d/rhos-optools-12.0.repo' '/etc/yum.repos.d/rhos-12.0-container-yum-need_images.repo'"},
			},
			ThrowAway: false,
		},
		{
			ID:      "0e7730eccb3d014b33147b745d771bc0e38a967fd932133a6f5325a3c84282e2",
			Parent:  "3e49094c0233214ab73f8e5c204af8a14cfc6f0403384553c17fbac2e9d38345",
			Created: time.Date(2017, 11, 21, 16, 49, 37, 292899000, time.UTC),
			ContainerConfig: schema1V1CompatibilityContainerConfig{
				Cmd: []string{"/bin/sh -c rm -f '/etc/yum.repos.d/compose-rpms-1.repo'"},
			},
			Author:    "Red Hat, Inc.",
			ThrowAway: false,
		},
		{
			ID:      "3e49094c0233214ab73f8e5c204af8a14cfc6f0403384553c17fbac2e9d38345",
			Comment: "Imported from -",
			Created: time.Date(2017, 11, 21, 16, 47, 27, 755341705, time.UTC),
			ContainerConfig: schema1V1CompatibilityContainerConfig{
				Cmd: []string{""},
			},
			ThrowAway: false,
		},
	}, m.ExtractedV1Compatibility)

	// Layer and history length mismatch
	_, err = Schema1FromComponents(nil, fsLayers, history[1:], "amd64")
	assert.Error(t, err)

	// No layers/history
	_, err = Schema1FromComponents(nil, []Schema1FSLayers{}, []Schema1History{}, "amd64")
	assert.Error(t, err)

	// Invalid history JSON
	_, err = Schema1FromComponents(nil,
		[]Schema1FSLayers{{BlobSum: "sha256:e623934bca8d1a74f51014256445937714481e49343a31bda2bc5f534748184d"}},
		[]Schema1History{{V1Compatibility: "-"}},
		"amd64")
	assert.Error(t, err)
}

func TestSchema1LayerInfos(t *testing.T) {
	// We use this instead of original schema1 manifests, because those, surprisingly,
	// seem not to set the "throwaway" flag.
	m := manifestSchema1FromFixture(t, "schema2-to-schema1-by-docker.json") // FIXME: Test also Schema1FromComponents
	assert.Equal(t, []LayerInfo{
		{BlobInfo: types.BlobInfo{Digest: "sha256:6a5a5368e0c2d3e5909184fa28ddfd56072e7ff3ee9a945876f7eee5896ef5bb", Size: -1}, EmptyLayer: false},
		{BlobInfo: types.BlobInfo{Digest: "sha256:a3ed95caeb02ffe68cdd9fd84406680ae93d633cb16422d00e8a7c22955b46d4", Size: -1}, EmptyLayer: true},
		{BlobInfo: types.BlobInfo{Digest: "sha256:a3ed95caeb02ffe68cdd9fd84406680ae93d633cb16422d00e8a7c22955b46d4", Size: -1}, EmptyLayer: true},
		{BlobInfo: types.BlobInfo{Digest: "sha256:a3ed95caeb02ffe68cdd9fd84406680ae93d633cb16422d00e8a7c22955b46d4", Size: -1}, EmptyLayer: true},
		{BlobInfo: types.BlobInfo{Digest: "sha256:1bbf5d58d24c47512e234a5623474acf65ae00d4d1414272a893204f44cc680c", Size: -1}, EmptyLayer: false},
		{BlobInfo: types.BlobInfo{Digest: "sha256:a3ed95caeb02ffe68cdd9fd84406680ae93d633cb16422d00e8a7c22955b46d4", Size: -1}, EmptyLayer: true},
		{BlobInfo: types.BlobInfo{Digest: "sha256:8f5dc8a4b12c307ac84de90cdd9a7f3915d1be04c9388868ca118831099c67a9", Size: -1}, EmptyLayer: false},
		{BlobInfo: types.BlobInfo{Digest: "sha256:a3ed95caeb02ffe68cdd9fd84406680ae93d633cb16422d00e8a7c22955b46d4", Size: -1}, EmptyLayer: true},
		{BlobInfo: types.BlobInfo{Digest: "sha256:a3ed95caeb02ffe68cdd9fd84406680ae93d633cb16422d00e8a7c22955b46d4", Size: -1}, EmptyLayer: true},
		{BlobInfo: types.BlobInfo{Digest: "sha256:a3ed95caeb02ffe68cdd9fd84406680ae93d633cb16422d00e8a7c22955b46d4", Size: -1}, EmptyLayer: true},
		{BlobInfo: types.BlobInfo{Digest: "sha256:a3ed95caeb02ffe68cdd9fd84406680ae93d633cb16422d00e8a7c22955b46d4", Size: -1}, EmptyLayer: true},
		{BlobInfo: types.BlobInfo{Digest: "sha256:bbd6b22eb11afce63cc76f6bc41042d99f10d6024c96b655dafba930b8d25909", Size: -1}, EmptyLayer: false},
		{BlobInfo: types.BlobInfo{Digest: "sha256:960e52ecf8200cbd84e70eb2ad8678f4367e50d14357021872c10fa3fc5935fa", Size: -1}, EmptyLayer: false},
		{BlobInfo: types.BlobInfo{Digest: "sha256:a3ed95caeb02ffe68cdd9fd84406680ae93d633cb16422d00e8a7c22955b46d4", Size: -1}, EmptyLayer: true},
		{BlobInfo: types.BlobInfo{Digest: "sha256:a3ed95caeb02ffe68cdd9fd84406680ae93d633cb16422d00e8a7c22955b46d4", Size: -1}, EmptyLayer: true},
	}, m.LayerInfos())
}

func TestSchema1UpdateLayerInfos(t *testing.T) {
	for _, c := range []struct {
		name            string
		sourceFixture   string
		updates         []types.BlobInfo
		expectedFixture string // or "" to indicate an expected failure
	}{
		// Many more tests cases could be added here
		{
			name:          "uncompressed → gzip encrypted",
			sourceFixture: "v2s1.manifest.json",
			updates: []types.BlobInfo{
				{
					Digest:               "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
					Size:                 32654,
					Annotations:          map[string]string{"org.opencontainers.image.enc.…": "layer1"},
					CompressionOperation: types.Compress,
					CompressionAlgorithm: &compression.Gzip,
					CryptoOperation:      types.Encrypt,
				},
				{
					Digest:               "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
					Size:                 16724,
					Annotations:          map[string]string{"org.opencontainers.image.enc.…": "layer2"},
					CompressionOperation: types.Compress,
					CompressionAlgorithm: &compression.Gzip,
					CryptoOperation:      types.Encrypt,
				},
				{
					Digest:               "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
					Size:                 73109,
					Annotations:          map[string]string{"org.opencontainers.image.enc.…": "layer2"},
					CompressionOperation: types.Compress,
					CompressionAlgorithm: &compression.Gzip,
					CryptoOperation:      types.Encrypt,
				},
			},
			expectedFixture: "", // Encryption is not supported
		},
		{
			name:          "gzip  → uncompressed decrypted", // We can’t represent encrypted images anyway, but verify that we reject decryption attempts.
			sourceFixture: "v2s1.manifest.json",
			updates: []types.BlobInfo{
				{
					Digest:               "sha256:e692418e4cbaf90ca69d05a66403747baa33ee08806650b51fab815ad7fc331f",
					Size:                 32654,
					CompressionOperation: types.Decompress,
					CryptoOperation:      types.Decrypt,
				},
				{
					Digest:               "sha256:3c3a4604a545cdc127456d94e421cd355bca5b528f4a9c1905b15da2eb4a4c6b",
					Size:                 16724,
					CompressionOperation: types.Decompress,
					CryptoOperation:      types.Decrypt,
				},
				{
					Digest:               "sha256:ec4b8955958665577945c89419d1af06b5f7636b4ac3da7f12184802ad867736",
					Size:                 73109,
					CompressionOperation: types.Decompress,
					CryptoOperation:      types.Decrypt,
				},
			},
			expectedFixture: "", // Decryption is not supported
		},
	} {
		manifest := manifestSchema1FromFixture(t, c.sourceFixture)

		err := manifest.UpdateLayerInfos(c.updates)
		if c.expectedFixture == "" {
			assert.Error(t, err, c.name)
		} else {
			require.NoError(t, err, c.name)

			updatedManifestBytes, err := manifest.Serialize()
			require.NoError(t, err, c.name)

			expectedManifest := manifestSchema1FromFixture(t, c.expectedFixture)
			expectedManifestBytes, err := expectedManifest.Serialize()
			require.NoError(t, err, c.name)

			assert.Equal(t, string(expectedManifestBytes), string(updatedManifestBytes), c.name)
		}
	}
}

func TestSchema1ImageID(t *testing.T) {
	m := manifestSchema1FromFixture(t, "schema2-to-schema1-by-docker.json")
	id, err := m.ImageID(schema1FixtureLayerDiffIDs)
	require.NoError(t, err)
	// NOTE: This value is dependent on the Schema1.ToSchema2Config implementation, and not necessarily stable over time.
	// This is mostly a smoke-test; it’s fine to just update this value if that implementation changes.
	assert.Equal(t, "9ca4bda0a6b3727a6ffcc43e981cad0f24e2ec79d338f6ba325b4dfd0756fb8f", id)
}

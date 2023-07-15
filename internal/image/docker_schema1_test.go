package image

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/types"
	digest "github.com/opencontainers/go-digest"
	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var schema1FixtureLayerInfos = []types.BlobInfo{
	{
		MediaType: "application/vnd.docker.image.rootfs.diff.tar.gzip",
		Size:      74876245,
		Digest:    "sha256:9cadd93b16ff2a0c51ac967ea2abfadfac50cfa3af8b5bf983d89b8f8647f3e4",
	},
	{
		MediaType: "application/vnd.docker.image.rootfs.diff.tar.gzip",
		Size:      1239,
		Digest:    "sha256:4aa565ad8b7a87248163ce7dba1dd3894821aac97e846b932ff6b8ef9a8a508a",
	},
	{
		MediaType: "application/vnd.docker.image.rootfs.diff.tar.gzip",
		Size:      78339724,
		Digest:    "sha256:f576d102e09b9eef0e305aaef705d2d43a11bebc3fd5810a761624bd5e11997e",
	},
	{
		MediaType: "application/vnd.docker.image.rootfs.diff.tar.gzip",
		Size:      76857203,
		Digest:    "sha256:9e92df2aea7dc0baf5f1f8d509678d6a6306de27ad06513f8e218371938c07a6",
	},
	{
		MediaType: "application/vnd.docker.image.rootfs.diff.tar.gzip",
		Size:      25923380,
		Digest:    "sha256:62e48e39dc5b30b75a97f05bccc66efbae6058b860ee20a5c9a184b9d5e25788",
	},
	{
		MediaType: "application/vnd.docker.image.rootfs.diff.tar.gzip",
		Size:      23511300,
		Digest:    "sha256:e623934bca8d1a74f51014256445937714481e49343a31bda2bc5f534748184d",
	},
}

var schema1FixtureLayerDiffIDs = []digest.Digest{
	"sha256:e1d829eddb62dc49f1c56dbf8acd0c71299b3996115399de853a9d66d81b822f",
	"sha256:02404b4d7e5d89b1383ca346b4462b199128aa4b238c5a2b2c186004ac148ba8",
	"sha256:45fad80a4b1cec165c421eb570dec312d825bd8fac362e255028fa3f2169148d",
	"sha256:7ddef8efd44586e54880ec4797458eac87b368544c438d7e7c63fbc0d9a7ae97",
	"sha256:b56b16b6407ba1b86252e7e50f98f142cf6844fab42e4495d56ebb7ce559e2af",
	"sha256:9bd63850e406167b4751f5050f6dc0ebd789bb5ef5e5c6c31ed062bda8c063e8",
}

var schema1WithThrowawaysFixtureLayerInfos = []types.BlobInfo{
	{Digest: "sha256:6a5a5368e0c2d3e5909184fa28ddfd56072e7ff3ee9a945876f7eee5896ef5bb", Size: 51354364},
	{Digest: GzippedEmptyLayerDigest, Size: int64(len(GzippedEmptyLayer))},
	{Digest: GzippedEmptyLayerDigest, Size: int64(len(GzippedEmptyLayer))},
	{Digest: GzippedEmptyLayerDigest, Size: int64(len(GzippedEmptyLayer))},
	{Digest: "sha256:1bbf5d58d24c47512e234a5623474acf65ae00d4d1414272a893204f44cc680c", Size: 150},
	{Digest: GzippedEmptyLayerDigest, Size: int64(len(GzippedEmptyLayer))},
	{Digest: "sha256:8f5dc8a4b12c307ac84de90cdd9a7f3915d1be04c9388868ca118831099c67a9", Size: 11739507},
	{Digest: GzippedEmptyLayerDigest, Size: int64(len(GzippedEmptyLayer))},
	{Digest: GzippedEmptyLayerDigest, Size: int64(len(GzippedEmptyLayer))},
	{Digest: GzippedEmptyLayerDigest, Size: int64(len(GzippedEmptyLayer))},
	{Digest: GzippedEmptyLayerDigest, Size: int64(len(GzippedEmptyLayer))},
	{Digest: "sha256:bbd6b22eb11afce63cc76f6bc41042d99f10d6024c96b655dafba930b8d25909", Size: 8841833},
	{Digest: "sha256:960e52ecf8200cbd84e70eb2ad8678f4367e50d14357021872c10fa3fc5935fa", Size: 291},
	{Digest: GzippedEmptyLayerDigest, Size: int64(len(GzippedEmptyLayer))},
	{Digest: GzippedEmptyLayerDigest, Size: int64(len(GzippedEmptyLayer))},
}

var schema1WithThrowawaysFixtureLayerDiffIDs = []digest.Digest{
	"sha256:142a601d97936307e75220c35dde0348971a9584c21e7cb42e1f7004005432ab",
	GzippedEmptyLayerDigest,
	GzippedEmptyLayerDigest,
	GzippedEmptyLayerDigest,
	"sha256:90fcc66ad3be9f1757f954b750deb37032f208428aa12599fcb02182b9065a9c",
	GzippedEmptyLayerDigest,
	"sha256:5a8624bb7e76d1e6829f9c64c43185e02bc07f97a2189eb048609a8914e72c56",
	GzippedEmptyLayerDigest,
	GzippedEmptyLayerDigest,
	GzippedEmptyLayerDigest,
	GzippedEmptyLayerDigest,
	"sha256:d349ff6b3afc6a2800054768c82bfbf4289c9aa5da55c1290f802943dcd4d1e9",
	"sha256:8c064bb1f60e84fa8cc6079b6d2e76e0423389fd6aeb7e497dfdae5e05b2b25b",
	GzippedEmptyLayerDigest,
	GzippedEmptyLayerDigest,
}

func manifestSchema1FromFixture(t *testing.T, fixture string) genericManifest {
	manifest, err := os.ReadFile(filepath.Join("fixtures", fixture))
	require.NoError(t, err)

	m, err := manifestSchema1FromManifest(manifest)
	require.NoError(t, err)
	return m
}

func manifestSchema1FromComponentsLikeFixture(t *testing.T) genericManifest {
	ref, err := reference.ParseNormalizedNamed("rhosp12/openstack-nova-api:latest")
	require.NoError(t, err)
	m, err := manifestSchema1FromComponents(ref, []manifest.Schema1FSLayers{
		{BlobSum: "sha256:e623934bca8d1a74f51014256445937714481e49343a31bda2bc5f534748184d"},
		{BlobSum: "sha256:62e48e39dc5b30b75a97f05bccc66efbae6058b860ee20a5c9a184b9d5e25788"},
		{BlobSum: "sha256:9e92df2aea7dc0baf5f1f8d509678d6a6306de27ad06513f8e218371938c07a6"},
		{BlobSum: "sha256:f576d102e09b9eef0e305aaef705d2d43a11bebc3fd5810a761624bd5e11997e"},
		{BlobSum: "sha256:4aa565ad8b7a87248163ce7dba1dd3894821aac97e846b932ff6b8ef9a8a508a"},
		{BlobSum: "sha256:9cadd93b16ff2a0c51ac967ea2abfadfac50cfa3af8b5bf983d89b8f8647f3e4"},
	}, []manifest.Schema1History{
		{V1Compatibility: "{\"architecture\":\"amd64\",\"config\":{\"Hostname\":\"9428cdea83ba\",\"Domainname\":\"\",\"User\":\"nova\",\"AttachStdin\":false,\"AttachStdout\":false,\"AttachStderr\":false,\"Tty\":false,\"OpenStdin\":false,\"StdinOnce\":false,\"Env\":[\"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin\",\"container=oci\",\"KOLLA_BASE_DISTRO=rhel\",\"KOLLA_INSTALL_TYPE=binary\",\"KOLLA_INSTALL_METATYPE=rhos\",\"PS1=$(tput bold)($(printenv KOLLA_SERVICE_NAME))$(tput sgr0)[$(id -un)@$(hostname -s) $(pwd)]$ \"],\"Cmd\":[\"kolla_start\"],\"Healthcheck\":{\"Test\":[\"CMD-SHELL\",\"/openstack/healthcheck\"]},\"ArgsEscaped\":true,\"Image\":\"3bf9afe371220b1eb1c57bec39b5a99ba976c36c92d964a1c014584f95f51e33\",\"Volumes\":null,\"WorkingDir\":\"\",\"Entrypoint\":null,\"OnBuild\":[],\"Labels\":{\"Kolla-SHA\":\"5.0.0-39-g6f1b947b\",\"architecture\":\"x86_64\",\"authoritative-source-url\":\"registry.access.redhat.com\",\"build-date\":\"2018-01-25T00:32:27.807261\",\"com.redhat.build-host\":\"ip-10-29-120-186.ec2.internal\",\"com.redhat.component\":\"openstack-nova-api-docker\",\"description\":\"Red Hat OpenStack Platform 12.0 nova-api\",\"distribution-scope\":\"public\",\"io.k8s.description\":\"Red Hat OpenStack Platform 12.0 nova-api\",\"io.k8s.display-name\":\"Red Hat OpenStack Platform 12.0 nova-api\",\"io.openshift.tags\":\"rhosp osp openstack osp-12.0\",\"kolla_version\":\"stable/pike\",\"name\":\"rhosp12/openstack-nova-api\",\"release\":\"20180124.1\",\"summary\":\"Red Hat OpenStack Platform 12.0 nova-api\",\"tripleo-common_version\":\"7.6.3-23-g4891cfe\",\"url\":\"https://access.redhat.com/containers/#/registry.access.redhat.com/rhosp12/openstack-nova-api/images/12.0-20180124.1\",\"vcs-ref\":\"9b31243b7b448eb2fc3b6e2c96935b948f806e98\",\"vcs-type\":\"git\",\"vendor\":\"Red Hat, Inc.\",\"version\":\"12.0\",\"version-release\":\"12.0-20180124.1\"}},\"container_config\":{\"Hostname\":\"9428cdea83ba\",\"Domainname\":\"\",\"User\":\"nova\",\"AttachStdin\":false,\"AttachStdout\":false,\"AttachStderr\":false,\"Tty\":false,\"OpenStdin\":false,\"StdinOnce\":false,\"Env\":[\"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin\",\"container=oci\",\"KOLLA_BASE_DISTRO=rhel\",\"KOLLA_INSTALL_TYPE=binary\",\"KOLLA_INSTALL_METATYPE=rhos\",\"PS1=$(tput bold)($(printenv KOLLA_SERVICE_NAME))$(tput sgr0)[$(id -un)@$(hostname -s) $(pwd)]$ \"],\"Cmd\":[\"/bin/sh\",\"-c\",\"#(nop) \",\"USER [nova]\"],\"Healthcheck\":{\"Test\":[\"CMD-SHELL\",\"/openstack/healthcheck\"]},\"ArgsEscaped\":true,\"Image\":\"sha256:274ce4dcbeb09fa173a5d50203ae5cec28f456d1b8b59477b47a42bd74d068bf\",\"Volumes\":null,\"WorkingDir\":\"\",\"Entrypoint\":null,\"OnBuild\":[],\"Labels\":{\"Kolla-SHA\":\"5.0.0-39-g6f1b947b\",\"architecture\":\"x86_64\",\"authoritative-source-url\":\"registry.access.redhat.com\",\"build-date\":\"2018-01-25T00:32:27.807261\",\"com.redhat.build-host\":\"ip-10-29-120-186.ec2.internal\",\"com.redhat.component\":\"openstack-nova-api-docker\",\"description\":\"Red Hat OpenStack Platform 12.0 nova-api\",\"distribution-scope\":\"public\",\"io.k8s.description\":\"Red Hat OpenStack Platform 12.0 nova-api\",\"io.k8s.display-name\":\"Red Hat OpenStack Platform 12.0 nova-api\",\"io.openshift.tags\":\"rhosp osp openstack osp-12.0\",\"kolla_version\":\"stable/pike\",\"name\":\"rhosp12/openstack-nova-api\",\"release\":\"20180124.1\",\"summary\":\"Red Hat OpenStack Platform 12.0 nova-api\",\"tripleo-common_version\":\"7.6.3-23-g4891cfe\",\"url\":\"https://access.redhat.com/containers/#/registry.access.redhat.com/rhosp12/openstack-nova-api/images/12.0-20180124.1\",\"vcs-ref\":\"9b31243b7b448eb2fc3b6e2c96935b948f806e98\",\"vcs-type\":\"git\",\"vendor\":\"Red Hat, Inc.\",\"version\":\"12.0\",\"version-release\":\"12.0-20180124.1\"}},\"created\":\"2018-01-25T00:37:48.268558Z\",\"docker_version\":\"1.12.6\",\"id\":\"486cbbaf6c6f7d890f9368c86eda3f4ebe3ae982b75098037eb3c3cc6f0e0cdf\",\"os\":\"linux\",\"parent\":\"20d0c9c79f9fee83c4094993335b9b321112f13eef60ed9ec1599c7593dccf20\"}"},
		{V1Compatibility: "{\"id\":\"20d0c9c79f9fee83c4094993335b9b321112f13eef60ed9ec1599c7593dccf20\",\"parent\":\"47a1014db2116c312736e11adcc236fb77d0ad32457f959cbaec0c3fc9ab1caa\",\"created\":\"2018-01-24T23:08:25.300741Z\",\"container_config\":{\"Cmd\":[\"/bin/sh -c rm -f '/etc/yum.repos.d/rhel-7.4.repo' '/etc/yum.repos.d/rhos-optools-12.0.repo' '/etc/yum.repos.d/rhos-12.0-container-yum-need_images.repo'\"]}}"},
		{V1Compatibility: "{\"id\":\"47a1014db2116c312736e11adcc236fb77d0ad32457f959cbaec0c3fc9ab1caa\",\"parent\":\"cec66cab6c92a5f7b50ef407b80b83840a0d089b9896257609fd01de3a595824\",\"created\":\"2018-01-24T22:00:57.807862Z\",\"container_config\":{\"Cmd\":[\"/bin/sh -c rm -f '/etc/yum.repos.d/rhel-7.4.repo' '/etc/yum.repos.d/rhos-optools-12.0.repo' '/etc/yum.repos.d/rhos-12.0-container-yum-need_images.repo'\"]}}"},
		{V1Compatibility: "{\"id\":\"cec66cab6c92a5f7b50ef407b80b83840a0d089b9896257609fd01de3a595824\",\"parent\":\"0e7730eccb3d014b33147b745d771bc0e38a967fd932133a6f5325a3c84282e2\",\"created\":\"2018-01-24T21:40:32.494686Z\",\"container_config\":{\"Cmd\":[\"/bin/sh -c rm -f '/etc/yum.repos.d/rhel-7.4.repo' '/etc/yum.repos.d/rhos-optools-12.0.repo' '/etc/yum.repos.d/rhos-12.0-container-yum-need_images.repo'\"]}}"},
		{V1Compatibility: "{\"id\":\"0e7730eccb3d014b33147b745d771bc0e38a967fd932133a6f5325a3c84282e2\",\"parent\":\"3e49094c0233214ab73f8e5c204af8a14cfc6f0403384553c17fbac2e9d38345\",\"created\":\"2017-11-21T16:49:37.292899Z\",\"container_config\":{\"Cmd\":[\"/bin/sh -c rm -f '/etc/yum.repos.d/compose-rpms-1.repo'\"]},\"author\":\"Red Hat, Inc.\"}"},
		{V1Compatibility: "{\"id\":\"3e49094c0233214ab73f8e5c204af8a14cfc6f0403384553c17fbac2e9d38345\",\"comment\":\"Imported from -\",\"created\":\"2017-11-21T16:47:27.755341705Z\",\"container_config\":{\"Cmd\":[\"\"]}}"},
	}, "amd64")
	require.NoError(t, err)
	return m
}

func TestManifestSchema1FromManifest(t *testing.T) {
	// This just tests that the JSON can be loaded; we test that the parsed
	// values are correctly returned in tests for the individual getter methods.
	_ = manifestSchema1FromFixture(t, "schema1.json")

	// FIXME: Detailed coverage of manifest.Schema1FromManifest failures
	_, err := manifestSchema1FromManifest([]byte{})
	assert.Error(t, err)
}

func TestManifestSchema1FromComponents(t *testing.T) {
	// This just smoke-tests that the manifest can be created; we test that the parsed
	// values are correctly returned in tests for the individual getter methods.
	_ = manifestSchema1FromComponentsLikeFixture(t)

	// Error on invalid input
	_, err := manifestSchema1FromComponents(nil, []manifest.Schema1FSLayers{}, []manifest.Schema1History{}, "amd64")
	assert.Error(t, err)
}

func TestManifestSchema1Serialize(t *testing.T) {
	for _, m := range []genericManifest{
		manifestSchema1FromFixture(t, "schema1.json"),
		manifestSchema1FromComponentsLikeFixture(t),
	} {
		serialized, err := m.serialize()
		require.NoError(t, err)
		// Drop "signatures" which is generated by AddDummyV2S1Signature
		// We would ideally like to compare “serialized” with some transformation of
		// the original fixture, but the ordering of fields in JSON maps is undefined, so this is
		// easier.
		assertJSONEqualsFixture(t, serialized, "schema1.json", "signatures")
	}
}

func TestManifestSchema1ManifestMIMEType(t *testing.T) {
	for _, m := range []genericManifest{
		manifestSchema1FromFixture(t, "schema1.json"),
		manifestSchema1FromComponentsLikeFixture(t),
	} {
		assert.Equal(t, manifest.DockerV2Schema1SignedMediaType, m.manifestMIMEType())
	}
}

func TestManifestSchema1ConfigInfo(t *testing.T) {
	for _, m := range []genericManifest{
		manifestSchema1FromFixture(t, "schema1.json"),
		manifestSchema1FromComponentsLikeFixture(t),
	} {
		assert.Equal(t, types.BlobInfo{Digest: ""}, m.ConfigInfo())
	}
}

func TestManifestSchema1ConfigBlob(t *testing.T) {
	for _, m := range []genericManifest{
		manifestSchema1FromFixture(t, "schema1.json"),
		manifestSchema1FromComponentsLikeFixture(t),
	} {
		blob, err := m.ConfigBlob(context.Background())
		require.NoError(t, err)
		assert.Nil(t, blob)
	}
}

func TestManifestSchema1OCIConfig(t *testing.T) {
	m := manifestSchema1FromFixture(t, "schema1-for-oci-config.json")
	configOCI, err := m.OCIConfig(context.Background())
	require.NoError(t, err)
	// FIXME: A more comprehensive test?
	assert.Equal(t, "/pause", configOCI.Config.Entrypoint[0])
}

func TestManifestSchema1LayerInfo(t *testing.T) {
	for _, m := range []genericManifest{
		manifestSchema1FromFixture(t, "schema1.json"),
		manifestSchema1FromComponentsLikeFixture(t),
	} {
		assert.Equal(t, []types.BlobInfo{
			{
				Digest: "sha256:9cadd93b16ff2a0c51ac967ea2abfadfac50cfa3af8b5bf983d89b8f8647f3e4",
				Size:   -1,
			},
			{
				Digest: "sha256:4aa565ad8b7a87248163ce7dba1dd3894821aac97e846b932ff6b8ef9a8a508a",
				Size:   -1,
			},
			{
				Digest: "sha256:f576d102e09b9eef0e305aaef705d2d43a11bebc3fd5810a761624bd5e11997e",
				Size:   -1,
			},
			{
				Digest: "sha256:9e92df2aea7dc0baf5f1f8d509678d6a6306de27ad06513f8e218371938c07a6",
				Size:   -1,
			},
			{
				Digest: "sha256:62e48e39dc5b30b75a97f05bccc66efbae6058b860ee20a5c9a184b9d5e25788",
				Size:   -1,
			},
			{
				Digest: "sha256:e623934bca8d1a74f51014256445937714481e49343a31bda2bc5f534748184d",
				Size:   -1,
			},
		}, m.LayerInfos())
	}
}

func TestManifestSchema1EmbeddedDockerReferenceConflicts(t *testing.T) {
	for _, m := range []genericManifest{
		manifestSchema1FromFixture(t, "schema1.json"),
		manifestSchema1FromComponentsLikeFixture(t),
	} {
		for name, expected := range map[string]bool{
			"rhosp12/openstack-nova-api:latest":                false, // Exactly the embedded reference
			"example.com/rhosp12/openstack-nova-api:latest":    false, // A different host name, but path and tag match
			"docker.io:3333/rhosp12/openstack-nova-api:latest": false, // A different port, but path and tag match
			"busybox":                              true, // Entirely different, minimal
			"example.com:5555/ns/repo:tag":         true, // Entirely different, maximal
			"rhosp12/openstack-nova-api":           true, // Missing tag
			"rhosp12/openstack-nova-api:notlatest": true, // Different tag
			"notrhosp12/openstack-nova-api:latest": true, // Different namespace
			"rhosp12/notopenstack-nova-api:latest": true, // Different repo
		} {
			ref, err := reference.ParseNormalizedNamed(name)
			require.NoError(t, err, name)
			conflicts := m.EmbeddedDockerReferenceConflicts(ref)
			assert.Equal(t, expected, conflicts, name)
		}
	}
}

func TestManifestSchema1Inspect(t *testing.T) {
	for _, m := range []genericManifest{
		manifestSchema1FromFixture(t, "schema1.json"),
		manifestSchema1FromComponentsLikeFixture(t),
	} {
		ii, err := m.Inspect(context.Background())
		require.NoError(t, err)
		created := time.Date(2018, 1, 25, 0, 37, 48, 268558000, time.UTC)
		var emptyAnnotations map[string]string
		assert.Equal(t, types.ImageInspectInfo{
			Tag:           "latest",
			Created:       &created,
			DockerVersion: "1.12.6",
			Labels: map[string]string{
				"Kolla-SHA":                "5.0.0-39-g6f1b947b",
				"architecture":             "x86_64",
				"authoritative-source-url": "registry.access.redhat.com",
				"build-date":               "2018-01-25T00:32:27.807261",
				"com.redhat.build-host":    "ip-10-29-120-186.ec2.internal",
				"com.redhat.component":     "openstack-nova-api-docker",
				"description":              "Red Hat OpenStack Platform 12.0 nova-api",
				"distribution-scope":       "public",
				"io.k8s.description":       "Red Hat OpenStack Platform 12.0 nova-api",
				"io.k8s.display-name":      "Red Hat OpenStack Platform 12.0 nova-api",
				"io.openshift.tags":        "rhosp osp openstack osp-12.0",
				"kolla_version":            "stable/pike",
				"name":                     "rhosp12/openstack-nova-api",
				"release":                  "20180124.1",
				"summary":                  "Red Hat OpenStack Platform 12.0 nova-api",
				"tripleo-common_version":   "7.6.3-23-g4891cfe",
				"url":                      "https://access.redhat.com/containers/#/registry.access.redhat.com/rhosp12/openstack-nova-api/images/12.0-20180124.1",
				"vcs-ref":                  "9b31243b7b448eb2fc3b6e2c96935b948f806e98",
				"vcs-type":                 "git",
				"vendor":                   "Red Hat, Inc.",
				"version":                  "12.0",
				"version-release":          "12.0-20180124.1",
			},
			Architecture: "amd64",
			Os:           "linux",
			Layers: []string{
				"sha256:9cadd93b16ff2a0c51ac967ea2abfadfac50cfa3af8b5bf983d89b8f8647f3e4",
				"sha256:4aa565ad8b7a87248163ce7dba1dd3894821aac97e846b932ff6b8ef9a8a508a",
				"sha256:f576d102e09b9eef0e305aaef705d2d43a11bebc3fd5810a761624bd5e11997e",
				"sha256:9e92df2aea7dc0baf5f1f8d509678d6a6306de27ad06513f8e218371938c07a6",
				"sha256:62e48e39dc5b30b75a97f05bccc66efbae6058b860ee20a5c9a184b9d5e25788",
				"sha256:e623934bca8d1a74f51014256445937714481e49343a31bda2bc5f534748184d",
			},
			LayersData: []types.ImageInspectLayer{{
				MIMEType:    "",
				Digest:      "sha256:9cadd93b16ff2a0c51ac967ea2abfadfac50cfa3af8b5bf983d89b8f8647f3e4",
				Size:        -1,
				Annotations: emptyAnnotations,
			}, {
				MIMEType:    "",
				Digest:      "sha256:4aa565ad8b7a87248163ce7dba1dd3894821aac97e846b932ff6b8ef9a8a508a",
				Size:        -1,
				Annotations: emptyAnnotations,
			}, {
				MIMEType:    "",
				Digest:      "sha256:f576d102e09b9eef0e305aaef705d2d43a11bebc3fd5810a761624bd5e11997e",
				Size:        -1,
				Annotations: emptyAnnotations,
			}, {
				MIMEType:    "",
				Digest:      "sha256:9e92df2aea7dc0baf5f1f8d509678d6a6306de27ad06513f8e218371938c07a6",
				Size:        -1,
				Annotations: emptyAnnotations,
			}, {
				MIMEType:    "",
				Digest:      "sha256:62e48e39dc5b30b75a97f05bccc66efbae6058b860ee20a5c9a184b9d5e25788",
				Size:        -1,
				Annotations: emptyAnnotations,
			},
				{
					MIMEType:    "",
					Digest:      "sha256:e623934bca8d1a74f51014256445937714481e49343a31bda2bc5f534748184d",
					Size:        -1,
					Annotations: emptyAnnotations,
				},
			},
			Author: "",
			Env: []string{
				"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
				"container=oci",
				"KOLLA_BASE_DISTRO=rhel",
				"KOLLA_INSTALL_TYPE=binary",
				"KOLLA_INSTALL_METATYPE=rhos",
				"PS1=$(tput bold)($(printenv KOLLA_SERVICE_NAME))$(tput sgr0)[$(id -un)@$(hostname -s) $(pwd)]$ ",
			},
		}, *ii)
	}
}

func TestManifestSchema1UpdatedImageNeedsLayerDiffIDs(t *testing.T) {
	for _, m := range []genericManifest{
		manifestSchema1FromFixture(t, "schema1.json"),
		manifestSchema1FromComponentsLikeFixture(t),
	} {
		for mt, expected := range map[string]bool{
			"":                                      false,
			manifest.DockerV2Schema1MediaType:       false,
			manifest.DockerV2Schema1SignedMediaType: false,
			manifest.DockerV2Schema2MediaType:       true,
			imgspecv1.MediaTypeImageManifest:        true,
		} {
			needsDiffIDs := m.UpdatedImageNeedsLayerDiffIDs(types.ManifestUpdateOptions{
				ManifestMIMEType: mt,
			})
			assert.Equal(t, expected, needsDiffIDs, mt)
		}
	}
}

func TestManifestSchema1UpdatedImage(t *testing.T) {
	original := manifestSchema1FromFixture(t, "schema1.json")

	// LayerInfos:
	layerInfos := append(original.LayerInfos()[1:], original.LayerInfos()[0])
	res, err := original.UpdatedImage(context.Background(), types.ManifestUpdateOptions{
		LayerInfos: layerInfos,
	})
	require.NoError(t, err)
	assert.Equal(t, layerInfos, res.LayerInfos())
	_, err = original.UpdatedImage(context.Background(), types.ManifestUpdateOptions{
		LayerInfos: append(layerInfos, layerInfos[0]),
	})
	assert.Error(t, err)

	// EmbeddedDockerReference:
	for _, refName := range []string{
		"busybox",
		"busybox:notlatest",
		"rhosp12/openstack-nova-api:latest",
	} {
		embeddedRef, err := reference.ParseNormalizedNamed(refName)
		require.NoError(t, err)
		res, err = original.UpdatedImage(context.Background(), types.ManifestUpdateOptions{
			EmbeddedDockerReference: embeddedRef,
		})
		require.NoError(t, err)
		// The previous embedded docker reference now does not match.
		nonEmbeddedRef, err := reference.ParseNormalizedNamed("rhosp12/openstack-nova-api:latest")
		require.NoError(t, err)
		conflicts := res.EmbeddedDockerReferenceConflicts(nonEmbeddedRef)
		assert.Equal(t, refName != "rhosp12/openstack-nova-api:latest", conflicts)
	}

	// ManifestMIMEType:
	// Only smoke-test the valid conversions, detailed tests are below. (This also verifies that “original” is not affected.)
	for _, mime := range []string{
		manifest.DockerV2Schema2MediaType,
		imgspecv1.MediaTypeImageManifest,
	} {
		_, err = original.UpdatedImage(context.Background(), types.ManifestUpdateOptions{
			ManifestMIMEType: mime,
			InformationOnly: types.ManifestUpdateInformation{
				LayerInfos:   schema1FixtureLayerInfos,
				LayerDiffIDs: schema1FixtureLayerDiffIDs,
			},
		})
		assert.NoError(t, err, mime)
	}
	for _, mime := range []string{
		"this is invalid",
	} {
		_, err = original.UpdatedImage(context.Background(), types.ManifestUpdateOptions{
			ManifestMIMEType: mime,
		})
		assert.Error(t, err, mime)
	}

	// m hasn’t been changed:
	m2 := manifestSchema1FromFixture(t, "schema1.json")
	typedOriginal, ok := original.(*manifestSchema1)
	require.True(t, ok)
	typedM2, ok := m2.(*manifestSchema1)
	require.True(t, ok)
	assert.Equal(t, *typedM2, *typedOriginal)
}

func TestManifestSchema1ConvertToSchema2(t *testing.T) {
	original := manifestSchema1FromFixture(t, "schema1.json")
	res, err := original.UpdatedImage(context.Background(), types.ManifestUpdateOptions{
		ManifestMIMEType: manifest.DockerV2Schema2MediaType,
		InformationOnly: types.ManifestUpdateInformation{
			LayerInfos:   schema1FixtureLayerInfos,
			LayerDiffIDs: schema1FixtureLayerDiffIDs,
		},
	})
	require.NoError(t, err)

	convertedJSON, mt, err := res.Manifest(context.Background())
	require.NoError(t, err)
	assert.Equal(t, manifest.DockerV2Schema2MediaType, mt)
	// Ignore "config": we don’t want to hard-code a specific digest and size of the marshaled config here.
	assertJSONEqualsFixture(t, convertedJSON, "schema1-to-schema2.json", "config")

	convertedConfig, err := res.ConfigBlob(context.Background())
	require.NoError(t, err)
	assertJSONEqualsFixture(t, convertedConfig, "schema1-to-schema2-config.json")

	// Conversion to schema2 together with changing LayerInfos works as expected (which requires
	// handling schema1 throwaway layers):
	// Use the recorded result of converting the schema2 fixture to schema1, because that one
	// (unlike schem1.json) contains throwaway layers.
	original = manifestSchema1FromFixture(t, "schema2-to-schema1-by-docker.json")
	updatedLayers, updatedLayersCopy := modifiedLayerInfos(t, schema1WithThrowawaysFixtureLayerInfos)
	res, err = original.UpdatedImage(context.Background(), types.ManifestUpdateOptions{
		LayerInfos:       updatedLayers,
		ManifestMIMEType: manifest.DockerV2Schema2MediaType,
		InformationOnly: types.ManifestUpdateInformation{
			LayerInfos:   updatedLayers,
			LayerDiffIDs: schema1WithThrowawaysFixtureLayerDiffIDs,
		},
	})
	require.NoError(t, err)
	assert.Equal(t, updatedLayersCopy, updatedLayers) // updatedLayers have not been modified in place
	convertedJSON, mt, err = res.Manifest(context.Background())
	require.NoError(t, err)
	assert.Equal(t, manifest.DockerV2Schema2MediaType, mt)
	// Layers have been updated as expected
	originalSrc := newSchema2ImageSource(t, "httpd:latest")
	s2Manifest, err := manifestSchema2FromManifest(originalSrc, convertedJSON)
	require.NoError(t, err)
	assert.Equal(t, []types.BlobInfo{
		{
			Digest:    "sha256:6a5a5368e0c2d3e5909184fa28ddfd56072e7ff3ee9a945876f7eee5896ef5ba",
			Size:      51354365,
			MediaType: "application/vnd.docker.image.rootfs.diff.tar.gzip",
		},
		{
			Digest:    "sha256:1bbf5d58d24c47512e234a5623474acf65ae00d4d1414272a893204f44cc680d",
			Size:      151,
			MediaType: "application/vnd.docker.image.rootfs.diff.tar.gzip",
		},
		{
			Digest:    "sha256:8f5dc8a4b12c307ac84de90cdd9a7f3915d1be04c9388868ca118831099c67a8",
			Size:      11739506,
			MediaType: "application/vnd.docker.image.rootfs.diff.tar.gzip",
		},
		{
			Digest:    "sha256:bbd6b22eb11afce63cc76f6bc41042d99f10d6024c96b655dafba930b8d25908",
			Size:      8841832,
			MediaType: "application/vnd.docker.image.rootfs.diff.tar.gzip",
		},
		{
			Digest:    "sha256:960e52ecf8200cbd84e70eb2ad8678f4367e50d14357021872c10fa3fc5935fb",
			Size:      290,
			MediaType: "application/vnd.docker.image.rootfs.diff.tar.gzip",
		},
	}, s2Manifest.LayerInfos())

	// Conversion to schema2 with encryption fails
	encryptedLayers := layerInfosWithCryptoOperation(original.LayerInfos(), types.Encrypt)
	_, err = original.UpdatedImage(context.Background(), types.ManifestUpdateOptions{
		LayerInfos:       encryptedLayers,
		ManifestMIMEType: manifest.DockerV2Schema2MediaType,
		InformationOnly: types.ManifestUpdateInformation{
			LayerInfos:   updatedLayers,
			LayerDiffIDs: schema1WithThrowawaysFixtureLayerDiffIDs,
		},
	})
	assert.Error(t, err)

	// FIXME? Test also the various failure cases, if only to see that we don't crash?
}

func TestManifestSchema1ConvertToManifestOCI1(t *testing.T) {
	original := manifestSchema1FromFixture(t, "schema1.json")
	res, err := original.UpdatedImage(context.Background(), types.ManifestUpdateOptions{
		ManifestMIMEType: imgspecv1.MediaTypeImageManifest,
		InformationOnly: types.ManifestUpdateInformation{
			LayerInfos:   schema1FixtureLayerInfos,
			LayerDiffIDs: schema1FixtureLayerDiffIDs,
		},
	})
	require.NoError(t, err)

	convertedJSON, mt, err := res.Manifest(context.Background())
	require.NoError(t, err)
	assert.Equal(t, imgspecv1.MediaTypeImageManifest, mt)
	// Ignore "config": we don’t want to hard-code a specific digest and size of the marshaled config here.
	assertJSONEqualsFixture(t, convertedJSON, "schema1-to-oci1.json", "config")

	convertedConfig, err := res.ConfigBlob(context.Background())
	require.NoError(t, err)
	assertJSONEqualsFixture(t, convertedConfig, "schema1-to-oci1-config.json")

	// Conversion to OCI together with changing LayerInfos works as expected (which requires
	// handling schema1 throwaway layers):
	// Use the recorded result of converting the schema2 fixture to schema1, because that one
	// (unlike schem1.json) contains throwaway layers.
	original = manifestSchema1FromFixture(t, "schema2-to-schema1-by-docker.json")
	updatedLayers, updatedLayersCopy := modifiedLayerInfos(t, schema1WithThrowawaysFixtureLayerInfos)
	res, err = original.UpdatedImage(context.Background(), types.ManifestUpdateOptions{
		LayerInfos:       updatedLayers,
		ManifestMIMEType: imgspecv1.MediaTypeImageManifest,
		InformationOnly: types.ManifestUpdateInformation{ // FIXME: deduplicate this data
			LayerInfos:   updatedLayers,
			LayerDiffIDs: schema1WithThrowawaysFixtureLayerDiffIDs,
		},
	})
	require.NoError(t, err)
	assert.Equal(t, updatedLayersCopy, updatedLayers) // updatedLayers have not been modified in place
	convertedJSON, mt, err = res.Manifest(context.Background())
	require.NoError(t, err)
	assert.Equal(t, imgspecv1.MediaTypeImageManifest, mt)
	// Layers have been updated as expected
	originalSrc := newSchema2ImageSource(t, "httpd:latest")
	ociManifest, err := manifestOCI1FromManifest(originalSrc, convertedJSON)
	require.NoError(t, err)
	assert.Equal(t, []types.BlobInfo{
		{
			Digest:    "sha256:6a5a5368e0c2d3e5909184fa28ddfd56072e7ff3ee9a945876f7eee5896ef5ba",
			Size:      51354365,
			MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
		},
		{
			Digest:    "sha256:1bbf5d58d24c47512e234a5623474acf65ae00d4d1414272a893204f44cc680d",
			Size:      151,
			MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
		},
		{
			Digest:    "sha256:8f5dc8a4b12c307ac84de90cdd9a7f3915d1be04c9388868ca118831099c67a8",
			Size:      11739506,
			MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
		},
		{
			Digest:    "sha256:bbd6b22eb11afce63cc76f6bc41042d99f10d6024c96b655dafba930b8d25908",
			Size:      8841832,
			MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
		},
		{
			Digest:    "sha256:960e52ecf8200cbd84e70eb2ad8678f4367e50d14357021872c10fa3fc5935fb",
			Size:      290,
			MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
		},
	}, ociManifest.LayerInfos())

	// Conversion to OCI with encryption is possible.
	encryptedLayers := layerInfosWithCryptoOperation(schema1WithThrowawaysFixtureLayerInfos, types.Encrypt)
	res, err = original.UpdatedImage(context.Background(), types.ManifestUpdateOptions{
		LayerInfos:       encryptedLayers,
		ManifestMIMEType: imgspecv1.MediaTypeImageManifest,
		InformationOnly: types.ManifestUpdateInformation{
			LayerInfos:   encryptedLayers,
			LayerDiffIDs: schema1WithThrowawaysFixtureLayerDiffIDs,
		},
	})
	require.NoError(t, err)
	convertedJSON, mt, err = res.Manifest(context.Background())
	require.NoError(t, err)
	assert.Equal(t, imgspecv1.MediaTypeImageManifest, mt)
	// Layers have been updated as expected
	ociManifest, err = manifestOCI1FromManifest(originalSrc, convertedJSON)
	require.NoError(t, err)
	assert.Equal(t, []types.BlobInfo{
		{
			Digest:    "sha256:6a5a5368e0c2d3e5909184fa28ddfd56072e7ff3ee9a945876f7eee5896ef5bb",
			Size:      51354364,
			MediaType: "application/vnd.oci.image.layer.v1.tar+gzip+encrypted",
		},
		{
			Digest:    "sha256:1bbf5d58d24c47512e234a5623474acf65ae00d4d1414272a893204f44cc680c",
			Size:      150,
			MediaType: "application/vnd.oci.image.layer.v1.tar+gzip+encrypted",
		},
		{
			Digest:    "sha256:8f5dc8a4b12c307ac84de90cdd9a7f3915d1be04c9388868ca118831099c67a9",
			Size:      11739507,
			MediaType: "application/vnd.oci.image.layer.v1.tar+gzip+encrypted",
		},
		{
			Digest:    "sha256:bbd6b22eb11afce63cc76f6bc41042d99f10d6024c96b655dafba930b8d25909",
			Size:      8841833,
			MediaType: "application/vnd.oci.image.layer.v1.tar+gzip+encrypted",
		},
		{
			Digest:    "sha256:960e52ecf8200cbd84e70eb2ad8678f4367e50d14357021872c10fa3fc5935fa",
			Size:      291,
			MediaType: "application/vnd.oci.image.layer.v1.tar+gzip+encrypted",
		},
	}, ociManifest.LayerInfos())

	// FIXME? Test also the various failure cases, if only to see that we don't crash?
}

func TestConvertSchema1ToManifestOCIWithAnnotations(t *testing.T) {
	// Test when converting an image from schema 1 (which doesn't support certain fields like
	// URLs, annotations, etc.) to an OCI image (which supports those fields),
	// that UpdatedImage propagates the features to the converted manifest.

	original := manifestSchema1FromFixture(t, "schema1.json")
	layerInfoOverwrites := []types.BlobInfo{
		{
			Digest:    "sha256:6a5a5368e0c2d3e5909184fa28ddfd56072e7ff3ee9a945876f7eee5896ef5bb",
			Size:      51354364,
			MediaType: imgspecv1.MediaTypeImageLayerGzip,
		},
		{
			Digest:    "sha256:1bbf5d58d24c47512e234a5623474acf65ae00d4d1414272a893204f44cc680c",
			Size:      150,
			MediaType: imgspecv1.MediaTypeImageLayerGzip,
		},
		{
			Digest: "sha256:8f5dc8a4b12c307ac84de90cdd9a7f3915d1be04c9388868ca118831099c67a9",
			Size:   11739507,
			URLs: []string{
				"https://layer.url",
			},
			MediaType: imgspecv1.MediaTypeImageLayerGzip,
		},
		{
			Digest: "sha256:bbd6b22eb11afce63cc76f6bc41042d99f10d6024c96b655dafba930b8d25909",
			Size:   8841833,
			Annotations: map[string]string{
				"test-annotation-2": "two",
			},
			MediaType: imgspecv1.MediaTypeImageLayerGzip,
		},
		{
			Digest:    "sha256:960e52ecf8200cbd84e70eb2ad8678f4367e50d14357021872c10fa3fc5935fa",
			Size:      291,
			MediaType: imgspecv1.MediaTypeImageLayerGzip,
		},
		{
			Digest:    "sha256:960e52ecf8200cbd84e70eb2ad8678f4367e50d14357021872c10fa3fc5935fa",
			Size:      291,
			MediaType: imgspecv1.MediaTypeImageLayerGzip,
		},
	}
	res, err := original.UpdatedImage(context.Background(), types.ManifestUpdateOptions{
		ManifestMIMEType: imgspecv1.MediaTypeImageManifest,
		LayerInfos:       layerInfoOverwrites,
		InformationOnly: types.ManifestUpdateInformation{
			LayerInfos:   schema1FixtureLayerInfos,
			LayerDiffIDs: schema1FixtureLayerDiffIDs,
		},
	})
	require.NoError(t, err)
	assert.Equal(t, res.LayerInfos(), layerInfoOverwrites)

	// Doing this with schema2 should fail
	original = manifestSchema1FromFixture(t, "schema1.json")
	res, err = original.UpdatedImage(context.Background(), types.ManifestUpdateOptions{
		ManifestMIMEType: manifest.DockerV2Schema2MediaType,
		LayerInfos:       layerInfoOverwrites,
		InformationOnly: types.ManifestUpdateInformation{
			LayerInfos:   schema1FixtureLayerInfos,
			LayerDiffIDs: schema1FixtureLayerDiffIDs,
		},
	})
	require.NoError(t, err)
	assert.NotEqual(t, res.LayerInfos(), layerInfoOverwrites)
}

func TestManifestSchema1CanChangeLayerCompression(t *testing.T) {
	for _, m := range []genericManifest{
		manifestSchema1FromFixture(t, "schema1.json"),
		manifestSchema1FromComponentsLikeFixture(t),
	} {
		assert.True(t, m.CanChangeLayerCompression(""))
	}
}

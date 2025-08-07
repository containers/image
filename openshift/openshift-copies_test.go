package openshift

import (
	"encoding"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v4"
)

const fixtureKubeConfigPath = "testdata/admin.kubeconfig"

var (
	_ yaml.Unmarshaler         = (*clustersMap)(nil)
	_ yaml.Unmarshaler         = (*authInfosMap)(nil)
	_ yaml.Unmarshaler         = (*contextsMap)(nil)
	_ encoding.TextUnmarshaler = (*yamlBinaryAsBase64String)(nil)
)

// These are only smoke tests based on the skopeo integration test cluster. Error handling, non-trivial configuration merging,
// and any other situations are not currently covered.

// Set up KUBECONFIG to point at the fixture.
// Callers MUST NOT call testing.T.Parallel().
func setupKubeConfigForSerialTest(t *testing.T) {
	t.Setenv("KUBECONFIG", fixtureKubeConfigPath)
}

func TestClientConfigLoadingRules(t *testing.T) {
	setupKubeConfigForSerialTest(t)

	rules := newOpenShiftClientConfigLoadingRules()
	res, err := rules.Load()
	require.NoError(t, err)
	expected := clientcmdConfig{
		Clusters: clustersMap{
			"172-17-0-2:8443": &clientcmdCluster{
				LocationOfOrigin:         fixtureKubeConfigPath,
				Server:                   "https://172.17.0.2:8443",
				CertificateAuthorityData: []byte("Cluster CA"),
			},
		},
		AuthInfos: authInfosMap{
			"system:admin/172-17-0-2:8443": &clientcmdAuthInfo{
				LocationOfOrigin:      fixtureKubeConfigPath,
				ClientCertificateData: []byte("Client cert"),
				ClientKeyData:         []byte("Client key"),
			},
		},
		Contexts: contextsMap{
			"default/172-17-0-2:8443/system:admin": &clientcmdContext{
				LocationOfOrigin: fixtureKubeConfigPath,
				Cluster:          "172-17-0-2:8443",
				AuthInfo:         "system:admin/172-17-0-2:8443",
				Namespace:        "default",
			},
		},
		CurrentContext: "default/172-17-0-2:8443/system:admin",
	}
	assert.Equal(t, &expected, res)
}

func TestDirectClientConfig(t *testing.T) {
	setupKubeConfigForSerialTest(t)

	rules := newOpenShiftClientConfigLoadingRules()
	config, err := rules.Load()
	require.NoError(t, err)

	direct := newNonInteractiveClientConfig(*config)
	res, err := direct.ClientConfig()
	require.NoError(t, err)
	assert.Equal(t, &restConfig{
		Host: "https://172.17.0.2:8443",
		TLSClientConfig: restTLSClientConfig{
			CertData: []byte("Client cert"),
			KeyData:  []byte("Client key"),
			CAData:   []byte("Cluster CA"),
		},
	}, res)
}

func TestDeferredLoadingClientConfig(t *testing.T) {
	setupKubeConfigForSerialTest(t)

	rules := newOpenShiftClientConfigLoadingRules()
	deferred := newNonInteractiveDeferredLoadingClientConfig(rules)
	res, err := deferred.ClientConfig()
	require.NoError(t, err)
	assert.Equal(t, &restConfig{
		Host: "https://172.17.0.2:8443",
		TLSClientConfig: restTLSClientConfig{
			CertData: []byte("Client cert"),
			KeyData:  []byte("Client key"),
			CAData:   []byte("Cluster CA"),
		},
	}, res)
}

func TestDefaultClientConfig(t *testing.T) {
	setupKubeConfigForSerialTest(t)

	config := defaultClientConfig()
	res, err := config.ClientConfig()
	require.NoError(t, err)
	assert.Equal(t, &restConfig{
		Host: "https://172.17.0.2:8443",
		TLSClientConfig: restTLSClientConfig{
			CertData: []byte("Client cert"),
			KeyData:  []byte("Client key"),
			CAData:   []byte("Cluster CA"),
		},
	}, res)
}

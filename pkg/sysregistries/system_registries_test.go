package sysregistries

import (
	"testing"

	"github.com/containers/image/types"
	"github.com/stretchr/testify/assert"
)

var testConfig = []byte("")

func init() {
	readConf = func(sys *types.SystemContext) ([]byte, error) {
		if testConfig != nil {
			return testConfig, nil
		}
		return readRegistryConf(sys)
	}
}

func TestGetRegistriesWithBlankData(t *testing.T) {
	testConfig = nil
	registriesConfig, _ := GetRegistries(&types.SystemContext{SystemRegistriesConfPath: "testdata/empty.conf"})
	assert.Nil(t, registriesConfig)
}

func TestGetRegistriesWithData(t *testing.T) {
	answer := []string{"one.com"}
	testConfig = []byte(`[registries.search]
registries= ['one.com']
`)
	registriesConfig, err := GetRegistries(nil)
	assert.Nil(t, err)
	assert.Equal(t, registriesConfig, answer)
}

func TestGetRegistriesWithBadData(t *testing.T) {
	testConfig = []byte(`registries:
    - one.com
    ,`)
	_, err := GetRegistries(nil)
	assert.Error(t, err)
}

func TestGetRegistriesWithTrailingSlash(t *testing.T) {
	answer := []string{"no-slash.com:5000/path", "one-slash.com", "two-slashes.com", "three-slashes.com:5000"}
	testConfig = []byte(`[registries.search]
	registries= ['no-slash.com:5000/path', 'one-slash.com', 'two-slashes.com//', 'three-slashes.com:5000///']
`)
	// note: only one trailing gets removed
	registriesConfig, err := GetRegistries(nil)
	assert.Nil(t, err)
	assert.Equal(t, registriesConfig, answer)
}

func TestGetInsecureRegistriesWithBlankData(t *testing.T) {
	answer := []string(nil)
	testConfig = []byte("")
	insecureRegistriesConfig, err := GetInsecureRegistries(nil)
	assert.Nil(t, err)
	assert.Equal(t, insecureRegistriesConfig, answer)
}

func TestGetInsecureRegistriesWithData(t *testing.T) {
	answer := []string{"two.com", "three.com"}
	testConfig = []byte(`[registries.search]
registries = ['one.com']
[registries.insecure]
registries = ['two.com', 'three.com']
`)
	insecureRegistriesConfig, err := GetInsecureRegistries(nil)
	if err != nil {
		t.Fail()
	}
	assert.Equal(t, insecureRegistriesConfig, answer)
}

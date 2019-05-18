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
	testConfig = nil
	registriesConfig, err := GetRegistries(&types.SystemContext{SystemRegistriesConfPath: "testdata/search.conf"})
	assert.Nil(t, err)
	assert.Equal(t, registriesConfig, answer)
}

func TestGetRegistriesWithBadData(t *testing.T) {
	testConfig = nil
	_, err := GetRegistries(&types.SystemContext{SystemRegistriesConfPath: "testdata/search-invalid.conf"})
	assert.Error(t, err)
}

func TestGetRegistriesWithTrailingSlash(t *testing.T) {
	answer := []string{"no-slash.com:5000/path", "one-slash.com", "two-slashes.com", "three-slashes.com:5000"}
	testConfig = nil
	// note: only one trailing gets removed
	registriesConfig, err := GetRegistries(&types.SystemContext{SystemRegistriesConfPath: "testdata/search-no-trailing-slash.conf"})
	assert.Nil(t, err)
	assert.Equal(t, registriesConfig, answer)
}

func TestGetInsecureRegistriesWithBlankData(t *testing.T) {
	answer := []string(nil)
	testConfig = nil
	insecureRegistriesConfig, err := GetInsecureRegistries(&types.SystemContext{SystemRegistriesConfPath: "testdata/search.conf"})
	assert.Nil(t, err)
	assert.Equal(t, insecureRegistriesConfig, answer)
}

func TestGetInsecureRegistriesWithData(t *testing.T) {
	answer := []string{"two.com", "three.com"}
	testConfig = nil
	insecureRegistriesConfig, err := GetInsecureRegistries(&types.SystemContext{SystemRegistriesConfPath: "testdata/insecure.conf"})
	if err != nil {
		t.Fail()
	}
	assert.Equal(t, insecureRegistriesConfig, answer)
}

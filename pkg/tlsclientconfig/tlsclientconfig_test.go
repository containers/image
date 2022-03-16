package tlsclientconfig

import (
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"io/ioutil"
	"os"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetupCertificates(t *testing.T) {
	// Success
	tlsc := tls.Config{}
	err := SetupCertificates("testdata/full", &tlsc)
	require.NoError(t, err)
	require.NotNil(t, tlsc.RootCAs)

	// SystemCertPool is implemented natively, and .Subjects() does not
	// return raw certificates, on some systems (as of Go 1.18,
	// Windows, macOS, iOS); so, .Subjects() is deprecated.
	// We still use .Subjects() in these tests, because they work
	// acceptably even in the native case, and they work fine on Linux,
	// which we care about the most.

	// For an unknown reason, with Go 1.18, as of Mar 15 2022,
	// (golangci-lint) reports staticcheck SA1019 about using
	// the deprecated .Subjects() function, but the //lint:ignore
	// directives are ineffective (and cause extra warnings about
	// pointless lint:ignore directives). So, use the big hammer
	// of silencing staticcheck entirely; that should be removed
	// as soon as practical.

	// On systems where SystemCertPool is not special-cased, RootCAs include SystemCertPool;
	// On systems where SystemCertPool is special cased, this compares two empty sets
	// and succeeds.
	// There isn’t a plausible alternative to calling .Subjects() here.
	loadedSubjectBytes := map[string]struct{}{}
	// lint:ignore SA1019 Receiving no data for system roots is acceptable.
	for _, s := range tlsc.RootCAs.Subjects() { //nolint staticcheck: the lint:ignore directive is somehow not recognized (and causes an extra warning!)
		loadedSubjectBytes[string(s)] = struct{}{}
	}
	systemCertPool, err := x509.SystemCertPool()
	require.NoError(t, err)
	// lint:ignore SA1019 Receiving no data for system roots is acceptable.
	for _, s := range systemCertPool.Subjects() { //nolint staticcheck: the lint:ignore directive is somehow not recognized (and causes an extra warning!)
		_, ok := loadedSubjectBytes[string(s)]
		assert.True(t, ok)
	}

	// RootCAs include our certificates.
	// We could possibly test without .Subjects() this by validating certificates
	// signed by our test CAs.
	loadedSubjectCNs := map[string]struct{}{}
	// lint:ignore SA1019 We only care about non-system roots here.
	for _, s := range tlsc.RootCAs.Subjects() { //nolint staticcheck: the lint:ignore directive is somehow not recognized (and causes an extra warning!)
		subjectRDN := pkix.RDNSequence{}
		rest, err := asn1.Unmarshal(s, &subjectRDN)
		require.NoError(t, err)
		require.Empty(t, rest)
		subject := pkix.Name{}
		subject.FillFromRDNSequence(&subjectRDN)
		loadedSubjectCNs[subject.CommonName] = struct{}{}
	}
	_, ok := loadedSubjectCNs["containers/image test CA certificate 1"]
	assert.True(t, ok)
	_, ok = loadedSubjectCNs["containers/image test CA certificate 2"]
	assert.True(t, ok)
	// Certificates include our certificates
	require.Len(t, tlsc.Certificates, 2)
	names := []string{}
	for _, c := range tlsc.Certificates {
		require.Len(t, c.Certificate, 1)
		parsed, err := x509.ParseCertificate(c.Certificate[0])
		require.NoError(t, err)
		names = append(names, parsed.Subject.CommonName)
	}
	sort.Strings(names)
	assert.Equal(t, []string{
		"containers/image test client certificate 1",
		"containers/image test client certificate 2",
	}, names)

	// Directory does not exist
	tlsc = tls.Config{}
	err = SetupCertificates("/this/does/not/exist", &tlsc)
	require.NoError(t, err)
	assert.Equal(t, &tls.Config{}, &tlsc)

	// Directory not accessible
	unreadableDir, err := ioutil.TempDir("", "containers-image-tlsclientconfig")
	require.NoError(t, err)
	defer func() {
		_ = os.Chmod(unreadableDir, 0700)
		_ = os.Remove(unreadableDir)
	}()
	err = os.Chmod(unreadableDir, 000)
	require.NoError(t, err)
	tlsc = tls.Config{}
	err = SetupCertificates(unreadableDir, &tlsc)
	assert.NoError(t, err)
	assert.Equal(t, &tls.Config{}, &tlsc)

	// Other error reading the directory
	tlsc = tls.Config{}
	err = SetupCertificates("/dev/null/is/not/a/directory", &tlsc)
	assert.Error(t, err)

	// Unreadable system cert pool untested
	// Unreadable CA certificate
	tlsc = tls.Config{}
	err = SetupCertificates("testdata/unreadable-ca", &tlsc)
	assert.NoError(t, err)
	assert.Nil(t, tlsc.RootCAs)

	// Missing key file
	tlsc = tls.Config{}
	err = SetupCertificates("testdata/missing-key", &tlsc)
	assert.Error(t, err)
	// Missing certificate file
	tlsc = tls.Config{}
	err = SetupCertificates("testdata/missing-cert", &tlsc)
	assert.Error(t, err)

	// Unreadable key file
	tlsc = tls.Config{}
	err = SetupCertificates("testdata/unreadable-key", &tlsc)
	assert.Error(t, err)
	// Unreadable certificate file
	tlsc = tls.Config{}
	err = SetupCertificates("testdata/unreadable-cert", &tlsc)
	assert.Error(t, err)
}

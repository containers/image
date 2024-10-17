package signature

import (
	"crypto/x509"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPKITrustRootValidate(t *testing.T) {
	certs := x509.NewCertPool()              // Empty is valid enough for our purposes.
	intermediatesCerts := x509.NewCertPool() // Empty is valid enough for our purposes.

	for _, tr := range []pkiTrustRoot{
		{
			caRootsCertificates:         certs,
			caIntermediatesCertificates: intermediatesCerts,
		},
	} {
		err := tr.validate()
		assert.Error(t, err)
	}

	for _, tr := range []pkiTrustRoot{
		{
			caRootsCertificates: certs,
			subjectEmail:        "",
			subjectHostname:     "hostname",
		},
		{
			caRootsCertificates: certs,
			subjectEmail:        "email",
			subjectHostname:     "",
		},
		{
			caRootsCertificates:         certs,
			caIntermediatesCertificates: intermediatesCerts,
			subjectEmail:                "",
			subjectHostname:             "hostname",
		},
		{
			caRootsCertificates:         certs,
			caIntermediatesCertificates: intermediatesCerts,
			subjectEmail:                "email",
			subjectHostname:             "",
		},
	} {
		err := tr.validate()
		assert.NoError(t, err)
	}
}

func TestPKIVerify(t *testing.T) {
	caRootsCertificates := x509.NewCertPool()
	pkiRootsCrtPEM, err := os.ReadFile("fixtures/pki_roots_crt.pem")
	require.NoError(t, err)
	ok := caRootsCertificates.AppendCertsFromPEM(pkiRootsCrtPEM)
	require.True(t, ok)
	caIntermediatesCertificates := x509.NewCertPool()
	pkiIntermediatesCrtPEM, err := os.ReadFile("fixtures/pki_intermediates_crt.pem")
	require.NoError(t, err)
	ok = caIntermediatesCertificates.AppendCertsFromPEM(pkiIntermediatesCrtPEM)
	require.True(t, ok)
	certBytes, err := os.ReadFile("fixtures/pki-cert")
	require.NoError(t, err)
	chainBytes, err := os.ReadFile("fixtures/pki-chain")
	require.NoError(t, err)

	// Success
	pk, err := verifyPKI(&pkiTrustRoot{
		caRootsCertificates:         caRootsCertificates,
		caIntermediatesCertificates: caIntermediatesCertificates,
		subjectEmail:                "qiwan@redhat.com",
		subjectHostname:             "myhost.example.com",
	}, certBytes, chainBytes)
	require.NoError(t, err)
	assertPublicKeyMatchesCert(t, certBytes, pk)

	// Failure
	pk, err = verifyPKI(&pkiTrustRoot{
		caRootsCertificates:         caRootsCertificates,
		caIntermediatesCertificates: caIntermediatesCertificates,
		subjectEmail:                "qiwan@redhat.com",
		subjectHostname:             "not-mutch.example.com",
	}, certBytes, chainBytes)
	require.Error(t, err)
	assert.Nil(t, pk)

	// Failure
	pk, err = verifyPKI(&pkiTrustRoot{
		caRootsCertificates:         caRootsCertificates,
		caIntermediatesCertificates: caIntermediatesCertificates,
		subjectEmail:                "this-does-not-match@redhat.com",
		subjectHostname:             "myhost.example.com",
	}, certBytes, chainBytes)
	require.Error(t, err)
	assert.Nil(t, pk)
}

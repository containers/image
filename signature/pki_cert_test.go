package signature

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"os"
	"testing"
	"time"

	"github.com/sigstore/sigstore/pkg/cryptoutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPKITrustRootValidate(t *testing.T) {
	certs := x509.NewCertPool()             // Empty is valid enough for our purposes.
	intermediateCerts := x509.NewCertPool() // Empty is valid enough for our purposes.

	for _, tr := range []pkiTrustRoot{
		{
			caRootsCertificates:        certs,
			caIntermediateCertificates: intermediateCerts,
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
			caRootsCertificates:        certs,
			caIntermediateCertificates: intermediateCerts,
			subjectEmail:               "",
			subjectHostname:            "hostname",
		},
		{
			caRootsCertificates:        certs,
			caIntermediateCertificates: intermediateCerts,
			subjectEmail:               "email",
			subjectHostname:            "",
		},
	} {
		err := tr.validate()
		assert.NoError(t, err)
	}
}

func TestPKIVerify(t *testing.T) {
	caRootsCertificates := x509.NewCertPool()
	pkiRootsCrtPEM, err := os.ReadFile("fixtures/pki_root_crts.pem")
	require.NoError(t, err)
	ok := caRootsCertificates.AppendCertsFromPEM(pkiRootsCrtPEM)
	require.True(t, ok)
	caIntermediateCertificates := x509.NewCertPool()
	pkiIntermediateCrtPEMs, err := os.ReadFile("fixtures/pki_intermediate_crts.pem")
	require.NoError(t, err)
	ok = caIntermediateCertificates.AppendCertsFromPEM(pkiIntermediateCrtPEMs)
	require.True(t, ok)
	certBytes, err := os.ReadFile("fixtures/pki-cert")
	require.NoError(t, err)
	chainBytes, err := os.ReadFile("fixtures/pki-chain")
	require.NoError(t, err)

	// Success
	tr := &pkiTrustRoot{
		caRootsCertificates:        caRootsCertificates,
		caIntermediateCertificates: caIntermediateCertificates,
		subjectEmail:               "qiwan@redhat.com",
		subjectHostname:            "myhost.example.com",
	}
	pk, err := verifyPKI(tr, certBytes, chainBytes)
	require.NoError(t, err)
	assertPublicKeyMatchesCert(t, certBytes, pk)

	// Invalid intermediate certificate
	pk, err = verifyPKI(tr, certBytes, []byte("not a certificate"))
	assert.Error(t, err)
	assert.Nil(t, pk)

	// Invalid leaf certificate
	pk, err = verifyPKI(tr, []byte("not a certificate"), chainBytes)
	assert.Error(t, err)
	assert.Nil(t, pk)

	// Failure with intermediates provided in neither signature nor config
	pk, err = verifyPKI(&pkiTrustRoot{
		caRootsCertificates: caRootsCertificates,
	}, certBytes, []byte{})
	require.Error(t, err)
	assert.Nil(t, pk)

	// Success with intermediate provided in config only
	pk, err = verifyPKI(&pkiTrustRoot{
		caRootsCertificates:        caRootsCertificates,
		caIntermediateCertificates: caIntermediateCertificates,
	}, certBytes, []byte{})
	require.NoError(t, err)
	assertPublicKeyMatchesCert(t, certBytes, pk)

	// Success with intermediate provided in signature only
	pk, err = verifyPKI(&pkiTrustRoot{
		caRootsCertificates: caRootsCertificates,
	}, certBytes, chainBytes)
	require.NoError(t, err)
	assertPublicKeyMatchesCert(t, certBytes, pk)

	referenceTime := time.Now()
	testCAKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	testCASN, err := cryptoutils.GenerateSerialNumber()
	require.NoError(t, err)
	testCAContents := x509.Certificate{
		SerialNumber:          testCASN,
		Subject:               pkix.Name{CommonName: "root CA"},
		NotBefore:             referenceTime.Add(-1 * time.Minute),
		NotAfter:              referenceTime.Add(1 * time.Hour),
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	testCACertBytes, err := x509.CreateCertificate(rand.Reader, &testCAContents, &testCAContents,
		testCAKey.Public(), testCAKey)
	require.NoError(t, err)
	testCACert, err := x509.ParseCertificate(testCACertBytes)
	require.NoError(t, err)
	testCACertPool := x509.NewCertPool()
	testCACertPool.AddCert(testCACert)

	for _, c := range []struct {
		name          string
		fn            func(cert *x509.Certificate)
		errorFragment string
	}{
		{
			// OtherName SAN element, non-Fulcio certificates does not create this value
			// should cause a failure if none of the Go-parsed SAN elements present
			name: "OtherName in SAN",
			fn: func(cert *x509.Certificate) {
				// Setting SAN in ExtraExtensions causes EmailAddresses to be ignored,
				// so we need to construct the whole SAN manually.
				sansBytes, err := asn1.Marshal([]asn1.RawValue{
					{
						Class:      2,
						Tag:        0,
						IsCompound: false,
						Bytes:      []byte("otherName"),
					},
				})
				require.NoError(t, err)
				cert.ExtraExtensions = append(cert.ExtraExtensions, pkix.Extension{
					Id:       cryptoutils.SANOID,
					Critical: true,
					Value:    sansBytes,
				})
			},
			errorFragment: "unhandled critical extension",
		},
		{ // Other completely unrecognized critical extensions still cause failures
			name: "Unhandled critical extension",
			fn: func(cert *x509.Certificate) {
				cert.ExtraExtensions = append(cert.ExtraExtensions, pkix.Extension{
					Id:       asn1.ObjectIdentifier{2, 99999, 99998, 99997, 99996},
					Critical: true,
					Value:    []byte("whatever"),
				})
			},
			errorFragment: "unhandled critical extension",
		},
		{
			name: "Missing subject hostname",
			fn: func(cert *x509.Certificate) {
				cert.DNSNames = nil
			},
			errorFragment: "Unexpected subject hostname",
		},
		{
			name: "Multiple hostnames, one matches",
			fn: func(cert *x509.Certificate) {
				cert.DNSNames = []string{"a.example.com", "myhost.example.com", "c.example.com"}
			},
			errorFragment: "",
		},
		{
			name: "Hostname mismatch",
			fn: func(cert *x509.Certificate) {
				cert.DNSNames = []string{"this-does-not-match.example.com"}
			},
			errorFragment: "Unexpected subject hostname",
		},
		{
			name: "Multiple hostnames, no matches",
			fn: func(cert *x509.Certificate) {
				cert.DNSNames = []string{"a.example.com", "b.example.com", "c.example.com"}
			},
			errorFragment: "Unexpected subject hostname",
		},
		{
			name: "Missing subject email",
			fn: func(cert *x509.Certificate) {
				cert.EmailAddresses = nil
			},
			errorFragment: `Required email "test-user@example.com" not found`,
		},
		{
			name: "Multiple emails, one matches",
			fn: func(cert *x509.Certificate) {
				cert.EmailAddresses = []string{"a@example.com", "test-user@example.com", "c@example.com"}
			},
			errorFragment: "",
		},
		{
			name: "Email mismatch",
			fn: func(cert *x509.Certificate) {
				cert.EmailAddresses = []string{"a@example.com"}
			},
			errorFragment: `Required email "test-user@example.com" not found`,
		},
		{
			name: "Multiple emails, no matches",
			fn: func(cert *x509.Certificate) {
				cert.EmailAddresses = []string{"a@example.com", "b@example.com", "c@example.com"}
			},
			errorFragment: `Required email "test-user@example.com" not found`,
		},
	} {
		testLeafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err, c.name)
		testLeafSN, err := cryptoutils.GenerateSerialNumber()
		require.NoError(t, err, c.name)
		testLeafContents := x509.Certificate{
			SerialNumber:   testLeafSN,
			Subject:        pkix.Name{CommonName: "leaf"},
			NotBefore:      referenceTime.Add(-1 * time.Minute),
			NotAfter:       referenceTime.Add(1 * time.Hour),
			EmailAddresses: []string{"test-user@example.com"},
			DNSNames:       []string{"myhost.example.com"},
		}
		c.fn(&testLeafContents)
		testLeafCert, err := x509.CreateCertificate(rand.Reader, &testLeafContents, testCACert, testLeafKey.Public(), testCAKey)
		require.NoError(t, err, c.name)
		tr := pkiTrustRoot{
			caRootsCertificates:        testCACertPool,
			caIntermediateCertificates: caIntermediateCertificates,
			subjectHostname:            "myhost.example.com",
			subjectEmail:               "test-user@example.com",
		}
		testLeafPEM := pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: testLeafCert,
		})
		pk, err := verifyPKI(&tr, testLeafPEM, chainBytes)
		if c.errorFragment == "" {
			require.NoError(t, err, c.name)
			assertPublicKeyMatchesCert(t, testLeafPEM, pk)
		} else {
			assert.ErrorContains(t, err, c.errorFragment, c.name)
			assert.Nil(t, pk, c.name)
		}
	}
}

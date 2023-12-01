//go:build !containers_image_fulcio_stub
// +build !containers_image_fulcio_stub

package signature

import (
	"bytes"
	"crypto"
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

	"github.com/sigstore/fulcio/pkg/certificate"
	"github.com/sigstore/sigstore/pkg/cryptoutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slices"
)

// assert that crypto.PublicKey matches the on in certPEM.
func assertPublicKeyMatchesCert(t *testing.T, certPEM []byte, pk crypto.PublicKey) {
	pkInterface, ok := pk.(interface {
		Equal(x crypto.PublicKey) bool
	})
	require.True(t, ok)
	certs, err := cryptoutils.UnmarshalCertificatesFromPEM(certPEM)
	require.NoError(t, err)
	require.Len(t, certs, 1)
	equal := pkInterface.Equal(certs[0].PublicKey)
	assert.True(t, equal)
}

func TestFulcioTrustRootValidate(t *testing.T) {
	certs := x509.NewCertPool() // Empty is valid enough for our purposes.

	for _, tr := range []fulcioTrustRoot{
		{
			caCertificates: certs,
			oidcIssuer:     "",
			subjectEmail:   "email",
		},
		{
			caCertificates: certs,
			oidcIssuer:     "issuer",
			subjectEmail:   "",
		},
	} {
		err := tr.validate()
		assert.Error(t, err)
	}

	tr := fulcioTrustRoot{
		caCertificates: certs,
		oidcIssuer:     "issuer",
		subjectEmail:   "email",
	}
	err := tr.validate()
	assert.NoError(t, err)
}

// oidIssuerV1Ext creates an certificate.OIDIssuer extension
func oidIssuerV1Ext(value string) pkix.Extension {
	return pkix.Extension{
		Id:    certificate.OIDIssuer, //nolint:staticcheck // This is deprecated, but we must continue to accept it.
		Value: []byte(value),
	}
}

// asn1MarshalTest is asn1.MarshalWithParams that must not fail
func asn1MarshalTest(t *testing.T, value any, params string) []byte {
	bytes, err := asn1.MarshalWithParams(value, params)
	require.NoError(t, err)
	return bytes
}

// oidIssuerV2Ext creates an certificate.OIDIssuerV2 extension
func oidIssuerV2Ext(t *testing.T, value string) pkix.Extension {
	return pkix.Extension{
		Id:    certificate.OIDIssuerV2,
		Value: asn1MarshalTest(t, value, "utf8"),
	}
}

func TestFulcioIssuerInCertificate(t *testing.T) {
	referenceTime := time.Now()
	fulcioExtensions, err := certificate.Extensions{Issuer: "https://github.com/login/oauth"}.Render()
	require.NoError(t, err)
	for _, c := range []struct {
		name          string
		extensions    []pkix.Extension
		errorFragment string
		expected      string
	}{
		{
			name:          "Missing issuer",
			extensions:    nil,
			errorFragment: "Fulcio certificate is missing the issuer extension",
		},
		{
			name: "Duplicate issuer v1 extension",
			extensions: []pkix.Extension{
				oidIssuerV1Ext("https://github.com/login/oauth"),
				oidIssuerV1Ext("this does not match"),
			},
			// Match both our message and the Go 1.19 message: "certificate contains duplicate extensions"
			errorFragment: "duplicate",
		},
		{
			name: "Duplicate issuer v2 extension",
			extensions: []pkix.Extension{
				oidIssuerV2Ext(t, "https://github.com/login/oauth"),
				oidIssuerV2Ext(t, "this does not match"),
			},
			// Match both our message and the Go 1.19 message: "certificate contains duplicate extensions"
			errorFragment: "duplicate",
		},
		{
			name: "Completely invalid issuer v2 extension - error parsing",
			extensions: []pkix.Extension{
				{
					Id:    certificate.OIDIssuerV2,
					Value: asn1MarshalTest(t, 1, ""), // not a string type
				},
			},
			errorFragment: "invalid ASN.1 in OIDC issuer v2 extension: asn1: structure error",
		},
		{
			name: "Completely invalid issuer v2 extension - trailing data",
			extensions: []pkix.Extension{
				{
					Id:    certificate.OIDIssuerV2,
					Value: append(slices.Clone(asn1MarshalTest(t, "https://", "utf8")), asn1MarshalTest(t, "example.com", "utf8")...),
				},
			},
			errorFragment: "invalid ASN.1 in OIDC issuer v2 extension, trailing data",
		},
		{
			name:       "One valid issuer v1",
			extensions: []pkix.Extension{oidIssuerV1Ext("https://github.com/login/oauth")},
			expected:   "https://github.com/login/oauth",
		},
		{
			name:       "One valid issuer v2",
			extensions: []pkix.Extension{oidIssuerV2Ext(t, "https://github.com/login/oauth")},
			expected:   "https://github.com/login/oauth",
		},
		{
			name: "Inconsistent issuer v1 and v2",
			extensions: []pkix.Extension{
				oidIssuerV1Ext("https://github.com/login/oauth"),
				oidIssuerV2Ext(t, "this does not match"),
			},
			errorFragment: "inconsistent OIDC issuer extension values",
		},
		{
			name: "Both issuer v1 and v2",
			extensions: []pkix.Extension{
				oidIssuerV1Ext("https://github.com/login/oauth"),
				oidIssuerV2Ext(t, "https://github.com/login/oauth"),
			},
			expected: "https://github.com/login/oauth",
		},
		{
			name:       "Fulcio interoperability",
			extensions: fulcioExtensions,
			expected:   "https://github.com/login/oauth",
		},
	} {
		testLeafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err, c.name)
		testLeafSN, err := cryptoutils.GenerateSerialNumber()
		require.NoError(t, err, c.name)
		testLeafContents := x509.Certificate{
			SerialNumber:    testLeafSN,
			Subject:         pkix.Name{CommonName: "leaf"},
			NotBefore:       referenceTime.Add(-1 * time.Minute),
			NotAfter:        referenceTime.Add(1 * time.Hour),
			ExtraExtensions: c.extensions,
			EmailAddresses:  []string{"test-user@example.com"},
		}
		// To be fairly representative, we do generate and parse a _real_ certificate, but we just use a self-signed certificate instead
		// of bothering with a CA.
		testLeafCert, err := x509.CreateCertificate(rand.Reader, &testLeafContents, &testLeafContents, testLeafKey.Public(), testLeafKey)
		require.NoError(t, err, c.name)
		testLeafPEM := pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: testLeafCert,
		})

		parsedLeafCerts, err := cryptoutils.UnmarshalCertificatesFromPEM(testLeafPEM)
		if err != nil {
			require.NotEqual(t, "", c.errorFragment)
			assert.ErrorContains(t, err, c.errorFragment, c.name)
		} else {
			require.Len(t, parsedLeafCerts, 1)
			parsedLeafCert := parsedLeafCerts[0]

			res, err := fulcioIssuerInCertificate(parsedLeafCert)
			if c.errorFragment == "" {
				require.NoError(t, err, c.name)
				assert.Equal(t, c.expected, res)
			} else {
				assert.ErrorContains(t, err, c.errorFragment, c.name)
				assert.Equal(t, "", res)
			}
		}
	}
}

func TestFulcioTrustRootVerifyFulcioCertificateAtTime(t *testing.T) {
	fulcioCACertificates := x509.NewCertPool()
	fulcioCABundlePEM, err := os.ReadFile("fixtures/fulcio_v1.crt.pem")
	require.NoError(t, err)
	ok := fulcioCACertificates.AppendCertsFromPEM(fulcioCABundlePEM)
	require.True(t, ok)
	fulcioCertBytes, err := os.ReadFile("fixtures/fulcio-cert")
	require.NoError(t, err)
	fulcioChainBytes, err := os.ReadFile("fixtures/fulcio-chain")
	require.NoError(t, err)

	// A successful verification
	tr := fulcioTrustRoot{
		caCertificates: fulcioCACertificates,
		oidcIssuer:     "https://github.com/login/oauth",
		subjectEmail:   "mitr@redhat.com",
	}
	pk, err := tr.verifyFulcioCertificateAtTime(time.Unix(1670870899, 0), fulcioCertBytes, fulcioChainBytes)
	require.NoError(t, err)
	assertPublicKeyMatchesCert(t, fulcioCertBytes, pk)

	// Invalid intermediate certificates
	pk, err = tr.verifyFulcioCertificateAtTime(time.Unix(1670870899, 0), fulcioCertBytes, []byte("not a certificate"))
	assert.Error(t, err)
	assert.Nil(t, pk)

	// No intermediate certificates: verification fails as is …
	pk, err = tr.verifyFulcioCertificateAtTime(time.Unix(1670870899, 0), fulcioCertBytes, []byte{})
	assert.Error(t, err)
	assert.Nil(t, pk)
	// … but succeeds if we add the intermediate certificates to the root of trust
	intermediateCertPool := x509.NewCertPool()
	ok = intermediateCertPool.AppendCertsFromPEM(fulcioChainBytes)
	require.True(t, ok)
	trWithIntermediates := fulcioTrustRoot{
		caCertificates: intermediateCertPool,
		oidcIssuer:     "https://github.com/login/oauth",
		subjectEmail:   "mitr@redhat.com",
	}
	pk, err = trWithIntermediates.verifyFulcioCertificateAtTime(time.Unix(1670870899, 0), fulcioCertBytes, []byte{})
	require.NoError(t, err)
	assertPublicKeyMatchesCert(t, fulcioCertBytes, pk)

	// Invalid leaf certificate
	for _, c := range [][]byte{
		[]byte("not a certificate"),
		{},                               // Empty
		bytes.Repeat(fulcioCertBytes, 2), // More than one certificate
	} {
		pk, err := tr.verifyFulcioCertificateAtTime(time.Unix(1670870899, 0), c, fulcioChainBytes)
		assert.Error(t, err)
		assert.Nil(t, pk)
	}

	// Unexpected relevantTime
	for _, tm := range []time.Time{
		time.Date(2022, time.December, 12, 18, 48, 17, 0, time.UTC),
		time.Date(2022, time.December, 12, 18, 58, 19, 0, time.UTC),
	} {
		pk, err := tr.verifyFulcioCertificateAtTime(tm, fulcioCertBytes, fulcioChainBytes)
		assert.Error(t, err)
		assert.Nil(t, pk)
	}

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
			// OtherName SAN element, with none of the Go-parsed SAN elements present,
			// should not be a reason to reject the certificate entirely;
			// but we don’t actually support matching it, so this basically tests that the code
			// gets far enough to do subject matching.
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
			errorFragment: "Required email test-user@example.com not found",
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
			name: "Missing issuer",
			fn: func(cert *x509.Certificate) {
				cert.ExtraExtensions = nil // Remove the issuer extension
			},
			errorFragment: "Fulcio certificate is missing the issuer extension",
		},
		{
			name: "Duplicate issuer extension",
			fn: func(cert *x509.Certificate) {
				cert.ExtraExtensions = append([]pkix.Extension{oidIssuerV1Ext("this does not match")}, cert.ExtraExtensions...)
			},
			// Match both our message and the Go 1.19 message: "certificate contains duplicate extensions"
			errorFragment: "duplicate",
		},
		{
			name: "Issuer mismatch",
			fn: func(cert *x509.Certificate) {
				cert.ExtraExtensions = []pkix.Extension{oidIssuerV1Ext("this does not match")}
			},
			errorFragment: "Unexpected Fulcio OIDC issuer",
		},
		{
			name: "Missing subject email",
			fn: func(cert *x509.Certificate) {
				cert.EmailAddresses = nil
			},
			errorFragment: "Required email test-user@example.com not found",
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
			errorFragment: "Required email test-user@example.com not found",
		},
		{
			name: "Multiple emails, no matches",
			fn: func(cert *x509.Certificate) {
				cert.EmailAddresses = []string{"a@example.com", "b@example.com", "c@example.com"}
			},
			errorFragment: "Required email test-user@example.com not found",
		},
	} {
		testLeafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err, c.name)
		testLeafSN, err := cryptoutils.GenerateSerialNumber()
		require.NoError(t, err, c.name)
		testLeafContents := x509.Certificate{
			SerialNumber:    testLeafSN,
			Subject:         pkix.Name{CommonName: "leaf"},
			NotBefore:       referenceTime.Add(-1 * time.Minute),
			NotAfter:        referenceTime.Add(1 * time.Hour),
			ExtraExtensions: []pkix.Extension{oidIssuerV1Ext("https://github.com/login/oauth")},
			EmailAddresses:  []string{"test-user@example.com"},
		}
		c.fn(&testLeafContents)
		testLeafCert, err := x509.CreateCertificate(rand.Reader, &testLeafContents, testCACert, testLeafKey.Public(), testCAKey)
		require.NoError(t, err, c.name)
		tr := fulcioTrustRoot{
			caCertificates: testCACertPool,
			oidcIssuer:     "https://github.com/login/oauth",
			subjectEmail:   "test-user@example.com",
		}
		testLeafPEM := pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: testLeafCert,
		})
		pk, err := tr.verifyFulcioCertificateAtTime(referenceTime, testLeafPEM, []byte{})
		if c.errorFragment == "" {
			require.NoError(t, err, c.name)
			assertPublicKeyMatchesCert(t, testLeafPEM, pk)
		} else {
			assert.ErrorContains(t, err, c.errorFragment, c.name)
			assert.Nil(t, pk, c.name)
		}
	}
}

func TestVerifyRekorFulcio(t *testing.T) {
	caCertificates := x509.NewCertPool()
	fulcioCABundlePEM, err := os.ReadFile("fixtures/fulcio_v1.crt.pem")
	require.NoError(t, err)
	ok := caCertificates.AppendCertsFromPEM(fulcioCABundlePEM)
	require.True(t, ok)
	certBytes, err := os.ReadFile("fixtures/fulcio-cert")
	require.NoError(t, err)
	chainBytes, err := os.ReadFile("fixtures/fulcio-chain")
	require.NoError(t, err)
	rekorKeyPEM, err := os.ReadFile("fixtures/rekor.pub")
	require.NoError(t, err)
	rekorKey, err := cryptoutils.UnmarshalPEMToPublicKey(rekorKeyPEM)
	require.NoError(t, err)
	rekorKeyECDSA, ok := rekorKey.(*ecdsa.PublicKey)
	require.True(t, ok)
	setBytes, err := os.ReadFile("fixtures/rekor-set")
	require.NoError(t, err)
	sigBase64, err := os.ReadFile("fixtures/rekor-sig")
	require.NoError(t, err)
	payloadBytes, err := os.ReadFile("fixtures/rekor-payload")
	require.NoError(t, err)

	// Success
	pk, err := verifyRekorFulcio(rekorKeyECDSA, &fulcioTrustRoot{
		caCertificates: caCertificates,
		oidcIssuer:     "https://github.com/login/oauth",
		subjectEmail:   "mitr@redhat.com",
	}, setBytes, certBytes, chainBytes, string(sigBase64), payloadBytes)
	require.NoError(t, err)
	assertPublicKeyMatchesCert(t, certBytes, pk)

	// Rekor failure
	pk, err = verifyRekorFulcio(rekorKeyECDSA, &fulcioTrustRoot{
		caCertificates: caCertificates,
		oidcIssuer:     "https://github.com/login/oauth",
		subjectEmail:   "mitr@redhat.com",
	}, setBytes, certBytes, chainBytes, string(sigBase64), []byte("this payload does not match"))
	assert.Error(t, err)
	assert.Nil(t, pk)

	// Fulcio failure
	pk, err = verifyRekorFulcio(rekorKeyECDSA, &fulcioTrustRoot{
		caCertificates: caCertificates,
		oidcIssuer:     "https://github.com/login/oauth",
		subjectEmail:   "this-does-not-match@example.com",
	}, setBytes, certBytes, chainBytes, string(sigBase64), payloadBytes)
	assert.Error(t, err)
	assert.Nil(t, pk)
}

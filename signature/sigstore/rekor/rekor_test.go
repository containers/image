package rekor

import (
	"context"
	"crypto/ecdsa"
	"encoding/base64"
	"io"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/containers/image/v5/signature/internal"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/sigstore/sigstore/pkg/cryptoutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_rekorUploadKeyOrCert(t *testing.T) {
	REKOR_SERVER := os.Getenv("REKOR_SERVER_URL")
	if REKOR_SERVER == "" {
		t.Skip("REKOR_SERVER_URL not set or empty. This test requires a proper rekor server to run against, use signature/sigstore/rekor/scripts/start-rekor.sh to set one up quickly")
	}

	cosignCertBytes, err := os.ReadFile("../../internal/testdata/rekor-cert")
	require.NoError(t, err)
	cosignSigBase64, err := os.ReadFile("../../internal/testdata/rekor-sig")
	require.NoError(t, err)
	cosignPayloadBytes, err := os.ReadFile("../../internal/testdata/rekor-payload")
	require.NoError(t, err)

	// server needs a moment to set to retry a bit
	resp, err := retryablehttp.Get(REKOR_SERVER + "/api/v1/log/publicKey")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "unexpected status code from rekor server")

	rekorPubKeyPEM, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	resp.Body.Close()

	rekorKey, err := cryptoutils.UnmarshalPEMToPublicKey(rekorPubKeyPEM)
	require.NoError(t, err)
	rekorKeyECDSA, ok := rekorKey.(*ecdsa.PublicKey)
	require.True(t, ok)
	rekorKeysECDSA := []*ecdsa.PublicKey{rekorKeyECDSA}

	type args struct {
		keyOrCertBytes  []byte
		signatureBase64 string
		payloadBytes    []byte
	}
	tests := []struct {
		name    string
		args    args
		wantErr string
	}{
		{
			name: "upload valid signature",
			args: args{
				keyOrCertBytes:  cosignCertBytes,
				signatureBase64: string(cosignSigBase64),
				payloadBytes:    cosignPayloadBytes,
			},
		},
		{
			name: "invalid key",
			args: args{
				keyOrCertBytes:  []byte{1, 2, 3, 4},
				signatureBase64: string(cosignSigBase64),
				payloadBytes:    cosignPayloadBytes,
			},
			wantErr: "Rekor /api/v1/log/entries failed: bad request (400), {Code:400 Message:error processing entry: invalid public key: failure decoding PEM}",
		},
		{
			name: "invalid signature",
			args: args{
				keyOrCertBytes:  cosignCertBytes,
				signatureBase64: "AAAA" + string(cosignSigBase64),
				payloadBytes:    cosignPayloadBytes,
			},
			wantErr: "Rekor /api/v1/log/entries failed: bad request (400), {Code:400 Message:error processing entry: verifying signature: ecdsa: Invalid IEEE_P1363 encoded bytes}",
		},
		{
			name: "invalid payload",
			args: args{
				keyOrCertBytes:  cosignCertBytes,
				signatureBase64: string(cosignSigBase64),
				payloadBytes:    []byte{2, 3, 4},
			},
			wantErr: "Rekor /api/v1/log/entries failed: bad request (400), {Code:400 Message:error processing entry: verifying signature: invalid signature when validating ASN.1 encoded signature}",
		},
	}
	u, err := url.Parse(REKOR_SERVER)
	require.NoError(t, err)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cl := newRekorClient(u)

			signatureBytes, err := base64.StdEncoding.DecodeString(tt.args.signatureBase64)
			require.NoError(t, err)

			currentTime := time.Now()

			// with go 1.24 this should use t.Context()
			got, err := cl.uploadKeyOrCert(context.Background(), tt.args.keyOrCertBytes, signatureBytes, tt.args.payloadBytes)
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)

			// Now verify the returned rekor set
			tm, err := internal.VerifyRekorSET(rekorKeysECDSA, got, tt.args.keyOrCertBytes, tt.args.signatureBase64, tt.args.payloadBytes)
			require.NoError(t, err)
			// Check that the returned timestamp makes sense.
			// Note that using time.After()/Before() to match will yield incorrect result.
			// time.Now() has nanosecond precision while the rekor time is constructed of the unix seconds.
			// That is why we can only compare the full unix seconds here.
			assert.GreaterOrEqual(t, tm.Unix(), currentTime.Unix(), "time: %s after rekor time: %s", currentTime, tm)
		})
	}
}

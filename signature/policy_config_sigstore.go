package signature

import (
	"encoding/json"
	"fmt"

	"github.com/containers/image/v5/signature/internal"
)

// newPRSigstoreSigned returns a new prSigstoreSigned if parameters are valid.
func newPRSigstoreSigned(keyPath string, keyData []byte, signedIdentity PolicyReferenceMatch) (*prSigstoreSigned, error) {
	if keyPath != "" && keyData != nil {
		return nil, InvalidPolicyFormatError("keyType and keyData cannot be used simultaneously")
	}
	if keyPath == "" && keyData == nil {
		return nil, InvalidPolicyFormatError("neither keyType nor keyData specified")
	}
	if signedIdentity == nil {
		return nil, InvalidPolicyFormatError("signedIdentity not specified")
	}
	return &prSigstoreSigned{
		prCommon:       prCommon{Type: prTypeSigstoreSigned},
		KeyPath:        keyPath,
		KeyData:        keyData,
		SignedIdentity: signedIdentity,
	}, nil
}

// newPRSigstoreSignedKeyPath is NewPRSigstoreSignedKeyPath, except it returns the private type.
func newPRSigstoreSignedKeyPath(keyPath string, signedIdentity PolicyReferenceMatch) (*prSigstoreSigned, error) {
	return newPRSigstoreSigned(keyPath, nil, signedIdentity)
}

// NewPRSigstoreSignedKeyPath returns a new "sigstoreSigned" PolicyRequirement using a KeyPath
func NewPRSigstoreSignedKeyPath(keyPath string, signedIdentity PolicyReferenceMatch) (PolicyRequirement, error) {
	return newPRSigstoreSignedKeyPath(keyPath, signedIdentity)
}

// newPRSigstoreSignedKeyData is NewPRSigstoreSignedKeyData, except it returns the private type.
func newPRSigstoreSignedKeyData(keyData []byte, signedIdentity PolicyReferenceMatch) (*prSigstoreSigned, error) {
	return newPRSigstoreSigned("", keyData, signedIdentity)
}

// NewPRSigstoreSignedKeyData returns a new "sigstoreSigned" PolicyRequirement using a KeyData
func NewPRSigstoreSignedKeyData(keyData []byte, signedIdentity PolicyReferenceMatch) (PolicyRequirement, error) {
	return newPRSigstoreSignedKeyData(keyData, signedIdentity)
}

// Compile-time check that prSigstoreSigned implements json.Unmarshaler.
var _ json.Unmarshaler = (*prSigstoreSigned)(nil)

// UnmarshalJSON implements the json.Unmarshaler interface.
func (pr *prSigstoreSigned) UnmarshalJSON(data []byte) error {
	*pr = prSigstoreSigned{}
	var tmp prSigstoreSigned
	var gotKeyPath, gotKeyData = false, false
	var signedIdentity json.RawMessage
	if err := internal.ParanoidUnmarshalJSONObject(data, func(key string) interface{} {
		switch key {
		case "type":
			return &tmp.Type
		case "keyPath":
			gotKeyPath = true
			return &tmp.KeyPath
		case "keyData":
			gotKeyData = true
			return &tmp.KeyData
		case "signedIdentity":
			return &signedIdentity
		default:
			return nil
		}
	}); err != nil {
		return err
	}

	if tmp.Type != prTypeSigstoreSigned {
		return InvalidPolicyFormatError(fmt.Sprintf("Unexpected policy requirement type \"%s\"", tmp.Type))
	}
	if signedIdentity == nil {
		tmp.SignedIdentity = NewPRMMatchRepoDigestOrExact()
	} else {
		si, err := newPolicyReferenceMatchFromJSON(signedIdentity)
		if err != nil {
			return err
		}
		tmp.SignedIdentity = si
	}

	var res *prSigstoreSigned
	var err error
	switch {
	case gotKeyPath && gotKeyData:
		return InvalidPolicyFormatError("keyPath and keyData cannot be used simultaneously")
	case gotKeyPath && !gotKeyData:
		res, err = newPRSigstoreSignedKeyPath(tmp.KeyPath, tmp.SignedIdentity)
	case !gotKeyPath && gotKeyData:
		res, err = newPRSigstoreSignedKeyData(tmp.KeyData, tmp.SignedIdentity)
	case !gotKeyPath && !gotKeyData:
		return InvalidPolicyFormatError("At least one of keyPath and keyData must be specified")
	default: // Coverage: This should never happen
		return fmt.Errorf("Impossible keyPath/keyData presence combination!?")
	}
	if err != nil {
		// Coverage: This cannot currently happen, creating a prSigstoreSigned only fails
		// if signedIdentity is nil, which we replace with a default above.
		return err
	}
	*pr = *res

	return nil
}

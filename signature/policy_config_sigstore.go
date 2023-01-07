package signature

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/containers/image/v5/signature/internal"
)

// PRSigstoreSignedOption is way to pass values to NewPRSigstoreSigned
type PRSigstoreSignedOption func(*prSigstoreSigned) error

// PRSigstoreSignedWithKeyPath specifies a value for the "keyPath" field when calling NewPRSigstoreSigned.
func PRSigstoreSignedWithKeyPath(keyPath string) PRSigstoreSignedOption {
	return func(pr *prSigstoreSigned) error {
		if pr.KeyPath != "" {
			return errors.New(`"keyPath" already specified`)
		}
		pr.KeyPath = keyPath
		return nil
	}
}

// PRSigstoreSignedWithKeyData specifies a value for the "keyData" field when calling NewPRSigstoreSigned.
func PRSigstoreSignedWithKeyData(keyData []byte) PRSigstoreSignedOption {
	return func(pr *prSigstoreSigned) error {
		if pr.KeyData != nil {
			return errors.New(`"keyData" already specified`)
		}
		pr.KeyData = keyData
		return nil
	}
}

// PRSigstoreSignedWithSignedIdentity specifies a value for the "signedIdentity" field when calling NewPRSigstoreSigned.
func PRSigstoreSignedWithSignedIdentity(signedIdentity PolicyReferenceMatch) PRSigstoreSignedOption {
	return func(pr *prSigstoreSigned) error {
		if pr.SignedIdentity != nil {
			return errors.New(`"signedIdentity" already specified`)
		}
		pr.SignedIdentity = signedIdentity
		return nil
	}
}

// newPRSigstoreSigned is NewPRSigstoreSigned, except it returns the private type.
func newPRSigstoreSigned(options ...PRSigstoreSignedOption) (*prSigstoreSigned, error) {
	res := prSigstoreSigned{
		prCommon: prCommon{Type: prTypeSigstoreSigned},
	}
	for _, o := range options {
		if err := o(&res); err != nil {
			return nil, err
		}
	}
	if res.KeyPath != "" && res.KeyData != nil {
		return nil, InvalidPolicyFormatError("keyType and keyData cannot be used simultaneously")
	}
	if res.KeyPath == "" && res.KeyData == nil {
		return nil, InvalidPolicyFormatError("At least one of keyPath and keyData must be specified")
	}
	if res.SignedIdentity == nil {
		return nil, InvalidPolicyFormatError("signedIdentity not specified")
	}

	return &res, nil
}

// NewPRSigstoreSigned returns a new "sigstoreSigned" PolicyRequirement based on options.
func NewPRSigstoreSigned(options ...PRSigstoreSignedOption) (PolicyRequirement, error) {
	return newPRSigstoreSigned(options...)
}

// NewPRSigstoreSignedKeyPath returns a new "sigstoreSigned" PolicyRequirement using a KeyPath
func NewPRSigstoreSignedKeyPath(keyPath string, signedIdentity PolicyReferenceMatch) (PolicyRequirement, error) {
	return NewPRSigstoreSigned(
		PRSigstoreSignedWithKeyPath(keyPath),
		PRSigstoreSignedWithSignedIdentity(signedIdentity),
	)
}

// NewPRSigstoreSignedKeyData returns a new "sigstoreSigned" PolicyRequirement using a KeyData
func NewPRSigstoreSignedKeyData(keyData []byte, signedIdentity PolicyReferenceMatch) (PolicyRequirement, error) {
	return NewPRSigstoreSigned(
		PRSigstoreSignedWithKeyData(keyData),
		PRSigstoreSignedWithSignedIdentity(signedIdentity),
	)
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

	var opts []PRSigstoreSignedOption
	if gotKeyPath {
		opts = append(opts, PRSigstoreSignedWithKeyPath(tmp.KeyPath))
	}
	if gotKeyData {
		opts = append(opts, PRSigstoreSignedWithKeyData(tmp.KeyData))
	}
	opts = append(opts, PRSigstoreSignedWithSignedIdentity(tmp.SignedIdentity))

	res, err := newPRSigstoreSigned(opts...)
	if err != nil {
		return err
	}
	*pr = *res

	return nil
}

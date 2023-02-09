% CONTAINERS-SIGSTORE-SIGNING-PARAMS.YAML 5 sigstore signing parameters Man Page
% Miloslav Trmač
% January 2023

# NAME
containers-sigstore-signing-params.yaml - syntax for the sigstore signing parameter file

# DESCRIPTION

Sigstore signing parameter files are used to store options that may be required to create sigstore signatures.
There is no default location for these files; they are user-managed, and used as inputs to a container image signing operation,
e.g. `skopeo copy --sign-by-sigstore=`_param-file_`.yaml` or `podman push --sign-by-sigstore=`_param-file_`.yaml` .

## FORMAT

Sigstore signing parameter files use YAML.

Many parameters are optional, but the file must specify enough to create a signature;
in particular either a private key, or Fulcio.

### Signing with Private Keys

- `privateKeyFile:` _path_

   Create a signature using a private key at _path_.
   Existence of this field triggers the use of a private key.

- `privateKeyPassphraseFile:` _passphrasePath_

   Read the passphrase required to use `privateKeyFile` from _passphrasePath_.
   Optional: if this is not set, the user must provide the passphrase interactively.

### Signing with Fulcio-generated Certificates

Instead of a static private key, the signing process generates a short-lived key pair
and requests a Fulcio server to issue a certificate for that key pair,
based on the user authenticating to an OpenID Connect provider.

To specify Fulcio, include a `fulcio` sub-object with one or more of the following keys.
In addition, a Rekor server must be specified as well.

- `fulcioURL:` _URL_

  Required. URL of the Fulcio server to use.

- `oidcMode:` `interactive` | `deviceGrant` | `staticToken`

  Required. Specifies how to obtain the necessary OpenID Connect credential.

  `interactive` opens a web browser on the same machine, or if that is not possible,
  asks the user to open a browser manually and to type in the provided code.
  It requires the user to be able to directly interact with the signing process.

  `deviceGrant` uses a device authorization grant flow (RFC 8628).
  It requires the user to be able to read text printed by the signing process, and to act on it reasonably promptly.

  `staticToken` provides a pre-existing OpenID Connect “ID token”, which must have been obtained separately.

- `oidcIssuerURL:` _URL_

  Required for `oidcMode:` `interactive` or `deviceGrant`. URL of an OpenID Connect issuer server to authenticate with.

- `oidcClientID:` _client ID_

  Used for `oidcMode:` `interactive` or `deviceGrant` to identify the client when contacting the issuer.
  Optional but likely to be necessary in those cases.

- `oidcClientSecret:` _client secret_

  Used for `oidcMode:` `interactive` or `deviceGrant` to authenticate the client when contacting the issuer.
  Optional.

- `oidcIDToken:` _token_

  Required for `oidcMode: staticToken`.
  An OpenID Connect ID token that identifies the user (and authorizes certificate issuance).

### Recording the Signature to a Rekor Transparency Server

This can be combined with either a private key or Fulcio.
It is, practically speaking, required for Fulcio; it is optional when a static private key is used, but necessary for
interoperability with the default configuration of `cosign`.

- `rekorURL`: _URL_

  URL of the Rekor server to use.

# EXAMPLES

### Sign Using a Pre-existing Private Key

Uses the ”community infrastructure” Rekor server.

```yaml
privateKeyFile: "/home/user/sigstore/private-key.key"
privateKeyPassphraseFile: "/mnt/user/sigstore-private-key"
rekorURL: "https://rekor.sigstore.dev"
```

### Sign Using a Fulcio-Issued Certificate

Uses the ”community infrastructure” Fulcio and Rekor server,
and the Dex OIDC issuer which delegates to other major issuers like Google and GitHub.

Other configurations will very likely need to also provide an OIDC client secret.

```yaml
fulcio:
  fulcioURL: "https://fulcio.sigstore.dev"
  oidcMode: "interactive"
  oidcIssuerURL: "https://oauth2.sigstore.dev/auth"
  oidcClientID: "sigstore"
rekorURL: "https://rekor.sigstore.dev"
```

# SEE ALSO
  skopeo(1), podman(1)

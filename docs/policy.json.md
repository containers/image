% POLICY.JSON(5) policy.json Man Page
% Miloslav Trmač
% September 2016

# Signature verification policy file format

Signature verification policy files are used to specify policy, e.g. trusted keys,
applicable when deciding whether to accept an image, or individual signatures of that image, as valid.

The default policy is stored (unless overridden at compile-time) at `/etc/containers/policy.json`;
applications performing verification may allow using a different policy instead.

## Overall structure

The signature verification policy file, usually called `policy.json`,
uses a JSON format.  Unlike some other JSON files, its parsing is fairly strict:
unrecognized, duplicated or otherwise invalid fields cause the entire file,
and usually the entire operation, to be rejected.

The purpose of the policy file is to define a set of *policy requirements* for a container image,
usually depending on its location (where it is being pulled from) or otherwise defined identity.

Images may be referenced by *name* (e.g. `busybox:1.25.1`) or by
*digest*
(e.g. `sha256:29f5d56d12684887bdfa50dcd29fc31eea4aaf4ad3bec43daf19026a7ce69912`).

Policy requirements can be associated via the following attributes:

* Image *names* (e.g. `busybox:1.25.1`).
* Image *name prefixes* (e.g. `busybox:`, `busybox:1.25`).
* *Reference transports* (e.g. `docker:/library`, `atomic:5000/vendor/product`).
* *CAS transports* (e.g. `docker:/library/busybox`, `oci:/var/lib/oci/images`).

The policy requirements matching a given image are, in order of
decreasing specificity:

1. A combination of transports and names.
2. Names.
3. A combination of transports and name prefixes.
4. Name prefixes.
5. Transports.
6. A global default policy.

If multiple policy requirements match a given image, only the requirements from the most specific match apply,
the more general policy requirements definitions are ignored.

For images referenced by digest, only the pure-transport and global policies apply.

This is expressed in JSON using the top-level syntax
```js
{
    "default": [/* policy requirements: global default */]
    "names": {
        image_name: [/* policy requirements: default for $image_name */],
        image_name_2: [/*…*/],
        /*…*/
    },
    "prefixes": {
        image_name_prefix: [/* policy requirements: default for $image_name_prefix */],
        image_name_prefix_2: [/*…*/],
        /*…*/
    },
    "transports": {
        transport_name: {
            "default": [/* policy requirements: default for transport $transport_name */],
            "names": {
                image_name_3: [/* policy requirements: default for $image_name_3 over $transport_name */],
                image_name_4: [/*…*/],
                /*…*/
            },
            "prefixes": {
                image_name_prefix_3: [/* policy requirements: default for $image_name_prefix_3 over $transport_name */],
                image_name_prefix_4: [/*…*/],
                /*…*/
            },
        },
        transport_name_2: {/*…*/}
        /*…*/
    }
}
```

The global `default` set of policy requirements is mandatory; all of
the other fields (`names`, `prefixes`, `transports`; any specific
name, prefix, or transport; the transport-specific default; etc.) are
optional.

<!-- NOTE: Keep this in sync with transports/transports.go! -->
## Supported transports

### `atomic:[<hostname>[:<port>][/<namespace>]]`

The `atomic:[<hostname>[:<port>][/<namespace>]]` transport
refers to images in an Atomic Registry.

*Note:* The `<hostname>` and `<port>` refer to the Docker registry host and port (the one used
e.g. for `docker pull`), _not_ to the OpenShift API host and port.

### `dir:[<dirname>]`

The `dir:[<dirname>]` transport refers to images stored in local
directories.

Supported names are paths of subdirectories (either containing a
single image or subdirectories possibly containing images).

*Note:* `<dirname>` must be absolute and contain no symlinks.

### `docker:[<hostname>]/<namespace>[/<repository>]`

The `docker:[<hostname>]/<namespace>[/<repository>]` transport
refers to images in a registry implementing the "Docker Registry HTTP
API V2".

For images referenced by name, `<repository>` is optional.  If set, the
name must start with `<repository>/`.

For images referenced by digest, `<repository>` is required.

### `oci:[<dirname>]`

The `oci:[<dirname>]` transport refers to images in directories
compliant with "Open Container Image Layout Specification".

*Note:* See `dir:` above for semantics and restrictions on the directory paths, they apply to `oci:` equivalently.

## Policy Requirements

Using the mechanisms above, a set of policy requirements is looked up.  The policy requirements
are represented as a JSON array of individual requirement objects.  For an image to be accepted,
*all* of the requirements must be satisfied simulatenously.

The policy requirements can also be used to decide whether an individual signature is accepted (= is signed by a recognized key of a known author);
in that case some requirements may apply only to some signatures, but each signature must be accepted by *at least one* requirement object.

The following requirement objects are supported:

### `insecureAcceptAnything`

A simple requirement with the following syntax

```json
{"type":"insecureAcceptAnything"}
```

This requirement accepts any image (but note that other requirements in the array still apply).

When deciding to accept an individual signature, this requirement does not have any effect; it does *not* cause the signature to be accepted, though.

This is useful primarily for transports where no signature verification is required;
because the array of policy requirements must not be empty, this requirement is used
to represent the lack of requirements explicitly.

### `reject`

A simple requirement with the following syntax:

```json
{"type":"reject"}
```

This requirement rejects every image, and every signature.

### `digestOnly`

This requirement accepts images referenced by digest and rejects
images referenced by name.

```json
{"type":"digestOnly"}
```

### `nameOnly`

This requirement accepts images referenced by name and rejects
images referenced by digest.

```json
{"type":"nameOnly"}
```

### `signedBy`

This requirement requires an image referenced by name to be signed
with an expected identity, or accepts a signature if it is using an
expected identity and key.  This requirement accepts images referenced
by digest.

```js
{
    "type":    "signedBy",
    "keyType": "GPGKeys", /* The only currently supported value */
    "keyPath": "/path/to/local/keyring/file",
    "keyData": "base64-encoded-keyring-data",
    "signedIdentity": identity_requirement
}
```
<!-- Later: other keyType values -->

Exactly one of `keyPath` and `keyData` must be present, containing a GPG keyring of one or more public keys.  Only signatures made by these keys are accepted.

The `signedIdentity` field, a JSON object, specifies what image identity the signature claims about the image.
One of the following alternatives are supported:

- The identity in the signature must exactly match the image name.

  ```json
  {"type":"matchExact"}
  ```
- The identity in the signature must be in the same repository as the image identity.  This is useful e.g. to pull an image using the `:latest` tag when the image is signed with a tag specifing an exact image version.

  ```json
  {"type":"matchRepository"}
  ```

  This policy may only be used in transports which include _repository_.

If the `signedIdentity` field is missing, it is treated as `matchExact`.

<!-- ### `signedBaseLayer` -->

## Examples

It is *strongly* recommended to set the `default` policy to `reject`, and then
selectively allow individual transports and scopes as desired.

### A reasonably locked-down system

(Note that the `/*`…`*/` comments are not valid in JSON, and must not be used in real policies.)

```js
{
    "default": [{"type": "reject"}], /* Reject anything not explicitly allowed */
    "names": {
        "alpine:3.4": {
            /* Trust alpine:3.4 signed by QA */
            "default": [
                {
                    "type": "signedBy",
                    "keyType": "GPGKeys",
                    "keyPath": "/path/to/official-pubkey.gpg"
                }
            ],
    "prefixes": {
        "busybox:": [
            /* Trust any BusyBox release signed by a lead developer */
            {
                "type": "signedBy",
                "keyType": "GPGKeys",
                "keyPath": "/path/to/pubkey/47B70C55ACC9965B.gpg"
            }
        ]
    }
    "transports": {
        "docker:docker.io/openshift": {
            /* Allow installing images from a specific repository namespace, without cryptographic verification.
               This namespace includes images like openshift/hello-openshift and openshift/origin. */
            "default": [{"type": "insecureAcceptAnything"}]
        },
        "docker:docker.io/library/nginx": {
            /* Allow installing the “official” Nginx images with repository
               name matching, but do not allow fetching by digest. */
            "default": [
                {"type": "nameOnly"},
                {
                    "type": "signedBy",
                    "keyType": "GPGKeys",
                    "keyPath": "/path/to/official-pubkey.gpg"
                    "signedIdentity": {
                        "type": "matchRepository"
                    }
                }
            ]
        },
        /* Other docker: images use the global default policy and are rejected */
        "dir:/var/lib/oci/runtime": {
            /* Allow any runtime images originating in local directories beneath /var/lib/oci/runtime */
            "default": [{"type": "insecureAcceptAnything"}]
        },
        "atomic:example.com:5000/myns/official": {
            /* The common case: using a known key for a repository or set of repositories */
            "default": [
                {
                    "type": "signedBy",
                    "keyType": "GPGKeys",
                    "keyPath": "/path/to/official-pubkey.gpg"
                }
            ],
        },
        "atomic:example.com:5000/vendor": [
            /* A more complex example, for a repository which contains a mirror of a third-party product,
               which must be signed-off by local IT */
            "default": [
                { /* Require the image to be signed by the original vendor. */
                    "type": "signedBy",
                    "keyType": "GPGKeys",
                    "keyPath": "/path/to/vendor-pubkey.gpg"
                },
                { /* Require the image to _also_ be signed by a local reviewer. */
                    "type": "signedBy",
                    "keyType": "GPGKeys",
                    "keyPath": "/path/to/reviewer-pubkey.gpg"
                }
            ]
        }
    }
}
```

### Completely disable security, allow all images, do not trust any signatures

```json
{
    "default": [{"type": "insecureAcceptAnything"}]
}
```

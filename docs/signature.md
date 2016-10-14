# Image Signature Specification

**Version 0.1**

## Introduction

This document defines a detached container image signature object and signing methods.

## Signature Format

```js
{
    "critical": {/* required fields */
        "identity": {/* identity reference */},
        "image": {/* signed object reference */ },
        "type": "..."
    },
    "optional": {/* optional metadata fields */}
    }
}
```

### Fields

There are two top-level fields, **critical** (required) and **optional** (optional).

#### `critical`

**identity** (string):

```js
{
    "docker-reference": imageName
}
```

`imageName` per [V2 API](https://docs.docker.com/registry/spec/api/#/overview) Required.

**image** (string):

```js
{
    "docker-manifest-digest": manifestDigest
}
```

`manifestDigest` in the form of `<algorithm>:<hashValue>`

**type** (string): Only supported value is "atomic container signature"

#### `optional`

**creator** (string): Creator ID. This refers to the tooling used to generate the signature.

**timestamp** (int64): timestamp epoch

### Example

```js
{
    "critical": {
        "identity": {
            "docker-reference": "busybox"
        },
        "image": {
            "docker-manifest-digest": "sha256:a59906e33509d14c036c8678d687bd4eec81ed7c4b8ce907b888c607f6a1e0e6"
        },
        "type": "atomic container signature"
    },
    "optional": {
        "creator": "atomic 0.1.0-dev",
        "timestamp": 1471035347
    }
}
```

### Encryption and Decryption

The signature data is written to a file that is encrypted and signed with a private key. The file may be decrypted (verified) using the corresponding public key.

**Example GPG Sign command**

Given signature file busybox.sig formatted per above:

```
$ gpg2 -r KEYID --encrypt --sign busybox.sig
```

**Example GPG Verify command**

```
$ gpg2 --decrypt busybox.sig.gpg
```

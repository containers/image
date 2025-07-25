#! /bin/bash

# NOTE: To generate v3 signatures, this MUST be run on a system with GPG < 2.1, e.g. a RHEL 7.
# WARNING: This lazily writes to $(pwd). It is best run on a short-term VM.

# This is only a fragment of the ideal script; as you regenerate any other keys, please work on improving it.

set -x

dest=$(pwd)/new-fixtures
mkdir -p "$dest"

function resign() {
    local key_id=$1
    local signature=$2
    local other_opts=$3

    (GNUPGHOME= gpg -d "signature/fixtures/$signature"; true) | gpg --sign --digest-algo SHA256 --default-key "$key_id" $other_opts - > "$dest/$signature"
}

export GNUPGHOME=$(mktemp -d -t regenerate-keys.XXXXXX)
echo "GNUPGHOME: $GNUPGHOME" # Don't set up trap(1) to delete it, to allow inspection / debugging.

# Key-Usage: auth is used because "cert" is implied, and the only one we want, but an empty value is not accepted
# by gpg.
cat >batch-input <<EOF
Key-Type: RSA
Key-Length: 3072
Key-Usage: auth
Subkey-Type: RSA
Subkey-Length: 3072
Subkey-Usage: sign
%no-protection
Name-Real: c/image test key with subkey
Expire-Date: 0
%commit
EOF
out=$(gpg --batch --gen-key --cert-digest-algo SHA256 < batch-input --status-fd 1 --with-colons)
echo "$out" | grep -v ' PROGRESS '

fingerprint=$(echo "$out" | awk '$2 == "KEY_CREATED" { print $4 }')
# Yes, --fingerprint is used twice, to include the subkey fingerprint.
subkey_fingerprint=$(gpg --list-keys --fingerprint --fingerprint --with-colon "$fingerprint" | awk -F ':' '$1 == "fpr" { fp = $10 } END { print fp }')
echo "TestKeyFingerprintPrimaryWithSubkey = \"$fingerprint\"" > fixtures_info
echo "TestKeyFingerprintSubkeyWithSubkey = \"$subkey_fingerprint\"" >> fixtures_info

resign $subkey_fingerprint subkey.signature
resign $subkey_fingerprint subkey.signature-v3 --force-v3-sigs
gpg --export --armor "$fingerprint" > $dest/public-key-with-subkey.gpg

# Key-Usage: auth is used because "cert" is implied, and the only one we want, but an empty value is not accepted
# by gpg.
cat >batch-input <<EOF
Key-Type: RSA
Key-Length: 3072
Key-Usage: auth
Subkey-Type: RSA
Subkey-Length: 3072
Subkey-Usage: sign
%no-protection
Name-Real: c/image test key with a REVOKED subkey
Expire-Date: 0
%commit
EOF
out=$(gpg --batch --gen-key --cert-digest-algo SHA256 < batch-input --status-fd 1 --with-colons)
echo "$out" | grep -v ' PROGRESS '

fingerprint=$(echo "$out" | awk '$2 == "KEY_CREATED" { print $4 }')
# Yes, --fingerprint is used twice, to include the subkey fingerprint.
subkey_fingerprint=$(gpg --list-keys --fingerprint --fingerprint --with-colon "$fingerprint" | awk -F ':' '$1 == "fpr" { fp = $10 } END { print fp }')
echo "TestKeyFingerprintPrimaryWithRevokedSubkey = \"$fingerprint\"" >> fixtures_info
echo "TestKeyFingerprintSubkeyWithRevokedSubkey = \"$subkey_fingerprint\"" >> fixtures_info

resign $subkey_fingerprint subkey-revoked.signature
resign $subkey_fingerprint subkey-revoked.signature-v3 --force-v3-sigs

# FIXME? Can this be fully automated? --batch alone doesn't work, --yes seems to be ignored.
# Answer "yes", "key is compromised" (NOT "no longer used", to break the subkey-revoked.signature* files created above),
# an empty message, and finally, "save"
gpg --yes --cert-digest-algo SHA256 --edit-key "$fingerprint" 'key 1' 'revkey'

gpg --export --armor "$fingerprint" > $dest/public-key-with-revoked-subkey.gpg




# EVENTUALLY, rebuild signature/fixtures/pubring.gpg from all keys (currently impossible because this script
# does not regenerate all keys that should be present there):
# GNUPGHOME=$dest gpg --import "$dest/public-key-with-subkey.gpg"

# === We are done. Show how the regenerated files differ.
for i in "$dest"/*; do
    (echo "==== $i"; diff -u <(gpg --list-packets < "signature/fixtures/${i#$dest/}") <(gpg --list-packets < "$i")) |& less
done

cat fixtures_info
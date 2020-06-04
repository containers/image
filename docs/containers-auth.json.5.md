% containers-auth.json(5)

# NAME
containers-auth.json - syntax for the registry authentication file

# DESCRIPTION

A credentials file in JSON format used to authenticate against container image registries.
On Linux it is stored at `${XDG_RUNTIME_DIR}/containers/auth.json`;
on Windows and macOS, at `$HOME/.config/containers/auth.json`

## FORMAT

The auth.json file stores encrypted authentication information for the
user to container image registries.  The file can have zero to many entries and
is created by a `login` command from a container tool such as `podman login`,
`buildah login` or `skopeo login`.  Each entry includes the name of the registry and then an auth
token in the form of a base64 encoded string from the concatenation of the
username, a colon, and the password.

The following example shows the values found in auth.json after the user logged in to
their accounts on quay.io and docker.io:

```
{
	"auths": {
		"docker.io": {
			"auth": "erfi7sYi89234xJUqaqxgmzcnQ2rRFWM5aJX0EC="
		},
		"quay.io": {
			"auth": "juQAqGmz5eR1ipzx8Evn6KGdw8fEa1w5MWczmgY="
		}
	}
}
```

An entry can be removed by using a `logout` command from a container
tool such as `podman logout` or `buildah logout`.

In addition, credential helpers can be configured for specific registries and the credentials-helper
software can be used to manage the credentials in a more secure way than depending on the base64 encoded authentication
provided by `login`.  If the credential helpers are configured for specific registries, the base64 encoded authentication will not be used
for operations concerning credentials of the specified registries.

When the credential helper is in use on a Linux platform, the auth.json file would contain keys that specify the registry domain, and values that specify the suffix of the program to use (i.e. everything after docker-credential-).  For example:

```
{
    "auths": {
        "localhost:5001": {}
    },
    "credHelpers": {
		"registry.example.com": "secretservice"
	}
}
```

For more information on credential helpers, please reference the [GitHub docker-credential-helpers project](https://github.com/docker/docker-credential-helpers/releases).

# SEE ALSO
    buildah-login(1), buildah-logout(1), podman-login(1), podman-logout(1), skopeo-login(1), skopeo-logout(1)

# HISTORY
Feb 2020, Originally compiled by Tom Sweeney <tsweeney@redhat.com>

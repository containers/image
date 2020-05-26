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
is created by a `login` command from a container tool such as `podman login` or
`buildah login`.  Each entry includes the name of the registry and then an auth
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

# SEE ALSO
    buildah-login(1), buildah-logout(1), podman-login(1), podman-logout(1)

# HISTORY
Feb 2020, Originally compiled by Tom Sweeney <tsweeney@redhat.com>

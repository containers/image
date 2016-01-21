skopeo
=

_Please be aware `skopeo` is still work in progress_

`skopeo` is a command line utility which is able to _inspect_ a repository on a Docker registry.
By _inspect_ I mean it fetches the repository's manifest and it is able to show you a `docker inspect`-like
json output about a whole repository or a tag. This tool, in constrast to `docker inspect`, helps you gather useful information about
a repository or a tag before pulling it (using disk space) - e.g. - which tags are available for the given repository? which labels the image has?

Examples:
```sh
# show repository's labels of rhel7:latest
$ skopeo registry.access.redhat.com/rhel7 | jq '.Config.Labels'
{
  "Architecture": "x86_64",
  "Authoritative_Registry": "registry.access.redhat.com",
  "BZComponent": "rhel-server-docker",
  "Build_Host": "rcm-img04.build.eng.bos.redhat.com",
  "Name": "rhel7/rhel",
  "Release": "38",
  "Vendor": "Red Hat, Inc.",
  "Version": "7.2"
}

# show repository's tags
$ skopeo docker.io/fedora | jq '.RepoTags'
[
  "20",
  "21",
  "22",
  "23",
  "heisenbug",
  "latest",
  "rawhide"
]

# show image's digest
$ skopeo docker.io/fedora:rawhide | jq '.Digest'
"sha256:905b4846938c8aef94f52f3e41a11398ae5b40f5855fb0e40ed9c157e721d7f8"
```

Private registries with authentication
-
When interacting with private registries, `skopeo` first looks for the Docker's cli config file (usually located at `$HOME/.docker/config.json`) to get the credentials needed to authenticate. When the file isn't available it falls back looking for `--username` and `--password` flags. The ultimate fallback, as Docker does, is to provide an empty authentication when interacting with those registries.

Examples:
```sh
# on my system
$ skopeo --help | grep docker-cfg
   --docker-cfg "/home/runcom/.docker"	Docker's cli config for auth

$ cat /home/runcom/.docker/config.json
{
	"auths": {
		"myregistrydomain.com:5000": {
			"auth": "dGVzdHVzZXI6dGVzdHBhc3N3b3Jk",
			"email": "stuf@ex.cm"
		}
	}
}

# we can see I'm already authenticated via docker login so everything will be fine
$ skopeo myregistrydomain.com:5000/busybox
{"Tag":"latest","Digest":"sha256:473bb2189d7b913ed7187a33d11e743fdc2f88931122a44d91a301b64419f092","RepoTags":["latest"],"Comment":"","Created":"2016-01-15T18:06:41.282540103Z","ContainerConfig":{"Hostname":"aded96b43f48","Domainname":"","User":"","AttachStdin":false,"AttachStdout":false,"AttachStderr":false,"Tty":false,"OpenStdin":false,"StdinOnce":false,"Env":null,"Cmd":["/bin/sh","-c","#(nop) CMD [\"sh\"]"],"Image":"9e77fef7a1c9f989988c06620dabc4020c607885b959a2cbd7c2283c91da3e33","Volumes":null,"WorkingDir":"","Entrypoint":null,"OnBuild":null,"Labels":null},"DockerVersion":"1.8.3","Author":"","Config":{"Hostname":"aded96b43f48","Domainname":"","User":"","AttachStdin":false,"AttachStdout":false,"AttachStderr":false,"Tty":false,"OpenStdin":false,"StdinOnce":false,"Env":null,"Cmd":["sh"],"Image":"9e77fef7a1c9f989988c06620dabc4020c607885b959a2cbd7c2283c91da3e33","Volumes":null,"WorkingDir":"","Entrypoint":null,"OnBuild":null,"Labels":null},"Architecture":"amd64","Os":"linux"}

# let's try now to fake a non existent Docker's config file
$ skopeo --docker-cfg="" myregistrydomain.com:5000/busybox
INFO[0000] Docker cli config file not found: stat : no such file or directory, falling back to --username and --password
FATA[0000] unable to ping registry endpoint http://myregistrydomain.com:5000/v0/
v2 ping attempt failed with error: Get http://myregistrydomain.com:5000/v2/: malformed HTTP response "\x15\x03\x01\x00\x02\x02"
 v1 ping attempt failed with error: Get http://myregistrydomain.com:5000/v1/_ping: malformed HTTP response "\x15\x03\x01\x00\x02\x02"

# we can see the cli config isn't found so it looks for --username and --password
# but because I didn't provide them I can't auth to the registry and I receive the above error
INFO[0000] Docker cli config file not found: stat : no such file or directory, falling back to --username and --password

# passing --username and --password - we can see that everything goes fine despite an info showing
# cli config can't be found
$ skopeo --docker-cfg="" --username=testuser --password=testpassword myregistrydomain.com:5000/busybox
INFO[0000] Docker cli config file not found: stat : no such file or directory, falling back to --username and --password
{"Tag":"latest","Digest":"sha256:473bb2189d7b913ed7187a33d11e743fdc2f88931122a44d91a301b64419f092","RepoTags":["latest"],"Comment":"","Created":"2016-01-15T18:06:41.282540103Z","ContainerConfig":{"Hostname":"aded96b43f48","Domainname":"","User":"","AttachStdin":false,"AttachStdout":false,"AttachStderr":false,"Tty":false,"OpenStdin":false,"StdinOnce":false,"Env":null,"Cmd":["/bin/sh","-c","#(nop) CMD [\"sh\"]"],"Image":"9e77fef7a1c9f989988c06620dabc4020c607885b959a2cbd7c2283c91da3e33","Volumes":null,"WorkingDir":"","Entrypoint":null,"OnBuild":null,"Labels":null},"DockerVersion":"1.8.3","Author":"","Config":{"Hostname":"aded96b43f48","Domainname":"","User":"","AttachStdin":false,"AttachStdout":false,"AttachStderr":false,"Tty":false,"OpenStdin":false,"StdinOnce":false,"Env":null,"Cmd":["sh"],"Image":"9e77fef7a1c9f989988c06620dabc4020c607885b959a2cbd7c2283c91da3e33","Volumes":null,"WorkingDir":"","Entrypoint":null,"OnBuild":null,"Labels":null},"Architecture":"amd64","Os":"linux"}
```
If your cli config is found but it doesn't contain the necessary credentials for the queried registry
you'll get an error. You can fix this by either logging in (via `docker login`) or providing `--username`
and `--password`.
Building
-
```sh
$ git clone https://github.com/runcom/skopeo
$ make
```
Installing
-
If you built from the _Building_ step, just do:
```sh
$ sudo make install
```
You can also grab a binary, built for _linux x86_64_, from the releases page, or:
```sh
$ export RELEASE=0.0.1
$ wget https://github.com/runcom/skopeo/releases/download/v$RELEASE/skopeo
$ chmod +x skopeo
$ sudo mv skopeo /usr/local/bin/skopeo
```
TODO
-
- show repo tags via flag or when reference isn't tagged or digested
- add tests (integration with deployed registries in container - Docker-like)
- get rid of Docker (meaning make this work w/o needing Docker installed)

NOT TODO
-
- provide a _format_ flag - just use the awesome [jq](https://stedolan.github.io/jq/)

License
-
MIT

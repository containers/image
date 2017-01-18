#!/bin/bash	

set -eu

dockerd -s vfs &
sleep 5

# explanation of sudo flags:
# -H: rewrite $HOME to target user $HOME (because -E)
# -E: persist environment
# -u: user to use
sudo -H -E -u gouser -- sh -c "PATH=\"/go/bin:${PATH}\" make .gitvalidation validate test test-skopeo"

status=$?

killall dockerd
wait
exit $status

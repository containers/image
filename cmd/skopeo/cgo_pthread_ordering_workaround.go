package main

/*
This is a pretty horrible workaround.  Due to a glibc bug
https://bugzilla.redhat.com/show_bug.cgi?id=1326903 , we must ensure we link
with -lgpgme before -lpthread.  Such arguments come from various packages
using cgo, and the ordering of these arguments is, with current (go tool link),
dependent on the order in which the cgo-using packages are found in a
breadth-first search following dependencies, starting from “main”.

Thus, if
   import "net"
is processed before
   import "…/skopeo/signature"
it will, in the next level of the BFS, pull in "runtime/cgo" (a dependency of
"net") before "mtrmac/gpgme" (a dependency of "…/skopeo/signature"), causing
-lpthread (used by "runtime/cgo") to be used before -lgpgme.

This might be possible to work around by careful import ordering, or by removing
a direct dependency on "net", but that would be very fragile.

So, until the above bug is fixed, add -lgpgme directly in the "main" package
to ensure the needed build order.

Unfortunately, this workaround needs to be applied at the top level of any user
of "…/skopeo/signature"; it cannot be added to "…/skopeo/signature" itself,
by that time this package is first processed by the linker, a -lpthread may
already be queued and it would be too late.
*/

// #cgo LDFLAGS: -lgpgme
import "C"

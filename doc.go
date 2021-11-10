// The package image provides libraries and commands to interact with container images.
//
//  package main
//
//  import (
//	"context"
//	"fmt"
//
//	"github.com/containers/image/v5/docker"
//  )
//
//  func main() {
//	ref, err := docker.ParseReference("//fedora")
//	if err != nil {
//		panic(err)
//	}
//	ctx := context.Background()
//	img, err := ref.NewImage(ctx, nil)
//	if err != nil {
//		panic(err)
//	}
//	defer img.Close()
//	b, _, err := img.Manifest(ctx)
//	if err != nil {
//		panic(err)
//	}
//	fmt.Printf("%s", string(b))
//  }
//
//
// ## Notes on running in rootless mode
//
// If your application needs to access a containers/storage store in rootless
// mode, then the following additional steps have to be performed at start-up of
// your application:
//
//  package main
//
//  import (
//  	"github.com/containers/storage/pkg/reexec"
//	"github.com/syndtr/gocapability/capability"
//      "github.com/containers/storage/pkg/unshare"
//  )
//
//  var neededCapabilities = []capability.Cap{
//          capability.CAP_CHOWN,
//          capability.CAP_DAC_OVERRIDE,
//          capability.CAP_FOWNER,
//          capability.CAP_FSETID,
//          capability.CAP_MKNOD,
//          capability.CAP_SETFCAP,
//  }
//
//  func main() {
//          	reexec.Init()
//
//        	capabilities, err := capability.NewPid(0)
//		if err != nil {
//			panic(err)
// 		}
//		for _, cap := range neededCapabilities {
//			if !capabilities.Get(capability.EFFECTIVE, cap) {
//				// We miss a capability we need, create a user namespaces
//				unshare.MaybeReexecUsingUserNamespace(true)
//			}
//		}
//  		// rest of your code follows here
//  }
//
// TODO(runcom)
package image

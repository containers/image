// Package image provides libraries and commands to interact with containers images.
//
//    package main
//
//    import (
//    	"fmt"
//
//    	"github.com/containers/image/docker"
//    )
//
//    func main() {
//    	img := "fedora:latest
//    	client, err := docker.NewClient(img, "", false)
//    	if err != nil {
//    		panic(err)
//    	}
//    	img, err := docker.NewImage(img, client)
//    	if err != nil {
//    		panic(err)
//    	}
//    	b, err := img.Manifest()
//    	if err != nil {
//    		panic(err)
//    	}
//    	fmt.Printf("%s", string(b))
//    }
//
// TODO(runcom)
package image

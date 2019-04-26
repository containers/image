package dist

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/image"
	"github.com/containers/image/v5/transports"
	"github.com/containers/image/v5/types"
	"github.com/pkg/errors"
)

//nolint (gochecknoinits)
func init() {
	transports.Register(Transport)
}

// nolint (gochecknoglobals)
var Transport = distTransport{}

type distTransport struct{}

func (s distTransport) Name() string {
	return "dist"
}

func splitReference(ref string) (fullname, server string, port int, err error) {
	port = 8080
	err = nil

	if ref[0] == '/' {
		fullname = ref[1:]
		return fullname, "", port, nil
	}

	fields := strings.SplitN(ref, "/", 2)
	subFields := strings.Split(fields[0], ":")

	if len(subFields) > 2 {
		err = fmt.Errorf("bad server:port")
		return "", "", -1, err
	}

	server = subFields[0]

	if len(subFields) == 2 {
		port, err = strconv.Atoi(subFields[1])
		if err != nil {
			return "", "", -1, err
		}

		if port < 1 || port > 65535 {
			err = fmt.Errorf("bad port %d", port)
			return "", "", -1, err
		}
	}

	fullname = fields[1]

	return fullname, server, port, nil
}

// NOTE - the transport interface is defined in types/types.go.
// Valid uris are:
//    dist:///name1/name2/tag
//    dist://server/name1/name2/name3/tag
// The tag can be separated by either / or :
//    dist://server:port/name1/name2/name3/tag
//    dist://server:port/name1/name2/name3:tag
// So the reference passed in here would be e.g.
//    ///name1/name2/tag
//    //server:port/name1/name2/tag
func (s distTransport) ParseReference(reference string) (types.ImageReference, error) {
	if !strings.HasPrefix(reference, "//") {
		return nil, errors.Errorf("dist image reference %s does not start with //", reference)
	}

	fullname, server, port, err := splitReference(reference[2:])
	if err != nil {
		return nil, errors.Wrapf(err, "Failed parsing reference: '%s'", reference)
	}

	// support : for tag separateion
	var name, tag string

	fields := strings.Split(fullname, ":")

	if len(fields) != 2 || len(fields[0]) == 0 || len(fields[1]) == 0 {
		return nil, fmt.Errorf("no tag specified in '%s'", fullname)
	}

	name = fields[0]
	tag = fields[1]

	return distReference{
		server:   server,
		port:     port,
		fullname: fullname,
		name:     name,
		tag:      tag,
	}, nil
}

func (s distTransport) ValidatePolicyConfigurationScope(scope string) error {
	return nil
}

type distReference struct {
	server   string
	port     int
	fullname string
	name     string
	tag      string
}

func (ref distReference) Transport() types.ImageTransport {
	return Transport
}

func (ref distReference) StringWithinTransport() string {
	port := ""

	if ref.port != -1 {
		port = fmt.Sprintf("%d/", ref.port)
	}

	return fmt.Sprintf("//%s:%s%s", ref.server, port, ref.fullname)
}

func (ref distReference) DockerReference() reference.Named {
	return nil
}

func (ref distReference) PolicyConfigurationIdentity() string {
	return ref.StringWithinTransport()
}

func (ref distReference) PolicyConfigurationNamespaces() []string {
	return []string{}
}

func (ref distReference) NewImage(ctx context.Context, sys *types.SystemContext) (types.ImageCloser, error) {
	src, err := ref.NewImageSource(ctx, sys)
	if err != nil {
		return nil, err
	}

	return image.FromSource(ctx, sys, src)
}

func (ref distReference) NewImageSource(ctx context.Context, sys *types.SystemContext) (types.ImageSource, error) {
	s, err := NewOciRepo(&ref, sys)
	if err != nil {
		return nil, errors.Wrap(err, "Failed connecting to server")
	}

	return &distImageSource{
		ref: ref,
		s:   &s,
	}, nil
}

func (ref distReference) NewImageDestination(ctx context.Context,
	sys *types.SystemContext) (types.ImageDestination, error) {
	s, err := NewOciRepo(&ref, sys)
	if err != nil {
		return nil, errors.Wrap(err, "Failed connecting to server")
	}

	return &distImageDest{
		ref: ref,
		s:   &s,
	}, nil
}

func (ref distReference) DeleteImage(ctx context.Context, sys *types.SystemContext) error {
	s, err := NewOciRepo(&ref, sys)
	if err != nil {
		return errors.Wrap(err, "Failed connecting to server")
	}

	return s.RemoveManifest()
}

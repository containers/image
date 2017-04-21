package copy

import (
	"github.com/Sirupsen/logrus"
	"github.com/containers/image/manifest"
	"github.com/containers/image/types"
	"github.com/pkg/errors"
)

// preferredManifestMIMETypes lists manifest MIME types in order of our preference, if we can't use the original manifest and need to convert.
// Prefer v2s2 to v2s1 because v2s2 does not need to be changed when uploading to a different location.
// Include v2s1 signed but not v2s1 unsigned, because docker/distribution requires a signature even if the unsigned MIME type is used.
var preferredManifestMIMETypes = []string{manifest.DockerV2Schema2MediaType, manifest.DockerV2Schema1SignedMediaType}

// determineManifestConversion updates manifestUpdates to convert manifest to a supported MIME type, if necessary and canModifyManifest.
// Note that the conversion will only happen later, through src.UpdatedImage
// Returns the preferred manifest MIME type (whether we are converting to it or using it unmodified).
func determineManifestConversion(manifestUpdates *types.ManifestUpdateOptions, src types.Image, destSupportedManifestMIMETypes []string, canModifyManifest bool) (string, error) {
	_, srcType, err := src.Manifest()
	if err != nil { // This should have been cached?!
		return "", errors.Wrap(err, "Error reading manifest")
	}

	if len(destSupportedManifestMIMETypes) == 0 {
		return srcType, nil // Anything goes
	}
	supportedByDest := map[string]struct{}{}
	for _, t := range destSupportedManifestMIMETypes {
		supportedByDest[t] = struct{}{}
	}

	if _, ok := supportedByDest[srcType]; ok {
		logrus.Debugf("Manifest MIME type %s is declared supported by the destination", srcType)
		return srcType, nil
	}

	// OK, we should convert the manifest.
	if !canModifyManifest {
		logrus.Debugf("Manifest MIME type %s is not supported by the destination, but we can't modify the manifest, hoping for the best...")
		return srcType, nil // Take our chances - FIXME? Or should we fail without trying?
	}

	var chosenType = destSupportedManifestMIMETypes[0] // This one is known to be supported.
	for _, t := range preferredManifestMIMETypes {
		if _, ok := supportedByDest[t]; ok {
			chosenType = t
			break
		}
	}
	logrus.Debugf("Will convert manifest from MIME type %s to %s", srcType, chosenType)
	manifestUpdates.ManifestMIMEType = chosenType
	return chosenType, nil
}

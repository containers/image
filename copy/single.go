package copy

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/internal/image"
	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/signature"
	"github.com/containers/image/v5/transports"
	"github.com/containers/image/v5/types"
	digest "github.com/opencontainers/go-digest"
	"github.com/sirupsen/logrus"
)

// copyOneImage copies a single (non-manifest-list) image unparsedImage, using policyContext to validate
// source image admissibility.
func (c *copier) copyOneImage(ctx context.Context, policyContext *signature.PolicyContext, options *Options, unparsedToplevel, unparsedImage *image.UnparsedImage, targetInstance *digest.Digest) (retManifest []byte, retManifestType string, retManifestDigest digest.Digest, retErr error) {
	// The caller is handling manifest lists; this could happen only if a manifest list contains a manifest list.
	// Make sure we fail cleanly in such cases.
	multiImage, err := isMultiImage(ctx, unparsedImage)
	if err != nil {
		// FIXME FIXME: How to name a reference for the sub-image?
		return nil, "", "", fmt.Errorf("determining manifest MIME type for %s: %w", transports.ImageName(unparsedImage.Reference()), err)
	}
	if multiImage {
		return nil, "", "", fmt.Errorf("Unexpectedly received a manifest list instead of a manifest for a single image")
	}

	// Please keep this policy check BEFORE reading any other information about the image.
	// (The multiImage check above only matches the MIME type, which we have received anyway.
	// Actual parsing of anything should be deferred.)
	if allowed, err := policyContext.IsRunningImageAllowed(ctx, unparsedImage); !allowed || err != nil { // Be paranoid and fail if either return value indicates so.
		return nil, "", "", fmt.Errorf("Source image rejected: %w", err)
	}
	src, err := image.FromUnparsedImage(ctx, options.SourceCtx, unparsedImage)
	if err != nil {
		return nil, "", "", fmt.Errorf("initializing image from source %s: %w", transports.ImageName(c.rawSource.Reference()), err)
	}

	// If the destination is a digested reference, make a note of that, determine what digest value we're
	// expecting, and check that the source manifest matches it.  If the source manifest doesn't, but it's
	// one item from a manifest list that matches it, accept that as a match.
	destIsDigestedReference := false
	if named := c.dest.Reference().DockerReference(); named != nil {
		if digested, ok := named.(reference.Digested); ok {
			destIsDigestedReference = true
			matches, err := manifest.MatchesDigest(src.ManifestBlob, digested.Digest())
			if err != nil {
				return nil, "", "", fmt.Errorf("computing digest of source image's manifest: %w", err)
			}
			if !matches {
				manifestList, _, err := unparsedToplevel.Manifest(ctx)
				if err != nil {
					return nil, "", "", fmt.Errorf("reading manifest from source image: %w", err)
				}
				matches, err = manifest.MatchesDigest(manifestList, digested.Digest())
				if err != nil {
					return nil, "", "", fmt.Errorf("computing digest of source image's manifest: %w", err)
				}
				if !matches {
					return nil, "", "", errors.New("Digest of source image's manifest would not match destination reference")
				}
			}
		}
	}

	if err := checkImageDestinationForCurrentRuntime(ctx, options.DestinationCtx, src, c.dest); err != nil {
		return nil, "", "", err
	}

	sigs, err := c.sourceSignatures(ctx, src, options,
		"Getting image source signatures",
		"Checking if image destination supports signatures")
	if err != nil {
		return nil, "", "", err
	}

	// Determine if we're allowed to modify the manifest.
	// If we can, set to the empty string. If we can't, set to the reason why.
	// Compare, and perhaps keep in sync with, the version in copyMultipleImages.
	cannotModifyManifestReason := ""
	if len(sigs) > 0 {
		cannotModifyManifestReason = "Would invalidate signatures"
	}
	if destIsDigestedReference {
		cannotModifyManifestReason = "Destination specifies a digest"
	}
	if options.PreserveDigests {
		cannotModifyManifestReason = "Instructed to preserve digests"
	}

	ic := imageCopier{
		c:               c,
		manifestUpdates: &types.ManifestUpdateOptions{InformationOnly: types.ManifestUpdateInformation{Destination: c.dest}},
		src:             src,
		// diffIDsAreNeeded is computed later
		cannotModifyManifestReason: cannotModifyManifestReason,
		ociEncryptLayers:           options.OciEncryptLayers,
	}
	// Decide whether we can substitute blobs with semantic equivalents:
	// - Don’t do that if we can’t modify the manifest at all
	// - Ensure _this_ copy sees exactly the intended data when either processing a signed image or signing it.
	//   This may be too conservative, but for now, better safe than sorry, _especially_ on the len(c.signers) != 0 path:
	//   The signature makes the content non-repudiable, so it very much matters that the signature is made over exactly what the user intended.
	//   We do intend the RecordDigestUncompressedPair calls to only work with reliable data, but at least there’s a risk
	//   that the compressed version coming from a third party may be designed to attack some other decompressor implementation,
	//   and we would reuse and sign it.
	ic.canSubstituteBlobs = ic.cannotModifyManifestReason == "" && len(c.signers) == 0

	if err := ic.updateEmbeddedDockerReference(); err != nil {
		return nil, "", "", err
	}

	destRequiresOciEncryption := (isEncrypted(src) && ic.c.ociDecryptConfig != nil) || options.OciEncryptLayers != nil

	manifestConversionPlan, err := determineManifestConversion(determineManifestConversionInputs{
		srcMIMEType:                    ic.src.ManifestMIMEType,
		destSupportedManifestMIMETypes: ic.c.dest.SupportedManifestMIMETypes(),
		forceManifestMIMEType:          options.ForceManifestMIMEType,
		requiresOCIEncryption:          destRequiresOciEncryption,
		cannotModifyManifestReason:     ic.cannotModifyManifestReason,
	})
	if err != nil {
		return nil, "", "", err
	}
	// We set up this part of ic.manifestUpdates quite early, not just around the
	// code that calls copyUpdatedConfigAndManifest, so that other parts of the copy code
	// (e.g. the UpdatedImageNeedsLayerDiffIDs check just below) can make decisions based
	// on the expected destination format.
	if manifestConversionPlan.preferredMIMETypeNeedsConversion {
		ic.manifestUpdates.ManifestMIMEType = manifestConversionPlan.preferredMIMEType
	}

	// If src.UpdatedImageNeedsLayerDiffIDs(ic.manifestUpdates) will be true, it needs to be true by the time we get here.
	ic.diffIDsAreNeeded = src.UpdatedImageNeedsLayerDiffIDs(*ic.manifestUpdates)

	// If enabled, fetch and compare the destination's manifest. And as an optimization skip updating the destination iff equal
	if options.OptimizeDestinationImageAlreadyExists {
		shouldUpdateSigs := len(sigs) > 0 || len(c.signers) != 0 // TODO: Consider allowing signatures updates only and skipping the image's layers/manifest copy if possible
		noPendingManifestUpdates := ic.noPendingManifestUpdates()

		logrus.Debugf("Checking if we can skip copying: has signatures=%t, OCI encryption=%t, no manifest updates=%t", shouldUpdateSigs, destRequiresOciEncryption, noPendingManifestUpdates)
		if !shouldUpdateSigs && !destRequiresOciEncryption && noPendingManifestUpdates {
			isSrcDestManifestEqual, retManifest, retManifestType, retManifestDigest, err := compareImageDestinationManifestEqual(ctx, options, src, targetInstance, c.dest)
			if err != nil {
				logrus.Warnf("Failed to compare destination image manifest: %v", err)
				return nil, "", "", err
			}

			if isSrcDestManifestEqual {
				c.Printf("Skipping: image already present at destination\n")
				return retManifest, retManifestType, retManifestDigest, nil
			}
		}
	}

	if err := ic.copyLayers(ctx); err != nil {
		return nil, "", "", err
	}

	// With docker/distribution registries we do not know whether the registry accepts schema2 or schema1 only;
	// and at least with the OpenShift registry "acceptschema2" option, there is no way to detect the support
	// without actually trying to upload something and getting a types.ManifestTypeRejectedError.
	// So, try the preferred manifest MIME type with possibly-updated blob digests, media types, and sizes if
	// we're altering how they're compressed.  If the process succeeds, fine…
	manifestBytes, retManifestDigest, err := ic.copyUpdatedConfigAndManifest(ctx, targetInstance)
	retManifestType = manifestConversionPlan.preferredMIMEType
	if err != nil {
		logrus.Debugf("Writing manifest using preferred type %s failed: %v", manifestConversionPlan.preferredMIMEType, err)
		// … if it fails, and the failure is either because the manifest is rejected by the registry, or
		// because we failed to create a manifest of the specified type because the specific manifest type
		// doesn't support the type of compression we're trying to use (e.g. docker v2s2 and zstd), we may
		// have other options available that could still succeed.
		var manifestTypeRejectedError types.ManifestTypeRejectedError
		var manifestLayerCompressionIncompatibilityError manifest.ManifestLayerCompressionIncompatibilityError
		isManifestRejected := errors.As(err, &manifestTypeRejectedError)
		isCompressionIncompatible := errors.As(err, &manifestLayerCompressionIncompatibilityError)
		if (!isManifestRejected && !isCompressionIncompatible) || len(manifestConversionPlan.otherMIMETypeCandidates) == 0 {
			// We don’t have other options.
			// In principle the code below would handle this as well, but the resulting  error message is fairly ugly.
			// Don’t bother the user with MIME types if we have no choice.
			return nil, "", "", err
		}
		// If the original MIME type is acceptable, determineManifestConversion always uses it as manifestConversionPlan.preferredMIMEType.
		// So if we are here, we will definitely be trying to convert the manifest.
		// With ic.cannotModifyManifestReason != "", that would just be a string of repeated failures for the same reason,
		// so let’s bail out early and with a better error message.
		if ic.cannotModifyManifestReason != "" {
			return nil, "", "", fmt.Errorf("writing manifest failed and we cannot try conversions: %q: %w", cannotModifyManifestReason, err)
		}

		// errs is a list of errors when trying various manifest types. Also serves as an "upload succeeded" flag when set to nil.
		errs := []string{fmt.Sprintf("%s(%v)", manifestConversionPlan.preferredMIMEType, err)}
		for _, manifestMIMEType := range manifestConversionPlan.otherMIMETypeCandidates {
			logrus.Debugf("Trying to use manifest type %s…", manifestMIMEType)
			ic.manifestUpdates.ManifestMIMEType = manifestMIMEType
			attemptedManifest, attemptedManifestDigest, err := ic.copyUpdatedConfigAndManifest(ctx, targetInstance)
			if err != nil {
				logrus.Debugf("Upload of manifest type %s failed: %v", manifestMIMEType, err)
				errs = append(errs, fmt.Sprintf("%s(%v)", manifestMIMEType, err))
				continue
			}

			// We have successfully uploaded a manifest.
			manifestBytes = attemptedManifest
			retManifestDigest = attemptedManifestDigest
			retManifestType = manifestMIMEType
			errs = nil // Mark this as a success so that we don't abort below.
			break
		}
		if errs != nil {
			return nil, "", "", fmt.Errorf("Uploading manifest failed, attempted the following formats: %s", strings.Join(errs, ", "))
		}
	}
	if targetInstance != nil {
		targetInstance = &retManifestDigest
	}

	newSigs, err := c.createSignatures(ctx, manifestBytes, options.SignIdentity)
	if err != nil {
		return nil, "", "", err
	}
	sigs = append(sigs, newSigs...)

	c.Printf("Storing signatures\n")
	if err := c.dest.PutSignaturesWithFormat(ctx, sigs, targetInstance); err != nil {
		return nil, "", "", fmt.Errorf("writing signatures: %w", err)
	}

	return manifestBytes, retManifestType, retManifestDigest, nil
}

package manifest

// Largely based on
// https://github.com/moby/moby/blob/bc846d2e8fe5538220e0c31e9d0e8446f6fbc022/distribution/cpuinfo_unix.go
// https://github.com/containerd/containerd/blob/726dcaea50883e51b2ec6db13caff0e7936b711d/platforms/cpuinfo.go

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/containers/image/v5/types"
	imgspecv1 "github.com/opencontainers/image-spec/specs-go/v1"
)

// For Linux, the kernel has already detected the ABI, ISA and Features.
// So we don't need to access the ARM registers to detect platform information
// by ourselves. We can just parse these information from /proc/cpuinfo
func getCPUInfo(pattern string) (info string, err error) {
	if runtime.GOOS != "linux" {
		return "", fmt.Errorf("getCPUInfo for OS %s not implemented", runtime.GOOS)
	}

	cpuinfo, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return "", err
	}
	defer cpuinfo.Close()

	// Start to Parse the Cpuinfo line by line. For SMP SoC, we parse
	// the first core is enough.
	scanner := bufio.NewScanner(cpuinfo)
	for scanner.Scan() {
		newline := scanner.Text()
		list := strings.Split(newline, ":")

		if len(list) > 1 && strings.EqualFold(strings.TrimSpace(list[0]), pattern) {
			return strings.TrimSpace(list[1]), nil
		}
	}

	// Check whether the scanner encountered errors
	err = scanner.Err()
	if err != nil {
		return "", err
	}

	return "", fmt.Errorf("getCPUInfo for pattern: %s not found", pattern)
}

func getCPUVariant() string {
	if runtime.GOOS == "windows" {
		// Windows only supports v7 for ARM32 and v8 for ARM64 and so we can use
		// runtime.GOARCH to determine the variants
		var variant string
		switch runtime.GOARCH {
		case "arm64":
			variant = "v8"
		case "arm":
			variant = "v7"
		default:
			variant = "unknown"
		}

		return variant
	}

	variant, err := getCPUInfo("Cpu architecture")
	if err != nil {
		return ""
	}
	// TODO handle RPi Zero mismatch (https://github.com/moby/moby/pull/36121#issuecomment-398328286)

	switch strings.ToLower(variant) {
	case "8", "aarch64":
		variant = "v8"
	case "7", "7m", "?(12)", "?(13)", "?(14)", "?(15)", "?(16)", "?(17)":
		variant = "v7"
	case "6", "6tej":
		variant = "v6"
	case "5", "5t", "5te", "5tej":
		variant = "v5"
	case "4", "4t":
		variant = "v4"
	case "3":
		variant = "v3"
	default:
		variant = "unknown"
	}

	return variant
}

var compatibility = map[string][]string{
	"arm":   []string{"v7", "v6", "v5"},
	"arm64": []string{"v8"},
}

func WantedPlatforms(ctx *types.SystemContext) ([]imgspecv1.Platform, error) {
	wantedArch := runtime.GOARCH
	if ctx != nil && ctx.ArchitectureChoice != "" {
		wantedArch = ctx.ArchitectureChoice
	}
	wantedOS := runtime.GOOS
	if ctx != nil && ctx.OSChoice != "" {
		wantedOS = ctx.OSChoice
	}

	var wantedPlatforms []imgspecv1.Platform

	wantedVariant := ""
	if wantedArch == "arm" || wantedArch == "arm64" {
		if ctx != nil && ctx.VariantChoice != "" {
			wantedVariant = ctx.VariantChoice
		} else {
			// TODO handle Variant == 'unknown'
			wantedVariant = getCPUVariant()
		}
	}

	if wantedVariant != "" && compatibility[wantedArch] != nil {
		wantedPlatforms = make([]imgspecv1.Platform, 0, len(compatibility[wantedArch]))
		for _, v := range compatibility[wantedArch] {
			if wantedVariant >= v {
				wantedPlatforms = append(wantedPlatforms, imgspecv1.Platform{
					OS:           wantedOS,
					Architecture: wantedArch,
					Variant:      v,
				})
			}
		}
	} else {
		wantedPlatforms = []imgspecv1.Platform{
			imgspecv1.Platform{
				OS:           wantedOS,
				Architecture: wantedArch,
				Variant:      wantedVariant,
			},
		}
	}

	return wantedPlatforms, nil
}

func MatchesPlatform(image Schema2PlatformSpec, wanted imgspecv1.Platform) bool {
	if image.Architecture != wanted.Architecture {
		return false
	}
	if image.OS != wanted.OS {
		return false
	}

	if wanted.Variant == "" || image.Variant == wanted.Variant {
		return true
	}

	return false
}

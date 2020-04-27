package manifest

// dupStringSlice returns a deep copy of a slice of strings, or nil if the
// source slice is empty.
func dupStringSlice(list []string) []string {
	if len(list) == 0 {
		return nil
	}
	dup := make([]string, len(list))
	copy(dup, list)
	return dup
}

// dupStringStringMap returns a deep copy of a map[string]string, or nil if the
// passed-in map is nil or has no keys.
func dupStringStringMap(m map[string]string) map[string]string {
	if len(m) == 0 {
		return nil
	}
	result := make(map[string]string)
	for k, v := range m {
		result[k] = v
	}
	return result
}

// layerInfosToStrings converts a list of layer infos, presumably obtained from a Manifest.LayerInfos()
// method call, into a format suitable for inclusion in a types.ImageInspectInfo structure.
func layerInfosToStrings(infos []LayerInfo) []string {
	layers := make([]string, len(infos))
	for i, info := range infos {
		layers[i] = info.Digest.String()
	}
	return layers
}

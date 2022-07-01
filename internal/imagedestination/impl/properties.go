package impl

// Properties collects properties of an ImageDestination that are constant throughout its lifetime
// (but might differ across instances).
type Properties struct {
	// MustMatchRuntimeOS is set to true if the destination can store only images targeted for the current runtime architecture and OS.
	MustMatchRuntimeOS bool
	// IgnoresEmbeddedDockerReference is set to true if the destination does not care about Image.EmbeddedDockerReferenceConflicts(),
	// and would prefer to receive an unmodified manifest instead of one modified for the destination.
	// Does not make a difference if Reference().DockerReference() is nil.
	IgnoresEmbeddedDockerReference bool
	// HasThreadSafePutBlob indicates that PutBlob can be executed concurrently.
	HasThreadSafePutBlob bool
}

// PropertyMethodsInitialize implements parts of private.ImageDestination corresponding to Properties.
type PropertyMethodsInitialize struct {
	// We need two separate structs, PropertyMethodsInitialize and Properties, because Go prohibits fields and methods with the same name.

	vals Properties
}

// PropertyMethods creates an PropertyMethodsInitialize for vals.
func PropertyMethods(vals Properties) PropertyMethodsInitialize {
	return PropertyMethodsInitialize{
		vals: vals,
	}
}

// MustMatchRuntimeOS returns true iff the destination can store only images targeted for the current runtime architecture and OS. False otherwise.
func (o PropertyMethodsInitialize) MustMatchRuntimeOS() bool {
	return o.vals.MustMatchRuntimeOS
}

// IgnoresEmbeddedDockerReference() returns true iff the destination does not care about Image.EmbeddedDockerReferenceConflicts(),
// and would prefer to receive an unmodified manifest instead of one modified for the destination.
// Does not make a difference if Reference().DockerReference() is nil.
func (o PropertyMethodsInitialize) IgnoresEmbeddedDockerReference() bool {
	return o.vals.IgnoresEmbeddedDockerReference
}

// HasThreadSafePutBlob indicates whether PutBlob can be executed concurrently.
func (o PropertyMethodsInitialize) HasThreadSafePutBlob() bool {
	return o.vals.HasThreadSafePutBlob
}

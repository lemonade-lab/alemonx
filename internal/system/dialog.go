package system

// PickerKind is deliberately small: a system plugin may ask the workbench Web
// Finder to select files or directories, but cannot turn it into an arbitrary
// command launcher.
type PickerKind string

const (
	PickerDirectory PickerKind = "directory"
	PickerFile      PickerKind = "file"
)

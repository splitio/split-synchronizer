package storage

// Snapshotter interface to be implemented by storages that allow full retrieval of all raw data
type Snapshotter interface {
	GetRawSnapshot() ([]byte, error)
}

// ContentIdentifiedSnapshotter is a Snapshotter that can also return a stable identifier of the logical content it exports,
// computed from the same read as the exported bytes.
type ContentIdentifiedSnapshotter interface {
	Snapshotter
	GetRawSnapshotWithContentID(flagSpecVersion string, flagSets []string) ([]byte, string, error)
}

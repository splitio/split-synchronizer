package storage

// Snapshotter interface to be implemented by storages that allow full retrieval of all raw data
type Snapshotter interface {
	GetRawSnapshot() ([]byte, error)
}

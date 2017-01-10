// Package storage implements different kind of storages for split information
package storage

// SplitStorage interface defines the split data storage
type SplitStorage interface {
	Save(key string, split interface{}) error
}

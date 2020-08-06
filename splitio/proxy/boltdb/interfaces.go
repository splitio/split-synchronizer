package boltdb

// CollectionItem is the item into a collection
type CollectionItem interface {
	SetID(id uint64)
	ID() uint64
}

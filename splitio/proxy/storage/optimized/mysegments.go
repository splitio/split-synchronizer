package optimized

import (
	"fmt"
	"strings"
	"sync"

	"github.com/splitio/go-toolkit/v5/datastructures/set"
)

// MySegmentsCache defines the interface for a per-user optimized segment storage
type MySegmentsCache interface {
	Update(name string, toAdd *set.ThreadUnsafeSet, toRemove *set.ThreadUnsafeSet) error
	SegmentsForUser(key string) []string
	KeyCount() int
}

// MySegmentsCacheImpl implements the MySegmentsCache interface
type MySegmentsCacheImpl struct {
	mySegments map[string][]string
	mutex      *sync.RWMutex
}

// NewMySegmentsCache constructs a new MySegments cache
func NewMySegmentsCache() *MySegmentsCacheImpl {
	return &MySegmentsCacheImpl{
		mySegments: make(map[string][]string),
		mutex:      &sync.RWMutex{},
	}
}

// KeyCount retuns the amount of keys who belong to at least one segment
func (m *MySegmentsCacheImpl) KeyCount() int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return len(m.mySegments)
}

// SegmentsForUser returns the list of segments a certain user belongs to
func (m *MySegmentsCacheImpl) SegmentsForUser(key string) []string {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	userSegments, ok := m.mySegments[key]
	if !ok {
		return []string{}
	}
	return userSegments
}

// Update adds and removes segments to keys
func (m *MySegmentsCacheImpl) Update(name string, toAdd *set.ThreadUnsafeSet, toRemove *set.ThreadUnsafeSet) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	invalidAdded := []string{}
	invalidRemoved := []string{}
	for _, addedKey := range toAdd.List() {
		strKey, ok := addedKey.(string)
		if !ok {
			invalidAdded = append(invalidAdded, fmt.Sprintf("%T::%+v", addedKey, addedKey))
			continue
		}
		m.addSegmentToUser(strKey, name)
	}

	for _, removedKey := range toRemove.List() {
		strKey, ok := removedKey.(string)
		if !ok {
			invalidRemoved = append(invalidRemoved, fmt.Sprintf("%T::%+v", removedKey, removedKey))
			continue
		}
		m.removeSegmentForUser(strKey, name)
	}

	if len(invalidAdded) > 0 || len(invalidRemoved) > 0 {
		return fmt.Errorf("invalid added and removed keys found: %s // %s",
			strings.Join(invalidAdded, ","), strings.Join(invalidRemoved, ","))
	}

	return nil
}

func (m *MySegmentsCacheImpl) addSegmentToUser(key string, segment string) {
	toAdd := []string{segment}
	userSegments, ok := m.mySegments[key]
	if ok {
		if m.isInSegment(segment, userSegments) {
			return
		}
		toAdd = append(userSegments, toAdd...)
	}
	m.mySegments[key] = toAdd
}

func (m *MySegmentsCacheImpl) removeSegmentForUser(key string, segment string) {
	userSegments, ok := m.mySegments[key]
	if !ok {
		return
	}
	toUpdate := make([]string, 0)
	for _, s := range userSegments {
		if s != segment {
			toUpdate = append(toUpdate, s)
		}
	}
	if len(toUpdate) == 0 {
		delete(m.mySegments, key)
		return
	}
	m.mySegments[key] = toUpdate
}

func (m *MySegmentsCacheImpl) isInSegment(segment string, segments []string) bool {
	for _, s := range segments {
		if s == segment {
			return true
		}
	}
	return false
}

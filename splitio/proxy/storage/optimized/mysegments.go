package optimized

import "sync"

// MySegmentsCache defines the interface for a per-user optimized segment storage
type MySegmentsCache interface {
	AddSegmentToUser(key string, segment string)
	RemoveSegmentForUser(key string, segment string)
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

// AddSegmentToUser adds a segment for a particular user
func (m *MySegmentsCacheImpl) AddSegmentToUser(key string, segment string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
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

// RemoveSegmentForUser removes a segment for a particular user
func (m *MySegmentsCacheImpl) RemoveSegmentForUser(key string, segment string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
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

func (m *MySegmentsCacheImpl) isInSegment(segment string, segments []string) bool {
	for _, s := range segments {
		if s == segment {
			return true
		}
	}
	return false
}

package v2

import "sync"

type MySegmentsCache struct {
	mySegments map[string]*[]string
	mutex      *sync.RWMutex
}

func NewMySegmentsCache() *MySegmentsCache {
	return &MySegmentsCache{
		mySegments: make(map[string]*[]string),
		mutex:      &sync.RWMutex{},
	}
}

func (m *MySegmentsCache) isInSegment(segment string, segments []string) bool {
	for _, s := range segments {
		if s == segment {
			return true
		}
	}
	return false
}

func (m *MySegmentsCache) AddSegmentToUser(key string, segment string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	toAdd := []string{segment}
	userSegments, ok := m.mySegments[key]
	if ok {
		if m.isInSegment(segment, *userSegments) {
			return
		}
		toAdd = append(*userSegments, toAdd...)
	}
	m.mySegments[key] = &toAdd
}

func (m *MySegmentsCache) GetSegmentsForUser(key string) *[]string {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.mySegments[key]
}

func (m *MySegmentsCache) RemoveSegmentForUser(key string, segment string) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	userSegments, ok := m.mySegments[key]
	if !ok {
		return
	}
	toUpdate := make([]string, 0)
	for _, s := range *userSegments {
		if s != segment {
			toUpdate = append(toUpdate, s)
		}
	}
	if len(toUpdate) == 0 {
		delete(m.mySegments, key)
		return
	}
	m.mySegments[key] = &toUpdate
}

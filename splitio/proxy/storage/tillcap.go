package storage

import "sync"

// tillCaps holds the restore-time ceilings on the till served to SDKs. After a restore from Redis, items above a
// committed cursor can be present (the writer freezes a cursor while later data keeps writing). They stay in the
// storage, but until the first sync succeeds the till served for incremental requests is capped at the cursor, so
// an SDK asks again from the cursor and gets them redelivered by the sync. The zero value is ready to use.
type tillCaps struct {
	mtx  sync.RWMutex
	caps map[string]int64
}

// set caps the till served for key at cn.
func (t *tillCaps) set(key string, cn int64) {
	t.mtx.Lock()
	defer t.mtx.Unlock()
	if t.caps == nil {
		t.caps = map[string]int64{}
	}
	t.caps[key] = cn
}

// lift removes every ceiling.
func (t *tillCaps) lift() {
	t.mtx.Lock()
	t.caps = nil
	t.mtx.Unlock()
}

// apply returns till limited to the ceiling for key, and never below since. It returns till unchanged when key has no
// ceiling.
func (t *tillCaps) apply(key string, till, since int64) int64 {
	t.mtx.RLock()
	c, ok := t.caps[key]
	t.mtx.RUnlock()
	if !ok || till <= c {
		return till
	}
	return max(c, since)
}

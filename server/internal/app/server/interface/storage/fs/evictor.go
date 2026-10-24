package fs

import (
	"container/list"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/RCM7/stashito/server/internal/app/server/metrics"
)

const touchPersistInterval = time.Minute

type entryKind string

const (
	kindBlob     entryKind = "blob"
	kindManifest entryKind = "manifest"
)

type entry struct {
	key         string
	kind        entryKind
	size        int64
	persistedAt time.Time
	cold        bool
}

func (e *entry) files() []string {
	if e.kind == kindManifest {
		return []string{e.key + ".meta", e.key + ".body"}
	}
	return []string{e.key}
}

func (e *entry) mtimeFile() string {
	if e.kind == kindManifest {
		return e.key + ".body"
	}
	return e.key
}

// evictor keeps an LRU index of everything under the cache root and deletes
// the least recently used entries whenever committed plus in-flight bytes
// would exceed max, freeing down to low so evictions come in batches. Last
// access is kept in memory and flushed to the file mtime at most once per
// touchPersistInterval so the order survives restarts.
type evictor struct {
	root string
	max  int64
	low  int64

	mu       sync.Mutex
	lru      *list.List
	entries  map[string]*list.Element
	used     int64
	inflight int64
}

func newEvictor(root string, max, headroom int64) (*evictor, error) {
	e := &evictor{
		root:    root,
		max:     max,
		low:     max - headroom,
		lru:     list.New(),
		entries: make(map[string]*list.Element),
	}
	start := time.Now()
	scanned, err := scanEntries(root)
	if err != nil {
		return nil, err
	}
	for _, en := range scanned {
		e.entries[en.key] = e.lru.PushFront(en)
		e.used += en.size
	}
	slog.Info("storage index built", "entries", len(scanned), "used_bytes", e.used, "max_bytes", max, "low_bytes", e.low, "duration", time.Since(start))
	metrics.StorageMaxBytes.Set(float64(max))
	e.evictIfNeeded()
	return e, nil
}

type scannedEntry struct {
	*entry
	mtime time.Time
}

func scanEntries(root string) ([]*entry, error) {
	byKey := make(map[string]*scannedEntry)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == root {
				return err
			}
			return nil
		}
		if d.IsDir() || strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		key, kind := path, kindBlob
		switch {
		case strings.HasSuffix(path, ".meta"), strings.HasSuffix(path, ".body"):
			key, kind = strings.TrimSuffix(strings.TrimSuffix(path, ".meta"), ".body"), kindManifest
		case !strings.Contains(path, string(filepath.Separator)+"blobs"+string(filepath.Separator)):
			return nil
		}
		se, ok := byKey[key]
		if !ok {
			se = &scannedEntry{entry: &entry{key: key, kind: kind}}
			byKey[key] = se
		}
		se.size += info.Size()
		if info.ModTime().After(se.mtime) {
			se.mtime = info.ModTime()
		}
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	all := make([]*scannedEntry, 0, len(byKey))
	for _, se := range byKey {
		se.persistedAt = se.mtime
		all = append(all, se)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].mtime.Before(all[j].mtime) })
	out := make([]*entry, len(all))
	for i, se := range all {
		out[i] = se.entry
	}
	return out, nil
}

func (e *evictor) touch(key string) {
	if e == nil {
		return
	}
	now := time.Now()
	e.mu.Lock()
	el, ok := e.entries[key]
	if !ok {
		e.mu.Unlock()
		return
	}
	en := el.Value.(*entry)
	if en.cold {
		e.mu.Unlock()
		return
	}
	e.lru.MoveToFront(el)
	persist := now.Sub(en.persistedAt) >= touchPersistInterval
	if persist {
		en.persistedAt = now
	}
	file := en.mtimeFile()
	e.mu.Unlock()
	if persist {
		_ = os.Chtimes(file, now, now)
	}
}

// add records a committed entry at the hot end of the index. A cold entry
// (one larger than max on its own) goes to the cold end instead and is the
// first thing the next eviction removes.
func (e *evictor) add(key string, kind entryKind, size int64, cold bool) {
	if e == nil {
		return
	}
	e.mu.Lock()
	if el, ok := e.entries[key]; ok {
		old := el.Value.(*entry)
		e.used -= old.size
		e.lru.Remove(el)
	}
	en := &entry{key: key, kind: kind, size: size, persistedAt: time.Now(), cold: cold}
	if cold {
		e.entries[key] = e.lru.PushBack(en)
	} else {
		e.entries[key] = e.lru.PushFront(en)
	}
	e.used += size
	e.mu.Unlock()
	if !cold {
		e.evictIfNeeded()
	}
}

// fits reports whether a blob of size bytes can ever be kept under the cap.
func (e *evictor) fits(size int64) bool {
	return e == nil || size <= e.max
}

// reserve accounts for bytes of a blob still being downloaded, announced or
// already written, and evicts right away if they push the cache past max.
func (e *evictor) reserve(delta int64) {
	if e == nil || delta == 0 {
		return
	}
	e.mu.Lock()
	e.inflight += delta
	e.mu.Unlock()
	e.evictIfNeeded()
}

func (e *evictor) release(n int64) {
	if e == nil || n == 0 {
		return
	}
	e.mu.Lock()
	e.inflight -= n
	e.mu.Unlock()
}

// freeDisk evicts one entry at a time until the filesystem under dir has at
// least need bytes free, or nothing is left to evict. Returns the free bytes
// measured last.
func (e *evictor) freeDisk(dir string, need int64) int64 {
	free, _ := diskFree(dir)
	if e == nil {
		return free
	}
	for free < need {
		popped := false
		e.mu.Lock()
		victims := e.popWhileLocked(func() bool {
			if popped {
				return false
			}
			popped = true
			return true
		})
		used, inflight := e.used, e.inflight
		e.mu.Unlock()
		if len(victims) == 0 {
			return free
		}
		e.deleteVictims(victims, used, inflight)
		free, _ = diskFree(dir)
	}
	return free
}

func (e *evictor) evictIfNeeded() {
	e.mu.Lock()
	victims := e.popWhileLocked(e.overLocked(0))
	used, inflight := e.used, e.inflight
	e.mu.Unlock()
	e.deleteVictims(victims, used, inflight)
}

func (e *evictor) overLocked(extra int64) func() bool {
	if e.used+e.inflight+extra <= e.max {
		return func() bool { return false }
	}
	return func() bool { return e.used+e.inflight+extra > e.low }
}

func (e *evictor) popWhileLocked(more func() bool) []*entry {
	var victims []*entry
	for e.lru.Len() > 0 && more() {
		el := e.lru.Back()
		en := el.Value.(*entry)
		e.lru.Remove(el)
		delete(e.entries, en.key)
		e.used -= en.size
		victims = append(victims, en)
	}
	return victims
}

func (e *evictor) deleteVictims(victims []*entry, used, inflight int64) {
	if len(victims) == 0 {
		return
	}
	var freed int64
	for _, en := range victims {
		for _, f := range en.files() {
			if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
				slog.Error("failed to evict cache entry", "path", f, "error", err)
			}
		}
		freed += en.size
		metrics.Evictions.WithLabelValues(string(en.kind)).Inc()
		metrics.EvictedBytes.Add(float64(en.size))
		slog.Debug("evicted cache entry", "type", en.kind, "key", strings.TrimPrefix(en.key, e.root), "size", en.size)
	}
	slog.Info("evicted cache entries", "entries", len(victims), "freed_bytes", freed, "used_bytes", used, "inflight_bytes", inflight, "max_bytes", e.max, "low_bytes", e.low)
}

func (e *evictor) usedBytes() int64 {
	if e == nil {
		return 0
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.used
}

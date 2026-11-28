package golang

import (
	"bytes"
	"cmp"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/renameio/v2/maybe"
)

// The content cache of docs/design.md §6.3 keeps what an expensive step
// computed, under the SHA-256 of a key that holds the content of its inputs.
// Equal inputs read the entry instead of computing it again. Entries are
// written atomically, so processes running at the same time share the
// cache. An entry that exists but cannot be read is an error that names it,
// never a silent miss.
//
// The API, for every step of this backend that caches:
//
//   - sharedCache() is the cache: the directory UNICODE_CACHE names, or
//     unicode/ in the user's cache directory; UNICODE_CACHE=off turns it off.
//   - get and put read and write an entry of bytes. A kind is a subdirectory;
//     a new kind is added to cacheKinds so that trim looks after it.
//   - getChecked and putChecked read and write an entry together with the
//     files it was computed from (an inputSet); a read checks those files
//     again and answers a miss when any of them changed.
//   - getFile and putFile copy a file, keeping its permission bits.
//
// A cache that cannot be written only loses the entry.

const (
	// cacheTrimInterval is how often a process removes stale entries.
	cacheTrimInterval = 24 * time.Hour
	// cacheUnused is how long an entry stays without being read.
	cacheUnused = 5 * 24 * time.Hour
	// cacheLimit bounds the bytes of this backend's entries; trim removes the
	// least recently read beyond it.
	cacheLimit = 512 << 20
	// cacheTouchInterval keeps reads from rewriting an entry's time on every hit.
	cacheTouchInterval = time.Hour
)

// The kinds of entries this backend keeps, each in a subdirectory of its own.
const (
	cacheExtraction = "go-extract"
	cachePlanned    = "go-planned"
	cacheTool       = "go-tool"
	cacheEffects    = "go-effects"
)

// cacheKinds are the subdirectories trim looks after; the cache directory
// may hold other tools' entries, which it leaves alone.
var cacheKinds = []string{cacheExtraction, cachePlanned, cacheTool, cacheEffects}

// contentCache is a directory of entries; the empty directory is the cache
// turned off.
type contentCache struct{ dir string }

var sharedCache = sync.OnceValues(func() (contentCache, error) {
	dir := os.Getenv("UNICODE_CACHE")
	if dir == "off" {
		return contentCache{}, nil
	}
	if dir == "" {
		base, err := os.UserCacheDir()
		if err != nil {
			return contentCache{}, fmt.Errorf("no directory for the content cache: %w; set UNICODE_CACHE to a directory, or to off", err)
		}
		dir = filepath.Join(base, "unicode")
	}
	c := contentCache{dir: dir}
	c.trim(time.Now())
	return c, nil
})

// entryPath is where the entry of kind for key lives.
func (c contentCache) entryPath(kind, key string) string {
	sum := sha256.Sum256([]byte(key))
	name := hex.EncodeToString(sum[:])
	return filepath.Join(c.dir, kind, name[:2], name)
}

// get reads the entry of kind for key.
func (c contentCache) get(kind, key string) ([]byte, bool, error) {
	if c.dir == "" {
		return nil, false, nil
	}
	path := c.entryPath(kind, key)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, c.corrupt(path, err)
	}
	c.touch(path)
	return data, true, nil
}

// put writes the entry of kind for key.
func (c contentCache) put(kind, key string, data []byte) {
	if c.dir == "" {
		return
	}
	path := c.entryPath(kind, key)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	_ = maybe.WriteFile(path, data, 0o644)
}

// checkedEntry is an entry with the files it was computed from, stored as
// gzip-compressed JSON.
type checkedEntry struct {
	Inputs inputSet        `json:"inputs"`
	Value  json.RawMessage `json:"value"`
}

// getChecked reads the entry of kind for key and checks its input set
// against root, with the files of overlay read as if written: a changed
// input is a miss.
func (c contentCache) getChecked(kind, key, root string, overlay map[string][]byte) (json.RawMessage, bool, error) {
	data, hit, err := c.get(kind, key)
	if err != nil || !hit {
		return nil, false, err
	}
	var entry checkedEntry
	zipped, err := gzip.NewReader(bytes.NewReader(data))
	if err == nil {
		err = json.NewDecoder(zipped).Decode(&entry)
	}
	if err != nil || entry.Inputs == nil || entry.Value == nil {
		return nil, false, c.corrupt(c.entryPath(kind, key), cmp.Or(err, errors.New("it lacks its inputs or its value")))
	}
	if !entry.Inputs.current(root, overlay) {
		return nil, false, nil
	}
	return entry.Value, true, nil
}

// putChecked writes value as the entry of kind for key, with its inputs.
func (c contentCache) putChecked(kind, key string, inputs inputSet, value any) {
	if c.dir == "" {
		return
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return
	}
	var out bytes.Buffer
	zipped, err := gzip.NewWriterLevel(&out, gzip.BestSpeed)
	if err != nil {
		return
	}
	if json.NewEncoder(zipped).Encode(checkedEntry{Inputs: inputs, Value: encoded}) != nil || zipped.Close() != nil {
		return
	}
	c.put(kind, key, out.Bytes())
}

// getFile copies the entry of kind for key to target with the entry's
// permission bits, and tells whether there was one.
func (c contentCache) getFile(kind, key, target string) (bool, error) {
	if c.dir == "" {
		return false, nil
	}
	path := c.entryPath(kind, key)
	in, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, c.corrupt(path, err)
	}
	defer func() { _ = in.Close() }()
	info, err := in.Stat()
	if err != nil {
		return false, c.corrupt(path, err)
	}
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return false, err
	}
	_, copyErr := io.Copy(out, in)
	if closeErr := out.Close(); copyErr != nil || closeErr != nil {
		_ = os.Remove(target)
		return false, c.corrupt(path, cmp.Or(copyErr, closeErr))
	}
	c.touch(path)
	return true, nil
}

// putFile copies the file at source into the entry of kind for key, keeping
// its permission bits.
func (c contentCache) putFile(kind, key, source string) {
	if c.dir == "" {
		return
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return
	}
	info, err := os.Stat(source)
	if err != nil {
		return
	}
	path := c.entryPath(kind, key)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	_ = maybe.WriteFile(path, data, info.Mode().Perm())
}

// corrupt is the error for an entry that exists but cannot be read.
func (c contentCache) corrupt(path string, err error) error {
	return fmt.Errorf("the content cache entry %s cannot be read: %v; delete it, or the cache directory %s", path, err, c.dir)
}

// touch marks the entry at path as read, at most once every cacheTouchInterval.
func (c contentCache) touch(path string) {
	if info, err := os.Stat(path); err == nil && time.Since(info.ModTime()) > cacheTouchInterval {
		now := time.Now()
		_ = os.Chtimes(path, now, now)
	}
}

// trim removes this backend's entries that no process has read for
// cacheUnused, then the least recently read ones beyond cacheLimit bytes,
// at most once every cacheTrimInterval; go-trim.txt holds when it last did.
func (c contentCache) trim(now time.Time) {
	marker := filepath.Join(c.dir, "go-trim.txt")
	if data, err := os.ReadFile(marker); err == nil {
		if last, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64); err == nil && now.Sub(time.Unix(last, 0)) < cacheTrimInterval {
			return
		}
	}
	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return
	}
	if err := maybe.WriteFile(marker, []byte(strconv.FormatInt(now.Unix(), 10)+"\n"), 0o644); err != nil {
		return
	}
	type entry struct {
		path string
		size int64
		read time.Time
	}
	var kept []entry
	var total int64
	for _, kind := range cacheKinds {
		_ = filepath.WalkDir(filepath.Join(c.dir, kind), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					return nil
				}
				return err
			}
			if d.IsDir() {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			if now.Sub(info.ModTime()) > cacheUnused {
				_ = os.Remove(path)
				return nil
			}
			kept = append(kept, entry{path, info.Size(), info.ModTime()})
			total += info.Size()
			return nil
		})
	}
	slices.SortFunc(kept, func(a, b entry) int { return a.read.Compare(b.read) })
	for _, e := range kept {
		if total <= cacheLimit {
			break
		}
		if os.Remove(e.path) == nil {
			total -= e.size
		}
	}
}

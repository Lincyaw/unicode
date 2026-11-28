package golang

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/renameio/v2/maybe"
)

// The content cache of docs/design.md §6.3 keeps what an expensive step
// computed from its inputs, under the SHA-256 of a key that holds the
// content of every input. Equal inputs read the entry instead of computing
// it again; any change of an input changes the key. Entries are written
// atomically, so processes running at the same time share the cache; an
// entry that cannot be read is a miss.
//
// UNICODE_CACHE names the cache's directory; "off" turns the cache off. By
// default it is unicode/ in the user's cache directory.

const (
	// cacheTrimInterval is how often a process removes stale entries.
	cacheTrimInterval = 24 * time.Hour
	// cacheUnused is how long an entry stays without being read.
	cacheUnused = 5 * 24 * time.Hour
	// cacheTouchInterval keeps reads from rewriting an entry's time on every hit.
	cacheTouchInterval = time.Hour
)

// The kinds of entries this backend keeps, each in a subdirectory of its own.
const (
	cacheExtraction = "go-extract"
	cachePlanned    = "go-planned"
	cacheTool       = "go-tool"
)

// cacheKinds are the subdirectories trim looks after; the cache directory
// may hold other tools' entries, which it leaves alone.
var cacheKinds = []string{cacheExtraction, cachePlanned, cacheTool}

// contentCache is a directory of entries; the empty directory is the cache
// turned off.
type contentCache struct{ dir string }

var sharedCache = sync.OnceValue(func() contentCache {
	dir := os.Getenv("UNICODE_CACHE")
	if dir == "off" {
		return contentCache{}
	}
	if dir == "" {
		base, err := os.UserCacheDir()
		if err != nil {
			return contentCache{}
		}
		dir = filepath.Join(base, "unicode")
	}
	c := contentCache{dir: dir}
	c.trim(time.Now())
	return c
})

// entryPath is where the entry of kind for key lives.
func (c contentCache) entryPath(kind, key string) string {
	sum := sha256.Sum256([]byte(key))
	name := hex.EncodeToString(sum[:])
	return filepath.Join(c.dir, kind, name[:2], name)
}

// get reads the entry of kind for key.
func (c contentCache) get(kind, key string) ([]byte, bool) {
	if c.dir == "" {
		return nil, false
	}
	path := c.entryPath(kind, key)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	if info, err := os.Stat(path); err == nil && time.Since(info.ModTime()) > cacheTouchInterval {
		now := time.Now()
		_ = os.Chtimes(path, now, now)
	}
	return data, true
}

// put writes the entry of kind for key. A cache that cannot be written only
// loses the entry: the result it holds is computed again next time.
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

// copyEntry copies the entry of kind for key to target with the entry's
// permission bits, and tells whether there was one.
func (c contentCache) copyEntry(kind, key, target string) bool {
	if c.dir == "" {
		return false
	}
	path := c.entryPath(kind, key)
	in, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = in.Close() }()
	info, err := in.Stat()
	if err != nil {
		return false
	}
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return false
	}
	_, copyErr := io.Copy(out, in)
	if closeErr := out.Close(); copyErr != nil || closeErr != nil {
		_ = os.Remove(target)
		return false
	}
	if time.Since(info.ModTime()) > cacheTouchInterval {
		now := time.Now()
		_ = os.Chtimes(path, now, now)
	}
	return true
}

// trim removes this backend's entries that no process has read for
// cacheUnused, at most once every cacheTrimInterval; go-trim.txt holds when
// it last did.
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
	for _, kind := range cacheKinds {
		_ = filepath.WalkDir(filepath.Join(c.dir, kind), func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					return nil
				}
				return err
			}
			if entry.IsDir() {
				return nil
			}
			if info, err := entry.Info(); err == nil && now.Sub(info.ModTime()) > cacheUnused {
				_ = os.Remove(path)
			}
			return nil
		})
	}
}

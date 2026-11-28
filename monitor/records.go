package monitor

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// maxCollected is how many collected calls one operation keeps.
const maxCollected = 1000

// writeRecord writes a run record under dir/runs, named by the SHA-256 of its
// content, so that replicas writing at once never overwrite each other.
func writeRecord(dir string, record Record) error {
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	return publish(filepath.Join(dir, "runs"), hex.EncodeToString(sum[:])+".json", data)
}

// writeTraffic writes a collected call under dir/traffic/<op>/<hash>.json,
// the hash being that of its receiver and call value, so that an input is
// kept once. It keeps at most maxCollected for one operation, counting the
// collected calls already on disk, but no temporary file left by an
// interrupted write, the first time it writes for the operation; it reports
// whether the operation is full.
func writeTraffic(dir string, collected Collected, counts map[string]int) (bool, error) {
	if strings.ContainsAny(collected.Op, `/\`) || collected.Op == "" || strings.HasPrefix(collected.Op, ".") {
		return false, errors.New("the operation identity is not a directory name")
	}
	directory := filepath.Join(dir, "traffic", collected.Op)
	count, known := counts[collected.Op]
	if !known {
		listed, err := os.ReadDir(directory)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return false, err
		}
		for _, entry := range listed {
			if strings.HasSuffix(entry.Name(), ".json") && !strings.HasPrefix(entry.Name(), pendingPrefix) {
				count++
			}
		}
		counts[collected.Op] = count
	}
	if count >= maxCollected {
		return true, nil
	}
	name := CollectedHash(collected) + ".json"
	if _, err := os.Lstat(filepath.Join(directory, name)); err == nil {
		return false, nil
	}
	if err := publish(directory, name, EncodeCollected(collected)); err != nil {
		return false, err
	}
	counts[collected.Op] = count + 1
	return count+1 >= maxCollected, nil
}

// pendingPrefix names the temporary file of a write in progress.
const pendingPrefix = ".pending-"

// publish writes data to directory/name through a temporary file renamed into
// place; a file already there is left as it is, its name being its content.
func publish(directory, name string, data []byte) error {
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	target := filepath.Join(directory, name)
	if _, err := os.Lstat(target); err == nil {
		return nil
	}
	temp, err := os.CreateTemp(directory, pendingPrefix)
	if err != nil {
		return err
	}
	_, writeErr := temp.Write(data)
	modeErr := temp.Chmod(0o644)
	closeErr := temp.Close()
	// A pending file that cannot be removed after a failed write is left
	// behind: readers and the collection count skip pending files.
	if err := errors.Join(writeErr, modeErr, closeErr); err != nil {
		_ = os.Remove(temp.Name())
		return err
	}
	if err := os.Rename(temp.Name(), target); err != nil {
		_ = os.Remove(temp.Name())
		return err
	}
	return nil
}

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
// kept once; it keeps at most maxCollected for one operation, counting those
// already on disk the first time it writes for the operation.
func writeTraffic(dir string, collected Collected, counts map[string]int) error {
	if strings.ContainsAny(collected.Op, `/\`) || collected.Op == "" || strings.HasPrefix(collected.Op, ".") {
		return errors.New("the operation identity is not a directory name")
	}
	directory := filepath.Join(dir, "traffic", collected.Op)
	count, known := counts[collected.Op]
	if !known {
		listed, err := os.ReadDir(directory)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		count = len(listed)
	}
	if count >= maxCollected {
		counts[collected.Op] = count
		return nil
	}
	name := collectedHash(collected) + ".json"
	if _, err := os.Lstat(filepath.Join(directory, name)); err == nil {
		counts[collected.Op] = count
		return nil
	}
	if err := publish(directory, name, EncodeCollected(collected)); err != nil {
		return err
	}
	counts[collected.Op] = count + 1
	return nil
}

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
	temp, err := os.CreateTemp(directory, ".pending-")
	if err != nil {
		return err
	}
	_, writeErr := temp.Write(data)
	modeErr := temp.Chmod(0o644)
	closeErr := temp.Close()
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

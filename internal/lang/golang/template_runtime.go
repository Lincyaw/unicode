package golang

import (
	ucbytes "bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	ucio "io"
	ucfs "io/fs"
	"os"
	ucfilepath "path/filepath"
	"reflect"
	"strings"
	ucsync "sync"
)

// State templates (docs/design.md §6.3, docs/language.md E25). The state a
// derived input builds in its case's directory is kept as a template under
// the key of the derivation and the model values it is computed from, and a
// later case with the same key gets a copy of the template and the derived
// value relocated to its own directory, instead of calling the derivation
// again. Templates live in go-derived/ of the content cache that UNICODE_CACHE
// names, which the parent of property processes makes for each run; without
// it there are none. A state that names its own directory in a file or a
// link cannot move, and is built afresh every time.

// ucTemplateRoot is the directory of this run's templates, or "".
func ucTemplateRoot() string {
	cache := os.Getenv("UNICODE_CACHE")
	if cache == "" || cache == "off" {
		return ""
	}
	return ucfilepath.Join(cache, "go-derived")
}

// ucTemplateExecutable is the SHA-256 of this test binary, which holds the
// derivation and every function it reaches.
var ucTemplateExecutable = ucsync.OnceValue(func() string {
	name, err := os.Executable()
	if err != nil {
		return ""
	}
	f, err := os.Open(name)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := ucio.Copy(h, f); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
})

// ucTemplateModel is the model type of the symbol name.
func ucTemplateModel(s *ucContract, name string) string {
	for _, symbol := range s.Symbols {
		if symbol.Name == name {
			return symbol.Type
		}
	}
	return ""
}

// ucTemplateKey is the key of the derived input name: the check, the
// symbol, this test binary, and the canonical values of the symbols args the
// derivation takes. It reports false when one of them has no canonical form.
func ucTemplateKey(s *ucContract, name string, args []string, values map[string]reflect.Value) (key string, ok bool) {
	exe := ucTemplateExecutable()
	if exe == "" {
		return "", false
	}
	defer func() {
		if recover() != nil {
			key, ok = "", false
		}
	}()
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "unicode-derived\n1\n%q\n%q\n%s\n", s.ID, name, exe)
	for _, arg := range args {
		wire := ucWireCanonical(ucFreezeValue(values[arg], ucTemplateModel(s, arg), s, map[uintptr]bool{}, 0))
		_, _ = fmt.Fprintf(h, "%q=%s\n", arg, ucWireText(wire))
	}
	return hex.EncodeToString(h.Sum(nil)), true
}

// ucTemplateManifest is what a template keeps besides the files: the
// directory the state was built in, and the derived value as a wire value.
type ucTemplateManifest struct {
	Root  string      `json:"root"`
	Value ucWireValue `json:"value"`
}

// ucTemplateUse copies the template of key into root and returns the derived
// value relocated there, or false when there is no template.
func ucTemplateUse(dir, key, root string, s *ucContract, name string, native reflect.Type) (reflect.Value, bool) {
	template := ucfilepath.Join(dir, key)
	data, err := os.ReadFile(ucfilepath.Join(template, "manifest.json"))
	if os.IsNotExist(err) {
		return reflect.Value{}, false
	}
	var manifest ucTemplateManifest
	if err == nil {
		err = json.Unmarshal(data, &manifest)
	}
	if err == nil {
		err = ucTemplateCopy(ucfilepath.Join(template, "state"), root)
	}
	if err != nil {
		ucFail("lang.golang.derivation-failed", "the state template "+template+" of the derived input "+name+" cannot be used: "+err.Error(), nil)
	}
	return ucThawValue(ucTemplateRelocate(manifest.Value, manifest.Root, root), native, ucTemplateModel(s, name), s, 0), true
}

// ucTemplateSave keeps the state built in root, and value, as the template
// of key, unless the state names root or value has no wire form. Two
// processes saving the same key keep the first.
func ucTemplateSave(dir, key, root string, s *ucContract, name string, value reflect.Value) {
	wire, ok := func() (w ucWireValue, ok bool) {
		defer func() {
			if recover() != nil {
				ok = false
			}
		}()
		return ucFreezeValue(value, ucTemplateModel(s, name), s, map[uintptr]bool{}, 0), true
	}()
	if !ok || !ucTemplateMovable(root) {
		return
	}
	// The value names its directory only at the start of texts, where
	// ucTemplateRelocate finds it.
	if text := ucWireText(ucTemplateRelocate(wire, root, "")); strings.Contains(text, root) {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	staging, err := os.MkdirTemp(dir, key+".staging-")
	if err != nil {
		return
	}
	defer func() { _ = os.RemoveAll(staging) }()
	manifest, err := json.Marshal(ucTemplateManifest{Root: root, Value: wire})
	if err != nil || ucTemplateCopy(root, ucfilepath.Join(staging, "state")) != nil || os.WriteFile(ucfilepath.Join(staging, "manifest.json"), manifest, 0o644) != nil {
		return
	}
	_ = os.Rename(staging, ucfilepath.Join(dir, key))
}

// ucTemplateMovable tells whether the state in root can move to another
// directory: it holds only directories, regular files and links, and none of
// them names root.
func ucTemplateMovable(root string) bool {
	names := []string{root}
	if real, err := ucfilepath.EvalSymlinks(root); err == nil && real != root {
		names = append(names, real)
	}
	mentions := func(data []byte) bool {
		for _, n := range names {
			if ucbytes.Contains(data, []byte(n)) {
				return true
			}
		}
		return false
	}
	err := ucfilepath.WalkDir(root, func(path string, d ucfs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		switch {
		case d.IsDir():
			return nil
		case d.Type()&ucfs.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil || mentions([]byte(target)) {
				return ucfs.ErrInvalid
			}
			return nil
		case d.Type().IsRegular():
			data, err := os.ReadFile(path)
			if err != nil || mentions(data) {
				return ucfs.ErrInvalid
			}
			return nil
		}
		return ucfs.ErrInvalid
	})
	return err == nil
}

// ucTemplateCopy copies the tree at from into to, which exists or is made,
// keeping permission bits and links. It copies each file with the file
// system's own copy, which clones it where the file system can.
func ucTemplateCopy(from, to string) error {
	return ucfilepath.WalkDir(from, func(path string, d ucfs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := ucfilepath.Rel(from, path)
		if err != nil {
			return err
		}
		target := ucfilepath.Join(to, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case d.IsDir():
			if err := os.MkdirAll(target, 0o700); err != nil {
				return err
			}
			return os.Chmod(target, info.Mode().Perm())
		case d.Type()&ucfs.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = in.Close() }()
		out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
		if err != nil {
			return err
		}
		// *os.File copies with copy_file_range on Linux, a clone on file
		// systems that share extents.
		if _, err := out.ReadFrom(in); err != nil {
			_ = out.Close()
			return err
		}
		return out.Close()
	})
}

// ucTemplateRelocate replaces the directory from at the start of every text
// of w with to.
func ucTemplateRelocate(w ucWireValue, from, to string) ucWireValue {
	if w.Text == from || strings.HasPrefix(w.Text, from+string(ucfilepath.Separator)) {
		w.Text = to + strings.TrimPrefix(w.Text, from)
	}
	if w.Items != nil {
		items := make([]ucWireValue, len(w.Items))
		for i, item := range w.Items {
			items[i] = ucTemplateRelocate(item, from, to)
		}
		w.Items = items
	}
	if w.Entries != nil {
		entries := make([]ucWireEntry, len(w.Entries))
		for i, e := range w.Entries {
			entries[i] = ucWireEntry{Key: ucTemplateRelocate(e.Key, from, to), Value: ucTemplateRelocate(e.Value, from, to)}
		}
		w.Entries = entries
	}
	return w
}

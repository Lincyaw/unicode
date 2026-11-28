package syntax

// Source is one source file: its slash-separated path relative to the
// repository root, the kind KindOf gives that path, and its bytes.
//
//uc:realizes syntax.Source
type Source struct {
	Path string
	Kind FileKind
	Text []byte
}

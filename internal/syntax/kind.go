package syntax

import (
	"path"
	"strings"
)

// FileKind decides which grammar of docs/language.md §3 a file is parsed with.
//
//uc:realizes syntax.FileKind
type FileKind string

const (
	FileKindSystem    FileKind = "system"
	FileKindComponent FileKind = "component"
	FileKindCorpus    FileKind = "corpus"
)

// NotADesignFile is the error of KindOf for a path that is neither a .uc file
// under design/ nor one under corpus/.
//
//uc:realizes syntax.NotADesignFile
type NotADesignFile struct{}

func (NotADesignFile) Error() string {
	return "the path is neither a .uc file under design/ nor one under corpus/"
}

const systemFilePath = "design/system.uc"

// KindOf tells the kind of a file from its slash-separated path relative to
// the repository root.
//
//uc:realizes syntax.kindOf
func KindOf(p string) (FileKind, error) {
	if p != path.Clean(p) || path.Ext(p) != ".uc" || path.Base(p) == ".uc" {
		return "", NotADesignFile{}
	}
	switch {
	case p == systemFilePath:
		return FileKindSystem, nil
	case strings.HasPrefix(p, "design/"):
		return FileKindComponent, nil
	case strings.HasPrefix(p, "corpus/"):
		return FileKindCorpus, nil
	}
	return "", NotADesignFile{}
}

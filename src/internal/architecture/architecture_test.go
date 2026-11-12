package architecture

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Keep package boundaries enforceable as new applications are added.
func TestDependencyBoundaries(t *testing.T) {
	root := ".."
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		parts := strings.Split(filepath.ToSlash(rel), "/")
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, spec := range file.Imports {
			imported, _ := strconv.Unquote(spec.Path.Value)
			dependency, internal := strings.CutPrefix(imported, "hachimodockplus/internal/")
			if !internal {
				continue
			}
			forbidden := false
			switch parts[0] {
			case "core":
				forbidden = true
			case "apps":
				forbidden = strings.HasPrefix(dependency, "ui/") || strings.HasPrefix(dependency, "bootstrap") ||
					(strings.HasPrefix(dependency, "apps/") && !strings.HasPrefix(dependency, "apps/"+parts[1]+"/") && dependency != "apps/"+parts[1])
			case "platform", "speech", "agent", "config":
				forbidden = strings.HasPrefix(dependency, "apps/") || strings.HasPrefix(dependency, "ui/") || strings.HasPrefix(dependency, "bootstrap")
			case "ui":
				forbidden = strings.HasPrefix(dependency, "bootstrap") ||
					(strings.HasPrefix(dependency, "apps/") && !strings.HasPrefix(filepath.Base(path), "page_"))
			}
			if forbidden {
				t.Errorf("%s imports forbidden dependency %s", rel, imported)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

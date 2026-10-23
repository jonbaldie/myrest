package verification

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"
)

// TaggedTest is one test function whose comments name a scenario id.
type TaggedTest struct {
	File string
	Func string
}

// ScenarioTags maps a scenario id to the tests that tag it.
type ScenarioTags map[string][]TaggedTest

var scenarioTagPattern = regexp.MustCompile(`\b[a-z]+-[0-9]{3}\b`)

// LoadScenarioTags reads every *_test.go file below roots in fsys. It returns
// each scenario id named in a comment on or inside a Test function.
func LoadScenarioTags(fsys fs.FS, roots ...string) (ScenarioTags, error) {
	tags := make(ScenarioTags)
	for _, root := range roots {
		err := fs.WalkDir(fsys, root, func(name string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || !strings.HasSuffix(name, "_test.go") {
				return nil
			}
			return addFileTags(fsys, name, tags)
		})
		if err != nil {
			return nil, fmt.Errorf("scenario tags: %w", err)
		}
	}
	return tags, nil
}

func addFileTags(fsys fs.FS, name string, tags ScenarioTags) error {
	src, err := fs.ReadFile(fsys, name)
	if err != nil {
		return err
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path.Base(name), src, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || !strings.HasPrefix(fn.Name.Name, "Test") {
			continue
		}
		test := TaggedTest{File: name, Func: fn.Name.Name}
		for _, id := range testTagIDs(file, fn) {
			tags[id] = append(tags[id], test)
		}
	}
	return nil
}

// testTagIDs returns the distinct ids in the doc comment and body comments of fn.
func testTagIDs(file *ast.File, fn *ast.FuncDecl) []string {
	var ids []string
	seen := make(map[string]struct{})
	for _, group := range file.Comments {
		if group != fn.Doc && (group.Pos() < fn.Pos() || group.End() > fn.End()) {
			continue
		}
		for _, id := range scenarioTagPattern.FindAllString(group.Text(), -1) {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	return ids
}

// CheckScenarioTags confirms every index id has a tagged test and every tag
// has an index row. The error names each offending id.
func CheckScenarioTags(index ScenarioIndex, tags ScenarioTags) error {
	var errs []error
	known := make(map[string]struct{}, len(index))
	for _, scenario := range index {
		known[scenario.ID] = struct{}{}
		if len(tags[scenario.ID]) == 0 {
			errs = append(errs, fmt.Errorf("%s: no tagged test", scenario.ID))
		}
	}
	ids := make([]string, 0, len(tags))
	for id := range tags {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if _, ok := known[id]; ok {
			continue
		}
		for _, test := range tags[id] {
			errs = append(errs, fmt.Errorf("%s: no index row (tagged in %s %s)", id, test.File, test.Func))
		}
	}
	return errors.Join(errs...)
}

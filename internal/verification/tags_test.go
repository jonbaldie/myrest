package verification_test

import (
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/jonbaldie/myrest/internal/verification"
)

func TestLoadScenarioTagsReadsTagAboveTestFunc(t *testing.T) {
	fsys := fstest.MapFS{
		"pkg/read_test.go": {Data: []byte(`package pkg

import "testing"

// read-001: a plain read returns every row.
func TestX(t *testing.T) {}
`)},
	}
	tags, err := verification.LoadScenarioTags(fsys, "pkg")
	if err != nil {
		t.Fatalf("LoadScenarioTags: %v", err)
	}
	want := verification.ScenarioTags{
		"read-001": {{File: "pkg/read_test.go", Func: "TestX"}},
	}
	if !reflect.DeepEqual(tags, want) {
		t.Fatalf("tags = %#v, want %#v", tags, want)
	}
}

func TestLoadScenarioTagsReturnsEveryTestThatTagsAnID(t *testing.T) {
	fsys := fstest.MapFS{
		"a/one_test.go": {Data: []byte(`package a

import "testing"

// read-001 and smoke-001: first proof.
func TestOne(t *testing.T) {}
`)},
		"b/nested/two_test.go": {Data: []byte(`package nested

import "testing"

func TestTwo(t *testing.T) {
	// read-001: second proof inside a subtest.
	// read-001: a second mention in the same test counts once.
	t.Run("x", func(t *testing.T) {})
}
`)},
	}
	tags, err := verification.LoadScenarioTags(fsys, "a", "b")
	if err != nil {
		t.Fatalf("LoadScenarioTags: %v", err)
	}
	want := verification.ScenarioTags{
		"read-001": {
			{File: "a/one_test.go", Func: "TestOne"},
			{File: "b/nested/two_test.go", Func: "TestTwo"},
		},
		"smoke-001": {{File: "a/one_test.go", Func: "TestOne"}},
	}
	if !reflect.DeepEqual(tags, want) {
		t.Fatalf("tags = %#v, want %#v", tags, want)
	}
}

func TestLoadScenarioTagsIgnoresNonTestCodeAndStrings(t *testing.T) {
	fsys := fstest.MapFS{
		"pkg/helper_test.go": {Data: []byte(`package pkg

import "testing"

// read-002: a helper is not a test.
func helper() {}

func TestY(t *testing.T) {
	_ = "alpha-175"
}

// read-004: a loose comment after a test does not belong to it.

type suite struct{}

// read-005: a method is not a top-level test.
func (suite) TestM(t *testing.T) {}
`)},
		"pkg/prod.go": {Data: []byte(`package pkg

// read-003: production code is not a test file.
func TestZ() {}
`)},
	}
	tags, err := verification.LoadScenarioTags(fsys, "pkg")
	if err != nil {
		t.Fatalf("LoadScenarioTags: %v", err)
	}
	if len(tags) != 0 {
		t.Fatalf("tags = %#v, want none", tags)
	}
}

func TestLoadScenarioTagsReportsUnparsableFile(t *testing.T) {
	fsys := fstest.MapFS{
		"pkg/bad_test.go": {Data: []byte("package pkg\nfunc {")},
	}
	_, err := verification.LoadScenarioTags(fsys, "pkg")
	if err == nil || !strings.Contains(err.Error(), "pkg/bad_test.go") {
		t.Fatalf("LoadScenarioTags() error = %v, want a parse error naming the file", err)
	}
}

func TestLoadScenarioTagsReportsMissingRoot(t *testing.T) {
	_, err := verification.LoadScenarioTags(fstest.MapFS{}, "missing")
	if err == nil {
		t.Fatal("LoadScenarioTags() = nil, want a missing root error")
	}
}

func TestCheckScenarioTagsPassesOnCompleteMatch(t *testing.T) {
	index := verification.ScenarioIndex{
		{ID: "read-001", Area: "read", Label: verification.FullMatch, Outcome: verification.Success},
	}
	tags := verification.ScenarioTags{
		"read-001": {{File: "pkg/read_test.go", Func: "TestX"}},
	}
	if err := verification.CheckScenarioTags(index, tags); err != nil {
		t.Fatalf("CheckScenarioTags: %v", err)
	}
}

func TestCheckScenarioTagsRejectsIndexIDWithNoTag(t *testing.T) {
	index := verification.ScenarioIndex{
		{ID: "read-001", Area: "read", Label: verification.FullMatch, Outcome: verification.Success},
		{ID: "read-002", Area: "read", Label: verification.FullMatch, Outcome: verification.Success},
		{ID: "read-003", Area: "read", Label: verification.FullMatch, Outcome: verification.Success},
	}
	tags := verification.ScenarioTags{
		"read-002": {{File: "pkg/read_test.go", Func: "TestX"}},
	}
	err := verification.CheckScenarioTags(index, tags)
	if err == nil {
		t.Fatal("CheckScenarioTags() = nil, want a missing tag error")
	}
	for _, id := range []string{"read-001", "read-003"} {
		if !strings.Contains(err.Error(), id+": no tagged test") {
			t.Fatalf("error = %q, want it to name %s", err, id)
		}
	}
	if strings.Contains(err.Error(), "read-002") {
		t.Fatalf("error = %q, must not name the tagged read-002", err)
	}
}

func TestCheckScenarioTagsRejectsTagWithNoIndexRow(t *testing.T) {
	index := verification.ScenarioIndex{
		{ID: "read-001", Area: "read", Label: verification.FullMatch, Outcome: verification.Success},
	}
	tags := verification.ScenarioTags{
		"read-001": {{File: "pkg/read_test.go", Func: "TestX"}},
		"read-099": {
			{File: "pkg/a_test.go", Func: "TestA"},
			{File: "pkg/b_test.go", Func: "TestB"},
		},
	}
	err := verification.CheckScenarioTags(index, tags)
	if err == nil {
		t.Fatal("CheckScenarioTags() = nil, want an orphan tag error")
	}
	for _, want := range []string{
		"read-099: no index row (tagged in pkg/a_test.go TestA)",
		"read-099: no index row (tagged in pkg/b_test.go TestB)",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want it to contain %q", err, want)
		}
	}
}

func TestCheckScenarioTagsNamesOrphanTagsInIDOrder(t *testing.T) {
	tags := verification.ScenarioTags{}
	ids := []string{"write-001", "auth-001", "read-001", "cfg-001", "embed-001"}
	for _, id := range ids {
		tags[id] = []verification.TaggedTest{{File: "pkg/x_test.go", Func: "TestX"}}
	}
	err := verification.CheckScenarioTags(nil, tags)
	if err == nil {
		t.Fatal("CheckScenarioTags() = nil, want orphan tag errors")
	}
	want := strings.Join([]string{
		"auth-001: no index row (tagged in pkg/x_test.go TestX)",
		"cfg-001: no index row (tagged in pkg/x_test.go TestX)",
		"embed-001: no index row (tagged in pkg/x_test.go TestX)",
		"read-001: no index row (tagged in pkg/x_test.go TestX)",
		"write-001: no index row (tagged in pkg/x_test.go TestX)",
	}, "\n")
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err, want)
	}
}

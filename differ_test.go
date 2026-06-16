/*
Copyright 2016 The gta AUTHORS. All rights reserved.

Use of this source code is governed by the Apache 2 license that can be found
in the LICENSE file.
*/
package gta

import (
	"bytes"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// check to make sure Git implements the Differ interface.
var _ Differ = &differ{}

func TestBaseFileReaderInterface(t *testing.T) {
	// NewGitDiffer returns a differ that satisfies BaseFileReader
	gitDiffer := NewGitDiffer()
	if _, ok := gitDiffer.(BaseFileReader); !ok {
		t.Error("git differ should implement BaseFileReader")
	}

	// NewFileDiffer returns a differ that satisfies BaseFileReader structurally
	// but its ReadBaseFile returns an error since there's no git backing.
	fileDiffer := NewFileDiffer(nil)
	if reader, ok := fileDiffer.(BaseFileReader); ok {
		_, err := reader.ReadBaseFile("/some/abs/path/go.mod")
		if err == nil {
			t.Error("file differ's ReadBaseFile should return an error")
		}
	}
}

// TestExecWithStderr_containsCommandAndStderr verifies that when a git command
// fails, the returned error contains both the full command line and git's stderr.
func TestExecWithStderr_containsCommandAndStderr(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found on PATH")
	}

	cmd := exec.Command("git", "definitely-not-a-real-subcommand")
	_, err := execWithStderr(cmd)

	if err == nil {
		t.Fatal("expected error from invalid git subcommand, got nil")
	}

	errStr := err.Error()

	// The error must contain the full command line.
	if !strings.Contains(errStr, "definitely-not-a-real-subcommand") {
		t.Errorf("error does not contain the subcommand; got: %q", errStr)
	}

	// The error must contain a stable substring from git's stderr output.
	// Confirmed empirically: git prints "git: 'definitely-not-a-real-subcommand' is not a git command."
	if !strings.Contains(errStr, "is not a git command") {
		t.Errorf("error does not contain git's stderr ('is not a git command'); got: %q", errStr)
	}

	// The underlying *exec.ExitError must be reachable via the error chain (%w).
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		t.Errorf("expected errors.As to find *exec.ExitError in chain, but it did not; err: %v", err)
	}
}

// TestGitCmdError_directUnit verifies gitCmdError output and %w chain without
// invoking git, so it is always fast and hermetic.
func TestGitCmdError_directUnit(t *testing.T) {
	sentinel := errors.New("sentinel error")
	cmd := exec.Command("git", "fake-sub", "--flag")

	// With a non-empty stderr.
	got := gitCmdError(cmd, "fatal: not a git repo\n", sentinel)
	gotStr := got.Error()

	if !strings.Contains(gotStr, "fake-sub") {
		t.Errorf("error does not contain command arg; got: %q", gotStr)
	}
	if !strings.Contains(gotStr, "fatal: not a git repo") {
		t.Errorf("error does not contain stderr; got: %q", gotStr)
	}
	if !errors.Is(got, sentinel) {
		t.Errorf("errors.Is(got, sentinel) = false; sentinel not in chain")
	}

	// With an empty stderr.
	gotEmpty := gitCmdError(cmd, "", sentinel)
	gotEmptyStr := gotEmpty.Error()
	if !strings.Contains(gotEmptyStr, "fake-sub") {
		t.Errorf("empty-stderr error does not contain command arg; got: %q", gotEmptyStr)
	}
	if !errors.Is(gotEmpty, sentinel) {
		t.Errorf("errors.Is(gotEmpty, sentinel) = false; sentinel not in chain")
	}
}

func Test_diffFileDirectories(t *testing.T) {
	var tests = []struct {
		desc string
		root string
		buf  []byte
		want map[string]struct{}
	}{
		{
			desc: "single changed file",
			root: "/",
			buf:  []byte("foo/bar.go\n"),
			want: map[string]struct{}{
				"/foo/bar.go": struct{}{},
			},
		},
		{
			desc: "multiple changed files in same directory (duplicate)",
			root: "/foo",
			buf: []byte(`bar/bar.go
bar/baz.go`),
			want: map[string]struct{}{
				"/foo/bar/bar.go": struct{}{},
				"/foo/bar/baz.go": struct{}{},
			},
		},
		{
			desc: "multiple changed files in different directories",
			root: "/foo/bar",
			buf: []byte(`baz/bar.go
baz/qux/baz.go`),
			want: map[string]struct{}{
				"/foo/bar/baz/bar.go":     struct{}{},
				"/foo/bar/baz/qux/baz.go": struct{}{},
			},
		},
		{
			desc: "multiple changed files in different directories, with duplicate directories",
			root: "/",
			buf: []byte(`foo/bar.go
foo/baz.go
bar/foo.go
bar/baz/qux.go
bar/baz/corge.go
bar/baz/qux/corge.go
`),
			want: map[string]struct{}{
				"/foo/bar.go":           struct{}{},
				"/foo/baz.go":           struct{}{},
				"/bar/foo.go":           struct{}{},
				"/bar/baz/qux.go":       struct{}{},
				"/bar/baz/corge.go":     struct{}{},
				"/bar/baz/qux/corge.go": struct{}{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {

			got, err := diffPaths(tt.root, bytes.NewReader(tt.buf))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("(-want, +got)\n%s", diff)
			}
		})
	}
}

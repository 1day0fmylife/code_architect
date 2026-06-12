package codeengine

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseGitStatusFiles(t *testing.T) {
	status := strings.Join([]string{
		" M README.md",
		"?? new file.txt",
		"R  old.go -> internal/new.go",
		" M README.md",
		"",
	}, "\n")

	got := parseGitStatusFiles(status)
	want := []string{"README.md", "new file.txt", "internal/new.go"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %#v, got %#v", want, got)
	}
}

func TestRedactMasksSecrets(t *testing.T) {
	got := redact("token=abc password=secret Authorization: Bearer value")
	for _, leaked := range []string{"abc", "secret", "Bearer value"} {
		if strings.Contains(got, leaked) {
			t.Fatalf("expected %q to be redacted in %q", leaked, got)
		}
	}
	if strings.Count(got, "[REDACTED]") != 3 {
		t.Fatalf("expected three redactions, got %q", got)
	}
}

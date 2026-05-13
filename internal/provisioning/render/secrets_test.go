package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadUserPassFilePreservesPasswordWhitespace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials")
	if err := os.WriteFile(path, []byte("user: pass \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := readUserPassFile(path, "test credentials")
	if err != nil {
		t.Fatalf("readUserPassFile returned error: %v", err)
	}
	if got.Username != "user" {
		t.Fatalf("username got %q, want user", got.Username)
	}
	if got.Password != " pass " {
		t.Fatalf("password got %q, want %q", got.Password, " pass ")
	}
}

func TestReadUserPassFileRejectsMultiline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials")
	if err := os.WriteFile(path, []byte("user:pass\nnext:line\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := readUserPassFile(path, "test credentials")
	if err == nil {
		t.Fatal("expected multiline credentials to error")
	}
	if !strings.Contains(err.Error(), "single username:password line") {
		t.Fatalf("unexpected error: %v", err)
	}
}

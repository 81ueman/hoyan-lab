package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLabsHelpListsCheck(t *testing.T) {
	var out bytes.Buffer
	cmd := NewLabsCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(out.String(), "check") {
		t.Fatalf("help output missing check:\n%s", out.String())
	}
}

func TestSelectedLabDescriptorsDefaultsToAllLabsSorted(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"z-lab", "a-lab"} {
		labDir := filepath.Join(dir, "labs", name)
		if err := os.MkdirAll(labDir, 0o755); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}
		if err := os.WriteFile(filepath.Join(labDir, "lab.yml"), []byte("name: "+name+"\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(lab.yml) error = %v", err)
		}
	}
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldwd); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})

	labs, err := selectedLabDescriptors(nil)
	if err != nil {
		t.Fatalf("selectedLabDescriptors() error = %v", err)
	}
	got := []string{labs[0].Name, labs[1].Name}
	want := []string{"a-lab", "z-lab"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("labs = %v, want %v", got, want)
	}
}

func TestSelectedLabDescriptorsIncludesDirsWithoutLabYAML(t *testing.T) {
	dir := t.TempDir()
	labDir := filepath.Join(dir, "labs", "listed")
	if err := os.MkdirAll(labDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(listed) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(labDir, "lab.yml"), []byte("name: listed\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(lab.yml) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "labs", "missing-metadata"), 0o755); err != nil {
		t.Fatalf("MkdirAll(missing-metadata) error = %v", err)
	}
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldwd); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})

	labs, err := selectedLabDescriptors(nil)
	if err != nil {
		t.Fatalf("selectedLabDescriptors() error = %v", err)
	}
	got := []string{labs[0].Name, labs[1].Name}
	want := []string{"listed", "missing-metadata"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("labs = %v, want %v", got, want)
	}
}

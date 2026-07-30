package vault

import (
	"os"
	"strings"
	"testing"
)

func TestCheckIntegrityFindsBrokenLinksAndDuplicateContent(t *testing.T) {
	v := New(t.TempDir())
	if _, err := v.CreateNote("a", "same body\n\n[[missing-note]]", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := v.CreateNote("b", "same body\n\n[[missing-note]]", nil); err != nil {
		t.Fatal(err)
	}
	report, err := v.CheckIntegrity()
	if err != nil {
		t.Fatal(err)
	}
	if len(report.BrokenLinks) != 2 {
		t.Fatalf("broken=%#v", report.BrokenLinks)
	}
	if len(report.Duplicates) != 1 {
		t.Fatalf("duplicates=%#v", report.Duplicates)
	}
	if report.OK {
		t.Fatal("report should not be OK")
	}
}

func TestUpdateNoteUsesAtomicReplacementWithoutTempFiles(t *testing.T) {
	root := t.TempDir()
	v := New(root)
	if _, err := v.CreateNote("note", "before", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := v.UpdateNote("note", "after", nil); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(root + string(os.PathSeparator) + "note.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "after") {
		t.Fatalf("content=%s", data)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("temporary files left behind: %#v", entries)
	}
}

func TestCheckIntegrityAcceptsAliasesAndExistingLinks(t *testing.T) {
	v := New(t.TempDir())
	if _, err := v.CreateNote("target", "target", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := v.CreateNote("source", "[[target|Target note]]", nil); err != nil {
		t.Fatal(err)
	}
	report, err := v.CheckIntegrity()
	if err != nil {
		t.Fatal(err)
	}
	if !report.OK {
		t.Fatalf("report=%#v", report)
	}
}

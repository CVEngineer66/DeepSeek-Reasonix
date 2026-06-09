package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fmt"
)

// ── Activation mode round-trip ──────────────────────────────────────────

func TestSaveLoadActivationModeRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: filepath.Join(dir, "memory")}

	modes := []ActivationMode{
		ActivationAlwaysOn,
		ActivationModelDecision,
	}
	for _, mode := range modes {
		name := "test-" + string(mode)
		if _, err := s.Save(Memory{
			Name:        name,
			Title:       "Test " + string(mode),
			Description: "testing " + string(mode),
			Type:        TypeProject,
			Activation:  mode,
			Body:        "body " + string(mode),
		}); err != nil {
			t.Fatalf("Save %s: %v", mode, err)
		}
	}

	list := s.List()
	if len(list) != 2 {
		t.Fatalf("want 2 facts (always_on + model_decision), got %d", len(list))
	}
	for _, m := range list {
		if m.Activation == "" {
			t.Errorf("fact %q has empty activation after round-trip", m.Name)
		}
	}
}

func TestSaveLoadDefaultActivation(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: filepath.Join(dir, "memory")}

	if _, err := s.Save(Memory{
		Name:        "default-activation",
		Description: "no activation set",
		Type:        TypeProject,
		Body:        "body",
	}); err != nil {
		t.Fatal(err)
	}

	list := s.List()
	if len(list) != 1 {
		t.Fatal("expected 1 fact")
	}
	if list[0].Activation != ActivationModelDecision {
		t.Fatalf("default activation should be model_decision, got %q", list[0].Activation)
	}
}

func TestNormalizeActivation(t *testing.T) {
	if got := NormalizeActivation("always_on"); got != ActivationAlwaysOn {
		t.Errorf("always_on: got %q", got)
	}
	if got := NormalizeActivation("ALWAYS_ON"); got != ActivationAlwaysOn {
		t.Errorf("ALWAYS_ON: got %q", got)
	}
	if got := NormalizeActivation("model_decision"); got != ActivationModelDecision {
		t.Errorf("model_decision: got %q", got)
	}
	if got := NormalizeActivation(""); got != ActivationModelDecision {
		t.Errorf("empty defaults to model_decision, got %q", got)
	}
	if got := NormalizeActivation("invalid"); got != ActivationModelDecision {
		t.Errorf("invalid defaults to model_decision, got %q", got)
	}
}

// ── Refs persistence (YAML list → single-line) ──────────────────────────

func TestSaveLoadRefsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: filepath.Join(dir, "memory")}

	m := Memory{
		Name:        "with-refs",
		Description: "has file refs",
		Type:        TypeProject,
		Body:        "See `src/main.go` and `config.yaml` for details.",
		Refs:        []string{"src/main.go", "config.yaml", "README.md"},
	}
	if _, err := s.Save(m); err != nil {
		t.Fatal(err)
	}

	list := s.List()
	if len(list) != 1 {
		t.Fatal("expected 1 fact")
	}
	got := list[0]
	if len(got.Refs) != 3 {
		t.Fatalf("expected 3 refs, got %d: %v", len(got.Refs), got.Refs)
	}
	// Check each ref round-trips.
	expected := map[string]bool{"src/main.go": true, "config.yaml": true, "README.md": true}
	for _, r := range got.Refs {
		if !expected[r] {
			t.Errorf("unexpected ref %q", r)
		}
	}
}

func TestSaveLoadEmptyRefs(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: filepath.Join(dir, "memory")}

	if _, err := s.Save(Memory{
		Name:        "no-refs",
		Description: "no file refs",
		Type:        TypeProject,
		Body:        "just a note",
	}); err != nil {
		t.Fatal(err)
	}
	list := s.List()
	if len(list) != 1 {
		t.Fatal("expected 1 fact")
	}
	if len(list[0].Refs) != 0 {
		t.Fatalf("expected 0 refs, got %d", len(list[0].Refs))
	}
}

// ── Verification metadata round-trip ────────────────────────────────────

func TestSaveLoadVerificationMetadataRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: filepath.Join(dir, "memory")}

	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	m := Memory{
		Name:        "verified-fact",
		Description: "a verified fact",
		Type:        TypeProject,
		Body:        "content",
		CreatedAt:   now.Add(-24 * time.Hour),
		UpdatedAt:   now.Add(-12 * time.Hour),
		VerifiedAt:  now,
		VerifyCount: 3,
	}
	if _, err := s.Save(m); err != nil {
		t.Fatal(err)
	}

	list := s.List()
	if len(list) != 1 {
		t.Fatal("expected 1 fact")
	}
	got := list[0]
	if got.CreatedAt.IsZero() {
		t.Error("created_at not persisted")
	}
	if got.UpdatedAt.IsZero() {
		t.Error("updated_at not persisted")
	}
	if got.VerifiedAt.IsZero() {
		t.Error("verified_at not persisted")
	}
	if got.VerifyCount != 3 {
		t.Errorf("verify_count = %d, want 3", got.VerifyCount)
	}
}

func TestSaveAutoPopulatesTimestamps(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: filepath.Join(dir, "memory")}

	if _, err := s.Save(Memory{
		Name:        "auto-ts",
		Description: "auto timestamps",
		Type:        TypeProject,
		Body:        "content",
	}); err != nil {
		t.Fatal(err)
	}

	m := s.List()[0]
	if m.CreatedAt.IsZero() {
		t.Error("created_at should be auto-populated")
	}
	if m.UpdatedAt.IsZero() {
		t.Error("updated_at should be auto-populated")
	}
	if !m.UpdatedAt.Equal(m.CreatedAt) {
		t.Error("updated_at should equal created_at on first save")
	}
}

// ── Topic directory support ─────────────────────────────────────────────

func TestSaveInTopicDirectory(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: filepath.Join(dir, "memory")}

	if _, err := s.Save(Memory{
		Name:        "naming-convention",
		Description: "naming rules",
		Type:        TypeProject,
		Topic:       "frontend-style",
		Body:        "Use camelCase.",
	}); err != nil {
		t.Fatal(err)
	}

	// File should be in topic subdirectory.
	expectedPath := filepath.Join(dir, "memory", "frontend-style", "naming-convention.md")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Fatalf("file not created at topic path: %s", expectedPath)
	}

	// List should return the fact.
	list := s.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 fact, got %d", len(list))
	}
	if list[0].Topic != "frontend-style" {
		t.Errorf("topic not round-tripped: %q", list[0].Topic)
	}
	if list[0].Name != "naming-convention" {
		t.Errorf("name not round-tripped: %q", list[0].Name)
	}
}

func TestTopicDirNormalization(t *testing.T) {
	if got := topicDir(""); got != "" {
		t.Errorf("empty topic: got %q", got)
	}
	if got := topicDir("Frontend Style"); got != "frontend-style" {
		t.Errorf("slugified topic: got %q", got)
	}
	if got := topicDir("frontend-style"); got != "frontend-style" {
		t.Errorf("already slug: got %q", got)
	}
}

func TestListRecursesIntoTopicDirs(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: filepath.Join(dir, "memory")}

	// Root fact.
	if _, err := s.Save(Memory{Name: "root-fact", Description: "root", Type: TypeProject, Body: "root"}); err != nil {
		t.Fatal(err)
	}
	// Topic fact.
	if _, err := s.Save(Memory{Name: "topic-fact", Description: "topic", Type: TypeProject, Topic: "mytopic", Body: "topic"}); err != nil {
		t.Fatal(err)
	}
	// Nested topic fact.
	if _, err := s.Save(Memory{Name: "nested-fact", Description: "nested", Type: TypeProject, Topic: "mytopic/subtopic", Body: "nested"}); err != nil {
		t.Fatal(err)
	}

	list := s.List()
	if len(list) != 3 {
		t.Fatalf("expected 3 facts, got %d", len(list))
	}
}

func TestDeleteSearchesTopicDirs(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: filepath.Join(dir, "memory")}

	if _, err := s.Save(Memory{Name: "topic-fact", Description: "d", Type: TypeProject, Topic: "mytopic", Body: "b"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("topic-fact"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(s.List()) != 0 {
		t.Fatal("fact should be deleted from topic dir")
	}
}

// ── AlwaysOnFacts and body cap ──────────────────────────────────────────

func TestAlwaysOnFactsReturnsOnlyAlwaysOn(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: filepath.Join(dir, "memory")}

	facts := []Memory{
		{Name: "always", Description: "always", Type: TypeProject, Activation: ActivationAlwaysOn, Body: "always body"},
		{Name: "model", Description: "model", Type: TypeProject, Activation: ActivationModelDecision, Body: "model body"},
		{Name: "model2", Description: "model2", Type: TypeProject, Activation: ActivationModelDecision, Body: "model2 body"},
	}
	for _, f := range facts {
		if _, err := s.Save(f); err != nil {
			t.Fatal(err)
		}
	}

	ao := s.AlwaysOnFacts()
	if len(ao) != 1 {
		t.Fatalf("expected 1 always_on fact, got %d", len(ao))
	}
	if ao[0].Name != "always" {
		t.Errorf("expected 'always', got %q", ao[0].Name)
	}
}

func TestAlwaysOnBodyCapEnforced(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: filepath.Join(dir, "memory")}

	bigBody := strings.Repeat("x", alwaysOnMaxBody+100)
	if _, err := s.Save(Memory{
		Name:        "big-body",
		Description: "big",
		Type:        TypeProject,
		Activation:  ActivationAlwaysOn,
		Body:        bigBody,
	}); err != nil {
		t.Fatal(err)
	}

	ao := s.AlwaysOnFacts()
	if len(ao) != 1 {
		t.Fatal("expected 1 always_on fact")
	}
	// Body should be capped but still contain the truncation message.
	// Allow some slop for the truncation notice appended after the cap.
	maxExpected := alwaysOnMaxBody + len("[body truncated by always_on size limit]") + 4
	if len(ao[0].Body) > maxExpected {
		t.Errorf("body length %d exceeds cap %d", len(ao[0].Body), alwaysOnMaxBody)
	}
	if !strings.Contains(ao[0].Body, "truncated") {
		t.Error("truncated body should contain truncation notice")
	}
}

// ── Refs auto-extraction from body ──────────────────────────────────────

func TestRefsFromBodyExtractsPaths(t *testing.T) {
	body := "See `src/main.go` for details.\nUpdate `config/production.yaml` in the config dir."
	refs := refsFromBody(body)
	if len(refs) == 0 {
		t.Fatal("expected at least one ref")
	}
}

func TestRefsFromBodyIgnoresPlainText(t *testing.T) {
	body := "This is just a note with no file paths whatsoever."
	refs := refsFromBody(body)
	if len(refs) != 0 {
		t.Fatalf("expected no refs, got %v", refs)
	}
}

// ── Verification ────────────────────────────────────────────────────────

func TestVerifyMemoryMissingRefs(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: filepath.Join(dir, "memory")}

	if _, err := s.Save(Memory{
		Name:        "test",
		Description: "test fact",
		Type:        TypeProject,
		Body:        "See `nonexistent.go`",
		Refs:        []string{"nonexistent.go", "also-missing.yaml"},
	}); err != nil {
		t.Fatal(err)
	}

	missing := s.VerifyMemory(s.List()[0], dir)
	if len(missing) == 0 {
		t.Fatal("expected missing refs for nonexistent files")
	}
}

func TestVerifyMemoryExistingRefs(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: filepath.Join(dir, "memory")}

	// Create the referenced file.
	os.MkdirAll(filepath.Join(dir, "subdir"), 0o755)
	os.WriteFile(filepath.Join(dir, "subdir", "exists.go"), []byte("package main"), 0o644)

	if _, err := s.Save(Memory{
		Name:        "test",
		Description: "test fact",
		Type:        TypeProject,
		Body:        "See `subdir/exists.go`",
		Refs:        []string{"subdir/exists.go"},
	}); err != nil {
		t.Fatal(err)
	}

	missing := s.VerifyMemory(s.List()[0], dir)
	if len(missing) > 0 {
		t.Fatalf("expected no missing refs, got %v", missing)
	}
}

func TestVerifyMemoryNoRefsNoError(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: filepath.Join(dir, "memory")}

	if _, err := s.Save(Memory{
		Name:        "no-refs",
		Description: "no refs",
		Type:        TypeProject,
		Body:        "just text",
	}); err != nil {
		t.Fatal(err)
	}

	missing := s.VerifyMemory(s.List()[0], dir)
	if len(missing) > 0 {
		t.Fatalf("expected no missing refs, got %v", missing)
	}
}

// ── Index line formatting ───────────────────────────────────────────────

func TestIndexLineFormat(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: filepath.Join(dir, "memory")}

	if _, err := s.Save(Memory{
		Name:        "test-fact",
		Title:       "Test Fact",
		Description: "A test fact for verification",
		Type:        TypeProject,
		Body:        "body",
	}); err != nil {
		t.Fatal(err)
	}

	idx := s.Index()
	if !strings.Contains(idx, "[Test Fact](test-fact.md)") {
		t.Errorf("index missing title link:\n%s", idx)
	}
	if !strings.Contains(idx, "A test fact for verification") {
		t.Errorf("index missing description:\n%s", idx)
	}
}

func TestIndex200LineCap(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: filepath.Join(dir, "memory")}

	// Save 210 facts (50 more than the cap).
	for i := 0; i < maxIndexLines+10; i++ {
		name := fmt.Sprintf("fact-%04d", i)
		if _, err := s.Save(Memory{
			Name:        name,
			Description: "fact " + name,
			Type:        TypeProject,
			Body:        "body",
		}); err != nil {
			t.Fatal(err)
		}
	}

	// Index should be capped at maxIndexLines.
	lines := strings.Split(strings.TrimSpace(s.Index()), "\n")
	// First line is "# Memory", so content lines should be <= maxIndexLines.
	contentLines := 0
	for _, l := range lines {
		if strings.HasPrefix(l, "- [") {
			contentLines++
		}
	}
	if contentLines > maxIndexLines {
		t.Fatalf("index has %d content lines, cap is %d", contentLines, maxIndexLines)
	}
	if contentLines == 0 {
		t.Fatal("index should contain at least some lines")
	}
}

func TestIndexRecencySorted(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: filepath.Join(dir, "memory")}

	// Save facts with explicit timestamps.
	old := time.Now().Add(-48 * time.Hour)
	recent := time.Now().Add(-1 * time.Hour)

	// Save old fact first, then recent fact.
	if _, err := s.Save(Memory{
		Name:        "old-fact",
		Description: "old fact",
		Type:        TypeProject,
		Body:        "old",
		CreatedAt:   old,
		UpdatedAt:   old,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save(Memory{
		Name:        "recent-fact",
		Description: "recent fact",
		Type:        TypeProject,
		Body:        "recent",
		CreatedAt:   recent,
		UpdatedAt:   recent,
	}); err != nil {
		t.Fatal(err)
	}

	// Re-read to verify recency sort.
	idx := s.Index()
	lines := strings.Split(idx, "\n")
	var contentLines []string
	for _, l := range lines {
		if strings.HasPrefix(l, "- [") {
			contentLines = append(contentLines, l)
		}
	}
	if len(contentLines) != 2 {
		t.Fatalf("expected 2 index lines, got %d", len(contentLines))
	}
	// After recency sort, recent-fact should come first.
	if !strings.Contains(contentLines[0], "recent-fact") {
		t.Errorf("expected recent-fact first (recency sort), got:\n%s\n%s", contentLines[0], contentLines[1])
	}
}

// ── Backward compatibility with old-format facts ────────────────────────

func TestLoadOldFormatFact(t *testing.T) {
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "memory")
	os.MkdirAll(storeDir, 0o755)

	// Write a fact in the OLD format (no activation, no timestamps).
	oldContent := `---
name: old-fact
title: Old Fact
description: An old fact without activation
metadata:
  type: user
---

This fact was saved before the memory system upgrade.
`
	os.WriteFile(filepath.Join(storeDir, "old-fact.md"), []byte(oldContent), 0o644)

	s := Store{Dir: storeDir}
	list := s.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 fact, got %d", len(list))
	}
	m := list[0]
	if m.Name != "old-fact" {
		t.Errorf("name = %q", m.Name)
	}
	if m.Activation != ActivationModelDecision {
		t.Errorf("old-format fact should default to model_decision, got %q", m.Activation)
	}
	if m.Type != TypeUser {
		t.Errorf("type should be user, got %q", m.Type)
	}
	if !strings.Contains(m.Body, "before the memory system upgrade") {
		t.Errorf("body not preserved:\n%s", m.Body)
	}
	if m.CreatedAt.IsZero() {
		t.Error("created_at should default to non-zero for old facts")
	}
}

// ── Always-on fact inclusion in new-index line format ───────────────────

func TestAlwaysOnFactsAfterReindex(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: filepath.Join(dir, "memory")}

	if _, err := s.Save(Memory{
		Name:        "always-rule",
		Description: "critical rule",
		Type:        TypeProject,
		Activation:  ActivationAlwaysOn,
		Body:        "Never delete this file.",
	}); err != nil {
		t.Fatal(err)
	}

	// AlwaysOnFacts should include the fact.
	ao := s.AlwaysOnFacts()
	if len(ao) != 1 {
		t.Fatalf("expected 1 always_on fact, got %d", len(ao))
	}
	if !strings.Contains(ao[0].Body, "Never delete") {
		t.Errorf("body not preserved in AlwaysOnFacts")
	}
}

// ── Store.List edge cases ───────────────────────────────────────────────

func TestListSkipsMEMORYMd(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: dir}

	// MEMORY.md should be skipped by List.
	os.WriteFile(filepath.Join(dir, indexFile), []byte("# Memory\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "real-fact.md"), []byte("---\nname: real-fact\n---\n\nbody"), 0o644)

	list := s.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 fact, got %d", len(list))
	}
	if list[0].Name != "real-fact" {
		t.Errorf("wrong fact returned: %q", list[0].Name)
	}
}

func TestListSkipsNonMdFilesExtended(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: dir}

	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("not a fact"), 0o644)
	os.WriteFile(filepath.Join(dir, "data.json"), []byte("{}"), 0o644)

	list := s.List()
	if len(list) != 0 {
		t.Fatalf("expected 0 facts from non-md files, got %d", len(list))
	}
}

func TestListEmptyDirExtended(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: dir}

	list := s.List()
	if list != nil {
		t.Fatal("List of empty dir should return nil")
	}
}

func TestListDisabledStore(t *testing.T) {
	var s Store
	list := s.List()
	if list != nil {
		t.Fatal("disabled store List should return nil")
	}
}

// ── slug edge cases ─────────────────────────────────────────────────────

func TestSlugEdgeCases(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"Prefers Tabs", "prefers-tabs"},
		{"My Great Fact!", "my-great-fact"},
		{"  leading/trailing  ", "leading-trailing"},
		{"UPPERCASE", "uppercase"},
		{"already-kebab", "already-kebab"},
		{"special@#$%chars", "special-chars"},
		{"", ""},
		{"---", ""},
	}
	for _, c := range cases {
		got := slug(c.input)
		if got != c.want {
			t.Errorf("slug(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

// ── refsFromBody edge cases ─────────────────────────────────────────────

func TestRefsFromBodyEmpty(t *testing.T) {
	if refs := refsFromBody(""); len(refs) != 0 {
		t.Errorf("expected empty refs, got %v", refs)
	}
}

func TestRefsFromBodyCodeBlock(t *testing.T) {
	body := "```\nconst x = 1\n```\nSee `internal/memory/store.go` for implementation."
	refs := refsFromBody(body)
	// Should find the reference in the backtick.
	found := false
	for _, r := range refs {
		if strings.Contains(r, "store.go") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected to find store.go reference, got %v", refs)
	}
}

// ── slugify edge cases ──────────────────────────────────────────────────

func TestSlugify(t *testing.T) {
	if got := slugify(""); got != "" {
		t.Errorf("empty path: got %q", got)
	}
	if got := slugify("/"); got != "-" {
		t.Errorf("root: got %q", got)
	}
	if got := slugify("/Users/me/My Project"); !strings.Contains(got, "-Users-me-My-Project") {
		t.Errorf("path with spaces: got %q", got)
	}
}

// ── Backward compatibility: fact with no frontmatter at all ─────────────

func TestLoadFactWithNoFrontmatter(t *testing.T) {
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "memory")
	os.MkdirAll(storeDir, 0o755)

	os.WriteFile(filepath.Join(storeDir, "bare-fact.md"), []byte("Just a bare markdown note."), 0o644)

	s := Store{Dir: storeDir}
	list := s.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 fact, got %d", len(list))
	}
	m := list[0]
	if m.Body != "Just a bare markdown note." {
		t.Errorf("body not preserved: %q", m.Body)
	}
	if m.Name != "bare-fact" {
		t.Errorf("name should derive from filename: %q", m.Name)
	}
}

func TestLoadFactWithNoFrontmatterDefaultActivation(t *testing.T) {
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "memory")
	os.MkdirAll(storeDir, 0o755)

	os.WriteFile(filepath.Join(storeDir, "bare-fact.md"), []byte("Just a bare note."), 0o644)

	s := Store{Dir: storeDir}
	m := s.List()[0]
	if m.Activation != ActivationModelDecision {
		t.Errorf("bare fact should default to model_decision, got %q", m.Activation)
	}
}

// ── Index after overwrite with more fields ──────────────────────────────

func TestOverwriteUpdatesRefsAndActivation(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: filepath.Join(dir, "memory")}

	// Save initial version.
	if _, err := s.Save(Memory{
		Name:        "config",
		Description: "initial config",
		Type:        TypeProject,
		Body:        "old body",
	}); err != nil {
		t.Fatal(err)
	}

	// Overwrite with more fields.
	if _, err := s.Save(Memory{
		Name:        "config",
		Description: "updated config",
		Type:        TypeProject,
		Activation:  ActivationAlwaysOn,
		Body:        "new body with file `src/config.go`",
	}); err != nil {
		t.Fatal(err)
	}

	list := s.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 fact, got %d", len(list))
	}
	m := list[0]
	if m.Description != "updated config" {
		t.Errorf("description = %q", m.Description)
	}
	if m.Activation != ActivationAlwaysOn {
		t.Errorf("activation should be always_on, got %q", m.Activation)
	}
	if !strings.Contains(m.Body, "new body") {
		t.Errorf("body not updated: %q", m.Body)
	}
}

// ── render and loadMemory edge cases ────────────────────────────────────

func TestRenderRefsMultiple(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: filepath.Join(dir, "memory")}

	m := Memory{
		Name:        "with-refs",
		Description: "test with refs",
		Type:        TypeProject,
		Body:        "body",
		Refs:        []string{"a.go", "b.go", "c/d.go"},
	}
	if _, err := s.Save(m); err != nil {
		t.Fatal(err)
	}

	got := s.List()[0]
	if len(got.Refs) != 3 {
		t.Fatalf("expected 3 refs, got %d: %v", len(got.Refs), got.Refs)
	}
}

func TestRenderTopic(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: filepath.Join(dir, "memory")}

	m := Memory{
		Name:        "nested-topic",
		Description: "deeply nested",
		Type:        TypeProject,
		Topic:       "parent/sub",
		Body:        "content",
	}
	if _, err := s.Save(m); err != nil {
		t.Fatal(err)
	}

	got := s.List()[0]
	if got.Topic != "parent/sub" {
		t.Errorf("topic = %q, want parent/sub", got.Topic)
	}
}

// ── VerifyAllFacts ──────────────────────────────────────────────────────

func TestVerifyAllFactsWithMixedRefs(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: filepath.Join(dir, "memory")}

	// Create file that will exist.
	os.MkdirAll(filepath.Join(dir, "src"), 0o755)
	os.WriteFile(filepath.Join(dir, "src", "exists.go"), []byte("package main"), 0o644)

	// Fact with valid refs.
	if _, err := s.Save(Memory{
		Name:        "valid",
		Description: "valid refs",
		Type:        TypeProject,
		Body:        "See `src/exists.go`",
		Refs:        []string{"src/exists.go"},
	}); err != nil {
		t.Fatal(err)
	}
	// Fact with missing refs.
	if _, err := s.Save(Memory{
		Name:        "invalid",
		Description: "invalid refs",
		Type:        TypeProject,
		Body:        "See `src/missing.go`",
		Refs:        []string{"src/missing.go"},
	}); err != nil {
		t.Fatal(err)
	}
	// Fact with no refs.
	if _, err := s.Save(Memory{
		Name:        "norefs",
		Description: "no refs at all",
		Type:        TypeProject,
		Body:        "just text",
	}); err != nil {
		t.Fatal(err)
	}

	verified, stale, missing, errs := s.VerifyAllFacts(dir)
	if verified == 0 {
		t.Error("expected at least some verified facts")
	}
	if missing == 0 {
		t.Error("expected at least one missing ref")
	}
	if len(errs) > 0 {
		t.Errorf("unexpected errors: %v", errs)
	}
	_ = stale // may be 0 or more depending on timestamps
}

// ── LastUpdated ─────────────────────────────────────────────────────────

func TestLastUpdatedEmptyStore(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: filepath.Join(dir, "memory")}
	lu := s.LastUpdated()
	if !lu.IsZero() {
		t.Errorf("empty store LastUpdated should be zero, got %v", lu)
	}
}

func TestLastUpdatedWithFacts(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: filepath.Join(dir, "memory")}
	now := time.Now()

	if _, err := s.Save(Memory{
		Name:        "earlier",
		Description: "earlier",
		Type:        TypeProject,
		Body:        "body",
		CreatedAt:   now.Add(-2 * time.Hour),
		UpdatedAt:   now.Add(-2 * time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	lu := s.LastUpdated()
	if lu.IsZero() {
		t.Error("LastUpdated should be non-zero")
	}

}

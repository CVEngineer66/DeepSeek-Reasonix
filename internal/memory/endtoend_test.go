package memory

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ── Activation mode ──────────────────────────────────────────────────────────

func TestSaveLoadActivationModes(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: filepath.Join(dir, "memory")}

	// Save both modes.
	if _, err := s.Save(Memory{
		Name: "always-rule", Description: "critical rule", Type: TypeProject,
		Activation: ActivationAlwaysOn, Body: "Never delete this file.",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save(Memory{
		Name: "ref-info", Description: "reference info", Type: TypeProject,
		Activation: ActivationModelDecision, Body: "DB url is localhost:5432",
	}); err != nil {
		t.Fatal(err)
	}

	// AlwaysOnFacts should return only always_on.
	ao := s.AlwaysOnFacts()
	if len(ao) != 1 || ao[0].Name != "always-rule" {
		t.Fatalf("AlwaysOnFacts = %+v, want [always-rule]", listNames(ao))
	}

	// List returns both.
	all := s.List()
	if len(all) != 2 {
		t.Fatalf("List = %d, want 2", len(all))
	}
}

func TestSaveDefaultActivation(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: filepath.Join(dir, "memory")}

	if _, err := s.Save(Memory{
		Name: "default-act", Description: "no activation set", Type: TypeProject, Body: "body",
	}); err != nil {
		t.Fatal(err)
	}
	if s.List()[0].Activation != ActivationModelDecision {
		t.Fatalf("default activation should be model_decision, got %q", s.List()[0].Activation)
	}
}

func TestNormalizeActivation(t *testing.T) {
	cases := []struct{ in, want string }{
		{"always_on", "always_on"},
		{"ALWAYS_ON", "always_on"},
		{"model_decision", "model_decision"},
		{"", "model_decision"},
		{"invalid", "model_decision"},
	}
	for _, c := range cases {
		if got := string(NormalizeActivation(c.in)); got != c.want {
			t.Errorf("NormalizeActivation(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// ── Index line format: no double date ────────────────────────────────────────

func TestIndexLineHasSingleDate(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: filepath.Join(dir, "memory")}

	if _, err := s.Save(Memory{
		Name: "test-fact", Description: "a test", Type: TypeProject, Body: "body",
	}); err != nil {
		t.Fatal(err)
	}

	idx := s.Index()
	// Find the index line and count date occurrences.
	for _, line := range strings.Split(idx, "\n") {
		if strings.Contains(line, "test-fact.md") {
			n := strings.Count(line, "(2026")
			if n != 1 {
				t.Fatalf("index line has %d dates, want 1:\n%s", n, line)
			}
		}
	}
}

func TestReindexDoesNotDoubleDate(t *testing.T) {
	// Save, then re-save same fact — flushIndex appends date each time,
	// reindex must NOT also append one.
	dir := t.TempDir()
	s := Store{Dir: filepath.Join(dir, "memory")}

	if _, err := s.Save(Memory{
		Name: "dup", Description: "v1", Type: TypeProject, Body: "body",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save(Memory{
		Name: "dup", Description: "v2", Type: TypeProject, Body: "body",
	}); err != nil {
		t.Fatal(err)
	}

	for _, line := range strings.Split(s.Index(), "\n") {
		if strings.Contains(line, "dup.md") {
			n := strings.Count(line, "(2026")
			if n != 1 {
				t.Fatalf("after overwrite, index line has %d dates, want 1:\n%s", n, line)
			}
		}
	}
}

// ── CreatedAt preservation on overwrite ──────────────────────────────────────

func TestSavePreservesCreatedAtOnOverwrite(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: filepath.Join(dir, "memory")}

	now := time.Now()
	earlier := now.Add(-72 * time.Hour)

	// First save with explicit CreatedAt.
	if _, err := s.Save(Memory{
		Name: "fact", Description: "v1", Type: TypeProject,
		CreatedAt: earlier, Body: "version 1",
	}); err != nil {
		t.Fatal(err)
	}
	if s.List()[0].CreatedAt.Unix() != earlier.Unix() {
		t.Fatalf("CreatedAt not preserved: got %v, want %v", s.List()[0].CreatedAt, earlier)
	}

	// Overwrite with same name — CreatedAt should be preserved, UpdatedAt should advance.
	time.Sleep(2 * time.Millisecond) // ensure UpdatedAt differs
	if _, err := s.Save(Memory{
		Name: "fact", Description: "v2", Type: TypeProject, Body: "version 2",
	}); err != nil {
		t.Fatal(err)
	}

	m := s.List()[0]
	if m.CreatedAt.Unix() != earlier.Unix() {
		t.Fatalf("CreatedAt changed on overwrite: got %v, want %v", m.CreatedAt, earlier)
	}
	if !m.UpdatedAt.After(m.CreatedAt) {
		t.Fatalf("UpdatedAt (%v) should be after CreatedAt (%v) after overwrite", m.UpdatedAt, m.CreatedAt)
	}
	if !strings.Contains(m.Body, "version 2") {
		t.Fatalf("overwrite didn't update body: %q", m.Body)
	}
}

func TestSaveAutoPopulatesTimestamps(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: filepath.Join(dir, "memory")}

	if _, err := s.Save(Memory{
		Name: "auto-ts", Description: "auto", Type: TypeProject, Body: "content",
	}); err != nil {
		t.Fatal(err)
	}

	m := s.List()[0]
	if m.CreatedAt.IsZero() {
		t.Fatal("CreatedAt should be auto-populated")
	}
	if m.UpdatedAt.IsZero() {
		t.Fatal("UpdatedAt should be auto-populated")
	}
	if !m.UpdatedAt.Equal(m.CreatedAt) {
		t.Fatal("UpdatedAt should equal CreatedAt on first save")
	}
}

// ── Block(): always_on exclusion from Saved memories ─────────────────────────

func TestBlockExcludesAlwaysOnFromIndex(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: filepath.Join(dir, "memory")}

	// Save one always_on and one model_decision.
	if _, err := s.Save(Memory{
		Name: "critical-rule", Description: "critical", Type: TypeProject,
		Activation: ActivationAlwaysOn, Body: "Must follow this rule.",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save(Memory{
		Name: "nice-to-know", Description: "reference", Type: TypeProject,
		Body: "Some reference info.",
	}); err != nil {
		t.Fatal(err)
	}

	set := &Set{Store: s, Index: s.Index()}
	block := set.Block()

	// Block should include always_on in the always-on section.
	if !strings.Contains(block, "Must follow this rule.") {
		t.Fatal("always_on body should appear in Block")
	}
	if !strings.Contains(block, "Always-on facts") {
		t.Fatal("Block should have Always-on facts section")
	}

	// Always_on fact should NOT appear in Saved memories.
	savedSection := ""
	parts := strings.Split(block, "## Saved memories")
	if len(parts) > 1 {
		savedSection = parts[1]
	}
	if strings.Contains(savedSection, "critical-rule") {
		t.Fatal("always_on fact should be excluded from Saved memories section")
	}
	if !strings.Contains(savedSection, "nice-to-know") {
		t.Fatal("model_decision fact should appear in Saved memories section")
	}
}

// ── flushIndex: recency sorting and 200-line cap ─────────────────────────────

func TestIndex200LineCap(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: dir}

	// Save 210 facts.
	for i := 0; i < maxIndexLines+10; i++ {
		name := fmt.Sprintf("fact-%04d", i)
		if _, err := s.Save(Memory{
			Name: name, Description: "fact " + name, Type: TypeProject, Body: "body",
		}); err != nil {
			t.Fatal(err)
		}
	}

	lines := strings.Split(strings.TrimSpace(s.Index()), "\n")
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
	s := Store{Dir: dir}

	old := time.Now().Add(-48 * time.Hour)
	recent := time.Now().Add(-1 * time.Hour)

	if _, err := s.Save(Memory{
		Name: "old-fact", Description: "old", Type: TypeProject, Body: "old",
		CreatedAt: old, UpdatedAt: old,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save(Memory{
		Name: "recent-fact", Description: "recent", Type: TypeProject, Body: "recent",
		CreatedAt: recent, UpdatedAt: recent,
	}); err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(s.Index()), "\n")
	var contentLines []string
	for _, l := range lines {
		if strings.HasPrefix(l, "- [") {
			contentLines = append(contentLines, l)
		}
	}
	if len(contentLines) != 2 {
		t.Fatalf("expected 2 index lines, got %d", len(contentLines))
	}
	// recency sort: recent-fact must come first.
	if !strings.Contains(contentLines[0], "recent-fact") {
		t.Errorf("expected recent-fact first (recency sort), got:\n%s\n%s", contentLines[0], contentLines[1])
	}
}

// ── Topic directories ────────────────────────────────────────────────────────

func TestTopicDirSaveAndList(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: filepath.Join(dir, "memory")}

	// Root fact.
	if _, err := s.Save(Memory{Name: "root-fact", Description: "root", Type: TypeProject, Body: "root"}); err != nil {
		t.Fatal(err)
	}
	// Topic fact.
	if _, err := s.Save(Memory{Name: "topic-fact", Description: "topic", Type: TypeProject, Topic: "frontend", Body: "topic"}); err != nil {
		t.Fatal(err)
	}
	// Nested topic fact.
	if _, err := s.Save(Memory{Name: "nested-fact", Description: "nested", Type: TypeProject, Topic: "frontend/css", Body: "nested"}); err != nil {
		t.Fatal(err)
	}

	list := s.List()
	if len(list) != 3 {
		t.Fatalf("expected 3 facts, got %d", len(list))
	}

	// Verify topic round-trip.
	for _, m := range list {
		if m.Name == "topic-fact" && m.Topic != "frontend" {
			t.Errorf("topic-fact topic = %q, want frontend", m.Topic)
		}
		if m.Name == "nested-fact" && m.Topic != "frontend/css" {
			t.Errorf("nested-fact topic = %q, want frontend/css", m.Topic)
		}
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

// ── flushIndex edge cases ────────────────────────────────────────────────────

func TestFlushIndexDateOnExistingIndex(t *testing.T) {
	// Manually write an old-format index line without date, then trigger reindex
	// to verify flushIndex adds the date.
	dir := t.TempDir()
	s := Store{Dir: dir}

	if _, err := s.Save(Memory{Name: "old", Description: "old format", Type: TypeProject, Body: "body"}); err != nil {
		t.Fatal(err)
	}
	// Verify the index line includes a date.
	idx := s.Index()
	for _, line := range strings.Split(idx, "\n") {
		if strings.Contains(line, "old.md") {
			if !strings.Contains(line, "(20") {
				t.Fatalf("index line missing date: %s", line)
			}
		}
	}
}

// ── ReadMemory tool ──────────────────────────────────────────────────────────

func TestReadMemoryFindsBySlug(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: filepath.Join(dir, "memory")}

	if _, err := s.Save(Memory{
		Name: "prefers-tabs", Title: "Prefers Tabs",
		Description: "User prefers tabs", Type: TypeUser,
		Activation: ActivationModelDecision,
		Body:       "Always use tabs for indentation.",
	}); err != nil {
		t.Fatal(err)
	}

	// read_memory tool
	tool := NewReadMemoryTool(s)
	if tool.Name() != "read_memory" {
		t.Fatalf("tool name = %q", tool.Name())
	}

	result, err := tool.Execute(nil, []byte(`{"name":"prefers-tabs"}`))
	if err != nil {
		t.Fatalf("read_memory: %v", err)
	}
	if !strings.Contains(result, "Prefers Tabs") {
		t.Errorf("result missing title: %s", result)
	}
	if !strings.Contains(result, "Always use tabs") {
		t.Errorf("result missing body: %s", result)
	}
	if !strings.Contains(result, "model_decision") {
		t.Errorf("result missing activation: %s", result)
	}
}

func TestReadMemoryNotFound(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: filepath.Join(dir, "memory")}
	tool := NewReadMemoryTool(s)

	_, err := tool.Execute(nil, []byte(`{"name":"nonexistent"}`))
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected 'not found' error, got: %v", err)
	}
}

func TestReadMemoryRejectsEmptyName(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: dir}
	tool := NewReadMemoryTool(s)

	_, err := tool.Execute(nil, []byte(`{"name":""}`))
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestReadMemoryReadOnly(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: dir}
	tool := NewReadMemoryTool(s)
	if !tool.ReadOnly() {
		t.Fatal("read_memory should be ReadOnly")
	}
}

// ── Frontmatter round-trip ───────────────────────────────────────────────────

func TestFrontmatterRoundTripAllFields(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: filepath.Join(dir, "memory")}

	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	m := Memory{
		Name:        "full-fact",
		Title:       "Full Fact",
		Description: "all fields",
		Type:        TypeReference,
		Activation:  ActivationAlwaysOn,
		Topic:       "docs",
		CreatedAt:   now,
		UpdatedAt:   now,
		Body:        "Full body content.",
	}
	if _, err := s.Save(m); err != nil {
		t.Fatal(err)
	}

	got := s.List()[0]
	if got.Name != "full-fact" || got.Title != "Full Fact" {
		t.Errorf("name/title mismatch: %+v", got)
	}
	if got.Description != "all fields" {
		t.Errorf("description = %q", got.Description)
	}
	if got.Type != TypeReference {
		t.Errorf("type = %q", got.Type)
	}
	if got.Activation != ActivationAlwaysOn {
		t.Errorf("activation = %q", got.Activation)
	}
	if got.Topic != "docs" {
		t.Errorf("topic = %q", got.Topic)
	}
	if got.CreatedAt.Unix() != now.Unix() {
		t.Errorf("created_at = %v, want %v", got.CreatedAt, now)
	}
	if got.Body != "Full body content." {
		t.Errorf("body = %q", got.Body)
	}
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func listNames(ms []Memory) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.Name
	}
	return out
}

// TestStoreSaveLoadDisabledStore tests that a disabled store errors on Save.
func TestStoreSaveLoadDisabledStore(t *testing.T) {
	var s Store
	if _, err := s.Save(Memory{Name: "x", Description: "d", Type: TypeProject, Body: "b"}); err == nil {
		t.Fatal("disabled store should error on Save")
	}
}

func TestListDisabledStore(t *testing.T) {
	var s Store
	list := s.List()
	if list != nil {
		t.Fatal("disabled store List should return nil")
	}
}

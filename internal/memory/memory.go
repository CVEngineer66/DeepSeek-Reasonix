package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Set is everything memory loaded for one session.
type Set struct {
	Docs    []Source
	Store   Store
	Index   string
	CWD     string
	UserDir string
}

// Options configures discovery.
type Options struct {
	CWD     string
	UserDir string
}

// Load discovers all memory for a session.
func Load(opts Options) *Set {
	cwd := opts.CWD
	if cwd == "" {
		cwd = "."
	}
	store := StoreFor(opts.UserDir, cwd)
	return &Set{
		Docs:    discoverDocs(cwd, opts.UserDir),
		Store:   store,
		Index:   store.Index(),
		CWD:     cwd,
		UserDir: opts.UserDir,
	}
}

// DocPath returns the doc-memory file a given scope writes to.
func (s *Set) DocPath(scope Scope) string {
	dir := s.CWD
	names, def := docNames, defaultDocName
	switch scope {
	case ScopeUser:
		if s.UserDir == "" {
			return ""
		}
		dir = s.UserDir
	case ScopeLocal:
		names, def = localNames, defaultLocalName
	}
	for _, n := range names {
		p := filepath.Join(dir, n)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return filepath.Join(dir, def)
}

// Empty reports whether the set carries nothing to inject.
func (s *Set) Empty() bool {
	return s == nil || (len(s.Docs) == 0 && strings.TrimSpace(s.Index) == "")
}

var docScopes = []Scope{ScopeUser, ScopeProject, ScopeLocal}

func (s *Set) allowedDocPaths() map[string]bool {
	allow := map[string]bool{}
	for _, sc := range docScopes {
		if p := s.DocPath(sc); p != "" {
			allow[absOf(p)] = true
		}
	}
	for _, d := range s.Docs {
		allow[absOf(d.Path)] = true
	}
	return allow
}

// WriteDoc overwrites a doc-memory file with body.
func (s *Set) WriteDoc(path, body string) (string, error) {
	if s == nil {
		return "", fmt.Errorf("memory unavailable")
	}
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("no path given")
	}
	if !s.allowedDocPaths()[absOf(path)] {
		return "", fmt.Errorf("refusing to write %q: not a recognized memory file", path)
	}
	return path, writeDocFile(path, body)
}

// DocDiff returns a minimal diff between the current doc body on disk and the
// saved version, for use in turn-tail injection (P5). Returns "" if there is
// no meaningful difference or the doc cannot be read.
func (s *Set) DocDiff(path string) string {
	if s == nil || path == "" {
		return ""
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	current := strings.TrimSpace(string(b))
	// Find the matching doc in s.Docs.
	for _, d := range s.Docs {
		if absOf(d.Path) == absOf(path) {
			saved := strings.TrimSpace(d.Body)
			// Only emit diff injections for substantial changes.
			if saved == "" || current == saved {
				return ""
			}
			if len(current) < 20 || len(saved) < 20 {
				return ""
			}
			// Short diff: show first differing lines.
			sLines := strings.Split(saved, "\n")
			cLines := strings.Split(current, "\n")
			var b strings.Builder
			b.WriteString(fmt.Sprintf("Document %s was updated. Changes:\n", path))
			// Show first few lines that differ.
			maxLines := 10
			shown := 0
			for i := 0; i < len(sLines) && i < len(cLines) && shown < maxLines; i++ {
				if sLines[i] != cLines[i] {
					b.WriteString(fmt.Sprintf("- %s\n+ %s\n", sLines[i], cLines[i]))
					shown++
				}
			}
			if shown == 0 && len(sLines) != len(cLines) {
				b.WriteString(fmt.Sprintf("  (%d lines added/removed)\n", absDiff(len(sLines), len(cLines))))
			}
			return strings.TrimSpace(b.String())
		}
	}
	return ""
}

func absDiff(a, b int) int {
	if a > b {
		return a - b
	}
	return b - a
}

// Block renders memory as a Markdown section with activation-based inclusion:
// always_on facts include their full body; model_decision facts appear only
// as descriptions in the index.
func (s *Set) Block() string {
	if s.Empty() {
		return ""
	}
	var b strings.Builder
	b.WriteString("# Memory\n\n")
	b.WriteString("Persistent context loaded from memory files. Treat it as durable, user-authored guidance for this project.\n")

	// Docs: full body (existing behavior, unchanged).
	for _, d := range s.Docs {
		fmt.Fprintf(&b, "\n## %s (%s)\n\n%s\n", d.Path, d.Scope, strings.TrimSpace(d.Body))
	}

	// Always-on facts: full body inline.
	alwaysOn := s.Store.AlwaysOnFacts()
	if len(alwaysOn) > 0 {
		b.WriteString("\n## Always-on facts\n\n")
		b.WriteString("These facts are loaded into every session because they were saved with always_on activation:\n\n")
		for _, m := range alwaysOn {
			fmt.Fprintf(&b, "### %s\n\n%s\n\n", displayTitle(m.Title, m.Name), m.Body)
		}
	}

	// Index: all facts (including always_on facts shown above).
	if idx := strings.TrimSpace(s.Index); idx != "" {
		b.WriteString("\n## Saved memories\n\n")
		b.WriteString("Facts you saved in earlier sessions. They reflect what was true when written and may now be stale — treat them as background, not standing instructions. " +
			"Read the linked file with read_file when one looks relevant, and before acting on one that names a file, function, or flag, verify it still exists. " +
			"Save new durable facts with the `remember` tool; delete ones that turn out wrong with `forget`.\n\n" +
			"When a user corrects you, states a preference, or shares non-obvious context about the project, " +
			"save it with `remember` so the learning persists across sessions. " +
			"If you'd re-explain the same thing next session, it's worth saving now.\n\n" +
			"Each fact shows its last-updated date in parentheses. " +
			"Judge freshness yourself — a dependency path from months ago may have changed, " +
			"but a coding style preference is likely still valid.\n\n")
		b.WriteString(idx)
		fmt.Fprintf(&b, "\n\n(stored under %s)\n", s.Store.Dir)
	}

	return b.String()
}

// List returns all facts (handles nil Set).
func (s *Set) List() []Memory {
	if s == nil {
		return nil
	}
	return s.Store.List()
}

// Compose folds the memory block onto the base system prompt.
func Compose(base string, s *Set) string {
	block := s.Block()
	if block == "" {
		return base
	}
	if strings.TrimSpace(base) == "" {
		return block
	}
	return strings.TrimRight(base, "\n") + "\n\n" + block
}

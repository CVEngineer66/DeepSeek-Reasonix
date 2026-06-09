package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"reasonix/internal/frontmatter"
)

// ActivationMode controls how a fact is injected into the system prompt.
type ActivationMode string

const (
	ActivationAlwaysOn      ActivationMode = "always_on"      // full body always in system prompt
	ActivationModelDecision ActivationMode = "model_decision" // description in index, body loaded via read_file on demand
)

var validActivations = map[ActivationMode]bool{
	ActivationAlwaysOn:      true,
	ActivationModelDecision: true,
}

// NormalizeActivation coerces to a known ActivationMode, defaulting to
// model_decision so new facts are discoverable but don't waste prefix.
func NormalizeActivation(s string) ActivationMode {
	a := ActivationMode(strings.ToLower(strings.TrimSpace(s)))
	if validActivations[a] {
		return a
	}
	return ActivationModelDecision
}

const (
	// maxIndexLines caps the MEMORY.md index so the system prompt never
	// grows unbounded. Lines beyond this are dropped from the index (they
	// stay on disk and can still be found by name).
	maxIndexLines = 200

	// alwaysOnMaxBody caps the total body size of always_on facts in the
	// system prompt so one verbose fact can't crowd out the rest.
	alwaysOnMaxBody = 4096
)

// Store is the per-project auto-memory: a directory of one-fact-per-file
// Markdown notes with frontmatter, plus a MEMORY.md index of one line per fact.
type Store struct {
	Dir string
}

// Type classifies a memory, mirroring the auto-memory taxonomy.
type Type string

const (
	TypeUser      Type = "user"
	TypeFeedback  Type = "feedback"
	TypeProject   Type = "project"
	TypeReference Type = "reference"
)

var validTypes = map[Type]bool{TypeUser: true, TypeFeedback: true, TypeProject: true, TypeReference: true}

func NormalizeType(s string) Type {
	t := Type(strings.ToLower(strings.TrimSpace(s)))
	if validTypes[t] {
		return t
	}
	return TypeProject
}

// Memory is one stored fact.
type Memory struct {
	Name        string // kebab-case slug; also the file stem (<name>.md)
	Title       string // human-readable index label
	Description string // one-line summary for the index
	Type        Type
	Activation  ActivationMode // how this fact loads into context
	Topic       string         // subdirectory topic ("" for root); e.g. "frontend-style"
	CreatedAt   time.Time      // when the fact was first saved
	UpdatedAt   time.Time      // when the fact was last updated
	Body        string         // the fact itself (Markdown)
}

// StoreFor resolves the auto-memory directory for a project working dir.
func StoreFor(userDir, cwd string) Store {
	if userDir == "" {
		return Store{}
	}
	return Store{Dir: filepath.Join(userDir, "projects", slugify(absOf(cwd)), "memory")}
}

const indexFile = "MEMORY.md"

func slugify(absPath string) string {
	r := strings.NewReplacer(string(os.PathSeparator), "-", "/", "-", "\\", "-", ":", "-", " ", "-")
	return r.Replace(absPath)
}

// Index returns the MEMORY.md contents.
func (s Store) Index() string {
	if s.Dir == "" {
		return ""
	}
	b, err := os.ReadFile(filepath.Join(s.Dir, indexFile))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// Path returns the absolute file path a memory with the given name lives at.
func (s Store) Path(name string) string {
	return filepath.Join(s.Dir, slug(name)+".md")
}

// Save writes (or overwrites) a memory file and refreshes the index.
func (s Store) Save(m Memory) (string, error) {
	if s.Dir == "" {
		return "", fmt.Errorf("memory store unavailable (no user config dir)")
	}
	name := slug(m.Name)
	if name == "" {
		return "", fmt.Errorf("memory needs a name")
	}
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return "", err
	}

	now := time.Now()
	// Preserve explicit timestamps from the caller (used in tests and migration).
	// Only auto-populate when zero.
	if m.CreatedAt.IsZero() {
		// On overwrite, keep the original created_at from the existing file.
		if existing, ok := s.loadOne(name); ok && !existing.CreatedAt.IsZero() {
			m.CreatedAt = existing.CreatedAt
		} else {
			m.CreatedAt = now
		}
	}
	if m.UpdatedAt.IsZero() {
		m.UpdatedAt = now
	}
	if m.UpdatedAt.Before(m.CreatedAt) {
		m.UpdatedAt = m.CreatedAt
	}
	if m.Activation == "" {
		m.Activation = ActivationModelDecision
	}

	path := filepath.Join(s.Dir, topicDir(m.Topic), name+".md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(render(m, name)), 0o644); err != nil {
		return "", err
	}
	if err := s.reindex(name, m); err != nil {
		return path, err
	}
	return path, nil
}

// topicDir returns the subdirectory path for a topic, or "" for root.
func topicDir(topic string) string {
	if topic == "" {
		return ""
	}
	return slug(topic)
}

// Delete removes a memory file and its index line.
func (s Store) Delete(name string) error {
	if s.Dir == "" {
		return fmt.Errorf("memory store unavailable (no user config dir)")
	}
	name = slug(name)
	if name == "" {
		return fmt.Errorf("memory needs a name")
	}
	// Search all topic subdirectories for the file.
	_ = removeMemoryFile(filepath.Join(s.Dir, name+".md"))
	if entries, err := os.ReadDir(s.Dir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				_ = removeMemoryFile(filepath.Join(s.Dir, e.Name(), name+".md"))
			}
		}
	}
	return s.flushIndex(s.indexLinesExcept(name))
}

func removeMemoryFile(path string) error {
	err := os.Remove(path)
	if err == nil || os.IsNotExist(err) {
		return nil
	}
	if !os.IsPermission(err) {
		return err
	}
	repairOwnerWrite(path, false)
	repairOwnerWrite(filepath.Dir(path), true)
	err = os.Remove(path)
	if err == nil || os.IsNotExist(err) {
		return nil
	}
	return err
}

func repairOwnerWrite(path string, dir bool) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	need := os.FileMode(0o600)
	if dir {
		need = 0o700
	}
	_ = os.Chmod(path, info.Mode().Perm()|need)
}

// render serializes a memory to frontmatter + body.
func render(m Memory, name string) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("name: " + name + "\n")
	if t := oneLine(m.Title); t != "" {
		b.WriteString("title: " + t + "\n")
	}
	b.WriteString("description: " + oneLine(m.Description) + "\n")
	b.WriteString("activation: " + string(m.Activation) + "\n")
	if m.Topic != "" {
		b.WriteString("topic: " + m.Topic + "\n")
	}
	b.WriteString("created_at: " + m.CreatedAt.Format(time.RFC3339) + "\n")
	b.WriteString("updated_at: " + m.UpdatedAt.Format(time.RFC3339) + "\n")
	b.WriteString("metadata:\n")
	b.WriteString("  type: " + string(NormalizeType(string(m.Type))) + "\n")
	b.WriteString("---\n\n")
	b.WriteString(strings.TrimSpace(m.Body))
	b.WriteString("\n")
	return b.String()
}

var indexLineRe = regexp.MustCompile(`\]\(([^)]+)\.md\)`)

func (s Store) indexLinesExcept(name string) map[string]string {
	existing, _ := os.ReadFile(filepath.Join(s.Dir, indexFile))
	keep := map[string]string{}
	for _, line := range strings.Split(string(existing), "\n") {
		if mt := indexLineRe.FindStringSubmatch(line); mt != nil && mt[1] != name {
			keep[mt[1]] = strings.TrimRight(line, "\r")
		}
	}
	return keep
}

// flushIndex rewrites MEMORY.md with 200-line cap, sorted by recency
// (most recently updated first) so the most relevant facts stay visible.
// Each index line includes the last-updated date so the model can judge
// freshness for itself.
func (s Store) flushIndex(lines map[string]string) error {
	if s.Dir == "" {
		return nil
	}

	// Build entries with recency info for sorting.
	type entry struct {
		slug      string
		line      string
		updatedAt time.Time
	}
	entries := make([]entry, 0, len(lines))
	for n, l := range lines {
		e := entry{slug: n, line: l}
		if m, ok := s.loadOne(n); ok {
			e.updatedAt = m.UpdatedAt
			if !m.UpdatedAt.IsZero() {
				e.line = l + " (" + m.UpdatedAt.Format("2006-01-02") + ")"
			}
		}
		entries = append(entries, e)
	}

	// Sort by recency (most recently updated first).
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].updatedAt.After(entries[j].updatedAt)
	})

	// Apply 200-line cap.
	if len(entries) > maxIndexLines {
		entries = entries[:maxIndexLines]
	}

	var b strings.Builder
	b.WriteString("# Memory\n\n")
	for _, e := range entries {
		b.WriteString(e.line)
		b.WriteString("\n")
	}
	return os.WriteFile(filepath.Join(s.Dir, indexFile), []byte(b.String()), 0o644)
}

// loadOne loads a single memory by slug.
func (s Store) loadOne(name string) (Memory, bool) {
	path := filepath.Join(s.Dir, name+".md")
	if m, ok := loadMemory(path); ok {
		return m, true
	}
	// Check topic subdirectories.
	if entries, err := os.ReadDir(s.Dir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				if m, ok := loadMemory(filepath.Join(s.Dir, e.Name(), name+".md")); ok {
					return m, true
				}
			}
		}
	}
	return Memory{}, false
}

// reindex rewrites the MEMORY.md line for name. Date is appended by flushIndex.
func (s Store) reindex(name string, m Memory) error {
	lines := s.indexLinesExcept(name)
	line := fmt.Sprintf("- [%s](%s.md) — %s", displayTitle(m.Title, name), name, oneLine(m.Description))
	lines[name] = line
	return s.flushIndex(lines)
}

// List returns all saved memories, including those in topic subdirectories.
func (s Store) List() []Memory {
	return s.listDir(s.Dir, "")
}

func (s Store) listDir(dir, topic string) []Memory {
	if s.Dir == "" || dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []Memory
	for _, e := range entries {
		if e.IsDir() {
			subTopic := e.Name()
			if topic != "" {
				subTopic = topic + "/" + e.Name()
			}
			out = append(out, s.listDir(filepath.Join(dir, e.Name()), subTopic)...)
			continue
		}
		if e.Name() == indexFile || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		if m, ok := loadMemory(filepath.Join(dir, e.Name())); ok {
			if topic != "" && m.Topic == "" {
				m.Topic = topic
			}
			out = append(out, m)
		}
	}
	return out
}

// AlwaysOnFacts returns only always_on activation facts with their full bodies,
// for inclusion in the system prompt.
func (s Store) AlwaysOnFacts() []Memory {
	all := s.List()
	out := make([]Memory, 0, len(all))
	var totalBody int
	for _, m := range all {
		if m.Activation != ActivationAlwaysOn {
			continue
		}
		totalBody += len(m.Body)
		if totalBody > alwaysOnMaxBody {
			// Truncate the body to fit within the limit.
			excess := totalBody - alwaysOnMaxBody
			if len(m.Body) > excess {
				m.Body = m.Body[:len(m.Body)-excess] + "\n\n[body truncated by always_on size limit]"
			}
		}
		out = append(out, m)
	}
	return out
}

// loadMemory parses one fact file back into a Memory.
func loadMemory(path string) (Memory, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Memory{}, false
	}
	fm, body := splitFrontmatter(string(b))
	m := Memory{
		Name:        fm["name"],
		Title:       fm["title"],
		Description: fm["description"],
		Type:        NormalizeType(fm["type"]),
		Activation:  NormalizeActivation(fm["activation"]),
		Topic:       fm["topic"],
		Body:        strings.TrimSpace(body),
	}
	if t, err := time.Parse(time.RFC3339, fm["created_at"]); err == nil {
		m.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339, fm["updated_at"]); err == nil {
		m.UpdatedAt = t
	}
	if m.Name == "" {
		m.Name = strings.TrimSuffix(filepath.Base(path), ".md")
	}
	if m.Activation == "" {
		m.Activation = ActivationModelDecision
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now()
	}
	if m.UpdatedAt.IsZero() {
		m.UpdatedAt = m.CreatedAt
	}
	return m, true
}

func splitFrontmatter(s string) (map[string]string, string) {
	return frontmatter.Split(s)
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

func slug(s string) string {
	return strings.Trim(slugRe.ReplaceAllString(strings.ToLower(strings.TrimSpace(s)), "-"), "-")
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func displayTitle(title, name string) string {
	if t := oneLine(title); t != "" {
		return t
	}
	return strings.ReplaceAll(name, "-", " ")
}

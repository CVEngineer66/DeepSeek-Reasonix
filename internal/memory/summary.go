package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SessionSummary captures learnings from one session for automatic fact extraction.
type SessionSummary struct {
	SessionPath  string    `json:"-"`             // the session this summary belongs to
	Learned      []string  `json:"learned"`       // new facts discovered in this session
	ChangedRules []string  `json:"changed_rules"` // user-requested practice changes
	FollowUps    []string  `json:"follow_ups"`    // incomplete items
	SavedAt      time.Time `json:"saved_at"`
}

// summaryFileName is the session-sidecar summary file.
const summaryFileSuffix = ".summary.json"

// SummaryPath returns the summary file path for a session file.
func SummaryPath(sessionPath string) string {
	return sessionPath + summaryFileSuffix
}

// SaveSummary writes a session summary to disk alongside the session file.
func SaveSummary(sessionPath string, summary SessionSummary) error {
	summary.SavedAt = time.Now()
	summary.SessionPath = sessionPath
	b, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(SummaryPath(sessionPath), b, 0o644)
}

// LoadSummary reads a session summary from disk.
func LoadSummary(sessionPath string) (SessionSummary, error) {
	b, err := os.ReadFile(SummaryPath(sessionPath))
	if err != nil {
		return SessionSummary{}, err
	}
	var s SessionSummary
	if err := json.Unmarshal(b, &s); err != nil {
		return SessionSummary{}, err
	}
	s.SessionPath = sessionPath
	return s, nil
}

// ExtractNewFactsFromSummary extracts new facts from a session summary
// and saves them to the store. Returns the names of facts saved.
func (s Store) ExtractNewFactsFromSummary(summary SessionSummary) []string {
	var saved []string
	for _, learned := range summary.Learned {
		learned = strings.TrimSpace(learned)
		if learned == "" {
			continue
		}
		// Deduplicate: check if a similar fact already exists.
		existing := s.List()
		duplicate := false
		for _, m := range existing {
			if strings.EqualFold(oneLine(m.Description), oneLine(learned)) {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		// Create a new fact from the learned item.
		name := slug(learned)
		if name == "" {
			continue
		}
		m := Memory{
			Name:        name,
			Title:       name,
			Description: oneLine(learned),
			Type:        TypeProject,
			Activation:  ActivationModelDecision,
			Body:        learned,
			CreatedAt:   time.Now(),
		}
		if _, err := s.Save(m); err == nil {
			saved = append(saved, name)
		}
	}
	return saved
}

// ExtractAndPersistFacts extracts facts from all recent summaries
// and persists any new ones. Intended to run at startup.
func (s Store) ExtractAndPersistFacts(sessionDir string, maxSessions int) (int, error) {
	entries, err := os.ReadDir(sessionDir)
	if err != nil {
		return 0, err
	}
	var count int
	seen := 0
	// Iterate most recent sessions last (alphabetical order by filename).
	for i := len(entries) - 1; i >= 0 && seen < maxSessions; i-- {
		e := entries[i]
		if e.IsDir() || !strings.HasSuffix(e.Name(), summaryFileSuffix) {
			continue
		}
		sum, err := LoadSummary(filepath.Join(sessionDir, strings.TrimSuffix(e.Name(), summaryFileSuffix)))
		if err != nil {
			continue
		}
		saved := s.ExtractNewFactsFromSummary(sum)
		count += len(saved)
		seen++
	}
	return count, nil
}

// BuildSessionSummary constructs a summary from a session's message history.
// It extracts potential facts from the most recent user and assistant exchanges.
// This is a best-effort helper; the actual extraction should be model-driven.
func BuildSessionSummary(messages []struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}) SessionSummary {
	var summary SessionSummary
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.Role != "assistant" || msg.Content == "" {
			continue
		}
		// Look for "remember" tool calls indicating important facts.
		if strings.Contains(msg.Content, "Saved memory") {
			continue // already persisted
		}
		// Extract lines that look like decisions or learnings.
		for _, line := range strings.Split(msg.Content, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			// Look for decision markers.
			if strings.HasPrefix(line, "**Why:**") || strings.Contains(strings.ToLower(line), "decided to") {
				summary.Learned = append(summary.Learned, line)
			}
		}
	}
	return summary
}

// FormatSummaryText returns a human-readable string of the session summary.
func FormatSummaryText(summary SessionSummary) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("## Session Summary (%s)\n\n", summary.SavedAt.Format("2006-01-02 15:04")))
	if len(summary.Learned) > 0 {
		b.WriteString("### Learned\n\n")
		for _, l := range summary.Learned {
			b.WriteString(fmt.Sprintf("- %s\n", l))
		}
		b.WriteString("\n")
	}
	if len(summary.ChangedRules) > 0 {
		b.WriteString("### Changed rules\n\n")
		for _, r := range summary.ChangedRules {
			b.WriteString(fmt.Sprintf("- %s\n", r))
		}
		b.WriteString("\n")
	}
	if len(summary.FollowUps) > 0 {
		b.WriteString("### Follow-ups\n\n")
		for _, f := range summary.FollowUps {
			b.WriteString(fmt.Sprintf("- %s\n", f))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// Multi-skill management for the AI features. A "skill" is a Markdown checklist
// the vision AI follows. Skills live as *.md files in the folder of GK_SKILL_PATH
// (dashboard/skillmd/), so managers can maintain several and pick which ones a
// given Deep Analisis / Deep Revisi run applies.
package service

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// SkillMeta is a lightweight listing entry for a skill file.
type SkillMeta struct {
	Name  string `json:"name"`  // slug = filename without .md (stable id)
	Title string `json:"title"` // first "# " heading, else the name
	Size  int    `json:"size"`  // bytes
}

// skillNameRe restricts skill names to a safe slug so they can never escape the
// skills directory (no "..", slashes, etc.).
var skillNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// skillsDir is the folder holding the *.md skill files (dir of GK_SKILL_PATH).
func (s *Service) skillsDir() string {
	p := strings.TrimSpace(s.gk.SkillPath)
	if p == "" {
		return ""
	}
	return filepath.Dir(p)
}

func (s *Service) skillPath(name string) (string, error) {
	if !skillNameRe.MatchString(name) {
		return "", fmt.Errorf("%w: nama skill tidak valid (huruf kecil, angka, tanda minus)", ErrValidation)
	}
	dir := s.skillsDir()
	if dir == "" {
		return "", fmt.Errorf("%w: folder skill tidak dikonfigurasi (GK_SKILL_PATH)", ErrValidation)
	}
	return filepath.Join(dir, name+".md"), nil
}

// skillTitle extracts the first "# " heading from markdown, else "".
func skillTitle(md string) string {
	for _, ln := range strings.Split(md, "\n") {
		ln = strings.TrimSpace(ln)
		if strings.HasPrefix(ln, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(ln, "# "))
		}
	}
	return ""
}

// ListSkills returns every *.md skill in the skills folder (sorted by name).
func (s *Service) ListSkills() ([]SkillMeta, error) {
	dir := s.skillsDir()
	if dir == "" {
		return nil, fmt.Errorf("%w: folder skill tidak dikonfigurasi (GK_SKILL_PATH)", ErrValidation)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []SkillMeta{}, nil
		}
		return nil, err
	}
	out := []SkillMeta{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".md") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		if !skillNameRe.MatchString(name) {
			continue
		}
		body, _ := os.ReadFile(filepath.Join(dir, e.Name()))
		title := skillTitle(string(body))
		if title == "" {
			title = name
		}
		out = append(out, SkillMeta{Name: name, Title: title, Size: len(body)})
	}
	return out, nil
}

// ReadSkill returns a skill's full markdown content.
func (s *Service) ReadSkill(name string) (string, error) {
	path, err := s.skillPath(name)
	if err != nil {
		return "", err
	}
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrNotFound
		}
		return "", err
	}
	return string(body), nil
}

// WriteSkill overwrites a skill's markdown (must already exist).
func (s *Service) WriteSkill(name, content string) error {
	path, err := s.skillPath(name)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err != nil {
		return ErrNotFound
	}
	if strings.TrimSpace(content) == "" {
		return fmt.Errorf("%w: skill tidak boleh kosong", ErrValidation)
	}
	if len(content) > 200000 {
		return fmt.Errorf("%w: skill terlalu panjang (maks 200 KB)", ErrValidation)
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// CreateSkill creates a new skill file with a starter template. Fails if it
// already exists.
func (s *Service) CreateSkill(name, title string) (SkillMeta, error) {
	path, err := s.skillPath(name)
	if err != nil {
		return SkillMeta{}, err
	}
	if _, err := os.Stat(path); err == nil {
		return SkillMeta{}, fmt.Errorf("%w: skill dengan nama itu sudah ada", ErrValidation)
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return SkillMeta{}, err
		}
	}
	t := strings.TrimSpace(title)
	if t == "" {
		t = name
	}
	body := "# " + t + "\n\n> Checklist yang diikuti AI vision.\n\n- Poin pertama…\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return SkillMeta{}, err
	}
	return SkillMeta{Name: name, Title: t, Size: len(body)}, nil
}

// DeleteSkill removes a skill file.
func (s *Service) DeleteSkill(name string) error {
	path, err := s.skillPath(name)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

// loadSkillsCombined reads the selected skills and concatenates them for a
// vision prompt. Unknown/empty names are skipped; if nothing usable is selected
// it falls back to the default GK checklist so a run is never skill-less.
func (s *Service) loadSkillsCombined(names []string) string {
	var parts []string
	total := 0
	const cap = 12000
	for _, n := range names {
		body, err := s.ReadSkill(strings.TrimSpace(n))
		if err != nil || strings.TrimSpace(body) == "" {
			continue
		}
		if total+len(body) > cap {
			body = body[:maxInt(0, cap-total)]
		}
		parts = append(parts, body)
		total += len(body)
		if total >= cap {
			break
		}
	}
	if len(parts) == 0 {
		return s.loadGKSkill() // default single checklist
	}
	return strings.Join(parts, "\n\n---\n\n")
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Cek AI for board card attachments — async vision QC of ONE attachment (PDF
// or image) against a checklist skill, mirroring the Deep Analisis (taskai.go)
// job/status pattern: one run at a time per card, in-memory state, poll via
// GET .../ai-check. PDFs reuse the render_pages.py -> visionSingle flow; images
// (JPG/PNG/WebP) are fed to the vision step directly. On completion a plain
// Indonesian summary comment (author "ai") is appended to the card — that
// mutation persists through the normal store and fires the realtime notifier.
package service

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"greenpark/perencanaan/internal/domain"
)

// ErrBoardAIRunning signals a Cek AI run is already in flight for the card
// (mapped to HTTP 409 by the transport).
var ErrBoardAIRunning = errors.New("pemeriksaan AI untuk kartu ini masih berjalan")

// BoardAICheckState is the poll payload of GET /api/board/cards/{id}/ai-check.
type BoardAICheckState struct {
	Status    string             `json:"status"` // idle | running | done | error
	AttID     string             `json:"attId,omitempty"`
	Summary   string             `json:"summary,omitempty"`
	Findings  []domain.GKFinding `json:"findings"`
	Error     string             `json:"error,omitempty"`
	CheckedAt string             `json:"checkedAt,omitempty"` // RFC3339, last completed run
}

// boardAIKind classifies an attachment for Cek AI: "pdf", "image" (with its
// mime), or ok=false when the type is unsupported.
func boardAIKind(att domain.BoardAttachment) (kind, mimeType string, ok bool) {
	ext := strings.ToLower(filepath.Ext(att.Name))
	m := strings.ToLower(strings.TrimSpace(att.Mime))
	switch {
	case ext == ".pdf" || strings.HasPrefix(m, "application/pdf"):
		return "pdf", "application/pdf", true
	case ext == ".jpg" || ext == ".jpeg" || m == "image/jpeg":
		return "image", "image/jpeg", true
	case ext == ".png" || m == "image/png":
		return "image", "image/png", true
	case ext == ".webp" || m == "image/webp":
		return "image", "image/webp", true
	}
	return "", "", false
}

// BoardStartAICheck kicks off an async Cek AI run on one card attachment.
// Contributor only. Returns ErrBoardAIRunning (409) when the card already has
// a run in flight, and a validation error (400) for unsupported types.
// skillName optionally picks one checklist skill; empty falls back to the
// default GK checklist (same default as Deep Analisis).
func (s *Service) BoardStartAICheck(actor domain.User, cardID, attID, skillName, token string) error {
	card, ok := s.repo.BoardCard(cardID)
	if !ok {
		return ErrNotFound
	}
	if !boardContributor(actor, card) {
		return ErrForbidden
	}
	attID = strings.TrimSpace(attID)
	var att *domain.BoardAttachment
	for i := range card.Attachments {
		if card.Attachments[i].ID == attID {
			att = &card.Attachments[i]
			break
		}
	}
	if att == nil {
		return fmt.Errorf("%w: lampiran tidak ditemukan di kartu ini", ErrValidation)
	}
	kind, mimeType, supported := boardAIKind(*att)
	if !supported {
		return fmt.Errorf("%w: tipe lampiran tidak didukung untuk Cek AI — hanya PDF atau gambar (JPG/PNG/WebP)", ErrValidation)
	}
	path := filepath.Join(s.uploadDir, att.ID)
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("%w: file lampiran tidak ditemukan di penyimpanan", ErrValidation)
	}
	// Vision runs via auth's central key — verify it is set there first (same
	// preflight as Deep Analisis).
	configured, _, _, err := s.authAIConfig(token)
	if err != nil {
		return fmt.Errorf("%w: gagal cek Kunci AI pusat: %v", ErrValidation, err)
	}
	if !configured {
		return fmt.Errorf("%w: Kunci AI belum diset — atur di Panel Admin (Kunci AI)", ErrValidation)
	}
	var skills []string
	if sk := strings.TrimSpace(skillName); sk != "" {
		skills = []string{sk}
	}

	s.boardAIMu.Lock()
	if st, ok := s.boardAIJobs[cardID]; ok && st.Status == "running" {
		s.boardAIMu.Unlock()
		return ErrBoardAIRunning
	}
	s.boardAIJobs[cardID] = &BoardAICheckState{Status: "running", AttID: att.ID, Findings: []domain.GKFinding{}}
	s.boardAIMu.Unlock()

	go s.runBoardAICheck(cardID, *att, kind, mimeType, path, token, skills)
	return nil
}

// BoardAICheckStatus returns the card's current Cek AI state (poll target).
// A card that never ran (or a restarted server — state is in-memory only)
// reports status "idle".
func (s *Service) BoardAICheckStatus(cardID string) (BoardAICheckState, error) {
	if _, ok := s.repo.BoardCard(cardID); !ok {
		return BoardAICheckState{}, ErrNotFound
	}
	s.boardAIMu.Lock()
	defer s.boardAIMu.Unlock()
	if st, ok := s.boardAIJobs[cardID]; ok {
		out := *st
		out.Findings = append([]domain.GKFinding{}, st.Findings...)
		return out, nil
	}
	return BoardAICheckState{Status: "idle", Findings: []domain.GKFinding{}}, nil
}

// setBoardAIState mutates the card's job state under the lock.
func (s *Service) setBoardAIState(cardID string, fn func(*BoardAICheckState)) {
	s.boardAIMu.Lock()
	defer s.boardAIMu.Unlock()
	if st, ok := s.boardAIJobs[cardID]; ok {
		fn(st)
	}
}

// runBoardAICheck is the background pipeline: render/encode -> vision per page
// -> record findings -> auto-comment (author "ai") on success.
func (s *Service) runBoardAICheck(cardID string, att domain.BoardAttachment, kind, mimeType, path, token string, skillNames []string) {
	fail := func(msg string) {
		s.setBoardAIState(cardID, func(st *BoardAICheckState) {
			st.Status = "error"
			st.Error = msg
		})
	}

	skill := s.loadSkillsCombined(skillNames)
	findings := []domain.GKFinding{}

	switch kind {
	case "image":
		data, err := os.ReadFile(path)
		if err != nil {
			fail("gagal baca file lampiran: " + err.Error())
			return
		}
		dataURL := "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data)
		pf, err := s.visionSingleURL(token, skill, dataURL)
		findings = appendPageFindings(findings, 1, pf, err)

	default: // "pdf" — reuse the Deep Analisis render -> vision flow
		tmpDir, err := os.MkdirTemp("", "boardai-*")
		if err != nil {
			fail("gagal buat folder sementara: " + err.Error())
			return
		}
		defer os.RemoveAll(tmpDir)
		data, err := os.ReadFile(path)
		if err != nil {
			fail("gagal baca file lampiran: " + err.Error())
			return
		}
		pdfPath := filepath.Join(tmpDir, "doc.pdf")
		if err := os.WriteFile(pdfPath, data, 0o644); err != nil {
			fail("gagal tulis PDF sementara: " + err.Error())
			return
		}
		// Single-doc render: the PDF goes in the "kontraktor" slot, "-" skips
		// the TTD side (same convention as taskai.go).
		manifest, err := s.renderPages(pdfPath, "-", tmpDir)
		if err != nil {
			fail("gagal render PDF ke gambar: " + err.Error())
			return
		}
		imgs := manifest.Kontraktor.Images
		for i := 0; i < len(imgs); i++ {
			pf, err := s.visionSingle(token, skill, imgs[i])
			findings = appendPageFindings(findings, i+1, pf, err)
			// Stream findings-so-far to the poller (same UX as Deep Analisis).
			progress := append([]domain.GKFinding{}, findings...)
			s.setBoardAIState(cardID, func(st *BoardAICheckState) { st.Findings = progress })
		}
	}

	summary := boardAISummary(att.Name, findings)
	checkedAt := s.now().Format(time.RFC3339)
	final := append([]domain.GKFinding{}, findings...)
	s.setBoardAIState(cardID, func(st *BoardAICheckState) {
		st.Status = "done"
		st.Summary = summary
		st.Findings = final
		st.Error = ""
		st.CheckedAt = checkedAt
	})

	// Auto-append the result comment (author "ai"). This is a normal card
	// mutation, so it persists through the store; notify the realtime hub so
	// open dashboards refresh without a page reload.
	text := boardAIComment(att.Name, summary, findings)
	now := s.nowRFC3339()
	_, found, err := s.repo.MutateBoardCard(cardID, func(c *domain.BoardCard, newID func(string) string) error {
		c.Comments = append(c.Comments, domain.BoardComment{ID: newID("cm"), Author: "ai", Text: text, At: now})
		c.UpdatedAt = now
		return nil
	})
	if found && err == nil {
		s.notifyChange()
	}
}

// boardAISummary builds the one-line Indonesian result summary.
func boardAISummary(name string, findings []domain.GKFinding) string {
	if len(findings) == 0 {
		return fmt.Sprintf("Cek AI selesai: tidak ditemukan masalah pada %q.", name)
	}
	return fmt.Sprintf("Cek AI selesai: %d temuan pada %q.", len(findings), name)
}

// boardAIComment renders the auto-comment: summary + top findings, plain text.
func boardAIComment(name, summary string, findings []domain.GKFinding) string {
	var b strings.Builder
	b.WriteString(summary)
	const maxListed = 5
	for i, f := range findings {
		if i >= maxListed {
			b.WriteString(fmt.Sprintf("\n… dan %d temuan lainnya (lihat detail Cek AI).", len(findings)-maxListed))
			break
		}
		line := strings.TrimSpace(f.Explain)
		if w, c := strings.TrimSpace(f.Wrong), strings.TrimSpace(f.Correct); w != "" || c != "" {
			pair := w
			if c != "" {
				pair += " -> " + c
			}
			if line != "" {
				line = pair + " (" + line + ")"
			} else {
				line = pair
			}
		}
		if line == "" {
			line = "temuan tanpa keterangan"
		}
		b.WriteString(fmt.Sprintf("\n%d. [hal %d] %s", i+1, f.Page, line))
	}
	return b.String()
}

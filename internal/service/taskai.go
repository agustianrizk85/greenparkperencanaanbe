// Deep Analisis AI for review deliverables — single-document vision QC of a
// task's uploaded PDF (Doc) against the checklist skill. It reuses the Deep
// Revisi machinery (render_pages.py + visionSingle + the central-key auth proxy)
// but keys progress/findings off the Task instead of a WorkDrawing, and skips
// annotation: the review flow only needs the per-page findings list.
package service

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"greenpark/perencanaan/internal/domain"
)

// StartTaskAI kicks off an async single-document analysis of a task's review
// PDF against the selected skill(s). Returns immediately; poll TaskAIStatus for
// progress. The task must have an uploaded Doc and the CENTRAL Kunci AI must be
// set (Panel Admin → Kunci AI). Empty skillNames falls back to the default GK
// checklist.
func (s *Service) StartTaskAI(projectID, taskID, token string, skillNames []string) error {
	data, _, ok := s.repo.TaskDocBytes(projectID, taskID)
	if !ok || len(data) == 0 {
		return fmt.Errorf("%w: unggah PDF dulu sebelum analisis AI", ErrValidation)
	}
	// Vision runs via auth's central key — verify it is set there first.
	configured, _, _, err := s.authAIConfig(token)
	if err != nil {
		return fmt.Errorf("%w: gagal cek Kunci AI pusat: %v", ErrValidation, err)
	}
	if !configured {
		return fmt.Errorf("%w: Kunci AI belum diset — atur di Panel Admin (Kunci AI)", ErrValidation)
	}
	already := false
	found := s.repo.MutateTask(projectID, taskID, func(t *domain.Task) {
		if t.AIStatus == domain.GKRunning {
			already = true
			return
		}
		t.AIStatus = domain.GKRunning
		t.AIError = ""
		t.AIFindings = nil
		t.AIAnnotated = nil
		t.AISkills = skillNames
		t.AIDone, t.AITotal = 0, 0
	})
	if !found {
		return ErrNotFound
	}
	if already {
		return nil // idempotent — a run is already in flight
	}
	go s.runTaskAICheck(projectID, taskID, token, skillNames)
	return nil
}

// TaskAIAnnotated returns the annotated Deep Analisis result PDF for a task.
func (s *Service) TaskAIAnnotated(projectID, taskID string) ([]byte, string, bool) {
	return s.repo.TaskAIAnnotatedBytes(projectID, taskID)
}

// TaskAIStatus returns the task's current Deep Analisis state (poll target).
func (s *Service) TaskAIStatus(projectID, taskID string) (domain.Task, error) {
	var out domain.Task
	ok := s.repo.MutateTask(projectID, taskID, func(t *domain.Task) { out = *t })
	if !ok {
		return domain.Task{}, ErrNotFound
	}
	return out, nil
}

func (s *Service) failTaskAI(projectID, taskID, msg string) {
	s.repo.MutateTask(projectID, taskID, func(t *domain.Task) {
		t.AIStatus = domain.GKFailed
		t.AIError = msg
	})
}

// setTaskAIProgress records pages-analysed so the polling frontend shows a
// running percentage.
func (s *Service) setTaskAIProgress(projectID, taskID string, done, total int) {
	s.repo.MutateTask(projectID, taskID, func(t *domain.Task) {
		t.AIDone, t.AITotal = done, total
	})
}

func (s *Service) runTaskAICheck(projectID, taskID, token string, skillNames []string) {
	data, docName, ok := s.repo.TaskDocBytes(projectID, taskID)
	if !ok || len(data) == 0 {
		s.failTaskAI(projectID, taskID, "dokumen tidak ditemukan")
		return
	}

	tmpDir, err := os.MkdirTemp("", "taskai-*")
	if err != nil {
		s.failTaskAI(projectID, taskID, "gagal buat folder sementara: "+err.Error())
		return
	}
	defer os.RemoveAll(tmpDir)

	pdfPath := filepath.Join(tmpDir, "doc.pdf")
	if err := os.WriteFile(pdfPath, data, 0o644); err != nil {
		s.failTaskAI(projectID, taskID, "gagal tulis PDF: "+err.Error())
		return
	}

	// Single-doc render: the PDF goes in the "kontraktor" slot, "-" skips the TTD
	// side (same convention runGKCheck uses for a single drawing).
	manifest, err := s.renderPages(pdfPath, "-", tmpDir)
	if err != nil {
		s.failTaskAI(projectID, taskID, "gagal render PDF ke gambar: "+err.Error())
		return
	}

	// Apply the selected skill(s); falls back to the default checklist if none.
	skill := s.loadSkillsCombined(skillNames)
	imgs := manifest.Kontraktor.Images
	findings := []domain.GKFinding{}
	s.setTaskAIProgress(projectID, taskID, 0, len(imgs))
	for i := 0; i < len(imgs); i++ {
		pf, err := s.visionSingle(token, skill, imgs[i])
		findings = appendPageFindings(findings, i+1, pf, err)
		// Stream findings-so-far to the poller so the UI shows what the AI is
		// finding while it is still working.
		s.repo.MutateTask(projectID, taskID, func(t *domain.Task) {
			t.AIFindings = findings
			t.AIDone, t.AITotal = i+1, len(imgs)
		})
	}

	// Annotate the checked PDF (circle + strike + correction note per finding),
	// producing the result PDF the user can open.
	outPath := filepath.Join(tmpDir, "annotated.pdf")
	annotatedName := "hasil-analisis-" + docName
	if err := s.annotate(pdfPath, findings, outPath); err != nil {
		// Annotation failure must not lose the findings — record them + a note.
		s.repo.MutateTask(projectID, taskID, func(t *domain.Task) {
			t.AIStatus = domain.GKDone
			t.AIFindings = findings
			t.AIError = "PDF hasil gagal dibuat: " + err.Error()
			t.AICheckedAt = s.now().Format(time.RFC3339)
		})
		return
	}
	if annotatedBytes, err := os.ReadFile(outPath); err == nil {
		s.repo.SetTaskAIAnnotated(projectID, taskID, annotatedName, annotatedBytes)
	}

	s.repo.MutateTask(projectID, taskID, func(t *domain.Task) {
		t.AIStatus = domain.GKDone
		t.AIFindings = findings
		t.AICheckedAt = s.now().Format(time.RFC3339)
	})
}

// Deep Revisi AI — vision-based comparison of GK Kontraktor vs GK TTD PDFs,
// producing an annotated correction PDF. Self-contained in this service
// (talks to Ollama Cloud directly) rather than routed through the shared
// be/auth AI proxy — see plan notes for why.
//
// Pipeline (runGKCheck, run in a background goroutine so the triggering HTTP
// request returns immediately):
//  1. Shell out to scripts/gk/render_pages.py to rasterize every page of both
//     PDFs to PNG (no Go PDF-rendering library exists in this codebase).
//  2. For each page pair (matched by index — MVP assumes same sheet order),
//     call Ollama Cloud's vision-capable model with both page images plus the
//     Gambar Kerja checklist (dashboard/skillmd/pengecekan-gambar-kerja.md),
//     asking for a JSON list of inconsistencies.
//  3. Shell out to scripts/gk/annotate.py to draw "coretan" (circle + strike +
//     correction note) onto GK Kontraktor for every finding, producing the
//     notes_ PDF the skill's manual procedure describes.
package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"greenpark/perencanaan/internal/domain"
)

// GKConfig configures the Deep Revisi AI feature. The zero value disables it
// (Configured() returns false) so the service still runs fine without it.
type GKConfig struct {
	OllamaModel string // vision model (default; overridable at runtime from the UI)
	PythonBin   string
	ScriptsDir  string
	SkillPath   string
	// AuthAPIBase is the auth service base URL (…/api). Deep Revisi proxies its
	// vision calls through auth so the CENTRAL Ollama key never leaves auth.
	AuthAPIBase string
}

// GKKeyStatus asks auth whether the CENTRAL Kunci AI is set + returns the
// general model and the VISION model — both managed in Panel Admin → Kunci AI.
// The modal shows this read-only ("pakai Kunci AI pusat · model vision X").
func (s *Service) GKKeyStatus(token string) (configured bool, model, visionModel string) {
	configured, model, visionModel, _ = s.authAIConfig(token)
	return
}

// maxGKDocBytes caps an uploaded GK PDF (100 MiB — real CAD-exported Gambar
// Kerja sets can be very large; sample seen: 11.5 MB).
const maxGKDocBytes = 100 << 20

/* ---- Upload ------------------------------------------------------------- */

// UploadGKDoc stores a GK Kontraktor or GK TTD PDF against a work drawing.
// kind must be "kontraktor" or "ttd".
func (s *Service) UploadGKDoc(actor domain.User, wdID, kind, filename string, data []byte) (WorkDrawingView, error) {
	if kind != "kontraktor" && kind != "ttd" {
		return WorkDrawingView{}, ErrValidation
	}
	if !strings.HasSuffix(strings.ToLower(filename), ".pdf") {
		return WorkDrawingView{}, fmt.Errorf("%w: hanya file PDF yang diperbolehkan", ErrValidation)
	}
	if len(data) == 0 {
		return WorkDrawingView{}, fmt.Errorf("%w: file kosong", ErrValidation)
	}
	if len(data) > maxGKDocBytes {
		return WorkDrawingView{}, fmt.Errorf("%w: ukuran PDF %d MB melebihi batas %d MB", ErrValidation, len(data)>>20, maxGKDocBytes>>20)
	}
	doc := domain.GKDoc{
		Name: filename, Size: len(data),
		UploadedBy: actor.Username, UploadedAt: s.now().Format(time.RFC3339),
	}
	if !s.repo.SetWorkDrawingDoc(wdID, kind, doc, data) {
		return WorkDrawingView{}, ErrNotFound
	}
	updated, ok := s.repo.MutateWorkDrawing(wdID, func(d *domain.WorkDrawing) {
		if d.GKStatus == "" {
			d.GKStatus = domain.GKIdle
		}
	})
	if !ok {
		return WorkDrawingView{}, ErrNotFound
	}
	projects := s.repo.Projects()
	return s.viewWorkDrawing(updated, projectName(projects, updated.ProjectID), s.today()), nil
}

// GKDocBytes returns the raw bytes of a stored GK document (kind =
// "kontraktor"|"ttd"|"annotated") for serving back to the browser.
func (s *Service) GKDocBytes(wdID, kind string) ([]byte, string, error) {
	data, name, ok := s.repo.WorkDrawingDocBytes(wdID, kind)
	if !ok {
		return nil, "", ErrNotFound
	}
	return data, name, nil
}

/* ---- Start / status ------------------------------------------------------ */

// StartDeepRevisi kicks off an async GK check. Returns immediately; poll
// GKStatus for progress. At least ONE document must be uploaded: with both it
// COMPARES GK Kontraktor vs GK TTD; with only one it QCs that single drawing
// against the checklist.
func (s *Service) StartDeepRevisi(wdID, token string, skillNames []string) error {
	updated, ok := s.repo.MutateWorkDrawing(wdID, func(d *domain.WorkDrawing) {})
	if !ok {
		return ErrNotFound
	}
	if updated.GKKontraktor == nil && updated.GKTTD == nil {
		return fmt.Errorf("%w: upload minimal satu GK (Kontraktor atau TTD) dulu", ErrValidation)
	}
	// The Ollama key is the CENTRAL one held by the auth service — verify it is
	// set there (Panel Admin → Kunci AI) before starting.
	configured, _, _, err := s.authAIConfig(token)
	if err != nil {
		return fmt.Errorf("%w: gagal cek Kunci AI pusat: %v", ErrValidation, err)
	}
	if !configured {
		return fmt.Errorf("%w: Kunci AI belum diset — atur di Panel Admin (Kunci AI)", ErrValidation)
	}
	if updated.GKStatus == domain.GKRunning {
		return nil // already running, treat as idempotent
	}
	s.repo.MutateWorkDrawing(wdID, func(d *domain.WorkDrawing) {
		d.GKStatus = domain.GKRunning
		d.GKError = ""
		d.GKFindings = nil
		d.GKDone, d.GKTotal = 0, 0
	})
	go s.runGKCheck(wdID, token, skillNames)
	return nil
}

// GKStatus returns the current Deep Revisi AI state for a work drawing (poll
// target for the frontend).
func (s *Service) GKStatus(wdID string) (WorkDrawingView, error) {
	updated, ok := s.repo.MutateWorkDrawing(wdID, func(d *domain.WorkDrawing) {})
	if !ok {
		return WorkDrawingView{}, ErrNotFound
	}
	projects := s.repo.Projects()
	return s.viewWorkDrawing(updated, projectName(projects, updated.ProjectID), s.today()), nil
}

func (s *Service) failGK(wdID, msg string) {
	s.repo.MutateWorkDrawing(wdID, func(d *domain.WorkDrawing) {
		d.GKStatus = domain.GKFailed
		d.GKError = msg
	})
}

// setGKProgress records how many pages have been analysed (done/total) so the
// polling frontend can show a running percentage.
func (s *Service) setGKProgress(wdID string, done, total int) {
	s.repo.MutateWorkDrawing(wdID, func(d *domain.WorkDrawing) {
		d.GKDone, d.GKTotal = done, total
	})
}

/* ---- Pipeline ------------------------------------------------------------- */

func (s *Service) runGKCheck(wdID, token string, skillNames []string) {
	kBytes, kName, hasK := s.repo.WorkDrawingDocBytes(wdID, "kontraktor")
	tBytes, tName, hasT := s.repo.WorkDrawingDocBytes(wdID, "ttd")
	if !hasK && !hasT {
		s.failGK(wdID, "tidak ada GK yang diunggah")
		return
	}

	tmpDir, err := os.MkdirTemp("", "gkcheck-*")
	if err != nil {
		s.failGK(wdID, "gagal buat folder sementara: "+err.Error())
		return
	}
	defer os.RemoveAll(tmpDir)

	// Write whichever docs are present; pass "-" for an absent side so the
	// renderer skips it (single-doc mode).
	kArg, tArg := "-", "-"
	if hasK {
		kArg = filepath.Join(tmpDir, "kontraktor.pdf")
		if err := os.WriteFile(kArg, kBytes, 0o644); err != nil {
			s.failGK(wdID, "gagal tulis GK Kontraktor: "+err.Error())
			return
		}
	}
	if hasT {
		tArg = filepath.Join(tmpDir, "ttd.pdf")
		if err := os.WriteFile(tArg, tBytes, 0o644); err != nil {
			s.failGK(wdID, "gagal tulis GK TTD: "+err.Error())
			return
		}
	}

	manifest, err := s.renderPages(kArg, tArg, tmpDir)
	if err != nil {
		s.failGK(wdID, "gagal render PDF ke gambar: "+err.Error())
		return
	}

	skill := s.loadSkillsCombined(skillNames)
	var findings []domain.GKFinding

	if hasK && hasT {
		// COMPARE mode — GK Kontraktor vs GK TTD, page pair by page pair.
		pageCount := len(manifest.Kontraktor.Images)
		if len(manifest.TTD.Images) < pageCount {
			pageCount = len(manifest.TTD.Images)
		}
		s.setGKProgress(wdID, 0, pageCount)
		for i := 0; i < pageCount; i++ {
			pf, err := s.visionCompare(token, skill, manifest.Kontraktor.Images[i], manifest.TTD.Images[i])
			findings = appendPageFindings(findings, i+1, pf, err)
			s.setGKProgress(wdID, i+1, pageCount)
		}
	} else {
		// SINGLE mode — QC the one uploaded drawing against the checklist.
		imgs := manifest.Kontraktor.Images
		if !hasK {
			imgs = manifest.TTD.Images
		}
		s.setGKProgress(wdID, 0, len(imgs))
		for i := 0; i < len(imgs); i++ {
			pf, err := s.visionSingle(token, skill, imgs[i])
			findings = appendPageFindings(findings, i+1, pf, err)
			s.setGKProgress(wdID, i+1, len(imgs))
		}
	}

	// Annotate the drawing that was checked (Kontraktor when present, else TTD).
	primaryPath, primaryName := kArg, kName
	if !hasK {
		primaryPath, primaryName = tArg, tName
	}
	outPath := filepath.Join(tmpDir, "annotated.pdf")
	if err := s.annotate(primaryPath, findings, outPath); err != nil {
		s.failGK(wdID, "gagal buat PDF beranotasi: "+err.Error())
		return
	}
	annotatedBytes, err := os.ReadFile(outPath)
	if err != nil {
		s.failGK(wdID, "gagal baca PDF beranotasi: "+err.Error())
		return
	}

	doc := domain.GKDoc{
		Name: "notes_" + primaryName, Size: len(annotatedBytes),
		UploadedBy: "deep-revisi-ai", UploadedAt: s.now().Format(time.RFC3339),
	}
	s.repo.SetWorkDrawingDoc(wdID, "annotated", doc, annotatedBytes)
	s.repo.MutateWorkDrawing(wdID, func(d *domain.WorkDrawing) {
		d.GKStatus = domain.GKDone
		d.GKFindings = findings
		d.GKCheckedAt = s.now().Format(time.RFC3339)
	})
}

// appendPageFindings tags each finding with its page number, or records a
// per-page failure placeholder (one page failing must not abort the rest).
func appendPageFindings(findings []domain.GKFinding, page int, pf []domain.GKFinding, err error) []domain.GKFinding {
	if err != nil {
		return append(findings, domain.GKFinding{
			Page:       page,
			Explain:    "Gagal analisis AI halaman ini: " + err.Error(),
			Confidence: "rendah",
		})
	}
	for _, f := range pf {
		f.Page = page
		findings = append(findings, f)
	}
	return findings
}

/* ---- Python helper shell-outs -------------------------------------------- */

type gkRenderSide struct {
	Pages  int      `json:"pages"`
	Images []string `json:"images"`
}

type gkRenderManifest struct {
	Kontraktor gkRenderSide `json:"kontraktor"`
	TTD        gkRenderSide `json:"ttd"`
}

func (s *Service) renderPages(kontraktorPath, ttdPath, outDir string) (gkRenderManifest, error) {
	script := filepath.Join(s.gk.ScriptsDir, "render_pages.py")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.gk.PythonBin, script, kontraktorPath, ttdPath, outDir)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return gkRenderManifest{}, fmt.Errorf("%s: %s", err, strings.TrimSpace(stderr.String()))
	}
	var manifest gkRenderManifest
	if err := json.Unmarshal(stdout.Bytes(), &manifest); err != nil {
		return gkRenderManifest{}, fmt.Errorf("parse manifest: %w", err)
	}
	return manifest, nil
}

func (s *Service) annotate(kontraktorPath string, findings []domain.GKFinding, outPath string) error {
	type findingJSON struct {
		Page       int    `json:"page"`
		Wrong      string `json:"wrong"`
		Correct    string `json:"correct"`
		Explain    string `json:"explain"`
		Confidence string `json:"confidence"`
	}
	list := make([]findingJSON, 0, len(findings))
	for _, f := range findings {
		if strings.TrimSpace(f.Wrong) == "" {
			continue // analysis-failure placeholder, nothing to draw
		}
		list = append(list, findingJSON{
			Page: f.Page, Wrong: f.Wrong, Correct: f.Correct,
			Explain: f.Explain, Confidence: f.Confidence,
		})
	}
	findingsBytes, err := json.Marshal(list)
	if err != nil {
		return err
	}
	findingsPath := filepath.Join(filepath.Dir(outPath), "findings.json")
	if err := os.WriteFile(findingsPath, findingsBytes, 0o644); err != nil {
		return err
	}

	script := filepath.Join(s.gk.ScriptsDir, "annotate.py")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.gk.PythonBin, script, kontraktorPath, findingsPath, outPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

/* ---- Vision via the auth AI proxy (central key stays inside auth) ---------- */

// authGetJSON GETs auth <base>+path with the caller's bearer token into v.
func (s *Service) authGetJSON(token, path string, v any) error {
	base := strings.TrimRight(s.gk.AuthAPIBase, "/")
	if base == "" {
		base = "http://127.0.0.1:8090/api"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("auth %s: status %d", path, res.StatusCode)
	}
	return json.NewDecoder(res.Body).Decode(v)
}

// authAIConfig asks auth whether the CENTRAL Ollama key is set (Panel Admin →
// Kunci AI) plus the general + vision models, using the caller's token.
func (s *Service) authAIConfig(token string) (bool, string, string, error) {
	var out struct {
		Configured  bool   `json:"configured"`
		Model       string `json:"model"`
		VisionModel string `json:"visionModel"`
	}
	if err := s.authGetJSON(token, "/ai/config", &out); err != nil {
		return false, "", "", err
	}
	return out.Configured, out.Model, out.VisionModel, nil
}

// callVision proxies a vision request (prompt + images) to auth's /ai/vision,
// which runs it with the CENTRAL key + the selected vision model, then parses
// the findings from the reply.
func (s *Service) callVision(token, prompt string, images []string) ([]domain.GKFinding, error) {
	base := strings.TrimRight(s.gk.AuthAPIBase, "/")
	if base == "" {
		base = "http://127.0.0.1:8090/api"
	}
	// Empty model → auth uses its central VISION model (Panel Admin → Kunci AI).
	body, _ := json.Marshal(map[string]any{"model": "", "prompt": prompt, "images": images})
	ctx, cancel := context.WithTimeout(context.Background(), 115*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/ai/vision", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	res, err := (&http.Client{Timeout: 120 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	var parsed struct {
		Content string `json:"content"`
		Error   string `json:"error"`
	}
	if err := json.NewDecoder(res.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("baca respons AI: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		if parsed.Error != "" {
			return nil, fmt.Errorf("AI vision %d: %s", res.StatusCode, parsed.Error)
		}
		return nil, fmt.Errorf("AI vision: status %d", res.StatusCode)
	}
	return parseGKFindings(parsed.Content), nil
}

// visionCompare sends one page pair (Kontraktor vs TTD) to the vision model via
// the auth proxy and returns the findings it reports for that page.
func (s *Service) visionCompare(token, skill, kontraktorImgPath, ttdImgPath string) ([]domain.GKFinding, error) {
	kImg, err := imageDataURL(kontraktorImgPath)
	if err != nil {
		return nil, err
	}
	tImg, err := imageDataURL(ttdImgPath)
	if err != nil {
		return nil, err
	}
	prompt := "Kamu adalah arsitek QC yang memeriksa Gambar Kerja properti Indonesia. " +
		"Gambar PERTAMA adalah halaman dari GK TTD (sudah disetujui, acuan benar). " +
		"Gambar KEDUA adalah halaman yang bersesuaian dari GK Kontraktor (draft yang diperiksa). " +
		"Ikuti checklist berikut secara ketat:\n\n" + skill + "\n\n" +
		"Bandingkan kedua gambar HANYA untuk hal yang benar-benar terlihat berbeda (jangan mengarang). " +
		"Balas HANYA JSON valid tanpa markdown fence, persis format: " +
		`{"findings":[{"wrong":"nilai/kondisi di GK Kontraktor","correct":"nilai/kondisi di GK TTD",` +
		`"explain":"penjelasan singkat","confidence":"tinggi|sedang|rendah"}]}` +
		` (array kosong jika kedua gambar sudah konsisten).`
	return s.callVision(token, prompt, []string{tImg, kImg})
}

// visionSingle QCs ONE Gambar Kerja page against the checklist (no comparison
// drawing) — used when only GK Kontraktor or only GK TTD was uploaded.
func (s *Service) visionSingle(token, skill, imgPath string) ([]domain.GKFinding, error) {
	img, err := imageDataURL(imgPath)
	if err != nil {
		return nil, err
	}
	return s.visionSingleURL(token, skill, img)
}

// visionSingleURL is visionSingle for an already-encoded data: URL image —
// lets callers with non-PNG sources (board Cek AI on JPG/WebP attachments)
// keep the correct mime type.
func (s *Service) visionSingleURL(token, skill, img string) ([]domain.GKFinding, error) {
	prompt := "Kamu adalah arsitek QC yang memeriksa SATU halaman Gambar Kerja properti Indonesia " +
		"terhadap checklist standar (tidak ada gambar pembanding). Periksa konsistensi INTERNAL halaman ini: " +
		"kop gambar (Luas Bangunan/Tanah), dimensi & level, kesesuaian denah dengan tampak/potongan, tinggi kusen. " +
		"Ikuti checklist berikut secara ketat:\n\n" + skill + "\n\n" +
		"Laporkan HANYA ketidaksesuaian yang benar-benar terlihat (jangan mengarang). " +
		"Balas HANYA JSON valid tanpa markdown fence, persis format: " +
		`{"findings":[{"wrong":"nilai/kondisi yang salah","correct":"nilai/kondisi seharusnya",` +
		`"explain":"penjelasan singkat","confidence":"tinggi|sedang|rendah"}]}` +
		` (array kosong jika halaman sudah sesuai).`
	return s.callVision(token, prompt, []string{img})
}

// parseGKFindings tolerantly extracts the findings array from the model's
// reply (first {...} block — models occasionally wrap JSON in prose/fences).
func parseGKFindings(out string) []domain.GKFinding {
	i := strings.Index(out, "{")
	j := strings.LastIndex(out, "}")
	if i < 0 || j <= i {
		return nil
	}
	var parsed struct {
		Findings []domain.GKFinding `json:"findings"`
	}
	if err := json.Unmarshal([]byte(out[i:j+1]), &parsed); err != nil {
		return nil
	}
	return parsed.Findings
}

func imageDataURL(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(data), nil
}

/* ---- Skill loading --------------------------------------------------------- */

// gkFallbackSkill keeps the pipeline principled even if the skill file moved.
const gkFallbackSkill = `Cek: (1) Luas Bangunan/Luas Tanah di kop gambar harus sama persis dengan acuan.
(2) Denah: dimensi, level lantai, posisi jendela/pintu/carport harus sama.
(3) Tampak & Potongan harus konsisten dengan denah (elevasi, posisi bukaan).
(4) Detail kusen: tinggi kusen/pintu/jendela pada dinding yang sama harus konsisten.
(5) Struktur (pondasi/balok) & rencana elektrikal harus komposit dengan denah.
Tandai tidak yakin sebagai confidence "rendah" — jangan mengarang.`

// loadGKSkill reads the checklist markdown (hot-editable) with a fallback.
func (s *Service) loadGKSkill() string {
	path := s.gk.SkillPath
	if strings.TrimSpace(path) == "" {
		return gkFallbackSkill
	}
	body, err := os.ReadFile(path)
	if err != nil || len(body) == 0 {
		return gkFallbackSkill
	}
	text := string(body)
	const maxSkillChars = 6000
	if len(text) > maxSkillChars {
		text = text[:maxSkillChars] + " …(dipotong)"
	}
	return text
}

// GKSkillContent returns the FULL checklist markdown (untruncated) for editing
// in the UI — falls back to the built-in default if no file is present.
func (s *Service) GKSkillContent() (string, bool) {
	path := strings.TrimSpace(s.gk.SkillPath)
	if path != "" {
		if body, err := os.ReadFile(path); err == nil && len(body) > 0 {
			return string(body), true // fromFile = true
		}
	}
	return gkFallbackSkill, false
}

// SaveGKSkill overwrites the checklist markdown file. The next Deep Revisi run
// reads it fresh (hot-editable), so edits take effect immediately.
func (s *Service) SaveGKSkill(content string) error {
	path := strings.TrimSpace(s.gk.SkillPath)
	if path == "" {
		return fmt.Errorf("%w: lokasi file skill tidak dikonfigurasi (GK_SKILL_PATH)", ErrValidation)
	}
	if strings.TrimSpace(content) == "" {
		return fmt.Errorf("%w: skill tidak boleh kosong", ErrValidation)
	}
	if len(content) > 200000 {
		return fmt.Errorf("%w: skill terlalu panjang (maks 200 KB)", ErrValidation)
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

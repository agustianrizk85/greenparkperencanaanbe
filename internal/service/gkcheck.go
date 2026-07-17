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
	OllamaAPIKey   string
	OllamaModel    string
	OllamaEndpoint string
	PythonBin      string
	ScriptsDir     string
	SkillPath      string
}

// Configured reports whether Deep Revisi AI has enough config to run.
func (c GKConfig) Configured() bool {
	return strings.TrimSpace(c.OllamaAPIKey) != "" && strings.TrimSpace(c.OllamaModel) != ""
}

// maxGKDocBytes caps an uploaded GK PDF (20 MiB — real CAD-exported Gambar
// Kerja sets run larger than typical review docs; sample seen: 11.5 MB).
const maxGKDocBytes = 20 << 20

/* ---- Upload ------------------------------------------------------------- */

// UploadGKDoc stores a GK Kontraktor or GK TTD PDF against a work drawing.
// kind must be "kontraktor" or "ttd".
func (s *Service) UploadGKDoc(actor domain.User, wdID, kind, filename string, data []byte) (WorkDrawingView, error) {
	if kind != "kontraktor" && kind != "ttd" {
		return WorkDrawingView{}, ErrValidation
	}
	if len(data) == 0 || len(data) > maxGKDocBytes || !strings.HasSuffix(strings.ToLower(filename), ".pdf") {
		return WorkDrawingView{}, ErrValidation
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

// StartDeepRevisi kicks off an async GK Kontraktor vs GK TTD check. Returns
// immediately; poll GKStatus for progress. Both documents must already be
// uploaded.
func (s *Service) StartDeepRevisi(wdID string) error {
	if !s.gk.Configured() {
		return fmt.Errorf("%w: Ollama belum dikonfigurasi (OLLAMA_API_KEY)", ErrValidation)
	}
	updated, ok := s.repo.MutateWorkDrawing(wdID, func(d *domain.WorkDrawing) {})
	if !ok {
		return ErrNotFound
	}
	if updated.GKKontraktor == nil || updated.GKTTD == nil {
		return fmt.Errorf("%w: upload GK Kontraktor & GK TTD dulu", ErrValidation)
	}
	if updated.GKStatus == domain.GKRunning {
		return nil // already running, treat as idempotent
	}
	s.repo.MutateWorkDrawing(wdID, func(d *domain.WorkDrawing) {
		d.GKStatus = domain.GKRunning
		d.GKError = ""
		d.GKFindings = nil
	})
	go s.runGKCheck(wdID)
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

/* ---- Pipeline ------------------------------------------------------------- */

func (s *Service) runGKCheck(wdID string) {
	kontraktorBytes, kontraktorName, ok := s.repo.WorkDrawingDocBytes(wdID, "kontraktor")
	if !ok {
		s.failGK(wdID, "GK Kontraktor tidak ditemukan")
		return
	}
	ttdBytes, _, ok := s.repo.WorkDrawingDocBytes(wdID, "ttd")
	if !ok {
		s.failGK(wdID, "GK TTD tidak ditemukan")
		return
	}

	tmpDir, err := os.MkdirTemp("", "gkcheck-*")
	if err != nil {
		s.failGK(wdID, "gagal buat folder sementara: "+err.Error())
		return
	}
	defer os.RemoveAll(tmpDir)

	kontraktorPath := filepath.Join(tmpDir, "kontraktor.pdf")
	ttdPath := filepath.Join(tmpDir, "ttd.pdf")
	if err := os.WriteFile(kontraktorPath, kontraktorBytes, 0o644); err != nil {
		s.failGK(wdID, "gagal tulis GK Kontraktor: "+err.Error())
		return
	}
	if err := os.WriteFile(ttdPath, ttdBytes, 0o644); err != nil {
		s.failGK(wdID, "gagal tulis GK TTD: "+err.Error())
		return
	}

	manifest, err := s.renderPages(kontraktorPath, ttdPath, tmpDir)
	if err != nil {
		s.failGK(wdID, "gagal render PDF ke gambar: "+err.Error())
		return
	}

	skill := s.loadGKSkill()
	pageCount := len(manifest.Kontraktor.Images)
	if len(manifest.TTD.Images) < pageCount {
		pageCount = len(manifest.TTD.Images)
	}

	var findings []domain.GKFinding
	for i := 0; i < pageCount; i++ {
		pf, err := s.visionCompare(skill, manifest.Kontraktor.Images[i], manifest.TTD.Images[i])
		if err != nil {
			// One page failing shouldn't abort the other 20+ — log and move on.
			findings = append(findings, domain.GKFinding{
				Page: i + 1, Wrong: "", Correct: "",
				Explain:    "Gagal analisis AI halaman ini: " + err.Error(),
				Confidence: "rendah",
			})
			continue
		}
		for _, f := range pf {
			f.Page = i + 1
			findings = append(findings, f)
		}
	}

	outPath := filepath.Join(tmpDir, "annotated.pdf")
	if err := s.annotate(kontraktorPath, findings, outPath); err != nil {
		s.failGK(wdID, "gagal buat PDF beranotasi: "+err.Error())
		return
	}
	annotatedBytes, err := os.ReadFile(outPath)
	if err != nil {
		s.failGK(wdID, "gagal baca PDF beranotasi: "+err.Error())
		return
	}

	doc := domain.GKDoc{
		Name: "notes_" + kontraktorName, Size: len(annotatedBytes),
		UploadedBy: "deep-revisi-ai", UploadedAt: s.now().Format(time.RFC3339),
	}
	s.repo.SetWorkDrawingDoc(wdID, "annotated", doc, annotatedBytes)
	s.repo.MutateWorkDrawing(wdID, func(d *domain.WorkDrawing) {
		d.GKStatus = domain.GKDone
		d.GKFindings = findings
		d.GKCheckedAt = s.now().Format(time.RFC3339)
	})
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

/* ---- Ollama vision call ---------------------------------------------------- */

type ollamaContentPart struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	ImageURL *ollamaImageURL `json:"image_url,omitempty"`
}

type ollamaImageURL struct {
	URL string `json:"url"`
}

type ollamaMessage struct {
	Role    string              `json:"role"`
	Content []ollamaContentPart `json:"content"`
}

type ollamaChatRequest struct {
	Model       string          `json:"model"`
	Messages    []ollamaMessage `json:"messages"`
	MaxTokens   int             `json:"max_tokens"`
	Temperature float64         `json:"temperature"`
	Stream      bool            `json:"stream"`
}

type ollamaChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error json.RawMessage `json:"error"`
}

// visionCompare sends one page pair (Kontraktor vs TTD) to the vision model
// and returns the findings it reports for that page.
func (s *Service) visionCompare(skill, kontraktorImgPath, ttdImgPath string) ([]domain.GKFinding, error) {
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

	reqBody := ollamaChatRequest{
		Model: s.gk.OllamaModel,
		Messages: []ollamaMessage{{
			Role: "user",
			Content: []ollamaContentPart{
				{Type: "text", Text: prompt},
				{Type: "image_url", ImageURL: &ollamaImageURL{URL: tImg}},
				{Type: "image_url", ImageURL: &ollamaImageURL{URL: kImg}},
			},
		}},
		MaxTokens:   1500,
		Temperature: 0.3,
		Stream:      false,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, s.gk.OllamaEndpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+s.gk.OllamaAPIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Title", "Greenpark Perencanaan — Deep Revisi AI")

	client := &http.Client{Timeout: 100 * time.Second}
	res, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	var parsed ollamaChatResponse
	if err := json.NewDecoder(res.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("baca respons: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Ollama %d: %s", res.StatusCode, string(parsed.Error))
	}
	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("respons kosong")
	}
	return parseGKFindings(parsed.Choices[0].Message.Content), nil
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

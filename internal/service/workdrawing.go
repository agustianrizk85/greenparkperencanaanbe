package service

import (
	"fmt"
	"sort"
	"strings"

	"greenpark/perencanaan/internal/domain"
)

// SLA windows, in working days, from the business process.
const (
	konsumenSLAWorkdays   = 15 // gambar kerja konsumen, after info masuk
	kontraktorSLAWorkdays = 5  // gambar kerja kontraktor, after TTD konsumen
)

// WorkDrawingView enriches a stored flow with derived SLA countdowns and a
// traffic-light severity for the currently active leg.
type WorkDrawingView struct {
	domain.WorkDrawing
	ProjectName        string `json:"projectName"`
	KonsumenDaysLeft   int    `json:"konsumenDaysLeft"`   // working days until konsumen due (negative = overdue)
	KontraktorDaysLeft int    `json:"kontraktorDaysLeft"` // working days until kontraktor due
	ActiveLeg          string `json:"activeLeg"`          // "konsumen" | "kontraktor" | ""
	Sev                Rag    `json:"sev"`
}

// CreateWorkDrawingInput registers a new consumer working-drawing flow.
type CreateWorkDrawingInput struct {
	ProjectID   string                 `json:"projectId"`
	Konsumen    string                 `json:"konsumen"`
	Unit        string                 `json:"unit"`
	PIC         string                 `json:"pic"`
	InfoMasuk   string                 `json:"infoMasuk"`             // YYYY-MM-DD; defaults to today
	Attachments []domain.WDAttachment  `json:"attachments,omitempty"` // optional linked files (e.g. cicle)
}

// WorkDrawings returns all flows enriched with SLA countdowns.
func (s *Service) WorkDrawings() []WorkDrawingView {
	projects := s.repo.Projects()
	today := s.today()
	raw := s.repo.WorkDrawings()
	out := make([]WorkDrawingView, len(raw))
	for i, d := range raw {
		out[i] = s.viewWorkDrawing(d, projectName(projects, d.ProjectID), today)
	}
	return out
}

// CreateWorkDrawing starts a consumer flow: the 15-working-day SLA begins at
// info masuk. The contractor SLA is only computed once the consumer signs (TTD).
func (s *Service) CreateWorkDrawing(in CreateWorkDrawingInput) (WorkDrawingView, error) {
	if strings.TrimSpace(in.ProjectID) == "" || strings.TrimSpace(in.Konsumen) == "" {
		return WorkDrawingView{}, ErrValidation
	}
	if _, ok := s.repo.Project(in.ProjectID); !ok {
		return WorkDrawingView{}, ErrNotFound
	}
	info := strings.TrimSpace(in.InfoMasuk)
	if info == "" {
		info = s.today()
	}
	// PIC opsional (nullable): biarkan kosong bila tidak diisi, terima nama custom.
	pic := strings.TrimSpace(in.PIC)
	d := domain.WorkDrawing{
		ProjectID:   in.ProjectID,
		Konsumen:    strings.TrimSpace(in.Konsumen),
		Unit:        strings.TrimSpace(in.Unit),
		PIC:         pic,
		InfoMasuk:   info,
		KonsumenDue: domain.AddWorkingDays(info, konsumenSLAWorkdays),
		Status:      domain.WDKonsumen,
		Attachments: in.Attachments,
	}
	saved := s.repo.AddWorkDrawing(d)
	projects := s.repo.Projects()
	return s.viewWorkDrawing(saved, projectName(projects, saved.ProjectID), s.today()), nil
}

// AdvanceWorkDrawingInput drives the flow forward one step.
type AdvanceWorkDrawingInput struct {
	Action string `json:"action"` // "konsumen-selesai" | "ttd-konsumen" | "kontraktor-selesai"
	Date   string `json:"date"`   // optional; defaults to today
}

// AdvanceWorkDrawing applies a state transition to a flow.
func (s *Service) AdvanceWorkDrawing(id string, in AdvanceWorkDrawingInput) (WorkDrawingView, error) {
	date := strings.TrimSpace(in.Date)
	if date == "" {
		date = s.today()
	}
	var transitionErr error
	updated, ok := s.repo.MutateWorkDrawing(id, func(d *domain.WorkDrawing) {
		switch in.Action {
		case "konsumen-selesai":
			d.KonsumenDone = date
			d.Status = domain.WDTTD
		case "ttd-konsumen":
			if d.KonsumenDone == "" {
				d.KonsumenDone = date
			}
			d.TTDKonsumen = date
			d.KontraktorDue = domain.AddWorkingDays(date, kontraktorSLAWorkdays)
			d.Status = domain.WDKontraktor
		case "kontraktor-selesai":
			d.KontraktorDone = date
			d.Status = domain.WDDone
		default:
			transitionErr = ErrValidation
		}
	})
	if !ok {
		return WorkDrawingView{}, ErrNotFound
	}
	if transitionErr != nil {
		return WorkDrawingView{}, transitionErr
	}
	projects := s.repo.Projects()
	return s.viewWorkDrawing(updated, projectName(projects, updated.ProjectID), s.today()), nil
}

// viewWorkDrawing computes the SLA countdowns and severity for the active leg.
func (s *Service) viewWorkDrawing(d domain.WorkDrawing, pname, today string) WorkDrawingView {
	v := WorkDrawingView{WorkDrawing: d, ProjectName: pname}
	switch d.Status {
	case domain.WDKonsumen:
		v.ActiveLeg = "konsumen"
		v.KonsumenDaysLeft = domain.WorkingDaysBetween(today, d.KonsumenDue)
		v.Sev = slaSeverity(v.KonsumenDaysLeft)
	case domain.WDKontraktor:
		v.ActiveLeg = "kontraktor"
		v.KontraktorDaysLeft = domain.WorkingDaysBetween(today, d.KontraktorDue)
		v.Sev = slaSeverity(v.KontraktorDaysLeft)
	case domain.WDDone:
		v.Sev = RagGreen
	default:
		v.Sev = RagGrey
	}
	return v
}

// slaSeverity maps working days remaining to a traffic light.
func slaSeverity(daysLeft int) Rag {
	switch {
	case daysLeft < 0:
		return RagRed
	case daysLeft <= 3:
		return RagAmber
	default:
		return RagGreen
	}
}

/* ---- Alerts ------------------------------------------------------------ */

// AlertItem is one SLA alert for the active leg of a flow.
type AlertItem struct {
	ID          string `json:"id"`
	Sev         Rag    `json:"sev"`
	Leg         string `json:"leg"` // "konsumen" | "kontraktor"
	ProjectName string `json:"projectName"`
	Konsumen    string `json:"konsumen"`
	Unit        string `json:"unit"`
	PIC         string `json:"pic"`
	Due         string `json:"due"`
	DaysLeft    int    `json:"daysLeft"`
	Message     string `json:"message"`
}

// Alerts returns SLA alerts for every flow with an active (not done) leg,
// ordered most-urgent first.
func (s *Service) Alerts() []AlertItem {
	out := []AlertItem{}
	for _, v := range s.WorkDrawings() {
		if v.ActiveLeg == "" {
			continue
		}
		var due string
		var daysLeft int
		var window int
		if v.ActiveLeg == "konsumen" {
			due, daysLeft, window = v.KonsumenDue, v.KonsumenDaysLeft, konsumenSLAWorkdays
		} else {
			due, daysLeft, window = v.KontraktorDue, v.KontraktorDaysLeft, kontraktorSLAWorkdays
		}
		out = append(out, AlertItem{
			ID: v.ID, Sev: v.Sev, Leg: v.ActiveLeg, ProjectName: v.ProjectName,
			Konsumen: v.Konsumen, Unit: v.Unit, PIC: v.PIC, Due: due, DaysLeft: daysLeft,
			Message: alertMessage(v.ActiveLeg, window, daysLeft),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return sevRank(out[i].Sev) < sevRank(out[j].Sev)
	})
	return out
}

func alertMessage(leg string, window, daysLeft int) string {
	what := "Gambar kerja konsumen"
	if leg == "kontraktor" {
		what = "Gambar kerja kontraktor"
	}
	switch {
	case daysLeft < 0:
		return fmt.Sprintf("%s TERLAMBAT %d hari kerja (SLA %d hk).", what, -daysLeft, window)
	case daysLeft == 0:
		return fmt.Sprintf("%s jatuh tempo hari ini (SLA %d hk).", what, window)
	default:
		return fmt.Sprintf("%s tersisa %d hari kerja (SLA %d hk).", what, daysLeft, window)
	}
}

func sevRank(r Rag) int {
	switch r {
	case RagRed:
		return 0
	case RagAmber:
		return 1
	case RagGreen:
		return 2
	default:
		return 3
	}
}

/* ---- AI revision (revisi gambar kerja konsumen) ------------------------ */

// ReviseWorkDrawing produces an AI-assisted revision analysis for a consumer
// working drawing and stores it on the flow. Per the business process the
// revision is only opened after the finance and legal dashboards have cleared
// the unit; that gate is enforced by the caller / upstream module.
//
// NOTE: this is a deterministic, rules-based placeholder standing in for the
// real AI integration (no model API key is configured in this environment).
func (s *Service) ReviseWorkDrawing(id, instruction string) (WorkDrawingView, error) {
	note := buildRevisiNote(instruction)
	updated, ok := s.repo.MutateWorkDrawing(id, func(d *domain.WorkDrawing) {
		d.RevisiNote = note
	})
	if !ok {
		return WorkDrawingView{}, ErrNotFound
	}
	projects := s.repo.Projects()
	return s.viewWorkDrawing(updated, projectName(projects, updated.ProjectID), s.today()), nil
}

func buildRevisiNote(instruction string) string {
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		instruction = "Penyesuaian gambar kerja sesuai permintaan konsumen."
	}
	var b strings.Builder
	b.WriteString("[AI Revisi — pratinjau] Permintaan: ")
	b.WriteString(instruction)
	b.WriteString("\n\nRencana revisi:\n")
	b.WriteString("1. Tinjau dampak terhadap dimensi & spesifikasi unit.\n")
	b.WriteString("2. Sesuaikan denah/tampak terkait dan perbarui gambar kerja.\n")
	b.WriteString("3. Cek konsistensi dengan persetujuan finance & legal.\n")
	b.WriteString("4. Terbitkan revisi untuk TTD ulang konsumen bila perlu.\n")
	b.WriteString("\n(Catatan: hasil ini placeholder integrasi AI — sambungkan ke model produksi untuk analisis penuh.)")
	return b.String()
}

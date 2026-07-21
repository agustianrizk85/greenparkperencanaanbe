package service

import (
	"fmt"
	"time"

	"greenpark/perencanaan/internal/domain"
)

// ResetProses clears only PROCESS data (task status + working-drawing flows),
// preserving all MASTER data including manually added projects. Only CEO /
// Kadep may reset. Backs the "Reset Proses" action.
func (s *Service) ResetProses(role string) error {
	if !canManage(role) {
		return ErrForbidden
	}
	s.repo.ResetProses()
	return nil
}

// ResetMaster rebuilds the MASTER portfolio back to the seeded 32 projects
// (dropping any added project) and clears process data too. Only CEO / Kadep
// may reset. Backs the "Reset Master" action.
func (s *Service) ResetMaster(role string) error {
	if !canManage(role) {
		return ErrForbidden
	}
	s.repo.ResetMaster()
	return nil
}

// EmptyAll wipes EVERYTHING to a blank slate: all projects/tasks + every master
// (GP/tipe/lebar/lokasi/blok/kavling) + drawings/docs, with NO re-seed. Also
// removes task attachment files from disk. Roster + shared board are untouched.
// CEO / Kadep only.
func (s *Service) EmptyAll(role string) error {
	if !canManage(role) {
		return ErrForbidden
	}
	var files []string
	for _, p := range s.repo.Projects() {
		for _, t := range p.Tasks {
			for _, a := range t.Attachments {
				files = append(files, a.ID)
			}
		}
	}
	s.repo.EmptyAll()
	s.removeBoardFiles(files)
	return nil
}

// SeedDemo replaces all dynamic data with a realistic sample set so the
// dashboard is populated for demos and first runs: varied task progress across
// the portfolio plus a handful of working-drawing flows spanning every SLA
// state (on-track, near-due, overdue, signed, done). Only CEO / Kadep may seed.
func (s *Service) SeedDemo(role string) error {
	if !canManage(role) {
		return ErrForbidden
	}
	s.seedDemo()
	return nil
}

// SeedDemoSystem runs the demo seed without a permission check. Intended for
// startup wiring (see cmd/server) so a freshly started server already shows
// data; it is not exposed over HTTP.
func (s *Service) SeedDemoSystem() { s.seedDemo() }

// statusCycle is the deterministic progression used to spread task statuses so
// the seeded portfolio shows a believable mix rather than all-or-nothing.
var statusCycle = []domain.TaskStatus{
	domain.StatusDone, domain.StatusDone, domain.StatusReview,
	domain.StatusProgress, domain.StatusProgress, domain.StatusTodo,
}

func (s *Service) seedDemo() {
	// 1. Clean slate, then rebuild the deterministic 32-project portfolio so every
	//    reference seeded below resolves against real ids (gp-001..gp-0NN).
	s.repo.EmptyAll()
	s.repo.SeedProjects()

	// 2. Reset the shared Papan Tugas to just the 4 fixed system columns with no
	//    leftover sample cards, so repeated "Isi Contoh" runs replace rather than
	//    pile up.
	s.repo.EnsureBoardSystemLists()
	s.repo.ClearBoardCards()

	at := s.now().Format(time.RFC3339)

	// 3. Seed the CONNECTED masters (GP/lokasi/tipe/lebar) + per-project
	//    bloks/kavling so Master Produk is populated and every project shows real,
	//    linked units instead of "0 unit"; then fill the board with sample cards.
	s.seedMasters()
	s.seedBoardSample(at)

	// Spread task statuses deterministically. Lower-numbered (older) projects
	// are further along; later tasks in each tree lag behind earlier ones.
	for _, p := range s.repo.Projects() {
		for i, t := range p.Tasks {
			idx := (p.No + i) % len(statusCycle)
			// Newer projects (higher No) start further back in the cycle.
			if p.No%4 == 0 {
				idx = (idx + 3) % len(statusCycle)
			}
			status := statusCycle[idx]
			if status != domain.StatusTodo {
				s.repo.UpdateTaskStatus(p.ID, t.ID, status, at)
			}
		}
	}

	// A spread of consumer working-drawing flows across every SLA state.
	// dates are calendar-days before today; the service computes the working-day
	// SLA gates from them.
	s.seedDrawingOverdue("gp-001", "Bpk. Andi Pratama", "A-12", domain.PicAgus, 24)
	s.seedDrawingActive("gp-002", "Ibu Sari Wahyuni", "B-03", domain.PicAgus, 4)
	s.seedDrawingActive("gp-004", "Bpk. Joko Santoso", "C-07", domain.PicAgus, 16)
	s.seedDrawingKontraktor("gp-005", "Ibu Maya Lestari", "D-01", domain.PicAgus, 22, 3)
	s.seedDrawingKontraktorOverdue("gp-007", "Bpk. Rudi Hartono", "E-09", domain.PicAgus, 30, 9)
	s.seedDrawingDone("gp-009", "Ibu Dewi Anggraini", "F-04", domain.PicAgus, 40, 18)
}

// demoGPNames maps the seeded GP codes to a friendly brand name so Master Produk
// shows readable groups instead of bare codes.
var demoGPNames = map[string]string{
	"GP1": "Grup Le Hauz Signature",
	"GP2": "Grup The Hauz",
	"GP3": "Grup Z Hauz",
	"GP4": "Grup Le Hauz Premiere",
}

// demoBuildingTypes is the reusable house-type master the sample kavling are
// built to (LuasBangunan / LuasTanah).
var demoBuildingTypes = []domain.BuildingType{
	{Name: "Garnet", LuasBangunan: 42, LuasTanah: 32},
	{Name: "Ruby", LuasBangunan: 42, LuasTanah: 33},
	{Name: "Zamrud", LuasBangunan: 36, LuasTanah: 60},
	{Name: "Safir", LuasBangunan: 45, LuasTanah: 72},
}

// demoLebars is the controlled kavling-frontage vocabulary for the sample.
var demoLebars = []string{"L3.5", "L4", "L5", "L6"}

// seedMasters derives the shared masters (GP + lokasi) from the freshly re-seeded
// projects so every project reference resolves, seeds a realistic tipe-bangunan +
// lebar vocabulary, then gives each project real bloks + kavling so its unit count
// is backed by linked data. Uses only PUBLIC repo methods (each takes its own
// lock) — safe to orchestrate from the service level.
func (s *Service) seedMasters() {
	projects := s.repo.Projects()

	// GP + Lokasi masters, distinct in first-seen order, derived from the portfolio.
	seenGP := map[string]bool{}
	seenLok := map[string]bool{}
	for _, p := range projects {
		if p.GP != "" && !seenGP[p.GP] {
			seenGP[p.GP] = true
			name := demoGPNames[p.GP]
			if name == "" {
				name = "Grup " + p.GP
			}
			s.repo.SaveGP(domain.GP{Code: p.GP, Name: name})
		}
		if p.Lokasi != "" && !seenLok[p.Lokasi] {
			seenLok[p.Lokasi] = true
			s.repo.SaveLokasi(domain.Lokasi{Name: p.Lokasi})
		}
	}

	// Tipe bangunan + lebar masters. Keep the stored building-type copies (with
	// generated ids) so kavling can link against a real TypeID.
	types := make([]domain.BuildingType, 0, len(demoBuildingTypes))
	for _, t := range demoBuildingTypes {
		types = append(types, s.repo.SaveBuildingType(t))
	}
	for _, name := range demoLebars {
		s.repo.SaveLebar(domain.Lebar{Name: name})
	}

	// Per-project bloks + kavling. Rotate type/lebar/blok deterministically so the
	// data is varied yet reproducible; every TypeID/BlokID is a real seeded id and
	// LebarKavling is a real lebar NAME.
	for _, p := range projects {
		blokNames := []string{"A"}
		if p.Units > 3 {
			blokNames = []string{"A", "B"}
		}
		bloks := make([]domain.Blok, 0, len(blokNames))
		for _, bn := range blokNames {
			bloks = append(bloks, s.repo.SaveBlok(domain.Blok{ProjectID: p.ID, Name: bn}))
		}
		n := p.Units
		if n > 6 {
			n = 6
		}
		if n < 3 {
			n = 3
		}
		for i := 0; i < n; i++ {
			blok := bloks[i%len(bloks)]
			t := types[i%len(types)]
			s.repo.SaveKavling(domain.Kavling{
				ProjectID:    p.ID,
				BlokID:       blok.ID,
				NoKav:        fmt.Sprintf("%s%d", blok.Name, i+1),
				TypeID:       t.ID,
				LuasBangunan: t.LuasBangunan,
				LuasKavling:  t.LuasTanah + 6 + (i%3)*6,
				LebarKavling: demoLebars[i%len(demoLebars)],
			})
		}
	}
}

// demoCard is one sample Papan Tugas card for the seed.
type demoCard struct {
	list    string   // system list id (domain.SysList*)
	title   string   // Indonesian title
	desc    string   // Catatan
	members []string // seeded usernames
	label   string   // demo label name to tag ("" = none)
}

// seedBoardSample fills the 4 system columns with a realistic spread of sample
// cards (Indonesian titles, seeded PIC members, a couple of labels) so the Papan
// Tugas board is populated after "Isi Contoh".
func (s *Service) seedBoardSample(at string) {
	// Idempotent labels: reuse an existing definition by name, else create it, so
	// repeated seeds don't accumulate duplicate labels (ClearBoardCards keeps labels).
	ensureLabel := func(name, color string) string {
		for _, lb := range s.repo.BoardLabels() {
			if lb.Name == name {
				return lb.ID
			}
		}
		return s.repo.AddBoardLabel(name, color).ID
	}
	labelIDs := map[string]string{
		"Prioritas":         ensureLabel("Prioritas", "#e11d48"),
		"Menunggu Approval": ensureLabel("Menunggu Approval", "#f59e0b"),
	}

	cards := []demoCard{
		{domain.SysListTodo, "Review siteplan GP4 Le Hauz Premiere", "Cek zonasi & GSB sebelum diajukan ke pemda.", []string{domain.PicRandi}, "Prioritas"},
		{domain.SysListTodo, "Siapkan data proyek Z Hauz Limo 2", "Lengkapi luas, jumlah unit, dan tipe.", []string{domain.PicAnanto}, ""},
		{domain.SysListProgress, "Koordinasi kontraktor blok A", "Sinkron jadwal gambar kerja dengan kontraktor.", []string{domain.PicAgus}, ""},
		{domain.SysListProgress, "Gambar kerja Ruby unit B-03", "Finalisasi denah & tampak tipe Ruby.", []string{domain.PicRio}, ""},
		{domain.SysListReview, "Approval gambar kerja Ruby", "Menunggu review kepala departemen.", []string{"kadep"}, "Menunggu Approval"},
		{domain.SysListReview, "Review desain kawasan The Hauz Premiere", "Cek konsistensi masterplan & fasum.", []string{domain.PicAnanto}, ""},
		{domain.SysListDone, "Update jadwal deliverable Q3", "Timeline deliverable sudah diperbarui.", []string{"kadep"}, ""},
		{domain.SysListDone, "Serah terima siteplan pemda GP1", "Dokumen siteplan pemda sudah diserahkan.", nil, ""},
	}

	for _, c := range cards {
		card := domain.BoardCard{
			Title:     c.title,
			Desc:      c.desc,
			Members:   c.members,
			CreatedBy: "kadep",
			CreatedAt: at,
			Division:  "perencanaan",
		}
		if id := labelIDs[c.label]; c.label != "" && id != "" {
			card.Labels = []string{id}
		}
		s.repo.AddBoardCard(c.list, card)
	}
}

// daysAgo returns the date n calendar days before "today" as YYYY-MM-DD.
func (s *Service) daysAgo(n int) string {
	return s.now().AddDate(0, 0, -n).Format(domain.DateLayout)
}

// seedDrawingActive creates a flow still in the consumer leg (info masuk only).
func (s *Service) seedDrawingActive(projectID, konsumen, unit, pic string, infoDaysAgo int) {
	_, _ = s.CreateWorkDrawing(CreateWorkDrawingInput{
		ProjectID: projectID, Konsumen: konsumen, Unit: unit, PIC: pic,
		InfoMasuk: s.daysAgo(infoDaysAgo),
	})
}

// seedDrawingOverdue is an active consumer leg whose 15-hk SLA has lapsed.
func (s *Service) seedDrawingOverdue(projectID, konsumen, unit, pic string, infoDaysAgo int) {
	s.seedDrawingActive(projectID, konsumen, unit, pic, infoDaysAgo)
}

// seedDrawingKontraktor creates a flow advanced past consumer TTD into the
// contractor leg (5-hk SLA running), signed ttdDaysAgo days ago.
func (s *Service) seedDrawingKontraktor(projectID, konsumen, unit, pic string, infoDaysAgo, ttdDaysAgo int) {
	v, err := s.CreateWorkDrawing(CreateWorkDrawingInput{
		ProjectID: projectID, Konsumen: konsumen, Unit: unit, PIC: pic,
		InfoMasuk: s.daysAgo(infoDaysAgo),
	})
	if err != nil {
		return
	}
	_, _ = s.AdvanceWorkDrawing(v.ID, AdvanceWorkDrawingInput{
		Action: "ttd-konsumen", Date: s.daysAgo(ttdDaysAgo),
	})
}

// seedDrawingKontraktorOverdue is a contractor leg whose 5-hk SLA has lapsed.
func (s *Service) seedDrawingKontraktorOverdue(projectID, konsumen, unit, pic string, infoDaysAgo, ttdDaysAgo int) {
	s.seedDrawingKontraktor(projectID, konsumen, unit, pic, infoDaysAgo, ttdDaysAgo)
}

// seedDrawingDone creates a fully delivered flow (contractor drawing complete).
func (s *Service) seedDrawingDone(projectID, konsumen, unit, pic string, infoDaysAgo, ttdDaysAgo int) {
	v, err := s.CreateWorkDrawing(CreateWorkDrawingInput{
		ProjectID: projectID, Konsumen: konsumen, Unit: unit, PIC: pic,
		InfoMasuk: s.daysAgo(infoDaysAgo),
	})
	if err != nil {
		return
	}
	if _, err = s.AdvanceWorkDrawing(v.ID, AdvanceWorkDrawingInput{
		Action: "ttd-konsumen", Date: s.daysAgo(ttdDaysAgo),
	}); err != nil {
		return
	}
	_, _ = s.AdvanceWorkDrawing(v.ID, AdvanceWorkDrawingInput{
		Action: "kontraktor-selesai", Date: s.daysAgo(ttdDaysAgo - 4),
	})
}

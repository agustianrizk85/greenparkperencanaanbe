package domain

// taskSpec is the static blueprint of one deliverable, shared by every project.
type taskSpec struct {
	id       string
	category string
	group    string
	name     string
	pic      string // author username
	output   Division
	weighted bool
}

// PIC usernames of the three design authors.
const (
	PicRandi  = "randi"
	PicAnanto = "ananto"
	PicAgus   = "agus"
)

// deliverableTemplate is the canonical "flow menambah proyek" tree. Every new
// project is instantiated from this list, so the structure and the PIC / output
// routing stay identical across the portfolio. Source of truth for the
// department's business process.
var deliverableTemplate = []taskSpec{
	// ---- Site Plan -----------------------------------------------------
	{"site-teknis", "Site Plan", "Site Plan", "Siteplan teknis", PicRandi, DivTeknik, false},
	{"site-data", "Site Plan", "Site Plan", "Data proyek", PicRandi, DivNone, false},
	{"site-pemda", "Site Plan", "Site Plan", "Siteplan pemda", PicRandi, DivLegal, false},
	{"site-splitzing", "Site Plan", "Site Plan", "Siteplan splitzing", PicRandi, DivLegal, false},
	{"site-marketing", "Site Plan", "Site Plan", "Siteplan marketing", PicRandi, DivMarketing, false},

	// ---- Desain Unit Hunian -------------------------------------------
	// Denah (100%)
	{"unit-denah-desain", "Desain Unit Hunian", "Denah", "Desain denah", PicRandi, DivNone, true},
	{"unit-denah-warna", "Desain Unit Hunian", "Denah", "Denah warna", PicRandi, DivMarketing, true},
	// Tampak (100%)
	{"unit-tampak-desain", "Desain Unit Hunian", "Tampak", "Desain tampak", PicRandi, DivNone, true},
	{"unit-tampak-fasad", "Desain Unit Hunian", "Tampak", "Render fasad", PicRandi, DivMarketing, true},
	{"unit-tampak-perspektif", "Desain Unit Hunian", "Tampak", "Render perspektif", PicRandi, DivMarketing, true},
	// Interior (100%)
	{"unit-interior-desain", "Desain Unit Hunian", "Interior", "Desain interior", PicAnanto, DivNone, true},
	{"unit-interior-render", "Desain Unit Hunian", "Interior", "Render interior", PicAnanto, DivMarketing, true},
	{"unit-interior-animasi", "Desain Unit Hunian", "Interior", "Animasi interior", PicAnanto, DivMarketing, true},
	// Detail Unit Hunian
	{"unit-detail-spesifikasi", "Desain Unit Hunian", "Detail Unit Hunian", "Spesifikasi unit", PicRandi, DivTeknik, false},
	{"unit-detail-gk", "Desain Unit Hunian", "Detail Unit Hunian", "Gambar kerja standar per tipe", PicAgus, DivTeknik, false},
	{"unit-detail-imb", "Desain Unit Hunian", "Detail Unit Hunian", "Gambar IMB / PBG", PicAgus, DivLegal, false},

	// ---- Desain Kawasan ------------------------------------------------
	// Maingate (100%)
	{"kawasan-maingate-desain", "Desain Kawasan", "Maingate", "Desain maingate", PicRandi, DivNone, true},
	{"kawasan-maingate-render", "Desain Kawasan", "Maingate", "Render maingate", PicRandi, DivMarketing, true},
	{"kawasan-maingate-gk", "Desain Kawasan", "Maingate", "Gambar kerja maingate", PicAgus, DivTeknik, true},
	// Fasos Fasum (100%)
	{"kawasan-fasos-desain", "Desain Kawasan", "Fasos Fasum", "Desain fasos fasum", PicAnanto, DivNone, true},
	{"kawasan-fasos-render", "Desain Kawasan", "Fasos Fasum", "Render fasos fasum", PicAnanto, DivMarketing, true},
	{"kawasan-fasos-gk", "Desain Kawasan", "Fasos Fasum", "Gambar kerja fasos fasum", PicAgus, DivTeknik, true},
	// Desain Infrastruktur (100%)
	{"kawasan-infra-rencana", "Desain Kawasan", "Desain Infrastruktur", "Rencana infrastruktur", PicRandi, DivTeknik, true},
	{"kawasan-infra-gk", "Desain Kawasan", "Desain Infrastruktur", "Gambar kerja infrastruktur", PicAgus, DivTeknik, true},
	// Rencana Leveling
	{"kawasan-leveling-rencana", "Desain Kawasan", "Rencana Leveling", "Rencana leveling", PicRandi, DivTeknik, false},
	{"kawasan-leveling-gk", "Desain Kawasan", "Rencana Leveling", "Gambar kerja leveling", PicAgus, DivTeknik, false},
	// Desain Landscape
	{"kawasan-landscape-taman", "Desain Kawasan", "Desain Landscape", "Desain taman", PicRandi, DivNone, false},
	{"kawasan-landscape-gk", "Desain Kawasan", "Desain Landscape", "Gambar kerja taman", PicRandi, DivTeknik, false},
	// Animasi (100%)
	{"kawasan-animasi", "Desain Kawasan", "Animasi", "Animasi kawasan", PicAnanto, DivMarketing, true},
}

// NewTaskTree instantiates the deliverable template as fresh Tasks (all todo)
// for a newly added project.
func NewTaskTree() []Task {
	tasks := make([]Task, len(deliverableTemplate))
	for i, s := range deliverableTemplate {
		tasks[i] = Task{
			ID:       s.id,
			Category: s.category,
			Group:    s.group,
			Name:     s.name,
			PIC:      s.pic,
			Output:   s.output,
			Weighted: s.weighted,
			Status:   StatusTodo,
		}
	}
	return tasks
}

// CategoryOrder is the canonical display order of the top-level categories.
var CategoryOrder = []string{"Site Plan", "Desain Unit Hunian", "Desain Kawasan"}

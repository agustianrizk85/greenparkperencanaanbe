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

// PIC usernames of the design authors.
const (
	PicRandi  = "randi"
	PicAnanto = "ananto"
	PicAgus   = "agus"
	PicRio    = "rio"
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
	{"kawasan-maingate-gk", "Desain Kawasan", "Maingate", "Gambar kerja maingate", PicRio, DivTeknik, true},
	// Fasos Fasum (100%)
	{"kawasan-fasos-desain", "Desain Kawasan", "Fasos Fasum", "Desain fasos fasum", PicAnanto, DivNone, true},
	{"kawasan-fasos-render", "Desain Kawasan", "Fasos Fasum", "Render fasos fasum", PicAnanto, DivMarketing, true},
	{"kawasan-fasos-gk", "Desain Kawasan", "Fasos Fasum", "Gambar kerja fasos fasum", PicRio, DivTeknik, true},
	// Desain Infrastruktur (100%)
	{"kawasan-infra-rencana", "Desain Kawasan", "Desain Infrastruktur", "Rencana infrastruktur", PicRandi, DivTeknik, true},
	{"kawasan-infra-gk", "Desain Kawasan", "Desain Infrastruktur", "Gambar kerja infrastruktur", PicAgus, DivTeknik, true},
	// Rencana Leveling
	{"kawasan-leveling-rencana", "Desain Kawasan", "Rencana Leveling", "Rencana leveling", PicRandi, DivTeknik, false},
	{"kawasan-leveling-gk", "Desain Kawasan", "Rencana Leveling", "Gambar kerja leveling", PicRio, DivTeknik, false},
	// Desain Landscape
	{"kawasan-landscape-taman", "Desain Kawasan", "Desain Landscape", "Desain taman", PicRandi, DivNone, false},
	{"kawasan-landscape-gk", "Desain Kawasan", "Desain Landscape", "Gambar kerja taman", PicRio, DivTeknik, false},
	// Animasi (100%)
	{"kawasan-animasi", "Desain Kawasan", "Animasi", "Animasi kawasan", PicAnanto, DivMarketing, true},
}

// ProjectSpec parameterises how a new project's deliverable tree is built. A
// project may carry several Site Plans (e.g. one per cluster/phase) and may
// opt in/out of the unit and area categories. The tree is only a starting
// point — deliverables and assignments can be edited dynamically afterwards.
type ProjectSpec struct {
	SitePlans      int  `json:"sitePlans"`      // number of Site Plan groups (>= 1)
	IncludeUnit    bool `json:"includeUnit"`    // include "Desain Unit Hunian"
	IncludeKawasan bool `json:"includeKawasan"` // include "Desain Kawasan"
}

// normalized clamps the spec to sane values (at least one site plan).
func (s ProjectSpec) normalized() ProjectSpec {
	if s.SitePlans < 1 {
		s.SitePlans = 1
	}
	if s.SitePlans > 20 {
		s.SitePlans = 20
	}
	return s
}

// DefaultProjectSpec is the full standard process (used by the seeded portfolio).
func DefaultProjectSpec() ProjectSpec {
	return ProjectSpec{SitePlans: 1, IncludeUnit: true, IncludeKawasan: true}
}

// BuildTaskTree instantiates a deliverable tree from a spec: the Site Plan
// group is repeated SitePlans times ("Site Plan 1", "Site Plan 2", …), and the
// unit / area categories are included per the flags.
func BuildTaskTree(spec ProjectSpec) []Task {
	spec = spec.normalized()
	tasks := []Task{}

	mk := func(s taskSpec, group, idPrefix string) Task {
		return Task{
			ID: idPrefix + s.id, Category: s.category, Group: group,
			Name: s.name, PIC: s.pic, Output: s.output, Weighted: s.weighted,
			Status: StatusTodo,
		}
	}

	for n := 1; n <= spec.SitePlans; n++ {
		group, idPrefix := "Site Plan", ""
		if spec.SitePlans > 1 {
			group = "Site Plan " + itoa(n)
			idPrefix = "sp" + itoa(n) + "-"
		}
		for _, s := range deliverableTemplate {
			if s.category == "Site Plan" {
				tasks = append(tasks, mk(s, group, idPrefix))
			}
		}
	}
	for _, s := range deliverableTemplate {
		if (s.category == "Desain Unit Hunian" && spec.IncludeUnit) ||
			(s.category == "Desain Kawasan" && spec.IncludeKawasan) {
			tasks = append(tasks, mk(s, s.group, ""))
		}
	}
	return tasks
}

// NewTaskTree builds the full standard deliverable tree (all categories, one
// site plan) for a newly seeded project.
func NewTaskTree() []Task { return BuildTaskTree(DefaultProjectSpec()) }

// itoa is a tiny strconv.Itoa to avoid importing strconv for one call site.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [4]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// CategoryOrder is the canonical display order of the top-level categories.
var CategoryOrder = []string{"Site Plan", "Desain Unit Hunian", "Desain Kawasan"}

/* ---- Template as a read-only master tree -------------------------------- */

// TemplateTask is one leaf of the master deliverable template (no process
// state — just the fixed structure: who owns it and where its output goes).
type TemplateTask struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	PIC      string   `json:"pic"`
	Output   Division `json:"output"`
	Weighted bool     `json:"weighted"`
}

// TemplateGroup is a mid-level deliverable (e.g. "Denah") and its leaves.
type TemplateGroup struct {
	Group string         `json:"group"`
	Tasks []TemplateTask `json:"tasks"`
}

// TemplateCategory is a top-level category and its groups.
type TemplateCategory struct {
	Category string          `json:"category"`
	Groups   []TemplateGroup `json:"groups"`
}

// TemplateTree returns the deliverable template grouped category -> group ->
// task, preserving the canonical category order and first-seen group order.
// This is MASTER reference data, identical for every project.
func TemplateTree() []TemplateCategory {
	byCat := map[string]map[string][]TemplateTask{}
	groupOrder := map[string][]string{}
	for _, s := range deliverableTemplate {
		groups, ok := byCat[s.category]
		if !ok {
			groups = map[string][]TemplateTask{}
			byCat[s.category] = groups
		}
		if _, seen := groups[s.group]; !seen {
			groupOrder[s.category] = append(groupOrder[s.category], s.group)
		}
		groups[s.group] = append(groups[s.group], TemplateTask{
			ID: s.id, Name: s.name, PIC: s.pic, Output: s.output, Weighted: s.weighted,
		})
	}

	out := []TemplateCategory{}
	for _, cat := range CategoryOrder {
		groups, ok := byCat[cat]
		if !ok {
			continue
		}
		tc := TemplateCategory{Category: cat}
		for _, gname := range groupOrder[cat] {
			tc.Groups = append(tc.Groups, TemplateGroup{Group: gname, Tasks: groups[gname]})
		}
		out = append(out, tc)
	}
	return out
}

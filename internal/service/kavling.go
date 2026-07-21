// Fase 2 of the relational project model: bloks (phase/cluster) + kavling
// (units) per project. A kavling sits in a blok and is built to a BuildingType;
// Jumlah Unit/Tipe become derived counts from these. CEO / Kadep manage.
package service

import (
	"fmt"
	"strings"

	"greenpark/perencanaan/internal/domain"
)

/* ---- Blok ---- */

func (s *Service) BloksOf(projectID string) []domain.Blok { return s.repo.BloksByProject(projectID) }

func (s *Service) SaveBlok(actorRole, projectID string, in domain.Blok) (domain.Blok, error) {
	if !canManage(actorRole) {
		return domain.Blok{}, ErrForbidden
	}
	if _, ok := s.repo.Project(projectID); !ok {
		return domain.Blok{}, ErrNotFound
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return domain.Blok{}, fmt.Errorf("%w: nama blok wajib diisi", ErrValidation)
	}
	if in.ID == "" {
		in.ProjectID = projectID
	}
	return s.repo.SaveBlok(in), nil
}

func (s *Service) DeleteBlok(actorRole, id string) error {
	if !canManage(actorRole) {
		return ErrForbidden
	}
	if !s.repo.DeleteBlok(id) {
		return ErrNotFound
	}
	return nil
}

/* ---- Kavling ---- */

func (s *Service) KavlingOf(projectID string) []domain.Kavling { return s.repo.KavlingByProject(projectID) }

func (s *Service) SaveKavling(actorRole, projectID string, in domain.Kavling) (domain.Kavling, error) {
	if !canManage(actorRole) {
		return domain.Kavling{}, ErrForbidden
	}
	if _, ok := s.repo.Project(projectID); !ok {
		return domain.Kavling{}, ErrNotFound
	}
	in.NoKav = strings.TrimSpace(in.NoKav)
	in.LebarKavling = strings.TrimSpace(in.LebarKavling)
	if in.NoKav == "" {
		return domain.Kavling{}, fmt.Errorf("%w: No. Kavling wajib diisi", ErrValidation)
	}
	if in.TypeID != "" && !hasID(s.repo.BuildingTypes(), func(t domain.BuildingType) string { return t.ID }, in.TypeID) {
		return domain.Kavling{}, fmt.Errorf("%w: tipe bangunan tidak dikenal", ErrValidation)
	}
	if in.BlokID != "" && !hasID(s.repo.BloksByProject(projectID), func(b domain.Blok) string { return b.ID }, in.BlokID) {
		return domain.Kavling{}, fmt.Errorf("%w: blok bukan milik proyek ini", ErrValidation)
	}
	if in.LuasBangunan < 0 || in.LuasKavling < 0 {
		return domain.Kavling{}, fmt.Errorf("%w: luas tidak boleh negatif", ErrValidation)
	}
	if in.ID == "" {
		in.ProjectID = projectID
	}
	return s.repo.SaveKavling(in), nil
}

func (s *Service) DeleteKavling(actorRole, id string) error {
	if !canManage(actorRole) {
		return ErrForbidden
	}
	if !s.repo.DeleteKavling(id) {
		return ErrNotFound
	}
	return nil
}

/* ---- Bulk import (paste / XLSX / CSV) ---- */

// KavlingImportRow is one spreadsheet row mapped to kavling fields (the frontend
// parses the file/paste + column mapping; the "%" and any unknown columns are
// simply not sent). Bangunan is optional — when 0 it inherits the tipe's luas.
type KavlingImportRow struct {
	NoKav    string `json:"noKav"`
	Tipe     string `json:"tipe"`
	Blok     string `json:"blok"`
	Bangunan int    `json:"bangunan"`
	Kavling  int    `json:"kavling"`
	Lebar    string `json:"lebar"`
}

// KavlingImportSkip records a row that could not be imported.
type KavlingImportSkip struct {
	Row    int    `json:"row"` // 1-based index within the submitted rows
	NoKav  string `json:"noKav"`
	Reason string `json:"reason"`
}

// KavlingImportResult summarizes a bulk import.
type KavlingImportResult struct {
	Created       int                 `json:"created"`
	Updated       int                 `json:"updated"`
	BloksCreated  []string            `json:"bloksCreated"`
	TypesCreated  []string            `json:"typesCreated"`
	LebarsCreated []string            `json:"lebarsCreated"`
	Skipped       []KavlingImportSkip `json:"skipped"`
}

// ImportKavling bulk-creates kavling from parsed spreadsheet rows, resolving Blok
// / BuildingType by NAME (case-insensitive) and CREATING any that are missing,
// plus the Lebar master. Existing kavling are matched by NoKav and updated when
// upsert=true (else a second row with the same NoKav creates a duplicate). CEO /
// Kadep only. Rows with an empty NoKav are skipped and reported.
func (s *Service) ImportKavling(actorRole, projectID string, rows []KavlingImportRow, upsert bool) (KavlingImportResult, error) {
	if !canManage(actorRole) {
		return KavlingImportResult{}, ErrForbidden
	}
	if _, ok := s.repo.Project(projectID); !ok {
		return KavlingImportResult{}, ErrNotFound
	}
	res := KavlingImportResult{
		BloksCreated: []string{}, TypesCreated: []string{}, LebarsCreated: []string{},
		Skipped: []KavlingImportSkip{},
	}
	eq := func(a, b string) bool { return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b)) }

	for i, r := range rows {
		noKav := strings.TrimSpace(r.NoKav)
		if noKav == "" {
			res.Skipped = append(res.Skipped, KavlingImportSkip{Row: i + 1, NoKav: r.NoKav, Reason: "No. Kavling kosong"})
			continue
		}

		// Blok — resolve by name within the project, else create (re-read so a
		// name repeated across rows is only created once).
		blokID := ""
		if bn := strings.TrimSpace(r.Blok); bn != "" {
			found := false
			for _, b := range s.repo.BloksByProject(projectID) {
				if eq(b.Name, bn) {
					blokID, found = b.ID, true
					break
				}
			}
			if !found {
				nb := s.repo.SaveBlok(domain.Blok{ProjectID: projectID, Name: bn})
				blokID = nb.ID
				res.BloksCreated = append(res.BloksCreated, bn)
			}
		}

		// BuildingType — resolve by name (global), else create.
		typeID := ""
		luasBangunan := r.Bangunan
		if tn := strings.TrimSpace(r.Tipe); tn != "" {
			found := false
			for _, t := range s.repo.BuildingTypes() {
				if eq(t.Name, tn) {
					typeID, found = t.ID, true
					if luasBangunan == 0 {
						luasBangunan = t.LuasBangunan
					}
					break
				}
			}
			if !found {
				nt := s.repo.SaveBuildingType(domain.BuildingType{Name: tn, LuasBangunan: r.Bangunan})
				typeID = nt.ID
				res.TypesCreated = append(res.TypesCreated, tn)
				if luasBangunan == 0 {
					luasBangunan = nt.LuasBangunan
				}
			}
		}

		// Lebar — kavling stores the name string; create the master if missing so
		// it shows up in the dropdown.
		lebar := strings.TrimSpace(r.Lebar)
		if lebar != "" {
			has := false
			for _, l := range s.repo.Lebars() {
				if eq(l.Name, lebar) {
					has = true
					break
				}
			}
			if !has {
				s.repo.SaveLebar(domain.Lebar{Name: lebar})
				res.LebarsCreated = append(res.LebarsCreated, lebar)
			}
		}

		k := domain.Kavling{
			ProjectID: projectID, NoKav: noKav, TypeID: typeID, BlokID: blokID,
			LuasBangunan: luasBangunan, LuasKavling: r.Kavling, LebarKavling: lebar,
		}
		if upsert {
			if ex, ok := s.kavlingByNo(projectID, noKav); ok {
				k.ID = ex.ID
				s.repo.SaveKavling(k)
				res.Updated++
				continue
			}
		}
		s.repo.SaveKavling(k)
		res.Created++
	}
	return res, nil
}

// kavlingByNo finds a project kavling by its NoKav (case-insensitive).
func (s *Service) kavlingByNo(projectID, noKav string) (domain.Kavling, bool) {
	for _, k := range s.repo.KavlingByProject(projectID) {
		if strings.EqualFold(strings.TrimSpace(k.NoKav), strings.TrimSpace(noKav)) {
			return k, true
		}
	}
	return domain.Kavling{}, false
}

// hasID reports whether any item's id (via key) equals target.
func hasID[T any](items []T, key func(T) string, target string) bool {
	for _, it := range items {
		if key(it) == target {
			return true
		}
	}
	return false
}

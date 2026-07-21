// Fase 1 of the relational project model: the GP (grup) + building-type masters.
// A project references a GP; building types are reusable house specs (Garnet,
// Ruby, …). Managed by CEO / Kadep in Data Master. Kavling (Fase 2) will
// reference these later.
package service

import (
	"fmt"
	"strings"

	"greenpark/perencanaan/internal/domain"
)

/* ---- GP master ---- */

func (s *Service) ListGPs() []domain.GP { return s.repo.GPs() }

// SaveGP creates (empty ID) or updates a GP. CEO / Kadep only.
func (s *Service) SaveGP(actorRole string, in domain.GP) (domain.GP, error) {
	if !canManage(actorRole) {
		return domain.GP{}, ErrForbidden
	}
	in.Code = strings.TrimSpace(in.Code)
	in.Name = strings.TrimSpace(in.Name)
	if in.Code == "" {
		return domain.GP{}, fmt.Errorf("%w: kode GP wajib diisi", ErrValidation)
	}
	// Unique code (ignoring the record being updated).
	for _, g := range s.repo.GPs() {
		if strings.EqualFold(g.Code, in.Code) && g.ID != in.ID {
			return domain.GP{}, fmt.Errorf("%w: kode GP %q sudah ada", ErrValidation, in.Code)
		}
	}
	return s.repo.SaveGP(in), nil
}

func (s *Service) DeleteGP(actorRole, id string) error {
	if !canManage(actorRole) {
		return ErrForbidden
	}
	if !s.repo.DeleteGP(id) {
		return ErrNotFound
	}
	return nil
}

/* ---- Building-type master ---- */

func (s *Service) ListBuildingTypes() []domain.BuildingType { return s.repo.BuildingTypes() }

// SaveBuildingType creates or updates a house type. CEO / Kadep only.
func (s *Service) SaveBuildingType(actorRole string, in domain.BuildingType) (domain.BuildingType, error) {
	if !canManage(actorRole) {
		return domain.BuildingType{}, ErrForbidden
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return domain.BuildingType{}, fmt.Errorf("%w: nama tipe wajib diisi", ErrValidation)
	}
	if in.LuasBangunan < 0 || in.LuasTanah < 0 {
		return domain.BuildingType{}, fmt.Errorf("%w: luas tidak boleh negatif", ErrValidation)
	}
	for _, t := range s.repo.BuildingTypes() {
		if strings.EqualFold(t.Name, in.Name) && t.ID != in.ID {
			return domain.BuildingType{}, fmt.Errorf("%w: tipe %q sudah ada", ErrValidation, in.Name)
		}
	}
	return s.repo.SaveBuildingType(in), nil
}

func (s *Service) DeleteBuildingType(actorRole, id string) error {
	if !canManage(actorRole) {
		return ErrForbidden
	}
	if !s.repo.DeleteBuildingType(id) {
		return ErrNotFound
	}
	return nil
}

/* ---- Lebar master ---- */

func (s *Service) ListLebars() []domain.Lebar { return s.repo.Lebars() }

func (s *Service) SaveLebar(actorRole string, in domain.Lebar) (domain.Lebar, error) {
	if !canManage(actorRole) {
		return domain.Lebar{}, ErrForbidden
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return domain.Lebar{}, fmt.Errorf("%w: nama lebar wajib diisi", ErrValidation)
	}
	for _, l := range s.repo.Lebars() {
		if strings.EqualFold(l.Name, in.Name) && l.ID != in.ID {
			return domain.Lebar{}, fmt.Errorf("%w: lebar %q sudah ada", ErrValidation, in.Name)
		}
	}
	return s.repo.SaveLebar(in), nil
}

func (s *Service) DeleteLebar(actorRole, id string) error {
	if !canManage(actorRole) {
		return ErrForbidden
	}
	if !s.repo.DeleteLebar(id) {
		return ErrNotFound
	}
	return nil
}

/* ---- Lokasi master ---- */

func (s *Service) ListLokasis() []domain.Lokasi { return s.repo.Lokasis() }

func (s *Service) SaveLokasi(actorRole string, in domain.Lokasi) (domain.Lokasi, error) {
	if !canManage(actorRole) {
		return domain.Lokasi{}, ErrForbidden
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return domain.Lokasi{}, fmt.Errorf("%w: nama lokasi wajib diisi", ErrValidation)
	}
	for _, l := range s.repo.Lokasis() {
		if strings.EqualFold(l.Name, in.Name) && l.ID != in.ID {
			return domain.Lokasi{}, fmt.Errorf("%w: lokasi %q sudah ada", ErrValidation, in.Name)
		}
	}
	return s.repo.SaveLokasi(in), nil
}

func (s *Service) DeleteLokasi(actorRole, id string) error {
	if !canManage(actorRole) {
		return ErrForbidden
	}
	if !s.repo.DeleteLokasi(id) {
		return ErrNotFound
	}
	return nil
}

/* ---- Bulk import (Master Produk: GP / Tipe / Lebar / Lokasi) ---- */

// MasterImportRow is one parsed row for a Master Produk import. Only the fields
// relevant to the kind are used — gp: code(+name); tipe: name+bangunan+tanah;
// lebar/lokasi: name. The frontend maps columns and sends these fields.
type MasterImportRow struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	Bangunan int    `json:"bangunan"`
	Tanah    int    `json:"tanah"`
}

// MasterImportSkip records a row that could not be imported.
type MasterImportSkip struct {
	Row    int    `json:"row"` // 1-based index within the submitted rows
	Key    string `json:"key"` // the code/name that was skipped
	Reason string `json:"reason"`
}

// MasterImportResult summarizes a Master Produk import.
type MasterImportResult struct {
	Created int                `json:"created"`
	Updated int                `json:"updated"`
	Skipped []MasterImportSkip `json:"skipped"`
}

// ImportMaster bulk-creates/updates ONE Master Produk kind from parsed rows.
// kind: "gp" | "tipe" | "lebar" | "lokasi". Existing records are matched by key
// (GP by code, the rest by name, case-insensitive) and updated when upsert=true
// (else skipped). Rows with an empty key are skipped and reported. CEO / Kadep.
func (s *Service) ImportMaster(actorRole, kind string, rows []MasterImportRow, upsert bool) (MasterImportResult, error) {
	if !canManage(actorRole) {
		return MasterImportResult{}, ErrForbidden
	}
	res := MasterImportResult{Skipped: []MasterImportSkip{}}
	eq := func(a, b string) bool { return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b)) }
	skip := func(i int, key, reason string) {
		res.Skipped = append(res.Skipped, MasterImportSkip{Row: i + 1, Key: key, Reason: reason})
	}

	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "gp":
		for i, r := range rows {
			code := strings.TrimSpace(r.Code)
			if code == "" {
				skip(i, "", "kode GP kosong")
				continue
			}
			exID := ""
			for _, g := range s.repo.GPs() {
				if eq(g.Code, code) {
					exID = g.ID
					break
				}
			}
			if exID != "" && !upsert {
				skip(i, code, "kode GP sudah ada")
				continue
			}
			s.repo.SaveGP(domain.GP{ID: exID, Code: code, Name: strings.TrimSpace(r.Name)})
			if exID != "" {
				res.Updated++
			} else {
				res.Created++
			}
		}
	case "tipe", "type", "tipebangunan":
		for i, r := range rows {
			name := strings.TrimSpace(r.Name)
			if name == "" {
				skip(i, "", "nama tipe kosong")
				continue
			}
			exID := ""
			for _, t := range s.repo.BuildingTypes() {
				if eq(t.Name, name) {
					exID = t.ID
					break
				}
			}
			if exID != "" && !upsert {
				skip(i, name, "tipe sudah ada")
				continue
			}
			s.repo.SaveBuildingType(domain.BuildingType{ID: exID, Name: name, LuasBangunan: r.Bangunan, LuasTanah: r.Tanah})
			if exID != "" {
				res.Updated++
			} else {
				res.Created++
			}
		}
	case "lebar":
		for i, r := range rows {
			name := strings.TrimSpace(r.Name)
			if name == "" {
				skip(i, "", "nama lebar kosong")
				continue
			}
			exID := ""
			for _, l := range s.repo.Lebars() {
				if eq(l.Name, name) {
					exID = l.ID
					break
				}
			}
			if exID != "" && !upsert {
				skip(i, name, "lebar sudah ada")
				continue
			}
			s.repo.SaveLebar(domain.Lebar{ID: exID, Name: name})
			if exID != "" {
				res.Updated++
			} else {
				res.Created++
			}
		}
	case "lokasi":
		for i, r := range rows {
			name := strings.TrimSpace(r.Name)
			if name == "" {
				skip(i, "", "nama lokasi kosong")
				continue
			}
			exID := ""
			for _, l := range s.repo.Lokasis() {
				if eq(l.Name, name) {
					exID = l.ID
					break
				}
			}
			if exID != "" && !upsert {
				skip(i, name, "lokasi sudah ada")
				continue
			}
			s.repo.SaveLokasi(domain.Lokasi{ID: exID, Name: name})
			if exID != "" {
				res.Updated++
			} else {
				res.Created++
			}
		}
	default:
		return MasterImportResult{}, fmt.Errorf("%w: jenis master tidak dikenal: %q", ErrValidation, kind)
	}
	return res, nil
}

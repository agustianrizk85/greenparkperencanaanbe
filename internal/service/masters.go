// Fase 1 of the relational project model: the GP (grup) + building-type masters.
// A project references a GP; building types are reusable house specs (Garnet,
// Ruby, …). Managed by CEO / Kadep in Data Master. Kavling (Fase 2) will
// reference these later.
package service

import (
	"fmt"
	"mime"
	"os"
	"path/filepath"
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

// SaveBuildingType creates or updates a house type. CEO / Kadep only. Images
// are NEVER touched here — they only change via Add/DeleteBuildingTypeImage —
// so a plain metadata edit (name/luas) can't accidentally wipe the gallery.
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
		if t.ID == in.ID {
			in.Images = t.Images
		}
	}
	return s.repo.SaveBuildingType(in), nil
}

func (s *Service) DeleteBuildingType(actorRole, id string) error {
	if !canManage(actorRole) {
		return ErrForbidden
	}
	var imgIDs []string
	for _, t := range s.repo.BuildingTypes() {
		if t.ID == id {
			for _, img := range t.Images {
				imgIDs = append(imgIDs, img.ID)
			}
			break
		}
	}
	if !s.repo.DeleteBuildingType(id) {
		return ErrNotFound
	}
	s.removeBoardFiles(imgIDs)
	return nil
}

// CanManageBuildingTypes reports whether role may manage building-type master
// data (CEO/Kadep) — used by the upload handler to reject fast BEFORE it
// consumes a potentially large multipart body.
func CanManageBuildingTypes(role string) bool { return canManage(role) }

// AddBuildingTypeImage registers an already-streamed upload as a reference
// photo on a house-type master: tmpPath is a fully written temp file inside
// the upload dir. On success the file is renamed to its image ID and the
// metadata is appended; on any error the CALLER removes the temp file.
// CEO / Kadep only.
func (s *Service) AddBuildingTypeImage(actor domain.User, typeID, filename, partMime, tmpPath string, size int64) (domain.BuildingTypeImage, error) {
	if !canManage(actor.Role) {
		return domain.BuildingTypeImage{}, ErrForbidden
	}
	filename = strings.TrimSpace(filename)
	if filename == "" {
		filename = "gambar"
	}
	mimeType := mime.TypeByExtension(strings.ToLower(filepath.Ext(filename)))
	if mimeType == "" {
		mimeType = strings.TrimSpace(partMime)
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	id := s.repo.NextBoardID("bti")
	if err := os.Rename(tmpPath, filepath.Join(s.uploadDir, id)); err != nil {
		return domain.BuildingTypeImage{}, fmt.Errorf("menyimpan file gambar: %w", err)
	}
	img := domain.BuildingTypeImage{ID: id, Name: filename, Size: size, Mime: mimeType, By: actor.Username, At: s.nowRFC3339()}
	ok := s.repo.MutateBuildingType(typeID, func(t *domain.BuildingType) {
		t.Images = append(t.Images, img)
	})
	if !ok {
		s.removeBoardFiles([]string{id}) // type vanished — drop the orphaned file
		return domain.BuildingTypeImage{}, ErrNotFound
	}
	return img, nil
}

// BuildingTypeImageFile resolves an image's metadata + its disk path (any
// authenticated department user may view).
func (s *Service) BuildingTypeImageFile(typeID, imgID string) (domain.BuildingTypeImage, string, error) {
	for _, t := range s.repo.BuildingTypes() {
		if t.ID != typeID {
			continue
		}
		for _, img := range t.Images {
			if img.ID == imgID {
				return img, filepath.Join(s.uploadDir, img.ID), nil
			}
		}
	}
	return domain.BuildingTypeImage{}, "", ErrNotFound
}

// DeleteBuildingTypeImage removes a reference photo (CEO / Kadep only) and
// deletes its file from disk (best-effort).
func (s *Service) DeleteBuildingTypeImage(actorRole, typeID, imgID string) error {
	if !canManage(actorRole) {
		return ErrForbidden
	}
	removed := false
	found := s.repo.MutateBuildingType(typeID, func(t *domain.BuildingType) {
		for i := range t.Images {
			if t.Images[i].ID == imgID {
				t.Images = append(t.Images[:i], t.Images[i+1:]...)
				removed = true
				return
			}
		}
	})
	if !found || !removed {
		return ErrNotFound
	}
	s.removeBoardFiles([]string{imgID})
	return nil
}

/* ---- Bulk import (Master Produk: GP / Tipe) ---- */

// MasterImportRow is one parsed row for a Master Produk import. Only the fields
// relevant to the kind are used — gp: code(+name); tipe: name+bangunan+tanah.
// The frontend maps columns and sends these fields.
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
// kind: "gp" | "tipe". Existing records are matched by key
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
	default:
		return MasterImportResult{}, fmt.Errorf("%w: jenis master tidak dikenal: %q", ErrValidation, kind)
	}
	return res, nil
}

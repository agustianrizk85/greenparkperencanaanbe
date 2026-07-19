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

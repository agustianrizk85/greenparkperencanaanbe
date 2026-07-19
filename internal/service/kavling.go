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

// hasID reports whether any item's id (via key) equals target.
func hasID[T any](items []T, key func(T) string, target string) bool {
	for _, it := range items {
		if key(it) == target {
			return true
		}
	}
	return false
}

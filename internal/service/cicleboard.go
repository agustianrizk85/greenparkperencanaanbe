package service

import "encoding/json"

// CicleBoard returns the stored raw mirror of the cicle Kanban board (columns +
// cards), or an empty object when nothing has been synced yet.
func (s *Service) CicleBoard() json.RawMessage {
	b := s.repo.CicleBoard()
	if len(b) == 0 {
		return json.RawMessage(`{"columns":[]}`)
	}
	return b
}

// SetCicleBoard stores the raw cicle board mirror. Only CEO / Kadep may push a
// sync. The payload is stored verbatim (the frontend interprets it), so the
// backend stays agnostic to cicle's exact card shape.
func (s *Service) SetCicleBoard(role string, data json.RawMessage) error {
	if !canManage(role) {
		return ErrForbidden
	}
	if !json.Valid(data) {
		return ErrValidation
	}
	s.repo.SetCicleBoard(data)
	return nil
}

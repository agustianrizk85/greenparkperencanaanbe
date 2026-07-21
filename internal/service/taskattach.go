// Multi-attachment support for formal Tasks (the board's task cards). A task may
// carry any number of files of ANY type (≤1 GiB each). This mirrors the board
// free-card attachment flow exactly: the bytes are streamed to disk at
// <uploadDir>/<attId> (never in the state snapshot) and only the metadata rides
// in the Task (persisted inside its Project). Deleting an attachment removes the
// disk file too. Editing a task's attachments is allowed for a manager or the
// task's owning PIC.
package service

import (
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"greenpark/perencanaan/internal/domain"
)

// canEditTask reports whether actor may attach/detach files on a task: a manager
// (CEO/Dirops/Kadep) or the task's owning PIC.
func canEditTask(actor domain.User, t domain.Task) bool {
	return canManage(actor.Role) || actor.Username == t.PIC
}

// taskByID returns a copy of a task (and whether it exists).
func (s *Service) taskByID(projectID, taskID string) (domain.Task, bool) {
	p, ok := s.repo.Project(projectID)
	if !ok {
		return domain.Task{}, false
	}
	for _, t := range p.Tasks {
		if t.ID == taskID {
			return t, true
		}
	}
	return domain.Task{}, false
}

// CanEditTask reports (as an error) whether actor may edit the task — used by the
// upload handler BEFORE it consumes a potentially huge body. Mirrors
// BoardCanEditCard.
func (s *Service) CanEditTask(actor domain.User, projectID, taskID string) error {
	t, ok := s.taskByID(projectID, taskID)
	if !ok {
		return ErrNotFound
	}
	if !canEditTask(actor, t) {
		return ErrForbidden
	}
	return nil
}

// AddTaskComment appends a comment to a task's discussion thread. Any authenticated
// board user may comment (mirrors free-card comments).
func (s *Service) AddTaskComment(actor domain.User, projectID, taskID, text string) (domain.BoardComment, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return domain.BoardComment{}, fmt.Errorf("%w: teks komentar wajib diisi", ErrValidation)
	}
	out := domain.BoardComment{
		ID:     s.repo.NextBoardID("cm"),
		Author: actor.Username,
		Text:   text,
		At:     s.nowRFC3339(),
	}
	found := s.repo.MutateTask(projectID, taskID, func(t *domain.Task) {
		t.Comments = append(t.Comments, out)
	})
	if !found {
		return domain.BoardComment{}, ErrNotFound
	}
	return out, nil
}

// DeleteTaskComment removes a task comment (its author, the task PIC, or a manager).
func (s *Service) DeleteTaskComment(actor domain.User, projectID, taskID, commentID string) error {
	var permErr error
	removed := false
	found := s.repo.MutateTask(projectID, taskID, func(t *domain.Task) {
		for i := range t.Comments {
			if t.Comments[i].ID != commentID {
				continue
			}
			if t.Comments[i].Author != actor.Username && !canEditTask(actor, *t) {
				permErr = ErrForbidden
				return
			}
			t.Comments = append(t.Comments[:i], t.Comments[i+1:]...)
			removed = true
			return
		}
	})
	if !found {
		return ErrNotFound
	}
	if permErr != nil {
		return permErr
	}
	if !removed {
		return ErrNotFound
	}
	return nil
}

// AddTaskAttachment registers an already-streamed upload as a task attachment:
// tmpPath is a fully written temp file inside the upload dir. On success the file
// is renamed to its attachment ID and the metadata is appended to the task; on
// any error the CALLER removes the temp file. Editor only (manager or PIC).
func (s *Service) AddTaskAttachment(actor domain.User, projectID, taskID, filename, partMime, tmpPath string, size int64) (domain.TaskAttachment, error) {
	// Re-check the editor permission (the pre-body check already ran, but the task
	// could have been reassigned in between).
	if err := s.CanEditTask(actor, projectID, taskID); err != nil {
		return domain.TaskAttachment{}, err
	}

	filename = strings.TrimSpace(filename)
	if filename == "" {
		filename = "lampiran"
	}
	mimeType := mime.TypeByExtension(strings.ToLower(filepath.Ext(filename)))
	if mimeType == "" {
		mimeType = strings.TrimSpace(partMime)
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	// Mint the id from the shared board sequence so it never collides with a
	// board-card attachment file in the same upload dir, then move the temp file
	// to its final name BEFORE committing the metadata.
	id := s.repo.NextBoardID("att")
	if err := os.Rename(tmpPath, filepath.Join(s.uploadDir, id)); err != nil {
		return domain.TaskAttachment{}, fmt.Errorf("menyimpan file lampiran: %w", err)
	}
	att := domain.TaskAttachment{ID: id, Name: filename, Size: size, Mime: mimeType, By: actor.Username, At: s.nowRFC3339()}
	ok := s.repo.MutateTask(projectID, taskID, func(t *domain.Task) {
		t.Attachments = append(t.Attachments, att)
	})
	if !ok {
		s.removeBoardFiles([]string{id}) // task vanished — drop the orphaned file
		return domain.TaskAttachment{}, ErrNotFound
	}
	return att, nil
}

// TaskAttachmentFile resolves a task attachment's metadata + its disk path (any
// authenticated user may download — mirrors BoardAttachment).
func (s *Service) TaskAttachmentFile(projectID, taskID, attID string) (domain.TaskAttachment, string, error) {
	t, ok := s.taskByID(projectID, taskID)
	if !ok {
		return domain.TaskAttachment{}, "", ErrNotFound
	}
	for _, a := range t.Attachments {
		if a.ID == attID {
			return a, filepath.Join(s.uploadDir, a.ID), nil
		}
	}
	return domain.TaskAttachment{}, "", ErrNotFound
}

// DeleteTaskAttachment removes a task attachment (editor only) and deletes its
// file from disk (best-effort), mirroring BoardDeleteAttachment.
func (s *Service) DeleteTaskAttachment(actor domain.User, projectID, taskID, attID string) error {
	if err := s.CanEditTask(actor, projectID, taskID); err != nil {
		return err
	}
	removed := false
	found := s.repo.MutateTask(projectID, taskID, func(t *domain.Task) {
		for i := range t.Attachments {
			if t.Attachments[i].ID == attID {
				t.Attachments = append(t.Attachments[:i], t.Attachments[i+1:]...)
				removed = true
				return
			}
		}
	})
	if !found || !removed {
		return ErrNotFound
	}
	s.removeBoardFiles([]string{attID})
	return nil
}

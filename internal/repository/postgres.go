package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"greenpark/perencanaan/internal/domain"

	_ "github.com/jackc/pgx/v5/stdlib" // pgx database/sql driver ("pgx")
)

// Persistent is a PostgreSQL-backed Store. It embeds the in-memory *Memory for
// all read logic and seeding, and mirrors the full state to a single JSONB row
// after every mutation. On startup it restores the saved snapshot, so the
// dashboard survives restarts. The in-memory business logic is unchanged.
type Persistent struct {
	*Memory
	db    *sql.DB
	fresh bool // true when the database had no prior snapshot (first run)
}

// Fresh reports whether the database was empty on startup (so callers may run
// one-time demo seeding). It is false once a snapshot has ever been saved.
func (p *Persistent) Fresh() bool { return p.fresh }

// NewPersistent connects to Postgres, ensures the state table, and restores the
// saved snapshot. On an empty database it writes the initial snapshot — seeded
// with the built-in example data when seedMaster is true, otherwise empty.
func NewPersistent(dsn string, seedMaster bool) (*Persistent, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Hour)
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS perencanaan_state (
		id INT PRIMARY KEY,
		data JSONB NOT NULL,
		updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	p := &Persistent{Memory: newMemory(seedMaster), db: db}

	var data []byte
	switch err := db.QueryRow(`SELECT data FROM perencanaan_state WHERE id = 1`).Scan(&data); err {
	case sql.ErrNoRows:
		// Fresh database — persist the seeded portfolio as the initial snapshot.
		p.fresh = true
		if err := p.save(); err != nil {
			return nil, fmt.Errorf("seed snapshot: %w", err)
		}
	case nil:
		if err := p.Memory.LoadJSON(data); err != nil {
			return nil, fmt.Errorf("restore: %w", err)
		}
	default:
		return nil, fmt.Errorf("load: %w", err)
	}
	return p, nil
}

// Close releases the database connection.
func (p *Persistent) Close() error { return p.db.Close() }

// save writes the full in-memory state to the single state row.
func (p *Persistent) save() error {
	data, err := p.Memory.SnapshotJSON()
	if err != nil {
		return err
	}
	_, err = p.db.Exec(`INSERT INTO perencanaan_state (id, data, updated_at)
		VALUES (1, $1, now())
		ON CONFLICT (id) DO UPDATE SET data = EXCLUDED.data, updated_at = now()`, data)
	return err
}

/* ---- mutations: delegate to Memory, then persist ----------------------- */

func (p *Persistent) ResetProses() { p.Memory.ResetProses(); _ = p.save() }
func (p *Persistent) ResetMaster() { p.Memory.ResetMaster(); _ = p.save() }

func (p *Persistent) MutateTask(projectID, taskID string, fn func(*domain.Task)) bool {
	ok := p.Memory.MutateTask(projectID, taskID, fn)
	if ok {
		_ = p.save()
	}
	return ok
}

func (p *Persistent) SetTaskDoc(projectID, taskID string, doc domain.TaskDoc, data []byte) bool {
	ok := p.Memory.SetTaskDoc(projectID, taskID, doc, data)
	if ok {
		_ = p.save()
	}
	return ok
}

func (p *Persistent) UpsertUser(u domain.User) bool {
	ok := p.Memory.UpsertUser(u)
	if ok {
		_ = p.save()
	}
	return ok
}

func (p *Persistent) SetDepartments(depts []domain.Department) {
	p.Memory.SetDepartments(depts)
	_ = p.save()
}

func (p *Persistent) SaveGP(gp domain.GP) domain.GP {
	out := p.Memory.SaveGP(gp)
	_ = p.save()
	return out
}

func (p *Persistent) DeleteGP(id string) bool {
	ok := p.Memory.DeleteGP(id)
	if ok {
		_ = p.save()
	}
	return ok
}

func (p *Persistent) SaveBuildingType(t domain.BuildingType) domain.BuildingType {
	out := p.Memory.SaveBuildingType(t)
	_ = p.save()
	return out
}

func (p *Persistent) DeleteBuildingType(id string) bool {
	ok := p.Memory.DeleteBuildingType(id)
	if ok {
		_ = p.save()
	}
	return ok
}

func (p *Persistent) SaveLebar(l domain.Lebar) domain.Lebar {
	out := p.Memory.SaveLebar(l)
	_ = p.save()
	return out
}
func (p *Persistent) DeleteLebar(id string) bool {
	ok := p.Memory.DeleteLebar(id)
	if ok {
		_ = p.save()
	}
	return ok
}
func (p *Persistent) SaveLokasi(l domain.Lokasi) domain.Lokasi {
	out := p.Memory.SaveLokasi(l)
	_ = p.save()
	return out
}
func (p *Persistent) DeleteLokasi(id string) bool {
	ok := p.Memory.DeleteLokasi(id)
	if ok {
		_ = p.save()
	}
	return ok
}

func (p *Persistent) SaveBlok(b domain.Blok) domain.Blok {
	out := p.Memory.SaveBlok(b)
	_ = p.save()
	return out
}

func (p *Persistent) DeleteProject(id string) bool {
	ok := p.Memory.DeleteProject(id)
	if ok {
		_ = p.save()
	}
	return ok
}

func (p *Persistent) DeleteBlok(id string) bool {
	ok := p.Memory.DeleteBlok(id)
	if ok {
		_ = p.save()
	}
	return ok
}

func (p *Persistent) SaveKavling(k domain.Kavling) domain.Kavling {
	out := p.Memory.SaveKavling(k)
	_ = p.save()
	return out
}

func (p *Persistent) DeleteKavling(id string) bool {
	ok := p.Memory.DeleteKavling(id)
	if ok {
		_ = p.save()
	}
	return ok
}

func (p *Persistent) SetTaskAIAnnotated(projectID, taskID, name string, data []byte) bool {
	ok := p.Memory.SetTaskAIAnnotated(projectID, taskID, name, data)
	if ok {
		_ = p.save()
	}
	return ok
}

func (p *Persistent) SetWorkDrawingDoc(wdID, kind string, doc domain.GKDoc, data []byte) bool {
	ok := p.Memory.SetWorkDrawingDoc(wdID, kind, doc, data)
	if ok {
		_ = p.save()
	}
	return ok
}

func (p *Persistent) AddUser(u domain.User) bool {
	ok := p.Memory.AddUser(u)
	if ok {
		_ = p.save()
	}
	return ok
}

func (p *Persistent) DeleteUser(username string) bool {
	ok := p.Memory.DeleteUser(username)
	if ok {
		_ = p.save()
	}
	return ok
}

func (p *Persistent) AddProject(gp, name, lokasi, luas string, units, types int, spec domain.ProjectSpec) domain.Project {
	pr := p.Memory.AddProject(gp, name, lokasi, luas, units, types, spec)
	_ = p.save()
	return pr
}

func (p *Persistent) AddTask(projectID string, t domain.Task) (domain.Task, bool) {
	task, ok := p.Memory.AddTask(projectID, t)
	if ok {
		_ = p.save()
	}
	return task, ok
}

func (p *Persistent) RemoveTask(projectID, taskID string) bool {
	ok := p.Memory.RemoveTask(projectID, taskID)
	if ok {
		_ = p.save()
	}
	return ok
}

func (p *Persistent) UpdateTaskMeta(projectID, taskID, pic string, output domain.Division) bool {
	ok := p.Memory.UpdateTaskMeta(projectID, taskID, pic, output)
	if ok {
		_ = p.save()
	}
	return ok
}

func (p *Persistent) UpdateTaskStatus(projectID, taskID string, status domain.TaskStatus, at string) (string, bool) {
	pic, ok := p.Memory.UpdateTaskStatus(projectID, taskID, status, at)
	if ok {
		_ = p.save()
	}
	return pic, ok
}

func (p *Persistent) AddWorkDrawing(d domain.WorkDrawing) domain.WorkDrawing {
	wd := p.Memory.AddWorkDrawing(d)
	_ = p.save()
	return wd
}

func (p *Persistent) MutateWorkDrawing(id string, fn func(*domain.WorkDrawing)) (domain.WorkDrawing, bool) {
	wd, ok := p.Memory.MutateWorkDrawing(id, fn)
	if ok {
		_ = p.save()
	}
	return wd, ok
}

func (p *Persistent) SetCicleBoard(data json.RawMessage) {
	p.Memory.SetCicleBoard(data)
	_ = p.save()
}

/* ---- department Kanban board ------------------------------------------- */

func (p *Persistent) EnsureBoardSystemLists() {
	p.Memory.EnsureBoardSystemLists()
	_ = p.save()
}

func (p *Persistent) AddBoardList(title, createdBy string) domain.BoardList {
	out := p.Memory.AddBoardList(title, createdBy)
	_ = p.save()
	return out
}

func (p *Persistent) UpdateBoardList(listID string, title *string, index *int) (domain.BoardList, bool) {
	out, ok := p.Memory.UpdateBoardList(listID, title, index)
	if ok {
		_ = p.save()
	}
	return out, ok
}

func (p *Persistent) DeleteBoardList(listID string) ([]string, bool) {
	atts, ok := p.Memory.DeleteBoardList(listID)
	if ok {
		_ = p.save()
	}
	return atts, ok
}

func (p *Persistent) AddBoardCard(listID string, card domain.BoardCard) (domain.BoardCard, bool) {
	out, ok := p.Memory.AddBoardCard(listID, card)
	if ok {
		_ = p.save()
	}
	return out, ok
}

func (p *Persistent) MutateBoardCard(cardID string, fn func(c *domain.BoardCard, newID func(prefix string) string) error) (domain.BoardCard, bool, error) {
	out, ok, err := p.Memory.MutateBoardCard(cardID, fn)
	if ok && err == nil {
		_ = p.save()
	}
	return out, ok, err
}

func (p *Persistent) MoveBoardCard(cardID, toListID string, index int, at string) (domain.BoardCard, bool) {
	out, ok := p.Memory.MoveBoardCard(cardID, toListID, index, at)
	if ok {
		_ = p.save()
	}
	return out, ok
}

func (p *Persistent) DeleteBoardCard(cardID string) ([]string, bool) {
	atts, ok := p.Memory.DeleteBoardCard(cardID)
	if ok {
		_ = p.save()
	}
	return atts, ok
}

func (p *Persistent) AddBoardLabel(name, color string) domain.BoardLabel {
	out := p.Memory.AddBoardLabel(name, color)
	_ = p.save()
	return out
}

func (p *Persistent) UpdateBoardLabel(labelID string, name, color *string) (domain.BoardLabel, bool) {
	out, ok := p.Memory.UpdateBoardLabel(labelID, name, color)
	if ok {
		_ = p.save()
	}
	return out, ok
}

func (p *Persistent) DeleteBoardLabel(labelID string) bool {
	ok := p.Memory.DeleteBoardLabel(labelID)
	if ok {
		_ = p.save()
	}
	return ok
}

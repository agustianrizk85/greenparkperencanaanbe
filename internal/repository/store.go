package repository

import (
	"encoding/json"

	"greenpark/perencanaan/internal/domain"
)

// Store is the persistence contract the service depends on. Both the in-memory
// *Memory and the Postgres-backed *Persistent satisfy it, so the storage backend
// is chosen at wiring time (env-driven) without touching business logic.
type Store interface {
	// Users.
	UserByUsername(username string) (domain.User, bool)
	Users() []domain.User
	AddUser(u domain.User) bool
	UpsertUser(u domain.User) bool
	DeleteUser(username string) bool

	// Department catalogue (synced from auth SSO) — drives the "output to
	// division" options + aggregation.
	Departments() []domain.Department
	SetDepartments(depts []domain.Department)

	// GP + building-type masters (Fase 1 of the relational project model).
	GPs() []domain.GP
	SaveGP(gp domain.GP) domain.GP
	DeleteGP(id string) bool
	BuildingTypes() []domain.BuildingType
	SaveBuildingType(t domain.BuildingType) domain.BuildingType
	DeleteBuildingType(id string) bool
	Lebars() []domain.Lebar
	SaveLebar(l domain.Lebar) domain.Lebar
	DeleteLebar(id string) bool
	Lokasis() []domain.Lokasi
	SaveLokasi(l domain.Lokasi) domain.Lokasi
	DeleteLokasi(id string) bool

	// Blok + kavling (Fase 2) — per-project.
	BloksByProject(projectID string) []domain.Blok
	SaveBlok(b domain.Blok) domain.Blok
	DeleteBlok(id string) bool
	KavlingByProject(projectID string) []domain.Kavling
	SaveKavling(k domain.Kavling) domain.Kavling
	DeleteKavling(id string) bool

	// Projects & tasks.
	Projects() []domain.Project
	Project(id string) (domain.Project, bool)
	AddProject(gp, name, lokasi, luas string, units, types int, spec domain.ProjectSpec) domain.Project
	AddTask(projectID string, t domain.Task) (domain.Task, bool)
	RemoveTask(projectID, taskID string) bool
	UpdateTaskMeta(projectID, taskID, pic string, output domain.Division) bool
	UpdateTaskStatus(projectID, taskID string, status domain.TaskStatus, at string) (pic string, ok bool)
	MutateTask(projectID, taskID string, fn func(*domain.Task)) bool

	// Review documents.
	SetTaskDoc(projectID, taskID string, doc domain.TaskDoc, data []byte) bool
	TaskDocBytes(projectID, taskID string) ([]byte, string, bool)
	// Deep Analisis AI annotated result PDF (separate from the review Doc).
	SetTaskAIAnnotated(projectID, taskID, name string, data []byte) bool
	TaskAIAnnotatedBytes(projectID, taskID string) ([]byte, string, bool)

	// Working drawings (gambar kerja).
	WorkDrawings() []domain.WorkDrawing
	AddWorkDrawing(d domain.WorkDrawing) domain.WorkDrawing
	MutateWorkDrawing(id string, fn func(*domain.WorkDrawing)) (domain.WorkDrawing, bool)

	// Deep Revisi AI documents (GK Kontraktor / GK TTD / annotated output).
	SetWorkDrawingDoc(wdID, kind string, doc domain.GKDoc, data []byte) bool
	WorkDrawingDocBytes(wdID, kind string) ([]byte, string, bool)

	// Admin resets.
	ResetProses()
	ResetMaster()

	// Cicle board mirror (raw JSON of the synced Kanban board).
	CicleBoard() json.RawMessage
	SetCicleBoard(data json.RawMessage)
}

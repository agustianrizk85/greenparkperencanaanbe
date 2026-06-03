package repository

import (
	"sync"

	"greenpark/perencanaan/internal/domain"
)

// Memory is a writable, concurrency-safe in-memory store. List entities are
// backed by generic collections; context, lead-quality, content and alerts are
// singletons guarded by a single mutex.
type Memory struct {
	funnel      *collection[domain.FunnelStage]
	kpis        *collection[domain.KPI]
	channels    *collection[domain.Channel]
	projects    *collection[domain.Project]
	assets      *collection[domain.Asset]
	igAccounts  *collection[domain.IGAccount]
	handover    *collection[domain.HandoverMetric]
	winning     *collection[domain.WinningCampaign]
	commands    *collection[domain.Command]
	reasonCodes *collection[domain.ReasonCode]

	muSingle    sync.RWMutex
	context     domain.Context
	leadQuality domain.LeadQuality
	content     domain.Content
	alerts      domain.Alerts

	users map[string]domain.User
}

// NewMemory returns a Memory store seeded with representative data and users.
func NewMemory() *Memory {
	return &Memory{
		funnel: newCollection("fnl", seedFunnel(),
			func(f *domain.FunnelStage) string { return f.ID }, func(f *domain.FunnelStage, id string) { f.ID = id }),
		kpis: newCollection("kpi", seedKPIs(),
			func(k *domain.KPI) string { return k.ID }, func(k *domain.KPI, id string) { k.ID = id }),
		channels: newCollection("ch", seedChannels(),
			func(c *domain.Channel) string { return c.ID }, func(c *domain.Channel, id string) { c.ID = id }),
		projects: newCollection("prj", seedProjects(),
			func(p *domain.Project) string { return p.ID }, func(p *domain.Project, id string) { p.ID = id }),
		assets: newCollection("ast", seedAssets(),
			func(a *domain.Asset) string { return a.ID }, func(a *domain.Asset, id string) { a.ID = id }),
		igAccounts: newCollection("ig", seedIGAccounts(),
			func(a *domain.IGAccount) string { return a.ID }, func(a *domain.IGAccount, id string) { a.ID = id }),
		handover: newCollection("hd", seedHandover(),
			func(h *domain.HandoverMetric) string { return h.ID }, func(h *domain.HandoverMetric, id string) { h.ID = id }),
		winning: newCollection("win", seedWinning(),
			func(w *domain.WinningCampaign) string { return w.ID }, func(w *domain.WinningCampaign, id string) { w.ID = id }),
		commands: newCollection("cmd", seedCommands(),
			func(c *domain.Command) string { return c.ID }, func(c *domain.Command, id string) { c.ID = id }),
		reasonCodes: newCollection("rc", seedReasonCodes(),
			func(r *domain.ReasonCode) string { return r.ID }, func(r *domain.ReasonCode, id string) { r.ID = id }),

		context:     seedContext(),
		leadQuality: seedLeadQuality(),
		content:     seedContent(),
		alerts:      seedAlerts(),

		users: seedUsers(),
	}
}

/* ---- Reads (lists) ----------------------------------------------------- */

func (m *Memory) Funnel() []domain.FunnelStage      { return m.funnel.list() }
func (m *Memory) KPIs() []domain.KPI                { return m.kpis.list() }
func (m *Memory) Channels() []domain.Channel        { return m.channels.list() }
func (m *Memory) Projects() []domain.Project        { return m.projects.list() }
func (m *Memory) Assets() []domain.Asset            { return m.assets.list() }
func (m *Memory) IGAccounts() []domain.IGAccount    { return m.igAccounts.list() }
func (m *Memory) Handover() []domain.HandoverMetric { return m.handover.list() }
func (m *Memory) Winning() []domain.WinningCampaign { return m.winning.list() }
func (m *Memory) Commands() []domain.Command        { return m.commands.list() }
func (m *Memory) ReasonCodes() []domain.ReasonCode  { return m.reasonCodes.list() }

func (m *Memory) ProjectByID(id string) (domain.Project, error) {
	if p, ok := m.projects.get(id); ok {
		return p, nil
	}
	return domain.Project{}, ErrNotFound
}

/* ---- Reads (singletons) ------------------------------------------------ */

func (m *Memory) Context() domain.Context {
	m.muSingle.RLock()
	defer m.muSingle.RUnlock()
	return m.context
}

func (m *Memory) LeadQuality() domain.LeadQuality {
	m.muSingle.RLock()
	defer m.muSingle.RUnlock()
	return m.leadQuality
}

func (m *Memory) Content() domain.Content {
	m.muSingle.RLock()
	defer m.muSingle.RUnlock()
	return m.content
}

func (m *Memory) Alerts() domain.Alerts {
	m.muSingle.RLock()
	defer m.muSingle.RUnlock()
	return m.alerts
}

/* ---- Writes (lists) ---------------------------------------------------- */

func (m *Memory) CreateFunnel(f domain.FunnelStage) domain.FunnelStage { return m.funnel.create(f) }
func (m *Memory) UpdateFunnel(id string, f domain.FunnelStage) (domain.FunnelStage, bool) {
	return m.funnel.update(id, f)
}
func (m *Memory) DeleteFunnel(id string) bool { return m.funnel.delete(id) }

func (m *Memory) CreateKPI(k domain.KPI) domain.KPI { return m.kpis.create(k) }
func (m *Memory) UpdateKPI(id string, k domain.KPI) (domain.KPI, bool) {
	return m.kpis.update(id, k)
}
func (m *Memory) DeleteKPI(id string) bool { return m.kpis.delete(id) }

func (m *Memory) CreateChannel(c domain.Channel) domain.Channel { return m.channels.create(c) }
func (m *Memory) UpdateChannel(id string, c domain.Channel) (domain.Channel, bool) {
	return m.channels.update(id, c)
}
func (m *Memory) DeleteChannel(id string) bool { return m.channels.delete(id) }

func (m *Memory) CreateProject(p domain.Project) domain.Project { return m.projects.create(p) }
func (m *Memory) UpdateProject(id string, p domain.Project) (domain.Project, bool) {
	return m.projects.update(id, p)
}
func (m *Memory) DeleteProject(id string) bool { return m.projects.delete(id) }

func (m *Memory) CreateAsset(a domain.Asset) domain.Asset { return m.assets.create(a) }
func (m *Memory) UpdateAsset(id string, a domain.Asset) (domain.Asset, bool) {
	return m.assets.update(id, a)
}
func (m *Memory) DeleteAsset(id string) bool { return m.assets.delete(id) }

func (m *Memory) CreateIGAccount(a domain.IGAccount) domain.IGAccount { return m.igAccounts.create(a) }
func (m *Memory) UpdateIGAccount(id string, a domain.IGAccount) (domain.IGAccount, bool) {
	return m.igAccounts.update(id, a)
}
func (m *Memory) DeleteIGAccount(id string) bool { return m.igAccounts.delete(id) }

func (m *Memory) CreateHandover(h domain.HandoverMetric) domain.HandoverMetric {
	return m.handover.create(h)
}
func (m *Memory) UpdateHandover(id string, h domain.HandoverMetric) (domain.HandoverMetric, bool) {
	return m.handover.update(id, h)
}
func (m *Memory) DeleteHandover(id string) bool { return m.handover.delete(id) }

func (m *Memory) CreateWinning(w domain.WinningCampaign) domain.WinningCampaign {
	return m.winning.create(w)
}
func (m *Memory) UpdateWinning(id string, w domain.WinningCampaign) (domain.WinningCampaign, bool) {
	return m.winning.update(id, w)
}
func (m *Memory) DeleteWinning(id string) bool { return m.winning.delete(id) }

func (m *Memory) CreateCommand(c domain.Command) domain.Command { return m.commands.create(c) }
func (m *Memory) UpdateCommand(id string, c domain.Command) (domain.Command, bool) {
	return m.commands.update(id, c)
}
func (m *Memory) DeleteCommand(id string) bool { return m.commands.delete(id) }

func (m *Memory) CreateReasonCode(r domain.ReasonCode) domain.ReasonCode {
	return m.reasonCodes.create(r)
}
func (m *Memory) UpdateReasonCode(id string, r domain.ReasonCode) (domain.ReasonCode, bool) {
	return m.reasonCodes.update(id, r)
}
func (m *Memory) DeleteReasonCode(id string) bool { return m.reasonCodes.delete(id) }

/* ---- Writes (singletons) ----------------------------------------------- */

func (m *Memory) UpdateContext(c domain.Context) domain.Context {
	m.muSingle.Lock()
	defer m.muSingle.Unlock()
	m.context = c
	return m.context
}

func (m *Memory) UpdateLeadQuality(q domain.LeadQuality) domain.LeadQuality {
	m.muSingle.Lock()
	defer m.muSingle.Unlock()
	m.leadQuality = q
	return m.leadQuality
}

func (m *Memory) UpdateContent(c domain.Content) domain.Content {
	m.muSingle.Lock()
	defer m.muSingle.Unlock()
	m.content = c
	return m.content
}

func (m *Memory) UpdateAlerts(a domain.Alerts) domain.Alerts {
	m.muSingle.Lock()
	defer m.muSingle.Unlock()
	m.alerts = a
	return m.alerts
}

/* ---- Users ------------------------------------------------------------- */

func (m *Memory) UserByUsername(username string) (domain.User, bool) {
	u, ok := m.users[username]
	return u, ok
}

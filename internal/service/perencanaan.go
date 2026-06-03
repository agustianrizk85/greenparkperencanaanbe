// Package service holds the business logic of the Perencanaan demand control
// tower: read composition (the derived executive summary), master-data CRUD with
// light validation, and authentication. It depends on the repository and the
// auth session store.
package service

import (
	"errors"
	"math"
	"strings"

	"greenpark/perencanaan/internal/auth"
	"greenpark/perencanaan/internal/domain"
	"greenpark/perencanaan/internal/repository"
)

// Sentinel errors mapped to HTTP status codes by the transport layer.
var (
	ErrNotFound           = errors.New("not found")
	ErrValidation         = errors.New("validation failed")
	ErrInvalidCredentials = errors.New("invalid username or password")
)

// Service is the application's use-case layer.
type Service struct {
	repo     *repository.Memory
	sessions *auth.SessionStore
}

// New builds a Service from the store and session manager.
func New(repo *repository.Memory, sessions *auth.SessionStore) *Service {
	return &Service{repo: repo, sessions: sessions}
}

/* ---- Dashboard composition -------------------------------------------- */

// Dashboard assembles the full payload including the derived summary.
func (s *Service) Dashboard() domain.Dashboard {
	return domain.Dashboard{
		Context:     s.repo.Context(),
		Funnel:      s.repo.Funnel(),
		KPIs:        s.repo.KPIs(),
		LeadQuality: s.repo.LeadQuality(),
		Handover:    s.repo.Handover(),
		Channels:    s.repo.Channels(),
		Projects:    s.repo.Projects(),
		Assets:      s.repo.Assets(),
		IGAccounts:  s.repo.IGAccounts(),
		Winning:     s.repo.Winning(),
		Content:     s.repo.Content(),
		Commands:    s.repo.Commands(),
		Alerts:      s.repo.Alerts(),
		ReasonCodes: s.repo.ReasonCodes(),
		Summary:     s.Summary(),
	}
}

// Summary computes the executive KPIs from context, channels, projects and
// alerts. Volume and spend figures are derived from the channel matrix so the
// headline numbers always reconcile with the channels table.
func (s *Service) Summary() domain.Summary {
	ctx := s.repo.Context()
	channels := s.repo.Channels()
	projects := s.repo.Projects()
	alerts := s.repo.Alerts()
	commands := s.repo.Commands()

	var totalLeads, totalMQL int
	var totalSpend int64
	for _, c := range channels {
		totalLeads += c.Leads
		totalMQL += c.MQL
		totalSpend += c.Spend
	}
	totalBooking := 0
	for _, p := range projects {
		totalBooking += p.Booking
	}
	openCommands := 0
	for _, c := range commands {
		if c.Status != "done" {
			openCommands++
		}
	}

	mqlRate := 0.0
	if totalLeads > 0 {
		mqlRate = math.Round(float64(totalMQL)/float64(totalLeads)*1000) / 10
	}
	var cpl int64
	if totalLeads > 0 {
		cpl = totalSpend / int64(totalLeads)
	}
	var costPerBooking int64
	if totalBooking > 0 {
		costPerBooking = totalSpend / int64(totalBooking)
	}
	achievement := 0
	if ctx.Goal > 0 {
		achievement = int(math.Round(float64(ctx.BookingYTD) / float64(ctx.Goal) * 100))
	}

	return domain.Summary{
		Goal:           ctx.Goal,
		BookingYTD:     ctx.BookingYTD,
		Achievement:    achievement,
		TotalLeads:     totalLeads,
		TotalMQL:       totalMQL,
		MQLRate:        mqlRate,
		TotalSpend:     totalSpend,
		CPL:            cpl,
		TotalBooking:   totalBooking,
		CostPerBooking: costPerBooking,
		RedAlerts:      len(alerts.Red),
		OpenCommands:   openCommands,
	}
}

func required(value string) error {
	if strings.TrimSpace(value) == "" {
		return ErrValidation
	}
	return nil
}

func notFoundIf(ok bool) error {
	if !ok {
		return ErrNotFound
	}
	return nil
}

/* ---- Funnel ------------------------------------------------------------ */

func (s *Service) Funnel() []domain.FunnelStage { return s.repo.Funnel() }

func (s *Service) CreateFunnel(v domain.FunnelStage) (domain.FunnelStage, error) {
	if err := required(v.Key); err != nil {
		return domain.FunnelStage{}, err
	}
	return s.repo.CreateFunnel(v), nil
}

func (s *Service) UpdateFunnel(id string, v domain.FunnelStage) (domain.FunnelStage, error) {
	if err := required(v.Key); err != nil {
		return domain.FunnelStage{}, err
	}
	out, ok := s.repo.UpdateFunnel(id, v)
	if !ok {
		return domain.FunnelStage{}, ErrNotFound
	}
	return out, nil
}

func (s *Service) DeleteFunnel(id string) error { return notFoundIf(s.repo.DeleteFunnel(id)) }

/* ---- KPIs -------------------------------------------------------------- */

func (s *Service) KPIs() []domain.KPI { return s.repo.KPIs() }

func (s *Service) CreateKPI(v domain.KPI) (domain.KPI, error) {
	if err := required(v.Label); err != nil {
		return domain.KPI{}, err
	}
	if v.Trend == nil {
		v.Trend = []float64{}
	}
	return s.repo.CreateKPI(v), nil
}

func (s *Service) UpdateKPI(id string, v domain.KPI) (domain.KPI, error) {
	if err := required(v.Label); err != nil {
		return domain.KPI{}, err
	}
	if v.Trend == nil {
		v.Trend = []float64{}
	}
	out, ok := s.repo.UpdateKPI(id, v)
	if !ok {
		return domain.KPI{}, ErrNotFound
	}
	return out, nil
}

func (s *Service) DeleteKPI(id string) error { return notFoundIf(s.repo.DeleteKPI(id)) }

/* ---- Channels ---------------------------------------------------------- */

func (s *Service) Channels() []domain.Channel { return s.repo.Channels() }

func (s *Service) CreateChannel(v domain.Channel) (domain.Channel, error) {
	if err := required(v.Name); err != nil {
		return domain.Channel{}, err
	}
	return s.repo.CreateChannel(v), nil
}

func (s *Service) UpdateChannel(id string, v domain.Channel) (domain.Channel, error) {
	if err := required(v.Name); err != nil {
		return domain.Channel{}, err
	}
	out, ok := s.repo.UpdateChannel(id, v)
	if !ok {
		return domain.Channel{}, ErrNotFound
	}
	return out, nil
}

func (s *Service) DeleteChannel(id string) error { return notFoundIf(s.repo.DeleteChannel(id)) }

/* ---- Projects ---------------------------------------------------------- */

func (s *Service) Projects() []domain.Project { return s.repo.Projects() }

func (s *Service) ProjectByID(id string) (domain.Project, error) {
	p, err := s.repo.ProjectByID(id)
	if err != nil {
		return domain.Project{}, ErrNotFound
	}
	return p, nil
}

func (s *Service) CreateProject(v domain.Project) (domain.Project, error) {
	if err := required(v.Name); err != nil {
		return domain.Project{}, err
	}
	return s.repo.CreateProject(v), nil
}

func (s *Service) UpdateProject(id string, v domain.Project) (domain.Project, error) {
	if err := required(v.Name); err != nil {
		return domain.Project{}, err
	}
	out, ok := s.repo.UpdateProject(id, v)
	if !ok {
		return domain.Project{}, ErrNotFound
	}
	return out, nil
}

func (s *Service) DeleteProject(id string) error { return notFoundIf(s.repo.DeleteProject(id)) }

/* ---- Assets ------------------------------------------------------------ */

func (s *Service) Assets() []domain.Asset { return s.repo.Assets() }

func (s *Service) CreateAsset(v domain.Asset) (domain.Asset, error) {
	if err := required(v.Type); err != nil {
		return domain.Asset{}, err
	}
	return s.repo.CreateAsset(v), nil
}

func (s *Service) UpdateAsset(id string, v domain.Asset) (domain.Asset, error) {
	if err := required(v.Type); err != nil {
		return domain.Asset{}, err
	}
	out, ok := s.repo.UpdateAsset(id, v)
	if !ok {
		return domain.Asset{}, ErrNotFound
	}
	return out, nil
}

func (s *Service) DeleteAsset(id string) error { return notFoundIf(s.repo.DeleteAsset(id)) }

/* ---- IG accounts ------------------------------------------------------- */

func (s *Service) IGAccounts() []domain.IGAccount { return s.repo.IGAccounts() }

func (s *Service) CreateIGAccount(v domain.IGAccount) (domain.IGAccount, error) {
	if err := required(v.Handle); err != nil {
		return domain.IGAccount{}, err
	}
	return s.repo.CreateIGAccount(v), nil
}

func (s *Service) UpdateIGAccount(id string, v domain.IGAccount) (domain.IGAccount, error) {
	if err := required(v.Handle); err != nil {
		return domain.IGAccount{}, err
	}
	out, ok := s.repo.UpdateIGAccount(id, v)
	if !ok {
		return domain.IGAccount{}, ErrNotFound
	}
	return out, nil
}

func (s *Service) DeleteIGAccount(id string) error { return notFoundIf(s.repo.DeleteIGAccount(id)) }

/* ---- Handover metrics -------------------------------------------------- */

func (s *Service) Handover() []domain.HandoverMetric { return s.repo.Handover() }

func (s *Service) CreateHandover(v domain.HandoverMetric) (domain.HandoverMetric, error) {
	if err := required(v.Label); err != nil {
		return domain.HandoverMetric{}, err
	}
	return s.repo.CreateHandover(v), nil
}

func (s *Service) UpdateHandover(id string, v domain.HandoverMetric) (domain.HandoverMetric, error) {
	if err := required(v.Label); err != nil {
		return domain.HandoverMetric{}, err
	}
	out, ok := s.repo.UpdateHandover(id, v)
	if !ok {
		return domain.HandoverMetric{}, ErrNotFound
	}
	return out, nil
}

func (s *Service) DeleteHandover(id string) error { return notFoundIf(s.repo.DeleteHandover(id)) }

/* ---- Winning campaigns ------------------------------------------------- */

func (s *Service) Winning() []domain.WinningCampaign { return s.repo.Winning() }

func (s *Service) CreateWinning(v domain.WinningCampaign) (domain.WinningCampaign, error) {
	if err := required(v.Name); err != nil {
		return domain.WinningCampaign{}, err
	}
	return s.repo.CreateWinning(v), nil
}

func (s *Service) UpdateWinning(id string, v domain.WinningCampaign) (domain.WinningCampaign, error) {
	if err := required(v.Name); err != nil {
		return domain.WinningCampaign{}, err
	}
	out, ok := s.repo.UpdateWinning(id, v)
	if !ok {
		return domain.WinningCampaign{}, ErrNotFound
	}
	return out, nil
}

func (s *Service) DeleteWinning(id string) error { return notFoundIf(s.repo.DeleteWinning(id)) }

/* ---- Commands ---------------------------------------------------------- */

func (s *Service) Commands() []domain.Command { return s.repo.Commands() }

func (s *Service) CreateCommand(v domain.Command) (domain.Command, error) {
	if err := required(v.Issue); err != nil {
		return domain.Command{}, err
	}
	return s.repo.CreateCommand(v), nil
}

func (s *Service) UpdateCommand(id string, v domain.Command) (domain.Command, error) {
	if err := required(v.Issue); err != nil {
		return domain.Command{}, err
	}
	out, ok := s.repo.UpdateCommand(id, v)
	if !ok {
		return domain.Command{}, ErrNotFound
	}
	return out, nil
}

func (s *Service) DeleteCommand(id string) error { return notFoundIf(s.repo.DeleteCommand(id)) }

/* ---- Reason codes ------------------------------------------------------ */

func (s *Service) ReasonCodes() []domain.ReasonCode { return s.repo.ReasonCodes() }

func (s *Service) CreateReasonCode(v domain.ReasonCode) (domain.ReasonCode, error) {
	if err := required(v.Code); err != nil {
		return domain.ReasonCode{}, err
	}
	return s.repo.CreateReasonCode(v), nil
}

func (s *Service) UpdateReasonCode(id string, v domain.ReasonCode) (domain.ReasonCode, error) {
	if err := required(v.Code); err != nil {
		return domain.ReasonCode{}, err
	}
	out, ok := s.repo.UpdateReasonCode(id, v)
	if !ok {
		return domain.ReasonCode{}, ErrNotFound
	}
	return out, nil
}

func (s *Service) DeleteReasonCode(id string) error { return notFoundIf(s.repo.DeleteReasonCode(id)) }

/* ---- Singletons -------------------------------------------------------- */

func (s *Service) Context() domain.Context { return s.repo.Context() }

func (s *Service) UpdateContext(c domain.Context) (domain.Context, error) {
	return s.repo.UpdateContext(c), nil
}

func (s *Service) LeadQuality() domain.LeadQuality { return s.repo.LeadQuality() }

func (s *Service) UpdateLeadQuality(q domain.LeadQuality) (domain.LeadQuality, error) {
	if q.Breakdown == nil {
		q.Breakdown = []domain.LeadBreakdown{}
	}
	if q.Stats == nil {
		q.Stats = []domain.LeadStat{}
	}
	return s.repo.UpdateLeadQuality(q), nil
}

func (s *Service) Content() domain.Content { return s.repo.Content() }

func (s *Service) UpdateContent(c domain.Content) (domain.Content, error) {
	return s.repo.UpdateContent(c), nil
}

func (s *Service) Alerts() domain.Alerts { return s.repo.Alerts() }

func (s *Service) UpdateAlerts(a domain.Alerts) (domain.Alerts, error) {
	if a.Red == nil {
		a.Red = []string{}
	}
	if a.Yellow == nil {
		a.Yellow = []string{}
	}
	if a.Green == nil {
		a.Green = []string{}
	}
	return s.repo.UpdateAlerts(a), nil
}

/* ---- Authentication ---------------------------------------------------- */

// Login verifies credentials and issues a bearer token.
func (s *Service) Login(username, password string) (string, domain.User, error) {
	u, ok := s.repo.UserByUsername(username)
	if !ok || !auth.Verify(password, u.PasswordHash, u.Salt) {
		return "", domain.User{}, ErrInvalidCredentials
	}
	token, err := s.sessions.Issue(u.Username)
	if err != nil {
		return "", domain.User{}, err
	}
	return token, u, nil
}

// UserByToken resolves the user behind a valid session token.
func (s *Service) UserByToken(token string) (domain.User, bool) {
	username, ok := s.sessions.Resolve(token)
	if !ok {
		return domain.User{}, false
	}
	return s.repo.UserByUsername(username)
}

// Logout revokes a session token.
func (s *Service) Logout(token string) { s.sessions.Revoke(token) }

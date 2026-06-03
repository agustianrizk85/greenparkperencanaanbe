// Package domain holds the core business entities of the Perencanaan (planning /
// demand) control tower — the "Qualified Demand Control Tower" consumed by the
// Greenpark CEO war-room. These types are the single source of truth for the
// data shape and carry no dependency on transport or storage concerns.
package domain

// Status is the common health indicator used by KPIs and handover metrics.
//
//	good = on/above target · warn = drifting · bad = off target
type Status string

const (
	StatusGood Status = "good"
	StatusWarn Status = "warn"
	StatusBad  Status = "bad"
)

// FunnelStage is one step of the demand funnel (Impression → Cash-In). Stages
// are ordered; each carries the owning department.
type FunnelStage struct {
	ID    string `json:"id"`
	No    int    `json:"no"`    // stage order (1 = top of funnel)
	Key   string `json:"key"`   // stage label, e.g. "Leads", "MQL"
	Value int    `json:"value"` // count at this stage
	Owner string `json:"owner"` // accountable department
}

// KPI is a North-Star indicator with its target, gap and 6-point spark trend.
type KPI struct {
	ID     string    `json:"id"`
	Label  string    `json:"label"`
	Value  string    `json:"value"`            // pre-formatted display value
	Suffix string    `json:"suffix,omitempty"` // optional unit suffix (e.g. "/100")
	Target string    `json:"target"`
	Gap    string    `json:"gap"` // delta vs target, pre-formatted
	Trend  []float64 `json:"trend"`
	Status Status    `json:"status"`
	Note   string    `json:"note"`
}

// Channel is an acquisition channel scored on spend, volume and efficiency.
type Channel struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Group  string `json:"group"` // Paid | Owned | Trust | Offline
	Spend  int64  `json:"spend"` // rupiah
	Leads  int    `json:"leads"`
	MQL    int    `json:"mql"`
	CPL    int64  `json:"cpl"`    // cost per lead (rupiah)
	CPQL   int64  `json:"cpql"`   // cost per qualified lead (rupiah)
	ROI    string `json:"roi"`    // pre-formatted, e.g. "4.8×"
	Status string `json:"status"` // scale | optimize | pause | test
}

// Project is a housing project ranked by demand strength and sales readiness.
type Project struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	IG        string `json:"ig"`        // Instagram handle
	Demand    int    `json:"demand"`    // demand score 0..100
	Readiness int    `json:"readiness"` // sales readiness score 0..100
	Leads     int    `json:"leads"`
	MQL       int    `json:"mql"`
	Booking   int    `json:"booking"`
}

// Asset is a non-Instagram digital property in the marketing registry.
type Asset struct {
	ID     string `json:"id"`
	Type   string `json:"type"` // Website | TikTok | YouTube | Google Business
	Handle string `json:"handle"`
	Health int    `json:"health"` // health score 0..100
	Active bool   `json:"active"`
	Note   string `json:"note"`
}

// IGAccount is a per-project Instagram account with activity recency.
type IGAccount struct {
	ID     string `json:"id"`
	Handle string `json:"handle"`
	Health int    `json:"health"`
	Active bool   `json:"active"`
	Days   int    `json:"days"` // days since last post
}

// HandoverMetric is a single MQL → SAL handover quality measure.
type HandoverMetric struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Value  string `json:"value"`
	Status Status `json:"status"`
}

// WinningCampaign is a campaign that meets the "winning" criteria threshold.
type WinningCampaign struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Project  string `json:"project"`
	Channel  string `json:"channel"`
	Criteria int    `json:"criteria"` // criteria met (out of the winning checklist)
	CPL      string `json:"cpl"`
	MQL      string `json:"mql"`
	Booking  int    `json:"booking"`
}

// Command is a CEO directive issued in response to a detected demand leak.
type Command struct {
	ID       string `json:"id"`
	Issue    string `json:"issue"`
	Cause    string `json:"cause"`
	Impact   string `json:"impact"`
	Command  string `json:"command"`
	PIC      string `json:"pic"`
	Deadline string `json:"deadline"`
	Expected string `json:"expected"`
	Status   string `json:"status"` // open | progress | done
}

// ReasonCode classifies why leads drop between funnel layers.
type ReasonCode struct {
	ID    string `json:"id"`
	Code  string `json:"code"`
	Layer string `json:"layer"` // funnel layer, e.g. "Leads→CV"
	Label string `json:"label"`
	Count int    `json:"count"`
}

/* ---- Singletons -------------------------------------------------------- */

// Context is the top-line period context shown in the header.
type Context struct {
	Period       string `json:"period"`
	Updated      string `json:"updated"`
	Goal         int    `json:"goal"`       // annual booking goal (units)
	BookingYTD   int    `json:"bookingYTD"` // bookings achieved year-to-date
	Completeness int    `json:"completeness"`
	Spend        int64  `json:"spend"` // total marketing spend (rupiah)
}

// LeadBreakdown is one MQL quality tier (hot/warm/nurture/low).
type LeadBreakdown struct {
	Label string `json:"label"`
	Value int    `json:"value"`
	Color string `json:"color"` // hot | warm | nurture | low
}

// LeadStat is a headline lead-quality ratio.
type LeadStat struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// LeadQuality aggregates MQL quality, top/bottom sources and projects.
type LeadQuality struct {
	Breakdown     []LeadBreakdown `json:"breakdown"`
	Stats         []LeadStat      `json:"stats"`
	TopSource     string          `json:"topSource"`
	BottomSource  string          `json:"bottomSource"`
	TopProject    string          `json:"topProject"`
	BottomProject string          `json:"bottomProject"`
}

// ContentItem is the best- or worst-performing creative.
type ContentItem struct {
	Name    string `json:"name"`
	Account string `json:"account"`
	Metric  string `json:"metric"`
}

// Content summarises creative health: best/worst piece and queue counts.
type Content struct {
	Best   ContentItem `json:"best"`
	Worst  ContentItem `json:"worst"`
	Rework int         `json:"rework"`
	Pause  int         `json:"pause"`
}

// Alerts groups operational alerts by severity.
type Alerts struct {
	Red    []string `json:"red"`
	Yellow []string `json:"yellow"`
	Green  []string `json:"green"`
}

// Summary holds the executive KPIs derived from the rest of the data set.
type Summary struct {
	Goal           int     `json:"goal"`
	BookingYTD     int     `json:"bookingYTD"`
	Achievement    int     `json:"achievement"` // bookingYTD / goal %
	TotalLeads     int     `json:"totalLeads"`
	TotalMQL       int     `json:"totalMQL"`
	MQLRate        float64 `json:"mqlRate"`
	TotalSpend     int64   `json:"totalSpend"`
	CPL            int64   `json:"cpl"`
	TotalBooking   int     `json:"totalBooking"`
	CostPerBooking int64   `json:"costPerBooking"`
	RedAlerts      int     `json:"redAlerts"`
	OpenCommands   int     `json:"openCommands"`
}

// Dashboard is the full payload consumed by the front-end in a single call.
type Dashboard struct {
	Context     Context           `json:"context"`
	Funnel      []FunnelStage     `json:"funnel"`
	KPIs        []KPI             `json:"kpis"`
	LeadQuality LeadQuality       `json:"leadQuality"`
	Handover    []HandoverMetric  `json:"handover"`
	Channels    []Channel         `json:"channels"`
	Projects    []Project         `json:"projects"`
	Assets      []Asset           `json:"assets"`
	IGAccounts  []IGAccount       `json:"igAccounts"`
	Winning     []WinningCampaign `json:"winning"`
	Content     Content           `json:"content"`
	Commands    []Command         `json:"commands"`
	Alerts      Alerts            `json:"alerts"`
	ReasonCodes []ReasonCode      `json:"reasonCodes"`
	Summary     Summary           `json:"summary"`
}

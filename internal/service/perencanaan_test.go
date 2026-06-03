package service

import (
	"testing"
	"time"

	"greenpark/perencanaan/internal/auth"
	"greenpark/perencanaan/internal/domain"
	"greenpark/perencanaan/internal/repository"
)

func newService() *Service {
	return New(repository.NewMemory(), auth.NewSessionStore(time.Hour))
}

func TestSummaryDerivedFromSeed(t *testing.T) {
	s := newService()
	sum := s.Summary()

	// Totals are derived from the channel matrix: leads sum to 3420, MQL to 1180.
	if sum.TotalLeads != 3420 {
		t.Errorf("TotalLeads = %d, want 3420", sum.TotalLeads)
	}
	if sum.TotalMQL != 1180 {
		t.Errorf("TotalMQL = %d, want 1180", sum.TotalMQL)
	}
	// MQL rate = 1180/3420 = 34.5%.
	if sum.MQLRate != 34.5 {
		t.Errorf("MQLRate = %v, want 34.5", sum.MQLRate)
	}
	// Total spend = 720+410+360+120+110 (juta dasar) = 1.72 M.
	if sum.TotalSpend != 1_720_000_000 {
		t.Errorf("TotalSpend = %d, want 1720000000", sum.TotalSpend)
	}
	// Achievement = bookingYTD(312) / goal(500) = 62%.
	if sum.Achievement != 62 {
		t.Errorf("Achievement = %d, want 62", sum.Achievement)
	}
	// Bookings sum across the 10 projects = 93.
	if sum.TotalBooking != 93 {
		t.Errorf("TotalBooking = %d, want 93", sum.TotalBooking)
	}
	// 4 of 5 seeded commands are not done (one is "progress", which still counts as open).
	if sum.OpenCommands != 5 {
		t.Errorf("OpenCommands = %d, want 5", sum.OpenCommands)
	}
	if sum.RedAlerts != 3 {
		t.Errorf("RedAlerts = %d, want 3", sum.RedAlerts)
	}
}

func TestChannelCRUDValidation(t *testing.T) {
	s := newService()

	if _, err := s.CreateChannel(channelInput("")); err == nil {
		t.Fatal("expected validation error for empty channel name")
	}
	created, err := s.CreateChannel(channelInput("Test Channel"))
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}
	if created.ID == "" {
		t.Fatal("created channel has empty ID")
	}

	created.Name = "Updated"
	updated, err := s.UpdateChannel(created.ID, created)
	if err != nil {
		t.Fatalf("UpdateChannel: %v", err)
	}
	if updated.Name != "Updated" {
		t.Errorf("Name = %q, want Updated", updated.Name)
	}

	if err := s.DeleteChannel(created.ID); err != nil {
		t.Fatalf("DeleteChannel: %v", err)
	}
	if err := s.DeleteChannel(created.ID); err == nil {
		t.Fatal("expected not-found deleting already-deleted channel")
	}
}

func TestLoginIssuesToken(t *testing.T) {
	s := newService()
	token, user, err := s.Login("admin", "admin123")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if token == "" {
		t.Fatal("empty token")
	}
	if _, ok := s.UserByToken(token); !ok {
		t.Fatal("token did not resolve to a user")
	}
	if user.Username != "admin" {
		t.Errorf("user = %q, want admin", user.Username)
	}
	if _, _, err := s.Login("admin", "wrong"); err == nil {
		t.Fatal("expected error for wrong password")
	}
}

func channelInput(name string) domain.Channel {
	return domain.Channel{Name: name, Group: "Paid", Spend: 1000, Leads: 10, MQL: 4, ROI: "2×", Status: "test"}
}

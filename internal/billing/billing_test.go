package billing

import (
	"testing"
	"time"
)

func TestQuotaSnapshotMonthly(t *testing.T) {
	now := time.Date(2026, time.July, 25, 0, 0, 0, 0, time.UTC)

	tests := map[string]struct {
		user      *copilotInternalUser
		year      int
		month     int
		wantOK    bool
		wantTotal float64
		wantLogin string
	}{
		"current cycle with premium_interactions snapshot": {
			user: &copilotInternalUser{
				Login: "octocat",
				QuotaSnapshots: map[string]quotaSnapshot{
					"premium_interactions": {CreditsUsed: 12.5, Entitlement: 300, Remaining: 287.5},
				},
			},
			year:      2026,
			month:     7,
			wantOK:    true,
			wantTotal: 12.5,
			wantLogin: "octocat",
		},
		"historical month is not serviceable from a snapshot": {
			user: &copilotInternalUser{
				Login: "octocat",
				QuotaSnapshots: map[string]quotaSnapshot{
					"premium_interactions": {CreditsUsed: 12.5},
				},
			},
			year:   2026,
			month:  6,
			wantOK: false,
		},
		"missing premium_interactions snapshot": {
			user: &copilotInternalUser{
				Login:          "octocat",
				QuotaSnapshots: map[string]quotaSnapshot{},
			},
			year:   2026,
			month:  7,
			wantOK: false,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, ok := quotaSnapshotMonthly(tt.user, tt.year, tt.month, now)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if got.TotalAIC != tt.wantTotal {
				t.Errorf("TotalAIC = %v, want %v", got.TotalAIC, tt.wantTotal)
			}
			if got.Login != tt.wantLogin {
				t.Errorf("Login = %q, want %q", got.Login, tt.wantLogin)
			}
			if got.Year != tt.year || got.Month != tt.month {
				t.Errorf("Year/Month = %d/%d, want %d/%d", got.Year, got.Month, tt.year, tt.month)
			}
			if got.ByModel != nil {
				t.Errorf("ByModel = %v, want nil", got.ByModel)
			}
		})
	}
}

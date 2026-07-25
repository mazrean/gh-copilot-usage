// Package billing fetches monthly GitHub AI credit (AIC) usage via the
// authenticated gh CLI session, using go-gh so no token handling is needed.
package billing

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
)

const apiVersion = "2026-03-10"

// ModelUsage is the net AIC quantity consumed by one model/sku pair.
type ModelUsage struct {
	Model       string  `json:"model"`
	SKU         string  `json:"sku"`
	NetQuantity float64 `json:"netQuantity"`
	NetAmount   float64 `json:"netAmount"`
	UnitType    string  `json:"unitType"`
}

// Monthly is the aggregated monthly AIC usage report for the authenticated user.
// ByModel is nil when the report was approximated from a quota snapshot (see
// quotaSnapshotMonthly), which doesn't expose a per-model breakdown.
type Monthly struct {
	Login    string       `json:"login"`
	Year     int          `json:"year"`
	Month    int          `json:"month"`
	TotalAIC float64      `json:"totalAIC"`
	ByModel  []ModelUsage `json:"byModel"`
}

type usageItem struct {
	Product     string  `json:"product"`
	SKU         string  `json:"sku"`
	Model       string  `json:"model"`
	UnitType    string  `json:"unitType"`
	NetQuantity float64 `json:"netQuantity"`
	NetAmount   float64 `json:"netAmount"`
}

type usageResponse struct {
	UsageItems []usageItem `json:"usageItems"`
}

// quotaSnapshot is one entry of copilot_internal/user's quota_snapshots map.
// Following GitHub's June 2026 migration to AI-credit billing, the legacy
// "premium_interactions" quota reports actual AI Credit units even though
// the field name predates that change.
type quotaSnapshot struct {
	CreditsUsed float64 `json:"credits_used"`
	Entitlement float64 `json:"entitlement"`
	Remaining   float64 `json:"remaining"`
	Unlimited   bool    `json:"unlimited"`
}

// copilotInternalUser is the response of GET copilot_internal/user, the same
// endpoint the Copilot CLI/IDE clients use to resolve plan and quota info.
// Unlike users/{login}/settings/billing/ai_credit/usage (an individual
// billing-settings endpoint), it is readable by any authenticated Copilot
// user regardless of whether their seat is billed personally or through a
// Business/Enterprise org.
type copilotInternalUser struct {
	Login          string                   `json:"login"`
	CopilotPlan    string                   `json:"copilot_plan"`
	QuotaSnapshots map[string]quotaSnapshot `json:"quota_snapshots"`
}

// Client fetches AIC usage using the gh CLI's authenticated REST client.
type Client struct {
	rest *api.RESTClient
}

// NewClient builds a Client from gh's default host/auth configuration.
// It returns an error if gh has no usable authentication configured.
func NewClient() (*Client, error) {
	rest, err := api.NewRESTClient(api.ClientOptions{
		Headers: map[string]string{
			"X-GitHub-Api-Version": apiVersion,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("gh REST client: %w", err)
	}
	return &Client{rest: rest}, nil
}

// Monthly fetches the AIC usage report for the given year/month for the
// currently authenticated gh user. It prefers the detailed
// users/{login}/settings/billing/ai_credit/usage report (per-model
// breakdown), which is only available for individually-billed seats. When
// that report is unavailable — as for Business/Enterprise seats, whose
// billing lives at the org/enterprise level — it falls back to the
// current-cycle total from the copilot_internal/user quota snapshot, the
// same source the Copilot CLI itself relies on.
func (c *Client) Monthly(ctx context.Context, year, month int) (*Monthly, error) {
	intUser, err := c.fetchCopilotInternalUser(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve copilot user: %w", err)
	}

	report, reportErr := c.billingReport(ctx, intUser.Login, year, month)
	if reportErr == nil {
		return report, nil
	}

	if fallback, ok := quotaSnapshotMonthly(intUser, year, month, time.Now()); ok {
		return fallback, nil
	}
	return nil, fmt.Errorf("fetch ai_credit usage: %w", reportErr)
}

func (c *Client) fetchCopilotInternalUser(ctx context.Context) (*copilotInternalUser, error) {
	var u copilotInternalUser
	if err := c.rest.DoWithContext(ctx, "GET", "copilot_internal/user", nil, &u); err != nil {
		return nil, err
	}
	return &u, nil
}

func (c *Client) billingReport(ctx context.Context, login string, year, month int) (*Monthly, error) {
	path := fmt.Sprintf("users/%s/settings/billing/ai_credit/usage?year=%d&month=%d",
		login, year, month)
	var resp usageResponse
	if err := c.rest.DoWithContext(ctx, "GET", path, nil, &resp); err != nil {
		return nil, err
	}

	byModel := map[string]*ModelUsage{}
	var total float64
	for _, it := range resp.UsageItems {
		total += it.NetQuantity
		key := it.Model
		if key == "" {
			key = it.SKU
		}
		mu, ok := byModel[key]
		if !ok {
			mu = &ModelUsage{Model: it.Model, SKU: it.SKU, UnitType: it.UnitType}
			byModel[key] = mu
		}
		mu.NetQuantity += it.NetQuantity
		mu.NetAmount += it.NetAmount
	}

	out := &Monthly{
		Login:    login,
		Year:     year,
		Month:    month,
		TotalAIC: total,
	}
	for _, mu := range byModel {
		out.ByModel = append(out.ByModel, *mu)
	}
	sort.Slice(out.ByModel, func(i, j int) bool {
		return out.ByModel[i].NetQuantity > out.ByModel[j].NetQuantity
	})
	return out, nil
}

// quotaSnapshotMonthly approximates the requested month's AIC usage from a
// copilot_internal/user quota snapshot. The snapshot only ever reflects the
// current billing cycle, so it can't serve a historical year/month request —
// ok is false in that case, and the caller should surface the original
// billing-report error instead.
func quotaSnapshotMonthly(u *copilotInternalUser, year, month int, now time.Time) (*Monthly, bool) {
	if year != now.Year() || month != int(now.Month()) {
		return nil, false
	}
	snap, ok := u.QuotaSnapshots["premium_interactions"]
	if !ok {
		return nil, false
	}
	return &Monthly{
		Login:    u.Login,
		Year:     year,
		Month:    month,
		TotalAIC: snap.CreditsUsed,
	}, true
}

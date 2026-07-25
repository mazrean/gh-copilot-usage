// Package billing fetches monthly GitHub AI credit (AIC) usage via the
// authenticated gh CLI session, using go-gh so no token handling is needed.
package billing

import (
	"context"
	"fmt"
	"sort"

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

type userResponse struct {
	Login string `json:"login"`
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
// currently authenticated gh user.
func (c *Client) Monthly(ctx context.Context, year, month int) (*Monthly, error) {
	var user userResponse
	if err := c.rest.DoWithContext(ctx, "GET", "user", nil, &user); err != nil {
		return nil, fmt.Errorf("resolve authenticated user: %w", err)
	}

	path := fmt.Sprintf("users/%s/settings/billing/ai_credit/usage?year=%d&month=%d",
		user.Login, year, month)
	var resp usageResponse
	if err := c.rest.DoWithContext(ctx, "GET", path, nil, &resp); err != nil {
		return nil, fmt.Errorf("fetch ai_credit usage: %w", err)
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
		Login:    user.Login,
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

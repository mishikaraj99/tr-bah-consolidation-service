package legacy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"traya-bah-service/internal/common"
)

// DefaultShopfloWalletType is config.js DEFAULT_SHOPFLO_WALLET_TYPE.
const DefaultShopfloWalletType = "REWARDS"

// ShopfloClient talks to the Shopflo wallet. Note the upstream env-var typo (ENPOINT) is preserved
// in setup.Config; the base URL must end with "/".
type ShopfloClient struct {
	HTTP       *http.Client
	Endpoint   string
	IssuerID   string
	MerchantID string
	APIKey     string
	Log        *slog.Logger
}

// Wallet is the Shopflo wallet read response.
type Wallet struct {
	TotalWalletBalance float64        `json:"total_wallet_balance"`
	Raw                map[string]any `json:"-"`
}

func (c *ShopfloClient) configured() bool {
	return c != nil && c.Endpoint != "" && c.IssuerID != "" && c.MerchantID != ""
}

func (c *ShopfloClient) walletURL(suffix string) string {
	return fmt.Sprintf("%sissuer/%s/merchant/%s/user-wallet%s", c.Endpoint, c.IssuerID, c.MerchantID, suffix)
}

func (c *ShopfloClient) do(ctx context.Context, method, url string, body any) (int, []byte, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, rdr)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", c.APIKey) // raw key, no scheme — matches api-server
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	return resp.StatusCode, out, err
}

// NormalizePhone prefixes +91 unless the number already carries a country code.
func NormalizePhone(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if strings.HasPrefix(p, "+") {
		return p
	}
	if strings.HasPrefix(p, "91") && len(p) > 10 {
		return "+" + p
	}
	return "+91" + p
}

// Read fetches the wallet. 404 → BadRequest("No record found for this user").
func (c *ShopfloClient) Read(ctx context.Context, phone string) (*Wallet, error) {
	if !c.configured() {
		return nil, common.Internal("Shopflo wallet is not configured")
	}
	status, body, err := c.do(ctx, http.MethodGet, c.walletURL("?phone-number="+url.QueryEscape(NormalizePhone(phone))), nil)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, common.BadRequest("No record found for this user")
	}
	if status < 200 || status >= 300 {
		return nil, common.Internal("Shopflo wallet read failed")
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	w := &Wallet{Raw: raw}
	if v, ok := raw["total_wallet_balance"].(float64); ok {
		w.TotalWalletBalance = v
	}
	return w, nil
}

// EnsureWallet reads the wallet and creates it on 404 (api-server getShopfloWalletTransactions).
func (c *ShopfloClient) EnsureWallet(ctx context.Context, phone string) error {
	if !c.configured() {
		return nil
	}
	status, _, err := c.do(ctx, http.MethodGet, c.walletURL("?phone-number="+url.QueryEscape(NormalizePhone(phone))), nil)
	if err != nil {
		return err
	}
	if status != http.StatusNotFound {
		return nil
	}
	status, _, err = c.do(ctx, http.MethodPost, c.walletURL(""), map[string]any{
		"oid": phone, "initial_balance": 0, "wallet_type": DefaultShopfloWalletType,
	})
	if err != nil {
		return err
	}
	if status == http.StatusNotFound || status == http.StatusBadRequest {
		return nil // swallowed upstream
	}
	return nil
}

// Credit mirrors createShopfloCreditRewardTransaction (amount = coins/10).
func (c *ShopfloClient) Credit(ctx context.Context, coins int, expiryEpochMS int64, referenceID, phone string) error {
	if !c.configured() {
		return nil
	}
	status, _, err := c.do(ctx, http.MethodPost, c.walletURL("/credit-transaction"), map[string]any{
		"oid": phone, "reference_id": referenceID, "event_type": "REFERRED", "reference": "REFERRAL",
		"amount": float64(coins) / 10, "expiry_at": expiryEpochMS,
	})
	if err != nil {
		return err
	}
	if status == http.StatusNotFound || status == http.StatusBadRequest {
		return nil // swallowed upstream
	}
	return nil
}

// Debit mirrors createShopfloDebitRewardTransaction.
func (c *ShopfloClient) Debit(ctx context.Context, coins int, referenceID, phone, reference string) error {
	if !c.configured() {
		return nil
	}
	if reference == "" {
		reference = "coins_debited_manually"
	}
	status, _, err := c.do(ctx, http.MethodPost, c.walletURL("/debit-transaction"), map[string]any{
		"oid": phone, "reference_id": referenceID, "amount": float64(coins) / 10, "transaction_reference": reference,
	})
	if err != nil {
		return err
	}
	switch status {
	case http.StatusNotFound:
		return common.BadRequest("No record found for this user")
	case http.StatusBadRequest:
		return common.BadRequest("User have less coin balance to debit")
	}
	return nil
}

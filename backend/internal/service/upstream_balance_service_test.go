package service

import (
	"testing"
	"time"
)

func TestUpstreamBalanceUsageWindow(t *testing.T) {
	now := time.Date(2026, 11, 17, 10, 0, 0, 0, time.Local)

	t.Run("with payment method uses current month", func(t *testing.T) {
		start, end := upstreamBalanceUsageWindow(true, now)
		if start != "2026-11-01" || end != "2026-11-17" {
			t.Fatalf("want 2026-11-01..2026-11-17, got %s..%s", start, end)
		}
	})

	t.Run("without payment method uses last 100 days", func(t *testing.T) {
		start, end := upstreamBalanceUsageWindow(false, now)
		if start != "2026-08-09" || end != "2026-11-17" {
			t.Fatalf("want 2026-08-09..2026-11-17, got %s..%s", start, end)
		}
	})
}

func TestParseOpenAIBillingResult(t *testing.T) {
	sub := []byte(`{"object":"billing_subscription","has_payment_method":true,"hard_limit_usd":120.0,"system_hard_limit_usd":9999.0}`)
	usage := []byte(`{"object":"list","total_usage":3250.5}`) // 32.505 美元

	res, err := parseOpenAIBillingResult(42, sub, func(start, end string) ([]byte, int, error) {
		if start == "" || end == "" {
			t.Fatalf("empty window: %s..%s", start, end)
		}
		return usage, 200, nil
	})
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	// 余额 = 120 - 3250.5/100 = 87.495
	if res.Balance < 87.49 || res.Balance > 87.50 {
		t.Fatalf("balance = %v, want ~87.495", res.Balance)
	}
	if res.HardLimit != 120 || res.Used < 32.50 || res.Used > 32.51 {
		t.Fatalf("hard_limit=%v used=%v", res.HardLimit, res.Used)
	}
	if res.Protocol != "openai_billing" || res.Status != "success" || res.Currency != "USD" {
		t.Fatalf("protocol/status/currency = %s/%s/%s", res.Protocol, res.Status, res.Currency)
	}
	if res.AccountID != 42 {
		t.Fatalf("account_id = %d", res.AccountID)
	}
}

func TestParseOpenAIBillingResultUsageError(t *testing.T) {
	sub := []byte(`{"has_payment_method":false,"hard_limit_usd":10}`)
	_, err := parseOpenAIBillingResult(1, sub, func(start, end string) ([]byte, int, error) {
		return []byte(`{"error":"boom"}`), 500, nil
	})
	if err == nil {
		t.Fatal("want error on usage HTTP 500")
	}
}

func TestParseSub2APIUsageResult(t *testing.T) {
	t.Run("remaining preferred over balance", func(t *testing.T) {
		res, err := parseSub2APIUsageResult(7, []byte(`{"remaining":12.5,"balance":99,"isValid":true}`))
		if err != nil {
			t.Fatalf("parse failed: %v", err)
		}
		if res.Balance != 12.5 || res.Protocol != "sub2api_usage" {
			t.Fatalf("balance=%v protocol=%s", res.Balance, res.Protocol)
		}
	})

	t.Run("falls back to balance when remaining missing", func(t *testing.T) {
		res, err := parseSub2APIUsageResult(7, []byte(`{"balance":3.25}`))
		if err != nil {
			t.Fatalf("parse failed: %v", err)
		}
		if res.Balance != 3.25 {
			t.Fatalf("balance = %v", res.Balance)
		}
	})

	t.Run("isValid false rejected", func(t *testing.T) {
		if _, err := parseSub2APIUsageResult(7, []byte(`{"remaining":1,"isValid":false}`)); err == nil {
			t.Fatal("want error when isValid=false")
		}
	})

	t.Run("no remaining or balance rejected", func(t *testing.T) {
		if _, err := parseSub2APIUsageResult(7, []byte(`{"foo":1}`)); err == nil {
			t.Fatal("want error when no remaining/balance")
		}
	})
}

func TestUpstreamBalanceQueryable(t *testing.T) {
	if !upstreamBalanceQueryable(&Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}) {
		t.Fatal("openai apikey account should be queryable")
	}
	if upstreamBalanceQueryable(&Account{Platform: PlatformGemini, Type: AccountTypeAPIKey}) {
		t.Fatal("gemini account should not be queryable")
	}
	if upstreamBalanceQueryable(&Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}) {
		t.Fatal("openai oauth account should not be queryable")
	}
	if upstreamBalanceQueryable(nil) {
		t.Fatal("nil account should not be queryable")
	}
}

func TestExtraMapToUpstreamBalanceSnapshot(t *testing.T) {
	got := extraMapToUpstreamBalanceSnapshot(map[string]any{
		"status":     "success",
		"protocol":   "openai_billing",
		"balance":    88.5,
		"hard_limit": float64(120),
		"used":       float64(31),
		"currency":   "USD",
		"checked_at": "2026-11-17T12:00:00Z",
	})
	if got == nil || got.Status != "success" || got.Balance != 88.5 || got.HardLimit != 120 || got.Used != 31 {
		t.Fatalf("snapshot = %+v", got)
	}
	if extraMapToUpstreamBalanceSnapshot(nil) != nil {
		t.Fatal("nil map should return nil")
	}
}

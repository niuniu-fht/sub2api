package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func errorFailoverAccount(t *testing.T, credentials map[string]any) *Account {
	t.Helper()
	return &Account{Credentials: credentials}
}

func TestMatchErrorFailoverRulesDisabled(t *testing.T) {
	account := errorFailoverAccount(t, map[string]any{
		"error_failover_rules": []any{
			map[string]any{"status_codes": []any{400}},
		},
	})
	require.False(t, account.MatchErrorFailoverRules(400, []byte("quota exceeded")))
}

func TestMatchErrorFailoverRulesStatusCode(t *testing.T) {
	account := errorFailoverAccount(t, map[string]any{
		"error_failover_enabled": true,
		"error_failover_rules": []any{
			map[string]any{"status_codes": []any{400, 429}},
		},
	})
	require.True(t, account.MatchErrorFailoverRules(400, []byte("anything")))
	require.True(t, account.MatchErrorFailoverRules(429, nil))
	require.False(t, account.MatchErrorFailoverRules(500, []byte("server error")))
}

func TestMatchErrorFailoverRulesKeywords(t *testing.T) {
	account := errorFailoverAccount(t, map[string]any{
		"error_failover_enabled": true,
		"error_failover_rules": []any{
			map[string]any{"keywords": []any{"quota_exhausted", "Resource_Exhausted"}},
		},
	})
	require.True(t, account.MatchErrorFailoverRules(400, []byte(`{"error":"QUOTA_EXHAUSTED details"}`)))
	require.True(t, account.MatchErrorFailoverRules(429, []byte("error: RESOURCE_EXHAUSTED, retry later")))
	require.False(t, account.MatchErrorFailoverRules(400, []byte("content policy violation")))
}

func TestMatchErrorFailoverRulesStatusAndKeywordAND(t *testing.T) {
	account := errorFailoverAccount(t, map[string]any{
		"error_failover_enabled": true,
		"error_failover_rules": []any{
			map[string]any{"status_codes": []any{400}, "keywords": []any{"content_policy"}},
		},
	})
	require.True(t, account.MatchErrorFailoverRules(400, []byte("blocked by content_policy")))
	require.False(t, account.MatchErrorFailoverRules(429, []byte("blocked by content_policy")))
	require.False(t, account.MatchErrorFailoverRules(400, []byte("invalid argument")))
}

func TestMatchErrorFailoverRulesMultipleRulesOR(t *testing.T) {
	account := errorFailoverAccount(t, map[string]any{
		"error_failover_enabled": true,
		"error_failover_rules": []any{
			map[string]any{"status_codes": []any{429}},
			map[string]any{"keywords": []any{"safety"}},
		},
	})
	require.True(t, account.MatchErrorFailoverRules(429, []byte("rate limited")))
	require.True(t, account.MatchErrorFailoverRules(400, []byte("blocked for SAFETY reasons")))
	require.False(t, account.MatchErrorFailoverRules(400, []byte("invalid argument")))
}

func TestMatchErrorFailoverRulesSkipsEmptyAndInvalidEntries(t *testing.T) {
	account := errorFailoverAccount(t, map[string]any{
		"error_failover_enabled": true,
		"error_failover_rules": []any{
			map[string]any{},
			"not-a-map",
			map[string]any{"status_codes": []any{700, "abc"}},
			map[string]any{"status_codes": []any{400}},
		},
	})
	rules := account.GetErrorFailoverRules()
	require.Len(t, rules, 1)
	require.Equal(t, []int{400}, rules[0].StatusCodes)
	require.True(t, account.MatchErrorFailoverRules(400, []byte("x")))
	require.False(t, account.MatchErrorFailoverRules(500, []byte("x")))
}

func TestNormalizeErrorFailoverCredentials(t *testing.T) {
	require.NoError(t, NormalizeErrorFailoverCredentials(nil))
	require.NoError(t, NormalizeErrorFailoverCredentials(map[string]any{}))
	require.NoError(t, NormalizeErrorFailoverCredentials(map[string]any{
		"error_failover_enabled": true,
		"error_failover_rules": []any{
			map[string]any{"status_codes": []any{400}, "keywords": []any{"quota"}},
		},
	}))

	require.Error(t, NormalizeErrorFailoverCredentials(map[string]any{"error_failover_enabled": "yes"}))
	require.Error(t, NormalizeErrorFailoverCredentials(map[string]any{"error_failover_rules": "nope"}))
	require.Error(t, NormalizeErrorFailoverCredentials(map[string]any{
		"error_failover_rules": []any{"not-a-map"},
	}))
	require.Error(t, NormalizeErrorFailoverCredentials(map[string]any{
		"error_failover_rules": []any{map[string]any{}},
	}))
	require.Error(t, NormalizeErrorFailoverCredentials(map[string]any{
		"error_failover_rules": []any{map[string]any{"status_codes": []any{99}}},
	}))
	require.Error(t, NormalizeErrorFailoverCredentials(map[string]any{
		"error_failover_rules": []any{map[string]any{"keywords": []any{123}}},
	}))
}

func TestUpstreamFailoverErrorSkipAccountPenaltyExemptions(t *testing.T) {
	policyErr := &UpstreamFailoverError{StatusCode: 400, SkipAccountPenalty: true}
	require.True(t, policyErr.ShouldRetryNextAccount())
	require.False(t, policyErr.ShouldReportAccountScheduleFailure())

	normalErr := &UpstreamFailoverError{StatusCode: 400}
	require.True(t, normalErr.ShouldReportAccountScheduleFailure())
}

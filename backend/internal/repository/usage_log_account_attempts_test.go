package repository

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestNullAccountAttemptsJSONOnlyPersistsSwitchedRequests(t *testing.T) {
	// 单账号（无切换）不持久化，避免全量 usage_log 行膨胀。
	require.Nil(t, nullAccountAttemptsJSON(nil))
	require.Nil(t, nullAccountAttemptsJSON([]service.AccountAttempt{
		{AccountID: 1, Success: true, StatusCode: 200},
	}))

	// 链长>1（发生过切换）才序列化。
	encoded := nullAccountAttemptsJSON([]service.AccountAttempt{
		{AccountID: 1, StatusCode: 400, Error: "bad request"},
		{AccountID: 2, Success: true, StatusCode: 200},
	})
	require.NotNil(t, encoded)
	require.Contains(t, encoded.(string), `"account_id":1`)
	require.Contains(t, encoded.(string), `"account_id":2`)
}

func TestAccountAttemptsFromNullJSON(t *testing.T) {
	require.Nil(t, accountAttemptsFromNullJSON(sql.NullString{}))
	require.Nil(t, accountAttemptsFromNullJSON(sql.NullString{String: "not json", Valid: true}))

	attempts := accountAttemptsFromNullJSON(sql.NullString{
		String: `[{"at_unix_ms":1,"account_id":5,"account_name":"n","platform":"gemini","status_code":400,"duration_ms":120,"error":"e","success":false}]`,
		Valid:  true,
	})
	require.Len(t, attempts, 1)
	require.Equal(t, int64(5), attempts[0].AccountID)
	require.Equal(t, "gemini", attempts[0].Platform)
	require.Equal(t, 400, attempts[0].StatusCode)
}

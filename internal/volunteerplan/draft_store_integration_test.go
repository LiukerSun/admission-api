//go:build integration

package volunteerplan

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestDraftStoreCreateAndUpdateLifecycle(t *testing.T) {
	databaseURL := os.Getenv("ADMISSION_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set ADMISSION_TEST_DATABASE_URL to run volunteerplan integration tests")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	require.NoError(t, err)
	defer pool.Close()

	cleanupVolunteerDraftTestData(t, pool)
	userID := seedVolunteerDraftTestUser(t, pool)
	convID := seedVolunteerDraftTestConversation(t, pool, userID)

	store := NewDraftStore(pool)
	draftID, err := store.Create(ctx, userID, convID, []byte(`{"hello":"world"}`), "recommendations_v1")
	require.NoError(t, err)
	require.NotZero(t, draftID)
	draftID2, err := store.Create(ctx, userID, convID, []byte(`{"hello":"world2"}`), "recommendations_v1")
	require.NoError(t, err)
	require.NotZero(t, draftID2)
	require.NotEqual(t, draftID, draftID2)

	drafts, err := store.ListByConversation(ctx, userID, convID)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(drafts), 2)
	require.Equal(t, draftID2, drafts[0].ID)
	require.Equal(t, draftID, drafts[1].ID)

	draft, err := store.GetByID(ctx, userID, draftID)
	require.NoError(t, err)
	require.Equal(t, DraftStatusGenerating, draft.Status)
	require.Equal(t, "recommendations_v1", draft.AlgorithmVersion)

	require.NoError(t, store.MarkReady(ctx, userID, draftID, []byte(`{"id":"x","name":"n","description":"","columns":[],"rows":[],"stats":{"schoolCount":0,"groupCount":0,"recordCount":0}}`)))

	draft, err = store.GetByID(ctx, userID, draftID)
	require.NoError(t, err)
	require.Equal(t, DraftStatusReady, draft.Status)
	require.True(t, len(draft.PlanJSON) > 0)

	require.NoError(t, store.MarkFailed(ctx, userID, draftID, "boom"))
	draft, err = store.GetByID(ctx, userID, draftID)
	require.NoError(t, err)
	require.Equal(t, DraftStatusFailed, draft.Status)
	require.Equal(t, "boom", draft.Error)
}

func cleanupVolunteerDraftTestData(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	_, _ = pool.Exec(context.Background(), `
		DELETE FROM conversation_plan_drafts
		WHERE conversation_id IN (SELECT id FROM conversations WHERE title = 'tdd_volunteerplan')
	`)
	_, _ = pool.Exec(context.Background(), `DELETE FROM conversations WHERE title = 'tdd_volunteerplan'`)
	_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE email = 'tdd_volunteerplan@example.com'`)
}

func seedVolunteerDraftTestUser(t *testing.T, pool *pgxpool.Pool) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(), `
		INSERT INTO users (email, password_hash, role, status)
		VALUES ('tdd_volunteerplan@example.com', 'x', 'user', 'active')
		RETURNING id
	`).Scan(&id)
	require.NoError(t, err)
	return id
}

func seedVolunteerDraftTestConversation(t *testing.T, pool *pgxpool.Pool, userID int64) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(), `
		INSERT INTO conversations (user_id, title, status)
		VALUES ($1, 'tdd_volunteerplan', 'active')
		RETURNING id
	`, userID).Scan(&id)
	require.NoError(t, err)
	return id
}

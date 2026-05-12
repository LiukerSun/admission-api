package knowledge

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Document represents a knowledge base entry.
type Document struct {
	ID        int64    `json:"id"`
	Title     string   `json:"title"`
	Content   string   `json:"content"`
	Category  string   `json:"category"`
	Source    string   `json:"source"`
	Tags      []string `json:"tags"`
	CreatedAt string   `json:"created_at"`
}

// Store defines knowledge base operations.
type Store interface {
	Search(ctx context.Context, query string, category string, limit int) ([]Document, error)
	GetByID(ctx context.Context, id int64) (*Document, error)
}

type pgStore struct {
	pool *pgxpool.Pool
}

// NewStore creates a new PostgreSQL-backed knowledge store.
func NewStore(pool *pgxpool.Pool) Store {
	return &pgStore{pool: pool}
}

// Search performs keyword-based retrieval on knowledge documents.
// It splits the query into words and matches against title, content, and tags.
func (s *pgStore) Search(ctx context.Context, query string, category string, limit int) ([]Document, error) {
	if limit <= 0 || limit > 20 {
		limit = 5
	}

	words := splitKeywords(query)
	if len(words) == 0 {
		return nil, nil
	}

	conditions := make([]string, 0, len(words)*3)
	args := make([]any, 0, len(words)*3+2)
	argIdx := 1

	for _, w := range words {
		pattern := "%" + w + "%"
		conditions = append(conditions, fmt.Sprintf("title ILIKE $%d", argIdx))
		args = append(args, pattern)
		argIdx++
		conditions = append(conditions, fmt.Sprintf("content ILIKE $%d", argIdx))
		args = append(args, pattern)
		argIdx++
		conditions = append(conditions, fmt.Sprintf("EXISTS (SELECT 1 FROM unnest(tags) t WHERE t ILIKE $%d)", argIdx))
		args = append(args, pattern)
		argIdx++
	}

	whereClause := "(" + strings.Join(conditions, " OR ") + ")"
	if category != "" && category != "any" {
		whereClause += fmt.Sprintf(" AND category = $%d", argIdx)
		args = append(args, category)
		argIdx++
	}

	querySQL := fmt.Sprintf(`
		SELECT id, title, content, category, source, tags, created_at
		FROM knowledge_documents
		WHERE %s
		ORDER BY
			CASE WHEN title ILIKE $%d THEN 3 ELSE 0 END +
			CASE WHEN content ILIKE $%d THEN 1 ELSE 0 END DESC,
			created_at DESC
		LIMIT $%d
	`, whereClause, argIdx, argIdx, argIdx+1)

	fullPattern := "%" + strings.Join(words, "%") + "%"
	args = append(args, fullPattern, fullPattern, limit)

	rows, err := s.pool.Query(ctx, querySQL, args...)
	if err != nil {
		return nil, fmt.Errorf("knowledge search: %w", err)
	}
	defer rows.Close()

	return scanDocuments(rows)
}

func (s *pgStore) GetByID(ctx context.Context, id int64) (*Document, error) {
	var doc Document
	var createdAt time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT id, title, content, category, source, tags, created_at
		FROM knowledge_documents
		WHERE id = $1
	`, id).Scan(&doc.ID, &doc.Title, &doc.Content, &doc.Category, &doc.Source, &doc.Tags, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("knowledge get by id: %w", err)
	}
	doc.CreatedAt = createdAt.Format("2006-01-02")
	return &doc, nil
}

func splitKeywords(query string) []string {
	cleaned := strings.ReplaceAll(query, "，", " ")
	cleaned = strings.ReplaceAll(cleaned, "。", " ")
	cleaned = strings.ReplaceAll(cleaned, "？", " ")
	cleaned = strings.ReplaceAll(cleaned, "！", " ")
	cleaned = strings.ReplaceAll(cleaned, ",", " ")
	cleaned = strings.ReplaceAll(cleaned, ".", " ")
	cleaned = strings.ReplaceAll(cleaned, "?", " ")
	cleaned = strings.ReplaceAll(cleaned, "!", " ")

	parts := strings.Fields(cleaned)
	var result []string
	for _, p := range parts {
		if len(p) >= 2 {
			result = append(result, p)
		}
	}
	return result
}

func scanDocuments(rows pgx.Rows) ([]Document, error) {
	var docs []Document
	for rows.Next() {
		var doc Document
		var createdAt time.Time
		if err := rows.Scan(&doc.ID, &doc.Title, &doc.Content, &doc.Category, &doc.Source, &doc.Tags, &createdAt); err != nil {
			return nil, fmt.Errorf("scan knowledge document: %w", err)
		}
		doc.CreatedAt = createdAt.Format("2006-01-02")
		docs = append(docs, doc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate knowledge rows: %w", err)
	}
	return docs, nil
}

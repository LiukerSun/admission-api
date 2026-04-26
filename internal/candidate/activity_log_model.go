package candidate

import "time"

// ActivityLog represents a candidate activity log entry.
type ActivityLog struct {
	ID           int64      `db:"id" json:"id"`
	UserID       int64      `db:"user_id" json:"user_id"`
	ActivityType string     `db:"activity_type" json:"activity_type"`
	TargetType   *string    `db:"target_type" json:"target_type,omitempty"`
	TargetID     *int64     `db:"target_id" json:"target_id,omitempty"`
	Metadata     any        `db:"metadata" json:"metadata,omitempty"`
	IPAddress    *string    `db:"ip_address" json:"ip_address,omitempty"`
	CreatedAt    time.Time  `db:"created_at" json:"created_at"`
}

// CreateActivityInput is the input for creating an activity log.
type CreateActivityInput struct {
	UserID       int64  `json:"user_id"`
	ActivityType string `json:"activity_type"`
	TargetType   string `json:"target_type,omitempty"`
	TargetID     int64  `json:"target_id,omitempty"`
	Metadata     any    `json:"metadata,omitempty"`
	IPAddress    string `json:"ip_address,omitempty"`
}

// ActivityFilter defines filters for listing activity logs.
type ActivityFilter struct {
	UserID       int64
	ActivityType string
	TargetType   string
	StartTime    *time.Time
	EndTime      *time.Time
}

// ActivityLogListResponse is the response for listing activity logs.
type ActivityLogListResponse struct {
	Logs  []*ActivityLog `json:"logs"`
	Total int64          `json:"total"`
}

// ActivityStatsResponse is the response for activity statistics.
type ActivityStatsResponse struct {
	TargetType string `json:"target_type"`
	TargetID   int64  `json:"target_id"`
	Count      int64  `json:"count"`
}

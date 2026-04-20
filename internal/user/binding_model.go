package user

import "time"

// Binding represents a parent-student relationship.
type Binding struct {
	ID        int64     `json:"id"`
	ParentID  int64     `json:"parent_id"`
	StudentID int64     `json:"student_id"`
	CreatedAt time.Time `json:"created_at"`
}

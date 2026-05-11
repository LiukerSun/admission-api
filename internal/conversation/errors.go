package conversation

import "errors"

// ErrNotFound is returned when a conversation or message does not exist, or
// when it exists but belongs to another user. Service layer collapses the
// not-owner case into ErrNotFound to avoid leaking conversation IDs.
var ErrNotFound = errors.New("conversation: not found")

// ErrInvalidArgument is returned for malformed input at the domain boundary.
var ErrInvalidArgument = errors.New("conversation: invalid argument")

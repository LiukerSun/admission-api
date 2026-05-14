package web

// PaywallReason values describe why the paywall middleware blocked a request.
// They are returned in the response `data.reason` field so the frontend can
// vary copy without changing how it detects the paywall (which is keyed off
// the response code, not the reason).
const (
	PaywallReasonMembershipRequired = "membership_required"
	// PaywallReasonQuotaExceeded is reserved for future per-user quota
	// enforcement; the frontend modal can switch copy on it while reusing
	// the same response shape.
	PaywallReasonQuotaExceeded = "quota_exceeded"
)

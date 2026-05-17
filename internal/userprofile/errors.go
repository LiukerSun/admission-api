package userprofile

import "errors"

// Domain-level errors. Handler layer maps these to HTTP statuses + error codes.
//
// migration 008 之后，user_profiles 只保留 4 项核心信息，因此校验错误也只剩与
// 核心信息相关的几类（region 格式、subject 枚举、score 范围、electives 4 选 2）。
var (
	ErrProfileNotFound    = errors.New("user profile not found")
	ErrInvalidRegion      = errors.New("invalid region_code (must be 6 digits)")
	ErrInvalidSubject     = errors.New("invalid subject_category_code")
	ErrScoreOutOfRange    = errors.New("score out of range")
	ErrInvalidElectiveSet = errors.New("elective_subjects must contain exactly 2 distinct values from {biology, chemistry, geography, politics}")
)

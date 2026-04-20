ALTER TABLE users
    ADD COLUMN IF NOT EXISTS user_type VARCHAR(20) NOT NULL DEFAULT 'student'
    CHECK (user_type IN ('parent', 'student', 'admin'));

CREATE TABLE IF NOT EXISTS user_bindings (
    id BIGSERIAL PRIMARY KEY,
    parent_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    student_id BIGINT NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    CHECK (parent_id != student_id)
);

CREATE INDEX idx_user_bindings_parent_id ON user_bindings(parent_id);
CREATE INDEX idx_user_bindings_student_id ON user_bindings(student_id);

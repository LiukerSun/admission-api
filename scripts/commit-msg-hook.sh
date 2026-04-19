#!/bin/sh
# commit-msg hook: validate conventional commits

MSG=$(head -n1 "$1")
PATTERN="^(feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert)(\([^)]*\))?: .+"

if ! echo "$MSG" | grep -qE "$PATTERN"; then
    echo ""
    echo "ERROR: Commit message does not follow Conventional Commits format."
    echo ""
    echo "Format:  <type>(<scope>): <description>"
    echo ""
    echo "Types:"
    echo "  feat      — new feature"
    echo "  fix       — bug fix"
    echo "  docs      — documentation only"
    echo "  style     — code style (formatting, semicolons, etc.)"
    echo "  refactor  — code change that neither fixes a bug nor adds a feature"
    echo "  perf      — performance improvement"
    echo "  test      — adding or fixing tests"
    echo "  build     — build system or external dependencies"
    echo "  ci        — CI configuration"
    echo "  chore     — other changes that don't modify src or test files"
    echo "  revert    — revert a previous commit"
    echo ""
    echo "Examples:"
    echo "  feat: add user registration endpoint"
    echo "  fix(auth): resolve nil pointer in login handler"
    echo "  docs(readme): update deployment instructions"
    echo ""
    exit 1
fi

exit 0

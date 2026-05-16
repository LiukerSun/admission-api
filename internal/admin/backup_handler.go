package admin

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"admission-api/internal/platform/web"

	"github.com/gin-gonic/gin"
)

// BackupHandler exposes PostgreSQL backup / restore operations to administrators.
//
// Both endpoints invoke pg_dump / pg_restore directly inside the API container,
// connecting to the database over the compose network. The runtime image must
// therefore bundle the postgres client tools (see Dockerfile).
type BackupHandler struct {
	web.BaseHandler

	pgHost     string
	pgPort     string
	pgUser     string
	pgPassword string
	pgDatabase string

	// maxRestoreUploadBytes guards against operators uploading multi-GB files
	// by accident. PostgreSQL custom-format dumps compress aggressively, so
	// 1 GiB is far above any realistic schema size for this app.
	maxRestoreUploadBytes int64
}

// NewBackupHandler parses a postgres DSN (the same one the API uses to connect)
// and returns a handler that will shell out to pg_dump / pg_restore against it.
func NewBackupHandler(databaseURL string) (*BackupHandler, error) {
	u, err := url.Parse(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return nil, fmt.Errorf("database url must use postgres scheme, got %q", u.Scheme)
	}
	password, _ := u.User.Password()
	port := u.Port()
	if port == "" {
		port = "5432"
	}
	database := strings.TrimPrefix(u.Path, "/")
	if database == "" {
		return nil, fmt.Errorf("database url missing database name")
	}
	return &BackupHandler{
		pgHost:                u.Hostname(),
		pgPort:                port,
		pgUser:                u.User.Username(),
		pgPassword:            password,
		pgDatabase:            database,
		maxRestoreUploadBytes: 1 << 30, // 1 GiB
	}, nil
}

// pgEnv returns the process environment augmented with PGPASSWORD so the
// pg_* clients can authenticate without prompting.
func (h *BackupHandler) pgEnv() []string {
	return append(os.Environ(), "PGPASSWORD="+h.pgPassword)
}

// Export godoc
// @Summary      管理员导出数据库备份
// @Description  调用 pg_dump（custom 压缩格式 -Fc），把整个数据库的 schema + data
// @Description  流式返回。文件名形如 admission-YYYYMMDD-HHMMSS.dump。
// @Tags         admin
// @Produce      application/octet-stream
// @Security     BearerAuth
// @Success      200 {file} binary
// @Failure      401 {object} web.Response
// @Failure      403 {object} web.Response
// @Failure      500 {object} web.Response
// @Router       /api/v1/admin/db/backup [get]
func (h *BackupHandler) Export(c *gin.Context) {
	filename := fmt.Sprintf("admission-%s.dump", time.Now().Format("20060102-150405"))

	cmd := exec.CommandContext(c.Request.Context(),
		"pg_dump", "-Fc", "--no-owner", "--no-privileges",
		"-h", h.pgHost, "-p", h.pgPort,
		"-U", h.pgUser, "-d", h.pgDatabase,
	)
	cmd.Env = h.pgEnv()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		slog.Error("backup export pipe", "err", err)
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "failed to start pg_dump")
		return
	}
	stderrPipe, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		slog.Error("backup export start", "err", err)
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "failed to start pg_dump")
		return
	}

	// Drain stderr asynchronously so a verbose pg_dump cannot block on a full pipe.
	stderrCh := make(chan []byte, 1)
	go func() {
		buf, _ := io.ReadAll(stderrPipe)
		stderrCh <- buf
	}()

	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	// Headers are written here; the response body below is the raw pg_dump output.
	if _, copyErr := io.Copy(c.Writer, stdout); copyErr != nil {
		// Body has already started — best we can do is log and let the client
		// notice a truncated stream.
		slog.Error("backup export copy", "err", copyErr)
		_ = cmd.Process.Kill()
		<-stderrCh
		return
	}

	if waitErr := cmd.Wait(); waitErr != nil {
		stderrBytes := <-stderrCh
		slog.Error("backup export pg_dump failed",
			"err", waitErr, "stderr", string(stderrBytes))
		// Headers + body already partially sent; cannot rewrite to error JSON.
	}
}

// Restore godoc
// @Summary      管理员从备份恢复数据库
// @Description  接收 multipart/form-data 的 backup 字段（pg_dump custom 格式 .dump
// @Description  文件），流式喂给 pg_restore --clean --if-exists。恢复完成后
// @Description  返回 stderr 摘要供操作员核对。
// @Description
// @Description  注意：恢复会先 DROP 现有对象再写入。建议在窗口期执行。
// @Tags         admin
// @Accept       multipart/form-data
// @Produce      json
// @Security     BearerAuth
// @Param        backup  formData  file  true  "pg_dump custom-format .dump 文件"
// @Success      200 {object} web.Response{data=BackupRestoreResult}
// @Failure      400 {object} web.Response
// @Failure      401 {object} web.Response
// @Failure      403 {object} web.Response
// @Failure      413 {object} web.Response
// @Failure      500 {object} web.Response
// @Router       /api/v1/admin/db/restore [post]
func (h *BackupHandler) Restore(c *gin.Context) {
	fileHeader, err := c.FormFile("backup")
	if err != nil {
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, "missing backup file (multipart field 'backup')")
		return
	}
	if fileHeader.Size > h.maxRestoreUploadBytes {
		h.RespondError(c, http.StatusRequestEntityTooLarge, web.ErrCodeBadRequest,
			fmt.Sprintf("backup file too large (limit %d bytes)", h.maxRestoreUploadBytes))
		return
	}

	src, err := fileHeader.Open()
	if err != nil {
		slog.Error("backup restore open upload", "err", err)
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "failed to read upload")
		return
	}
	defer src.Close()

	// Use a fresh context with a generous timeout — restore can take a while
	// for a fully-populated dataset, but we still want a ceiling so a stuck
	// pg_restore doesn't pin the goroutine forever.
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx,
		"pg_restore", "--clean", "--if-exists", "--no-owner", "--no-privileges",
		"-h", h.pgHost, "-p", h.pgPort,
		"-U", h.pgUser, "-d", h.pgDatabase,
	)
	cmd.Env = h.pgEnv()
	cmd.Stdin = src

	stderrPipe, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		slog.Error("backup restore start", "err", err)
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "failed to start pg_restore")
		return
	}

	stderrBytes, _ := io.ReadAll(stderrPipe)
	waitErr := cmd.Wait()
	// pg_restore --clean almost always emits "errors ignored on restore"-style
	// warnings (objects that don't exist yet on the first run). A non-zero
	// exit code is still failure, so surface it; warnings are advisory.
	if waitErr != nil {
		slog.Error("backup restore pg_restore failed",
			"err", waitErr, "stderr", string(stderrBytes))
		h.RespondJSON(c, http.StatusInternalServerError, web.ErrorResponse(
			web.ErrCodeInternal,
			fmt.Sprintf("pg_restore failed: %v\n%s", waitErr, string(stderrBytes)),
		))
		return
	}

	h.RespondJSON(c, http.StatusOK, web.SuccessResponse(BackupRestoreResult{
		Filename:   fileHeader.Filename,
		SizeBytes:  fileHeader.Size,
		StderrTail: string(stderrBytes),
	}))
}

// BackupRestoreResult is the success payload returned by Restore.
type BackupRestoreResult struct {
	Filename   string `json:"filename"`
	SizeBytes  int64  `json:"size_bytes"`
	StderrTail string `json:"stderr_tail,omitempty"`
}

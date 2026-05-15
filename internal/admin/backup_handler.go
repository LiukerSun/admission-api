package admin

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os/exec"
	"time"

	"admission-api/internal/platform/web"

	"github.com/gin-gonic/gin"
)

// BackupHandler exposes PostgreSQL backup / restore operations to administrators.
//
// Both endpoints shell out to the postgres client tools (pg_dump / pg_restore)
// inside the admission-db container via `docker exec`. This keeps the host
// machine free of postgresql-client and works for any user who can already run
// the dev compose stack. Production deployments that move the API into a
// container should either install postgresql-client in the API image and call
// the binaries directly, or mount the host's docker socket.
type BackupHandler struct {
	web.BaseHandler

	containerName string
	pgUser        string
	pgDatabase    string

	// maxRestoreUploadBytes guards against operators uploading multi-GB files
	// by accident. PostgreSQL custom-format dumps compress aggressively, so
	// 1 GiB is far above any realistic schema size for this app.
	maxRestoreUploadBytes int64
}

// NewBackupHandler creates a backup handler bound to the given Postgres
// container / role / database. For the dev compose stack these come from
// docker-compose.yml: container_name=admission-db, user/db=app/admission.
func NewBackupHandler(containerName, pgUser, pgDatabase string) *BackupHandler {
	return &BackupHandler{
		containerName:         containerName,
		pgUser:                pgUser,
		pgDatabase:            pgDatabase,
		maxRestoreUploadBytes: 1 << 30, // 1 GiB
	}
}

// Export godoc
// @Summary      管理员导出数据库备份
// @Description  通过 docker exec 调用容器内的 pg_dump（custom 压缩格式 -Fc），
// @Description  把整个数据库的 schema + data 流式返回。文件名形如
// @Description  admission-YYYYMMDD-HHMMSS.dump。
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
		"docker", "exec", "-i", h.containerName,
		"pg_dump", "-Fc", "--no-owner", "--no-privileges",
		"-U", h.pgUser, "-d", h.pgDatabase,
	)

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
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
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
// @Description  文件），通过 docker exec 流式喂给容器内的 pg_restore --clean
// @Description  --if-exists。恢复完成后返回 stderr 摘要供操作员核对。
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
		"docker", "exec", "-i", h.containerName,
		"pg_restore", "--clean", "--if-exists", "--no-owner", "--no-privileges",
		"-U", h.pgUser, "-d", h.pgDatabase,
	)
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

package httpapi

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"yyb_go/internal/scriptrunner"
)

const (
	maxScriptSize     = 1 << 20
	maxScriptLogBytes = 256 << 10
	defaultLogLimit   = 256 << 10
)

func (a *App) handleScriptList(c *gin.Context) {
	if a.scripts == nil {
		writeError(c.Writer, http.StatusServiceUnavailable, "脚本运行器未启用")
		return
	}
	writeJSON(c.Writer, http.StatusOK, gin.H{
		"scripts":    a.scripts.List(),
		"dir":        a.scripts.Dir(),
		"sdk_dir":    a.scripts.SdkDir(),
		"server_url": a.scripts.ServerURL(),
		"python":     a.scripts.Python(),
		"python_ok":  a.scripts.PythonAvailable(),
	})
}

func (a *App) handleScriptUpload(c *gin.Context) {
	if a.scripts == nil {
		writeError(c.Writer, http.StatusServiceUnavailable, "脚本运行器未启用")
		return
	}
	file, err := c.FormFile("file")
	if err != nil {
		writeError(c.Writer, http.StatusBadRequest, "缺少 file 文件字段")
		return
	}
	if file.Size > maxScriptSize {
		writeError(c.Writer, http.StatusRequestEntityTooLarge, "脚本超过 1 MiB 限制")
		return
	}
	name := filepath.Base(file.Filename)
	if !strings.HasSuffix(strings.ToLower(name), ".py") || !scriptrunner.ValidName(name) {
		writeError(c.Writer, http.StatusBadRequest, "文件名不合法：必须是 *.py，且仅含字母数字、._-")
		return
	}
	dest := a.scripts.ScriptPath(name)
	if _, err = os.Stat(dest); err == nil && c.Query("overwrite") != "1" {
		writeError(c.Writer, http.StatusConflict, "脚本已存在：上传时加 ?overwrite=1 覆盖")
		return
	}
	src, err := file.Open()
	if err != nil {
		writeError(c.Writer, http.StatusBadRequest, err.Error())
		return
	}
	defer src.Close()
	data, err := io.ReadAll(io.LimitReader(src, maxScriptSize+1))
	if err != nil {
		writeError(c.Writer, http.StatusBadRequest, err.Error())
		return
	}
	if len(data) > maxScriptSize {
		writeError(c.Writer, http.StatusRequestEntityTooLarge, "脚本超过 1 MiB 限制")
		return
	}
	if err = os.MkdirAll(a.scripts.Dir(), 0o755); err != nil {
		writeError(c.Writer, http.StatusInternalServerError, err.Error())
		return
	}
	if err = os.WriteFile(dest, data, 0o644); err != nil {
		writeError(c.Writer, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c.Writer, http.StatusOK, gin.H{"name": name, "size": len(data)})
}

func (a *App) handleScriptRun(c *gin.Context) {
	var body struct {
		Args string `json:"args"`
	}
	if err := decodeOptionalJSON(c.Request, &body); err != nil {
		writeError(c.Writer, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if err := a.scripts.RunArgs(c.Param("name"), scriptrunner.SplitArgs(body.Args)); err != nil {
		writeError(c.Writer, http.StatusConflict, err.Error())
		return
	}
	script, _ := a.scripts.Status(c.Param("name"))
	writeJSON(c.Writer, http.StatusOK, script)
}

func (a *App) handleScriptStop(c *gin.Context) {
	if err := a.scripts.StopRun(c.Param("name")); err != nil {
		writeError(c.Writer, http.StatusConflict, err.Error())
		return
	}
	writeJSON(c.Writer, http.StatusOK, gin.H{"stopped": c.Param("name")})
}

func (a *App) handleScriptLogs(c *gin.Context) {
	limit := defaultLogLimit
	if raw := c.Query("limit"); raw != "" {
		var parsed int
		if _, err := fmt.Sscanf(raw, "%d", &parsed); err == nil && parsed > 0 && parsed <= maxScriptLogBytes {
			limit = parsed
		}
	}
	content, err := a.scripts.Logs(c.Param("name"), limit)
	if err != nil {
		writeError(c.Writer, http.StatusNotFound, err.Error())
		return
	}
	script, ok := a.scripts.Status(c.Param("name"))
	if !ok {
		writeError(c.Writer, http.StatusNotFound, "脚本不存在: "+c.Param("name"))
		return
	}
	writeJSON(c.Writer, http.StatusOK, gin.H{
		"name":        c.Param("name"),
		"content":     content,
		"running":     script.Running,
		"started_at":  script.StartedAt,
		"finished_at": script.FinishedAt,
		"exit_code":   script.ExitCode,
		"last_error":  script.LastError,
	})
}

func (a *App) handleScriptSchedule(c *gin.Context) {
	var body struct {
		Cron string `json:"cron"`
	}
	if err := decodeOptionalJSON(c.Request, &body); err != nil {
		writeError(c.Writer, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if err := a.scripts.SetSchedule(c.Param("name"), strings.TrimSpace(body.Cron)); err != nil {
		writeError(c.Writer, http.StatusBadRequest, err.Error())
		return
	}
	script, _ := a.scripts.Status(c.Param("name"))
	writeJSON(c.Writer, http.StatusOK, script)
}

func (a *App) handleScriptUnschedule(c *gin.Context) {
	if err := a.scripts.ClearSchedule(c.Param("name")); err != nil {
		writeError(c.Writer, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c.Writer, http.StatusOK, gin.H{"unscheduled": c.Param("name")})
}

func (a *App) handleScriptDelete(c *gin.Context) {
	if err := a.scripts.Delete(c.Param("name")); err != nil {
		writeError(c.Writer, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(c.Writer, http.StatusOK, gin.H{"deleted": c.Param("name")})
}

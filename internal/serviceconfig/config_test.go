package serviceconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadValidConfig(t *testing.T) {
	path := writeConfig(t, `{
		"listen_address":"127.0.0.1:8000",
		"data_root":"resource",
		"asset_root":"resource",
		"database_filename":"yyb.db",
		"tcp_proxy":""
	}`)
	config, err := Load(path)
	if err != nil {
		t.Fatalf("读取有效配置失败: %v", err)
	}
	if config.ListenAddress != "127.0.0.1:8000" || config.DatabaseFilename != "yyb.db" {
		t.Fatalf("配置内容错误: %+v", config)
	}
}

func TestLoadRejectsInvalidConfig(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"未知字段", validConfig(`,"unknown":true`), "unknown field"},
		{"监听地址", strings.Replace(validConfig(""), `"127.0.0.1:8000"`, `"8000"`, 1), "listen_address"},
		{"数据库路径", strings.Replace(validConfig(""), `"yyb.db"`, `"../yyb.db"`, 1), "database_filename"},
		{"旧聚合字段", validConfig(`,"aggregator":{"mode":"enabled"}`), "unknown field"},
		{"尾部对象", validConfig("") + `{}`, "只能包含一个 JSON 对象"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Load(writeConfig(t, test.content))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("错误=%v，期望包含 %q", err, test.want)
			}
		})
	}
}

func TestLoadAdminConfig(t *testing.T) {
	path := writeConfig(t, `{
		"listen_address":"127.0.0.1:8000",
		"data_root":"resource",
		"asset_root":"resource",
		"database_filename":"yyb.db",
		"tcp_proxy":"",
		"admin_user":"admin",
		"admin_password":"secret",
		"integration_token":"tok"
	}`)
	config, err := Load(path)
	if err != nil {
		t.Fatalf("读取管理员配置失败: %v", err)
	}
	if config.AdminUser != "admin" || config.SessionDurationMinute != 1440 {
		t.Fatalf("配置内容错误: %+v", config)
	}
}

func TestLoadRejectsIncompleteAdminConfig(t *testing.T) {
	_, err := Load(writeConfig(t, validConfig(`,"admin_user":"admin"`)))
	if err == nil || !strings.Contains(err.Error(), "admin_user") {
		t.Fatalf("错误=%v，期望 admin_user 校验", err)
	}
}

func validConfig(extra string) string {
	return `{
		"listen_address":"127.0.0.1:8000",
		"data_root":"resource",
		"asset_root":"resource",
		"database_filename":"yyb.db",
		"tcp_proxy":""` + extra + `
	}`
}

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "service.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("写入测试配置失败: %v", err)
	}
	return path
}

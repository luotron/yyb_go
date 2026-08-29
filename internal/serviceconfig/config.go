package serviceconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const maximumConfigSize = 1 << 20

type Config struct {
	ListenAddress           string `json:"listen_address"`
	DataRoot                string `json:"data_root"`
	AssetRoot               string `json:"asset_root"`
	DatabaseFilename        string `json:"database_filename"`
	TCPProxy                string `json:"tcp_proxy"`
	KeepAliveIntervalMinute int64  `json:"keepalive_interval_minutes"`
	KeepAliveAheadMinute    int64  `json:"keepalive_ahead_minutes"`
	PythonCommand           string `json:"python_command"`
	TLSCert                 string `json:"tls_cert"`
	TLSKey                  string `json:"tls_key"`
}

// TLSEnabled 证书与私钥同时配置时启用 HTTPS/WSS。
func (c *Config) TLSEnabled() bool {
	return c.TLSCert != "" && c.TLSKey != ""
}

func Load(filename string) (Config, error) {
	file, err := os.Open(filename)
	if err != nil {
		return Config{}, fmt.Errorf("打开服务配置失败: %w", err)
	}
	defer file.Close()

	raw, err := io.ReadAll(io.LimitReader(file, maximumConfigSize+1))
	if err != nil {
		return Config{}, fmt.Errorf("读取服务配置失败: %w", err)
	}
	if len(raw) > maximumConfigSize {
		return Config{}, errors.New("服务配置超过 1 MiB 大小限制")
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var config Config
	if err = decoder.Decode(&config); err != nil {
		return Config{}, fmt.Errorf("解析服务配置失败: %w", err)
	}
	if err = ensureJSONEnd(decoder); err != nil {
		return Config{}, err
	}
	if err = config.normalizeAndValidate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

func (c *Config) normalizeAndValidate() error {
	c.ListenAddress = strings.TrimSpace(c.ListenAddress)
	c.DataRoot = filepath.Clean(strings.TrimSpace(c.DataRoot))
	c.AssetRoot = filepath.Clean(strings.TrimSpace(c.AssetRoot))
	c.DatabaseFilename = strings.TrimSpace(c.DatabaseFilename)
	c.TCPProxy = strings.TrimSpace(c.TCPProxy)
	c.PythonCommand = strings.TrimSpace(c.PythonCommand)
	c.TLSCert = strings.TrimSpace(c.TLSCert)
	c.TLSKey = strings.TrimSpace(c.TLSKey)

	if err := validateListenAddress(c.ListenAddress); err != nil {
		return err
	}
	if c.DataRoot == "." || c.AssetRoot == "." {
		return errors.New("data_root 和 asset_root 必须明确配置")
	}
	if c.DatabaseFilename == "" || filepath.Base(c.DatabaseFilename) != c.DatabaseFilename {
		return errors.New("database_filename 必须是不含目录的文件名")
	}
	if c.KeepAliveIntervalMinute < 0 || c.KeepAliveAheadMinute < 0 {
		return errors.New("keepalive 配置必须是非负整数分钟")
	}
	if c.KeepAliveIntervalMinute == 0 {
		c.KeepAliveIntervalMinute = 1
	}
	if c.KeepAliveAheadMinute == 0 {
		c.KeepAliveAheadMinute = 45
	}
	if c.PythonCommand == "" {
		c.PythonCommand = "python"
	}
	if (c.TLSCert == "") != (c.TLSKey == "") {
		return errors.New("tls_cert 与 tls_key 必须同时配置（留空则不启用 HTTPS）")
	}
	if c.TLSCert != "" {
		for _, path := range []string{c.TLSCert, c.TLSKey} {
			if info, err := os.Stat(path); err != nil || info.IsDir() {
				return fmt.Errorf("TLS 文件不存在: %s", path)
			}
		}
	}
	return nil
}

func validateListenAddress(address string) error {
	host, rawPort, err := net.SplitHostPort(address)
	if err != nil || strings.TrimSpace(host) == "" {
		return errors.New("listen_address 必须是明确的 host:port")
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil || port < 1 || port > 65535 {
		return errors.New("listen_address 端口必须在 1 到 65535 之间")
	}
	return nil
}

func ensureJSONEnd(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return fmt.Errorf("服务配置包含无效尾部内容: %w", err)
	}
	return errors.New("服务配置只能包含一个 JSON 对象")
}

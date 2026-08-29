package httpapi

import (
	"os"
	"path/filepath"
)

type resources struct {
	Root       string
	DB         string
	Avatars    string
	QR         string
	Templates  string
	Static     string
	Scripts    string
	ScriptSDK  string
	ScriptLogs string
}

func ensureResources(dataRoot, assetRoot string) (resources, error) {
	if assetRoot == "" {
		assetRoot = dataRoot
	}
	res := resources{
		Root:       dataRoot,
		DB:         filepath.Join(dataRoot, "db"),
		Avatars:    filepath.Join(dataRoot, "avatars"),
		QR:         filepath.Join(dataRoot, "qr"),
		Templates:  filepath.Join(assetRoot, "templates"),
		Static:     filepath.Join(assetRoot, "static"),
		Scripts:    filepath.Join(dataRoot, "scripts"),
		ScriptSDK:  filepath.Join(assetRoot, "scripts", "sdk"),
		ScriptLogs: filepath.Join(dataRoot, "scripts", "logs"),
	}
	for _, p := range []string{res.DB, res.Avatars, res.QR, res.Scripts, res.ScriptLogs} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			return res, err
		}
	}
	return res, nil
}

func (r resources) avatarPath(openid string) string {
	return filepath.Join(r.Avatars, safeName(openid)+".jpg")
}

func (r resources) qrPath(sessionID string) string {
	return filepath.Join(r.QR, sessionID+".jpg")
}

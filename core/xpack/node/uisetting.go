package node

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"regexp"
	"strings"
	"sync"

	"github.com/1Panel-dev/1Panel/core/global"
)

// Interface settings: theme colour, panel title, logo / favicon, login page images and the
// watermark. The browser reads them through settings/search before login.

var uiKeys = map[string]string{
	"Title":             "title",
	"Theme":             "theme",
	"ThemeColor":        "themeColor",
	"Watermark":         "watermark",
	"WatermarkShow":     "watermarkShow",
	"MasterAlias":       "masterAlias",
	"LoginBgType":       "loginBgType",
	"LoginBgColor":      "loginBgColor",
	"LoginBackground":   "loginBackground",
	"LoginBtnLinkColor": "loginBtnLinkColor",
	"Logo":              "logo",
	"LogoWithText":      "logoWithText",
	"Favicon":           "favicon",
	"LoginImage":        "loginImage",
}

var cssColorRule = regexp.MustCompile(`^(#[0-9a-fA-F]{3,8}|rgba?\(\s*\d{1,3}\s*,\s*\d{1,3}\s*,\s*\d{1,3}(\s*,\s*(0|1|0?\.\d+))?\s*\))$`)

func isValidCSSColor(s string) bool {
	return cssColorRule.MatchString(strings.TrimSpace(s))
}

// keys that hold an uploaded file named after the key's json name
var uiFileKinds = map[string]bool{"logo": true, "logoWithText": true, "favicon": true, "loginImage": true, "loginBackground": true}

var (
	uiMu   sync.Mutex
	uiData map[string]string
)

func uiConfigPath() string { return path.Join(global.CONF.Base.InstallDir, "1panel", "ui", "settings.json") }
func uiThemeDir() string   { return path.Join(global.CONF.Base.InstallDir, "1panel", "uploads", "theme") }

func uiLoadLocked() map[string]string {
	if uiData != nil {
		return uiData
	}
	m := map[string]string{}
	if b, err := os.ReadFile(uiConfigPath()); err == nil {
		_ = json.Unmarshal(b, &m)
	}
	uiData = m
	return m
}

func uiSaveLocked() error {
	if err := os.MkdirAll(path.Dir(uiConfigPath()), 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(uiData, "", "  ")
	return os.WriteFile(uiConfigPath(), b, 0o600)
}

func UIGetSettings() map[string]string {
	uiMu.Lock()
	defer uiMu.Unlock()
	out := map[string]string{}
	for _, j := range uiKeys {
		out[j] = ""
	}
	for k, v := range uiLoadLocked() {
		out[k] = v
	}
	if out["watermarkShow"] == "" {
		out["watermarkShow"] = "Disable"
	}
	return out
}

func UISetSetting(key, value string) error {
	j, ok := uiKeys[key]
	if !ok {
		return fmt.Errorf("unknown setting: %s", key)
	}
	if len(value) > 64*1024 {
		return fmt.Errorf("value too long")
	}
	value = strings.TrimSpace(value)
	uiMu.Lock()
	defer uiMu.Unlock()
	uiLoadLocked()

	switch j {
	case "loginBackground":
		if value != "" && value != "loginBackground" && !isValidCSSColor(value) {
			return fmt.Errorf("invalid loginBackground value")
		}
		if value == "" {
			if uiData["loginBgType"] != "color" {
				_ = os.Remove(path.Join(uiThemeDir(), "loginBackground"))
			}
		} else if isValidCSSColor(value) {
			uiData["loginBgColor"] = value
		}
	case "loginBgColor":
		if value != "" && !isValidCSSColor(value) {
			return fmt.Errorf("invalid loginBgColor value")
		}
		if uiData["loginBgType"] == "color" {
			if value == "" {
				delete(uiData, "loginBackground")
			} else {
				uiData["loginBackground"] = value
			}
		}
	case "loginBtnLinkColor":
		if value != "" && !isValidCSSColor(value) {
			return fmt.Errorf("invalid loginBtnLinkColor value")
		}
	case "loginBgType":
		if value != "" && value != "image" && value != "color" {
			return fmt.Errorf("invalid loginBgType value")
		}
		if value == "image" {
			if _, err := os.Stat(path.Join(uiThemeDir(), "loginBackground")); err == nil {
				uiData["loginBackground"] = "loginBackground"
			} else if uiData["loginBackground"] != "loginBackground" {
				delete(uiData, "loginBackground")
			}
		} else if value == "color" {
			if col := uiData["loginBgColor"]; col != "" {
				uiData["loginBackground"] = col
			} else if uiData["loginBackground"] == "loginBackground" {
				delete(uiData, "loginBackground")
			}
		}
	case "themeColor", "watermark":
		if value != "" && !json.Valid([]byte(value)) {
			return fmt.Errorf("invalid json")
		}
		if j == "watermark" {
			if value != "" {
				uiData["watermarkShow"] = "Enable"
			} else {
				uiData["watermarkShow"] = "Disable"
			}
		}
	default:
		if uiFileKinds[j] {
			if value != "" {
				return fmt.Errorf("use upload for %s", key)
			}
			_ = os.Remove(path.Join(uiThemeDir(), j))
		}
	}

	if value == "" {
		delete(uiData, j)
	} else {
		uiData[j] = value
	}
	return uiSaveLocked()
}

// UIUpload stores an image for kind (logo, logoWithText, favicon, loginImage, loginBackground).
func UIUpload(kind string, r io.Reader) error {
	if !uiFileKinds[kind] {
		return fmt.Errorf("unknown kind: %s", kind)
	}
	data, err := io.ReadAll(io.LimitReader(r, 4<<20+1))
	if err != nil {
		return err
	}
	if len(data) == 0 || len(data) > 4<<20 {
		return fmt.Errorf("image must be between 1 byte and 4 MB")
	}
	mt := http.DetectContentType(data)
	head := string(data[:min(len(data), 512)])
	if !strings.HasPrefix(mt, "image/") && !strings.Contains(head, "<svg") {
		return fmt.Errorf("not an image")
	}
	uiMu.Lock()
	defer uiMu.Unlock()
	uiLoadLocked()
	if err := os.MkdirAll(uiThemeDir(), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path.Join(uiThemeDir(), kind), data, 0o644); err != nil {
		return err
	}
	uiData[kind] = kind
	return uiSaveLocked()
}

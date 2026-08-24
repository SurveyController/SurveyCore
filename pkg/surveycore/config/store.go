package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/SurveyController/SurveyCore/pkg/surveycore/model"
)

var replaceConfigFile = atomicReplace

// BuildDefaultConfigFilename returns a filesystem-safe JSON filename.
func BuildDefaultConfigFilename(surveyTitle string) string {
	return sanitizeFilename(surveyTitle) + ".json"
}

// Load reads either a schemaVersion 2 document or a supported legacy document.
func Load(path string, strict bool) (model.RunRequest, error) {
	document, err := LoadDocument(path, strict)
	if err != nil {
		return model.RunRequest{}, err
	}
	if document.SchemaVersion == 0 {
		return model.RunRequest{}, nil
	}
	return RunRequestFromConfigDocument(document)
}

// LoadDocument reads and validates a local configuration document.
func LoadDocument(path string, strict bool) (ConfigDocument, error) {
	if strings.TrimSpace(path) == "" {
		return ConfigDocument{}, fmt.Errorf("配置路径不能为空")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if strict {
			return ConfigDocument{}, fmt.Errorf("读取配置失败: %s -> %w", path, err)
		}
		return ConfigDocument{}, nil
	}
	cleaned := StripJSONComments(string(data))
	if strings.TrimSpace(cleaned) == "" {
		if strict {
			return ConfigDocument{}, fmt.Errorf("配置文件为空")
		}
		return ConfigDocument{}, nil
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(cleaned), &payload); err != nil {
		if strict {
			return ConfigDocument{}, fmt.Errorf("读取配置失败: %s -> %w", path, err)
		}
		return ConfigDocument{}, nil
	}
	if _, ok := payload["schemaVersion"]; ok {
		document, err := DeserializeConfigDocument(payload)
		if err != nil {
			if strict {
				return ConfigDocument{}, fmt.Errorf("配置不兼容: %s -> %w", path, err)
			}
			return ConfigDocument{}, nil
		}
		return document, nil
	}
	document, err := migrateLegacyDocument(payload)
	if err != nil {
		if strict {
			return ConfigDocument{}, fmt.Errorf("配置不兼容: %s -> %w", path, err)
		}
		return ConfigDocument{}, nil
	}
	return document, nil
}

// Save atomically stores a run request as a schemaVersion 2 document.
func Save(config model.RunRequest, path string) (string, error) {
	return SaveDocument(ConfigDocumentFromRunRequest(config), path)
}

// SaveDocument atomically stores a schemaVersion 2 document.
func SaveDocument(document ConfigDocument, path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("配置路径不能为空")
	}
	path = filepath.Clean(path)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	payload, err := SerializeConfigDocument(document)
	if err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".surveycontroller-config-*.tmp")
	if err != nil {
		return "", err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o644); err != nil {
		_ = temp.Close()
		return "", err
	}
	if _, err := temp.Write(append(data, '\n')); err != nil {
		_ = temp.Close()
		return "", err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return "", err
	}
	if err := temp.Close(); err != nil {
		return "", err
	}
	if err := replaceConfigFile(tempPath, path); err != nil {
		return "", err
	}
	return path, nil
}

// StripJSONComments removes line and block comments while preserving strings.
func StripJSONComments(raw string) string {
	text := strings.TrimLeft(raw, "\ufeff")
	var out strings.Builder
	inString := false
	escaped := false
	inLineComment := false
	inBlockComment := false
	for i := 0; i < len(text); i++ {
		ch := text[i]
		var next byte
		if i+1 < len(text) {
			next = text[i+1]
		}
		if inLineComment {
			if ch == '\n' {
				inLineComment = false
				out.WriteByte(ch)
			}
			continue
		}
		if inBlockComment {
			if ch == '*' && next == '/' {
				inBlockComment = false
				i++
			}
			continue
		}
		if inString {
			out.WriteByte(ch)
			if escaped {
				escaped = false
			} else if ch == '\\' {
				escaped = true
			} else if ch == '"' {
				inString = false
			}
			continue
		}
		if ch == '"' {
			inString = true
			out.WriteByte(ch)
			continue
		}
		if ch == '/' && next == '/' {
			inLineComment = true
			i++
			continue
		}
		if ch == '/' && next == '*' {
			inBlockComment = true
			i++
			continue
		}
		out.WriteByte(ch)
	}
	return out.String()
}

func sanitizeFilename(value string) string {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return "wjx_config"
	}
	var out strings.Builder
	for _, ch := range normalized {
		if strings.ContainsRune(`\/:*?"<>|`, ch) || !strconvPrintable(ch) {
			continue
		}
		if ch == ' ' {
			out.WriteRune('_')
			continue
		}
		out.WriteRune(ch)
	}
	text := out.String()
	if text == "" {
		return "wjx_config"
	}
	if len([]rune(text)) > 80 {
		return string([]rune(text)[:80])
	}
	return text
}

func strconvPrintable(ch rune) bool {
	return ch >= 32 && ch != 127
}

package appconfig

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/SurveyController/SurveyCore/internal/execution"
)

const (
	DefaultPath      = "configs/surveycore.toml"
	defaultHost      = "127.0.0.1"
	defaultPort      = 19178
	defaultDBPath    = "data/surveycore.db"
	defaultAIBaseURL = "https://api.deepseek.com/v1"
	defaultAIModel   = "deepseek-chat"
)

type Config struct {
	Server  ServerConfig
	Storage StorageConfig
	AI      AIConfig
}

type ServerConfig struct {
	Port int
}

type StorageConfig struct {
	DBPath string
}

type AIConfig struct {
	BaseURL string
	Model   string
	APIKey  string
}

func Load(path string) (Config, error) {
	cfg := Default()
	path = strings.TrimSpace(path)
	if path == "" {
		path = strings.TrimSpace(os.Getenv("SURVEYCORE_CONFIG"))
	}
	if path == "" {
		path = DefaultPath
	}

	if err := loadFile(path, &cfg); err != nil {
		return Config{}, err
	}
	if err := applyEnv(&cfg); err != nil {
		return Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func Default() Config {
	return Config{
		Server: ServerConfig{Port: defaultPort},
		Storage: StorageConfig{
			DBPath: defaultDBPath,
		},
		AI: AIConfig{
			BaseURL: defaultAIBaseURL,
			Model:   defaultAIModel,
		},
	}
}

func (c Config) Validate() error {
	if err := validatePort(c.Server.Port); err != nil {
		return err
	}
	if strings.TrimSpace(c.Storage.DBPath) == "" {
		return errors.New("storage.db_path 不能为空")
	}
	if strings.TrimSpace(c.AI.BaseURL) == "" {
		return errors.New("ai.base_url 不能为空")
	}
	if strings.TrimSpace(c.AI.Model) == "" {
		return errors.New("ai.model 不能为空")
	}
	return nil
}

func (c Config) ListenAddr() string {
	return net.JoinHostPort(defaultHost, strconv.Itoa(c.Server.Port))
}

func (c Config) ApplyExecutionDefaults(cfg *execution.ExecutionConfig) {
	if cfg == nil {
		return
	}
	if strings.TrimSpace(cfg.AIBaseURL) == "" {
		cfg.AIBaseURL = strings.TrimSpace(c.AI.BaseURL)
	}
	if strings.TrimSpace(cfg.AIModel) == "" {
		cfg.AIModel = strings.TrimSpace(c.AI.Model)
	}
	if strings.TrimSpace(cfg.AIAPIKey) == "" {
		cfg.AIAPIKey = strings.TrimSpace(c.AI.APIKey)
	}
}

func loadFile(path string, cfg *Config) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("读取配置文件失败: %w", err)
	}
	defer file.Close()

	section := ""
	scanner := bufio.NewScanner(file)
	for lineNum := 1; scanner.Scan(); lineNum++ {
		line := strings.TrimSpace(stripComment(scanner.Text()))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
			if section != "server" && section != "storage" && section != "ai" {
				return fmt.Errorf("配置文件第 %d 行包含未知分区: %s", lineNum, section)
			}
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("配置文件第 %d 行格式无效", lineNum)
		}
		if err := applySetting(cfg, section, strings.TrimSpace(key), strings.TrimSpace(value)); err != nil {
			return fmt.Errorf("配置文件第 %d 行无效: %w", lineNum, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("读取配置文件失败: %w", err)
	}
	return nil
}

func stripComment(line string) string {
	inQuote := false
	escaped := false
	for i, r := range line {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' && inQuote {
			escaped = true
			continue
		}
		if r == '"' {
			inQuote = !inQuote
			continue
		}
		if r == '#' && !inQuote {
			return line[:i]
		}
	}
	return line
}

func applySetting(cfg *Config, section, key, rawValue string) error {
	switch section {
	case "server":
		if key != "port" {
			return fmt.Errorf("server.%s 是未知字段", key)
		}
		port, err := strconv.Atoi(rawValue)
		if err != nil {
			return fmt.Errorf("server.port 必须是数字")
		}
		cfg.Server.Port = port
	case "storage":
		if key != "db_path" {
			return fmt.Errorf("storage.%s 是未知字段", key)
		}
		cfg.Storage.DBPath = parseStringValue(rawValue)
	case "ai":
		switch key {
		case "base_url":
			cfg.AI.BaseURL = parseStringValue(rawValue)
		case "model":
			cfg.AI.Model = parseStringValue(rawValue)
		case "api_key":
			cfg.AI.APIKey = parseStringValue(rawValue)
		default:
			return fmt.Errorf("ai.%s 是未知字段", key)
		}
	default:
		return fmt.Errorf("字段 %s 必须放在 [server]、[storage] 或 [ai] 分区下", key)
	}
	return nil
}

func parseStringValue(value string) string {
	parsed, err := strconv.Unquote(value)
	if err == nil {
		return strings.TrimSpace(parsed)
	}
	return strings.Trim(strings.TrimSpace(value), `"`)
}

func applyEnv(cfg *Config) error {
	if value := strings.TrimSpace(os.Getenv("SURVEY_PORT")); value != "" {
		port, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("SURVEY_PORT 必须是数字")
		}
		cfg.Server.Port = port
	}
	if value := strings.TrimSpace(os.Getenv("SURVEYCORE_DB_PATH")); value != "" {
		cfg.Storage.DBPath = value
	}
	if value := strings.TrimSpace(os.Getenv("AI_BASE_URL")); value != "" {
		cfg.AI.BaseURL = value
	}
	if value := strings.TrimSpace(os.Getenv("AI_MODEL")); value != "" {
		cfg.AI.Model = value
	}
	if value := strings.TrimSpace(os.Getenv("AI_API_KEY")); value != "" {
		cfg.AI.APIKey = value
	}
	return nil
}

func validatePort(port int) error {
	if port < 1 || port > 65535 {
		return errors.New("server.port 必须是 1 到 65535 之间的数字")
	}
	return nil
}

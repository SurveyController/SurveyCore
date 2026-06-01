package appconfig

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
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
	Server  ServerConfig  `toml:"server"`
	Storage StorageConfig `toml:"storage"`
	AI      AIConfig      `toml:"ai"`
}

type ServerConfig struct {
	Port int `toml:"port"`
}

type StorageConfig struct {
	DBPath string `toml:"db_path"`
}

type AIConfig struct {
	BaseURL string `toml:"base_url"`
	Model   string `toml:"model"`
	APIKey  string `toml:"api_key"`
}

func Load(path string) (Config, error) {
	cfg := Default()
	path = strings.TrimSpace(path)
	if path == "" {
		path = DefaultPath
	}

	if _, err := os.Stat(path); err == nil {
		meta, err := toml.DecodeFile(path, &cfg)
		if err != nil {
			return Config{}, fmt.Errorf("读取配置文件失败: %w", err)
		}
		if undecoded := meta.Undecoded(); len(undecoded) > 0 {
			return Config{}, fmt.Errorf("配置文件包含未知字段: %s", undecoded[0].String())
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return Config{}, fmt.Errorf("读取配置文件失败: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func Default() Config {
	return Config{
		Server: ServerConfig{
			Port: defaultPort,
		},
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

func validatePort(port int) error {
	if port < 1 || port > 65535 {
		return errors.New("server.port 必须是 1 到 65535 之间的数字")
	}
	return nil
}

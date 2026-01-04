package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	envIMAPHost   = "POSTMANPAT_IMAP_HOST"
	envIMAPPort   = "POSTMANPAT_IMAP_PORT"
	envIMAPUser   = "POSTMANPAT_IMAP_USER"
	envIMAPPass   = "POSTMANPAT_IMAP_PASS"
	envDOEndpoint = "POSTMANPAT_DO_ENDPOINT"
	envDORegion   = "POSTMANPAT_DO_REGION"
	envDOBucket   = "POSTMANPAT_DO_BUCKET"
	envDOKey      = "POSTMANPAT_DO_KEY"
	envDOSecret   = "POSTMANPAT_DO_SECRET"
	envWebhookURL = "POSTMANPAT_WEBHOOK_URL"
)

// Config holds non-secret configuration loaded from YAML.
type Config struct {
	Rules      []Rule     `yaml:"rules"`
	Reporting  Reporting  `yaml:"reporting"`
	Checkpoint Checkpoint `yaml:"checkpoint"`
}

// IMAPEnv holds the IMAP connection details from environment variables.
type IMAPEnv struct {
	Host string
	Port int
	User string
	Pass string
}

// Rule describes a single cleanup rule.
type Rule struct {
	Name      string            `yaml:"name"`
	Matchers  Matchers          `yaml:"matchers"`
	Actions   []Action          `yaml:"actions"`
	Variables map[string]string `yaml:"variables"`
}

// Matchers define the matching criteria for a rule.
type Matchers struct {
	AgeDays         *int     `yaml:"age_days"`
	SenderSubstring []string `yaml:"sender_substring"`
	Recipients      []string `yaml:"recipients"`
	BodySubstring   []string `yaml:"body_substring"`
	Folders         []string `yaml:"folders"`
}

type ActionName string

const (
	DELETE  ActionName = "delete"
	UNKNOWN ActionName = ""
)

// Action defines an operation to apply when a rule matches.
type Action struct {
	Type   ActionName        `yaml:"type"`
	Params map[string]string `yaml:"params"`
}

// Reporting configures the reporting output.
type Reporting struct {
	Channel string `yaml:"channel"`
}

// Checkpoint configures checkpoint storage.
type Checkpoint struct {
	Path string `yaml:"path"`
}

// Load reads configuration from a YAML file.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// ValidateEnv ensures required environment variables are set.
func ValidateEnv() error {
	missing := []string{}
	for _, name := range requiredEnvVars() {
		if strings.TrimSpace(os.Getenv(name)) == "" {
			missing = append(missing, name)
		}
	}

	if len(missing) == 0 {
		return nil
	}

	return fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
}

// IMAPEnvFromEnv loads IMAP connection details and validates required entries.
func IMAPEnvFromEnv() (IMAPEnv, error) {
	missing := []string{}

	host := strings.TrimSpace(os.Getenv(envIMAPHost))
	if host == "" {
		missing = append(missing, envIMAPHost)
	}

	portRaw := strings.TrimSpace(os.Getenv(envIMAPPort))
	if portRaw == "" {
		missing = append(missing, envIMAPPort)
	}

	user := strings.TrimSpace(os.Getenv(envIMAPUser))
	if user == "" {
		missing = append(missing, envIMAPUser)
	}

	pass := strings.TrimSpace(os.Getenv(envIMAPPass))
	if pass == "" {
		missing = append(missing, envIMAPPass)
	}

	if len(missing) > 0 {
		return IMAPEnv{}, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}

	port, err := strconv.Atoi(portRaw)
	if err != nil {
		return IMAPEnv{}, fmt.Errorf("invalid %s: %w", envIMAPPort, err)
	}

	return IMAPEnv{
		Host: host,
		Port: port,
		User: user,
		Pass: pass,
	}, nil
}

// Summary returns a concise config summary for validation runs.
func Summary(cfg Config) string {
	return fmt.Sprintf(
		"Config summary\n"+
			"- rules: %d\n"+
			"- reporting channel: %s\n"+
			"- checkpoint path: %s",
		len(cfg.Rules),
		defaultIfEmpty(cfg.Reporting.Channel, "(not set)"),
		defaultIfEmpty(cfg.Checkpoint.Path, "(not set)"),
	)
}

func requiredEnvVars() []string {
	return []string{
		envIMAPHost,
		envIMAPPort,
		envIMAPUser,
		envIMAPPass,
		envDOEndpoint,
		envDORegion,
		envDOBucket,
		envDOKey,
		envDOSecret,
		envWebhookURL,
	}
}

func defaultIfEmpty(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

// Validate performs basic validation on non-secret config.
func Validate(cfg Config) error {
	if len(cfg.Rules) == 0 {
		return errors.New("config must define at least one rule")
	}
	for i, rule := range cfg.Rules {
		if len(rule.Matchers.Folders) == 0 {
			return fmt.Errorf("rule %d must define matchers.folders", i+1)
		}
	}
	return nil
}

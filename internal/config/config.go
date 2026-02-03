package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	envIMAPHost   = "POSTMANPAT_IMAP_HOST"
	envIMAPPort   = "POSTMANPAT_IMAP_PORT"
	envIMAPUser   = "POSTMANPAT_IMAP_USER"
	envIMAPPass   = "POSTMANPAT_IMAP_PASS"
	envS3Endpoint = "POSTMANPAT_S3_ENDPOINT"
	envS3Region   = "POSTMANPAT_S3_REGION"
	envS3Bucket   = "POSTMANPAT_S3_BUCKET"
	envS3Key      = "POSTMANPAT_S3_KEY"
	envS3Secret   = "POSTMANPAT_S3_SECRET"
	envWebhookURL = "POSTMANPAT_WEBHOOK_URL"
)

// Config holds non-secret configuration loaded from YAML.
type Config struct {
	Rules      []Rule     `yaml:"rules"`
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
	Actions   []Action          `yaml:"actions"`
	Variables map[string]string `yaml:"variables"`
	Server    *ServerMatchers   `yaml:"server"`
	Client    *ClientMatchers   `yaml:"client"`
}

type ClientMatchers struct {
	SubjectRegex      []string `yaml:"subject_regex"`
	BodyRegex         []string `yaml:"body_regex"`
	SenderRegex       []string `yaml:"sender_regex"`
	RecipientsRegex   []string `yaml:"recipients_regex"`
	ReplyToRegex      []string `yaml:"replyto_regex"`
	ListIDRegex       []string `yaml:"list_id_regex"`
	RecipientTagRegex []string `yaml:"recipient_tag_regex"`
}

func (m *ClientMatchers) IsEmpty() bool {
	if m == nil {
		return true
	}
	return len(m.SubjectRegex) == 0 &&
		len(m.BodyRegex) == 0 &&
		len(m.SenderRegex) == 0 &&
		len(m.RecipientsRegex) == 0 &&
		len(m.ReplyToRegex) == 0 &&
		len(m.ListIDRegex) == 0 &&
		len(m.RecipientTagRegex) == 0
}

type AgeWindow struct {
	Min string `yaml:"min"`
	Max string `yaml:"max"`
}

func (a *AgeWindow) IsEmpty() bool {
	if a == nil {
		return true
	}
	return strings.TrimSpace(a.Min) == "" && strings.TrimSpace(a.Max) == ""
}

// ServerMatchers define the matching criteria for a rule.
type ServerMatchers struct {
	AgeWindow        *AgeWindow `yaml:"age_window"`
	SenderSubstring  []string   `yaml:"sender_substring"`
	Recipients       []string   `yaml:"recipients"`
	BodySubstring    []string   `yaml:"body_substring"`
	ReplyToSubstring []string   `yaml:"replyto_substring"`
	ListIDSubstring  []string   `yaml:"list_id_substring"`
	Folders          []string   `yaml:"folders"`
}

func ParseRelativeDuration(value string) (time.Duration, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, nil
	}
	if strings.HasSuffix(trimmed, "d") {
		daysValue := strings.TrimSuffix(trimmed, "d")
		days, err := strconv.ParseFloat(strings.TrimSpace(daysValue), 64)
		if err != nil {
			return 0, err
		}
		if days < 0 {
			return 0, errors.New("duration must be positive")
		}
		return time.Duration(days * float64(24*time.Hour)), nil
	}
	dur, err := time.ParseDuration(trimmed)
	if err != nil {
		return 0, err
	}
	if dur < 0 {
		return 0, errors.New("duration must be positive")
	}
	return dur, nil
}

func AgeWindowBounds(now time.Time, window *AgeWindow) (string, string, error) {
	after := now.Format(time.RFC3339)
	before := ""
	if window == nil {
		return after, before, nil
	}
	if strings.TrimSpace(window.Max) != "" {
		dur, err := ParseRelativeDuration(window.Max)
		if err != nil {
			return "", "", fmt.Errorf("invalid age_window.max: %w", err)
		}
		after = now.Add(-dur).Format(time.RFC3339)
	}
	if strings.TrimSpace(window.Min) != "" {
		dur, err := ParseRelativeDuration(window.Min)
		if err != nil {
			return "", "", fmt.Errorf("invalid age_window.min: %w", err)
		}
		before = now.Add(-dur).Format(time.RFC3339)
	}
	return after, before, nil
}

type ActionName string

const (
	DELETE  ActionName = "delete"
	MOVE    ActionName = "move"
	UNKNOWN ActionName = ""
)

// Action defines an operation to apply when a rule matches.
type Action struct {
	Type               ActionName `yaml:"type"`
	Destination        string     `yaml:"destination"`
	ExpungeAfterDelete *bool      `yaml:"expunge_after_delete"`
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
	reportingStatus := "disabled"
	if ReportingEnabled() {
		reportingStatus = "enabled"
	}
	return fmt.Sprintf(
		"Config summary\n"+
			"- rules: %d\n"+
			"- reporting webhook: %s\n"+
			"- checkpoint path: %s",
		len(cfg.Rules),
		reportingStatus,
		defaultIfEmpty(cfg.Checkpoint.Path, "(not set)"),
	)
}

// ReportingEnabled returns true when a webhook URL is configured via env var.
func ReportingEnabled() bool {
	return strings.TrimSpace(os.Getenv(envWebhookURL)) != ""
}

func requiredEnvVars() []string {
	return []string{
		envIMAPHost,
		envIMAPPort,
		envIMAPUser,
		envIMAPPass,
		envS3Endpoint,
		envS3Region,
		envS3Bucket,
		envS3Key,
		envS3Secret,
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
		if rule.Server == nil && rule.Client == nil {
			return fmt.Errorf("rule %d must define server or client", i+1)
		}
		if rule.Server != nil && len(rule.Server.Folders) == 0 {
			return fmt.Errorf("rule %d must define server.folders", i+1)
		}
	}
	return nil
}

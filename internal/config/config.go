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

// ServerMatchers define the matching criteria for a rule.
type ServerMatchers struct {
	AgeDays          *int     `yaml:"age_days"`
	SenderSubstring  []string `yaml:"sender_substring"`
	Recipients       []string `yaml:"recipients"`
	BodySubstring    []string `yaml:"body_substring"`
	ReplyToSubstring []string `yaml:"replyto_substring"`
	ListIDSubstring  []string `yaml:"list_id_substring"`
	Folders          []string `yaml:"folders"`
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
		if rule.Server == nil && rule.Client == nil {
			return fmt.Errorf("rule %d must define server or client", i+1)
		}
		if rule.Server != nil && len(rule.Server.Folders) == 0 {
			return fmt.Errorf("rule %d must define server.folders", i+1)
		}
	}
	return nil
}

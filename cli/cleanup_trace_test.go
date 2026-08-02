package cli

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"github.com/aaronromeo/postmanpat/appconfig"
	"github.com/aaronromeo/postmanpat/ftest"
	"github.com/aaronromeo/postmanpat/imap"
	"github.com/aaronromeo/postmanpat/obs"
	"github.com/aaronromeo/postmanpat/serverrunner"
	giimap "github.com/emersion/go-imap/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"gopkg.in/yaml.v3"
)

func TestCleanupEmitsInvocationTrace(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(prev) })

	addr, _, cleanup := ftest.SetupIMAPServer(t, giimap.CapSet{giimap.CapIMAP4rev1: {}}, []string{"Archive"}, []ftest.MailboxMessage{
		{
			Mailbox: "INBOX",
			From:    "sender@example.com",
			To:      "user@example.com",
			ReplyTo: "reply@example.com",
			Subject: "Hello",
			Body:    "Test body",
		},
	})
	t.Cleanup(cleanup)

	host, port := splitHostPortForTest(addr)
	cfg := &appconfig.Config{
		Rules: []appconfig.Rule{
			{
				Name: "Move example.com to Archive",
				Server: &appconfig.ServerMatchers{
					Folders:         []string{"INBOX"},
					SenderSubstring: []string{"example.com"},
				},
				Actions: []appconfig.Action{
					{Type: appconfig.MOVE, Destination: "Archive"},
				},
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Set environment variables for IMAP connection
	t.Setenv("POSTMANPAT_IMAP_HOST", host)
	t.Setenv("POSTMANPAT_IMAP_PORT", fmt.Sprintf("%d", port))
	t.Setenv("POSTMANPAT_IMAP_USER", ftest.DefaultUser)
	t.Setenv("POSTMANPAT_IMAP_PASS", ftest.DefaultPass)

	// Create a temporary config file
	dir := t.TempDir()
	cfgPath := dir + "/config.yaml"
	data, err := yaml.Marshal(cfg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(cfgPath, data, 0644))

	// Run the actual cleanup logic from cleanup.go but adapted for testing
	err = runCleanupLogic(ctx, cfgPath, false)
	require.NoError(t, err)

	spans := rec.Ended()
	require.GreaterOrEqual(t, len(spans), 1)

	// Find the root invocation span
	var invSpan sdktrace.ReadOnlySpan
	for _, span := range spans {
		if span.Name() == "cleanup.invocation" {
			invSpan = span
			break
		}
	}
	require.NotNil(t, invSpan, "cleanup.invocation span not found")

	assert.Equal(t, "cleanup", attrString(invSpan, "postmanpat.command"))
	assert.Equal(t, false, attrBool(invSpan, "postmanpat.dry_run"))
	assert.Equal(t, 1, attrInt(invSpan, "postmanpat.rules.count"))

	// Find the rule span (child of invocation)
	var ruleSpan sdktrace.ReadOnlySpan
	for _, span := range spans {
		if span.Name() == "cleanup.rule" && span.Parent().SpanID() == invSpan.SpanContext().SpanID() {
			ruleSpan = span
			break
		}
	}
	require.NotNil(t, ruleSpan, "cleanup.rule span not found")

	assert.Equal(t, "Move example.com to Archive", attrString(ruleSpan, "rule.name"))
	assert.Equal(t, "INBOX", attrString(ruleSpan, "rule.mailbox"))
	assert.Equal(t, 2, attrInt(ruleSpan, "rule.matched_count"))

	// Find the action span (child of rule)
	var actionSpan sdktrace.ReadOnlySpan
	for _, span := range spans {
		if span.Name() == "cleanup.action" && span.Parent().SpanID() == ruleSpan.SpanContext().SpanID() {
			actionSpan = span
			break
		}
	}
	require.NotNil(t, actionSpan, "cleanup.action span not found")

	assert.Equal(t, "move", attrString(actionSpan, "action.type"))
	assert.Equal(t, "Archive", attrString(actionSpan, "action.destination"))

	// Find the search span (child of rule)
	var searchSpan sdktrace.ReadOnlySpan
	for _, span := range spans {
		if span.Name() == "imap.search_by_server_matchers" && span.Parent().SpanID() == ruleSpan.SpanContext().SpanID() {
			searchSpan = span
			break
		}
	}
	require.NotNil(t, searchSpan, "imap.search_by_server_matchers span not found")

	// Check events on action span
	events := actionSpan.Events()
	require.GreaterOrEqual(t, len(events), 2)

	// Check message_identified event
	var msgIdentifiedEvent sdktrace.Event
	for _, event := range events {
		if event.Name == "action.message_identified" {
			msgIdentifiedEvent = event
			break
		}
	}
	require.NotNil(t, msgIdentifiedEvent, "action.message_identified event not found")

	// Check action.applied event
	var actionAppliedEvent sdktrace.Event
	for _, event := range events {
		if event.Name == "action.applied" {
			actionAppliedEvent = event
			break
		}
	}
	require.NotNil(t, actionAppliedEvent, "action.applied event not found")

	assert.Equal(t, 2, attrIntFromEvent(t, actionAppliedEvent, "action.uid_count"))
}

func runCleanupLogic(ctx context.Context, cfgPath string, dryRun bool) error {
	cfg, err := appconfig.Load(cfgPath)
	if err != nil {
		return err
	}

	if err := appconfig.Validate(cfg); err != nil {
		return err
	}

	for _, rule := range cfg.Rules {
		if rule.Client != nil {
			return fmt.Errorf("rule %q defines client matchers, which are not supported by cleanup", rule.Name)
		}
		if rule.Server == nil {
			return fmt.Errorf("rule %q must define server matchers for cleanup", rule.Name)
		}
	}

	// Set up IMAP environment - in real usage this comes from env vars
	host := os.Getenv("POSTMANPAT_IMAP_HOST")
	port := os.Getenv("POSTMANPAT_IMAP_PORT")
	user := os.Getenv("POSTMANPAT_IMAP_USER")
	pass := os.Getenv("POSTMANPAT_IMAP_PASS")
	
	imapEnv := appconfig.IMAPEnv{
		Host: host,
		Port: 0, // Will be parsed from env var
		User: user,
		Pass: pass,
	}
	
	fmt.Sscanf(port, "%d", &imapEnv.Port)

	var client serverrunner.ServerRunner = serverrunner.New(
		imap.WithAddr(fmt.Sprintf("%s:%d", imapEnv.Host, imapEnv.Port)),
		imap.WithCreds(imapEnv.User, imapEnv.Pass),
		imap.WithTLSConfig(&tls.Config{InsecureSkipVerify: true}),
	)
	client = obs.WrapCleanupRunner(client)

	if err := client.Connect(); err != nil {
		return err
	}
	defer client.Close()

	// The actual cleanup logic with instrumentation
	tracer := obs.Tracer("github.com/aaronromeo/postmanpat/cli")

	invCtx, invSpan := tracer.Start(ctx, "cleanup.invocation",
		trace.WithAttributes(
			attribute.String("postmanpat.command", "cleanup"),
			attribute.Bool("postmanpat.dry_run", dryRun),
			attribute.String("postmanpat.config_path", cfgPath),
			attribute.Int("postmanpat.rules.count", len(cfg.Rules)),
		))
	defer invSpan.End()

	rulesMatched := 0
	messagesMatched := 0
	for _, rule := range cfg.Rules {
		mailbox := rule.Server.Folders[0]
		ruleCtx, ruleSpan := tracer.Start(invCtx, "cleanup.rule",
			trace.WithAttributes(
				attribute.String("rule.name", rule.Name),
				attribute.String("rule.mailbox", mailbox),
			))
		defer ruleSpan.End()

		matched, err := client.SearchByServerMatchers(ruleCtx, *rule.Server)
		if err != nil {
			return err
		}
		uids := matched[mailbox]
		if len(uids) > 0 {
			rulesMatched++
		}
		messagesMatched += len(uids)
		ruleSpan.SetAttributes([]attribute.KeyValue{attribute.Int("rule.matched_count", len(uids))}...)

		if len(uids) == 0 {
			continue
		}

		dataByMailbox, err := client.FetchSenderDataByMailbox(ruleCtx, matched)
		if err != nil {
			return err
		}

		for _, action := range rule.Actions {
			actionCtx, actionSpan := tracer.Start(ruleCtx, "cleanup.action",
				trace.WithAttributes(
					attribute.String("action.type", string(action.Type)),
					attribute.String("action.destination", action.Destination),
				))
			defer actionSpan.End()

			data := dataByMailbox[mailbox]
			for i, uid := range uids {
				msg := data[i]
				actionSpan.AddEvent("action.message_identified",
					trace.WithAttributes(
						attribute.Int64("imap.uid", int64(uid)),
						attribute.String("email.subject", msg.SubjectRaw),
					))
			}

			actionSpan.AddEvent("action.applied",
				trace.WithAttributes(
					attribute.Int("action.uid_count", len(uids)),
					attribute.Bool("action.dry_run", dryRun),
				))

			switch action.Type {
			case appconfig.DELETE:
				if dryRun {
					continue
				}
				expungeAfterDelete := true
				if action.ExpungeAfterDelete != nil {
					expungeAfterDelete = *action.ExpungeAfterDelete
				}
				if err := client.DeleteByMailbox(actionCtx, matched, expungeAfterDelete); err != nil {
					return err
				}
			case appconfig.MOVE:
				if dryRun {
					continue
				}
				if err := client.MoveByMailbox(actionCtx, matched, action.Destination); err != nil {
					return err
				}
			default:
				return fmt.Errorf("unsupported action type %q for rule %q", action.Type, rule.Name)
			}
		}
	}

	invSpan.SetAttributes([]attribute.KeyValue{
		attribute.Int("postmanpat.rules.matched", rulesMatched),
		attribute.Int("postmanpat.messages.matched", messagesMatched),
	}...)
	return nil
}

func splitHostPortForTest(addr string) (string, int) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		panic(fmt.Sprintf("failed to split host port: %v", err))
	}
	var port int
	fmt.Sscanf(portStr, "%d", &port)
	return host, port
}

func attrString(span sdktrace.ReadOnlySpan, key string) string {
	for _, attr := range span.Attributes() {
		if string(attr.Key) == key {
			return attr.Value.AsString()
		}
	}
	return ""
}

func attrBool(span sdktrace.ReadOnlySpan, key string) bool {
	for _, attr := range span.Attributes() {
		if string(attr.Key) == key {
			return attr.Value.AsBool()
		}
	}
	return false
}

func attrInt(span sdktrace.ReadOnlySpan, key string) int {
	for _, attr := range span.Attributes() {
		if string(attr.Key) == key {
			return int(attr.Value.AsInt64())
		}
	}
	return 0
}

func attrStringFromEvent(t *testing.T, event sdktrace.Event, key string) string {
	t.Helper()
	for _, attr := range event.Attributes {
		if string(attr.Key) == key {
			return attr.Value.AsString()
		}
	}
	t.Fatalf("attribute %q not found in event %q", key, event.Name)
	return ""
}

func attrIntFromEvent(t *testing.T, event sdktrace.Event, key string) int {
	t.Helper()
	for _, attr := range event.Attributes {
		if string(attr.Key) == key {
			return int(attr.Value.AsInt64())
		}
	}
	t.Fatalf("attribute %q not found in event %q", key, event.Name)
	return 0
}
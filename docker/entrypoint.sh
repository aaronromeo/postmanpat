#!/bin/sh
set -eu

if [ -z "${POSTMANPAT_CONFIG:-}" ]; then
  echo "POSTMANPAT_CONFIG is required" >&2
  exit 1
fi

{
  printf 'SHELL=/bin/sh\n'
  printf 'PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin\n'
  printf 'POSTMANPAT_CONFIG=%s\n' "$POSTMANPAT_CONFIG"
  printf 'POSTMANPAT_IMAP_HOST=%s\n' "$POSTMANPAT_IMAP_HOST"
  printf 'POSTMANPAT_IMAP_PORT=%s\n' "$POSTMANPAT_IMAP_PORT"
  printf 'POSTMANPAT_IMAP_USER=%s\n' "$POSTMANPAT_IMAP_USER"
  printf 'POSTMANPAT_IMAP_PASS=%s\n' "$POSTMANPAT_IMAP_PASS"
  printf 'POSTMANPAT_S3_ENDPOINT=%s\n' "$POSTMANPAT_S3_ENDPOINT"
  printf 'POSTMANPAT_S3_REGION=%s\n' "$POSTMANPAT_S3_REGION"
  printf 'POSTMANPAT_S3_BUCKET=%s\n' "$POSTMANPAT_S3_BUCKET"
  printf 'POSTMANPAT_S3_KEY=%s\n' "$POSTMANPAT_S3_KEY"
  printf 'POSTMANPAT_S3_SECRET=%s\n' "$POSTMANPAT_S3_SECRET"
  [ -n "${OTEL_EXPORTER_OTLP_ENDPOINT:-}" ] && printf 'OTEL_EXPORTER_OTLP_ENDPOINT=%s\n' "$OTEL_EXPORTER_OTLP_ENDPOINT"
  [ -n "${OTEL_EXPORTER_OTLP_INSECURE:-}" ] && printf 'OTEL_EXPORTER_OTLP_INSECURE=%s\n' "$OTEL_EXPORTER_OTLP_INSECURE"
  [ -n "${OTEL_EXPORTER_OTLP_HEADERS:-}" ] && printf 'OTEL_EXPORTER_OTLP_HEADERS=%s\n' "$OTEL_EXPORTER_OTLP_HEADERS"
  [ -n "${OTEL_SERVICE_NAME:-}" ] && printf 'OTEL_SERVICE_NAME=%s\n' "$OTEL_SERVICE_NAME"
  [ -n "${OTEL_SDK_DISABLED:-}" ] && printf 'OTEL_SDK_DISABLED=%s\n' "$OTEL_SDK_DISABLED"
  printf 'POSTMANPAT_WEBHOOK_URL=%s\n' "$POSTMANPAT_WEBHOOK_URL"
  printf '\n'
  printf '0 * * * * /usr/local/bin/postmanpat cleanup --config "$POSTMANPAT_CONFIG" >>/proc/1/fd/1 2>>/proc/1/fd/2\n'
  if [ -n "${POSTMANPAT_ANALYZE_CONFIG:-}" ]; then
    printf '30 3 * * * /usr/local/bin/postmanpat analyze --config /config/config_analyze.yaml --out /analyze-out --min-count 1 >>/proc/1/fd/1 2>>/proc/1/fd/2\n'
  fi
} >/etc/cron.d/postmanpat

chmod 0644 /etc/cron.d/postmanpat
crontab /etc/cron.d/postmanpat

exec cron -f

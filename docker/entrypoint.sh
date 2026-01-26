#!/bin/sh
set -eu

if [ -z "${POSTMANPAT_CONFIG:-}" ]; then
  echo "POSTMANPAT_CONFIG is required" >&2
  exit 1
fi

cat >/etc/cron.d/postmanpat <<EOF
SHELL=/bin/sh
PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
POSTMANPAT_CONFIG=${POSTMANPAT_CONFIG}

*/15 * * * * root /usr/local/bin/postmanpat cleanup --config "$POSTMANPAT_CONFIG" >>/proc/1/fd/1 2>>/proc/1/fd/2
EOF

chmod 0644 /etc/cron.d/postmanpat
crontab /etc/cron.d/postmanpat

exec cron -f

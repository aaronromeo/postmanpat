#!/bin/bash

# Setup script for Dokku app configuration
# This script is idempotent and can be run multiple times safely

set -e

# Load environment variables from .env file if it exists
if [ -f .env ]; then
    echo "📄 Loading environment variables from .env file"
    export $(cat .env | grep -v '^#' | grep -v '^$' | xargs)
fi

APP_NAME="postmanpat"
DOMAIN="postmanpat.overachieverlabs.com"

echo "🔧 Setting up Dokku app: $APP_NAME"

# Create app if it doesn't exist
if ! ssh dokku-admin "dokku apps:exists $APP_NAME" 2>/dev/null; then
    echo "📱 Creating Dokku app: $APP_NAME"
    ssh dokku-admin "dokku apps:create $APP_NAME"
else
    echo "✅ App $APP_NAME already exists"
fi

# Configure domain
echo "🌐 Configuring domain: $DOMAIN"
if ! ssh dokku-admin "dokku domains:report $APP_NAME" | grep -q "$DOMAIN"; then
    ssh dokku-admin "dokku domains:add $APP_NAME $DOMAIN"
    echo "✅ Domain $DOMAIN added"
else
    echo "✅ Domain $DOMAIN already configured"
fi

# Configure buildpack (Go buildpack for this application)
echo "🔧 Configuring buildpack"
ssh dokku-admin "dokku buildpacks:clear $APP_NAME" || echo "⚠️  No existing buildpacks to clear"
ssh dokku-admin "dokku buildpacks:set $APP_NAME https://github.com/heroku/heroku-buildpack-go.git"

# Configure ports
echo "🔌 Configuring ports"
ssh dokku-admin "dokku config:set $APP_NAME PORT=3000"
ssh dokku-admin "dokku ports:set $APP_NAME http:80:3000"
ssh dokku-admin "dokku ports:set $APP_NAME https:443:3000"

# Set environment variables from GitHub secrets
echo "🔐 Setting environment variables"

# Set sensitive environment variables (from GitHub secrets)
if [ -n "$IMAP_URL" ]; then
    ssh dokku-admin "dokku config:set --no-restart $APP_NAME IMAP_URL='$IMAP_URL'"
fi

if [ -n "$IMAP_USER" ]; then
    ssh dokku-admin "dokku config:set --no-restart $APP_NAME IMAP_USER='$IMAP_USER'"
fi

if [ -n "$IMAP_PASS" ]; then
    ssh dokku-admin "dokku config:set --no-restart $APP_NAME IMAP_PASS='$IMAP_PASS'"
fi

if [ -n "$DIGITALOCEAN_BUCKET_ACCESS_KEY" ]; then
    ssh dokku-admin "dokku config:set --no-restart $APP_NAME DIGITALOCEAN_BUCKET_ACCESS_KEY='$DIGITALOCEAN_BUCKET_ACCESS_KEY'"
fi

if [ -n "$DIGITALOCEAN_BUCKET_SECRET_KEY" ]; then
    ssh dokku-admin "dokku config:set --no-restart $APP_NAME DIGITALOCEAN_BUCKET_SECRET_KEY='$DIGITALOCEAN_BUCKET_SECRET_KEY'"
fi

if [ -n "$UPTRACE_DSN" ]; then
    ssh dokku-admin "dokku config:set --no-restart $APP_NAME UPTRACE_DSN='$UPTRACE_DSN'"
fi

# Set the PORT environment variable to ensure correct port detection
echo "🔧 Setting PORT environment variable..."
ssh dokku-admin "dokku config:set --no-restart $APP_NAME PORT=3000"

# Configure Let's Encrypt (optional)
if [ "$ENABLE_LETSENCRYPT" = "true" ]; then
    echo "🔒 Setting up Let's Encrypt SSL"

    # Set Let's Encrypt email if provided
    if [ -n "$LETSENCRYPT_EMAIL" ]; then
        echo "📧 Setting Let's Encrypt email: $LETSENCRYPT_EMAIL"
        ssh dokku-admin "dokku letsencrypt:set $APP_NAME email $LETSENCRYPT_EMAIL"
    else
        echo "⚠️  No LETSENCRYPT_EMAIL provided, using default"
    fi

    # Check if Let's Encrypt is already enabled
    echo "🔍 Checking Let's Encrypt status..."
    # Use the dedicated active check command for more reliable detection
    if ssh dokku-admin "dokku letsencrypt:active $APP_NAME" >/dev/null 2>&1; then
        echo "✅ Let's Encrypt SSL certificate is already active for $APP_NAME"
        echo "� Current SSL Certificate Status:"
        cert_info=$(ssh dokku-admin "dokku letsencrypt:list" | grep "$APP_NAME")
        echo "$cert_info"

        # Check if certificate expires within 21 days and renew proactively
        echo "🔍 Checking certificate expiry status..."

        # Parse the days from the "Time before expiry" column (format: "89d, 22h, 35m, 5s")
        if echo "$cert_info" | grep -q "[0-9]\+d,"; then
            days_to_expiry=$(echo "$cert_info" | grep -o '[0-9]\+d,' | head -1 | grep -o '[0-9]\+')

            if [ "$days_to_expiry" -le 21 ] 2>/dev/null; then
                echo "⚠️ Certificate expires in $days_to_expiry days - renewing proactively (threshold: 21 days)"
                echo "🔄 Renewing Let's Encrypt certificate..."

                if ssh dokku-admin "dokku letsencrypt:enable $APP_NAME"; then
                    echo "✅ Certificate renewed successfully"
                    echo "📋 Updated SSL Certificate Status:"
                    ssh dokku-admin "dokku letsencrypt:list" | grep "$APP_NAME"
                else
                    echo "❌ Certificate renewal failed"
                    echo "⚠️ Certificate still expires in $days_to_expiry days - manual intervention needed"
                fi
            else
                echo "✅ Certificate is healthy - expires in $days_to_expiry days (renewal threshold: 21 days)"
            fi
        else
            echo "⚠️ Could not parse certificate expiry information"
            echo "📋 Raw certificate info: $cert_info"
        fi
    else
        echo "�🔒 Enabling Let's Encrypt SSL certificate"
        if ssh dokku-admin "dokku letsencrypt:enable $APP_NAME"; then
            echo "✅ Let's Encrypt SSL certificate enabled successfully"

            # Verify SSL certificate status
            echo "📋 SSL Certificate Status:"
            ssh dokku-admin "dokku letsencrypt:list"
        else
            echo "❌ Let's Encrypt setup failed"
            echo "🔍 Checking app status for troubleshooting:"
            ssh dokku-admin "dokku ps:report $APP_NAME"
            echo "🔍 Checking domain configuration:"
            ssh dokku-admin "dokku domains:report $APP_NAME"
            echo "⚠️  Continuing without SSL - manual intervention may be required"
        fi
    fi
fi

echo "✅ Dokku app setup complete"

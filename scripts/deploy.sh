#!/bin/bash

# Deployment script for Dokku
# Handles Git-based deployment with health checks and rollback

set -e

APP_NAME="postmanpat"
REMOTE_NAME="dokku"
DOKKU_HOST="dokku-admin"

echo "🚀 Starting deployment to Dokku"

# Add Dokku remote if it doesn't exist
if ! git remote get-url $REMOTE_NAME 2>/dev/null; then
    echo "📡 Adding Dokku remote"
    git remote add $REMOTE_NAME dokku@overachieverlabs.com:$APP_NAME
else
    echo "✅ Dokku remote already exists"
fi

# Get current deployment info for potential rollback
echo "📊 Getting current deployment info"
CURRENT_RELEASE=$(ssh $DOKKU_HOST "dokku ps:report $APP_NAME" | grep "Running:" | awk '{print $2}' || echo "false")

echo "Current app running status: $CURRENT_RELEASE"

# Deploy to Dokku
echo "🔄 Deploying to Dokku..."
if git push $REMOTE_NAME main:master; then
    echo "✅ Git push successful"
else
    echo "❌ Git push failed"
    exit 1
fi

# Wait for deployment to complete
echo "⏳ Waiting for deployment to complete..."
sleep 15

# Check if deployment was successful
echo "🔍 Checking deployment status..."
DEPLOY_STATUS=$(ssh $DOKKU_HOST "dokku ps:report $APP_NAME" | grep "Running:" | awk '{print $2}' || echo "false")

if [ "$DEPLOY_STATUS" != "true" ]; then
    echo "❌ Deployment failed - app is not running"
    
    # Attempt rollback if there was a previous deployment
    if [ "$CURRENT_RELEASE" = "true" ]; then
        echo "🔄 Attempting rollback..."
        ssh $DOKKU_HOST "dokku ps:rollback $APP_NAME" || echo "⚠️  Rollback failed"
    fi
    
    exit 1
fi

echo "✅ Deployment completed successfully"

# Additional verification will be done in the GitHub Actions workflow
echo "🎉 Deployment script completed"

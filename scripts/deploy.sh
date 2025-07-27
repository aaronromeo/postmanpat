#!/bin/bash

# Deployment script for Dokku
# Handles Git-based deployment with health checks and rollback

set -e

APP_NAME="postmanpat"
REMOTE_NAME="dokku"
DOKKU_HOST="dokku-admin"

echo "🚀 Starting deployment to Dokku"

# Verify Dokku app exists before proceeding
echo "🔍 Verifying Dokku app exists..."
if ! ssh $DOKKU_HOST "dokku apps:exists $APP_NAME" 2>/dev/null; then
    echo "❌ Dokku app '$APP_NAME' does not exist"
    echo "Please ensure the setup script has been run successfully"
    exit 1
else
    echo "✅ Dokku app '$APP_NAME' exists"
fi

# Add Dokku remote if it doesn't exist
if ! git remote get-url $REMOTE_NAME 2>/dev/null; then
    echo "📡 Adding Dokku remote"
    git remote add $REMOTE_NAME root@overachieverlabs.com:$APP_NAME
else
    echo "✅ Dokku remote already exists"
fi

# Get current deployment info for potential rollback
echo "📊 Getting current deployment info"
CURRENT_RELEASE=$(ssh $DOKKU_HOST "dokku ps:report $APP_NAME" | grep "Running:" | awk '{print $2}' || echo "false")

echo "Current app running status: $CURRENT_RELEASE"

# Get current branch name
CURRENT_BRANCH=$(git branch --show-current)
echo "📋 Current branch: $CURRENT_BRANCH"

# Test Git remote connectivity
echo "🔗 Testing Git remote connectivity..."
if git ls-remote $REMOTE_NAME HEAD >/dev/null 2>&1; then
    echo "✅ Git remote is accessible"
else
    echo "❌ Cannot access Git remote"
    echo "🔍 Debugging information:"
    echo "Remote URL: $(git remote get-url $REMOTE_NAME)"
    echo "Testing SSH connection to Dokku host..."
    ssh -o ConnectTimeout=10 $DOKKU_HOST "echo 'SSH connection test successful'" || echo "SSH connection failed"
    exit 1
fi

# Deploy to Dokku
echo "🔄 Deploying to Dokku..."
if git push --force $REMOTE_NAME $CURRENT_BRANCH:master; then
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

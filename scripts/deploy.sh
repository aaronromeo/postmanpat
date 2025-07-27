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

# Verify SSH key exists and has correct permissions
echo "🔐 Verifying SSH key setup..."
if [ ! -f ~/.ssh/dokku_key ]; then
    echo "❌ SSH key ~/.ssh/dokku_key not found"
    exit 1
fi

key_perms=$(stat -c '%a' ~/.ssh/dokku_key 2>/dev/null || echo "unknown")
if [ "$key_perms" != "600" ]; then
    echo "⚠️ SSH key permissions are $key_perms, should be 600. Fixing..."
    chmod 600 ~/.ssh/dokku_key
fi
echo "✅ SSH key verified (permissions: $(stat -c '%a' ~/.ssh/dokku_key))"

# Verify known_hosts entry
echo "🔍 Verifying known_hosts entry..."
if ! grep -q "overachieverlabs.com" ~/.ssh/known_hosts 2>/dev/null; then
    echo "⚠️ overachieverlabs.com not in known_hosts, adding..."
    ssh-keyscan -H overachieverlabs.com >> ~/.ssh/known_hosts
else
    echo "✅ overachieverlabs.com found in known_hosts"
fi

# Configure Git to use SSH properly
echo "🔧 Configuring Git SSH settings..."
export GIT_SSH_COMMAND="ssh -i ~/.ssh/dokku_key -o IdentitiesOnly=yes -o StrictHostKeyChecking=yes"

# Test Git remote connectivity
echo "🔗 Testing Git remote connectivity..."
if git ls-remote $REMOTE_NAME HEAD >/dev/null 2>&1; then
    echo "✅ Git remote is accessible"
else
    echo "❌ Cannot access Git remote"
    echo "🔍 Debugging information:"
    echo "Remote URL: $(git remote get-url $REMOTE_NAME)"
    echo "GIT_SSH_COMMAND: $GIT_SSH_COMMAND"
    echo "Testing SSH connection to Dokku host..."
    ssh -o ConnectTimeout=10 $DOKKU_HOST "echo 'SSH connection test successful'" || echo "SSH connection failed"
    echo "Testing direct SSH to Git remote host..."
    ssh -i ~/.ssh/dokku_key -o IdentitiesOnly=yes -o StrictHostKeyChecking=yes -o ConnectTimeout=10 root@overachieverlabs.com "echo 'Direct SSH to Git host successful'" || echo "Direct SSH to Git host failed"
    exit 1
fi

# Deploy to Dokku
echo "🔄 Deploying to Dokku..."
echo "Using GIT_SSH_COMMAND: $GIT_SSH_COMMAND"
if GIT_SSH_COMMAND="$GIT_SSH_COMMAND" git push --force $REMOTE_NAME $CURRENT_BRANCH:master; then
    echo "✅ Git push successful"
else
    echo "❌ Git push failed"
    echo "🔍 Additional debugging:"
    echo "Checking if SSH key exists: $(ls -la ~/.ssh/dokku_key 2>/dev/null || echo 'SSH key not found')"
    echo "SSH key permissions: $(stat -c '%a' ~/.ssh/dokku_key 2>/dev/null || echo 'Cannot check permissions')"
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

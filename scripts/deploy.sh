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
DOKKU_REMOTE_URL="dokku@overachieverlabs.com:$APP_NAME"

if ! git remote get-url $REMOTE_NAME 2>/dev/null; then
    echo "📡 Adding Dokku remote"
    git remote add $REMOTE_NAME $DOKKU_REMOTE_URL
    echo "Added remote '$REMOTE_NAME' with URL: $DOKKU_REMOTE_URL"
else
    current_url=$(git remote get-url $REMOTE_NAME)
    echo "✅ Dokku remote already exists with URL: $current_url"

    # Ensure we're using the standard Dokku format
    if [[ "$current_url" != "$DOKKU_REMOTE_URL" ]]; then
        echo "⚠️ Updating remote URL to standard Dokku format..."
        git remote set-url $REMOTE_NAME $DOKKU_REMOTE_URL
        echo "Updated remote URL to: $DOKKU_REMOTE_URL"
    fi
fi

# Get current deployment info for potential rollback
echo "📊 Getting current deployment info"
CURRENT_RELEASE=$(ssh $DOKKU_HOST "dokku ps:report $APP_NAME" | grep "Running:" | awk '{print $2}' || echo "false")

echo "Current app running status: $CURRENT_RELEASE"

# Get current branch name with fallback for detached HEAD
CURRENT_BRANCH=$(git branch --show-current 2>/dev/null)

# If we're in detached HEAD state (common in CI), use HEAD
if [ -z "$CURRENT_BRANCH" ]; then
    echo "📋 Detached HEAD detected, using HEAD for deployment"
    CURRENT_BRANCH="HEAD"

    # Show what commit we're deploying
    CURRENT_COMMIT=$(git rev-parse HEAD 2>/dev/null | cut -c1-8)
    echo "📋 Deploying commit: $CURRENT_COMMIT"
else
    echo "📋 Current branch: $CURRENT_BRANCH"
fi

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
export GIT_SSH_COMMAND="ssh -i ~/.ssh/dokku_key -o IdentitiesOnly=yes -o StrictHostKeyChecking=yes -o User=dokku"

# Test Git remote connectivity
echo "🔗 Testing Git remote connectivity..."
echo "Current remote URL: $(git remote get-url $REMOTE_NAME 2>/dev/null || echo 'Remote not found')"

# Test if we can access the Dokku Git repository
echo "🔍 Testing Dokku Git repository access..."
if ssh -i ~/.ssh/dokku_key -o IdentitiesOnly=yes -o StrictHostKeyChecking=yes -o ConnectTimeout=10 root@overachieverlabs.com "dokku apps:exists $APP_NAME"; then
    echo "✅ Dokku app exists and is accessible via SSH"
else
    echo "❌ Cannot access Dokku app via SSH"
    exit 1
fi

# Try to list the Git repository
echo "🔍 Testing Git repository structure..."
if ssh -i ~/.ssh/dokku_key -o IdentitiesOnly=yes -o StrictHostKeyChecking=yes -o ConnectTimeout=10 root@overachieverlabs.com "ls -la /home/dokku/$APP_NAME 1>/dev/null"; then
    echo "✅ Dokku app directory exists"
else
    echo "❌ Dokku app directory not found"
    exit 1
fi

# Test Git ls-remote with detailed debugging
echo "🔍 Testing Git ls-remote with debugging..."
if GIT_SSH_COMMAND="ssh -i ~/.ssh/dokku_key -o IdentitiesOnly=yes -o StrictHostKeyChecking=yes -v" git ls-remote $REMOTE_NAME HEAD 1>/dev/null 2>&1; then
    echo "✅ Git remote is accessible"
else
    echo "❌ Git ls-remote failed"
    echo "🔍 Trying alternative remote URL format..."

    # Try standard Dokku Git URL format
    echo "🔍 Trying standard Dokku Git URL format..."

    DOKKU_REMOTE_URL="dokku@overachieverlabs.com:$APP_NAME"
    echo "Testing: $DOKKU_REMOTE_URL"

    if GIT_SSH_COMMAND="ssh -i ~/.ssh/dokku_key -o IdentitiesOnly=yes -o StrictHostKeyChecking=yes -o User=dokku" git ls-remote $DOKKU_REMOTE_URL HEAD >/dev/null 2>&1; then
        echo "✅ Standard Dokku format works, updating remote..."
        git remote set-url $REMOTE_NAME $DOKKU_REMOTE_URL
        echo "Updated remote URL to: $DOKKU_REMOTE_URL"
    else
        echo "❌ Standard Dokku Git URL format failed"
        echo "💡 Check if Dokku is properly configured on the server"
        exit 1
    fi
fi

# Deploy to Dokku
echo "🔄 Deploying to Dokku..."
echo "Using GIT_SSH_COMMAND: $GIT_SSH_COMMAND"

# Attempt deployment with deploy lock handling
echo "Attempting Git push..."
deploy_output=$(GIT_SSH_COMMAND="$GIT_SSH_COMMAND" git push --force $REMOTE_NAME $CURRENT_BRANCH:master 2>&1)
deploy_exit_code=$?

if [ $deploy_exit_code -eq 0 ]; then
    echo "✅ Git push successful"
else
    echo "❌ Git push failed (exit code: $deploy_exit_code)"
    echo "🔍 Deploy output:"
    echo "$deploy_output"
    echo ""
    echo "🔍 Debug information:"
    echo "Branch being pushed: $CURRENT_BRANCH"
    echo "Remote name: $REMOTE_NAME"
    echo "Remote URL: $(git remote get-url $REMOTE_NAME 2>/dev/null || echo 'Unknown')"

    # Check if it's a deploy lock issue
    if echo "$deploy_output" | grep -q "deploy lock in place"; then
        echo "� Deploy lock detected - another deployment is in progress"
        echo "❌ Cannot proceed with deployment while lock is active"
        echo "💡 This usually means another deployment is currently running"
        echo "💡 Wait for the current deployment to complete, or if it's stuck:"
        echo "💡   ssh $DOKKU_HOST 'dokku apps:unlock $APP_NAME'"
        exit 1
    else
        echo "�🔍 Additional debugging:"
        echo "Checking if SSH key exists: $(ls -la ~/.ssh/dokku_key 2>/dev/null || echo 'SSH key not found')"
        echo "SSH key permissions: $(stat -f '%A' ~/.ssh/dokku_key 2>/dev/null || echo 'Cannot check permissions')"
        exit 1
    fi
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

#!/bin/bash
# Setup script for pdfer development
# Run this after cloning the repository

set -e

echo "Setting up pdfer development environment..."

# Configure git hooks
echo "Configuring git hooks..."
git config core.hooksPath .githooks
echo "✓ Git hooks configured"

# Install dependencies
echo "Installing dependencies..."
go mod download
echo "✓ Dependencies installed"

# Verify build
echo "Verifying build..."
go build ./...
echo "✓ Build successful"

# Run tests
echo "Running tests..."
go test ./...
echo "✓ All tests passed"

echo ""
echo "✅ Setup complete! You're ready to contribute."
echo ""
echo "Git hooks installed:"
echo "  - pre-commit: Runs formatting and vet checks"
echo "  - pre-push: Runs full test suite"

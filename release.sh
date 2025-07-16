#!/bin/bash

# Release script for Caddy Serverless Plugin

set -e

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

# Get version from argument
VERSION=$1

if [ -z "$VERSION" ]; then
    echo -e "${RED}Error: Version not provided${NC}"
    echo "Usage: ./release.sh v0.1.0"
    exit 1
fi

# Validate version format
if ! [[ $VERSION =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo -e "${RED}Error: Invalid version format${NC}"
    echo "Version should be in format: v0.1.0"
    exit 1
fi

echo -e "${GREEN}Preparing release $VERSION${NC}"

# Check if we're on main/master branch
CURRENT_BRANCH=$(git branch --show-current)
if [[ "$CURRENT_BRANCH" != "main" && "$CURRENT_BRANCH" != "master" ]]; then
    echo -e "${YELLOW}Warning: Not on main/master branch (current: $CURRENT_BRANCH)${NC}"
    read -p "Continue anyway? (y/N) " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        exit 1
    fi
fi

# Check for uncommitted changes
if ! git diff-index --quiet HEAD --; then
    echo -e "${RED}Error: Uncommitted changes found${NC}"
    echo "Please commit or stash your changes before releasing"
    exit 1
fi

# Run tests
echo -e "${GREEN}Running tests...${NC}"
if command -v go &> /dev/null; then
    go test -v ./... || {
        echo -e "${RED}Tests failed!${NC}"
        exit 1
    }
else
    echo -e "${YELLOW}Go not installed, skipping tests${NC}"
fi

# Update version in go.mod if needed
echo -e "${GREEN}Checking go.mod...${NC}"
if grep -q "module github.com/jose/caddy-serverless" go.mod; then
    echo "go.mod module path is correct"
else
    echo -e "${YELLOW}Warning: go.mod module path may need updating${NC}"
fi

# Create tag
echo -e "${GREEN}Creating git tag $VERSION...${NC}"
git tag -a "$VERSION" -m "Release $VERSION"

echo -e "${GREEN}Release $VERSION prepared successfully!${NC}"
echo
echo "Next steps:"
echo "1. Push the tag: git push origin $VERSION"
echo "2. Push commits: git push origin $CURRENT_BRANCH"
echo "3. Create release on GitHub with the following command:"
echo
echo "gh release create $VERSION \\"
echo "  --title \"Release $VERSION\" \\"
echo "  --notes-file RELEASE_NOTES_${VERSION}.md \\"
echo "  --draft"
echo
echo "4. Review and publish the draft release on GitHub"
echo "5. Announce in Caddy community forums"
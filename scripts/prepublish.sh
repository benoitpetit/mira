#!/bin/bash

# Pre-publish script for MIRA
# Usage: ./scripts/prepublish.sh [version]
# Example: ./scripts/prepublish.sh 0.1.1

set -e  # Exit on error

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
cd "$PROJECT_DIR"

# Get version from argument or prompt
if [ -z "$1" ]; then
    echo -e "${BLUE}Current version:$(grep -h "version.*0\." config.yaml config/config.go 2>/dev/null | head -1 | sed 's/.*version.*"\([0-9.]*\)".*/\1/')${NC}"
    read -p "Enter new version (e.g., 0.1.1): " VERSION
else
    VERSION="$1"
fi

# Validate version format
if ! [[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo -e "${RED}Error: Version must be in format X.Y.Z (e.g., 0.1.1)${NC}"
    exit 1
fi

echo -e "\n${BLUE}═══════════════════════════════════════════════════${NC}"
echo -e "${BLUE}  MIRA Pre-publish Script v${VERSION}${NC}"
echo -e "${BLUE}═══════════════════════════════════════════════════${NC}\n"

# Store old version for comparison
OLD_VERSION=$(grep -oP 'Version:.*"\K[0-9.]+' config/config.go 2>/dev/null | head -1 || echo "unknown")

echo -e "${YELLOW}Step 1/6: Updating version from ${OLD_VERSION} to ${VERSION}...${NC}"

# Update version in config.go
sed -i "s/Version:.*\"[0-9.]*\"/Version:              \"${VERSION}\"/g" config/config.go
sed -i "s/Version:.*\"[0-9.]*\"/Version:        \"${VERSION}\"/g" config/config.go

# Update version in cmd/mira/main.go
sed -i "s/MIRA v[0-9.]* /MIRA v${VERSION} /g" cmd/mira/main.go

# Update version in config files (example + local if exists)
sed -i "s/version: \"[0-9.]*\"/version: \"${VERSION}\"/g" config.example.yaml
if [ -f "config.yaml" ]; then
    sed -i "s/version: \"[0-9.]*\"/version: \"${VERSION}\"/g" config.yaml
fi

# Update version in config_test.go
sed -i "s/Expected version [0-9.]*/Expected version ${VERSION}/g" config/config_test.go
sed -i "s/!= \"[0-9.]*\"/!= \"${VERSION}\"/g" config/config_test.go

# Update version in README files
sed -i "s/\*\*Version:\*\* [0-9.]*/**Version:** ${VERSION}/g" README.md README_FR.md
sed -i "s/version: \"[0-9.]*\"/version: \"${VERSION}\"/g" README.md README_FR.md

# Update changelog headers (only if not already updated)
if ! grep -q "### v${VERSION}" README.md 2>/dev/null; then
    # Add new version section in changelog
    TODAY=$(date +%Y-%m-%d)
    sed -i "s/## Changelog/## Changelog\n\n### v${VERSION} (${TODAY})\n\n- 🚀 New version ${VERSION}/" README.md
    sed -i "s/## Changelog/## Changelog\n\n### v${VERSION} (${TODAY})\n\n- 🚀 Nouvelle version ${VERSION}/" README_FR.md
fi

echo -e "${GREEN}✓ Version updated to ${VERSION}${NC}\n"

echo -e "${YELLOW}Step 2/6: Building project...${NC}"
make build > /tmp/build.log 2>&1
if [ $? -eq 0 ]; then
    echo -e "${GREEN}✓ Build successful${NC}\n"
else
    echo -e "${RED}✗ Build failed${NC}"
    cat /tmp/build.log
    exit 1
fi

echo -e "${YELLOW}Step 3/6: Running tests...${NC}"
go test -race ./... > /tmp/test.log 2>&1
if [ $? -eq 0 ]; then
    echo -e "${GREEN}✓ All tests passed${NC}\n"
else
    echo -e "${RED}✗ Tests failed${NC}"
    cat /tmp/test.log
    exit 1
fi

echo -e "${YELLOW}Step 4/6: Running benchmarks...${NC}"
echo -e "${BLUE}This may take a minute...${NC}"
go test -bench=. -benchmem -benchtime=100ms -count=1 ./... > /tmp/bench.log 2>&1
if [ $? -eq 0 ]; then
    echo -e "${GREEN}✓ Benchmarks completed${NC}\n"
    # Show summary
    echo -e "${BLUE}Benchmark Summary:${NC}"
    grep "Benchmark" /tmp/bench.log | grep -E "(ns/op|µs/op|ms/op)" | head -10
    echo ""
else
    echo -e "${RED}✗ Benchmarks failed${NC}"
    cat /tmp/bench.log
    exit 1
fi

echo -e "${YELLOW}Step 5/6: Verifying binary...${NC}"
VERSION_OUTPUT=$(./bin/mira --version 2>&1)
if echo "$VERSION_OUTPUT" | grep -q "v${VERSION}"; then
    echo -e "${GREEN}✓ Binary reports correct version: ${VERSION_OUTPUT}${NC}\n"
else
    echo -e "${RED}✗ Binary version mismatch: ${VERSION_OUTPUT}${NC}"
    exit 1
fi

echo -e "${YELLOW}Step 6/6: Checking git status...${NC}"
if [ -d ".git" ]; then
    # Show what files will be changed
    echo -e "${BLUE}Modified files:${NC}"
    git diff --name-only 2>/dev/null || echo "(none yet)"
    echo ""
    
    # Count changes
    CHANGED=$(git diff --name-only 2>/dev/null | wc -l)
    if [ "$CHANGED" -gt 0 ]; then
        echo -e "${GREEN}✓ ${CHANGED} files modified${NC}\n"
    fi
fi

echo -e "${BLUE}═══════════════════════════════════════════════════${NC}"
echo -e "${GREEN}  Pre-publish completed successfully!${NC}"
echo -e "${BLUE}═══════════════════════════════════════════════════${NC}\n"

echo -e "Summary:"
echo -e "  - Version: ${GREEN}${VERSION}${NC}"
echo -e "  - Build: ${GREEN}OK${NC}"
echo -e "  - Tests: ${GREEN}PASS${NC}"
echo -e "  - Benchmarks: ${GREEN}OK${NC}"
echo -e "  - Binary: ${GREEN}OK${NC}\n"

echo -e "Next steps:"
echo -e "  1. Review changes: ${YELLOW}git diff${NC}"
echo -e "  2. Stage changes: ${YELLOW}git add -A${NC}"
echo -e "  3. Commit: ${YELLOW}git commit -m \"release: Version ${VERSION}\"${NC}"
echo -e "  4. Tag: ${YELLOW}git tag v${VERSION}${NC}"
echo -e "  5. Push: ${YELLOW}git push origin main --tags${NC}\n"

echo -e "${YELLOW}Ready to publish!${NC}"

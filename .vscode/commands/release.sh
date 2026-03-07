#!/usr/bin/env bash
# =============================================================================
# Release — Tag and push a new version
# =============================================================================
# Sourced by .vscode/commands.sh. Requires FOREMAN_ROOT to be set.

Release.Tag() {
    echo "=== Release New Version ==="
    cd "$FOREMAN_ROOT"

    # Ensure working tree is clean
    if ! git diff --quiet || ! git diff --cached --quiet; then
        echo "Error: working tree has uncommitted changes. Commit or stash first."
        return 1
    fi

    # Get the latest version tag (default v0.0.0 if none)
    local latest
    latest=$(git tag --sort=-v:refname --list 'v*' | head -n1)
    if [ -z "$latest" ]; then
        latest="v0.0.0"
    fi

    # Parse major.minor.patch
    local version="${latest#v}"
    local major minor patch
    IFS='.' read -r major minor patch <<< "$version"
    major="${major:-0}"; minor="${minor:-0}"; patch="${patch:-0}"

    echo "Current version: v${major}.${minor}.${patch}"
    echo ""
    echo "Bump type:"
    echo "  1) patch  (v${major}.${minor}.$((patch+1)))"
    echo "  2) minor  (v${major}.$((minor+1)).0)"
    echo "  3) major  (v$((major+1)).0.0)"
    printf "Select [1/2/3] (default: 1): "
    read -r choice

    case "${choice:-1}" in
        1) patch=$((patch+1)) ;;
        2) minor=$((minor+1)); patch=0 ;;
        3) major=$((major+1)); minor=0; patch=0 ;;
        *) echo "Invalid choice"; return 1 ;;
    esac

    local new_tag="v${major}.${minor}.${patch}"
    echo ""
    echo "New version: $new_tag"
    printf "Proceed? [y/N]: "
    read -r confirm
    if [ "${confirm,,}" != "y" ]; then
        echo "Aborted."
        return 0
    fi

    git tag -a "$new_tag" -m "Release $new_tag" || return 1
    git push origin "$new_tag" || return 1
    echo ""
    echo "✅ Pushed tag $new_tag — GitHub Actions will create the release."
}

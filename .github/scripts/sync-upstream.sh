#!/usr/bin/env bash
#
# Merge upstream into this fork's feature branch, and leave the frontend
# submodule on a commit the fork owns.
#
# Why this exists: upstream is not going to take the multi-region node work, so
# this fork has to track upstream indefinitely. Every one of those merges hits
# the same conflict -- the `frontend` gitlink -- because upstream and the fork
# both advance the same submodule. That conflict has exactly one correct
# resolution (merge the upstream frontend commit into the fork's submodule
# branch), so it is resolved here rather than by hand each time.
#
# Anything else that conflicts is a real decision, so the merge is aborted, the
# working tree is restored, and the caller sees a failure.
#
# The script never pushes: the workflow compiles both sides first and pushes
# only afterwards, so a branch that does not build never reaches the remote.
#
# Runs from the superproject root. Writes `changed` to $GITHUB_OUTPUT when set.

set -euo pipefail

SUBMODULE=frontend
OUT=${GITHUB_OUTPUT:-/dev/null}

log() { printf '== %s ==\n' "$*"; }

abort() {
    printf '::error::%s\n' "$*" >&2
    exit 1
}

changed() { printf 'changed=%s\n' "$1" >> "$OUT"; }

git rev-parse --verify -q upstream/main >/dev/null \
    || abort "upstream/main is not fetched"

log "head $(git rev-parse --short HEAD), upstream/main $(git rev-parse --short upstream/main)"

# Nothing to do: an earlier run already merged this upstream commit, or upstream
# has not moved since the last deployment.
if git merge-base --is-ancestor upstream/main HEAD; then
    log "already contains upstream/main"
    changed false
    exit 0
fi

git merge --no-edit upstream/main || {
    conflicts=$(git diff --name-only --diff-filter=U | tr '\n' ' ')
    log "conflicts: ${conflicts:-<none reported>}"

    # The gitlink is the expected conflict; anything else means upstream edited
    # code this fork also owns, and only a human can decide the right result.
    if [ "$(git diff --name-only --diff-filter=U)" != "$SUBMODULE" ]; then
        git merge --abort
        abort "upstream conflicts with fork changes: $conflicts"
    fi

    log "the only conflict is the $SUBMODULE gitlink; merging its upstream"
    git -C "$SUBMODULE" merge --no-edit upstream/main \
        || {
            git -C "$SUBMODULE" merge --abort || true
            git merge --abort
            abort "$SUBMODULE has conflicts of its own"
        }

    git add "$SUBMODULE"
    git commit --no-edit
}

log "$SUBMODULE is at $(git -C "$SUBMODULE" rev-parse --short HEAD)"

# The superproject must never point at a commit that exists only upstream: this
# branch is pushed to the fork, while .gitmodules names the upstream remote, so
# a fresh clone resolves the gitlink by asking upstream for it. Merging upstream
# into the submodule branch keeps every recorded commit reachable from both.
if ! git -C "$SUBMODULE" merge-base --is-ancestor upstream/main HEAD; then
    log "merging upstream $SUBMODULE into the submodule branch"
    git -C "$SUBMODULE" merge --no-edit upstream/main \
        || abort "$SUBMODULE has conflicts of its own"
fi

git add "$SUBMODULE"
if ! git diff --cached --quiet; then
    git commit -m "chore(sync): point $SUBMODULE at the merged upstream commit"
fi

log "head is now $(git rev-parse --short HEAD) ($(git log --oneline -1 --format=%s))"
changed true

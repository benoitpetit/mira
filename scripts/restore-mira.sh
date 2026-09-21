#!/usr/bin/env bash
set -euo pipefail

usage() {
	printf 'Usage: %s <backup-dir> <mira-storage-dir> [--force]\n' "$0" >&2
}

if [ "$#" -lt 2 ] || [ "$#" -gt 3 ]; then
	usage
	exit 2
fi

backup_dir=$1
target_dir=$2
force=${3:-}

if [ "$force" != "" ] && [ "$force" != "--force" ]; then
	usage
	exit 2
fi
if [ ! -f "$backup_dir/mira.db" ]; then
	printf 'Backup database not found: %s\n' "$backup_dir/mira.db" >&2
	exit 1
fi
if [ -e "$target_dir" ] && [ "$force" != "--force" ]; then
	printf 'Target already exists; pass --force to replace its files: %s\n' "$target_dir" >&2
	exit 1
fi

if [ "$(sqlite3 "$backup_dir/mira.db" 'PRAGMA integrity_check;')" != "ok" ]; then
	printf 'Backup integrity check failed\n' >&2
	exit 1
fi

mkdir -p "$target_dir"
cp -p "$backup_dir/mira.db" "$target_dir/mira.db"
for file in vectors.bin vectors.bin.sha256; do
	if [ -f "$backup_dir/$file" ]; then
		cp -p "$backup_dir/$file" "$target_dir/$file"
	fi
done

if [ -d "$backup_dir/models" ]; then
	rm -rf "$target_dir/models"
	cp -a "$backup_dir/models" "$target_dir/models"
fi

printf 'MIRA backup restored to: %s\n' "$target_dir"

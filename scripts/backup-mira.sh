#!/usr/bin/env bash
set -euo pipefail

usage() {
	printf 'Usage: %s <mira-storage-dir> <backup-dir>\n' "$0" >&2
}

if [ "$#" -ne 2 ]; then
	usage
	exit 2
fi

source_dir=$1
backup_dir=$2
database="$source_dir/mira.db"

if [ ! -f "$database" ]; then
	printf 'MIRA database not found: %s\n' "$database" >&2
	exit 1
fi
if [ -e "$backup_dir" ]; then
	printf 'Backup destination already exists: %s\n' "$backup_dir" >&2
	exit 1
fi

mkdir -p "$backup_dir"

# SQLite's online backup API keeps the copy consistent while MIRA is running.
sqlite3 "$database" ".backup '$backup_dir/mira.db'"

for file in vectors.bin vectors.bin.sha256; do
	if [ -f "$source_dir/$file" ]; then
		cp -p "$source_dir/$file" "$backup_dir/$file"
	fi
done

if [ -d "$source_dir/models" ]; then
	cp -a "$source_dir/models" "$backup_dir/models"
fi

if [ "$(sqlite3 "$backup_dir/mira.db" 'PRAGMA integrity_check;')" != "ok" ]; then
	rm -f "$backup_dir/mira.db"
	rm -f "$backup_dir/vectors.bin" "$backup_dir/vectors.bin.sha256"
	rm -rf "$backup_dir/models"
	rmdir "$backup_dir" 2>/dev/null || true
	printf 'Backup integrity check failed\n' >&2
	exit 1
fi

printf 'MIRA backup created: %s\n' "$backup_dir"

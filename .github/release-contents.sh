#!/bin/sh
# Prints the release archives in the directory $1, each followed by the files it holds, with the version replaced by
# VERSION. ci.yml's release-build job and release.yml's verify job compare it with .github/release-contents.txt.
# Directory entries are left out: goreleaser writes none, and extraction creates the directories all the same.
set -eu
export LC_ALL=C
for archive in "$1"/*.tar.gz "$1"/*.zip; do
	basename "$archive"
	case $archive in
	*.zip) unzip -Z1 "$archive" ;;
	*) tar -tzf "$archive" ;;
	esac | grep -v '/$' | sort | sed 's/^/  /'
done | sed -E 's/_v[0-9][^_/]*_/_VERSION_/g'

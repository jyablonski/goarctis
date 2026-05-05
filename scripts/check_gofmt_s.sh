#!/usr/bin/env bash
set -euo pipefail

unformatted="$(gofmt -l -s .)"
if [ -n "$unformatted" ]; then
  echo "The following files are not formatted with gofmt -s:"
  echo "$unformatted"
  exit 1
fi

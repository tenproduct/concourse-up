#!/bin/bash

cleanup() {
  status=$?
  ./cup --non-interactive destroy $deployment
  exit $status
}

set +u
if [ -z "$SKIP_TEARDOWN" ]; then
  trap cleanup EXIT
else
  trap "echo Skipping teardown" EXIT
fi
set -u

cp "$BINARY_PATH" ./cup
chmod +x ./cup

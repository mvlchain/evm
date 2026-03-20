#!/usr/bin/env sh
set -x

BINARY=/tadad/${BINARY:-tadad}
ID=${ID:-0}
LOG=${LOG:-tadad.log}

if ! [ -f "${BINARY}" ]; then
	echo "The binary $(basename "${BINARY}") cannot be found. Please add the binary to the shared folder. Please use the BINARY environment variable if the name of the binary is not 'tadad'"
	exit 1
fi

export TADADHOME="/data/node${ID}/tadad"

if [ -d "$(dirname "${TADADHOME}"/"${LOG}")" ]; then
  "${BINARY}" --home "${TADADHOME}" "$@" | tee "${TADADHOME}/${LOG}"
else
  "${BINARY}" --home "${TADADHOME}" "$@"
fi

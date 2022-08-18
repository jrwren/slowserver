#!/bin/bash

set -euo pipefail
IFS=$'\n\t'

exec /app -httpPort $NOMAD_PORT_http


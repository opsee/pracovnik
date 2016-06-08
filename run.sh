#!/bin/bash
set -e

APPENV=${APPENV:-pracovnikenv}

/opt/bin/s3kms -r us-west-1 get -b opsee-keys -o dev/$APPENV > /$APPENV

source /$APPENV && \
	/worker

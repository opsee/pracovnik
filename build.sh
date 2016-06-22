#!/bin/bash
set -e

migrate -url $PRACOVNIK_POSTGRES_CONN -path ./migrations up

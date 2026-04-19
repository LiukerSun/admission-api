#!/bin/sh
set -e

echo "Running database migrations..."
./api -migrate up

echo "Starting server..."
exec ./api

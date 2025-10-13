#!/bin/bash
set -e

echo "Updating goarctis..."

# Stop the service
echo "Stopping service..."
systemctl --user stop goarctis.service

# Build the new binary
echo "Building new binary..."
go build -o goarctis cmd/goarctis/main.go

# Install the new binary
echo "Installing binary..."
sudo cp goarctis /usr/local/bin/
sudo chmod +x /usr/local/bin/goarctis

# Start the service
echo "Starting service..."
systemctl --user start goarctis.service

# Check status
echo "Status:"
systemctl --user status goarctis.service --no-pager

echo "Update complete!"

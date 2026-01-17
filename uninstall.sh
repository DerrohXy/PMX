#!/bin/bash

BINARY_NAME="pmx"
INSTALL_PATH="/usr/local/bin/$BINARY_NAME"
WORKING_DIR="/var/lib/pmx"
MONITOR_SERVICE="/etc/systemd/system/pmx-monitor.service"
SERVE_SERVICE="/etc/systemd/system/pmx-serve.service"
CURRENT_USER=$(whoami)
API_PORT="8081"

echo "Uninstalling PMX ..."
echo "Stopping services ..."

sudo systemctl disable pmx-monitor
sudo systemctl stop pmx-monitor

sudo systemctl disable pmx-serve
sudo systemctl stop pmx-serve

echo "Deleting installation files ..."

sudo rm -rf "$INSTALL_PATH"
sudo rm -rf "$WORKING_DIR"
sudo rm -rf "$MONITOR_SERVICE"
sudo rm -rf "$SERVE_SERVICE"

echo "--- Uninstall Complete ---"
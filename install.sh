#!/bin/bash

BINARY_NAME="pmx"
INSTALL_PATH="/usr/local/bin/$BINARY_NAME"
WORKING_DIR="/var/lib/pmx"
MONITOR_SERVICE="/etc/systemd/system/pmx-monitor.service"
SERVE_SERVICE="/etc/systemd/system/pmx-serve.service"
CURRENT_USER=$(whoami)
API_PORT="8081"

echo "Starting PMX Installation..."

if [ ! -f "./$BINARY_NAME" ]; then
    echo "Error: '$BINARY_NAME' binary not found. Please run 'go build' first."
    exit 1
fi

echo "Provisioning storage at $WORKING_DIR..."
sudo mkdir -p "$WORKING_DIR"
sudo chown -R "$CURRENT_USER:$CURRENT_USER" "$WORKING_DIR"
sudo chmod 755 "$WORKING_DIR"

echo "Moving binary to $INSTALL_PATH..."
sudo cp "./$BINARY_NAME" "$INSTALL_PATH"
sudo chmod +x "$INSTALL_PATH"

echo "Creating Monitor service..."
sudo bash -c "cat > $MONITOR_SERVICE" <<EOF
[Unit]
Description=PMX Process Monitor Daemon
After=network.target

[Service]
User=$CURRENT_USER
WorkingDirectory=$WORKING_DIR
ExecStart=$INSTALL_PATH monitor
Restart=always
RestartSec=5
StandardOutput=append:$WORKING_DIR/monitor.log
StandardError=append:$WORKING_DIR/monitor.log

[Install]
WantedBy=multi-user.target
EOF

echo "Creating Serve service on port $API_PORT..."
sudo bash -c "cat > $SERVE_SERVICE" <<EOF
[Unit]
Description=PMX Web API Server
After=network.target

[Service]
User=$CURRENT_USER
WorkingDirectory=$WORKING_DIR
ExecStart=$INSTALL_PATH serve port=$API_PORT
Restart=always
RestartSec=10
StandardOutput=append:$WORKING_DIR/serve.log
StandardError=append:$WORKING_DIR/serve.log

[Install]
WantedBy=multi-user.target
EOF

echo "Reloading systemd and enabling services..."
sudo systemctl daemon-reload

sudo systemctl enable pmx-monitor
sudo systemctl start pmx-monitor

sudo systemctl enable pmx-serve
sudo systemctl start pmx-serve

echo "--- Installation Complete ---"
echo "Binary: $INSTALL_PATH"
echo "API: http://0.0.0.0:$API_PORT/status"
echo "Logs: $WORKING_DIR"
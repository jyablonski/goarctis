# Notes

```sh
mkdir -p ~/.config/systemd/user/
nano ~/.config/systemd/user/goarctis.service

[Unit]
Description=Arctis GameBuds Manager
After=graphical-session.target

[Service]
Type=simple
ExecStart=/usr/local/bin/goarctis
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target

# Reload systemd user daemon
systemctl --user daemon-reload

# Enable to start on boot
systemctl --user enable goarctis.service

# Start it now
systemctl --user start goarctis.service

# Check status
systemctl --user status goarctis.service

sudo tee /etc/udev/rules.d/99-steelseries-gamebuds.rules << 'EOF'
# SteelSeries Arctis GameBuds
KERNEL=="hidraw*", ATTRS{idVendor}=="1038", ATTRS{idProduct}=="230a", MODE="0666", TAG+="uaccess"
EOF

sudo udevadm control --reload-rules
sudo udevadm trigger

make install

# Create user service directory
mkdir -p ~/.config/systemd/user/

# Create service file
cat > ~/.config/systemd/user/goarctis.service << 'EOF'
[Unit]
Description=Arctis GameBuds Manager
After=graphical-session.target

[Service]
Type=simple
ExecStart=/usr/local/bin/goarctis
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
EOF

# Enable and start the service
systemctl --user daemon-reload
systemctl --user enable goarctis.service
systemctl --user start goarctis.service
```

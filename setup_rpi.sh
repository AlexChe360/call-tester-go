#!/bin/bash
set -e
echo "=== RPi setup for call-tester ==="
sudo systemctl stop ModemManager 2>/dev/null || true
sudo systemctl disable ModemManager 2>/dev/null || true
sudo apt update && sudo apt install -y libqmi-utils udhcpc curl iproute2 iputils-ping
groups | grep -q dialout || { sudo usermod -aG dialout $USER; echo "Relogin needed!"; }
echo ""
echo "Modems:"; ls /dev/ttyUSB* 2>/dev/null
echo "QMI:"; ls /dev/cdc-wdm* 2>/dev/null
echo "WWAN:"; ls /sys/class/net/ | grep wwan
echo ""
echo "Edit config.toml, then: ./call-tester check"

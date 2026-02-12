# Install
mkdir -p /mnt/user/appdata/misc
curl -o /mnt/user/appdata/misc/wolvm https://github.com/b1on1cdog/wolvm/releases/download/0.0.1/wolvm
chmod +x /mnt/user/appdata/misc/wolvm

# User Script
#!/bin/bash

VM_NAME="Windows"<br>
VM_MAC="VM_MAC_ADDRESS"<br>
IFACE="br0"<br>

/mnt/user/appdata/misc/wolvm -iface $IFACE -mac $VM_MAC -vm $VM_NAME

# Features
- Starts or resume VM if a WoL packet is detected with matching VM Mac Addr<br>
- Support Webhooks to notify when WoL was received or VM shutsdown<br>
- Supports multiple VMs at same time<br>
- Automatically find the VM info (name, mac addr, interface) if unspecified<br>

# Install
mkdir -p /mnt/user/appdata/misc<br>
curl -o /mnt/user/appdata/misc/wolvm https://github.com/b1on1cdog/wolvm/releases/download/0.0.2/wolvm<br>
chmod +x /mnt/user/appdata/misc/wolvm<br>

# User Script
wolvm is expected to be used with the plugin User Scripts<br/>

[Script for all VMs](files/user_script_all)<br>
[Script without webhooks](files/user_script)<br>
[Script with webhooks](files/user_script_webhooks)<br>

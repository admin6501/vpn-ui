# Distribution and architecture reference

The published release binary is **Linux amd64** and requires a running **systemd** system and root
access. Ubuntu 24.04 and Debian 12/13 are recommended. The table lists upstream target
distributions, not a full test certification of this fork.

| Distribution            | Upstream targets |
| ----------------------- | ---------------- |
| Ubuntu                  | 24.04, 26.04     |
| Debian                  | 12, 13           |
| Fedora                  | 43, 44           |
| AlmaLinux / Rocky Linux | 9, 10            |
| CentOS Stream           | 9, 10            |
| Arch Linux              | Rolling          |

Ubuntu 22.04 is not officially supported upstream. Kernel modules and protocol packages vary between
distributions and kernel flavors; installing the panel is not proof that every VPN backend works on
that host.

The optional apt step in [the offline installer](install-offline.sh) applies to apt-based
distributions. Other distributions use their own package managers. [Installation guide](README.md) ·
[Offline prerequisites](docs/OFFLINE-INSTALL.md) · [VM test harness](test_unit/README.md).

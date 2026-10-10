# VPN-UI — admin6501 版本

[English](README.md) | [فارسی](README_FA.md) | [العربية](README_AR.md) | [Español](README_ES.md) | [Русский](README_RU.md) | [Türkçe](README_TR.md) | [中文](README_ZH.md)

基于 Sir-MmD/vpn-ui 和 3X-UI 的面板。支持迁移 PasarGuard 用户、管理员归属、主机和订阅令牌，管理代理角色，并按唯一用户统计数量。不导入节点。

**Repository:** [admin6501/vpn-ui](https://github.com/admin6501/vpn-ui) · **Release:** [v1.0.0 / latest](https://github.com/admin6501/vpn-ui/releases/latest)

## 在线安装

```bash
curl -fsSL https://raw.githubusercontent.com/admin6501/vpn-ui/main/deploy.sh -o deploy.sh
sudo bash deploy.sh
```

## 离线安装

从发行版下载 vpn-ui-amd64、install-offline.sh 和 SHA256SUMS，并放入服务器的同一目录。需要 Linux amd64、root 和 systemd。部分协议需要预先安装系统软件包和内核模块。

```bash
sudo bash install-offline.sh ./vpn-ui-amd64
```

## 源码与文档

```bash
git clone --recurse-submodules https://github.com/admin6501/vpn-ui.git
cd vpn-ui
./build.sh
```

[English documentation](README.md) · [Offline setup/recovery](docs/OFFLINE-INSTALL.md) · [Migration (فارسی)](migration/INSTALL-MIGRATION.fa.md) · [Architecture](docs/ARCHITECTURE.md) · [Tests](test_unit/README.md)

Ubuntu 24.04 / Debian 12–13 recommended. Full protocol/distribution coverage has not been verified for this fork. Certificate issuance/renewal requires internet. Updates preserve credentials; fresh installs print random credentials. PasarGuard subscription tokens remain unchanged, but a new server domain/IP needs a new base URL.

**License:** [GPL-3.0](LICENSE). Based on [Sir-MmD/vpn-ui](https://github.com/Sir-MmD/vpn-ui) and [3X-UI](https://github.com/MHSanaei/3x-ui); original attribution is preserved.

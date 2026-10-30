# VPN-UI v1.0.0 — 第一个稳定版本

[English](README.md) | [فارسی](README_FA.md) | [العربية](README_AR.md) | [Español](README_ES.md) | [Bahasa Indonesia](README_ID.md) | [日本語](README_JA.md) | [Português (Brasil)](README_PT_BR.md) | [Русский](README_RU.md) | [Türkçe](README_TR.md) | [Українська](README_UK.md) | [Tiếng Việt](README_VI.md) | [简体中文](README_ZH.md) | [繁體中文](README_ZH_TW.md)

VPN-UI 是一个多协议 VPN 管理面板。 **v1.0.0 是该分支的第一个稳定版本。** 源、安装程序和更新来自 [admin6501/vpn-ui](https://github.com/admin6501/vpn-ui)，基于 [Sir-MmD/vpn-ui](https://github.com/Sir-MmD/vpn-ui) 和 [3X-UI](https://github.com/MHSanaei/3x-ui)。

## 十三种语言

面板、订阅浏览器页面、内置 Telegram 机器人和共享 UI 控件支持英语、波斯语、阿拉伯语、西班牙语、印度尼西亚语、日语、巴西葡萄牙语、俄语、土耳其语、乌克兰语、越南语、简体中文和繁体中文。在语言选择器中选择网络语言；在 Telegram 设置中配置机器人语言。该机器人使用配置的语言，而不是每个 Telegram 聊天的单独自动首选项。

Web 请求保留自己的语言，包括并发面板和订阅请求。区域变体解析为受支持的区域设置。对话框、日历和分页使用所选语言。协议标识符、配置键、命令名称和诊断输出保留其原始语法。终端安装程序使用英语；迁移向导提供波斯语和英语。请参阅 [docs/TRANSLATIONS.md](docs/TRANSLATIONS.md)。

## 协议和核心

| 运行时 | 协议和实现 |
| --- | --- |
| 捆绑式 X 射线 | VLESS, VMess, Trojan, Shadowsocks, Hysteria2, AnyTLS, TUIC, NaiveProxy, WireGuard |
| 便携式 VPN 后端 | OpenVPN (TCP/UDP)、OpenConnect (ocserv)、L2TP、PPTP、SSTP 和 IKEv2 |
| 内核隧道 | WireGuard (`wg-c`)、AmneziaWG (`awg`) 和 GRE，在支持的情况下具有可选的 IPsec 或 FOU |
| 内置继电器 | 具有按帐户身份验证的 MTProto 代理和 SSH 代理 |

IPsec 可以在核心安装中选择，也是 L2TP/IKEv2 的共享依赖项。当另一个已安装的核心需要它们时，所需的依赖项将被保留。内核 WireGuard 和本机 Xray WireGuard 是不同的实现。内置SSH功能转发VPN流量；它不提供 root shell。

完整的二进制文件包括便携式后端、修补的 Xray 核心、ACME 客户端和六个地理数据文件：base、伊朗和俄罗斯 geoip/geosite。操作系统包、内​​核模块、内核头和 DKMS 仍然是主机要求。 “核心”页面显示安装进度、缺少的先决条件和实际服务状态。除非启用 IPsec，否则普通 GRE 不会加密； PPTP 是一个遗留协议。请参阅 [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)。

## 用户、组和订阅

在**用户**中管理客户，在**入站**中管理基础架构，在**组**中管理可重复使用的入站分配。一个帐户可以使用多个具有共享配额和订阅的兼容入站和位置。组更改更新分配而不替换帐户凭据。个人会员资格切换和全局帐户切换保持独立。

尽管 API 字段名称为 `totalGB`，但流量限制仍以字节为单位存储。零流量限制的账户，流量不受限制；到期时间为零意味着无限的时间。延迟启动计划从首次使用时开始。概览总计计算的是唯一帐户，而不是每个会员对应一个用户。在不同帐户之间共享订阅 ID 可以一起发布其配置；它不会合并他们的身份或将他们单独的限制转变为一个共享帐户配额。

捆绑核心将新用途归因于其源入站。帐户信息分别显示测量的份额和任何历史未归属的剩余部分。升级保留现有用途；旧的仅帐户记录无法重新创建历史拆分。外部代理端点在帐户/配置导出中一致使用。 OpenVPN 配置文件和 WireGuard 对等文件可以从相应的配置视图下载。

### 更新、重置流量并冻结

**用户 → 客户端的三点菜单 → 续订** 清除其会员当前的上传/下载，从现在开始开始完整的套餐持续时间并启用帐户。支持仅流量、仅时间、组合和完全无限的计划。配额、密码、UUID、订阅 ID、设备密钥、对等地址和入站访问保持不变。未知的遗留持续时间询问一次天数并记住它。

**重置流量**清除当前使用情况，无需重新启动到期或启用手动禁用的帐户。 **冻结** 暂停剩余天数；解冻可恢复剩余部分。续订则使用完整记录的套餐持续时间。保留生命周期记账和累积节点计数器。代表限制和残疾人会员资格仍然适用。请参阅 [docs/CLIENT-RENEWAL.md](docs/CLIENT-RENEWAL.md) 和 [docs/PANEL-GUIDE.md](docs/PANEL-GUIDE.md)。

### 速度和访问限制

下载/上传覆盖接受 KB/s、MB/s 或 Mbps。空继承入站设置；零意味着在该方向上没有限制。存储使用 1024 字节 KB/s，而 Mbps 表示十进制兆位。 UI 的 MB/s 使用基于 1024 的字节：**3 MB/s = 24 Mib/s ≈ 25.17 Mbps**。当速度值为正数时，速度值至少四舍五入为 1 KB/s。

源 IP 限制计数可见地址，而不是一个 NAT 后面的物理设备。会话和对等限制取决于协议。 WireGuard/AmneziaWG 发布单独的设备配置。支持的限制显示在每种表格中；节点会话/IP 限制是每个节点的本地限制。

## 管理员和代表

初始管理员是受保护面板的所有者。管理员角色和自定义授权决定操作；小组/入站分配决定了其范围。所有者敏感的数据库、身份验证和可执行操作保留单独的保护。明确的面板设置授权会打开其完整的允许设置页面，包括 Telegram、LDAP、证书和服务设置。

新的代表账户使用基于消费的会计和范围角色。现有的预付费帐户保留自己的计费规则。更新、删除和使用重置授权是分开的。手动续订需要更新和重置权限，范围较窄。所有权转让保留了客户凭证，并且仅限于符合资格的所有者。配额用完的代表仍受配置的断开连接政策的约束。

每个登录帐户都可以在**帐户安全**中管理自己的身份验证器。注册需要当前密码和有效的一次性代码。经理可以要求代表进行两步注册；待注册仅允许帐户安全和注销。机密和订阅链接必须被视为凭证。

## 多位置节点

一个中央面板通过出站验证的 HTTPS 管理专用位置代理。一个节点与一个面板配对。在单独的位置服务器上安装代理，然后在节点的“核心”对话框中选择所需的核心；初始代理安装不会激活任何 VPN 核心。使用 **管理节点和位置** 委派节点管理。代表无法获得此基础设施拨款。

代理轮询和可见节点页面每秒刷新一次。主机诊断单独刷新，通常每五秒刷新一次。不变的快照避免重写客户端表；更改的配置会协调受影响的核心。核心安装/删除、运行时控制和配置编辑显示持续的进度和确认。

帐户保留一个集中配额并跨地点订阅。接受的累积报告不会重复计费。更改异步传播；飞行中的流量和同步位置可能会短暂超出配额。在线节点数据在 35 秒后就会失效。失去管理租约 90 秒会停止 VPN 数据平面，同时保留计数器；一个新的有效快照恢复服务。请参阅 [docs/NODES.md](docs/NODES.md) 和 [docs/NODES.fa.md](docs/NODES.fa.md)。

## 系统要求

需要 Linux amd64 (x86-64) 或 arm64 (aarch64)、root 访问权限、Bash 和正在运行的 systemd 系统。 Ubuntu 24.04 和 Debian 12/13 是推荐的部署目标。主机网络权限和特定于协议的模块仍然是必要的。成功的面板安装并不能证明每个发行版、内核或 VPS 上的每个协议。请参阅[表.md](table.md)。

标准安装目录为`/opt/vpn-ui`。安装的可执行文件在两种体系结构上均保留兼容性名称 `/opt/vpn-ui/vpn-ui-amd64`。捆绑的核心文件名遵循其实际架构。该节点使用 `/opt/vpn-ui-node` 及其自己的 systemd 服务。

## 在线安装或更新

```bash
curl -fsSL https://raw.githubusercontent.com/admin6501/vpn-ui/main/deploy.sh -o deploy.sh
sudo bash deploy.sh
```

同一命令更新现有标准安装，同时保留其数据库和登录设置。先备份一下。全新安装会打印随机凭据、端口和 Web 路径。使用打印的 HTTP/HTTPS 方案打开确切的 URL，并将侦听地址（例如 `0.0.0.0`）替换为服务器的可访问地址。全新安装使用 HTTP，直到配置 TLS。

对于现有的配对节点，下载并运行其安装程序；从菜单中选择安装/更新。现有的配对和计数器将被保留：

```bash
curl -fsSL https://raw.githubusercontent.com/admin6501/vpn-ui/main/install-node.sh -o install-node.sh
sudo bash install-node.sh
```

新节点必须使用中央面板中生成的配对命令。节点菜单还提供状态、启动、停止、重新启动、最近日志和确认删除。 `--install` 保留直接非菜单更新路径。请参阅[management-cli-plan.md](management-cli-plan.md)。

## 离线安装和更新

从[稳定版本](https://github.com/admin6501/vpn-ui/releases/tag/v1.0.0)下载架构匹配的二进制文件`install-offline.sh`和`SHA256SUMS`。将它们一起传输并在安装前验证可用文件：

```bash
sha256sum --ignore-missing -c SHA256SUMS
sudo bash install-offline.sh ./vpn-ui-amd64
# ARM64: sudo bash install-offline.sh ./vpn-ui-arm64
```

可选的 apt 先决条件问题默认为 **否**，包括 EOF。否将面板安装保持在本地；是的，需要可访问的 apt 存储库。报告 apt 失败并继续本地安装。分别预安装或传输所需的操作系统包和内核模块。更新会保留数据库、凭据和 TLS 设置并保留私有回滚备份。

安装程序会记录 `VPNUI_OFFLINE=1`，禁用自动包下载、公共 IP 查找和地理数据更新。 VPN 流量仍然有效；手动更新、WARP 和 ACME 发行/更新仍然需要互联网。对于离线节点安装，请使用匹配的节点二进制文件和 `install-node.sh` 及其配对选项。请参阅[docs/OFFLINE-INSTALL.md](docs/OFFLINE-INSTALL.md)。

## PasarGuard 备份迁移

首先安装兼容的目标面板。其 VPN 帐户和入站必须为空；现有的面板 Web/TLS 设置将被保留。运行：

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/admin6501/vpn-ui/main/migration/install.sh)
```

选择波斯语或英语并提供绝对备份 ZIP 路径。迁移包包含脚本和配置，而不是另一个面板二进制文件。需要 Python 3 并解压。对于离线转换，传输并提取`vpn-ui-auto-migration.zip`，然后直接使用备份路径运行其`migrate.sh`。

导入支持的帐户身份、管理员所有权、组/分配、主机、密码哈希和订阅兼容性元数据。使用之前的管理员密码；哈希值无法泄露明文密码。 PasarGuard 节点未导入。更改的域/IP 需要更新订阅基地址并检查导出的端点。查看转换后的配置并测试您实际使用的协议。请参阅[迁移/INSTALL-MIGRATION.fa.md](migration/INSTALL-MIGRATION.fa.md)。

## 操作、备份和恢复

```bash
sudo vpn-ui
systemctl status vpn-ui --no-pager
journalctl -u vpn-ui -n 100 --no-pager
```

使用打印的私人登录详细信息或管理菜单来恢复访问权限。在复制 SQLite 文件之前停止服务并保留 WAL/SHM sidecar，或进行一致的 SQLite 备份。安全地保存 TLS 材料、服务覆盖和节点数据库。流量重置或更新不是数据库恢复。卸载会删除数据；在继续之前检查其确认。请参阅 [accounts-upgrade-guide.md](accounts-upgrade-guide.md) 和 [SECURITY.md](SECURITY.md)。

## 释放资产

| 文件 | 目的 |
| --- | --- |
| `vpn-ui-amd64` | 适用于 Linux x86-64 的完整中央面板 |
| `vpn-ui-arm64` | Linux aarch64 的完整中央面板 |
| `vpn-ui-node-amd64` | 适用于 Linux x86-64 的专用位置代理 |
| `vpn-ui-node-arm64` | Linux aarch64 的专用位置代理 |
| `install-offline.sh` | 具有回滚支持的本地面板安装程序/更新 |
| `install-node.sh` | 节点安装、更新和管理菜单 |
| `vpn-ui-auto-migration.zip` | PasarGuard 转换脚本和配置 |
| `SHA256SUMS` | 上述七个可下载工件的 SHA-256 校验和 |

## 构建、测试和文档

```bash
git clone --recurse-submodules https://github.com/admin6501/vpn-ui.git
cd vpn-ui
./build.sh --help
./build.sh
```

使用 `go.mod` 中的 Go 工具链和固定的子模块/安全补丁。完整版本必须构建其核心、后端和所有地理数据；跳过的捆绑包会生成开发二进制文件。构建需要在线依赖项、C 工具链和后端构建工具，包括需要时的 Docker。即使客户端捆绑了，ACME 也需要互联网。

发布工作流程以受支持的语言验证本机架构、重点记账和权限行为、真实 Xray/节点数据路径、离线安装/更新以及浏览器行为。单独的Incus协议/分发矩阵需要准备好的特权VM主机和互联网；它的存在并不能证明每个矩阵条目都已重新运行。请参阅 [test_unit/README.md](test_unit/README.md)、[docs/INDEX.md](docs/INDEX.md)、[api-reference.md](api-reference.md) 和 [.github/workflows/release.yml](.github/workflows/release.yml)。

## 许可证

[GPL-3.0](LICENSE)。项目和第三方的原始署名及许可声明保持不变。子模块和第三方组件的 README 描述各自组件；安装和运行本面板请使用本分支的指南。


## Telegram 销售机器人

可在 Ubuntu 或 Debian 上安装可选的 VPN-UI Telegram 销售和服务管理机器人，支持钱包。全新安装会创建本地数据库，不会导入 Cloudflare 数据。[机器人指南](telegram-bot/README-SALES-BOT.md)。

独立销售机器人使用波斯语，支持多个面板、分类、累加续费和可选迁移。[API](docs/REPRESENTATIVE-BOT-API.fa.md) · [迁移指南](docs/BOT-LOCATION-MIGRATION.fa.md)。

```bash
curl -fsSL https://raw.githubusercontent.com/admin6501/vpn-ui/main/scripts/install-telegram-bot.sh -o install-telegram-bot.sh && sudo bash install-telegram-bot.sh
```

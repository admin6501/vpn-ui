# VPN-UI v1.0.0 — 第一款穩定版本

[English](README.md) | [فارسی](README_FA.md) | [العربية](README_AR.md) | [Español](README_ES.md) | [Bahasa Indonesia](README_ID.md) | [日本語](README_JA.md) | [Português (Brasil)](README_PT_BR.md) | [Русский](README_RU.md) | [Türkçe](README_TR.md) | [Українська](README_UK.md) | [Tiếng Việt](README_VI.md) | [简体中文](README_ZH.md) | [繁體中文](README_ZH_TW.md)

VPN-UI 是一個多協定 VPN 管理面板。 **v1.0.0 是該分支的第一個穩定版本。 ** 來源、安裝程式和更新來自 [admin6501/vpn-ui](https://github.com/admin6501/vpn-ui)，基於 [Sir-MmD/vpn-ui](https://github.com/Sir-MmD/vpn-ui) 和 [3X-UI](https://github.com/MHSanaei/3x-ui)。

## 十三種語言

面板、訂閱瀏覽器頁面、內建 Telegram 機器人和共享 UI 控制支援英語、波斯語、阿拉伯語、西班牙語、印尼語、日語、巴西葡萄牙語、俄語、土耳其語、烏克蘭語、越南語、簡體中文和繁體中文。在語言選擇器中選擇網路語言；在 Telegram 設定中配置機器人語言。該機器人使用配置的語言，而不是每個 Telegram 聊天的單獨自動首選項。

Web 要求保留自己的語言，包括並發面板和訂閱請求。區域變體解析為受支援的區域設定。對話方塊、日曆和分頁使用所選語言。協定標識符、配置鍵、命令名稱和診斷輸出保留其原始語法。終端安裝程式使用英語；遷移精靈提供波斯語和英語。請參閱 [docs/TRANSLATIONS.md](docs/TRANSLATIONS.md)。

## 協議和核心

| 運行時 | 協議和實現 |
| --- | --- |
| 捆綁式 X 射線 | VLESS, VMess, Trojan, Shadowsocks, Hysteria2, AnyTLS, TUIC, NaiveProxy, WireGuard |
| 便攜式 VPN 後端 | OpenVPN (TCP/UDP)、OpenConnect (ocserv)、L2TP、PPTP、SSTP 與 IKEv2 |
| 內核隧道 | WireGuard (`wg-c`)、AmneziaWG (`awg`) 和 GRE，在支援的情況下具有可選的 IPsec 或 FOU |
| 內建繼電器 | 具有按帳戶驗證的 MTProto 代理程式和 SSH 代理 |

IPsec 可以在核心安裝中選擇，也是 L2TP/IKEv2 的共用依賴項。當另一個已安裝的核心需要它們時，所需的依賴項將被保留。核心 WireGuard 和本機 Xray WireGuard 是不同的實作。內建SSH功能轉送VPN流量；它不提供 root shell。

完整的二進位檔案包括便攜式後端、修補的 Xray 核心、ACME 用戶端和六個地理資料檔案：base、伊朗和俄羅斯 geoip/geosite。作業系統包、核心模組、核心頭和 DKMS 仍然是主機需求。 「核心」頁面顯示安裝進度、缺少的先決條件和實際服務狀態。除非啟用 IPsec，否則普通 GRE 不會加密； PPTP 是遺留協定。請參閱 [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)。

## 使用者、群組和訂閱

在**使用者**中管理客戶，在**入站**中管理基礎架構，在**群組**中管理可重複使用的入站分配。一個帳戶可以使用多個具有共用配額和訂閱的相容入站和位置。群組更改更新分配而不替換帳戶憑證。個人會員資格切換和全域帳戶切換保持獨立。

儘管 API 欄位名稱為 `totalGB`，但流量限制仍以位元組為單位儲存。零流量限制的帳戶，流量不受限制；到期時間為零意味著無限的時間。延遲啟動計劃從首次使用時開始。概覽總計計算的是唯一帳戶，而不是每個會員對應一個使用者。在不同帳戶之間共用訂閱 ID 可以一起發布其配置；它不會合併他們的身份或將他們單獨的限制轉變為一個共享帳戶配額。

捆綁核心將新用途歸因於其來源入站。帳戶資訊分別顯示測量的份額和任何歷史未歸屬的剩餘部分。升級保留現有用途；舊的僅帳戶記錄無法重新建立歷史分割。外部代理端點在帳戶/設定匯出中一致使用。 OpenVPN 設定檔和 WireGuard 對等檔案可以從對應的設定視圖下載。

### 更新、重置流量並凍結

**使用者 → 用戶端的三點選單 → 續約** 清除其會員目前的上傳/下載，從現在開始開始完整的套餐持續時間並啟用帳戶。支援僅流量、僅時間、組合和完全無限的計劃。配額、密碼、UUID、訂閱 ID、裝置金鑰、對等位址和入站存取保持不變。未知的遺留持續時間詢問一次天數並記住它。

**重置流量**清除目前使用情況，無需重新啟動到期或啟用手動停用的帳戶。 **凍結** 暫停剩餘天數；解凍可恢復剩餘部分。續訂則使用完整記錄的套餐持續時間。保留生命週期記帳和累積節點計數器。代表限制和殘疾人會員資格仍然適用。請參閱 [docs/CLIENT-RENEWAL.md](docs/CLIENT-RENEWAL.md) 和 [docs/PANEL-GUIDE.md](docs/PANEL-GUIDE.md)。

### 速度和存取限制

下載/上傳覆蓋接受 KB/s、MB/s 或 Mbps。空繼承入站設定；零意味著在該方向上沒有限制。儲存使用 1024 位元組 KB/s，而 Mbps 表示十進位兆位元。 UI 的 MB/s 使用基於 1024 的位元組：**3 MB/s = 24 Mib/s ≈ 25.17 Mbps**。當速度值為正數時，速度值至少會四捨五入為 1 KB/s。

來源 IP 限制計數可見位址，而不是一個 NAT 後面的實體設備。會話和對等限制取決於協定。 WireGuard/AmneziaWG 發佈單獨的裝置配置。支援的限制顯示在每種表格中；節點會話/IP 限制是每個節點的本地限制。

## 管理員和代表

初始管理員是受保護面板的擁有者。管理員角色和自訂授權決定操作；小組/入站分配決定了其範圍。擁有者敏感的資料庫、身份驗證和可執行操作保留單獨的保護。明確的面板設定授權會開啟其完整的允許設定頁面，包括 Telegram、LDAP、憑證和服務設定。

新的代表帳戶使用基於消費的會計和範圍角色。現有的預付費帳戶保留自己的計費規則。更新、刪除和使用重置授權是分開的。手動續訂需要更新重置權限，範圍較窄。所有權轉讓保留了客戶憑證，並且僅限於符合資格的所有者。配額用完的代表仍受配置的斷線政策的約束。

每個登入帳戶都可以在**帳戶安全性**中管理自己的身份驗證器。註冊需要當前密碼和有效的一次性代碼。經理可以要求代表進行兩步驟註冊；待註冊僅允許帳戶安全和註銷。機密和訂閱連結必須被視為憑證。

## 多位置節點

一個中央面板透過出站驗證的 HTTPS 管理專用位置代理。一個節點與一個面板配對。在單獨的位置伺服器上安裝代理，然後在節點的「核心」對話方塊中選擇所需的核心；初始代理安裝不會啟動任何 VPN 核心。使用 **管理節點和位置** 委派節點管理。代表無法獲得此基礎設施撥款。

代理程式輪詢和可見節點頁面每秒刷新一次。主機診斷單獨刷新，通常每五秒刷新一次。不變的快照避免重寫客戶端表；更改的配置會協調受影響的核心。核心安裝/刪除、運行時控制和配置編輯顯示持續的進度和確認。

帳戶保留一個集中配額並跨地點訂閱。接受的累積報告不會重複計費。更改非同步傳播；飛行中的流量和同步位置可能會短暫超出配額。線上節點資料在 35 秒後就會失效。失去管理租約 90 秒會停止 VPN 資料平面，同時保留計數器；一個新的有效快照復原服務。請參閱 [docs/NODES.md](docs/NODES.md) 和 [docs/NODES.fa.md](docs/NODES.fa.md)。

## 系統需求

需要 Linux amd64 (x86-64) 或 arm64 (aarch64)、root 存取權、Bash 和正在執行的 systemd 系統。 Ubuntu 24.04 和 Debian 12/13 是建議的部署目標。主機網路權限和特定於協定的模組仍然是必要的。成功的面板安裝並不能證明每個發行版、核心或 VPS 上的每個協定。請參閱[表.md](table.md)。

標準安裝目錄為`/opt/vpn-ui`。安裝的可執行檔在兩種體系結構上保留相容性名稱 `/opt/vpn-ui/vpn-ui-amd64`。捆綁的核心檔案名稱遵循其實際架構。該節點使用 `/opt/vpn-ui-node` 及其自己的 systemd 服務。

## 線上安裝或更新

```bash
curl -fsSL https://raw.githubusercontent.com/admin6501/vpn-ui/main/deploy.sh -o deploy.sh
sudo bash deploy.sh
```

同一指令更新現有標準安裝，同時保留其資料庫和登入設定。先備份一下。全新安裝會列印隨機憑證、連接埠和 Web 路徑。使用列印的 HTTP/HTTPS 方案開啟確切的 URL，並將偵聽位址（例如 `0.0.0.0`）替換為伺服器的可存取位址。全新安裝使用 HTTP，直到設定 TLS。

對於現有的配對節點，下載並執行其安裝程式；從選單中選擇安裝/更新。現有的配對和計數器將被保留：

```bash
curl -fsSL https://raw.githubusercontent.com/admin6501/vpn-ui/main/install-node.sh -o install-node.sh
sudo bash install-node.sh
```

新節點必須使用中央面板中產生的配對命令。節點選單還提供狀態、啟動、停止、重新啟動、最近日誌和確認刪除。 `--install` 保留直接非選單更新路徑。請參閱[management-cli-plan.md](management-cli-plan.md)。

## 離線安裝和更新

從[穩定版本](https://github.com/admin6501/vpn-ui/releases/tag/v1.0.0)下載架構匹配的二進位檔案`install-offline.sh`和`SHA256SUMS`。將它們一起傳輸並在安裝前驗證可用檔案：

```bash
sha256sum --ignore-missing -c SHA256SUMS
sudo bash install-offline.sh ./vpn-ui-amd64
# ARM64: sudo bash install-offline.sh ./vpn-ui-arm64
```

可選的 apt 先決條件問題預設為 **否**，包括 EOF。否將面板安裝保持在本地；是的，需要可存取的 apt 儲存庫。報告 apt 失敗並繼續本地安裝。分別預先安裝或傳輸所需的作業系統套件和核心模組。更新會保留資料庫、憑證和 TLS 設定並保留私有回滾備份。

安裝程式會記錄 `VPNUI_OFFLINE=1`，停用自動包下載、公共 IP 查找和地理資料更新。 VPN 流量仍然有效；手動更新、WARP 和 ACME 發行/更新仍然需要網路。對於離線節點安裝，請使用相符的節點二進位和 `install-node.sh` 及其配對選項。請參閱[docs/OFFLINE-INSTALL.md](docs/OFFLINE-INSTALL.md)。

## PasarGuard 備份遷移

首先安裝相容的目標面板。其 VPN 帳戶和入站必須為空；現有的面板 Web/TLS 設定將保留。運行：

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/admin6501/vpn-ui/main/migration/install.sh)
```

選擇波斯語或英語並提供絕對備份 ZIP 路徑。遷移包包含腳本和配置，而不是另一個面板二進位。需要 Python 3 並解壓縮。對於離線轉換，傳輸並提取`vpn-ui-auto-migration.zip`，然後直接使用備份路徑運行其`migrate.sh`。

匯入支援的帳戶身分、管理員所有權、群組/分配、主機、密碼雜湊和訂閱相容性元資料。使用之前的管理員密碼；雜湊值無法洩漏明文密碼。 PasarGuard 節點未導入。變更的網域/IP 需要更新訂閱基底位址並檢查匯出的端點。查看轉換後的配置並測試您實際使用的協定。請參閱[遷移/INSTALL-MIGRATION.fa.md](migration/INSTALL-MIGRATION.fa.md)。

## 操作、備份和恢復

```bash
sudo vpn-ui
systemctl status vpn-ui --no-pager
journalctl -u vpn-ui -n 100 --no-pager
```

使用列印的私人登入詳細資訊或管理選單來恢復存取權限。在複製 SQLite 檔案之前停止服務並保留 WAL/SHM sidecar，或進行一致的 SQLite 備份。安全地保存 TLS 材料、服務覆蓋和節點資料庫。流量重置或更新不是資料庫恢復。卸載會刪除資料；在繼續之前檢查其確認。請參閱 [accounts-upgrade-guide.md](accounts-upgrade-guide.md) 和 [SECURITY.md](SECURITY.md)。

## 釋放資產

| 文件 | 目的 |
| --- | --- |
| `vpn-ui-amd64` | 適用於 Linux x86-64 的完整中央面板 |
| `vpn-ui-arm64` | Linux aarch64 的完整中央面板 |
| `vpn-ui-node-amd64` | 適用於 Linux x86-64 的專用位置代理 |
| `vpn-ui-node-arm64` | Linux aarch64 的專用位置代理 |
| `install-offline.sh` | 具有回滾支援的本機面板安裝程式/更新 |
| `install-node.sh` | 節點安裝、更新與管理選單 |
| `vpn-ui-auto-migration.zip` | PasarGuard 轉換腳本和配置 |
| `SHA256SUMS` | 上述七個可下載工件的 SHA-256 校驗和 |

## 建置、測試和文檔

```bash
git clone --recurse-submodules https://github.com/admin6501/vpn-ui.git
cd vpn-ui
./build.sh --help
./build.sh
```

使用 `go.mod` 中的 Go 工具鏈和固定的子模組/安全性修補程式。完整版本必須建立其核心、後端和所有地理資料；跳過的捆綁包會產生開發二進位檔案。建置需要線上相依性、C 工具鍊和後端建置工具，包括需要時的 Docker。即使客戶端捆綁了，ACME 也需要網路。

發布工作流程會在 amd64 與 arm64 原生環境檢查記帳、權限、Xray/節點資料傳輸、離線安裝與更新，以及各語言的瀏覽器操作。獨立的 Incus 協定與發行版測試矩陣需要準備好的特權 VM 主機和網路；不能把測試工具的存在當成所有項目已重新測試。請參閱 [測試指南](test_unit/README.md)、[文件索引](docs/INDEX.md)、[API](api-reference.md) 及 [發布工作流程](.github/workflows/release.yml)。

## 許可證

[GPL-3.0](LICENSE)。保留原專案及第三方的署名和授權聲明。子模組與第三方元件的 README 描述各自元件；安裝及操作本面板請使用此分支的指南。


## Telegram 銷售機器人

可在 Ubuntu 或 Debian 上安裝選用的 VPN-UI Telegram 銷售與服務管理機器人，支援錢包。全新安裝會建立本機資料庫，不會匯入 Cloudflare 資料。[機器人指南](telegram-bot/README-SALES-BOT.md)。

獨立銷售機器人使用波斯語，支援多個面板、分類、累加續費及選用遷移。[API](docs/REPRESENTATIVE-BOT-API.fa.md) · [遷移指南](docs/BOT-LOCATION-MIGRATION.fa.md)。

```bash
curl -fsSL https://raw.githubusercontent.com/admin6501/vpn-ui/main/scripts/install-telegram-bot.sh -o install-telegram-bot.sh && sudo bash install-telegram-bot.sh
```

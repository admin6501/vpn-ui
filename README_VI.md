# VPN-UI v1.0.0 — Bản phát hành ổn định đầu tiên

[English](README.md) | [فارسی](README_FA.md) | [العربية](README_AR.md) | [Español](README_ES.md) | [Bahasa Indonesia](README_ID.md) | [日本語](README_JA.md) | [Português (Brasil)](README_PT_BR.md) | [Русский](README_RU.md) | [Türkçe](README_TR.md) | [Українська](README_UK.md) | [Tiếng Việt](README_VI.md) | [简体中文](README_ZH.md) | [繁體中文](README_ZH_TW.md)

VPN-UI là bảng quản lý VPN đa giao thức. **v1.0.0 là bản phát hành ổn định đầu tiên của nhánh phân nhánh này.** Nguồn, trình cài đặt và bản cập nhật đến từ [admin6501/vpn-ui](https://github.com/admin6501/vpn-ui), dựa trên [Sir-MmD/vpn-ui](https://github.com/Sir-MmD/vpn-ui) và [3X-UI](https://github.com/MHSanaei/3x-ui).

## Mười ba ngôn ngữ

Bảng điều khiển, trang trình duyệt đăng ký, bot Telegram và các điều khiển giao diện người dùng dùng chung hỗ trợ tiếng Anh, tiếng Ba Tư, tiếng Ả Rập, tiếng Tây Ban Nha, tiếng Indonesia, tiếng Nhật, tiếng Bồ Đào Nha Brazil, tiếng Nga, tiếng Thổ Nhĩ Kỳ, tiếng Ukraina, tiếng Việt, tiếng Trung giản thể và tiếng Trung phồn thể. Chọn ngôn ngữ web trong bộ chọn ngôn ngữ; định cấu hình ngôn ngữ bot trong cài đặt Telegram. Bot sử dụng ngôn ngữ được định cấu hình, không phải tùy chọn tự động riêng biệt cho mỗi cuộc trò chuyện Telegram.

Các yêu cầu web giữ ngôn ngữ riêng của chúng, bao gồm các yêu cầu đăng ký và bảng điều khiển đồng thời. Các biến thể khu vực sẽ phân giải thành các ngôn ngữ được hỗ trợ. Hộp thoại, lịch và phân trang sử dụng ngôn ngữ đã chọn. Mã định danh giao thức, khóa cấu hình, tên lệnh và đầu ra chẩn đoán vẫn giữ nguyên cú pháp ban đầu. Trình cài đặt thiết bị đầu cuối sử dụng tiếng Anh; trình hướng dẫn di chuyển cung cấp tiếng Ba Tư và tiếng Anh. Xem [docs/TRANSLATIONS.md](docs/TRANSLATIONS.md).

## Giao thức và lõi

| Thời gian chạy | Giao thức và triển khai |
| --- | --- |
| X-quang kèm theo | VLESS, VMess, Trojan, Shadowsocks, Hysteria2, AnyTLS, TUIC, NaiveProxy, WireGuard |
| Phụ trợ VPN di động | OpenVPN (TCP/UDP), OpenConnect (ocserv), L2TP, PPTP, SSTP và IKEv2 |
| Đường hầm hạt nhân | WireGuard (`wg-c`), AmneziaWG (`awg`) và GRE, với IPsec hoặc FOU tùy chọn nếu được hỗ trợ |
| Rơle tích hợp | Proxy MTProto và proxy SSH với xác thực trên mỗi tài khoản |

IPsec có thể được lựa chọn trong cài đặt lõi và cũng là phần phụ thuộc chung của L2TP/IKEv2. Các phần phụ thuộc cần thiết sẽ được giữ lại trong khi lõi đã cài đặt khác cần chúng. Kernel WireGuard và Xray WireGuard gốc là những cách triển khai khác nhau. Tính năng SSH tích hợp chuyển tiếp lưu lượng VPN; nó không cung cấp shell gốc.

Các tệp nhị phân đầy đủ bao gồm các chương trình phụ trợ di động, lõi Xray được vá, máy khách ACME và sáu tệp dữ liệu địa lý: base, geoip/geosite của Iran và Nga. Các gói hệ điều hành, mô-đun hạt nhân, tiêu đề hạt nhân và DKMS vẫn là yêu cầu của máy chủ. Trang Lõi hiển thị tiến trình cài đặt, các điều kiện tiên quyết còn thiếu và trạng thái dịch vụ thực tế. Plain GRE không được mã hóa trừ khi IPsec được bật; PPTP là một giao thức kế thừa. Xem [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## Người dùng, nhóm và đăng ký

Quản lý khách hàng trong **Người dùng**, cơ sở hạ tầng trong **Inbounds** và các nhiệm vụ gửi đến có thể sử dụng lại trong **Nhóm**. Một tài khoản có thể sử dụng một số điểm đến và vị trí tương thích với hạn ngạch và đăng ký chung. Nhóm thay đổi cập nhật bài tập mà không thay thế thông tin xác thực tài khoản. Chuyển đổi thành viên cá nhân và chuyển đổi tài khoản toàn cầu vẫn tách biệt.

Giới hạn lưu lượng truy cập được lưu trữ theo byte mặc dù tên trường API `totalGB`. Tài khoản có giới hạn lưu lượng bằng 0 sẽ không giới hạn lưu lượng truy cập; hết hạn bằng 0 nghĩa là không giới hạn thời gian. Kế hoạch trì hoãn bắt đầu bắt đầu từ lần sử dụng đầu tiên. Tổng số tổng quan tính các tài khoản duy nhất, không phải một người dùng cho mỗi thành viên. Chia sẻ ID đăng ký giữa các tài khoản khác nhau sẽ xuất bản cấu hình của chúng cùng nhau; nó không hợp nhất danh tính của họ hoặc biến các giới hạn riêng biệt của họ thành một hạn ngạch tài khoản chung.

Lõi đi kèm phân bổ việc sử dụng mới cho nguồn gửi đến của nó. Thông tin tài khoản hiển thị riêng phần chia sẻ được đo lường và mọi phần còn lại chưa được phân bổ trong lịch sử. Nâng cấp duy trì việc sử dụng hiện tại; bản ghi chỉ dành cho tài khoản cũ không thể tạo lại sự phân chia lịch sử. Điểm cuối Proxy bên ngoài được sử dụng nhất quán trong quá trình xuất tài khoản/cấu hình. Có thể tải xuống cấu hình OpenVPN và tệp ngang hàng WireGuard từ chế độ xem cấu hình tương ứng của chúng.

### Gia hạn, thiết lập lại lưu lượng và đóng băng

**Người dùng → menu ba chấm của khách hàng → Gia hạn** xóa nội dung tải lên/tải xuống hiện tại trên các tư cách thành viên, bắt đầu thời hạn gói đầy đủ kể từ bây giờ và kích hoạt tài khoản. Các gói chỉ lưu lượng truy cập, chỉ thời gian, kết hợp và hoàn toàn không giới hạn được hỗ trợ. Hạn ngạch, mật khẩu, UUID, ID đăng ký, khóa thiết bị, địa chỉ ngang hàng và quyền truy cập gửi đến không thay đổi. Khoảng thời gian kế thừa không xác định yêu cầu số ngày một lần và ghi nhớ nó.

**Đặt lại lưu lượng truy cập** xóa mức sử dụng hiện tại mà không cần khởi động lại khi hết hạn hoặc kích hoạt tài khoản bị vô hiệu hóa theo cách thủ công. **Đóng băng** tạm dừng những ngày còn lại; rã đông sẽ khôi phục phần còn lại. Thay vào đó, việc gia hạn sử dụng toàn bộ thời lượng gói đã ghi. Bộ đếm nút tích lũy và tính toán trọn đời được bảo toàn. Giới hạn đại diện và tư cách thành viên khuyết tật vẫn được áp dụng. Xem [docs/CLIENT-RENEWAL.md](docs/CLIENT-RENEWAL.md) và [docs/PANEL-GUIDE.md](docs/PANEL-GUIDE.md).

### Giới hạn tốc độ và quyền truy cập

Ghi đè tải xuống/tải lên chấp nhận KB/s, MB/s hoặc Mbps. Trống kế thừa cài đặt gửi đến; số không có nghĩa là không giới hạn theo hướng đó. Bộ nhớ sử dụng 1024 byte KB/s, trong khi Mbps có nghĩa là megabit thập phân. MB/s của giao diện người dùng sử dụng byte dựa trên 1024: **3 MB/s = 24 Mib/s ≈ 25.17 Mbps**. Giá trị tốc độ được làm tròn thành ít nhất 1 KB/s khi dương.

Giới hạn nguồn-IP tính các địa chỉ hiển thị, không tính các thiết bị vật lý đằng sau một NAT. Giới hạn phiên và ngang hàng phụ thuộc vào giao thức. WireGuard/AmneziaWG đưa ra các cấu hình thiết bị riêng biệt. Các giới hạn được hỗ trợ được thể hiện dưới từng hình thức; giới hạn phiên/IP của nút là cục bộ đối với mỗi nút.

## Quản trị viên và đại diện

Quản trị viên ban đầu là chủ sở hữu bảng điều khiển được bảo vệ. Vai trò quản trị viên và cấp phép tùy chỉnh xác định hoạt động; nhiệm vụ nhóm/inbound xác định phạm vi của họ. Cơ sở dữ liệu nhạy cảm với chủ sở hữu, hoạt động xác thực và thực thi được bảo vệ riêng biệt. Quyền cài đặt bảng điều khiển rõ ràng sẽ mở ra trang cài đặt được phép hoàn chỉnh, bao gồm Telegram, LDAP, chứng chỉ và cài đặt dịch vụ.

Các tài khoản đại diện mới sử dụng vai trò kế toán dựa trên mức tiêu thụ và phạm vi. Các tài khoản trả trước hiện tại vẫn giữ nguyên quy tắc thanh toán của riêng mình. Các khoản trợ cấp cập nhật, xóa và đặt lại mức sử dụng là riêng biệt. Gia hạn thủ công yêu cầu cả quyền cập nhật và đặt lại với phạm vi hẹp hơn. Chuyển quyền sở hữu bảo toàn thông tin xác thực của khách hàng và được giới hạn ở những chủ sở hữu đủ điều kiện. Người đại diện đã hết trợ cấp vẫn phải tuân theo chính sách ngắt kết nối đã định cấu hình.

Mỗi tài khoản đã đăng nhập có thể quản lý trình xác thực của riêng mình trong **Bảo mật tài khoản**. Việc đăng ký yêu cầu mật khẩu hiện tại và mã dùng một lần hợp lệ. Người quản lý có thể yêu cầu đăng ký hai yếu tố cho người đại diện; đang chờ đăng ký chỉ cho phép bảo mật tài khoản và đăng xuất. Bí mật và liên kết đăng ký phải được coi là thông tin xác thực.

## Nút đa vị trí

Một bảng điều khiển trung tâm quản lý các tác nhân định vị chuyên dụng qua HTTPS đã được xác minh bên ngoài. Một nút được ghép nối với một bảng điều khiển. Cài đặt tác nhân trên một máy chủ vị trí riêng biệt, sau đó chọn các lõi cần thiết trong hộp thoại Lõi của nút; cài đặt tác nhân ban đầu không kích hoạt lõi VPN. Ủy quyền quản lý nút bằng **Quản lý nút và vị trí**. Người đại diện không thể nhận được khoản trợ cấp cơ sở hạ tầng này.

Thăm dò đại lý và trang Nút hiển thị làm mới mỗi giây. Chẩn đoán máy chủ được làm mới riêng biệt, thông thường cứ 5 giây một lần. Ảnh chụp nhanh không thay đổi tránh việc viết lại bảng khách hàng; cấu hình đã thay đổi sẽ điều chỉnh các lõi bị ảnh hưởng. Cài đặt/gỡ bỏ lõi, kiểm soát thời gian chạy và chỉnh sửa cấu hình hiển thị tiến trình và xác nhận liên tục.

Tài khoản giữ lại một hạn ngạch trung tâm và đăng ký trên nhiều địa điểm. Báo cáo tích lũy được chấp nhận không được lập hóa đơn hai lần. Các thay đổi lan truyền không đồng bộ; lưu lượng trên chuyến bay và các địa điểm đồng thời có thể vượt quá hạn ngạch trong thời gian ngắn. Dữ liệu nút trực tuyến sẽ trở nên cũ sau 35 giây. Mất hợp đồng quản lý trong 90 giây sẽ dừng mặt phẳng dữ liệu VPN trong khi vẫn giữ lại bộ đếm; một ảnh chụp nhanh hợp lệ mới sẽ khôi phục dịch vụ. Xem [docs/NODES.md](docs/NODES.md) và [docs/NODES.fa.md](docs/NODES.fa.md).

## Yêu cầu hệ thống

Cần có Linux amd64 (x86-64) hoặc arm64 (aarch64), quyền truy cập root, Bash và hệ thống systemd đang chạy. Ubuntu 24.04 và Debian 13/12 là các mục tiêu triển khai được đề xuất. Các đặc quyền mạng máy chủ và các mô-đun dành riêng cho giao thức vẫn cần thiết. Việc cài đặt bảng điều khiển thành công không chứng nhận mọi giao thức trên mọi bản phân phối, kernel hoặc VPS. Xem [table.md](table.md).

Thư mục cài đặt tiêu chuẩn là `/opt/vpn-ui`. Tệp thực thi được cài đặt giữ nguyên tên tương thích `/opt/vpn-ui/vpn-ui-amd64` trên cả hai kiến ​​trúc. Tên tệp cốt lõi được gói theo kiến ​​trúc thực tế của chúng. Nút sử dụng `/opt/vpn-ui-node` và dịch vụ systemd của riêng nó.

## Cài đặt hoặc cập nhật trực tuyến

```bash
curl -fsSL https://raw.githubusercontent.com/admin6501/vpn-ui/main/deploy.sh -o deploy.sh
sudo bash deploy.sh
```

Lệnh tương tự cập nhật cài đặt tiêu chuẩn hiện có trong khi vẫn giữ lại cơ sở dữ liệu và cài đặt đăng nhập. Sao lưu trước. Các bản cài đặt mới sẽ in thông tin xác thực, cổng và đường dẫn web ngẫu nhiên. Mở URL chính xác bằng lược đồ HTTP/HTTPS đã in và thay thế địa chỉ nghe như `0.0.0.0` bằng địa chỉ có thể truy cập của máy chủ. Cài đặt mới sử dụng HTTP cho đến khi TLS được định cấu hình.

Đối với nút được ghép nối hiện có, hãy tải xuống và chạy trình cài đặt của nút đó; chọn cài đặt/cập nhật từ menu. Việc ghép nối và bộ đếm hiện tại được giữ nguyên:

```bash
curl -fsSL https://raw.githubusercontent.com/admin6501/vpn-ui/main/install-node.sh -o install-node.sh
sudo bash install-node.sh
```

Nút mới phải sử dụng lệnh ghép nối được tạo trong bảng trung tâm. Menu nút cũng cung cấp trạng thái, bắt đầu, dừng, khởi động lại, nhật ký gần đây và xác nhận xóa. `--install` giữ đường dẫn cập nhật trực tiếp không có menu. Xem [management-cli-plan.md](management-cli-plan.md).

## Cài đặt và cập nhật ngoại tuyến

Tải xuống tệp nhị phân phù hợp với kiến trúc, `install-offline.sh` và `SHA256SUMS` từ [bản phát hành ổn định](https://github.com/admin6501/vpn-ui/releases/tag/v1.0.0). Chuyển chúng lại với nhau và xác minh các tệp có sẵn trước khi cài đặt:

```bash
sha256sum --ignore-missing -c SHA256SUMS
sudo bash install-offline.sh ./vpn-ui-amd64
# ARM64: sudo bash install-offline.sh ./vpn-ui-arm64
```

Câu hỏi tiên quyết tùy chọn apt mặc định là **Không**, bao gồm cả EOF. Không giữ cài đặt bảng điều khiển cục bộ; Có yêu cầu kho lưu trữ apt có thể truy cập. lỗi apt được báo cáo và quá trình cài đặt cục bộ vẫn tiếp tục. Cài đặt trước hoặc chuyển riêng các gói hệ điều hành và mô-đun hạt nhân cần thiết. Các bản cập nhật bảo vệ cơ sở dữ liệu, thông tin đăng nhập và cài đặt TLS, đồng thời giữ các bản sao lưu khôi phục riêng tư.

Trình cài đặt ghi lại `VPNUI_OFFLINE=1`, vô hiệu hóa tải xuống gói tự động, tra cứu IP công cộng và cập nhật dữ liệu địa lý. Lưu lượng VPN vẫn hoạt động; cập nhật thủ công, phát hành/gia hạn WARP và ACME vẫn cần internet. Để cài đặt nút ngoại tuyến, hãy sử dụng nhị phân nút phù hợp và `install-node.sh` với các tùy chọn ghép nối của nó. Xem [docs/OFFLINE-INSTALL.md](docs/OFFLINE-INSTALL.md).

## Di chuyển sao lưu PasarGuard

Trước tiên hãy cài đặt bảng đích tương thích. Tài khoản VPN và thư đến của nó phải trống; cài đặt web/TLS của bảng điều khiển hiện có được giữ nguyên. Chạy:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/admin6501/vpn-ui/main/migration/install.sh)
```

Chọn tiếng Ba Tư hoặc tiếng Anh và cung cấp đường dẫn ZIP dự phòng tuyệt đối. Gói di chuyển chứa các tập lệnh và cấu hình chứ không phải một bảng nhị phân khác. Cần có Python 3 và giải nén. Để chuyển đổi ngoại tuyến, hãy chuyển và giải nén `vpn-ui-auto-migration.zip`, sau đó chạy trực tiếp `migrate.sh` của nó với đường dẫn sao lưu.

Danh tính tài khoản được hỗ trợ, quyền sở hữu của quản trị viên, nhóm/bài tập, máy chủ, băm mật khẩu và siêu dữ liệu tương thích đăng ký đều được nhập. Sử dụng mật khẩu quản trị viên trước đó; băm không thể tiết lộ mật khẩu văn bản gốc. Các nút PasarGuard không được nhập. Miền/IP đã thay đổi yêu cầu cập nhật địa chỉ cơ sở đăng ký và kiểm tra các điểm cuối đã xuất. Xem lại cấu hình đã chuyển đổi và kiểm tra các giao thức bạn thực sự sử dụng. Xem [migration/INSTALL-MIGration.fa.md](migration/INSTALL-MIGRATION.fa.md).

## Vận hành, sao lưu và phục hồi

```bash
sudo vpn-ui
systemctl status vpn-ui --no-pager
journalctl -u vpn-ui -n 100 --no-pager
```

Sử dụng chi tiết đăng nhập riêng tư được in hoặc menu quản lý để khôi phục quyền truy cập. Dừng dịch vụ trước khi sao chép các tệp SQLite và giữ lại các sidecar WAL/SHM hoặc tạo một bản sao lưu SQLite nhất quán. Bảo quản tài liệu TLS, ghi đè dịch vụ và cơ sở dữ liệu nút một cách an toàn. Việc thiết lập lại hoặc gia hạn lưu lượng truy cập không phải là khôi phục cơ sở dữ liệu. Gỡ cài đặt sẽ xóa dữ liệu; kiểm tra xác nhận của nó trước khi tiếp tục. Xem [accounts-upgrade-guide.md](accounts-upgrade-guide.md) và [SECURITY.md](SECURITY.md).

## Phát hành tài sản

| Tập tin | Mục đích |
| --- | --- |
| `vpn-ui-amd64` | Bảng điều khiển trung tâm đầy đủ cho Linux x86-64 |
| `vpn-ui-arm64` | Bảng điều khiển trung tâm đầy đủ cho Linux aarch64 |
| `vpn-ui-node-amd64` | Tác nhân định vị chuyên dụng cho Linux x86-64 |
| `vpn-ui-node-arm64` | Tác nhân định vị chuyên dụng cho Linux aarch64 |
| `install-offline.sh` | Trình cài đặt/cập nhật bảng điều khiển cục bộ có hỗ trợ khôi phục |
| `install-node.sh` | Menu cài đặt, cập nhật và quản lý nút |
| `vpn-ui-auto-migration.zip` | Tập lệnh và cấu hình chuyển đổi PasarGuard |
| `SHA256SUMS` | Tổng kiểm tra SHA-256 cho bảy tạo phẩm có thể tải xuống ở trên |

## Xây dựng, thử nghiệm và tài liệu

```bash
git clone --recurse-submodules https://github.com/admin6501/vpn-ui.git
cd vpn-ui
./build.sh --help
./build.sh
```

Sử dụng chuỗi công cụ Go từ `go.mod` và các mô-đun con/bản vá bảo mật được ghim. Bản phát hành đầy đủ phải xây dựng lõi, phần phụ trợ và tất cả dữ liệu địa lý; các gói bị bỏ qua tạo ra các tệp nhị phân phát triển. Việc xây dựng yêu cầu các phần phụ thuộc trực tuyến, chuỗi công cụ C và các công cụ xây dựng phụ trợ, bao gồm cả Docker nếu cần. ACME cần internet ngay cả khi có gói ứng dụng khách của nó.

Quy trình phát hành xác thực cả kiến ​​trúc gốc, hành vi cấp phép và tính toán tập trung, đường dẫn dữ liệu Xray/nút thực, cài đặt/cập nhật ngoại tuyến và hành vi trình duyệt bằng các ngôn ngữ được hỗ trợ. Ma trận phân phối/giao thức Incus riêng biệt yêu cầu máy chủ VM và Internet đặc quyền được chuẩn bị sẵn; sự hiện diện của nó không phải là bằng chứng cho thấy mọi mục nhập ma trận đều được chạy lại. Xem [test_unit/README.md](test_unit/README.md), [docs/INDEX.md](docs/INDEX.md), [api-reference.md](api-reference.md) và [.github/workflows/release.yml](.github/workflows/release.yml).

## Giấy phép

[GPL-3.0](LICENSE). Dự án ban đầu và thông báo của bên thứ ba vẫn còn nguyên. README được cung cấp/mô-đun con mô tả các thành phần của riêng chúng; sử dụng hướng dẫn của nĩa này để lắp đặt và vận hành bảng điều khiển.


## Bot bán hàng Telegram

Có thể cài bot Telegram tùy chọn để bán và quản lý dịch vụ VPN-UI cùng ví trên Ubuntu hoặc Debian. Cài mới tạo cơ sở dữ liệu cục bộ và không nhập dữ liệu Cloudflare. [Hướng dẫn bot](telegram-bot/README-server.txt).

```bash
curl -fsSL https://raw.githubusercontent.com/admin6501/vpn-ui/main/scripts/install-telegram-bot.sh -o install-telegram-bot.sh && sudo bash install-telegram-bot.sh
```

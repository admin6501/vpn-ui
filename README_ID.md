# VPN-UI v1.0.0 — Rilis stabil pertama

[English](README.md) | [فارسی](README_FA.md) | [العربية](README_AR.md) | [Español](README_ES.md) | [Bahasa Indonesia](README_ID.md) | [日本語](README_JA.md) | [Português (Brasil)](README_PT_BR.md) | [Русский](README_RU.md) | [Türkçe](README_TR.md) | [Українська](README_UK.md) | [Tiếng Việt](README_VI.md) | [简体中文](README_ZH.md) | [繁體中文](README_ZH_TW.md)

VPN-UI adalah panel manajemen VPN multi-protokol. **v1.0.0 adalah rilis stabil pertama dari fork ini.** Sumber, penginstal, dan pembaruan berasal dari [admin6501/vpn-ui](https://github.com/admin6501/vpn-ui), berdasarkan [Sir-MmD/vpn-ui](https://github.com/Sir-MmD/vpn-ui) dan [3X-UI](https://github.com/MHSanaei/3x-ui).

## Tiga belas bahasa

Panel, halaman browser berlangganan, bot Telegram, dan kontrol UI bersama mendukung bahasa Inggris, Persia, Arab, Spanyol, Indonesia, Jepang, Portugis Brasil, Rusia, Turki, Ukraina, Vietnam, China Sederhana, dan China Tradisional. Pilih bahasa web di pemilih bahasa; konfigurasikan bahasa bot di pengaturan Telegram. Bot menggunakan bahasa yang dikonfigurasi, bukan preferensi otomatis terpisah untuk setiap obrolan Telegram.

Permintaan web mempertahankan bahasanya sendiri, termasuk panel bersamaan dan permintaan berlangganan. Varian regional menentukan lokal yang didukung. Dialog, kalender, dan penomoran halaman menggunakan bahasa yang dipilih. Pengidentifikasi protokol, kunci konfigurasi, nama perintah, dan keluaran diagnostik mempertahankan sintaksis aslinya. Pemasang terminal menggunakan bahasa Inggris; wizard migrasi menawarkan bahasa Persia dan Inggris. Lihat [docs/TRANSLATIONS.md](docs/TRANSLATIONS.md).

## Protokol dan inti

| Waktu proses | Protokol dan implementasi |
| --- | --- |
| X-ray yang dibundel | VLESS, VMess, Trojan, Shadowsocks, Hysteria2, AnyTLS, TUIC, NaiveProxy, WireGuard |
| Backend VPN portabel | OpenVPN (TCP/UDP), OpenConnect (ocserv), L2TP, PPTP, SSTP dan IKEv2 |
| Terowongan kernel | WireGuard (`wg-c`), AmneziaWG (`awg`) dan GRE, dengan IPsec atau FOU opsional jika didukung |
| Relai bawaan | Proksi MTProto dan proksi SSH dengan otentikasi per akun |

IPsec dapat dipilih dalam instalasi inti dan juga merupakan ketergantungan bersama dari L2TP/IKEv2. Ketergantungan yang diperlukan dipertahankan sementara inti lain yang terinstal membutuhkannya. Kernel WireGuard dan Xray WireGuard asli adalah implementasi yang berbeda. Fitur SSH bawaan meneruskan lalu lintas VPN; itu tidak menyediakan shell root.

Biner lengkap mencakup backend portabel, inti Xray yang ditambal, klien ACME dan enam file geodata: base, geoip/geosite Iran dan Rusia. Paket OS, modul kernel, header kernel, dan DKMS tetap menjadi persyaratan host. Halaman Inti menunjukkan kemajuan instalasi, prasyarat yang hilang, dan status layanan sebenarnya. GRE biasa tidak terenkripsi kecuali IPsec diaktifkan; PPTP adalah protokol lama. Lihat [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## Pengguna, grup, dan langganan

Kelola pelanggan di **Pengguna**, infrastruktur di **Masuk**, dan tugas masuk yang dapat digunakan kembali di **Grup**. Satu akun dapat menggunakan beberapa inbound dan lokasi yang kompatibel dengan kuota dan langganan bersama. Perubahan grup memperbarui tugas tanpa mengganti kredensial akun. Peralihan keanggotaan individual dan peralihan akun global tetap terpisah.

Batas lalu lintas disimpan dalam byte meskipun nama bidang API `totalGB`. Akun dengan batas lalu lintas nol tidak terbatas dalam lalu lintas; berakhirnya angka nol berarti waktu yang tidak terbatas. Rencana mulai tertunda dimulai pada penggunaan pertama. Total ikhtisar menghitung akun unik, bukan satu pengguna untuk setiap keanggotaan. Berbagi ID langganan antar akun yang berbeda akan memublikasikan konfigurasinya bersama-sama; itu tidak menggabungkan identitas mereka atau mengubah batasan terpisah menjadi satu kuota akun bersama.

Inti yang dibundel mengatribusikan penggunaan baru ke sumbernya yang masuk. Info Akun menunjukkan saham terukur dan sisa historis yang tidak diatribusikan secara terpisah. Peningkatan versi mempertahankan penggunaan yang sudah ada; catatan lama yang hanya berisi akun tidak dapat membuat ulang pemisahan historis. Titik akhir Proksi Eksternal digunakan secara konsisten dalam ekspor akun/konfigurasi. Profil OpenVPN dan file rekan WireGuard dapat diunduh dari tampilan konfigurasi yang sesuai.

### Perbarui, setel ulang lalu lintas, dan bekukan

**Pengguna → menu tiga titik klien → Perpanjang** menghapus unggahan/unduhan saat ini di seluruh keanggotaannya, memulai durasi paket penuh dari sekarang dan mengaktifkan akun. Paket khusus lalu lintas, hanya waktu, gabungan, dan tidak terbatas didukung. Kuota, kata sandi, UUID, ID langganan, kunci perangkat, alamat rekan, dan akses masuk tetap tidak berubah. Durasi warisan yang tidak diketahui menanyakan jumlah hari sekali dan mengingatnya.

**Setel ulang lalu lintas** menghapus penggunaan saat ini tanpa memulai ulang masa berlakunya atau mengaktifkan akun yang dinonaktifkan secara manual. **Bekukan** menjeda hari-hari yang tersisa; pencairan akan mengembalikan sisanya. Perpanjangan menggunakan durasi paket penuh yang tercatat. Penghitungan seumur hidup dan penghitung node kumulatif dipertahankan. Batasan perwakilan dan keanggotaan penyandang disabilitas masih berlaku. Lihat [docs/CLIENT-RENEWAL.md](docs/CLIENT-RENEWAL.md) dan [docs/PANEL-GUIDE.md](docs/PANEL-GUIDE.md).

### Batas kecepatan dan akses

Pengabaian unduh/unggah menerima KB/dtk, MB/dtk, atau Mbps. Kosong mewarisi pengaturan masuk; nol berarti tidak terbatas ke arah itu. Penyimpanan menggunakan 1024-byte KB/s, sedangkan Mbps berarti megabit desimal. MB/dtk UI menggunakan byte berbasis 1024: **3 MB/s = 24 Mib/s ≈ 25.17 Mbps**. Nilai kecepatan dibulatkan menjadi setidaknya 1 KB/s bila positif.

Batas IP Sumber menghitung alamat yang terlihat, bukan perangkat fisik di balik satu NAT. Batas sesi dan rekan bergantung pada protokol. WireGuard/AmneziaWG mengeluarkan konfigurasi perangkat terpisah. Batasan yang didukung ditampilkan di setiap formulir; batas sesi/IP node bersifat lokal untuk setiap node.

## Pengurus dan perwakilan

Administrator awal adalah pemilik panel yang dilindungi. Peran administrator dan hibah khusus menentukan operasi; tugas grup/masuk menentukan cakupannya. Basis data yang sensitif terhadap pemilik, autentikasi, dan operasi yang dapat dieksekusi mempertahankan perlindungan terpisah. Hibah pengaturan panel eksplisit membuka halaman lengkap pengaturan yang diizinkan, termasuk Telegram, LDAP, sertifikat, dan pengaturan layanan.

Akun perwakilan baru menggunakan akuntansi berbasis konsumsi dan peran tercakup. Akun prabayar yang ada mempertahankan aturan penagihannya sendiri. Hibah pembaruan, penghapusan, dan pengaturan ulang penggunaan bersifat terpisah. Perpanjangan manual memerlukan izin pembaruan dan pengaturan ulang dengan cakupan yang lebih sempit. Transfer kepemilikan mempertahankan kredensial pelanggan dan dibatasi untuk pemilik yang memenuhi syarat. Perwakilan yang jatahnya habis tetap tunduk pada kebijakan pemutusan yang dikonfigurasi.

Setiap akun yang masuk dapat mengelola pengautentikasinya sendiri di **Keamanan akun**. Pendaftaran memerlukan kata sandi saat ini dan kode satu kali yang valid. Manajer dapat mewajibkan pendaftaran dua faktor untuk perwakilan; pendaftaran yang tertunda hanya mengizinkan keamanan akun dan logout. Rahasia dan tautan berlangganan harus diperlakukan sebagai kredensial.

## Node multi-lokasi

Satu panel pusat mengelola agen lokasi khusus melalui HTTPS terverifikasi keluar. Sebuah node dipasangkan dengan satu panel. Instal agen pada server lokasi terpisah, lalu pilih inti yang diperlukan dalam dialog Inti simpul; instalasi agen awal tidak mengaktifkan inti VPN. Delegasikan pengelolaan node dengan **Kelola node dan lokasi**. Perwakilan tidak dapat memperoleh hibah infrastruktur ini.

Jajak pendapat agen dan halaman Node yang terlihat disegarkan setiap detik. Diagnostik host diperbarui secara terpisah, biasanya setiap lima detik. Snapshot yang tidak diubah menghindari penulisan ulang tabel klien; konfigurasi yang diubah merekonsiliasi inti yang terpengaruh. Instalasi/penghapusan inti, kontrol runtime dan pengeditan konfigurasi menunjukkan kemajuan dan pengakuan yang terus-menerus.

Akun mempertahankan satu kuota pusat dan langganan di seluruh lokasi. Laporan kumulatif yang diterima tidak ditagih dua kali. Perubahan menyebar secara asinkron; lalu lintas dalam penerbangan dan lokasi simultan dapat melebihi kuota untuk sementara waktu. Data node online menjadi basi setelah 35 detik. Kehilangan sewa pengelolaan selama 90 detik menghentikan bidang data VPN sambil mempertahankan penghitung; layanan pemulihan snapshot baru yang valid. Lihat [docs/NODES.md](docs/NODES.md) dan [docs/NODES.fa.md](docs/NODES.fa.md).

## Persyaratan sistem

Linux amd64 (x86-64) atau arm64 (aarch64), akses root, Bash, dan sistem systemd yang sedang berjalan diperlukan. Ubuntu 24.04 dan Debian 12/13 adalah target penerapan yang direkomendasikan. Hak istimewa jaringan host dan modul khusus protokol masih diperlukan. Instalasi panel yang berhasil tidak mensertifikasi setiap protokol di setiap distribusi, kernel, atau VPS. Lihat [tabel.md](table.md).

Direktori instalasi standar adalah `/opt/vpn-ui`. Eksekusi yang diinstal menyimpan nama kompatibilitas `/opt/vpn-ui/vpn-ui-amd64` pada kedua arsitektur. Nama file inti yang dibundel mengikuti arsitektur sebenarnya. Node menggunakan `/opt/vpn-ui-node` dan layanan systemd miliknya sendiri.

## Instal atau perbarui secara online

```bash
curl -fsSL https://raw.githubusercontent.com/admin6501/vpn-ui/main/deploy.sh -o deploy.sh
sudo bash deploy.sh
```

Perintah yang sama memperbarui instalasi standar yang ada sambil mempertahankan database dan pengaturan loginnya. Cadangkan dulu. Instalasi baru mencetak kredensial acak, port dan jalur web. Buka URL yang tepat dengan skema HTTP/HTTPS yang dicetak dan ganti alamat pendengar seperti `0.0.0.0` dengan alamat server yang dapat dijangkau. Instalasi baru menggunakan HTTP hingga TLS dikonfigurasi.

Untuk node berpasangan yang sudah ada, unduh dan jalankan penginstalnya; pilih instal/perbarui dari menu. Pasangan dan penghitung yang ada dipertahankan:

```bash
curl -fsSL https://raw.githubusercontent.com/admin6501/vpn-ui/main/install-node.sh -o install-node.sh
sudo bash install-node.sh
```

Node baru harus menggunakan perintah pemasangan yang dihasilkan di panel pusat. Menu simpul juga menyediakan status, mulai, berhenti, mulai ulang, log terkini, dan penghapusan yang dikonfirmasi. `--install` mempertahankan jalur pembaruan non-menu langsung. Lihat [manajemen-cli-plan.md](management-cli-plan.md).

## Instalasi dan pembaruan offline

Unduh biner yang cocok dengan arsitektur, `install-offline.sh` dan `SHA256SUMS` dari [rilis stabil](https://github.com/admin6501/vpn-ui/releases/tag/v1.0.0). Transfer bersama-sama dan verifikasi file yang tersedia sebelum instalasi:

```bash
sha256sum --ignore-missing -c SHA256SUMS
sudo bash install-offline.sh ./vpn-ui-amd64
# ARM64: sudo bash install-offline.sh ./vpn-ui-arm64
```

Pertanyaan prasyarat opsional yang tepat defaultnya adalah **Tidak**, termasuk EOF. Tidak ada instalasi panel yang tetap lokal; Ya, memerlukan repositori apt yang dapat dijangkau. kegagalan apt dilaporkan dan instalasi lokal dilanjutkan. Pra-instal atau transfer paket OS dan modul kernel yang diperlukan secara terpisah. Pembaruan mempertahankan database, kredensial, dan pengaturan TLS serta menyimpan cadangan rollback pribadi.

Penginstal mencatat `VPNUI_OFFLINE=1`, menonaktifkan pengunduhan paket otomatis, pencarian IP publik, dan pembaruan geodata. Lalu lintas VPN masih berfungsi; pembaruan manual, penerbitan/perpanjangan WARP dan ACME masih membutuhkan internet. Untuk instalasi node offline gunakan biner node yang cocok dan `install-node.sh` dengan opsi pemasangannya. Lihat [docs/OFFLINE-INSTALL.md](docs/OFFLINE-INSTALL.md).

## Migrasi cadangan PasarGuard

Instal panel tujuan yang kompatibel terlebih dahulu. Akun VPN dan masuknya harus kosong; pengaturan panel web/TLS yang ada dipertahankan. Jalankan:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/admin6501/vpn-ui/main/migration/install.sh)
```

Pilih bahasa Persia atau Inggris dan berikan jalur ZIP cadangan absolut. Paket migrasi berisi skrip dan konfigurasi, bukan biner panel lainnya. Python 3 dan unzip diperlukan. Untuk konversi offline, transfer dan ekstrak `vpn-ui-auto-migration.zip`, lalu jalankan `migrate.sh` langsung dengan jalur cadangan.

Identitas akun yang didukung, kepemilikan administrator, grup/penugasan, host, hash kata sandi, dan metadata kompatibilitas langganan diimpor. Gunakan kata sandi administrator sebelumnya; hash tidak dapat mengungkapkan kata sandi teks biasa. Node PasarGuard tidak diimpor. Domain/IP yang diubah memerlukan pembaruan alamat basis langganan dan pemeriksaan titik akhir yang diekspor. Tinjau konfigurasi yang dikonversi dan uji protokol yang sebenarnya Anda gunakan. Lihat [migration/INSTALL-MIGRATION.fa.md](migration/INSTALL-MIGRATION.fa.md).

## Operasi, pencadangan, dan pemulihan

```bash
sudo vpn-ui
systemctl status vpn-ui --no-pager
journalctl -u vpn-ui -n 100 --no-pager
```

Gunakan detail login pribadi yang dicetak atau menu manajemen untuk memulihkan akses. Hentikan layanan sebelum menyalin file SQLite dan pertahankan sidecar WAL/SHM, atau buat cadangan SQLite yang konsisten. Pertahankan materi TLS, penggantian layanan, dan database node dengan aman. Penyetelan ulang atau pembaruan lalu lintas bukanlah pemulihan basis data. Copot pemasangan menghapus data; periksa konfirmasinya sebelum melanjutkan. Lihat [accounts-upgrade-guide.md](accounts-upgrade-guide.md) dan [SECURITY.md](SECURITY.md).

## Lepaskan aset

| Mengajukan | Tujuan |
| --- | --- |
| `vpn-ui-amd64` | Panel tengah penuh untuk Linux x86-64 |
| `vpn-ui-arm64` | Panel pusat penuh untuk Linux aarch64 |
| `vpn-ui-node-amd64` | Agen lokasi khusus untuk Linux x86-64 |
| `vpn-ui-node-arm64` | Agen lokasi khusus untuk Linux aarch64 |
| `install-offline.sh` | Penginstal/pembaruan panel lokal dengan dukungan rollback |
| `install-node.sh` | Menu instalasi, pembaruan, dan manajemen node |
| `vpn-ui-auto-migration.zip` | Skrip dan konfigurasi konversi PasarGuard |
| `SHA256SUMS` | Checksum SHA-256 untuk tujuh artefak yang dapat diunduh di atas |

## Pembuatan, pengujian, dan dokumentasi

```bash
git clone --recurse-submodules https://github.com/admin6501/vpn-ui.git
cd vpn-ui
./build.sh --help
./build.sh
```

Gunakan rantai alat Go dari `go.mod` dan submodul/patch keamanan yang disematkan. Rilis penuh harus membangun inti, backend, dan semua geodatanya; bundel yang dilewati menghasilkan biner pengembangan. Pembangunan memerlukan dependensi online, rantai alat C, dan alat pembangunan backend, termasuk Docker jika diperlukan. ACME membutuhkan internet bahkan dengan kliennya yang dibundel.

Alur kerja rilis memvalidasi arsitektur asli, akuntansi terfokus dan perilaku izin, jalur data Xray/node nyata, penginstalan/pembaruan offline, dan perilaku browser dalam bahasa yang didukung. Protokol/matriks distribusi Incus yang terpisah memerlukan host VM dan internet istimewa yang telah disiapkan; kehadirannya bukanlah bukti bahwa setiap entri matriks diputar ulang. Lihat [test_unit/README.md](test_unit/README.md), [docs/INDEX.md](docs/INDEX.md), [api-reference.md](api-reference.md) dan [.github/workflows/release.yml](.github/workflows/release.yml).

## Lisensi

[GPL-3.0](LICENSE). Proyek asli dan pemberitahuan pihak ketiga tetap utuh. README yang divendorkan/submodul menjelaskan komponennya sendiri; gunakan panduan garpu ini untuk pemasangan dan pengoperasian panel.

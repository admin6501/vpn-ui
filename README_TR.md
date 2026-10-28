# VPN-UI v1.0.0 — İlk kararlı sürüm

[English](README.md) | [فارسی](README_FA.md) | [العربية](README_AR.md) | [Español](README_ES.md) | [Bahasa Indonesia](README_ID.md) | [日本語](README_JA.md) | [Português (Brasil)](README_PT_BR.md) | [Русский](README_RU.md) | [Türkçe](README_TR.md) | [Українська](README_UK.md) | [Tiếng Việt](README_VI.md) | [简体中文](README_ZH.md) | [繁體中文](README_ZH_TW.md)

VPN-UI, çok protokollü bir VPN yönetim panelidir. **v1.0.0, bu çatalın ilk kararlı sürümüdür.** Kaynak, yükleyiciler ve güncellemeler, [Sir-MmD/vpn-ui](https://github.com/admin6501/vpn-ui) ve [3X-UI](https://github.com/Sir-MmD/vpn-ui) temel alınarak [admin6501/vpn-ui](https://github.com/MHSanaei/3x-ui) adresinden gelir.

## On üç dil

Panel, abonelik tarayıcı sayfası, Telegram botu ve paylaşılan kullanıcı arayüzü kontrolleri İngilizce, Farsça, Arapça, İspanyolca, Endonezce, Japonca, Brezilya Portekizcesi, Rusça, Türkçe, Ukraynaca, Vietnamca, Basitleştirilmiş Çince ve Geleneksel Çince'yi desteklemektedir. Dil seçicide web dilini seçin; Telegram ayarlarında bot dilini yapılandırın. Bot, her Telegram sohbeti için ayrı bir otomatik tercihi değil, yapılandırılmış dili kullanır.

Eş zamanlı panel ve abonelik istekleri de dahil olmak üzere web istekleri kendi dillerini korur. Bölgesel değişkenler desteklenen yerel ayarlara çözümlenir. Diyaloglar, takvimler ve sayfalandırmalar seçilen dili kullanır. Protokol tanımlayıcıları, konfigürasyon anahtarları, komut adları ve tanılama çıktıları orijinal söz dizimini korur. Terminal kurulumcuları İngilizce kullanır; geçiş sihirbazı Farsça ve İngilizce sunmaktadır. Bkz. [docs/TRANSLATIONS.md](docs/TRANSLATIONS.md).

## Protokoller ve çekirdekler

| Çalışma zamanı | Protokoller ve uygulamalar |
| --- | --- |
| Paketlenmiş Röntgen | VLESS, VMess, Trojan, Shadowsocks, Hysteria2, AnyTLS, TUIC, NaiveProxy, WireGuard |
| Taşınabilir VPN arka uçları | OpenVPN (TCP/UDP), OpenConnect (ocserv), L2TP, PPTP, SSTP ve IKEv2 |
| Çekirdek tünelleri | WireGuard (`wg-c`), AmneziaWG (`awg`) ve GRE, desteklendiğinde isteğe bağlı IPsec veya FOU ile |
| Dahili röleler | Hesap başına kimlik doğrulamayla MTProto proxy ve SSH proxy |

IPsec, çekirdek kurulumda seçilebilir ve aynı zamanda L2TP/IKEv2'nin paylaşılan bir bağımlılığıdır. Kurulu başka bir çekirdek onlara ihtiyaç duyarken gerekli bağımlılıklar korunur. Kernel WireGuard ve yerel Xray WireGuard farklı uygulamalardır. Yerleşik SSH özelliği VPN trafiğini iletir; bir kök kabuk sağlamaz.

Tam ikili dosyalar arasında taşınabilir arka uçlar, yamalı Xray çekirdeği, bir ACME istemcisi ve altı coğrafi veri dosyası bulunur: taban, İran ve Rusya geoip/geosit. İşletim sistemi paketleri, çekirdek modülleri, çekirdek başlıkları ve DKMS, ana bilgisayar gereksinimleri olmaya devam ediyor. Çekirdekler sayfası yükleme ilerlemesini, eksik önkoşulları ve gerçek hizmet durumunu gösterir. Düz GRE, IPsec etkinleştirilmediği sürece şifrelenmez; PPTP eski bir protokoldür. Bkz. [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## Kullanıcılar, gruplar ve abonelikler

**Kullanıcılar**'da müşterileri, **Gelenler**'de altyapıyı ve **Gruplar**'da yeniden kullanılabilir gelen atamaları yönetin. Bir hesap, paylaşılan bir kota ve abonelikle birden fazla uyumlu gelen ve konumu kullanabilir. Grup, hesap kimlik bilgilerini değiştirmeden güncelleme atamalarını değiştirir. Bireysel üyelik anahtarları ve genel hesap anahtarı ayrı kalır.

API alan adı `totalGB` olmasına rağmen trafik sınırları bayt cinsinden saklanır. Sıfır trafik limiti olan bir hesabın trafikte sınırsız olması; sıfırın sona ermesi sınırsız süre anlamına gelir. Gecikmeli başlatma planları ilk kullanımda başlar. Genel bakış toplamları, her üyelik için bir kullanıcıyı değil, benzersiz hesapları sayar. Abonelik kimliğinin farklı hesaplar arasında paylaşılması, bunların yapılandırmalarını birlikte yayınlar; kimliklerini birleştirmez veya ayrı limitlerini tek bir ortak hesap kotasına dönüştürmez.

Paketlenmiş çekirdek, yeni kullanımı kaynağına bağlar. Hesap Bilgileri, ölçülen hisseleri ve geçmişe ait ilişkilendirilmemiş bakiyeleri ayrı ayrı gösterir. Yükseltme mevcut kullanımı korur; yalnızca hesaba ait eski kayıtlar, geçmiş bir bölünmeyi yeniden oluşturamaz. Hesap/yapılandırma dışa aktarımlarında harici Proxy uç noktaları tutarlı bir şekilde kullanılır. OpenVPN profilleri ve WireGuard eş dosyaları, ilgili yapılandırma görünümlerinden indirilebilir.

### Trafiği yenileyin, sıfırlayın ve dondurun

**Kullanıcılar → müşterinin üç noktalı menüsü → Yenile** üyeliklerdeki mevcut yükleme/indirmeyi temizler, tam paket süresini şu andan itibaren başlatır ve hesabı etkinleştirir. Yalnızca trafik, yalnızca süreli, birleşik ve tamamen sınırsız planlar desteklenir. Kota, şifre, UUID, abonelik kimliği, cihaz anahtarları, eş adresleri ve gelen erişim değişmeden kalır. Bilinmeyen miras süreleri bir kez gün sayısını sorar ve hatırlar.

**Trafiği sıfırla**, geçerlilik süresini yeniden başlatmadan veya manuel olarak devre dışı bırakılan bir hesabı etkinleştirmeden mevcut kullanımı temizler. **Dondur** kalan günleri duraklatır; dondurma işlemi geri kalan kısmı geri yükler. Yenilemede bunun yerine kayıtlı paket süresinin tamamı kullanılır. Ömür boyu muhasebe ve kümülatif düğüm sayaçları korunur. Temsilci sınırlamaları ve engelli üyelikler hâlâ geçerlidir. Bkz. [docs/CLIENT-RENEWAL.md](docs/CLIENT-RENEWAL.md) ve [docs/PANEL-GUIDE.md](docs/PANEL-GUIDE.md).

### Hız ve erişim sınırları

İndirme/yükleme geçersiz kılmaları KB/s, MB/s veya Mbps'yi kabul eder. Boş, gelen ayarı devralır; sıfır o yönde sınırsız demektir. Depolama 1024 bayt KB/s kullanır, Mbps ise ondalık megabit anlamına gelir. Kullanıcı arayüzünün MB/sn'si 1024 tabanlı bayt kullanır: **3 MB/s = 24 Mib/s ≈ 25.17 Mbps**. Hız değerleri pozitif olduğunda en az 1 KB/s'ye yuvarlanır.

Kaynak IP sınırları, bir NAT'ın arkasındaki fiziksel cihazları değil, görünür adresleri sayar. Oturum ve eş sınırları protokole bağlıdır. WireGuard/AmneziaWG ayrı cihaz yapılandırmaları yayınlar. Desteklenen limitler her formda gösterilmektedir; düğüm oturumu/IP sınırları her düğüm için yereldir.

## Yöneticiler ve temsilciler

İlk yönetici, korunan panelin sahibidir. Yönetici rolleri ve özel izinler işlemleri belirler; grup/gelen atamalar kapsamlarını belirler. Sahibine duyarlı veritabanı, kimlik doğrulama ve yürütülebilir işlemler ayrı korumayı korur. Açık bir panel ayarları izni, Telegram, LDAP, sertifikalar ve hizmet ayarları dahil olmak üzere izin verilen ayarlar sayfasının tamamını açar.

Yeni temsilci hesapları, tüketime dayalı muhasebeyi ve kapsamlı rolleri kullanır. Mevcut ön ödemeli hesaplar kendi faturalandırma kurallarını korur. Güncelleme, silme ve kullanım sıfırlama izinleri ayrıdır. Manuel yenileme, daha dar kapsamda hem güncelleme hem de sıfırlama izni gerektirir. Sahiplik aktarımı, müşteri kimlik bilgilerini korur ve uygun sahiplerle sınırlıdır. Ödeneği tükenen bir temsilci, yapılandırılmış bağlantı kesme ilkesine tabi olmaya devam eder.

Oturum açılan her hesap, **Hesap güvenliği**'nde kendi kimlik doğrulayıcısını yönetebilir. Kayıt, geçerli şifreyi ve geçerli bir tek kullanımlık kodu gerektirir. Yöneticiler temsilciler için iki faktörlü kayıt talep edebilir; Bekleyen kayıt yalnızca hesap güvenliğine ve oturumu kapatmaya izin verir. Sırlar ve abonelik bağlantıları kimlik bilgileri olarak değerlendirilmelidir.

## Çok konumlu düğümler

Tek bir merkezi panel, giden doğrulanmış HTTPS üzerinden özel konum aracılarını yönetir. Bir düğüm bir panelle eşleştirilir. Aracıyı ayrı bir konum sunucusuna yükleyin, ardından düğümün Çekirdekler iletişim kutusunda gerekli çekirdekleri seçin; ilk aracı kurulumu hiçbir VPN çekirdeğini etkinleştirmez. **Düğümleri ve konumları yönetin** ile düğüm yönetimini devredin. Temsilciler bu altyapı hibesini alamazlar.

Temsilci yoklaması ve görünür Düğümler sayfası her saniye yenilenir. Ana bilgisayar tanılamaları normalde her beş saniyede bir ayrı olarak yenilenir. Değiştirilmemiş anlık görüntüler, istemci tablolarının yeniden yazılmasını önler; Değiştirilen konfigürasyon, etkilenen çekirdekleri uzlaştırır. Çekirdek yükleme/kaldırma, çalışma zamanı kontrolleri ve yapılandırma düzenlemeleri, kalıcı ilerleme ve onayları gösterir.

Hesaplarda konumlar arasında tek bir merkezi kota ve abonelik bulunur. Kabul edilen kümülatif raporlar iki kez faturalandırılmaz. Değişiklikler eşzamansız olarak yayılır; uçuş içi trafik ve eşzamanlı konumlar kısa süreliğine kotayı aşabilir. Çevrimiçi düğüm verileri 35 saniye sonra eski hale gelir. Yönetim kiralamasının 90 saniye süreyle kaybedilmesi, sayaçları korurken VPN veri düzlemini durdurur; yeni ve geçerli bir anlık görüntü hizmeti geri yükler. Bkz. [docs/NODES.md](docs/NODES.md) ve [docs/NODES.fa.md](docs/NODES.fa.md).

## Sistem gereksinimleri

Linux amd64 (x86-64) veya arm64 (aarch64), root erişimi, Bash ve çalışan bir systemd sistemi gereklidir. Ubuntu 24.04 ve Debian 12/13 önerilen dağıtım hedefleridir. Ana bilgisayar ağı ayrıcalıkları ve protokole özgü modüller hâlâ gereklidir. Başarılı bir panel kurulumu, her dağıtım, çekirdek veya VPS'deki her protokolü onaylamaz. Bkz. [tablo.md](table.md).

Standart kurulum dizini `/opt/vpn-ui`'dir. Yüklenen yürütülebilir dosya, her iki mimaride de `/opt/vpn-ui/vpn-ui-amd64` uyumluluk adını korur. Paketlenmiş çekirdek dosya adları gerçek mimarilerini takip eder. Düğüm `/opt/vpn-ui-node` ve kendi systemd hizmetini kullanır.

## Çevrimiçi yükleyin veya güncelleyin

```bash
curl -fsSL https://raw.githubusercontent.com/admin6501/vpn-ui/main/deploy.sh -o deploy.sh
sudo bash deploy.sh
```

Aynı komut, veritabanını ve oturum açma ayarlarını korurken mevcut bir standart kurulumu günceller. Önce yedekleyin. Yeni kurulumlar rastgele kimlik bilgilerini, bağlantı noktasını ve web yolunu yazdırır. Yazdırılan HTTP/HTTPS şemasıyla tam URL'yi açın ve `0.0.0.0` gibi bir dinleme adresini sunucunun ulaşılabilir adresiyle değiştirin. Yeni kurulumlar, TLS yapılandırılana kadar HTTP kullanır.

Mevcut eşleştirilmiş bir düğüm için yükleyicisini indirin ve çalıştırın; menüden yükle/güncelle'yi seçin. Mevcut eşleştirme ve sayaçlar korunur:

```bash
curl -fsSL https://raw.githubusercontent.com/admin6501/vpn-ui/main/install-node.sh -o install-node.sh
sudo bash install-node.sh
```

Yeni bir düğüm, merkezi panelde oluşturulan eşleştirme komutunu kullanmalıdır. Düğüm menüsü ayrıca durum, başlatma, durdurma, yeniden başlatma, son günlükler ve onaylanmış kaldırma işlemlerini de sağlar. `--install` doğrudan menü dışı güncelleme yolunu tutar. Bkz. [management-cli-plan.md](management-cli-plan.md).

## Çevrimdışı kurulum ve güncelleme

Mimariyle eşleşen `install-offline.sh` ve `SHA256SUMS` ikili dosyasını [kararlı sürümden](https://github.com/admin6501/vpn-ui/releases/tag/v1.0.0) indirin. Bunları birlikte aktarın ve kurulumdan önce mevcut dosyaları doğrulayın:

```bash
sha256sum --ignore-missing -c SHA256SUMS
sudo bash install-offline.sh ./vpn-ui-amd64
# ARM64: sudo bash install-offline.sh ./vpn-ui-arm64
```

İsteğe bağlı apt önkoşul sorusu, EOF dahil olmak üzere varsayılan olarak **Hayır** şeklindedir. Hayır, panel kurulumunu yerel tutar; Evet ulaşılabilir apt depoları gerektirir. apt hataları raporlanır ve yerel kurulum devam eder. Gerekli işletim sistemi paketlerini ve çekirdek modüllerini ayrı ayrı önceden yükleyin veya aktarın. Güncellemeler veritabanını, kimlik bilgilerini ve TLS ayarlarını korur ve özel geri alma yedeklerini saklar.

Yükleyici `VPNUI_OFFLINE=1` kayıtlarını yaparak otomatik paket indirmeleri, genel IP aramasını ve coğrafi veri güncellemelerini devre dışı bırakır. VPN trafiği hala çalışıyor; manuel güncellemeler, WARP ve ACME verilmesi/yenilenmesi hala internete ihtiyaç duyuyor. Çevrimdışı düğüm kurulumu için, eşleştirme seçenekleriyle eşleşen düğüm ikilisini ve `install-node.sh`'yi kullanın. Bkz. [docs/OFFLINE-INSTALL.md](docs/OFFLINE-INSTALL.md).

## PasarGuard yedekleme geçişi

Önce uyumlu bir hedef panel yükleyin. VPN hesapları ve gelenleri boş olmalıdır; mevcut panel web/TLS ayarları korunur. Çalıştır:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/admin6501/vpn-ui/main/migration/install.sh)
```

Farsça veya İngilizce'yi seçin ve mutlak yedek ZIP yolunu sağlayın. Geçiş paketi, başka bir panel ikili dosyasını değil, komut dosyalarını ve yapılandırmayı içerir. Python 3 ve unzip gereklidir. Çevrimdışı dönüştürme için `vpn-ui-auto-migration.zip`'yi aktarın ve çıkarın, ardından `migrate.sh`'yi doğrudan yedekleme yoluyla çalıştırın.

Desteklenen hesap kimlikleri, yönetici sahipliği, gruplar/atamalar, ana bilgisayarlar, parola karmaları ve abonelik uyumluluğu meta verileri içe aktarılır. Önceki yönetici şifresini kullanın; karmalar düz metin parolasını açığa çıkaramaz. PasarGuard düğümleri içe aktarılmaz. Değiştirilen alan adı/IP, abonelik temel adresinin güncellenmesini ve dışa aktarılan uç noktaların kontrol edilmesini gerektirir. Dönüştürülen yapılandırmayı gözden geçirin ve gerçekte kullandığınız protokolleri test edin. Bkz. [migration/INSTALL-MIGRATION.fa.md](migration/INSTALL-MIGRATION.fa.md).

## Çalıştırma, yedekleme ve kurtarma

```bash
sudo vpn-ui
systemctl status vpn-ui --no-pager
journalctl -u vpn-ui -n 100 --no-pager
```

Erişimi kurtarmak için yazdırılan özel oturum açma ayrıntılarını veya yönetim menüsünü kullanın. SQLite dosyalarını kopyalamadan önce hizmeti durdurun ve WAL/SHM sepetlerini koruyun veya tutarlı bir SQLite yedeklemesi yapın. TLS malzemesini, hizmet geçersiz kılmalarını ve düğüm veritabanlarını güvenli bir şekilde koruyun. Trafiğin sıfırlanması veya yenilenmesi, veritabanı geri yüklemesi değildir. Kaldırma verileri kaldırır; Devam etmeden önce onayını inceleyin. Bkz. [accounts-upgrade-guide.md](accounts-upgrade-guide.md) ve [SECURITY.md](SECURITY.md).

## Varlıkları serbest bırak

| Dosya | Amaç |
| --- | --- |
| `vpn-ui-amd64` | Linux x86-64 için tam merkezi panel |
| `vpn-ui-arm64` | Linux aarch64 için tam merkezi panel |
| `vpn-ui-node-amd64` | Linux x86-64 için özel konum aracısı |
| `vpn-ui-node-arm64` | Linux aarch64 için özel konum aracısı |
| `install-offline.sh` | Geri alma desteğiyle yerel panel yükleyici/güncelleme |
| `install-node.sh` | Node kurulumu, güncelleme ve yönetim menüsü |
| `vpn-ui-auto-migration.zip` | PasarGuard dönüşüm komut dosyaları ve yapılandırması |
| `SHA256SUMS` | Yukarıdaki indirilebilir yedi yapı için SHA-256 sağlama toplamları |

## Oluşturma, test etme ve belgeleme

```bash
git clone --recurse-submodules https://github.com/admin6501/vpn-ui.git
cd vpn-ui
./build.sh --help
./build.sh
```

`go.mod`'deki Go araç zincirini ve sabitlenmiş alt modülleri/güvenlik yamalarını kullanın. Tam sürüm, çekirdeklerini, arka uçlarını ve tüm coğrafi verilerini oluşturmalıdır; atlanan paketler geliştirme ikili dosyaları üretir. Oluşturma, çevrimiçi bağımlılıkları, bir C araç zincirini ve gerektiğinde Docker da dahil olmak üzere arka uç oluşturma araçlarını gerektirir. ACME, istemci paketiyle birlikte bile internete ihtiyaç duyuyor.

Sürüm iş akışı, desteklenen dillerdeki yerel mimarileri, odaklanmış muhasebe ve izin davranışını, gerçek Xray/düğüm veri yollarını, çevrimdışı yükleme/güncellemeyi ve tarayıcı davranışını doğrular. Ayrı Incus protokolü/dağıtım matrisi, hazırlanmış ayrıcalıklı bir VM ana bilgisayarı ve internet gerektirir; varlığı her matris girişinin yeniden çalıştırıldığının kanıtı değildir. Bkz. [test_unit/README.md](test_unit/README.md), [docs/INDEX.md](docs/INDEX.md), [api-reference.md](api-reference.md) ve [.github/workflows/release.yml](.github/workflows/release.yml).

## Lisans

[GPL-3.0](LICENSE). Orijinal proje ve üçüncü taraf bildirimleri bozulmadan kalır. Satıcı/alt modül README'ler kendi bileşenlerini tanımlar; Panel kurulumu ve çalıştırılması için bu çatalın kılavuzlarını kullanın.


## Telegram satış botu

VPN-UI için cüzdanlı isteğe bağlı Telegram satış ve servis yönetimi botu Ubuntu veya Debian üzerine kurulabilir. Yeni kurulum yerel veritabanı oluşturur; Cloudflare verilerini aktarmaz. [Bot kılavuzu](telegram-bot/README-server.txt).

```bash
curl -fsSL https://raw.githubusercontent.com/admin6501/vpn-ui/main/scripts/install-telegram-bot.sh -o install-telegram-bot.sh && sudo bash install-telegram-bot.sh
```

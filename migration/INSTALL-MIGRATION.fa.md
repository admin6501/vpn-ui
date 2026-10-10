بسته مهاجرت فقط اسکریپت‌ها و پیکربندی پیش‌فرض را دارد؛ هیچ باینری پنل یا ابزار اجرایی داخل آن نیست. نسخه سازگار admin6501/vpn-ui باید از قبل نصب باشد. اسکریپت از ساختار دیتابیس همان پنل استفاده می‌کند و فایل اجرایی را تغییر نمی‌دهد.

The migration package contains scripts and default configuration only. A compatible admin6501/vpn-ui panel must already be installed. Migration uses its database schema and leaves the installed binary untouched.

## نصب و اجرای خودکار / Automatic migration

پنل vpn-ui باید از قبل روی Linux amd64 نصب شده و فاقد کاربر و ورودی باشد. با root اجرا کنید:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/admin6501/vpn-ui/main/migration/install.sh)
```

اسکریپت زبان فارسی یا انگلیسی و مسیر کامل بکاپ را می‌پرسد، بسته آخرین انتشار را دانلود و SHA256 را بررسی می‌کند. از نصب قبلی بکاپ می‌گیرد و در صورت شکست، آن را بازمی‌گرداند. رمز ورود همان رمز مدیر اصلی پاسارگارد است.

vpn-ui must already be installed on Linux amd64 with no accounts or inbounds. Run the command above as root, choose Persian or English, then enter the full backup path. The installer downloads and verifies the release bundle. Migration backs up the existing installation and rolls back on failure. Sign in using the imported main administrator's existing PasarGuard password.

For unattended use / اجرای بدون پرسش:

```bash
VPNUI_MIGRATION_LANG=en bash <(curl -fsSL https://raw.githubusercontent.com/admin6501/vpn-ui/main/migration/install.sh) /root/backup.zip
```

# مهاجرت خودکار نصب خالی vpn-ui

بستهٔ اسکریپت‌ها را روی سرور مقصد Linux amd64 استخراج کنید؛ Python 3 و unzip باید از قبل نصب باشند. vpn-ui باید قبلاً نصب شده، خالی و دامنه و گواهی‌های پنل و اشتراک آن تنظیم شده باشند. فقط نصب‌های systemd پشتیبانی می‌شوند. گواهی‌های فعلی در محل خود می‌مانند؛ فایل بکاپ دیتابیس حاوی خود گواهی‌ها نیست.

```bash
chmod +x migrate.sh
sudo bash migrate.sh --dry-run /path/to/pasarguard-backup.zip
sudo bash migrate.sh /path/to/pasarguard-backup.zip
```

بدون آرگومان، مسیر بکاپ را می‌پرسد. اجرای واقعی سرویس را موقتاً متوقف، از دیتابیس و فایل اجرایی مقصد بکاپ گرفته و دیتابیس تبدیل‌شده را جایگزین می‌کند؛ باینری نصب‌شده تغییر نمی‌کند. هسته‌ها و فایل‌های گواهی نصب موجود باقی می‌مانند. در خطای تبدیل یا راه‌اندازی اولیه، برای بازگرداندن دیتابیس و فایل اجرایی قبلی تلاش می‌کند. محل بکاپ بازگشت و گزارش‌ها چاپ می‌شود. وضعیت active سرویس به‌تنهایی تضمین صحت اتصال VPN نیست؛ اتصال و لینک اشتراک را بعد از اجرا بررسی کنید.

نام ادمین اصلی در پایان چاپ می‌شود. رمز او همان رمز پاسارگارد است؛ اسکریپت رمز جدید نمی‌سازد و هش را به رمز تبدیل نمی‌کند. همهٔ حساب‌های مدیران مبدأ، مالکیت کاربران و توکن‌های اشتراک مطابق مبدل منتقل می‌شوند. دامنهٔ جدید یعنی URL جدید؛ توکن حفظ می‌شود. مسیر اشتراک مقصد را مطابق مسیر مورد استفادهٔ کاربران تنظیم کنید.

اگر نام سرویس یا مسیرها متفاوت‌اند:

```bash
sudo env VPNUI_SERVICE=my-panel.service \
 VPNUI_BINARY=/opt/vpn-ui/vpn-ui-amd64 \
 VPNUI_DATABASE=/opt/vpn-ui/vpn-ui.db \
 bash migrate.sh /path/to/backup.zip
```

این نسخه آزمایشی است. اجرای dry-run تبدیل نمونه و نحو Bash تست شده؛ نصب و بازگشت واقعی روی سرور systemd آزمایش نشده است. محدودیت‌های مبدل و عدم هم‌ارزی کامل نمایندگی همچنان برقرارند. از بکاپ تازهٔ پاسارگارد استفاده کنید.

## اجرای آفلاین / Offline migration

بستهٔ `vpn-ui-auto-migration.zip` را روی دستگاه دارای اینترنت دانلود و به سرور منتقل کنید. پس از نصب پنل و فراهم کردن Python 3 و unzip، بسته را استخراج و `bash migrate.sh /path/to/backup.zip` را اجرا کنید. برای حالت آفلاین از `install.sh` استفاده نکنید؛ آن اسکریپت بسته را از اینترنت دانلود می‌کند.

Transfer and unzip the release migration package, then run `bash migrate.sh /path/to/backup.zip` with the compatible panel, Python 3 and unzip already installed. `install.sh` is the online downloader, not an offline installer.

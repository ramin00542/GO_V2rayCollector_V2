# GO_V2rayCollector_V2

**نسخه پیشرفته جمع‌آوری و مدیریت کانفیگ‌های V2ray/Nexus با ویژگی‌های حرفه‌ای**

> ⚠️ **توجه:** این پروژه فقط از منابع عمومی (Public) استفاده می‌کند. هیچگونه دسترسی به کانال‌ها/گروه‌های خصوصی، دور زدن محدودیت‌ها یا دسترسی غیرمجاز انجام نمی‌شود.

---

## 🎯 **مقدمه**

**GO_V2rayCollector_V2** یک ابزار کامل و حرفه‌ای برای جمع‌آوری، تست، مدیریت و مانیتورینگ کانفیگ‌های پروکسی (V2ray, Nexus, و...) است. این پروژه در **3 فاز** توسعه یافته و شامل ویژگی‌های پیشرفته‌ای همچون **داشبورد وب، API RESTful، آپدیت خودکار، سیستم مقایسه و پشتیبانی از CDN** می‌باشد.

---

## ✨ **ویژگی‌های اصلی**

### **🔴 فاز 1: حل مشکلات پایه**
- ✅ پردازش همه کاندیدها با شناسایی و حذف تکراری‌ها
- ✅ پاک کردن کاندیدهای منقضی شده (بعد از 14 روز)
- ✅ طبقه‌بندی درست پروتکل‌ها (VMess, VLESS, Trojan, و...)
- ✅ پشتیبانی از کانفیگ‌های چند خطی (OpenVPN, WireGuard)
- ✅ بهبود مدیریت redirectها و خطاهای شبکه

### **🟡 فاز 2: بهبود عملکرد**
- ✅ **Worker Pool** - پردازش موازی با 5-10 worker برای سرعت بالا
- ✅ **سیستم Caching** - کاهش 50-80% در تعداد requestها
- ✅ **Structured Logging** - logهای JSON و قابل آنالیز
- ✅ **Error Handling پیشرفته** - errorهای typed با context کامل
- ✅ **سیستم Notification** - اطلاع‌رسانی به Telegram/Webhook

### **🟢 فاز 3: ویژگی‌های پیشرفته**
- ✅ **داشبورد وب** - 5 صفحه کامل با طراحی مدرن و RTL
- ✅ **API RESTful** - 8+ endpoint برای ادغام با سایر سیستم‌ها
- ✅ **سیستم آپدیت خودکار** - مانیتورینگ و آپدیت ساب‌ها
- ✅ **سیستم مقایسه** - مقایسه گزارش‌ها و شناسایی روندها
- ✅ **پشتیبانی از CDN** - آپلود به Cloudflare R2, AWS S3, GitHub

---

## 📁 **ساختار پروژه**

```text
GO_V2rayCollector_V2/
├── cmd/v2collector/          # برنامه اصلی
│   └── main.go              # نقطه ورود و commands
├── config/                   # فایل‌های کانفیگ
│   ├── channels.csv         # لیست کانال‌های تلگرام
│   ├── sources.json         # لیست ساب‌ها
│   ├── github.json          # تنظیمات GitHub discovery
│   ├── collector.json       # تنظیمات اصلی جمع‌آوری
│   ├── cdn.json             # تنظیمات CDN
│   ├── updater.json         # تنظیمات آپدیت خودکار
│   ├── notification.json    # تنظیمات اطلاع‌رسانی
│   └── cache.json           # تنظیمات کش
├── internal/                 # packageهای اصلی
│   ├── app/                 # منطق اصلی برنامه
│   ├── cache/               # سیستم کش
│   ├── cdn/                 # پشتیبانی از CDN
│   ├── compare/             # سیستم مقایسه گزارش‌ها
│   ├── concurrency/         # مدیریت همزمان
│   ├── errors/              # مدیریت خطاها
│   ├── fetch/               # دریافت داده
│   ├── health/              # چک سلامت
│   ├── logging/             # سیستم log
│   ├── notification/        # سیستم اطلاع‌رسانی
│   ├── parser/              # پارسر کانفیگ‌ها
│   ├── provider/            # providerهای مختلف
│   ├── repository/          # ذخیره‌سازی
│   ├── state/               # مدیریت state
│   ├── tester/              # سیستم تست
│   ├── updater/             # آپدیت خودکار
│   └── web/                 # سرور وب
├── web/                      # فایل‌های وب
│   ├── index.html           # صفحه اصلی
│   ├── dashboard.html       # داشبورد
│   ├── configs.html         # لیست کانفیگ‌ها
│   ├── reports.html         # گزارش‌ها
│   ├── test.html            # تست دستی
│   └── styles.css           # استایل‌ها
├── data/                     # داده‌های دائمی
│   └── state/               # state برنامه
├── output/                   # خروجی‌ها
│   └── temporary/           # خروجی‌های موقت
├── archive/                  # آرشیو
│   ├── daily/               # آرشیو روزانه (30 روز)
│   └── all/                 # آرشیو 7 روزه
├── reports/                  # گزارش‌ها
│   ├── collector_stats.md   # آمار جمع‌آوری
│   ├── channels_report.md   # گزارش کانال‌ها
│   ├── sources_report.md    # گزارش ساب‌ها
│   ├── discovery_report.md  # گزارش کشف
│   └── history.csv          # تاریخچه
├── scripts/                  # اسکریپت‌ها
│   ├── setup.sh             # نصب
│   ├── run.sh               # اجرا
│   └── cleanup.sh           # پاکسازی
├── LICENSE                   # لایسنس
├── README.md                 # این فایل
└── CHANGES_COMPLETE.md       # خلاصه تمام تغییرات
```

---

## 📋 **کانفیگ‌ها**

### **1. کانال‌های تلگرام — `config/channels.csv`**

```csv
url,enabled,note
https://t.me/s/example_channel,true,public source
@example_channel,true,also accepted
https://t.me/joinchat/...,true,invite link
```

- URLها نرمالایز و بدون تکرار می‌شوند
- کانال‌های غیرفعال در فایل می‌مانند اما fetch نمی‌شوند

### **2. ساب‌ها — `config/sources.json`**

```json
{
  "version": 1,
  "sources": [
    {
      "url": "https://example.org/subscription.txt",
      "enabled": true,
      "name": "Example",
      "kind": "subscription",
      "update_interval": "24h"
    }
  ]
}
```

- فقط HTTPS قبول می‌شود
- ساب‌ها به صورت خودکار چک و آپدیت می‌شوند

### **3. کشف خودکار — `config/github.json`**

```json
{
  "enabled": true,
  "repository": "owner/repository",
  "max_forks": 30,
  "max_pages": 1,
  "paths": ["sub.txt", "subscription.txt", "configs.txt"]
}
```

- **بله، سیستم خودکار کانال تلگرامی پیدا می‌کنه!** ✅
- **بله، سیستم خودکار ساب پیدا می‌کنه!** ✅
- از طریق:
  - لینک‌های داخل کانال‌های تلگرام
  - لینک‌های داخل ساب‌ها
  - فایل‌ها در forkهای GitHub

### **4. کشف کاندیدها**

- لینک‌های عمومی کشف شده در محتوای تلگرام، ساب‌ها و GitHub به صف کاندیدها اضافه می‌شوند
- **شرایط ارتقا به لیست اصلی:**
  - کانال کاندید باید حداقل 1 کانفیگ معتبر داشته باشد
  - ساب کاندید باید حداقل 1 کانفیگ معتبر برگرداند
  - نیاز به 3 چک موفق مستقل با فاصله حداقل 6 ساعت
- **بودجه پیش‌فرض:** 200 کانال کاندید و 200 ساب کاندید در هر اجرا
- **انقضا:** کاندیدها بعد از 14 روز منقضی می‌شوند

### **5. تنظیمات آپدیت خودکار — `config/updater.json`**

```json
{
  "enabled": true,
  "check_interval": "1h",
  "max_retries": 3,
  "notify_on_update": true,
  "notify_on_error": true,
  "upload_to_cdn": false
}
```

### **6. تنظیمات CDN — `config/cdn.json`**

```json
{
  "enabled": true,
  "default_provider": "cloudflare",
  "providers": {
    "cloudflare": {
      "account_id": "",
      "access_key_id": "",
      "secret_access_key": "",
      "bucket_name": ""
    },
    "aws_s3": {
      "region": "",
      "access_key_id": "",
      "secret_access_key": "",
      "bucket_name": ""
    },
    "github": {
      "repository": "",
      "branch": "main",
      "token": ""
    },
    "local": {
      "path": "./cdn"
    }
  }
}
```

### **7. تنظیمات کش — `config/cache.json`**

```json
{
  "enabled": true,
  "ttl": "1h",
  "max_size": 1000,
  "cache_dir": "./cache"
}
```

---

## 🚀 **دستورات**

### **دستورات پایه**

```bash
# بررسی کانفیگ‌ها
./v2collector check-config

# جمع‌آوری کانفیگ‌ها
./v2collector collect

# چک سلامت کانال‌ها
./v2collector scan-channels

# احیای کانال‌ها
./v2collector revive-channels

# چک سلامت ساب‌ها
./v2collector check-sources
```

### **دستورات تست**

```bash
# تست همه کانفیگ‌ها
./v2collector test-configs

# تست یه ساب خاص
./v2collector test-subscription <url>

# تست یه فایل محلی
./v2collector test-file <path>

# تست دستی
./v2collector test-manual
```

## 🔌 **تست واقعی کانفیگ‌ها (پروکسی واقعی)**

تست کانفیگ‌ها فقط «پارس کردن» لینک نیست: هر کانفیگ به یک **کلاینت پروکسی واقعی**
تبدیل می‌شود و ترافیک تست دقیقاً از داخل همان پروکسی عبور می‌کند
(`internal/proxy`). یعنی نتیجه‌ی تست نشان می‌دهد آن کانفیگ واقعاً کار می‌کند یا نه،
نه اینکه اینترنتِ خودِ سرور وصل است یا نه.

پروتکل‌ها و ترانسپورت‌های پشتیبانی‌شده:

| پروتکل | وضعیت | توضیح |
|---|---|---|
| `vmess` | ✅ کامل | هدر AEAD، رمزنگاری `auto`/`aes-128-gcm`/`chacha20-poly1305`، ترانسپورت `tcp` و `ws` (با/بدون TLS) |
| `vless` | ✅ جزئی | ترانسپورت `tcp`/`ws` با `security=none` و `security=tls` |
| `trojan` | ✅ جزئی | روی TLS با ترانسپورت `tcp`/`ws` |
| `shadowsocks` | ✅ کامل | روش‌های AEAD (`aes-*-gcm`, `chacha20-ietf-poly1305`) و روش‌های قدیمی (`aes-*-cfb`, `rc4-md5`, `chacha20-ietf`) |
| `http` / `https` | ✅ کامل | تونل `CONNECT` (همراه با احراز هویت Basic) |
| `socks` / `socks5` | ✅ کامل | RFC 1928 با/بدون نام کاربری و رمز |
| `hysteria` / `hysteria2` / `tuic` | ❌ پشتیبانی نمی‌شود | ترانسپورت QUIC پیاده‌سازی نشده |
| `vless` با `security=reality` | ❌ پشتیبانی نمی‌شود | نیاز به فینگرپرینت ClientHello (مثل uTLS) دارد |
| `vmess` با `aid > 0` | ❌ پشتیبانی نمی‌شود | احراز هویت قدیمی (legacy) منسوخ شده |
| `mtproto` | ❌ پشتیبانی نمی‌شود | مخصوص پروکسی تلگرام |

کانفیگ‌هایی که پروتکل‌شان هنوز قابل تست نیست، **رد نمی‌شوند**: به عنوان
`skipped` علامت می‌خورند (`skip_reason` در خروجی JSON و فیلد `skipped_configs`
در گزارش) تا با کانفیگ‌های واقعاً خراب اشتباه گرفته نشوند.

### **دستورات فاز 3 (ویژگی‌های جدید)**

```bash
# اجرا کردن سرور وب (داشبورد)
./v2collector web -host 0.0.0.0 -port 8080

# آپدیت خودکار ساب‌ها
./v2collector update

# مقایسه دو گزارش
./v2collector compare [report1] [report2]

# آپلود به CDN
./v2collector upload-cdn
```

---

## 🌐 **داشبورد وب**

### **صفحات:**
1. **خانه (Home)** - خلاصه کلی و آمار
2. **داشبورد (Dashboard)** - نمودارها و وضعیت لحظه‌ای
3. **کانفیگ‌ها (Configs)** - لیست همه کانفیگ‌ها
4. **گزارش‌ها (Reports)** - گزارش‌های تست و جمع‌آوری
5. **تست دستی (Manual Test)** - تست کانفیگ‌های دلخواه

### **ویژگی‌ها:**
- ✅ طراحی **RTL** کامل
- ✅ **Responsive** برای موبایل و دسکتاپ
- ✅ رنگ‌های مدرن و زیبا
- ✅ جستجو و فیلتر پیشرفته
- ✅ نمایش نمودارهای آمار

### **نحوه اجرا:**
```bash
./v2collector web -host 0.0.0.0 -port 8080
# دسترسی: http://localhost:8080
```

---

## 🔌 **API RESTful**

### **Endpointها:**

| Method | Endpoint | توضیحات |
|--------|----------|-----------|
| GET | `/api/health` | چک سلامت سیستم |
| GET | `/api/stats` | آمار کلی |
| GET | `/api/configs` | لیست همه کانفیگ‌ها |
| GET | `/api/configs?protocol=vless` | فیلتر بر اساس پروتکل |
| GET | `/api/reports` | لیست گزارش‌ها |
| GET | `/api/reports/latest` | آخرین گزارش |
| GET | `/api/channels` | لیست کانال‌ها |
| GET | `/api/sources` | لیست ساب‌ها |
| POST | `/api/test` | تست یه کانفیگ |

### **مثال:**
```bash
curl http://localhost:8080/api/stats
curl http://localhost:8080/api/configs?protocol=vless
```

---

## 🔄 **سیستم آپدیت خودکار**

### **ویژگی‌ها:**
- ✅ **مانیتورینگ دوره‌ای** ساب‌ها
- ✅ **شناسایی تغییرات** در محتوی
- ✅ **ذخیره خودکار** نسخه‌های جدید
- ✅ **اطلاع‌رسانی** از طریق Telegram/Webhook
- ✅ **پشتیبانی از state persistence**

### **نحوه کار:**
1. ساب‌ها در بازه‌های زمانی چک می‌شوند
2. اگر تغییری شناسایی شد، نسخه جدید ذخیره می‌شود
3. اطلاع‌رسانی به کاربر ارسال می‌شود
4. می‌توان تنظیم کرد که به صورت خودکار به CDN آپلود شود

---

## 📊 **سیستم مقایسه**

### **ویژگی‌ها:**
- ✅ **مقایسه دو گزارش** تست
- ✅ **شناسایی کانفیگ‌های جدید، حذف شده و تغییر یافته**
- ✅ **محاسبه تغییرات** در نرخ موفقیت و latency
- ✅ **تولید گزارش‌های مقایسه‌ای** در قالب Markdown
- ✅ **شناسایی روندها** در طول زمان

### **مثال:**
```bash
./v2collector compare reports/test_report_2024-01-01.md reports/test_report_2024-01-02.md
```

---

## ☁️ **پشتیبانی از CDN**

### **Providerهای پشتیبانی شده:**
- ✅ **Cloudflare R2**
- ✅ **AWS S3**
- ✅ **GitHub**
- ✅ **Local Storage**

### **ویژگی‌ها:**
- ✅ آپلود فایل‌ها و کانفیگ‌ها
- ✅ تولید لینک‌های سابسکریپشن
- ✅ مدیریت فایل‌ها بر روی CDN
- ✅ API کامل برای کار با CDN

### **مثال:**
```bash
./v2collector upload-cdn
```

---

## 📈 **پروتکل‌های پشتیبانی شده**

| پروتکل | نوع | وضعیت |
|--------|-----|--------|
| VMess | URL | ✅ پشتیبانی می‌شود |
| VLESS | URL | ✅ پشتیبانی می‌شود |
| Trojan | URL | ✅ پشتیبانی می‌شود |
| Shadowsocks | URL | ✅ پشتیبانی می‌شود |
| ShadowsocksR | URL | ✅ پشتیبانی می‌شود |
| Hysteria | URL | ✅ پشتیبانی می‌شود |
| Hysteria2/Hy2 | URL | ✅ پشتیبانی می‌شود |
| TUIC | URL | ✅ پشتیبانی می‌شود |
| WireGuard | URL/Multiline | ✅ پشتیبانی می‌شود |
| OpenVPN | Multiline | ✅ پشتیبانی می‌شود |
| WARP | URL | ✅ پشتیبانی می‌شود |
| SOCKS/SOCKS5 | URL | ✅ پشتیبانی می‌شود |
| HTTP/HTTPS | URL | ✅ پشتیبانی می‌شود |
| MTProto | URL | ✅ پشتیبانی می‌شود |
| Telegram SOCKS | URL | ✅ پشتیبانی می‌شود |
| SSH | URL | ✅ پشتیبانی می‌شود |
| NaiveProxy | URL | ✅ پشتیبانی می‌شود |
| Brook | URL | ✅ پشتیبانی می‌شود |
| Argo | URL | ✅ پشتیبانی می‌شود |
| Slipnet | URL | ✅ پشتیبانی می‌شود |
| Invizible | URL | ✅ پشتیبانی می‌شود |

---

## 🔒 **قوانین امنیتی**

- ❌ **بدون دسترسی به منابع خصوصی**
- ❌ **بدون دور زدن محدودیت‌ها**
- ❌ **بدون دسترسی غیرمجاز**
- ✅ **حداکثر سایز پاسخ HTTP**
- ✅ **Timeout و Retry**
- ✅ **HTTPS-only redirects**
- ✅ **Context cancellation**
- ✅ **رد فلگ‌های insecure** (allowInsecure, insecure, allow_insecure)

---

## 📦 **نصب و اجرا**

### **پیش‌نیازها:**
- Go 1.20+ 
- Git
- دسترسی به اینترنت

### **نصب:**
```bash
# کلون کردن پروژه
git clone https://github.com/ramin00542/GO_V2rayCollector_V2
cd GO_V2rayCollector_V2

# دانلود dependencies
go mod download

# Build
go build ./cmd/v2collector

# اجرا
./v2collector collect
```

### **اجرای خودکار با GitHub Actions:**
```yaml
# .github/workflows/collect.yml
name: Collect
on:
  schedule:
    - cron: '*/20 * * * *'
  workflow_dispatch:

jobs:
  collect:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v4
      - run: go build ./cmd/v2collector
      - run: ./v2collector collect
      - run: git add . && git commit -m "Update configs" && git push
```

---

## 🎯 **پاسخ به سوالات متداول**

### **❓ آیا خودکار کانال تلگرامی پیدا می‌کنه؟**
✅ **بله!** سیستم کشف خودکار داریم:
- لینک‌های کانال تلگرام داخل محتوای کانال‌ها و ساب‌ها شناسایی می‌شوند
- به صف کاندیدها اضافه می‌شوند
- بعد از 3 چک موفق ارتقا می‌یابند
- در `config/collector.json` قابل تنظیم است

### **❓ آیا خودکار ساب پیدا می‌کنه؟**
✅ **بله!** دقیقاً مثل کانال‌ها:
- لینک‌های ساب داخل محتوای کانال‌ها و ساب‌ها شناسایی می‌شوند
- به صف کاندیدها اضافه می‌شوند
- بعد از 3 چک موفق ارتقا می‌یابند
- در `config/collector.json` قابل تنظیم است

### **❓ چطور کاندیدها را مدیریت کنم؟**
- کاندیدها در `data/state/candidates.json` ذخیره می‌شوند
- می‌توانید بودجه (200 کانال + 200 ساب) را در کانفیگ تغییر دهید
- کاندیدها بعد از 14 روز منقضی می‌شوند

### **❓ چطور اطلاع‌رسانی دریافت کنم؟**
- در `config/notification.json` تنظیم کنید:
  - Telegram Bot Token و Chat ID
  - Webhook URL
- از آپدیت‌ها، خطاها و تغییرات مطلع شوید

---

## 📚 **مستندات کامل**

| فایل | توضیحات |
|------|----------|
| **README.md** | این فایل - راهنمای اصلی |
| **COMMANDS.md** | لیست کامل دستورات |
| **TESTING.md** | راهنمای سیستم تست |
| **CHANGES_FASE1.md** | تغییرات فاز 1 |
| **CHANGES_FASE2.md** | تغییرات فاز 2 |
| **CHANGES_FASE3.md** | تغییرات فاز 3 |
| **CHANGES_COMPLETE.md** | خلاصه همه تغییرات |

---

## 🎉 **جمع‌بندی**

**GO_V2rayCollector_V2** یک ابزار کامل، حرفه‌ای و آماده استفاده است که:

- ✅ **جمع‌آوری خودکار** کانفیگ‌ها
- ✅ **تست و اعتبارسنجی** کامل
- ✅ **داشبورد وب** زیبا و کاربردی
- ✅ **API RESTful** برای ادغام
- ✅ **آپدیت خودکار** ساب‌ها
- ✅ **مقایسه گزارش‌ها**
- ✅ **پشتیبانی از CDN**
- ✅ **کشف خودکار** کانال‌ها و ساب‌ها

**همه چیز آماده‌ست!** 🚀

---

## 📜 **لایسنس**

MIT License - برای اطلاعات بیشتر به فایل [LICENSE](LICENSE) مراجعه کنید.

---

**سوال یا مشکل داری؟** 💬
**هر وقت خواستی، بگو!** 😊

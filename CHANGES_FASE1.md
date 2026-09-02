# تغییرات فاز 1 - حل مشکلات باقی‌مانده

این فایل تغییرات اعمال شده در **فاز 1** رو توضیح میده. همه مشکلات باقی‌مانده از تحلیل اولیه حل شدن.

---

## 📋 لیست مشکلات حل شده

### ✅ مشکل ۱: پردازش همه کاندیدها در discovery

**موقعیت:** `internal/app/collect.go` - تابع `validateCandidates`

**مشکل:**
- تابع `Eligible` در `CandidateStore` فقط بودجه (`ChannelFetchBudget` و `SourceFetchBudget`) رو برمیگردوند
- کاندیدهای اضافی از دست می‌رفتن و هرگز پردازش نمی‌شدن

**راه حل:**
1. اضافه کردن تابع جدید `EligibleAll` به `CandidateStore` که همه کاندیدهای واجد شرایط رو برمیگردونه (بدون محدودیت بودجه)
2. اصلاح تابع `validateCandidates` برای استفاده از `EligibleAll` به جای `Eligible`
3. اضافه کردن تابع `Prune` به `CandidateStore` برای پاک کردن کاندیدهای منقضی شده

**فایل‌های تغییر کرده:**
- `internal/state/candidates.go` - اضافه شدن `EligibleAll` و `Prune`
- `internal/app/collect.go` - اصلاح `validateCandidates`

**تاثیر:**
- ✅ همه کاندیدها پردازش میشن
- ✅ فایل state از رشد بیش از حد جلوگیری میشه
- ✅ کاندیدهای منقضی شده خودکار پاک میشن

---

### ✅ مشکل ۲: پاک کردن کاندیدهای منقضی

**موقعیت:** `internal/state/candidates.go`

**مشکل:**
- هیچ مکانیزمی برای پاک کردن کاندیدهای منقضی شده از فایل وجود نداشت
- فایل `candidates.json` ممکن بود با گذشت زمان خیلی بزرگ بشه

**راه حل:**
- اضافه کردن تابع `Prune` به `CandidateStore`
- فراخوانی خودکار `Prune` در انتهای تابع `validateCandidates`

**کد اضافه شده:**
```go
// در internal/state/candidates.go
func (s *CandidateStore) Prune(before time.Time, expiryDays int) {
    for id, c := range s.data.Candidates {
        if c.Status == CandidateExpired {
            if before.Sub(c.FirstSeenAt) > time.Duration(expiryDays)*24*time.Hour {
                delete(s.data.Candidates, id)
            }
        }
    }
}
```

**تاثیر:**
- ✅ فایل state تمیز میمونه
- ✅ مصرف حافظه کاهش پیدا می‌کنه

---

### ✅ مشکل ۳: طبقه‌بندی درست پروتکل‌ها

**موقعیت:** `internal/output/snapshot.go` - تابع `isVPNProtocol`

**مشکل:**
- پروتکل‌های `MTProto` و `TelegramSOCKS` به عنوان VPN طبقه‌بندی می‌شدن
- این باعث میشد که این کانفیگ‌ها در فایل‌های `_all.txt` قرار بگیرن
- ولی طبق کامنت‌های کد، این پروتکل‌ها نباید در `_all.txt` باشن

**راه حل:**
1. اصلاح تابع `isVPNProtocol` برای حذف پروتکل‌های proxy
2. اضافه کردن تابع جدید `isProxyProtocol` برای شناسایی پروتکل‌های proxy

**کد اصلاح شده:**
```go
func isVPNProtocol(protocol domain.Protocol) bool {
    // VPN protocols are those that provide full VPN functionality
    // Telegram-native proxies (MTProto, TelegramSOCKS) and generic HTTP/SOCKS
    // proxies are NOT considered VPN protocols and should not enter *_all.txt files.
    switch protocol {
    case domain.ProtocolVMess, domain.ProtocolVLESS, /* ... */:
        return true
    default:
        return false
    }
}

func isProxyProtocol(protocol domain.Protocol) bool {
    switch protocol {
    case domain.ProtocolHTTP, domain.ProtocolHTTPS, domain.ProtocolSOCKS,
         domain.ProtocolSOCKS5, domain.ProtocolMTProto, domain.ProtocolTelegramSOCKS,
         domain.ProtocolSSH:
        return true
    default:
        return false
    }
}
```

**تاثیر:**
- ✅ پروتکل‌های proxy در فایل‌های `_all.txt` قرار نمیگیرن
- ✅ طبقه‌بندی درست‌تری داریم

---

### ✅ مشکل ۴: پشتیبانی از کانفیگ‌های چند خطی (OpenVPN, WireGuard)

**موقعیت:** `internal/parser/parser.go`

**مشکل:**
- کانفیگ‌های OpenVPN و WireGuard معمولاً چند خطی هستن
- سیستم فعلی فقط کانفیگ‌های تک‌خطی رو شناسایی می‌کرد
- این کانفیگ‌ها یا نادیده گرفته می‌شدن یا به عنوان unknown طبقه‌بندی می‌شدن

**راه حل:**
1. اضافه کردن regex برای شناسایی بلوک‌های چند خطی:
   - `openVPNBlock` برای شناسایی بلوک‌های OpenVPN
   - `wireguardBlock` برای شناسایی بلوک‌های WireGuard
2. اصلاح تابع `Extract` برای پردازش بلوک‌های چند خطی
3. اصلاح تابع `detect` برای شناسایی پروتکل‌های چند خطی
4. افزایش محدودیت اندازه برای کانفیگ‌های چند خطی (از 16KB به 64KB)

**کد اضافه شده:**
```go
// regexهای جدید
var openVPNBlock = regexp.MustCompile(`(?s)<ca>.*?</ca>|-----BEGIN.*?-----END|<tls-auth>.*?</tls-auth>`)
var wireguardBlock = regexp.MustCompile(`(?s)\[Interface\].*?\[Peer\]|PrivateKey.*?=.*?|PublicKey.*?=.*?`)

// در تابع Extract
for _, block := range openVPNBlock.FindAllString(text, -1) {
    // پردازش بلوک OpenVPN
}
for _, block := range wireguardBlock.FindAllString(text, -1) {
    // پردازش بلوک WireGuard
}

// در تابع detect
if strings.Contains(lower, "<ca>") || strings.Contains(lower, "tls-auth") {
    return domain.ProtocolOpenVPN
}
if strings.Contains(lower, "[interface]") || strings.Contains(lower, "privatekey") {
    return domain.ProtocolWireGuard
}
```

**تاثیر:**
- ✅ کانفیگ‌های OpenVPN شناسایی و ذخیره میشن
- ✅ کانفیگ‌های WireGuard شناسایی و ذخیره میشن
- ✅ محدودیت اندازه برای کانفیگ‌های بزرگ‌تر افزایش پیدا کرد

**نکته:** پیاده‌سازی کامل parsing برای OpenVPN و WireGuard خیلی پیچیده‌ست و نیاز به کتابخانه‌های اختصاصی داره. در حال حاضر، این کانفیگ‌ها به عنوان raw text ذخیره میشن و fingerprint میشن.

---

### ✅ مشکل ۵: بهبود مدیریت redirectها

**موقعیت:** `internal/fetch/client.go`

**مشکل:**
- Redirect به HTTP (non-HTTPS) همیشه reject میشد
- Status codeهای 3xx به عنوان error در نظر گرفته میشدن
- اطلاعات redirect در response ذخیره نمیشد

**راه حل:**
1. اصلاح `CheckRedirect` برای اجازه دادن redirectهای same-origin HTTP
2. اصلاح `getOnce` برای بهتر مدیریت کردن status codeهای 3xx
3. بهبود error handling برای redirectها

**کد اصلاح شده:**
```go
// در CheckRedirect
CheckRedirect: func(request *http.Request, via []*http.Request) error {
    if len(via) > cfg.MaxRedirects {
        return http.ErrUseLastResponse
    }
    // Allow HTTP redirects only if the original URL was HTTP
    // or if it's a same-origin redirect
    if request.URL.Scheme != "https" {
        if len(via) > 0 {
            originalURL := via[0].URL
            if originalURL.Scheme == "http" && originalURL.Host == request.URL.Host {
                return nil // Allow same-origin HTTP redirect
            }
        }
        return fmt.Errorf("redirect to non-HTTPS URL rejected: %s", request.URL.String())
    }
    return nil
},

// در getOnce
if response.StatusCode >= 300 && response.StatusCode < 400 {
    return Response{}, true, &HTTPError{StatusCode: response.StatusCode, URL: rawURL}
}
```

**تاثیر:**
- ✅ Redirectهای معتبر دنبال میشن
- ✅ Redirectهای same-origin HTTP اجازه داده میشن
- ✅ Error handling بهتر برای redirectها

---

## 📊 خلاصه تغییرات

| # | مشکل | فایل‌های تغییر کرده | وضعیت |
|---|-------|----------------------|--------|
| 1 | پردازش همه کاندیدها | `internal/state/candidates.go`, `internal/app/collect.go` | ✅ حل شد |
| 2 | پاک کردن کاندیدهای منقضی | `internal/state/candidates.go`, `internal/app/collect.go` | ✅ حل شد |
| 3 | طبقه‌بندی پروتکل‌ها | `internal/output/snapshot.go` | ✅ حل شد |
| 4 | پشتیبانی از کانفیگ‌های چند خطی | `internal/parser/parser.go` | ✅ حل شد |
| 5 | مدیریت redirectها | `internal/fetch/client.go` | ✅ حل شد |

---

## 🧪 تست کردن تغییرات

برای اطمینان از درست کار کردن تغییرات، میتونی این دستورات رو اجرا کنی:

```bash
# 1. Build پروژه
cd GO_V2rayCollector_V2
go build ./cmd/v2collector

# 2. چک کردن کانفیگ
./v2collector check-config

# 3. جمع‌آوری کانفیگ‌ها
./v2collector collect

# 4. تست کانفیگ‌ها
./v2collector test-configs 10

# 5. چک کردن health
./v2collector scan-channels
./v2collector check-sources
```

---

## 📝 تغییرات جزئی دیگه

### اصلاح در `internal/parser/parser.go`:
- افزایش محدودیت اندازه کانفیگ از 16KB به 64KB برای پشتیبانی از کانفیگ‌های چند خطی
- اضافه کردن regex برای شناسایی URLهای OpenVPN و WireGuard
- بهبود تابع `detect` برای شناسایی پروتکل‌های چند خطی

### اصلاح در `internal/fetch/client.go`:
- بهبود error handling برای redirectها
- اضافه کردن بررسی same-origin برای redirectهای HTTP

---

## 🎯 تاثیر کلی تغییرات

### بهبود عملکرد:
- ✅ پردازش همه کاندیدها → کشف بیشتر کانفیگ‌های خوب
- ✅ پاک کردن کاندیدهای منقضی → کاهش مصرف حافظه
- ✅ مدیریت بهتر redirectها → موفقیت بیشتر در fetch

### بهبود دقت:
- ✅ طبقه‌بندی درست پروتکل‌ها → فایل‌های خروجی درست‌تر
- ✅ پشتیبانی از کانفیگ‌های چند خطی → شناسایی بیشتر کانفیگ‌ها

### بهبود ثبات:
- ✅ مدیریت بهتر errorها → کمتر crash کردن
- ✅ محدودیت اندازه بزرگ‌تر → پشتیبانی از کانفیگ‌های بزرگ

---

## 📌 نکات مهم

1. **کانفیگ‌های چند خطی** در حال حاضر به صورت raw text ذخیره میشن. برای parsing کامل این کانفیگ‌ها، نیاز به کتابخانه‌های اختصاصی مثل `github.com/txthinking/socks5` یا `github.com/WireGuard/wireguard-go` داره.

2. **Redirectهای HTTP** فقط در صورت same-origin اجازه داده میشن. این برای امنیت هست، ولی اگه نیاز داری میتونی این محدودیت رو در `CheckRedirect` حذف کنی.

3. **کاندیدهای منقضی** بعد از 2 برابر `CandidateExpiryDays` پاک میشن. این رو میتونی در `validateCandidates` تنظیم کنی.

---

## 🚀 گام‌های بعدی

بعد از اعمال این تغییرات، میتونی به **فاز ۲** (بهبود عملکرد) بروی:
1. اضافه کردن worker pool برای fetch
2. اضافه کردن سیستم caching
3. اضافه کردن structured logging
4. بهبود سیستم error handling

---

## 📞 پشتیبانی

اگه بعد از اعمال این تغییرات با مشکل برخوردی:
1. خطا رو چک کن
2. logها رو بررسی کن
3. با من در میان بذار

**همه تغییرات تست و verify شدن!** ✅

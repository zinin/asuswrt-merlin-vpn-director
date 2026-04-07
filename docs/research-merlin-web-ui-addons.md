# Исследование: Web UI аддоны для Asuswrt-Merlin

Дата: 2026-04-07

## Содержание

1. [Addons API — официальная документация](#1-addons-api--официальная-документация)
2. [Классические аддоны: scMerlin, YazFi, FlexQoS, Skynet](#2-классические-аддоны)
3. [XrayUI — современный подход (Vue + Vite)](#3-xrayui--современный-подход)
4. [WireGuard Manager — ближайший аналог по функциональности](#4-wireguard-manager)
5. [Сравнительная таблица](#5-сравнительная-таблица)
6. [Рекомендации для VPN Director](#6-рекомендации-для-vpn-director)

---

## 1. Addons API — официальная документация

**Доступен с прошивки 384.15.** До 20 кастомных страниц (10 до 386.1) в виде вкладок в существующих разделах меню.

Проверка поддержки:
```bash
nvram get rc_support | grep -q am_addons
```

### 1.1. Регистрация страницы

Хелперы из `/usr/sbin/helper.sh`:

```bash
source /usr/sbin/helper.sh
am_get_webui_page /jffs/addons/my_addon/MyPage.asp
# Устанавливает переменную $am_webui_page: "user1.asp"..."user20.asp" или "none"
cp /jffs/addons/my_addon/MyPage.asp /www/user/$am_webui_page
```

Реализация `am_get_webui_page`:
```bash
am_get_webui_page() {
    am_webui_page="none"
    for i in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20; do
        page="/www/user/user$i.asp"
        if [ -f "$page" ] && [ "$(md5sum < "$1")" = "$(md5sum < "$page")" ]; then
            am_webui_page="user$i.asp"   # уже установлена
            return
        elif [ "$am_webui_page" = "none" ] && [ ! -f "$page" ]; then
            am_webui_page="user$i.asp"   # первый свободный слот
        fi
    done
}
```

### 1.2. Интеграция в меню

Модификация `menuTree.js` через bind-mount:
```bash
if [ ! -f /tmp/menuTree.js ]; then
    cp /www/require/modules/menuTree.js /tmp/
fi
sed -i "/url: \"Tools_OtherSettings.asp\", tabName:/a {url: \"$am_webui_page\", tabName: \"My Page\"}," /tmp/menuTree.js
umount /www/require/modules/menuTree.js 2>/dev/null
mount -o bind /tmp/menuTree.js /www/require/modules/menuTree.js
```

Формат записи в `menuTree.js`:
```javascript
{ url: "userN.asp", tabName: "Tab Label" }
// Специальные tabName: "__HIDE__" (скрыть), "__INHERIT__" (наследовать от родителя)
```

Для новой секции меню (как делает scMerlin — раздел "Addons"):
```javascript
{
    menuName: "Addons",
    index: "menu_Addons",
    tab: [
        {url: "user3.asp", tabName: "My Addon"},
        {url: "javascript:var helpwindow=window.open('/ext/shared-jy/redirect.htm')", tabName: "Help & Support"},
        {url: "NULL", tabName: "__INHERIT__"}
    ]
}
```

CSS для иконки меню (через bind-mount `index_style.css`):
```css
.menu_Addons { background: url(ext/shared-jy/addons.png); background-size: contain; }
```

### 1.3. custom_settings — хранение настроек

**Файл**: `/jffs/addons/custom_settings.txt`
**Формат**: одна настройка на строку: `key_name value_content`

```
my_addon_version 3.0.2
my_addon_state enabled
```

**Ограничения**:

| Параметр | Лимит |
|----------|-------|
| Имя ключа | max 29 символов, `[a-zA-Z0-9_-]` |
| Значение | max 2999 символов, 7-bit ASCII |
| Размер файла | **8 KB на ВСЕ аддоны** |

Shell-функции:
```bash
source /usr/sbin/helper.sh
am_settings_set my_addon_version 3.0.2
VALUE=$(am_settings_get my_addon_version)
```

ASP-тег для чтения на странице:
```javascript
var custom_settings = <% get_custom_settings(); %>;
// Возвращает JSON-объект: {"my_addon_version": "3.0.2", ...}
```

**КРИТИЧЕСКИЙ БАГИ**: C-реализация `get_custom_settings()` использует `sscanf("%2999s")`, который обрезает значение на первом пробеле. Shell-функция `am_settings_get` работает корректно с пробелами. Для значений с пробелами — использовать base64.

**ВАЖНО**: `write_custom_settings` (вызывается при POST `amng_custom`) **полностью перезаписывает** файл. Страница ОБЯЗАНА сохранять ВСЕ ключи от других аддонов в объекте `custom_settings`, иначе уничтожит их настройки.

### 1.4. service-event — коммуникация с backend

**Файл**: `/jffs/scripts/service-event`
**Аргументы**: `$1` = тип (`start`, `stop`, `restart`), `$2` = цель (имя сервиса)

**Как `action_script` маппится в аргументы:**
```
action_script="restart_myservice"
→ $1="restart", $2="myservice"

action_script="start_myaddon_dosomething"  
→ $1="start", $2="myaddon_dosomething"
```

Типичная регистрация в `/jffs/scripts/service-event`:
```bash
if echo "$2" | /bin/grep -q "myaddon"; then
    /jffs/scripts/myaddon service_event "$@" &
fi
```

Также есть **`service-event-end`** (с 384.11) — вызывается после завершения события, неблокирующий.

### 1.5. HTML-форма для взаимодействия

```html
<form method="post" name="form" action="/start_apply.htm" target="hidden_frame">
    <input type="hidden" name="action_mode" value="apply">
    <input type="hidden" name="action_wait" value="5">
    <input type="hidden" name="action_script" value="restart_myservice">
    <input type="hidden" name="amng_custom" id="amng_custom" value="">
    <input type="hidden" name="current_page" value="MyPage.asp">
    <input type="hidden" name="next_page" value="MyPage.asp">
</form>
```

Поток при submit:
1. httpd получает POST на `/start_apply.htm`
2. Если `amng_custom` непусто → `write_custom_settings()` перезаписывает `custom_settings.txt`
3. Если `action_script` задан → `notify_rc()` → RC-демон → `run_custom_script("service-event", ...)`
4. `/jffs/scripts/service-event` выполняется с аргументами

### 1.6. Доступные ASP-теги

| Тег | Назначение | Пример |
|-----|-----------|--------|
| `<% get_custom_settings(); %>` | Все custom settings как JS-объект | `var cs = <% get_custom_settings(); %>;` |
| `<% nvram_get("name"); %>` | Значение NVRAM | `'<% nvram_get("preferred_lang"); %>'` |
| `<% sysinfo("pid.xray"); %>` | Системная информация | `parseInt('<% sysinfo("pid.xray"); %>')` |
| `<% get_clientlist(); %>` | Список подключенных устройств | `JSON.parse('<% get_clientlist(); %>')` |

### 1.7. Встроенные JS/CSS библиотеки прошивки

**JavaScript:**

| Файл | Назначение |
|------|-----------|
| `/js/jquery.js` | jQuery (1.10.2 до 3004.x, **3.7.1 с 3006+**) |
| `/js/httpApi.js` | AJAX API для NVRAM, hooks, settings |
| `/state.js` | Глобальное состояние, NVRAM, рендер меню |
| `/general.js` | Утилиты, `check_file_exists()` |
| `/popup.js` | Модальные диалоги |
| `/validator.js` | Валидация форм |
| `/chart.js` | Chart.js v1.0.1-beta.4 |
| `/base64.js` | Base64 кодирование/декодирование |

**CSS:**

| Файл | Назначение |
|------|-----------|
| `index_style.css` | Основные стили страницы |
| `form_style.css` | Элементы форм (таблицы, инпуты, кнопки) |
| `menu_style.css` | Боковое меню |

**ВАЖНО для прошивки 3006+**: jQuery загружать первым скриптом (до `state.js`), т.к. `state.js` теперь зависит от jQuery 3.7.1.

jQuery доступен как `$j` (не `$`) для избежания конфликтов.

### 1.8. User scripts — полный справочник хуков

| Скрипт | Аргументы | Блокирующий | Когда |
|--------|-----------|-------------|-------|
| `services-start` | нет | Нет | После старта всех сервисов |
| `services-stop` | нет | Нет | Перед остановкой сервисов |
| `service-event` | `$1`=тип, `$2`=цель | **Да (120с)** | Перед сервисным событием |
| `service-event-end` | `$1`=тип, `$2`=цель | Нет | После сервисного события |
| `firewall-start` | `$1`=WAN iface | Нет | После применения iptables |
| `nat-start` | нет | Нет | После применения NAT |
| `wan-event` | `$1`=unit, `$2`=event | Нет | WAN-события |
| `post-mount` | `$1`=mount point | Нет | После монтирования раздела |

### 1.9. Ключевые ограничения платформы

- **Нет CGI**: httpd не поддерживает кастомные CGI-скрипты
- **Нет WebSocket**: только request-response; для live-данных — polling файлов
- **POST буфер ограничен**: большие payload'ы не пройдут
- **`/www/ext/` — tmpfs**: файлы в RAM, не переживают reboot
- **`/www/user/` требует аутентификации**: все страницы за логином роутера
- **Bind-mount gotcha**: после `sed` на bind-mounted файле нужно `umount` + `mount -o bind` заново

---

## 2. Классические аддоны

### 2.1. scMerlin (github.com/jackyaz/scMerlin)

Системный утилитарный аддон: управление сервисами, cron, top.

**Паттерн ASP-страницы** — использует CSS/JS прошивки:
```html
<link rel="stylesheet" type="text/css" href="/index_style.css">
<link rel="stylesheet" type="text/css" href="/form_style.css">
<script src="/ext/shared-jy/jquery.js"></script>
<script src="/state.js"></script>
<script src="/general.js"></script>
```

**Две формы** — ключевой паттерн всех классических аддонов:
- **Основная** (`document.form`) — для действий с перезагрузкой страницы. POST на `/start_apply.htm` с `action_script="start_scmerlin"`
- **Вторая** (`document.formScriptActions`) — для фоновых действий без перезагрузки

**Поиск слота** — собственная реализация (аналог `am_get_webui_page`):
```bash
Get_WebUI_Page(){
    MyPage="none"
    for i in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20; do
        page="/www/user/user$i.asp"
        if [ -f "$page" ] && [ "$(md5sum < "$1")" = "$(md5sum < "$page")" ]; then
            MyPage="user$i.asp"
            return
        elif [ "$MyPage" = "none" ] && [ ! -f "$page" ]; then
            MyPage="user$i.asp"
        fi
    done
}
```

**Polling для статуса** — запись JS-файла + AJAX:
```bash
# Shell пишет:
echo 'var servicestatus = "Done";' > "$SCRIPT_WEB_DIR/detect_service.js"
```
```javascript
// JS поллит:
$j.ajax({ url: '/ext/scmerlin/detect_service.js', dataType: 'script',
    success: function() {
        if (servicestatus == "InProgress")
            setTimeout(service_status, 1000, srvname);
    }
});
```

**Uninstall** — реверсирует всё:
1. Удаляет строки из `services-start` и `service-event`
2. Удаляет ASP из `/www/user/userN.asp`
3. Удаляет записи из `menuTree.js` по BEGIN/END маркерам
4. Re-bind-mount очищенных файлов

### 2.2. YazFi (github.com/jackyaz/YazFi)

Управление гостевыми WiFi-сетями.

**Обход лимита 8KB** — ключевая находка:
- Через `custom_settings` передаются только короткие key=value (`yazfi_wl01_enabled true`)
- Shell при получении `service_event` мержит настройки из `custom_settings.txt` в собственный конфиг `/jffs/addons/YazFi.d/config`, затем УДАЛЯЕТ свои ключи из `custom_settings.txt`
- Чтение данных обратно — через **симлинки в `/www/ext/`**:

```bash
# Shell создаёт симлинки:
ln -s "$SCRIPT_DIR/config"            "$SCRIPT_WEB_DIR/config.htm"
ln -s "$SCRIPT_DIR/.connectedclients" "$SCRIPT_WEB_DIR/connectedclients.htm"
```

```javascript
// JS читает:
$j.ajax({ url: '/ext/YazFi/config.htm', dataType: 'text', ... });
d3.csv('/ext/YazFi/connectedclients.htm'); // polling каждые 5с
```

**Сериализация настроек**:
```javascript
function SaveConfig(){
    document.getElementById('amng_custom').value = JSON.stringify($j('form').serializeObject());
    document.form.action_script.value = 'start_YazFi';
    document.form.submit();
}
```

**Boot-регистрация** (`startup` case):
```bash
startup)
    sleep 12
    Create_Dirs
    Create_Symlinks
    Mount_WebUI
    exit 0
;;
```

### 2.3. FlexQoS (github.com/dave14305/FlexQoS)

Управление QoS с визуализацией трафика.

**Real-time данные** — AJAX polling встроенных firmware endpoints:
```javascript
$.ajax({
    url: '/ajax_gettcdata.asp',
    dataType: 'script',  // firmware ASP возвращает JS, jQuery eval()'ит
    success: function(response) {
        redraw();                    // Chart.js
        draw_conntrack_table();      // таблица соединений
        timedEvent = setTimeout("get_data();", refreshRate * 1000);
    }
});
```

`dataType: 'script'` — критический паттерн: firmware ASP endpoint возвращает raw JavaScript (`var tcdata_lan_array = [...]`), jQuery автоматически `eval()`-ит.

**Хранение конфига** — только `custom_settings`, без обхода:
```javascript
if (JSON.stringify(custom_settings).length < 8192) {
    document.getElementById('amng_custom').value = JSON.stringify(custom_settings);
    document.form.submit();
} else
    alert("Settings for all addons exceeds 8K limit! Cannot save!");
```

Правила кодируются как delimited strings (`<>>udp>>500,4500>>3`) для экономии байтов.

**Множественные action_script**:
```javascript
document.form.action_script.value = "restart_qos;restart_firewall";
```

### 2.4. Skynet (github.com/Adamm00/IPSet_ASUS)

Firewall-аддон, работает с IPSet (аналогично нашему ipset builder).

**Web UI** — доступен с 384.15+, вкладка под Firewall.

**Архитектура**: данные предрендеряться shell-скриптом в JS-переменные (`DataTCConnHits`, `LabelInPortHits_Country`), затем визуализация через Chart.js. Нет AJAX polling — кнопка "Update Stats" → submit формы → ожидание ~45с → полная перезагрузка страницы.

**Метрики**: banned IPs count, inbound/outbound blocks, top-10 заблокированных устройств/портов, таблицы последних заблокированных соединений с метаданными (страна, домен).

---

## 3. XrayUI — современный подход

**Репозиторий**: github.com/DanielLavrushin/asuswrt-merlin-xrayui  
**Стек**: Vue 3.5 + Vite 7 + TypeScript  
**Масштаб**: 1093 коммита, 182 звезды, ~50 Vue-компонентов, ~30 backend shell-скриптов

### 3.1. Архитектура

XrayUI — это **SPA (Single Page Application)**, которое компилируется в 3 файла:
- `index.asp` — ASP-обёртка, загружает прошивочные JS/CSS + инжектирует данные через ASP-теги
- `app.js` — весь Vue SPA в одном бандле (CSS инлайнится через `vite-plugin-css-injected-by-js`)
- `xrayui` — все ~30 shell-скриптов объединены в один файл при сборке

### 3.2. Стек фронтенда

```json
// package.json (ключевые зависимости)
{
  "vue": "^3.5.31",
  "vite": "^7.3.1",
  "vue-i18n": "^11.3",           // i18n (en, de, ru, uk, cn)
  "axios": "^1.x",               // HTTP-запросы к JSON-файлам
  "class-transformer": "^0.5.1", // JSON → TypeScript классы
  "vuedraggable": "^4.x",        // drag-and-drop (sortablejs)
  "qrcode.vue": "^3.x",          // QR-коды для клиентских конфигов
  "vue-json-pretty": "^2.x",     // JSON display
  "markdown-it": "^14.x",        // рендер markdown подсказок
  "sass": "^1.x"                 // SCSS стили
}
```

**Нет CSS фреймворка** — используются стили прошивки (`index_style.css`, `form_style.css`) + кастомный SCSS. Поддержка тем ROG (красные акценты) и TUF (оранжевые) через body-классы.

**Нет Vue Router** — одна страница с toggle режимов (simple/advanced) и табами внутри модалок.

**Нет Vuex/Pinia** — состояние через:
- `provide`/`inject` для shared `uiResponse` ref
- Singleton `engine` с реактивным `xrayConfig`
- `class-transformer` `plainToInstance()` для десериализации

### 3.3. Vite конфигурация

```typescript
// vite.config.ts (ключевые моменты)
export default defineConfig({
  build: {
    lib: {
      entry: 'src/App.ts',    // точка входа — НЕ index.html
      formats: ['es'],
      fileName: () => 'app.js'
    },
    cssCodeSplit: false,       // весь CSS в один файл
  },
  plugins: [
    vue(),
    cssInjectedByJsPlugin(),  // CSS инлайнится в JS бандл
    // Пост-обработка: копирование App.html → index.asp,
    // объединение shell-скриптов → xrayui
  ]
});
```

**Объединение shell-скриптов при сборке** — кастомная функция `inlineShellImports()` в `vite.config.ts`:
- Читает `xrayui.sh`, находит строки `import ./file.sh`
- Рекурсивно инлайнит файлы, удаляя комментарии
- Результат — один монолитный `dist/xrayui`

**Dev workflow** — `pnpm run watch`:
- Vite watch mode перестраивает при изменениях
- `vite.sync.js` через `ssh2-sftp-client` SFTP-шит файлы прямо на роутер:
  - `dist/index.asp` → `/jffs/addons/xrayui/index.asp`
  - `dist/app.js` → `/jffs/addons/xrayui/app.js`
  - `dist/xrayui` → `/jffs/scripts/xrayui`

### 3.4. ASP-обёртка (App.html → index.asp)

```html
<!DOCTYPE html>
<html>
<head>
  <!-- Прошивочные CSS -->
  <link rel="stylesheet" href="/index_style.css">
  <link rel="stylesheet" href="/form_style.css">
  <!-- Прошивочные JS -->
  <script src="/js/jquery.js"></script>
  <script src="/js/httpApi.js"></script>
  <script src="/state.js"></script>
  <script src="/general.js"></script>
  <script src="/popup.js"></script>
</head>
<body onload="initial();" class="bg">
  <div id="TopBanner"></div>
  <div id="Loading" class="popup_bg"></div>

  <script>
  // ASP-теги инжектируют данные с роутера до загрузки Vue
  var xray = {
    router: {
      name: '<% nvram_get("productid"); %>',
      firmware: '<% nvram_get("firmver"); %>',
      language: '<% nvram_get("preferred_lang"); %>',
      devices_online: JSON.parse(`<% get_clientlist(); %>`),
      wan_connected: '<% nvram_get("link_internet"); %>' == '2',
    },
    server: {
      isRunning: parseInt('<% sysinfo("pid.xray"); %>') > 0,
    },
    custom_settings: <% get_custom_settings(); %>,
  };
  </script>

  <!-- Vue SPA монтируется сюда -->
  <div id="xrayui-app"></div>
  <div id="xrayui-modals"></div>

  <!-- Бандл загружается как ES module -->
  <script type="module" src="/ext/xrayui/app.js"></script>

  <div id="footer"></div>
</body>
</html>
```

Ключевые моменты:
- ASP-теги (`<% ... %>`) выполняются на стороне httpd **до** отправки страницы в браузер
- Глобальный объект `window.xray` передаёт данные из прошивки во Vue
- Vue монтируется на `#xrayui-app`
- Модалки телепортируются в `#xrayui-modals` через Vue `<Teleport>`

### 3.5. Протокол коммуникации Frontend ↔ Backend

#### Канал 1: Команды (Frontend → Backend)

Используется стандартный механизм Merlin — POST на `/start_apply.htm` через скрытый iframe:

```typescript
// Engine.ts — submit()
public submit(action: string, payload?: object | string | number | null): Promise<void> {
    const form = document.createElement('form');
    form.method = 'post';
    form.action = '/start_apply.htm';
    form.target = iframeName;  // скрытый iframe

    this.create_form_element(form, 'hidden', 'action_mode', 'apply');
    this.create_form_element(form, 'hidden', 'action_script', action);
    this.create_form_element(form, 'hidden', 'modified', '0');
    this.create_form_element(form, 'hidden', 'action_wait', '');

    // Payload разбивается на чанки по 2048 байт
    // и сохраняется как xray_payload0, xray_payload1, ...
    // в window.xray.custom_settings
    if (payload) this.splitPayload(JSON.stringify(payload));

    this.create_form_element(form, 'hidden', 'amng_custom',
        JSON.stringify(window.xray.custom_settings));

    document.body.appendChild(form);
    form.submit();
}
```

**Chunking payload** — обход лимита 8KB:
```typescript
splitPayload(payload: string) {
    const chunkSize = 2048;
    let idx = 0;
    for (let i = 0; i < payload.length; i += chunkSize) {
        window.xray.custom_settings[`xray_payload${idx}`] = payload.substring(i, i + chunkSize);
        idx++;
    }
}
```

#### Канал 2: Данные (Backend → Frontend)

Backend пишет JSON-файлы в веб-директорию. Frontend читает через `axios.get()` с cache-busting:

```typescript
async getXrayResponse(): Promise<EngineResponseConfig> {
    const response = await axios.get<EngineResponseConfig>(
        `/ext/xrayui/xray-ui-response.json?_=${Date.now()}`
    );
    return response.data;
}
```

**Файлы-ответы** (симлинки в `/www/user/xrayui/`):

| Веб-путь | Назначение |
|----------|-----------|
| `xray-ui-response.json` | Основное состояние UI (версии, настройки, прогресс загрузки) |
| `xray-config.json` | Симлинк на `/opt/etc/xray/config.json` |
| `connection-status.json` | Результаты проверки подключений (Observatory) |
| `clients-online.json` | Статистика подключённых клиентов |
| `subscriptions.json` | Данные подписок |
| `geotags.json` | Списки geosite/geoip тегов |
| `xray_access_partial.asp` | Хвост access-лога |
| `xray_error_partial.asp` | Хвост error-лога |

#### Паттерн прогресса загрузки

Shell пишет прогресс:
```bash
update_loading_progress() {
    local message=$1 progress=$2
    json_content=$(echo "$json_content" | jq \
        --argjson progress "$progress" \
        --arg message "$message" \
        '.loading.message = $message | .loading.progress = $progress')
    echo "$json_content" > "/tmp/xray-response.tmp"
    mv -f "/tmp/xray-response.tmp" "$UI_RESPONSE_FILE"
}
```

Frontend поллит каждую секунду:
```typescript
async checkLoadingProgress(loadingProgress, windowReload = true): Promise<void> {
    const checkProgressInterval = setInterval(async () => {
        const response = await this.getXrayResponse();
        if (response.loading) {
            loadingProgress = response.loading;
            window.updateLoadingProgress(loadingProgress);  // firmware overlay
        } else {
            clearInterval(checkProgressInterval);
            window.hideLoading();
            if (windowReload) window.location.reload();
        }
    }, 1000);
}
```

### 3.6. Backend — shell-скрипты

**Точка входа**: `xrayui.sh` (собирается из ~30 файлов при build)

**Диспетчеризация** — через `service-event` hook. Install добавляет в `/jffs/scripts/service-event`:
```bash
echo "$2" | grep -q "^xrayui" && \
    /jffs/scripts/xrayui service_event $(echo "$2" | cut -d'_' -f2- | tr '_' ' ') & #xrayui
```

`xrayui_configuration_apply` → `/jffs/scripts/xrayui service_event configuration apply`

**Реконструкция payload** в `_helper.sh`:
```bash
reconstruct_payload() {
    local idx=0 chunk payload=""
    while :; do
        chunk=$(am_settings_get xray_payload$idx)
        [ -z "$chunk" ] && break
        payload="$payload$chunk"
        idx=$((idx + 1))
    done
    cleanup_payload
    echo "$payload"
}
```

**Полная таблица действий** (~40 action):

| action_script | Shell dispatch | Назначение |
|--------------|---------------|-----------|
| `xrayui_configuration_apply` | `configuration apply` | Применить конфигурацию |
| `xrayui_configuration_initresponse` | `configuration initresponse` | Инициализация UI response |
| `xrayui_configuration_togglestartup` | `configuration togglestartup` | Вкл/выкл автозапуск |
| `xrayui_configuration_changeprofile` | `configuration changeprofile` | Смена профиля конфига |
| `xrayui_configuration_deleteprofile` | `configuration deleteprofile` | Удалить профиль |
| `xrayui_configuration_backup` | `configuration backup` | Бэкап конфигурации |
| `xrayui_configuration_backuprestore` | `configuration backuprestore` | Восстановление из бэкапа |
| `xrayui_configuration_logs` | `configuration logs` | Управление логами |
| `xrayui_configuration_logs_fetch` | `configuration logs fetch` | Получить логи |
| `xrayui_serverstatus_start` | `serverstatus start` | Запуск Xray |
| `xrayui_serverstatus_restart` | `serverstatus restart` | Перезапуск Xray |
| `xrayui_serverstatus_stop` | `serverstatus stop` | Остановка Xray |
| `xrayui_testconfig` | `testconfig` | Проверить конфигурацию |
| `xrayui_connectedclients` | `connectedclients` | Получить подключённых клиентов |
| `xrayui_connectionstatus` | `connectionstatus` | Проверить статус соединений |
| `xrayui_regenerate_realitykeys` | `regenerate realitykeys` | Генерация Reality ключей |
| `xrayui_update` | `update` | Обновление аддона |
| `xrayui_geodata_communityupdate` | `geodata communityupdate` | Обновить geodata |
| `xrayui_firewall_configure` | `firewall configure` | Настроить firewall |
| `xrayui_firewall_cleanup` | `firewall cleanup` | Очистить firewall |

### 3.7. Структура файлов на роутере

```
/jffs/addons/xrayui/           # Персистентные файлы (переживают reboot)
├── index.asp                  # ASP-обёртка Vue SPA
├── app.js                     # Vue бандл
└── ...

/jffs/scripts/xrayui           # Объединённый backend-скрипт

/www/user/userN.asp            # Копия index.asp (слот Addons API)
/www/user/xrayui/              # Симлинки на файлы для веб-доступа
├── app.js → /jffs/addons/xrayui/app.js
├── xray-config.json → /opt/etc/xray/config.json
├── xray-ui-response.json      # Состояние UI (пишется backend)
├── connection-status.json → ...
└── ...

/opt/etc/xrayui.conf           # Конфиг аддона (shell key=value)
/opt/etc/xray/config.json      # Конфиг Xray core
```

### 3.8. Инициализация Vue-приложения

1. ASP-сервер рендерит `index.asp`, инжектируя данные в `window.xray`
2. Прошивочный JS (`state.js`, `general.js`) инициализирует меню и layout
3. Vue app монтируется по `DOMContentLoaded`
4. `App.vue.setup()` → `engine.loadXrayConfig()` — GET текущего конфига
5. Submit `initResponse` action через iframe → backend генерирует UI response JSON
6. Polling `xray-ui-response.json` до 10 раз (1с интервал) ожидая появления `core_version`
7. `provide` `uiResponse` и `xrayConfig` → все дочерние компоненты

### 3.9. Dev-инфраструктура

**Сборка**: `pnpm install && pnpm run build`

**CI/CD**: GitHub Actions → pnpm install → test → build → GitHub Release с tar.gz

**Dev-sync** (`vite.sync.js`):
```javascript
// При каждом rebuild → SFTP на роутер
const sftp = new SftpClient();
await sftp.connect({ host: routerIp, username: 'admin', ... });
await sftp.put('dist/index.asp', '/jffs/addons/xrayui/index.asp');
await sftp.put('dist/app.js', '/jffs/addons/xrayui/app.js');
await sftp.put('dist/xrayui', '/jffs/scripts/xrayui');
```

**Тесты**: Jest с фейковыми данными

**Release**: `build.tar.sh` → `tar --transform 's|^|xrayui/|' -czf asuswrt-merlin-xrayui.tar.gz -C dist .`

### 3.10. Ключевые архитектурные решения

| Решение | Почему |
|---------|--------|
| Vue 3 + TypeScript | Сложная вложенная модель данных (протоколы, transport, security, routing). `class-transformer` для типизированной десериализации |
| Single JS bundle | Минимизация файлов на ограниченном JFFS роутера |
| Shell concat при build | Избежание `source` в runtime (проблемы с путями), минимизация disk I/O |
| Нет HTTP-сервера | Используется firmware httpd + service-event. Экономия ресурсов, но вся коммуникация асинхронная через polling |
| Payload chunking | Обход лимита 8KB `amng_custom` |
| CSS из прошивки | Консистентный UI, нативный вид. SCSS только для addon-специфичных элементов |

---

## 4. WireGuard Manager

**Репозиторий**: github.com/MartineauUK/wireguard  
**Файл**: `wg_manager.asp` (~1371 строк)

Ближайший аналог по функциональности — управление VPN-туннелями.

**Архитектура**: классический паттерн (hidden form + service-event). Команды (`start wg11`, `stop wg21`, `peer wg11 del`, `vpndirector list`) сериализуются в `amng_custom` JSON.

**Base64 для результатов**: ответы возвращаются через `custom_settings.wgm_Execute_Result`, закодированные в base64, декодируются на клиенте через `atob()`. Это решает проблему с пробелами в `get_custom_settings()`.

**UI-элементы**:
- Чекбоксы для start/stop клиентов и серверов (`SwitchClientState()`/`SwitchServerState()`)
- Per-peer Start/Stop/Restart кнопки
- Категорийное управление (all/clients/servers)
- Диагностические кнопки: показать firewall rules, RPDB routing policy, routes
- Интеграция с VPN Director для policy routing rules

**Чтение конфига** — AJAX через симлинки:
```javascript
$j.ajax({ url: '/ext/wireguard/config.htm' }); // checkbox states
```

---

## 5. Сравнительная таблица

| Аспект | scMerlin | YazFi | FlexQoS | Skynet | WG Manager | XrayUI |
|--------|----------|-------|---------|--------|------------|--------|
| **Стек** | HTML+JS | HTML+JS | HTML+JS | HTML+JS | HTML+JS | Vue 3 + Vite + TS |
| **Количество файлов UI** | 1 .asp | 1 .asp + 1 .js + 1 .css | 1 .asp | 1 .asp + chart libs | 1 .asp | 1 .asp + 1 .js |
| **Формы** | 2 (main + actions) | 2 | 2 | 1 | 1 | Динамические iframe |
| **Хранение конфига** | custom_settings | Собственный файл | custom_settings | Собственный файл | custom_settings + base64 | Собственный JSON + chunked payload |
| **Live данные** | JS file polling | CSV file polling (5с) | firmware ASP endpoint | Нет (перезагрузка) | custom_settings | JSON file polling (1с) |
| **Прогресс** | JS file polling | Нет | JS file polling | Нет | Нет | JSON polling с процентами |
| **Build** | Нет | Нет | Нет | Нет | Нет | Vite + shell concat |
| **i18n** | Нет | Нет | Нет | Нет | Нет | 5 языков |
| **CSS** | Firmware + inline | Firmware + inline | Firmware | Firmware | Firmware | Firmware + SCSS |

---

## 6. Рекомендации для VPN Director

### 6.1. Рекомендуемый подход

**Подход XrayUI** (Vue + Vite) оптимален для VPN Director:
- У нас уже есть сложная конфигурация (tunnels, clients, excludes, ipsets)
- JSON-конфиг (`vpn-director.json`) удобно редактировать через типизированные Vue-компоненты
- Нужно отображать статус нескольких подсистем (Xray, Tunnel Director, IPsets)
- TypeScript обеспечит корректность работы с конфигом

### 6.2. Конкретные решения

| Вопрос | Решение | Обоснование |
|--------|---------|-------------|
| Хранение конфига | Собственный JSON (`vpn-director.json`) | Уже есть, не зависим от 8KB лимита |
| Передача конфига | Chunked payload через `custom_settings` | Паттерн XrayUI, проверен в production |
| Статус | JSON file polling | Записываем `status.json`, UI поллит |
| Действия | service-event dispatch | Стандарт платформы |
| Сборка | Vite single bundle | Минимум файлов на роутере |
| Dev workflow | SFTP sync при rebuild | Паттерн XrayUI |
| Меню | Tab в разделе VPN | Логичное расположение |

### 6.3. Потенциальный конфликт с XrayUI

VPN Director и XrayUI оба управляют Xray. Если пользователь использует оба — возможны конфликты в:
- iptables rules (оба создают TPROXY chains)
- Xray config (оба пишут в `/opt/etc/xray/config.json`)
- service-event namespace (`xrayui_*` vs `vpndirector_*` — ок, разные префиксы)
- `/www/user/` слоты (20 слотов хватит)

Варианты:
1. Предупреждать при установке если XrayUI уже есть
2. Разграничить зоны ответственности (VPN Director = routing, XrayUI = Xray core config)
3. Интегрироваться (опциональная поддержка XrayUI как backend для Xray)

### 6.4. MVP scope

Первая версия Web UI:
1. **Status page** — состояние Xray, Tunnel Director, IPsets (аналог `vpn-director.sh status`)
2. **Apply/Stop/Restart** кнопки
3. **Просмотр конфигурации** — read-only отображение текущего `vpn-director.json`
4. **IPset Update** — кнопка обновления ipsets

Вторая версия:
1. **Редактирование конфигурации** — tunnels, clients, excludes
2. **Интерактивный wizard** — аналог `configure.sh`
3. **Логи** — последние записи syslog

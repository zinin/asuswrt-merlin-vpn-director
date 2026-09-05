# Web UI: единое самообновление (блок 3) — план реализации

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Заменить самообновление, знающее только про бота, на общий поток обновления, который выкладывает и перезапускает оба демона (`telegram-bot` и `webui`), запускается одинаково из Telegram и из Web UI и восстанавливает работавшие демоны при сбое.

**Architecture:** Пакет `updater` получает таблицу демонов (имя ассета = имя файла = имя процесса), качает по бинарнику на демона и рендерит шаблон скрипта из этой же таблицы; скрипт запоминает работавших демонов, останавливает их, копирует всё, пишет `notify.json` со `status`/`initiator` и восстанавливается через `trap ... EXIT` (у ash нет `ERR`). Новый пакет `updateflow` держит всю оркестрацию: `Check` с 30-минутным кешем и минутным порогом для `force`, `Start` с синхронным пре-флайтом и фоновой загрузкой, типизированные ошибки. Бот и Web UI становятся тонкими адаптерами над `Flow`: бот шлёт прогресс в чат, Web UI — в свой лог; `startup.CheckAndSendNotify` при `chat_id` 0 уведомляет все активные чаты.

**Tech Stack:** Go 1.25 (`text/template`, `context`, `sync.Mutex`, `errors.As`, `net/http` `ResponseController`), BusyBox ash на роутере (`#!/bin/sh`, `set -e`, `trap ... EXIT`, параметрические подстановки вместо массивов), Vue 3 + TypeScript (axios).

**Spec:** `docs/superpowers/specs/2026-09-05-webui-hardening-design.md`, секция 6 «Блок 3. Единое самообновление». Секции 1–3 — контекст и принятые решения; секция 5 (блок 2) выполнена и служит источником уже существующих API (`extendWriteDeadline`, `UpdateVPNConfig`, `WebUILogPath`).

---

## Global Constraints

- Ветка: `feature/webui-behavior`. Все четыре блока идут в одной ветке и одном PR, по порядку; блок 3 начинается с HEAD блока 2 (`ad03b7e` + docs-коммит `cbdb212`). Новых веток и worktree нет, `feature/unified-updater` из таблицы §2 спека **не создаётся**.
- `docs/superpowers/` в этом блоке **не удалять**: удаление одним `git rm` перед единственным PR после блока 4.
- Сборка, тесты, `vet`, `vue-tsc` — только через `claude-forge:build` (build-runner), включая исполнителей-субагентов. `bats`, `jq`, `git` — напрямую. Go-тесты всегда с `-count=1` (иначе пакет вернётся как `(cached)`).
- Git: в `git add` только явные пути; никаких `-A`, `.`, `commit -a`, `stash`, `clean`. В рабочем дереве лежат untracked-файлы автора (`.mcp.json`, `review.diff`, `test_exit.sh`, `server/bot`, `server/webui`, `docs/superpowers/plans/*-prompt.md`) — они не должны попасть ни в один коммит.
- Каждый коммит заканчивается пустой строкой и `Claude-Session: <URL текущей сессии>` (используйте `-m "..." -m "Claude-Session: ..."`).
- Go-код gofmt-чистый в затронутых файлах (`gofmt -l <файлы>` пуст). **Не переформатировать файлы, которых задача не касается** — в модуле 11 исторически не-gofmt файлов.
- Выравнивание в литералах в тексте этого плана не нормализовано: после вставки кода прогоняйте `gofmt -w` по файлам своей задачи, прежде чем проверять `gofmt -l`.
- Комментарии, идентификаторы, тексты коммитов — по-английски, как в остальном репозитории. Общение с пользователем — по-русски.
- Тексты из спеки дословно: имена ассетов `telegram-bot-<arch>` и `webui-<arch>` для `arm64` и `arm`; init-скрипт `S98vpn-director-webui`; формат `notify.json` — `{"chat_id":0,"old_version":"v1.2.0","new_version":"v1.3.0","status":"ok","initiator":"webui"}`; сообщение о провале `Update failed: see /tmp/vpn-director-update/update.log`; баннер во фронте `Version vX available, see Settings`.
- Кеш проверки обновлений: 30 минут; `force` обходит кеш не чаще раза в минуту (GitHub без токена — 60 запросов в час).
- Опрос версии во фронте: каждые 3 секунды, `skipAuthRedirect`, потолок 5 минут, затем перезагрузка страницы.
- Скрипт обновления остаётся `#!/bin/sh` с `set -e`. Восстановление только через `trap ... EXIT` с проверкой кода выхода: ash не знает `ERR`.
- HTTP: глобальный `WriteTimeout` 30 с (блок 2). **Каждый новый маршрут, который синхронно ходит в GitHub, обязан продлевать дедлайн** через `extendWriteDeadline`.
- `service.ConfigStore` не содержит `SaveVPNConfig`; писать `vpn-director.json` можно только через `UpdateVPNConfig`. Блок 3 конфиг не пишет вообще: обновление **не должно** трогать `jwt_secret` — на этом держится обещание «cookie переживает обновление».
- `updatechecker` не меняется (спек §6.3).
- Первый переход на версию с этим блоком выполнит старый updater бота, который не трогает Web UI. Release notes этой версии просят один раз перезапустить `install.sh` — текст релиза не входит в объём плана, но упоминается в документации (Task 10).

---

## Отклонения от спека (согласованы с автором до написания плана)

1. **`Flow.Start` получает `initiator` явным параметром.** Спек §6.1 определяет `RunOptions.Initiator`, но сигнатура §6.3 `Start(ctx, chatID, progress)` не даёт его передать. Принято: `Start(ctx, initiator string, chatID int64, progress func(string))`.
2. **При `status: failed` каталог обновления не удаляется целиком.** Спек §6.6 велит и сослать пользователя на `/tmp/vpn-director-update/update.log`, и вычистить каталог после успешной отправки. Принято: при `failed` удаляется только `notify.json`, `update.log` остаётся; при `ok` — весь каталог, как в спеке.
3. **Три текста бота про валидацию версии сворачиваются в один.** Вместо «Invalid current version: X» / «Invalid release version: X» / «Failed to parse version: e» бот отвечает `Failed to start update: <ошибка>`. Путь достижим только при некорректном теге сборки. Остальные тексты сохранены дословно.
4. **Добавлено продление write-дедлайна на новых маршрутах** (`GET /api/update/check`, `POST /api/update`). В спеке шага нет; без него соединение рвётся на 30 с посреди запроса к GitHub API (у `updater` собственный таймаут 30 с).
5. **Текст «Update script started, bot will restart in a few seconds...» → «Update script started, the service will restart in a few seconds...»**: перезапускается уже не только бот.
6. **`GET /api/update/check` отвечает `{"update_available": false, "dev": true}` не только при `--dev`, но и для сборки с версией `dev`**: такая сборка не умеет сравнивать версии, и отдавать 500 на штатной ситуации незачем.

---

## Файловая структура

| Область | Файлы | Ответственность |
|---|---|---|
| Таблица демонов | `server/internal/updater/updater.go` | `Daemon`, `Daemons` — единственный источник имён ассетов, путей бинарников и init-скриптов |
| Загрузка | `server/internal/updater/downloader.go` (+`_test`) | `scriptFiles` с `S98vpn-director-webui`; `downloadBinaries` по одному бинарнику на демона; отсутствие ассета — ошибка |
| Запуск скрипта | `server/internal/updater/script.go` (+`_test`), `update_script.sh.tmpl`, `testdata/update_script.golden.sh` (новый) | `RunOptions`; рендер шаблона из таблицы демонов; golden-тест и `sh -n` |
| Интерфейс updater | `server/internal/updater/updater.go`, `server/internal/updater/github.go` | `RunUpdateScript(RunOptions)`; экспорт `APITimeout` |
| Оркестрация | `server/internal/updateflow/flow.go`, `start.go` (+`flow_test.go`, `start_test.go`) — новый пакет | `Check` (кеш 30 мин, порог `force`), `Start` (пре-флайт, лок, фоновая загрузка), `InProgress`, типизированные ошибки |
| Бот | `server/internal/handler/update.go` (+`_test`), `server/internal/handler/handler.go`, `server/internal/bot/bot.go` | `/update` как адаптер над `Flow`; удаление мёртвого `Deps.Updater` |
| Уведомление | `server/internal/startup/notify.go` (+`_test`), `server/internal/bot/bot.go` | `chat_id` 0 → все активные чаты; `status: failed`; правила очистки |
| Web API | `server/internal/webapi/handler_update.go` (новый, +`_test`), `router.go`, `deadline.go`, `handler_logs.go`, `handler_logs_test.go`, `test_helpers_test.go` | три маршрута, дедлайны, удаление заглушки 501 |
| Входная точка | `server/cmd/webui/main.go` | `updater.New()` + `updateflow.New` + `Deps.Update` |
| Фронт | `web/src/api.ts`, `web/src/types.ts`, `web/src/components/SettingsTab.vue`, `web/src/App.vue` | вкладка Settings, опрос версии, баннер в шапке |
| Документация | `README.md`, `README.ru.md`, `.claude/rules/telegram-bot.md` | поток обновления, оба демона, одноразовая миграция |

**Порядок задач обязателен:** 1 → 2 → 3 (таблица демонов до загрузки и до шаблона), 4 → 5 (Check до Start), 5 → 6 → 7 (Flow до адаптера бота, адаптер до уведомления), 5 → 8 → 9 (Flow до Web API, Web API до фронта). Задача 10 — последняя.

---

### Task 1: таблица демонов и загрузка обоих бинарников

**Files:**
- Modify: `server/internal/updater/updater.go` (добавить `Daemon`/`Daemons` после блока `Asset`)
- Modify: `server/internal/updater/downloader.go` (`scriptFiles`, `Service.archSuffix`, `downloadBinaries`, удалить `downloadBotBinary`)
- Test: `server/internal/updater/downloader_test.go`

**Interfaces:**
- Consumes: ничего.
- Produces: `updater.Daemon{Name, Binary, InitScript string}`; `var updater.Daemons []Daemon`; `Service.archSuffix string` (тестовая инъекция); `(*Service).downloadBinaries(ctx, *Release) error`; `archAssetSuffix(goarch string) (string, error)`.

- [ ] **Step 1: Написать падающие тесты**

В `server/internal/updater/downloader_test.go` **удалить** `TestDownloadBotBinary_AssetNotFound` (функции `downloadBotBinary` больше не будет) и добавить:

```go
func TestScriptFiles_ExistInRepo(t *testing.T) {
	// The list is a copy of install.sh's downloads; a renamed or removed file
	// silently breaks every future update, so pin it to the working tree.
	for _, f := range scriptFiles {
		path := filepath.Join("..", "..", "..", f)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("scriptFiles entry %q not found in the repo: %v", f, err)
		}
	}
}

func TestScriptFiles_IncludesWebUIInitScript(t *testing.T) {
	const want = "router/opt/etc/init.d/S98vpn-director-webui"
	for _, f := range scriptFiles {
		if f == want {
			return
		}
	}
	t.Errorf("scriptFiles must ship %s, otherwise an update leaves the old init script", want)
}

func TestArchAssetSuffix(t *testing.T) {
	tests := []struct {
		goarch  string
		want    string
		wantErr bool
	}{
		{goarch: "arm64", want: "arm64"},
		{goarch: "arm", want: "arm"},
		{goarch: "amd64", wantErr: true},
		{goarch: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.goarch, func(t *testing.T) {
			got, err := archAssetSuffix(tt.goarch)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("archAssetSuffix(%q) = %q, want error", tt.goarch, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("archAssetSuffix(%q) error = %v", tt.goarch, err)
			}
			if got != tt.want {
				t.Errorf("archAssetSuffix(%q) = %q, want %q", tt.goarch, got, tt.want)
			}
		})
	}
}

func TestDownloadBinaries_DownloadsEveryDaemon(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("binary of " + strings.TrimPrefix(r.URL.Path, "/")))
	}))
	defer server.Close()

	tempDir := t.TempDir()
	s := &Service{httpClient: &http.Client{}, updateDir: tempDir, archSuffix: "arm64"}

	release := &Release{TagName: "v1.0.0"}
	for _, d := range Daemons {
		release.Assets = append(release.Assets, Asset{
			Name:        d.Name + "-arm64",
			DownloadURL: server.URL + "/" + d.Name + "-arm64",
		})
	}

	if err := s.downloadBinaries(context.Background(), release); err != nil {
		t.Fatalf("downloadBinaries() error = %v", err)
	}

	for _, d := range Daemons {
		target := filepath.Join(tempDir, "files", d.Name)
		data, err := os.ReadFile(target)
		if err != nil {
			t.Fatalf("binary for %s not downloaded: %v", d.Name, err)
		}
		if want := "binary of " + d.Name + "-arm64"; string(data) != want {
			t.Errorf("%s content = %q, want %q", d.Name, data, want)
		}
	}
}

func TestDownloadBinaries_MissingAssetIsAnError(t *testing.T) {
	// A release that ships only one of the two binaries would install a new
	// bot next to an old Web UI; refuse the whole download instead.
	for _, missing := range Daemons {
		t.Run("without "+missing.Name, func(t *testing.T) {
			tempDir := t.TempDir()
			s := &Service{httpClient: &http.Client{}, updateDir: tempDir, archSuffix: "arm64"}

			release := &Release{TagName: "v1.0.0"}
			for _, d := range Daemons {
				if d.Name == missing.Name {
					continue
				}
				release.Assets = append(release.Assets, Asset{
					Name:        d.Name + "-arm64",
					DownloadURL: "https://example.invalid/" + d.Name,
				})
			}

			err := s.downloadBinaries(context.Background(), release)
			if err == nil {
				t.Fatalf("downloadBinaries() must fail without asset %s-arm64", missing.Name)
			}
			if !strings.Contains(err.Error(), missing.Name+"-arm64") {
				t.Errorf("error %q should name the missing asset %s-arm64", err, missing.Name)
			}
		})
	}
}
```

- [ ] **Step 2: Запустить и убедиться, что тесты падают**

Через `claude-forge:build`: `cd server && go test ./internal/updater/ -run 'TestScriptFiles|TestArchAssetSuffix|TestDownloadBinaries' -count=1 -v`
Ожидание: FAIL — `undefined: archAssetSuffix`, `undefined: Daemons`, `s.archSuffix undefined`, а `TestScriptFiles_IncludesWebUIInitScript` падает по существу.

- [ ] **Step 3: Добавить таблицу демонов**

В `server/internal/updater/updater.go` сразу после типа `Asset`:

```go
// Daemon describes an updatable daemon. Name is both the release asset prefix
// (<Name>-<arch>) and the file name under files/; Binary is where the update
// script installs it and what pgrep matches on; InitScript is the Entware
// script that starts and stops it.
type Daemon struct {
	Name       string
	Binary     string
	InitScript string
}

// Daemons lists every daemon a release ships. DownloadRelease fetches one
// binary per entry and the update script restarts the entries that were
// running before the update. This table is the single source of truth: the
// downloader, the script template and install.sh must not drift apart.
var Daemons = []Daemon{
	{Name: "telegram-bot", Binary: "/opt/vpn-director/telegram-bot", InitScript: "S98telegram-bot"},
	{Name: "webui", Binary: "/opt/vpn-director/webui", InitScript: "S98vpn-director-webui"},
}
```

В том же файле добавить поле в `Service` (после `scriptFile`):

```go
	archSuffix string // Injectable for testing, empty = derived from runtime.GOARCH
```

- [ ] **Step 4: Переписать загрузку бинарников**

В `server/internal/updater/downloader.go` добавить в `scriptFiles` строку после `S98telegram-bot`:

```go
	"router/opt/etc/init.d/S98vpn-director-webui",
```

Заменить вызов в `DownloadRelease`:

```go
	// Download daemon binaries
	if err := s.downloadBinaries(ctx, release); err != nil {
		return fmt.Errorf("download binaries: %w", err)
	}
```

Заменить `downloadBotBinary` на:

```go
// archAssetSuffix maps a Go architecture to the release asset suffix.
func archAssetSuffix(goarch string) (string, error) {
	switch goarch {
	case "arm64":
		return "arm64", nil
	case "arm":
		return "arm", nil
	default:
		return "", fmt.Errorf("unsupported architecture: %s", goarch)
	}
}

// getArchSuffix returns the release asset suffix for this build.
func (s *Service) getArchSuffix() (string, error) {
	if s.archSuffix != "" {
		return s.archSuffix, nil
	}
	return archAssetSuffix(runtime.GOARCH)
}

// downloadBinaries downloads one binary per daemon into files/<name>.
// A release missing any of them is a download error: installing a new bot
// next to an old Web UI leaves two halves of different versions on the router.
func (s *Service) downloadBinaries(ctx context.Context, release *Release) error {
	suffix, err := s.getArchSuffix()
	if err != nil {
		return err
	}

	for _, d := range Daemons {
		assetName := d.Name + "-" + suffix
		url := assetURL(release, assetName)
		if url == "" {
			return fmt.Errorf("asset %s not found in release", assetName)
		}
		target := filepath.Join(s.getFilesDir(), d.Name)
		if err := s.downloadFile(ctx, url, target); err != nil {
			return fmt.Errorf("download %s: %w", assetName, err)
		}
	}
	return nil
}

// assetURL returns the download URL of the named asset, or "" when the
// release does not carry it.
func assetURL(release *Release, name string) string {
	for _, a := range release.Assets {
		if a.Name == name {
			return a.DownloadURL
		}
	}
	return ""
}
```

- [ ] **Step 5: Запустить тесты пакета**

Через `claude-forge:build`: `cd server && go test ./internal/updater/ -count=1 -v`
Ожидание: PASS, включая старые тесты (`TestDownloadRelease_CleansBeforeDownload` продолжает работать — он падает на скриптах до бинарников).

- [ ] **Step 6: Проверить сборку модуля и формат**

Через `claude-forge:build`: `cd server && go build ./... && go vet ./... && gofmt -l internal/updater/updater.go internal/updater/downloader.go internal/updater/downloader_test.go`
Ожидание: сборка и vet чистые, `gofmt -l` не печатает ничего.

- [ ] **Step 7: Коммит**

```bash
git add server/internal/updater/updater.go server/internal/updater/downloader.go server/internal/updater/downloader_test.go
git commit -m "feat(updater): download a binary for every daemon and ship the Web UI init script" -m "Claude-Session: <URL текущей сессии>"
```

---

### Task 2: `RunOptions` вместо трёх позиционных аргументов

**Files:**
- Modify: `server/internal/updater/updater.go` (интерфейс `Updater`)
- Modify: `server/internal/updater/script.go` (`RunOptions`, `RunUpdateScript`, `generateScript`, `scriptData`)
- Modify: `server/internal/handler/update.go:126` (единственная точка вызова)
- Test: `server/internal/updater/script_test.go`, `server/internal/handler/update_test.go` (мок)

**Interfaces:**
- Consumes: `updater.Daemons` (Task 1) — пока не используется, появится в Task 3.
- Produces: `updater.RunOptions{OldVersion, NewVersion string; ChatID int64; Initiator string}`; `Updater.RunUpdateScript(opts RunOptions) error`.

- [ ] **Step 1: Написать падающие тесты**

В `server/internal/updater/script_test.go` заменить вызовы `s.RunUpdateScript(123, old, new)` на `RunOptions` и добавить проверку инициатора. Полный набор изменений:

```go
func validOpts() RunOptions {
	return RunOptions{OldVersion: "v1.0.0", NewVersion: "v1.1.0", ChatID: 123, Initiator: "bot"}
}

func TestRunUpdateScript_InvalidInitiator(t *testing.T) {
	// Initiator is interpolated into the shell script the same way versions
	// are, so it gets the same allow-list treatment.
	tmpDir := t.TempDir()
	s := &Service{updateDir: tmpDir, scriptFile: filepath.Join(tmpDir, "update.sh")}

	for _, initiator := range []string{"", "cron", `bot";rm -rf /;"`} {
		t.Run(initiator, func(t *testing.T) {
			opts := validOpts()
			opts.Initiator = initiator
			err := s.RunUpdateScript(opts)
			if err == nil || !strings.Contains(err.Error(), "invalid initiator") {
				t.Fatalf("RunUpdateScript(initiator=%q) error = %v, want invalid initiator", initiator, err)
			}
		})
	}
}

func TestGenerateScript_EmbedsInitiator(t *testing.T) {
	tmpDir := t.TempDir()
	s := &Service{updateDir: tmpDir}

	script, err := s.generateScript(RunOptions{
		OldVersion: "v1.0.0", NewVersion: "v1.1.0", ChatID: 0, Initiator: "webui",
	})
	if err != nil {
		t.Fatalf("generateScript() error = %v", err)
	}
	if !strings.Contains(script, `INITIATOR="webui"`) {
		t.Error("script must carry the initiator so notify.json can name it")
	}
	if !strings.Contains(script, "CHAT_ID=0") {
		t.Error("a Web UI update has chat_id 0")
	}
}
```

В существующих тестах `TestGenerateScript`, `TestGenerateScript_PathsCorrect`, `TestRunUpdateScript_InvalidVersion`, `TestRunUpdateScript_ValidVersion`, `TestRunUpdateScript_ScriptContent` заменить сигнатуры: `s.generateScript(123456789, "v1.0.0", "v1.1.0")` → `s.generateScript(RunOptions{ChatID: 123456789, OldVersion: "v1.0.0", NewVersion: "v1.1.0", Initiator: "bot"})`, аналогично для `RunUpdateScript`. Таблица в `TestRunUpdateScript_InvalidVersion` строится из `validOpts()` с подменой одного поля.

- [ ] **Step 2: Запустить и убедиться, что тесты падают**

Через `claude-forge:build`: `cd server && go test ./internal/updater/ -count=1`
Ожидание: FAIL — `undefined: RunOptions`.

- [ ] **Step 3: Ввести `RunOptions`**

В `server/internal/updater/updater.go` заменить строку интерфейса:

```go
	// RunUpdateScript generates and runs the update shell script.
	RunUpdateScript(opts RunOptions) error
```

В `server/internal/updater/script.go` добавить тип и переписать сигнатуры:

```go
// RunOptions describes one update run.
type RunOptions struct {
	OldVersion string
	NewVersion string
	ChatID     int64  // 0 when the update was started from the Web UI
	Initiator  string // "bot" or "webui"
}

// scriptData holds the data for the update script template.
type scriptData struct {
	ChatID     int64
	Initiator  string
	OldVersion string
	NewVersion string
	UpdateDir  string
	FilesDir   string
	NotifyFile string
	LockFile   string
	Daemons    []Daemon
}

// RunUpdateScript generates the update script and runs it detached.
// Validates every string embedded into the script to prevent shell injection.
func (s *Service) RunUpdateScript(opts RunOptions) error {
	if !IsValidVersion(opts.OldVersion) {
		return fmt.Errorf("invalid old version: %q", opts.OldVersion)
	}
	if !IsValidVersion(opts.NewVersion) {
		return fmt.Errorf("invalid new version: %q", opts.NewVersion)
	}
	if opts.Initiator != "bot" && opts.Initiator != "webui" {
		return fmt.Errorf("invalid initiator: %q", opts.Initiator)
	}

	script, err := s.generateScript(opts)
	// ... остальное тело без изменений
```

`generateScript` меняет сигнатуру и заполнение:

```go
// generateScript creates the update script content from template.
func (s *Service) generateScript(opts RunOptions) (string, error) {
	tmpl, err := template.New("update").Parse(updateScriptTemplate)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}

	updateDir := s.getUpdateDir()

	data := scriptData{
		ChatID:     opts.ChatID,
		Initiator:  opts.Initiator,
		OldVersion: opts.OldVersion,
		NewVersion: opts.NewVersion,
		UpdateDir:  updateDir,
		FilesDir:   filepath.Join(updateDir, "files"),
		NotifyFile: filepath.Join(updateDir, "notify.json"),
		LockFile:   filepath.Join(updateDir, "lock"),
		Daemons:    Daemons,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}

	return buf.String(), nil
}
```

- [ ] **Step 4: Поправить точку вызова и мок**

В `server/internal/updater/update_script.sh.tmpl` добавить строку после `CHAT_ID={{.ChatID}}` (шаблон переписывается в Task 3, здесь нужен минимум, чтобы `TestGenerateScript_EmbedsInitiator` прошёл):

```
INITIATOR="{{.Initiator}}"
```

В `server/internal/handler/update.go` заменить строку 126:

```go
	if err := h.updater.RunUpdateScript(updater.RunOptions{
		OldVersion: h.version,
		NewVersion: release.TagName,
		ChatID:     chatID,
		Initiator:  "bot",
	}); err != nil {
```

В `server/internal/handler/update_test.go` поправить мок и его геттер:

```go
	runScriptOpts updater.RunOptions

func (m *mockUpdater) RunUpdateScript(opts updater.RunOptions) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runScriptCalled = true
	m.runScriptOpts = opts
	return m.runScriptErr
}

func (m *mockUpdater) getRunScriptOpts() updater.RunOptions {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.runScriptOpts
}
```

Удалить поля `runScriptChatID`, `runScriptOldVer`, `runScriptNewVer` и функцию `getRunScriptArgs`; в `TestUpdateHandler_Success` заменить их использование на `opts := upd.getRunScriptOpts()` и сверять `opts.ChatID`, `opts.OldVersion`, `opts.NewVersion`, а также `opts.Initiator == "bot"`.

- [ ] **Step 5: Запустить тесты**

Через `claude-forge:build`: `cd server && go test ./internal/updater/ ./internal/handler/ -count=1`
Ожидание: PASS.

- [ ] **Step 6: Проверить сборку и формат**

Через `claude-forge:build`: `cd server && go build ./... && go vet ./... && gofmt -l internal/updater/script.go internal/updater/updater.go internal/updater/script_test.go internal/handler/update.go internal/handler/update_test.go`
Ожидание: чисто.

- [ ] **Step 7: Коммит**

```bash
git add server/internal/updater/script.go server/internal/updater/updater.go server/internal/updater/script_test.go server/internal/updater/update_script.sh.tmpl server/internal/handler/update.go server/internal/handler/update_test.go
git commit -m "refactor(updater): take run parameters as a struct and record the initiator" -m "Claude-Session: <URL текущей сессии>"
```

---

### Task 3: скрипт обновления работает со списком демонов

**Files:**
- Rewrite: `server/internal/updater/update_script.sh.tmpl`
- Create: `server/internal/updater/testdata/update_script.golden.sh`
- Test: `server/internal/updater/script_test.go`

**Interfaces:**
- Consumes: `updater.Daemons`, `scriptData.Daemons`, `scriptData.Initiator` (Tasks 1–2).
- Produces: `notify.json` с полями `status` (`ok`/`failed`) и `initiator`; поведение «перезапускаются только те демоны, что работали».

- [ ] **Step 1: Написать падающие тесты**

Добавить в `server/internal/updater/script_test.go`:

```go
func TestGenerateScript_CoversEveryDaemon(t *testing.T) {
	s := &Service{}
	script, err := s.generateScript(validOpts())
	if err != nil {
		t.Fatalf("generateScript() error = %v", err)
	}

	for _, d := range Daemons {
		for _, want := range []string{d.Name, d.Binary, d.InitScript} {
			if !strings.Contains(script, want) {
				t.Errorf("script missing %q for daemon %s", want, d.Name)
			}
		}
	}

	// The daemon table drives every loop; a stray hardcoded init call would
	// silently skip the other daemon.
	if strings.Contains(script, "/opt/etc/init.d/S98telegram-bot stop") {
		t.Error("script must stop daemons through the table, not by name")
	}
}

func TestGenerateScript_RecoveryOnFailure(t *testing.T) {
	s := &Service{}
	script, err := s.generateScript(validOpts())
	if err != nil {
		t.Fatalf("generateScript() error = %v", err)
	}

	checks := map[string]string{
		"trap on_exit EXIT":       "recovery must hang off EXIT: ash has no ERR trap",
		"code=$?":                 "the EXIT trap must inspect the exit code",
		"set +e":                  "recovery must survive its own failing steps",
		"write_notify failed":     "a failed update must leave status failed in notify.json",
		"start_running":           "recovery must restart the daemons that were running",
		`rm -f "$LOCK_FILE"`:      "a failed update must release the lock",
	}
	for needle, why := range checks {
		if !strings.Contains(script, needle) {
			t.Errorf("script missing %q: %s", needle, why)
		}
	}
}

func TestGenerateScript_NotifyFormat(t *testing.T) {
	s := &Service{}
	script, err := s.generateScript(RunOptions{
		OldVersion: "v1.2.0", NewVersion: "v1.3.0", ChatID: 0, Initiator: "webui",
	})
	if err != nil {
		t.Fatalf("generateScript() error = %v", err)
	}

	const want = `{"chat_id":$CHAT_ID,"old_version":"$OLD_VERSION","new_version":"$NEW_VERSION","status":"$1","initiator":"$INITIATOR"}`
	if !strings.Contains(script, want) {
		t.Errorf("notify.json template line missing or changed, want %s", want)
	}
}

func TestGenerateScript_IsValidShell(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh not available")
	}

	s := &Service{}
	script, err := s.generateScript(validOpts())
	if err != nil {
		t.Fatalf("generateScript() error = %v", err)
	}

	path := filepath.Join(t.TempDir(), "update.sh")
	if err := os.WriteFile(path, []byte(script), 0644); err != nil {
		t.Fatal(err)
	}

	out, err := exec.Command(sh, "-n", path).CombinedOutput()
	if err != nil {
		t.Fatalf("generated script has a syntax error: %v\n%s", err, out)
	}
}

func TestGenerateScript_Golden(t *testing.T) {
	// Default paths keep the render deterministic, so the golden file shows
	// the exact script that ships to routers. Regenerate with:
	//   UPDATE_GOLDEN=1 go test ./internal/updater -run TestGenerateScript_Golden -count=1
	s := &Service{}
	script, err := s.generateScript(RunOptions{
		OldVersion: "v1.2.0", NewVersion: "v1.3.0", ChatID: 42, Initiator: "bot",
	})
	if err != nil {
		t.Fatalf("generateScript() error = %v", err)
	}

	golden := filepath.Join("testdata", "update_script.golden.sh")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll("testdata", 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(golden, []byte(script), 0644); err != nil {
			t.Fatal(err)
		}
		t.Logf("rewrote %s", golden)
		return
	}

	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read golden: %v (regenerate with UPDATE_GOLDEN=1)", err)
	}
	if string(want) != script {
		t.Errorf("generated script differs from %s; review the diff and regenerate with UPDATE_GOLDEN=1 if the change is intended", golden)
	}
}
```

Добавить в импорты теста `os/exec`.

- [ ] **Step 2: Запустить и убедиться, что тесты падают**

Через `claude-forge:build`: `cd server && go test ./internal/updater/ -run TestGenerateScript -count=1 -v`
Ожидание: FAIL — в шаблоне нет ни `trap on_exit EXIT`, ни таблицы демонов, ни golden-файла.

- [ ] **Step 3: Переписать шаблон**

Полностью заменить `server/internal/updater/update_script.sh.tmpl`:

```sh
#!/bin/sh
# Auto-generated by VPN Director self-update
# Do not edit manually

set -e

CHAT_ID={{.ChatID}}
INITIATOR="{{.Initiator}}"
OLD_VERSION="{{.OldVersion}}"
NEW_VERSION="{{.NewVersion}}"
UPDATE_DIR="{{.UpdateDir}}"
FILES_DIR="{{.FilesDir}}"
NOTIFY_FILE="{{.NotifyFile}}"
LOCK_FILE="{{.LockFile}}"
LOG_FILE="$UPDATE_DIR/update.log"
INIT_DIR="/opt/etc/init.d"

# Daemon table: "name|binary|init script" entries separated by spaces. The
# loops below rely on word splitting, so no field may contain a space.
DAEMONS="{{range $i, $d := .Daemons}}{{if $i}} {{end}}{{$d.Name}}|{{$d.Binary}}|{{$d.InitScript}}{{end}}"

# Init scripts of the daemons that were running when the update started. The
# EXIT trap reads it, so it must exist before anything can fail.
RUNNING_INITS=""

log() {
    echo "$(date '+%Y-%m-%d %H:%M:%S') $1" >> "$LOG_FILE"
}

# write_notify $1=status - the bot reads this file on its next startup.
write_notify() {
    cat > "$NOTIFY_FILE" << EOF
{"chat_id":$CHAT_ID,"old_version":"$OLD_VERSION","new_version":"$NEW_VERSION","status":"$1","initiator":"$INITIATOR"}
EOF
}

# Start back the daemons that were running before the update. A daemon that
# was stopped beforehand stays stopped.
start_running() {
    for init in $RUNNING_INITS; do
        log "Starting $init"
        "$INIT_DIR/$init" start || log "WARNING: $init start failed"
    done
}

# ash has no ERR trap, so recovery hangs off EXIT and inspects the code.
# set +e keeps a failing recovery step from cutting the recovery short.
on_exit() {
    code=$?
    set +e
    trap - EXIT
    if [ "$code" -eq 0 ]; then
        exit 0
    fi
    log "ERROR: update failed with exit code $code"
    if command -v monit >/dev/null 2>&1; then
        for entry in $DAEMONS; do
            monit monitor "${entry%%|*}" 2>/dev/null || true
        done
    fi
    start_running
    write_notify failed
    rm -f "$LOCK_FILE"
    exit "$code"
}
trap on_exit EXIT

log "Starting update from $OLD_VERSION to $NEW_VERSION (initiator: $INITIATOR)"

# 1. Remember which daemons are running. Matching the full binary path keeps
#    pgrep off unrelated processes.
for entry in $DAEMONS; do
    name="${entry%%|*}"
    rest="${entry#*|}"
    bin="${rest%%|*}"
    init="${rest##*|}"
    if pgrep -f "$bin" >/dev/null 2>&1; then
        RUNNING_INITS="$RUNNING_INITS $init"
        log "$name is running"
    else
        log "$name is not running, it stays stopped after the update"
    fi
done

# 2. Unmonitor in monit (if available) - optional, continue on failure
if command -v monit >/dev/null 2>&1; then
    for entry in $DAEMONS; do
        log "Unmonitoring ${entry%%|*} in monit"
        monit unmonitor "${entry%%|*}" 2>/dev/null || true
    done
fi

# 3. Stop the running daemons and wait for them to exit (max 30 seconds each)
for entry in $DAEMONS; do
    name="${entry%%|*}"
    rest="${entry#*|}"
    bin="${rest%%|*}"
    init="${rest##*|}"
    if ! pgrep -f "$bin" >/dev/null 2>&1; then
        continue
    fi
    log "Stopping $name"
    "$INIT_DIR/$init" stop || log "WARNING: $init stop returned non-zero"
    count=0
    while pgrep -f "$bin" >/dev/null 2>&1 && [ $count -lt 30 ]; do
        sleep 1
        count=$((count + 1))
    done
    if pgrep -f "$bin" >/dev/null 2>&1; then
        log "WARNING: $name still running after 30s, killing"
        # || true handles the race: the process may exit between pgrep and pkill
        pkill -9 -f "$bin" || true
        sleep 1
    fi
done

# 4. Copy files - NO || true, fail on error
log "Copying files"
cp -f "$FILES_DIR/opt/vpn-director/"*.sh /opt/vpn-director/
cp -f "$FILES_DIR/opt/vpn-director/lib/"*.sh /opt/vpn-director/lib/
cp -f "$FILES_DIR/opt/vpn-director/"*.template /opt/vpn-director/
cp -f "$FILES_DIR/opt/etc/xray/"*.template /opt/etc/xray/
cp -f "$FILES_DIR/opt/etc/init.d/"* "$INIT_DIR/"
cp -f "$FILES_DIR/jffs/scripts/"* /jffs/scripts/
for entry in $DAEMONS; do
    rest="${entry#*|}"
    cp -f "$FILES_DIR/${entry%%|*}" "${rest%%|*}"
done

# 5. Set permissions
chmod +x /opt/vpn-director/*.sh
chmod +x /opt/vpn-director/lib/*.sh
chmod +x /jffs/scripts/firewall-start
chmod +x /jffs/scripts/wan-event
chmod +x "$INIT_DIR/S99vpn-director"
for entry in $DAEMONS; do
    rest="${entry#*|}"
    chmod +x "$INIT_DIR/${entry##*|}"
    chmod +x "${rest%%|*}"
done

# 6. Create notify file
log "Creating notify file"
write_notify ok

# 7. Remove lock
rm -f "$LOCK_FILE"

# 8. Re-monitor in monit (if available) - optional, continue on failure
if command -v monit >/dev/null 2>&1; then
    for entry in $DAEMONS; do
        log "Re-monitoring ${entry%%|*} in monit"
        monit monitor "${entry%%|*}" 2>/dev/null || true
    done
fi

# 9. Start the daemons that were running before the update
start_running

# 10. Cleanup deferred to the bot after a successful startup notification
# (leave $UPDATE_DIR for notify.json and update.log)
log "Update script completed"
```

- [ ] **Step 4: Поправить устаревшие проверки в старых тестах**

В `TestGenerateScript` (Task 2 его уже трогала) заменить строки списка `checks`, которые ссылаются на исчезнувшие конструкции:

```go
	checks := []string{
		"CHAT_ID=123456789",
		`OLD_VERSION="v1.0.0"`,
		`NEW_VERSION="v1.1.0"`,
		"set -e",
		`pgrep -f "$bin"`, // full binary path, from the daemon table
		"telegram-bot|/opt/vpn-director/telegram-bot|S98telegram-bot",
		"webui|/opt/vpn-director/webui|S98vpn-director-webui",
	}
```

и заменить две проверки monit (они больше не именуют демона, а идут по таблице):

```go
	// monit commands stay optional
	if !strings.Contains(script, `monit unmonitor "${entry%%|*}" 2>/dev/null || true`) {
		t.Error("Script missing || true for monit unmonitor")
	}
	if !strings.Contains(script, `monit monitor "${entry%%|*}" 2>/dev/null || true`) {
		t.Error("Script missing || true for monit monitor")
	}
```

Проверка «cp без `|| true`» остаётся как есть.

- [ ] **Step 5: Сгенерировать golden-файл и прочитать его глазами**

Через `claude-forge:build`: `cd server && UPDATE_GOLDEN=1 go test ./internal/updater/ -run TestGenerateScript_Golden -count=1 -v`
Затем прочитать `server/internal/updater/testdata/update_script.golden.sh` целиком и убедиться, что: таблица `DAEMONS` содержит обе записи через пробел; в теле нет ни одного жёстко зашитого `S98telegram-bot`, кроме как в самой таблице; `INIT_DIR` подставляется везде; heredoc `notify.json` однострочный.

- [ ] **Step 6: Запустить тесты**

Через `claude-forge:build`: `cd server && go test ./internal/updater/ -count=1 -v`
Ожидание: PASS, включая `TestGenerateScript_IsValidShell` (не SKIP на Linux) и `TestGenerateScript_Golden`.

- [ ] **Step 7: Коммит**

```bash
git add server/internal/updater/update_script.sh.tmpl server/internal/updater/script_test.go server/internal/updater/testdata/update_script.golden.sh
git commit -m "feat(updater): update every daemon from one table and recover on failure" -m "Claude-Session: <URL текущей сессии>"
```

---

### Task 4: пакет `updateflow` — проверка обновлений с кешем

**Files:**
- Create: `server/internal/updateflow/flow.go`
- Test: `server/internal/updateflow/flow_test.go`

**Interfaces:**
- Consumes: `updater.Updater`, `updater.Release` (Tasks 1–2).
- Produces: `updateflow.Flow`; `New(upd updater.Updater, currentVersion string, devMode bool) *Flow`; `CheckResult{Current, Latest string; UpdateAvailable bool; Changelog string; CheckedAt time.Time}`; `(*Flow).Check(ctx context.Context, force bool) (CheckResult, error)`; `(*Flow).InProgress() bool`; ошибки `ErrDevMode`, `ErrDevVersion`, `ErrInProgress`, `ErrUpToDate`, тип `GitHubError`; тестовый `mockUpdater`.

- [ ] **Step 1: Написать падающие тесты**

Создать `server/internal/updateflow/flow_test.go`:

```go
package updateflow

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/updater"
)

// mockUpdater implements updater.Updater for testing.
type mockUpdater struct {
	mu sync.Mutex

	release    *updater.Release
	releaseErr error
	releases   int // GetLatestRelease call count

	shouldUpdate    bool
	shouldUpdateErr error

	inProgress    bool
	createLockErr error
	downloadErr   error
	runScriptErr  error

	createLockCalled bool
	cleanFilesCalled bool
	removeLockCalled bool
	runScriptCalled  bool
	runScriptOpts    updater.RunOptions
}

func newMockUpdater() *mockUpdater {
	return &mockUpdater{
		release:      &updater.Release{TagName: "v1.3.0", Body: "changelog text"},
		shouldUpdate: true,
	}
}

func (m *mockUpdater) GetLatestRelease(_ context.Context) (*updater.Release, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.releases++
	if m.releaseErr != nil {
		return nil, m.releaseErr
	}
	return m.release, nil
}

func (m *mockUpdater) ShouldUpdate(_, _ string) (bool, error) {
	return m.shouldUpdate, m.shouldUpdateErr
}

func (m *mockUpdater) IsUpdateInProgress() bool { return m.inProgress }

func (m *mockUpdater) CreateLock() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createLockCalled = true
	return m.createLockErr
}

func (m *mockUpdater) RemoveLock() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removeLockCalled = true
}

func (m *mockUpdater) CleanFiles() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanFilesCalled = true
}

func (m *mockUpdater) DownloadRelease(_ context.Context, _ *updater.Release) error {
	return m.downloadErr
}

func (m *mockUpdater) RunUpdateScript(opts updater.RunOptions) error {
	m.mu.Lock()
	m.runScriptCalled = true
	m.runScriptOpts = opts
	err := m.runScriptErr
	m.mu.Unlock()
	return err
}

func (m *mockUpdater) calls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.releases
}

func TestCheck_ReturnsRelease(t *testing.T) {
	upd := newMockUpdater()
	f := New(upd, "v1.2.0", false)

	res, err := f.Check(context.Background(), false)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if res.Current != "v1.2.0" || res.Latest != "v1.3.0" {
		t.Errorf("Check() = %+v, want current v1.2.0 latest v1.3.0", res)
	}
	if !res.UpdateAvailable {
		t.Error("UpdateAvailable must follow ShouldUpdate")
	}
	if res.Changelog != "changelog text" {
		t.Errorf("Changelog = %q", res.Changelog)
	}
	if res.CheckedAt.IsZero() {
		t.Error("CheckedAt must be stamped")
	}
}

func TestCheck_UsesCacheWithinTTL(t *testing.T) {
	// GitHub allows 60 unauthenticated requests an hour; every SPA load calls
	// this endpoint for the header banner.
	upd := newMockUpdater()
	f := New(upd, "v1.2.0", false)

	for i := 0; i < 5; i++ {
		if _, err := f.Check(context.Background(), false); err != nil {
			t.Fatalf("Check() error = %v", err)
		}
	}
	if upd.calls() != 1 {
		t.Errorf("GetLatestRelease called %d times, want 1", upd.calls())
	}
}

func TestCheck_ForceBypassesCacheOncePerMinute(t *testing.T) {
	upd := newMockUpdater()
	f := New(upd, "v1.2.0", false)

	if _, err := f.Check(context.Background(), true); err != nil {
		t.Fatalf("first Check() error = %v", err)
	}
	if _, err := f.Check(context.Background(), true); err != nil {
		t.Fatalf("second Check() error = %v", err)
	}
	if upd.calls() != 1 {
		t.Errorf("a second force within a minute must be served from cache, got %d calls", upd.calls())
	}

	// Move the throttle window into the past; the cache is still fresh, so
	// only force may pierce it.
	f.mu.Lock()
	f.lastForce = time.Now().Add(-2 * time.Minute)
	f.mu.Unlock()

	if _, err := f.Check(context.Background(), true); err != nil {
		t.Fatalf("third Check() error = %v", err)
	}
	if upd.calls() != 2 {
		t.Errorf("force after the cooldown must hit GitHub, got %d calls", upd.calls())
	}
}

func TestCheck_RefreshesAfterTTL(t *testing.T) {
	upd := newMockUpdater()
	f := New(upd, "v1.2.0", false)

	if _, err := f.Check(context.Background(), false); err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	f.mu.Lock()
	f.cachedAt = time.Now().Add(-31 * time.Minute)
	f.mu.Unlock()

	if _, err := f.Check(context.Background(), false); err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if upd.calls() != 2 {
		t.Errorf("a stale cache must be refreshed, got %d calls", upd.calls())
	}
}

func TestCheck_DevModeAndDevVersion(t *testing.T) {
	tests := []struct {
		name    string
		version string
		devMode bool
		want    error
	}{
		{name: "dev mode", version: "v1.2.0", devMode: true, want: ErrDevMode},
		{name: "dev build", version: "dev", devMode: false, want: ErrDevVersion},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upd := newMockUpdater()
			f := New(upd, tt.version, tt.devMode)

			_, err := f.Check(context.Background(), false)
			if !errors.Is(err, tt.want) {
				t.Fatalf("Check() error = %v, want %v", err, tt.want)
			}
			if upd.calls() != 0 {
				t.Error("a dev build must not reach GitHub at all")
			}
		})
	}
}

func TestCheck_GitHubFailureIsTyped(t *testing.T) {
	upd := newMockUpdater()
	upd.releaseErr = errors.New("fetch release: connection refused")
	f := New(upd, "v1.2.0", false)

	_, err := f.Check(context.Background(), false)
	var ghErr *GitHubError
	if !errors.As(err, &ghErr) {
		t.Fatalf("Check() error = %v, want *GitHubError", err)
	}
	if ghErr.Err.Error() != "fetch release: connection refused" {
		t.Errorf("wrapped cause = %v", ghErr.Err)
	}
}

func TestCheck_FailureDoesNotPoisonCache(t *testing.T) {
	upd := newMockUpdater()
	upd.releaseErr = errors.New("boom")
	f := New(upd, "v1.2.0", false)

	if _, err := f.Check(context.Background(), false); err == nil {
		t.Fatal("Check() must fail")
	}
	upd.releaseErr = nil
	res, err := f.Check(context.Background(), false)
	if err != nil {
		t.Fatalf("second Check() error = %v", err)
	}
	if res.Latest != "v1.3.0" {
		t.Errorf("a failed check must not be cached, got %+v", res)
	}
}

func TestInProgress_DelegatesToUpdater(t *testing.T) {
	upd := newMockUpdater()
	f := New(upd, "v1.2.0", false)

	if f.InProgress() {
		t.Error("InProgress() = true with no lock")
	}
	upd.inProgress = true
	if !f.InProgress() {
		t.Error("InProgress() must follow the updater's lock file")
	}
}
```

- [ ] **Step 2: Запустить и убедиться, что тесты падают**

Через `claude-forge:build`: `cd server && go test ./internal/updateflow/ -count=1`
Ожидание: FAIL — пакета `updateflow` ещё нет.

- [ ] **Step 3: Реализовать `flow.go`**

Создать `server/internal/updateflow/flow.go`:

```go
// Package updateflow owns the self-update orchestration shared by the
// Telegram bot and the Web UI: one cached check against the GitHub release
// API and one guarded start of the download plus the update script.
package updateflow

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/updater"
)

// Cache windows for Check. GitHub allows 60 unauthenticated requests an hour
// and the Web UI asks on every page load, so an uncached check is a rate
// limit waiting to happen.
const (
	cacheTTL      = 30 * time.Minute
	forceCooldown = time.Minute
)

// Errors a caller is expected to branch on. Everything else (a lock that
// cannot be created, a version string the shell would choke on) comes back
// as a plain error.
var (
	ErrDevMode    = errors.New("update is not available in dev mode")
	ErrDevVersion = errors.New("cannot check updates for a dev build")
	ErrInProgress = errors.New("update is already in progress")
	ErrUpToDate   = errors.New("already running the latest version")
)

// GitHubError marks a failure to reach the release API, so a caller can tell
// "GitHub is down" (502, retry later) from "this build cannot update".
type GitHubError struct{ Err error }

func (e *GitHubError) Error() string { return "check for updates: " + e.Err.Error() }
func (e *GitHubError) Unwrap() error { return e.Err }

// CheckResult describes the latest release relative to the running build.
type CheckResult struct {
	Current         string    `json:"current"`
	Latest          string    `json:"latest"`
	UpdateAvailable bool      `json:"update_available"`
	Changelog       string    `json:"changelog"`
	CheckedAt       time.Time `json:"checked_at"`
}

// Flow orchestrates update checks and runs. One instance per daemon.
type Flow struct {
	upd            updater.Updater
	currentVersion string
	devMode        bool

	mu            sync.Mutex
	cached        CheckResult
	cachedRelease *updater.Release
	cachedAt      time.Time
	lastForce     time.Time
}

// New creates a Flow for the given updater and running version.
func New(upd updater.Updater, currentVersion string, devMode bool) *Flow {
	return &Flow{upd: upd, currentVersion: currentVersion, devMode: devMode}
}

// InProgress reports whether an update script is running, by the lock file.
func (f *Flow) InProgress() bool { return f.upd.IsUpdateInProgress() }

// Check returns the cached result when it is younger than cacheTTL. force
// bypasses the cache, but not more often than once every forceCooldown; a
// throttled force is served from the cache rather than rejected.
func (f *Flow) Check(ctx context.Context, force bool) (CheckResult, error) {
	if f.devMode {
		return CheckResult{}, ErrDevMode
	}
	if f.currentVersion == "dev" {
		return CheckResult{}, ErrDevVersion
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	now := time.Now()
	if !f.cachedAt.IsZero() {
		if force && now.Sub(f.lastForce) < forceCooldown {
			return f.cached, nil
		}
		if !force && now.Sub(f.cachedAt) < cacheTTL {
			return f.cached, nil
		}
	}
	if force {
		f.lastForce = now
	}

	release, err := f.upd.GetLatestRelease(ctx)
	if err != nil {
		return CheckResult{}, &GitHubError{Err: err}
	}

	available, err := f.upd.ShouldUpdate(f.currentVersion, release.TagName)
	if err != nil {
		return CheckResult{}, fmt.Errorf("compare versions: %w", err)
	}

	res := CheckResult{
		Current:         f.currentVersion,
		Latest:          release.TagName,
		UpdateAvailable: available,
		Changelog:       release.Body,
		CheckedAt:       now,
	}
	f.cached, f.cachedRelease, f.cachedAt = res, release, now
	return res, nil
}

// latestRelease returns the release behind the last successful Check.
func (f *Flow) latestRelease() *updater.Release {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.cachedRelease
}
```

- [ ] **Step 4: Запустить тесты**

Через `claude-forge:build`: `cd server && go test ./internal/updateflow/ -count=1 -v`
Ожидание: PASS все девять тестов.

- [ ] **Step 5: Проверить сборку, vet и формат**

Через `claude-forge:build`: `cd server && go build ./... && go vet ./... && gofmt -l internal/updateflow/`
Ожидание: чисто. (`latestRelease` пока не используется — `go vet` на неиспользуемые методы не ругается; её потребитель появится в Task 5.)

- [ ] **Step 6: Коммит**

```bash
git add server/internal/updateflow/flow.go server/internal/updateflow/flow_test.go
git commit -m "feat(updateflow): add a cached release check shared by both daemons" -m "Claude-Session: <URL текущей сессии>"
```

---

### Task 5: `updateflow.Start` — пре-флайт, лок, фоновое обновление

**Files:**
- Create: `server/internal/updateflow/start.go`
- Test: `server/internal/updateflow/start_test.go`

**Interfaces:**
- Consumes: `Flow`, `Check`, `latestRelease`, ошибки из Task 4; `updater.RunOptions`, `updater.IsValidVersion` (Tasks 1–2).
- Produces: `StartResult{From, To string}`; `(*Flow).Start(ctx context.Context, initiator string, chatID int64, progress func(string)) (StartResult, error)`.

- [ ] **Step 1: Написать падающие тесты**

Создать `server/internal/updateflow/start_test.go`:

```go
package updateflow

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// collectProgress returns a progress func and a getter for what it received.
func collectProgress() (func(string), func() []string) {
	var mu sync.Mutex
	var lines []string
	return func(s string) {
			mu.Lock()
			lines = append(lines, s)
			mu.Unlock()
		}, func() []string {
			mu.Lock()
			defer mu.Unlock()
			return append([]string(nil), lines...)
		}
}

// waitFor polls cond for up to a second: Start hands the download to a
// goroutine, so the assertions have to wait for it.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestStart_RunsTheScriptWithTheInitiator(t *testing.T) {
	upd := newMockUpdater()
	f := New(upd, "v1.2.0", false)
	progress, lines := collectProgress()

	res, err := f.Start(context.Background(), "webui", 0, progress)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if res.From != "v1.2.0" || res.To != "v1.3.0" {
		t.Errorf("StartResult = %+v, want v1.2.0 -> v1.3.0", res)
	}

	waitFor(t, "the update script to start", func() bool {
		upd.mu.Lock()
		defer upd.mu.Unlock()
		return upd.runScriptCalled
	})

	upd.mu.Lock()
	opts := upd.runScriptOpts
	upd.mu.Unlock()
	if opts.Initiator != "webui" || opts.ChatID != 0 {
		t.Errorf("RunOptions = %+v, want initiator webui and chat 0", opts)
	}
	if opts.OldVersion != "v1.2.0" || opts.NewVersion != "v1.3.0" {
		t.Errorf("RunOptions versions = %+v", opts)
	}

	got := strings.Join(lines(), "\n")
	if !strings.Contains(got, "Files downloaded, starting update...") {
		t.Errorf("progress missing the download line: %q", got)
	}
}

func TestStart_UpToDateKeepsTheVersionsAndTakesNoLock(t *testing.T) {
	upd := newMockUpdater()
	upd.shouldUpdate = false
	f := New(upd, "v1.3.0", false)
	progress, _ := collectProgress()

	res, err := f.Start(context.Background(), "bot", 42, progress)
	if !errors.Is(err, ErrUpToDate) {
		t.Fatalf("Start() error = %v, want ErrUpToDate", err)
	}
	if res.From != "v1.3.0" || res.To != "v1.3.0" {
		t.Errorf("StartResult = %+v: the caller needs the versions for its message", res)
	}
	upd.mu.Lock()
	defer upd.mu.Unlock()
	if upd.runScriptCalled {
		t.Error("an up-to-date router must not run the script")
	}
}

func TestStart_RejectsWithoutTouchingAnything(t *testing.T) {
	tests := []struct {
		name       string
		version    string
		devMode    bool
		inProgress bool
		want       error
	}{
		{name: "dev mode", version: "v1.2.0", devMode: true, want: ErrDevMode},
		{name: "dev build", version: "dev", want: ErrDevVersion},
		{name: "already running", version: "v1.2.0", inProgress: true, want: ErrInProgress},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upd := newMockUpdater()
			upd.inProgress = tt.inProgress
			f := New(upd, tt.version, tt.devMode)
			progress, _ := collectProgress()

			if _, err := f.Start(context.Background(), "bot", 42, progress); !errors.Is(err, tt.want) {
				t.Fatalf("Start() error = %v, want %v", err, tt.want)
			}
			upd.mu.Lock()
			defer upd.mu.Unlock()
			if upd.runScriptCalled {
				t.Error("rejected start must not run the script")
			}
		})
	}
}

func TestStart_GitHubFailureIsTyped(t *testing.T) {
	upd := newMockUpdater()
	upd.releaseErr = errors.New("connection refused")
	f := New(upd, "v1.2.0", false)
	progress, _ := collectProgress()

	_, err := f.Start(context.Background(), "bot", 42, progress)
	var ghErr *GitHubError
	if !errors.As(err, &ghErr) {
		t.Fatalf("Start() error = %v, want *GitHubError", err)
	}
}

func TestStart_DownloadFailureCleansUp(t *testing.T) {
	// A half-downloaded release plus a stale lock would block every later
	// attempt, so the goroutine has to unwind both.
	upd := newMockUpdater()
	upd.downloadErr = errors.New("HTTP 404")
	f := New(upd, "v1.2.0", false)
	progress, lines := collectProgress()

	if _, err := f.Start(context.Background(), "bot", 42, progress); err != nil {
		t.Fatalf("Start() error = %v: the download failure is reported through progress", err)
	}

	waitFor(t, "cleanup after a failed download", func() bool {
		upd.mu.Lock()
		defer upd.mu.Unlock()
		return upd.cleanFilesCalled && upd.removeLockCalled
	})
	upd.mu.Lock()
	defer upd.mu.Unlock()
	if upd.runScriptCalled {
		t.Error("a failed download must not run the script")
	}
	if got := strings.Join(lines(), "\n"); !strings.Contains(got, "Download failed: HTTP 404") {
		t.Errorf("progress missing the failure line: %q", got)
	}
}

func TestStart_ScriptFailureCleansUp(t *testing.T) {
	upd := newMockUpdater()
	upd.runScriptErr = errors.New("start script: permission denied")
	f := New(upd, "v1.2.0", false)
	progress, lines := collectProgress()

	if _, err := f.Start(context.Background(), "bot", 42, progress); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	waitFor(t, "cleanup after a failed script", func() bool {
		upd.mu.Lock()
		defer upd.mu.Unlock()
		return upd.cleanFilesCalled && upd.removeLockCalled
	})
	if got := strings.Join(lines(), "\n"); !strings.Contains(got, "Failed to run update script") {
		t.Errorf("progress missing the failure line: %q", got)
	}
}

func TestStart_LockFailureIsReportedSynchronously(t *testing.T) {
	upd := newMockUpdater()
	upd.createLockErr = errors.New("lock file already exists (update in progress)")
	f := New(upd, "v1.2.0", false)
	progress, _ := collectProgress()

	_, err := f.Start(context.Background(), "bot", 42, progress)
	if err == nil || !strings.Contains(err.Error(), "lock file already exists") {
		t.Fatalf("Start() error = %v, want the lock failure", err)
	}
	upd.mu.Lock()
	defer upd.mu.Unlock()
	if upd.runScriptCalled {
		t.Error("no lock, no update")
	}
}

func TestStart_RejectsUnsafeInitiator(t *testing.T) {
	upd := newMockUpdater()
	f := New(upd, "v1.2.0", false)
	progress, _ := collectProgress()

	if _, err := f.Start(context.Background(), "cron", 42, progress); err == nil {
		t.Fatal("Start() must reject an unknown initiator before taking the lock")
	}
	upd.mu.Lock()
	defer upd.mu.Unlock()
	if upd.createLockCalled {
		t.Error("a rejected initiator must not have taken the lock")
	}
}
```

- [ ] **Step 2: Запустить и убедиться, что тесты падают**

Через `claude-forge:build`: `cd server && go test ./internal/updateflow/ -count=1`
Ожидание: FAIL — `f.Start undefined`.

- [ ] **Step 3: Реализовать `start.go`**

Создать `server/internal/updateflow/start.go`:

```go
package updateflow

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/updater"
)

// StartResult names the versions of a run, filled in even when Start refuses
// with ErrUpToDate so the caller can put them in its message.
type StartResult struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// Start runs the pre-flight checks synchronously, takes the lock and launches
// the download plus the update script in a goroutine; progress receives
// human-readable status lines. It returns as soon as the background work is
// under way, because the script kills this very process a few seconds later.
func (f *Flow) Start(ctx context.Context, initiator string, chatID int64, progress func(string)) (StartResult, error) {
	if f.devMode {
		return StartResult{}, ErrDevMode
	}
	if f.currentVersion == "dev" {
		return StartResult{}, ErrDevVersion
	}
	if initiator != "bot" && initiator != "webui" {
		return StartResult{}, fmt.Errorf("invalid initiator: %q", initiator)
	}
	if f.upd.IsUpdateInProgress() {
		return StartResult{}, ErrInProgress
	}

	check, err := f.Check(ctx, false)
	if err != nil {
		return StartResult{}, err
	}

	res := StartResult{From: check.Current, To: check.Latest}
	if !check.UpdateAvailable {
		return res, ErrUpToDate
	}

	// Both strings end up inside the generated shell script.
	if !updater.IsValidVersion(check.Current) {
		return res, fmt.Errorf("invalid current version: %s", check.Current)
	}
	if !updater.IsValidVersion(check.Latest) {
		return res, fmt.Errorf("invalid release version: %s", check.Latest)
	}

	release := f.latestRelease()
	if release == nil {
		return res, fmt.Errorf("no release information available")
	}

	if err := f.upd.CreateLock(); err != nil {
		return res, err
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("update goroutine panicked", "error", r)
				progress("Update failed unexpectedly. Check logs.")
				f.upd.CleanFiles()
				f.upd.RemoveLock()
			}
		}()
		f.run(release, res, initiator, chatID, progress)
	}()

	return res, nil
}

// run downloads the release and hands over to the update script. It builds
// its own context: a closed browser tab or a finished Telegram poll must not
// abort an update that is already downloading.
func (f *Flow) run(release *updater.Release, res StartResult, initiator string, chatID int64, progress func(string)) {
	if err := f.upd.DownloadRelease(context.Background(), release); err != nil {
		f.upd.CleanFiles()
		f.upd.RemoveLock()
		progress(fmt.Sprintf("Download failed: %v", err))
		return
	}
	progress("Files downloaded, starting update...")

	err := f.upd.RunUpdateScript(updater.RunOptions{
		OldVersion: res.From,
		NewVersion: res.To,
		ChatID:     chatID,
		Initiator:  initiator,
	})
	if err != nil {
		f.upd.CleanFiles()
		f.upd.RemoveLock()
		progress(fmt.Sprintf("Failed to run update script: %v", err))
		return
	}

	progress("Update script started, the service will restart in a few seconds...")
}
```

- [ ] **Step 4: Запустить тесты**

Через `claude-forge:build`: `cd server && go test ./internal/updateflow/ -count=1 -v`
Ожидание: PASS.

- [ ] **Step 5: Проверить гонки**

Через `claude-forge:build`: `cd server && go test ./internal/updateflow/ -count=1 -race`
Ожидание: PASS без предупреждений детектора гонок (`Start` возвращается, пока горутина работает — тесты обязаны быть чистыми).

- [ ] **Step 6: Проверить сборку, vet и формат**

Через `claude-forge:build`: `cd server && go build ./... && go vet ./... && gofmt -l internal/updateflow/`
Ожидание: чисто.

- [ ] **Step 7: Коммит**

```bash
git add server/internal/updateflow/start.go server/internal/updateflow/start_test.go
git commit -m "feat(updateflow): start an update behind pre-flight checks and a lock" -m "Claude-Session: <URL текущей сессии>"
```

---

### Task 6: `/update` бота становится адаптером над `Flow`

**Files:**
- Rewrite: `server/internal/handler/update.go`
- Modify: `server/internal/handler/handler.go` (удалить мёртвое поле `Updater`)
- Modify: `server/internal/bot/bot.go` (создать `Flow`, передать его в обработчик, убрать `Updater:` из `handler.Deps`)
- Test: `server/internal/handler/update_test.go` (переписать на мок `UpdateFlow`)

**Interfaces:**
- Consumes: `updateflow.Flow`, `StartResult`, `ErrDevMode`, `ErrDevVersion`, `ErrInProgress`, `ErrUpToDate`, `GitHubError` (Tasks 4–5).
- Produces: `handler.UpdateFlow` (интерфейс-потребитель); `handler.NewUpdateHandler(sender telegram.MessageSender, flow UpdateFlow, version string) *UpdateHandler`.

`version` остаётся у обработчика только ради текста «Already running the latest version: %s» — брать его из `StartResult.From` нельзя, когда `Start` отказал до `Check` (dev-режим), а сообщение про последнюю версию печатается именно из `res.From`; для единообразия обработчик хранит обе величины и предпочитает `res.From`, если он непуст.

- [ ] **Step 1: Написать падающие тесты**

Переписать `server/internal/handler/update_test.go`: удалить `mockUpdater` целиком (он больше не нужен — `Updater` в обработчик не приходит) и заменить тесты на таблицу по ошибкам. Новый файл:

```go
package handler

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/updateflow"
)

// mockFlow implements UpdateFlow for testing.
type mockFlow struct {
	mu sync.Mutex

	res       updateflow.StartResult
	err       error
	progress  []string
	started   bool
	initiator string
	chatID    int64
}

func (m *mockFlow) Start(_ context.Context, initiator string, chatID int64, progress func(string)) (updateflow.StartResult, error) {
	m.mu.Lock()
	m.started = true
	m.initiator = initiator
	m.chatID = chatID
	lines := m.progress
	res, err := m.res, m.err
	m.mu.Unlock()

	for _, line := range lines {
		progress(line)
	}
	return res, err
}

// mockUpdateSender implements telegram.MessageSender for testing.
type mockUpdateSender struct {
	mu       sync.Mutex
	messages []string
}

func (m *mockUpdateSender) Send(_ int64, text string) error      { return m.record(text) }
func (m *mockUpdateSender) SendPlain(_ int64, text string) error { return m.record(text) }
func (m *mockUpdateSender) SendLongPlain(_ int64, text string) error {
	return m.record(text)
}

func (m *mockUpdateSender) SendWithKeyboard(_ int64, text string, _ tgbotapi.InlineKeyboardMarkup) error {
	return m.record(text)
}
func (m *mockUpdateSender) SendCodeBlock(_ int64, _, content string) error { return m.record(content) }
func (m *mockUpdateSender) EditMessage(_ int64, _ int, text string, _ tgbotapi.InlineKeyboardMarkup) error {
	return m.record(text)
}
func (m *mockUpdateSender) AckCallback(_ string) error { return nil }

func (m *mockUpdateSender) record(text string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, text)
	return nil
}

func (m *mockUpdateSender) all() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return strings.Join(m.messages, "\n")
}

func testMessage() *tgbotapi.Message {
	return &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 42}}
}

func TestUpdateHandler_MessagePerOutcome(t *testing.T) {
	tests := []struct {
		name string
		res  updateflow.StartResult
		err  error
		want string
	}{
		{
			name: "started",
			res:  updateflow.StartResult{From: "v1.2.0", To: "v1.3.0"},
			want: "Starting update v1.2.0 → v1.3.0...",
		},
		{
			name: "dev mode",
			err:  updateflow.ErrDevMode,
			want: "Command /update is not available in dev mode",
		},
		{
			name: "dev build",
			err:  updateflow.ErrDevVersion,
			want: "Cannot check updates for dev build",
		},
		{
			name: "in progress",
			err:  updateflow.ErrInProgress,
			want: "Update is already in progress, please wait...",
		},
		{
			name: "up to date",
			res:  updateflow.StartResult{From: "v1.3.0", To: "v1.3.0"},
			err:  updateflow.ErrUpToDate,
			want: "Already running the latest version: v1.3.0",
		},
		{
			name: "github down",
			err:  &updateflow.GitHubError{Err: errors.New("connection refused")},
			want: "Failed to check for updates: connection refused",
		},
		{
			name: "anything else",
			err:  errors.New("invalid release version: vX"),
			want: "Failed to start update: invalid release version: vX",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sender := &mockUpdateSender{}
			flow := &mockFlow{res: tt.res, err: tt.err}
			h := NewUpdateHandler(sender, flow, "v1.2.0")

			h.HandleUpdate(testMessage())

			if got := sender.all(); !strings.Contains(got, tt.want) {
				t.Errorf("messages = %q, want to contain %q", got, tt.want)
			}
		})
	}
}

func TestUpdateHandler_ForwardsProgressToTheChat(t *testing.T) {
	sender := &mockUpdateSender{}
	flow := &mockFlow{
		res:      updateflow.StartResult{From: "v1.2.0", To: "v1.3.0"},
		progress: []string{"Files downloaded, starting update..."},
	}
	h := NewUpdateHandler(sender, flow, "v1.2.0")

	h.HandleUpdate(testMessage())

	if got := sender.all(); !strings.Contains(got, "Files downloaded, starting update...") {
		t.Errorf("progress not forwarded: %q", got)
	}
}

func TestUpdateHandler_StartsAsBotWithTheChatID(t *testing.T) {
	sender := &mockUpdateSender{}
	flow := &mockFlow{res: updateflow.StartResult{From: "v1.2.0", To: "v1.3.0"}}
	h := NewUpdateHandler(sender, flow, "v1.2.0")

	h.HandleUpdate(testMessage())

	flow.mu.Lock()
	defer flow.mu.Unlock()
	if flow.initiator != "bot" || flow.chatID != 42 {
		t.Errorf("Start(initiator=%q, chatID=%d), want bot/42", flow.initiator, flow.chatID)
	}
}

func TestUpdateHandler_HandleCallback_UpdateRun(t *testing.T) {
	sender := &mockUpdateSender{}
	flow := &mockFlow{res: updateflow.StartResult{From: "v1.2.0", To: "v1.3.0"}}
	h := NewUpdateHandler(sender, flow, "v1.2.0")

	h.HandleCallback(&tgbotapi.CallbackQuery{
		Data:    "update:run",
		Message: &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 42}},
		From:    &tgbotapi.User{UserName: "admin"},
	})

	flow.mu.Lock()
	defer flow.mu.Unlock()
	if !flow.started {
		t.Error("the update:run button must start an update")
	}
}

func TestUpdateHandler_HandleCallback_IgnoresOtherCallbacks(t *testing.T) {
	sender := &mockUpdateSender{}
	flow := &mockFlow{}
	h := NewUpdateHandler(sender, flow, "v1.2.0")

	h.HandleCallback(&tgbotapi.CallbackQuery{Data: "clients:add"})

	flow.mu.Lock()
	defer flow.mu.Unlock()
	if flow.started {
		t.Error("an unrelated callback must not start an update")
	}
}

func TestUpdateHandler_HandleCallback_WithoutMessage(t *testing.T) {
	sender := &mockUpdateSender{}
	flow := &mockFlow{}
	h := NewUpdateHandler(sender, flow, "v1.2.0")

	h.HandleCallback(&tgbotapi.CallbackQuery{Data: "update:run"})

	flow.mu.Lock()
	defer flow.mu.Unlock()
	if flow.started {
		t.Error("an inline callback without a chat has nowhere to report to")
	}
}
```

- [ ] **Step 2: Запустить и убедиться, что тесты падают**

Через `claude-forge:build`: `cd server && go test ./internal/handler/ -run TestUpdateHandler -count=1`
Ожидание: FAIL — `NewUpdateHandler` принимает другой набор аргументов, `UpdateFlow` не определён.

- [ ] **Step 3: Переписать обработчик**

Полностью заменить `server/internal/handler/update.go`:

```go
package handler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/telegram"
	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/updateflow"
)

// UpdateFlow is the update orchestration behind /update. Declared here so the
// command's tests can drive every outcome without a fake GitHub.
type UpdateFlow interface {
	Start(ctx context.Context, initiator string, chatID int64, progress func(string)) (updateflow.StartResult, error)
}

// UpdateHandler handles the /update command. It is an adapter: every decision
// lives in updateflow, this type only turns outcomes into chat messages.
type UpdateHandler struct {
	sender  telegram.MessageSender
	flow    UpdateFlow
	version string
}

// NewUpdateHandler creates a new update handler.
func NewUpdateHandler(sender telegram.MessageSender, flow UpdateFlow, version string) *UpdateHandler {
	return &UpdateHandler{sender: sender, flow: flow, version: version}
}

// HandleUpdate processes the /update command: it starts an update of both
// daemons and reports progress into the chat it came from.
func (h *UpdateHandler) HandleUpdate(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID

	res, err := h.flow.Start(context.Background(), "bot", chatID, func(line string) {
		h.send(chatID, line)
	})

	current := res.From
	if current == "" {
		current = h.version
	}

	var ghErr *updateflow.GitHubError
	switch {
	case err == nil:
		h.send(chatID, fmt.Sprintf("Starting update %s → %s...", res.From, res.To))
	case errors.Is(err, updateflow.ErrDevMode):
		h.send(chatID, "Command /update is not available in dev mode")
	case errors.Is(err, updateflow.ErrDevVersion):
		h.send(chatID, "Cannot check updates for dev build")
	case errors.Is(err, updateflow.ErrInProgress):
		h.send(chatID, "Update is already in progress, please wait...")
	case errors.Is(err, updateflow.ErrUpToDate):
		h.send(chatID, fmt.Sprintf("Already running the latest version: %s", current))
	case errors.As(err, &ghErr):
		h.send(chatID, fmt.Sprintf("Failed to check for updates: %v", ghErr.Err))
	default:
		h.send(chatID, fmt.Sprintf("Failed to start update: %v", err))
	}
}

// HandleCallback handles update callbacks from inline buttons.
func (h *UpdateHandler) HandleCallback(cb *tgbotapi.CallbackQuery) {
	if cb.Data != "update:run" {
		return
	}
	// cb.Message can be nil for inline callbacks
	if cb.Message == nil {
		slog.Warn("Callback without message, cannot process update:run")
		return
	}
	// Create a message-like structure to reuse HandleUpdate logic
	msg := &tgbotapi.Message{
		Chat: cb.Message.Chat,
		From: cb.From,
	}
	h.HandleUpdate(msg)
}

// send sends a plain text message. Errors are logged but not returned.
func (h *UpdateHandler) send(chatID int64, text string) {
	_ = h.sender.SendPlain(chatID, text)
}
```

- [ ] **Step 4: Убрать мёртвое поле и подключить `Flow`**

В `server/internal/handler/handler.go` удалить строку 28 (`Updater updater.Updater // Update service for /update command`) **и импорт `updater`**: это поле — его единственный потребитель в файле, без удаления импорта пакет не соберётся.

В `server/internal/bot/bot.go`:
- добавить импорт `"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/updateflow"`;
- убрать `Updater: b.updater,` из литерала `handler.Deps`;
- заменить создание обработчика:

```go
	// updateflow owns every decision behind /update; the handler is an adapter.
	updateFlow := updateflow.New(b.updater, version, b.devMode)
	updateHandler := handler.NewUpdateHandler(sender, updateFlow, version)
```

- если импорт `updater` в `bot.go` остался нужен только для `WithUpdater`, оставить его.

- [ ] **Step 5: Запустить тесты**

Через `claude-forge:build`: `cd server && go test ./internal/handler/ ./internal/bot/ ./internal/updateflow/ -count=1`
Ожидание: PASS.

- [ ] **Step 6: Проверить сборку, vet и формат**

Через `claude-forge:build`: `cd server && go build ./... && go vet ./... && gofmt -l internal/handler/update.go internal/handler/handler.go internal/handler/update_test.go internal/bot/bot.go`
Ожидание: чисто.

- [ ] **Step 7: Коммит**

```bash
git add server/internal/handler/update.go server/internal/handler/handler.go server/internal/handler/update_test.go server/internal/bot/bot.go
git commit -m "refactor(bot): drive /update through updateflow" -m "Claude-Session: <URL текущей сессии>"
```

---

### Task 7: уведомление после обновления для всех активных чатов

**Files:**
- Modify: `server/internal/startup/notify.go`
- Modify: `server/internal/bot/bot.go` (передать chatstore, обойти ловушку nil-интерфейса)
- Test: `server/internal/startup/notify_test.go`

**Interfaces:**
- Consumes: формат `notify.json` со `status`/`initiator` (Task 3); `chatstore.UserChat`.
- Produces: `startup.ChatStore` (интерфейс-потребитель); `startup.CheckAndSendNotify(sender telegram.MessageSender, store ChatStore, notifyFile, updateDir string) error`; `UpdateNotification` с полями `Status`, `Initiator`.

- [ ] **Step 1: Написать падающие тесты**

В `server/internal/startup/notify_test.go` добавить мок хранилища и новые тесты, заменив `TestCheckAndSendNotify_MissingChatID` (нулевой `chat_id` больше не ошибка):

```go
// mockStore implements ChatStore for testing.
type mockStore struct {
	users []chatstore.UserChat
	err   error
}

func (m *mockStore) GetActiveUsers() ([]chatstore.UserChat, error) { return m.users, m.err }

func TestCheckAndSendNotify_ZeroChatIDGoesToEveryActiveUser(t *testing.T) {
	// A Web UI update has no chat of its own; the bot tells everyone it talks to.
	tmpDir := t.TempDir()
	notifyFile := filepath.Join(tmpDir, "notify.json")
	writeNotify(t, notifyFile, `{"chat_id":0,"old_version":"v1.2.0","new_version":"v1.3.0","status":"ok","initiator":"webui"}`)

	sender := &mockSender{}
	store := &mockStore{users: []chatstore.UserChat{
		{Username: "alice", ChatID: 111},
		{Username: "bob", ChatID: 222},
	}}

	if err := CheckAndSendNotify(sender, store, notifyFile, tmpDir); err != nil {
		t.Fatalf("CheckAndSendNotify() error = %v", err)
	}

	if len(sender.sentMessages) != 2 {
		t.Fatalf("sent %d messages, want 2", len(sender.sentMessages))
	}
	for _, msg := range sender.sentMessages {
		if msg.text != "Update complete: v1.2.0 → v1.3.0" {
			t.Errorf("text = %q", msg.text)
		}
	}
	if _, err := os.Stat(tmpDir); !os.IsNotExist(err) {
		t.Error("the update directory must be cleaned after a successful send")
	}
}

func TestCheckAndSendNotify_FailedStatusKeepsTheLog(t *testing.T) {
	// The message points at update.log, so the log has to survive the cleanup.
	tmpDir := t.TempDir()
	notifyFile := filepath.Join(tmpDir, "notify.json")
	logFile := filepath.Join(tmpDir, "update.log")
	writeNotify(t, notifyFile, `{"chat_id":42,"old_version":"v1.2.0","new_version":"v1.3.0","status":"failed","initiator":"bot"}`)
	if err := os.WriteFile(logFile, []byte("cp: cannot stat\n"), 0644); err != nil {
		t.Fatal(err)
	}

	sender := &mockSender{}
	if err := CheckAndSendNotify(sender, nil, notifyFile, tmpDir); err != nil {
		t.Fatalf("CheckAndSendNotify() error = %v", err)
	}

	if len(sender.sentMessages) != 1 {
		t.Fatalf("sent %d messages, want 1", len(sender.sentMessages))
	}
	want := "Update failed: see " + logFile
	if sender.sentMessages[0].text != want {
		t.Errorf("text = %q, want %q", sender.sentMessages[0].text, want)
	}
	if _, err := os.Stat(logFile); err != nil {
		t.Errorf("update.log must survive a failed update: %v", err)
	}
	if _, err := os.Stat(notifyFile); !os.IsNotExist(err) {
		t.Error("notify.json must go, or the message repeats on every restart")
	}
}

func TestCheckAndSendNotify_ZeroChatIDWithoutStoreIsSkipped(t *testing.T) {
	// Dev mode has no chat store; there is nobody to notify and nothing to fix.
	tmpDir := t.TempDir()
	notifyFile := filepath.Join(tmpDir, "notify.json")
	writeNotify(t, notifyFile, `{"chat_id":0,"old_version":"v1.2.0","new_version":"v1.3.0","status":"ok","initiator":"webui"}`)

	sender := &mockSender{}
	if err := CheckAndSendNotify(sender, nil, notifyFile, tmpDir); err != nil {
		t.Fatalf("CheckAndSendNotify() error = %v", err)
	}
	if len(sender.sentMessages) != 0 {
		t.Errorf("sent %d messages, want none", len(sender.sentMessages))
	}
	if _, err := os.Stat(notifyFile); err != nil {
		t.Error("without a store the notification stays for the next start")
	}
}

func TestCheckAndSendNotify_ZeroChatIDWithNoActiveUsersIsCleanedUp(t *testing.T) {
	// Nothing can ever be delivered, so leaving the file would keep the
	// directory around forever.
	tmpDir := t.TempDir()
	notifyFile := filepath.Join(tmpDir, "notify.json")
	writeNotify(t, notifyFile, `{"chat_id":0,"old_version":"v1.2.0","new_version":"v1.3.0","status":"ok","initiator":"webui"}`)

	sender := &mockSender{}
	if err := CheckAndSendNotify(sender, &mockStore{}, notifyFile, tmpDir); err != nil {
		t.Fatalf("CheckAndSendNotify() error = %v", err)
	}
	if _, err := os.Stat(tmpDir); !os.IsNotExist(err) {
		t.Error("an undeliverable notification must not linger")
	}
}

func TestCheckAndSendNotify_PartialFailureKeepsNothingBack(t *testing.T) {
	// One blocked user must not hold the notification for everyone else.
	tmpDir := t.TempDir()
	notifyFile := filepath.Join(tmpDir, "notify.json")
	writeNotify(t, notifyFile, `{"chat_id":0,"old_version":"v1.2.0","new_version":"v1.3.0","status":"ok","initiator":"webui"}`)

	sender := &mockSender{failFor: map[int64]bool{111: true}}
	store := &mockStore{users: []chatstore.UserChat{
		{Username: "alice", ChatID: 111},
		{Username: "bob", ChatID: 222},
	}}

	if err := CheckAndSendNotify(sender, store, notifyFile, tmpDir); err != nil {
		t.Fatalf("CheckAndSendNotify() error = %v", err)
	}
	if _, err := os.Stat(tmpDir); !os.IsNotExist(err) {
		t.Error("one successful send is enough to clean up")
	}
}

func TestCheckAndSendNotify_AllSendsFailKeepTheFile(t *testing.T) {
	tmpDir := t.TempDir()
	notifyFile := filepath.Join(tmpDir, "notify.json")
	writeNotify(t, notifyFile, `{"chat_id":42,"old_version":"v1.2.0","new_version":"v1.3.0","status":"ok","initiator":"bot"}`)

	sender := &mockSender{sendErr: errors.New("network down")}
	if err := CheckAndSendNotify(sender, nil, notifyFile, tmpDir); err == nil {
		t.Fatal("CheckAndSendNotify() must report a total delivery failure")
	}
	if _, err := os.Stat(notifyFile); err != nil {
		t.Error("an undelivered notification must be retried on the next start")
	}
}

func writeNotify(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}
```

Расширить `mockSender`, чтобы он умел отказывать выборочно, и добавить импорты `chatstore`, `errors`:

```go
type mockSender struct {
	sentMessages []sentMessage
	sendErr      error
	failFor      map[int64]bool
}

func (m *mockSender) SendPlain(chatID int64, text string) error {
	if m.failFor[chatID] {
		return errors.New("bot was blocked by the user")
	}
	if m.sendErr != nil {
		return m.sendErr
	}
	m.sentMessages = append(m.sentMessages, sentMessage{chatID, text, true})
	return nil
}
```

(`Send` оставить прежним, приведя тело к тому же виду через вызов `SendPlain` не нужно — оно уже не используется этим кодом.)

Существующие тесты `TestCheckAndSendNotify_Success`, `_NoFile`, `_InvalidJSON`, `_SendError`, `_CleanupAfterSuccess`, `_EmptyVersions` получают дополнительный аргумент `nil` вторым параметром; `TestCheckAndSendNotify_MissingChatID` удаляется (его сценарий покрыт новыми тестами).

- [ ] **Step 2: Запустить и убедиться, что тесты падают**

Через `claude-forge:build`: `cd server && go test ./internal/startup/ -count=1`
Ожидание: FAIL — сигнатура `CheckAndSendNotify` не совпадает, полей `Status`/`Initiator` нет.

- [ ] **Step 3: Переписать `notify.go`**

Заменить `server/internal/startup/notify.go`:

```go
// Package startup handles bot startup tasks like update notifications.
package startup

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/chatstore"
	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/telegram"
)

// Default paths for update notification files.
const (
	DefaultNotifyFile = "/tmp/vpn-director-update/notify.json"
	DefaultUpdateDir  = "/tmp/vpn-director-update"
)

// ChatStore is the part of chatstore.Store the notifier needs: who to tell
// about an update that was started somewhere without a chat of its own.
type ChatStore interface {
	GetActiveUsers() ([]chatstore.UserChat, error)
}

// UpdateNotification represents the JSON structure in notify.json.
type UpdateNotification struct {
	ChatID     int64  `json:"chat_id"` // 0 when the update came from the Web UI
	OldVersion string `json:"old_version"`
	NewVersion string `json:"new_version"`
	Status     string `json:"status"`    // "ok" or "failed"
	Initiator  string `json:"initiator"` // "bot" or "webui"
}

// CheckAndSendNotify checks for a pending update notification and sends it.
// A notification with chat_id 0 was written by a Web UI update and goes to
// every active chat; store may be nil (dev mode), in which case it is skipped
// and kept for the next start. A successful update clears the whole update
// directory; a failed one keeps update.log, because the message points at it.
func CheckAndSendNotify(sender telegram.MessageSender, store ChatStore, notifyFile, updateDir string) error {
	data, err := os.ReadFile(notifyFile)
	if os.IsNotExist(err) {
		// No notification pending - this is normal
		return nil
	}
	if err != nil {
		return fmt.Errorf("read notify file: %w", err)
	}

	var n UpdateNotification
	if err := json.Unmarshal(data, &n); err != nil {
		return fmt.Errorf("parse notify file: %w", err)
	}

	recipients, err := recipientsFor(n, store)
	if err != nil {
		return err
	}
	if recipients == nil {
		// Skipped: nothing to do now, the file waits for the next start.
		return nil
	}

	text := notificationText(n, updateDir)

	sent := 0
	for _, chatID := range recipients {
		if err := sender.SendPlain(chatID, text); err != nil {
			slog.Warn("Failed to send update notification",
				"chat_id", chatID,
				"old_version", n.OldVersion,
				"new_version", n.NewVersion,
				"error", err)
			continue
		}
		sent++
	}

	if sent == 0 && len(recipients) > 0 {
		return fmt.Errorf("send notification: no recipient could be reached")
	}

	cleanup(n, notifyFile, updateDir)
	return nil
}

// recipientsFor resolves the chats to notify. A nil slice means "skip for
// now"; an empty slice means "nobody to tell, but nothing is pending either".
func recipientsFor(n UpdateNotification, store ChatStore) ([]int64, error) {
	if n.ChatID != 0 {
		return []int64{n.ChatID}, nil
	}
	if store == nil {
		slog.Info("Update notification has no chat_id and no chat store, skipping",
			"initiator", n.Initiator)
		return nil, nil
	}
	users, err := store.GetActiveUsers()
	if err != nil {
		return nil, fmt.Errorf("get active users: %w", err)
	}
	chats := make([]int64, 0, len(users))
	for _, u := range users {
		chats = append(chats, u.ChatID)
	}
	if len(chats) == 0 {
		slog.Info("Update notification has no active chats to go to", "initiator", n.Initiator)
	}
	return chats, nil
}

// notificationText renders the message. A failed update points at the log the
// script left behind rather than repeating its exit code.
func notificationText(n UpdateNotification, updateDir string) string {
	if n.Status == "failed" {
		return "Update failed: see " + filepath.Join(updateDir, "update.log")
	}
	return fmt.Sprintf("Update complete: %s → %s", n.OldVersion, n.NewVersion)
}

// cleanup removes what the notification leaves behind. Errors are logged, not
// returned: the message is already delivered and cleanup is best-effort.
func cleanup(n UpdateNotification, notifyFile, updateDir string) {
	if n.Status == "failed" {
		if err := os.Remove(notifyFile); err != nil {
			slog.Warn("Failed to remove notify file", "path", notifyFile, "error", err)
		}
		return
	}
	if err := os.RemoveAll(updateDir); err != nil {
		slog.Warn("Failed to cleanup update directory", "dir", updateDir, "error", err)
	}
}
```

- [ ] **Step 4: Подключить chatstore в боте**

В `server/internal/bot/bot.go`, в `Run`, заменить вызов:

```go
	// b.chatStore is a typed nil in dev mode; assigning it straight into the
	// interface would hand CheckAndSendNotify a non-nil interface over a nil
	// pointer and panic on the first call.
	var store startup.ChatStore
	if b.chatStore != nil {
		store = b.chatStore
	}
	if err := startup.CheckAndSendNotify(b.sender, store, startup.DefaultNotifyFile, startup.DefaultUpdateDir); err != nil {
		slog.Warn("Failed to send update notification", "error", err)
	}
```

- [ ] **Step 5: Запустить тесты**

Через `claude-forge:build`: `cd server && go test ./internal/startup/ ./internal/bot/ -count=1 -v`
Ожидание: PASS.

- [ ] **Step 6: Проверить сборку, vet и формат**

Через `claude-forge:build`: `cd server && go build ./... && go vet ./... && gofmt -l internal/startup/notify.go internal/startup/notify_test.go internal/bot/bot.go`
Ожидание: чисто.

- [ ] **Step 7: Коммит**

```bash
git add server/internal/startup/notify.go server/internal/startup/notify_test.go server/internal/bot/bot.go
git commit -m "feat(startup): notify every active chat about an update started from the Web UI" -m "Claude-Session: <URL текущей сессии>"
```

---

### Task 8: маршруты `/api/update*` и подключение Web UI

**Files:**
- Create: `server/internal/webapi/handler_update.go`, `server/internal/webapi/handler_update_test.go`
- Modify: `server/internal/webapi/router.go` (поле `Deps.Update`, интерфейс `UpdateFlow`, три маршрута)
- Modify: `server/internal/webapi/deadline.go` (`githubDeadline`)
- Modify: `server/internal/webapi/handler_logs.go` (удалить заглушку `handleUpdate`)
- Modify: `server/internal/webapi/handler_logs_test.go` (удалить `TestHandleUpdate_NotImplemented`)
- Modify: `server/internal/webapi/test_helpers_test.go` (мок `UpdateFlow` + поле в `newTestDeps`)
- Modify: `server/internal/updater/github.go` (экспорт `APITimeout`)
- Modify: `server/cmd/webui/main.go` (создать `Flow`, положить в `Deps`)

**Interfaces:**
- Consumes: `updateflow.Flow`, `CheckResult`, `StartResult`, ошибки (Tasks 4–5); `extendWriteDeadline` (блок 2).
- Produces: `webapi.UpdateFlow` (интерфейс-потребитель); `Deps.Update UpdateFlow`; `updater.APITimeout`.

- [ ] **Step 1: Написать падающие тесты**

Создать `server/internal/webapi/handler_update_test.go`:

```go
package webapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/updateflow"
)

func TestHandleUpdateCheck_ReturnsTheResult(t *testing.T) {
	deps := newTestDeps(t)
	flow := deps.Update.(*mockUpdateFlow)
	flow.checkResult = updateflow.CheckResult{
		Current:         "v1.2.0",
		Latest:          "v1.3.0",
		UpdateAvailable: true,
		Changelog:       "what is new",
		CheckedAt:       time.Now(),
	}

	rec := httptest.NewRecorder()
	handleUpdateCheck(deps)(rec, httptest.NewRequest("GET", "/api/update/check", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["latest"] != "v1.3.0" || resp["update_available"] != true {
		t.Errorf("unexpected body: %v", resp)
	}
}

func TestHandleUpdateCheck_ForcePassedThrough(t *testing.T) {
	deps := newTestDeps(t)
	flow := deps.Update.(*mockUpdateFlow)

	rec := httptest.NewRecorder()
	handleUpdateCheck(deps)(rec, httptest.NewRequest("GET", "/api/update/check?force=1", nil))

	if !flow.checkForce {
		t.Error("force=1 must reach the flow, otherwise the button does nothing")
	}
}

func TestHandleUpdateCheck_DevBuild(t *testing.T) {
	for _, devErr := range []error{updateflow.ErrDevMode, updateflow.ErrDevVersion} {
		deps := newTestDeps(t)
		deps.Update.(*mockUpdateFlow).checkErr = devErr

		rec := httptest.NewRecorder()
		handleUpdateCheck(deps)(rec, httptest.NewRequest("GET", "/api/update/check", nil))

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for %v, got %d", devErr, rec.Code)
		}
		var resp map[string]interface{}
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if resp["dev"] != true || resp["update_available"] != false {
			t.Errorf("unexpected dev body: %v", resp)
		}
	}
}

func TestHandleUpdateCheck_GitHubDown(t *testing.T) {
	deps := newTestDeps(t)
	deps.Update.(*mockUpdateFlow).checkErr = &updateflow.GitHubError{Err: errors.New("connection refused")}

	rec := httptest.NewRecorder()
	handleUpdateCheck(deps)(rec, httptest.NewRequest("GET", "/api/update/check", nil))

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleUpdateStart_Accepted(t *testing.T) {
	deps := newTestDeps(t)
	flow := deps.Update.(*mockUpdateFlow)
	flow.startResult = updateflow.StartResult{From: "v1.2.0", To: "v1.3.0"}

	rec := httptest.NewRecorder()
	handleUpdateStart(deps)(rec, httptest.NewRequest("POST", "/api/update", nil))

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["ok"] != true || resp["from"] != "v1.2.0" || resp["to"] != "v1.3.0" {
		t.Errorf("unexpected body: %v", resp)
	}
	if flow.startInitiator != "webui" || flow.startChatID != 0 {
		t.Errorf("Start(initiator=%q, chat=%d), want webui/0", flow.startInitiator, flow.startChatID)
	}
}

func TestHandleUpdateStart_StatusPerOutcome(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "up to date", err: updateflow.ErrUpToDate, want: http.StatusOK},
		{name: "dev mode", err: updateflow.ErrDevMode, want: http.StatusBadRequest},
		{name: "dev build", err: updateflow.ErrDevVersion, want: http.StatusBadRequest},
		{name: "in progress", err: updateflow.ErrInProgress, want: http.StatusConflict},
		{name: "github down", err: &updateflow.GitHubError{Err: errors.New("timeout")}, want: http.StatusBadGateway},
		{name: "anything else", err: errors.New("invalid release version"), want: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := newTestDeps(t)
			deps.Update.(*mockUpdateFlow).startErr = tt.err

			rec := httptest.NewRecorder()
			handleUpdateStart(deps)(rec, httptest.NewRequest("POST", "/api/update", nil))

			if rec.Code != tt.want {
				t.Fatalf("expected %d, got %d: %s", tt.want, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestHandleUpdateStart_UpToDateBody(t *testing.T) {
	deps := newTestDeps(t)
	deps.Update.(*mockUpdateFlow).startErr = updateflow.ErrUpToDate

	rec := httptest.NewRecorder()
	handleUpdateStart(deps)(rec, httptest.NewRequest("POST", "/api/update", nil))

	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["ok"] != true || resp["update_available"] != false {
		t.Errorf("unexpected body: %v", resp)
	}
}

func TestHandleUpdateStatus(t *testing.T) {
	deps := newTestDeps(t)
	deps.Update.(*mockUpdateFlow).inProgress = true

	rec := httptest.NewRecorder()
	handleUpdateStatus(deps)(rec, httptest.NewRequest("GET", "/api/update/status", nil))

	var resp map[string]bool
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp["in_progress"] {
		t.Error("in_progress must follow the flow")
	}
}
```

Добавить мок в `server/internal/webapi/test_helpers_test.go`:

```go
// mockUpdateFlow implements UpdateFlow for testing.
type mockUpdateFlow struct {
	checkResult updateflow.CheckResult
	checkErr    error
	checkForce  bool

	startResult    updateflow.StartResult
	startErr       error
	startInitiator string
	startChatID    int64

	inProgress bool
}

func (m *mockUpdateFlow) Check(_ context.Context, force bool) (updateflow.CheckResult, error) {
	m.checkForce = force
	return m.checkResult, m.checkErr
}

func (m *mockUpdateFlow) Start(_ context.Context, initiator string, chatID int64, _ func(string)) (updateflow.StartResult, error) {
	m.startInitiator = initiator
	m.startChatID = chatID
	return m.startResult, m.startErr
}

func (m *mockUpdateFlow) InProgress() bool { return m.inProgress }
```

и в `newTestDeps` добавить в литерал `Deps`:

```go
		Update:  &mockUpdateFlow{},
```

Импорты теста дополнить `context` и `updateflow`.

Удалить `TestHandleUpdate_NotImplemented` из `server/internal/webapi/handler_logs_test.go`.

- [ ] **Step 2: Запустить и убедиться, что тесты падают**

Через `claude-forge:build`: `cd server && go test ./internal/webapi/ -count=1`
Ожидание: FAIL — `Deps.Update` не существует, обработчиков нет.

- [ ] **Step 3: Экспортировать таймаут GitHub API**

В `server/internal/updater/github.go` переименовать константу и её единственное использование (строка 35):

```go
	// APITimeout bounds one GitHub API call. Handlers that make such a call
	// inside an HTTP request size their response deadline from it.
	APITimeout = 30 * time.Second
```

```go
	ctx, cancel := context.WithTimeout(ctx, APITimeout)
```

- [ ] **Step 4: Добавить дедлайн и обработчики**

В `server/internal/webapi/deadline.go` добавить в блок констант и импорт `updater`:

```go
	// githubDeadline covers the synchronous part of an update route: one
	// GitHub API call under updater.APITimeout. POST /api/update answers 202
	// as soon as the download goroutine is under way, so the script's own
	// minutes never sit on the connection.
	githubDeadline = updater.APITimeout + deadlineSlack
```

Создать `server/internal/webapi/handler_update.go`:

```go
package webapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/updateflow"
)

// handleUpdateCheck reports the latest release. force=1 bypasses the flow's
// 30-minute cache, at most once a minute. A dev build cannot compare versions,
// so it answers with the dev marker instead of an error the UI would have to
// explain.
func handleUpdateCheck(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		extendWriteDeadline(w, githubDeadline)

		res, err := deps.Update.Check(r.Context(), r.URL.Query().Get("force") == "1")
		if err == nil {
			jsonOK(w, res)
			return
		}
		if errors.Is(err, updateflow.ErrDevMode) || errors.Is(err, updateflow.ErrDevVersion) {
			jsonOK(w, map[string]interface{}{"update_available": false, "dev": true})
			return
		}
		var ghErr *updateflow.GitHubError
		if errors.As(err, &ghErr) {
			jsonError(w, http.StatusBadGateway, "failed to check for updates: "+ghErr.Err.Error())
			return
		}
		slog.Warn("update check failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "failed to check for updates")
	}
}

// handleUpdateStart launches the unified self-update. It answers 202 as soon
// as the download starts: the update script stops this process a few seconds
// later, so the client learns the outcome by polling /api/version.
func handleUpdateStart(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		extendWriteDeadline(w, githubDeadline)

		res, err := deps.Update.Start(r.Context(), "webui", 0, func(line string) {
			slog.Info("self-update", "status", line)
		})
		if err == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":   true,
				"from": res.From,
				"to":   res.To,
			})
			return
		}

		switch {
		case errors.Is(err, updateflow.ErrUpToDate):
			jsonOK(w, map[string]interface{}{"ok": true, "update_available": false})
		case errors.Is(err, updateflow.ErrDevMode), errors.Is(err, updateflow.ErrDevVersion):
			jsonError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, updateflow.ErrInProgress):
			jsonError(w, http.StatusConflict, err.Error())
		default:
			var ghErr *updateflow.GitHubError
			if errors.As(err, &ghErr) {
				jsonError(w, http.StatusBadGateway, "failed to check for updates: "+ghErr.Err.Error())
				return
			}
			slog.Warn("update start failed", "error", err)
			jsonError(w, http.StatusInternalServerError, "failed to start update: "+err.Error())
		}
	}
}

// handleUpdateStatus reports whether an update script is running, so a client
// that reconnected after a restart can tell "still updating" from "done".
func handleUpdateStatus(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		jsonOK(w, map[string]bool{"in_progress": deps.Update.InProgress()})
	}
}
```

Удалить `handleUpdate` из `server/internal/webapi/handler_logs.go` (строки 94–100).

- [ ] **Step 5: Подключить маршруты и зависимость**

В `server/internal/webapi/router.go` добавить импорты `context` и `updateflow`, интерфейс и поле:

```go
// UpdateFlow is the subset of updateflow.Flow the API handlers use. Declared
// here so the handler tests can drive every outcome without a fake GitHub.
type UpdateFlow interface {
	Check(ctx context.Context, force bool) (updateflow.CheckResult, error)
	Start(ctx context.Context, initiator string, chatID int64, progress func(string)) (updateflow.StartResult, error)
	InProgress() bool
}
```

В структуре `Deps` после `Logs`:

```go
	Update       UpdateFlow        // self-update orchestration, shared with the bot
```

Заменить в `registerProtectedRoutes` блок `// Self-update`:

```go
	// Self-update
	mux.HandleFunc("GET /api/update/check", handleUpdateCheck(deps))
	mux.HandleFunc("POST /api/update", handleUpdateStart(deps))
	mux.HandleFunc("GET /api/update/status", handleUpdateStatus(deps))
```

В `server/cmd/webui/main.go` добавить импорты `updateflow` и `updater`, создать поток перед `deps` и положить в литерал:

```go
	// The Web UI updates both daemons through the same flow as the bot; the
	// progress lines go to the Web UI log, since there is no chat to answer in.
	updateFlow := updateflow.New(updater.New(), Version, *devFlag)
```

```go
		Update:  updateFlow,
```

- [ ] **Step 6: Запустить тесты**

Через `claude-forge:build`: `cd server && go test ./internal/webapi/ ./internal/updater/ -count=1 -v`
Ожидание: PASS; ни один тест не ссылается на удалённую заглушку 501.

- [ ] **Step 7: Проверить сборку, vet и формат**

Через `claude-forge:build`: `cd server && go build ./... && go vet ./... && gofmt -l internal/webapi/handler_update.go internal/webapi/handler_update_test.go internal/webapi/router.go internal/webapi/deadline.go internal/webapi/handler_logs.go internal/webapi/handler_logs_test.go internal/webapi/test_helpers_test.go internal/updater/github.go cmd/webui/main.go`
Ожидание: чисто.

- [ ] **Step 8: Проверить dev-режим руками**

```bash
cd server && go run ./cmd/webui --dev
```
В другом терминале: залогиниться (`admin`/`admin`) и запросить `POST /api/update` — ожидается 400 и текст `update is not available in dev mode`; `GET /api/update/check` — 200 с `{"update_available":false,"dev":true}`. Остановить сервер.

- [ ] **Step 9: Коммит**

```bash
git add server/internal/webapi/handler_update.go server/internal/webapi/handler_update_test.go server/internal/webapi/router.go server/internal/webapi/deadline.go server/internal/webapi/handler_logs.go server/internal/webapi/handler_logs_test.go server/internal/webapi/test_helpers_test.go server/internal/updater/github.go server/cmd/webui/main.go
git commit -m "feat(webapi): replace the update stub with the real check, start and status routes" -m "Claude-Session: <URL текущей сессии>"
```

---

### Task 9: вкладка Settings и баннер о новой версии

**Files:**
- Modify: `web/src/api.ts`, `web/src/types.ts`
- Modify: `web/src/components/SettingsTab.vue`
- Modify: `web/src/App.vue`

**Interfaces:**
- Consumes: `GET /api/update/check`, `POST /api/update`, `GET /api/version` (Task 8).
- Produces: ничего для Go-кода.

- [ ] **Step 1: Расширить клиент API и типы**

В `web/src/api.ts` заменить блок `// System`:

```ts
  // Self-update
  checkUpdate: (force = false) =>
    api.get('/api/update/check', { params: force ? { force: 1 } : {} }),
  update: () =>
    api.post('/api/update'),
  updateStatus: () =>
    api.get('/api/update/status'),
  // pollVersion is used while the server restarts: connection errors and a
  // brief 401 must not bounce the user to the login page.
  pollVersion: () =>
    api.get('/api/version', { skipAuthRedirect: true } as any),
```

В `web/src/types.ts` добавить:

```ts
export interface UpdateCheckResponse {
  current?: string
  latest?: string
  update_available: boolean
  changelog?: string
  checked_at?: string
  dev?: boolean
}

export interface UpdateStartResponse {
  ok: boolean
  from?: string
  to?: string
  update_available?: boolean
}
```

- [ ] **Step 2: Переписать вкладку Settings**

Заменить блок `<script setup>` и кнопку обновления в `web/src/components/SettingsTab.vue`:

```vue
<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import api from '../api'
import type { VersionResponse, UpdateCheckResponse, UpdateStartResponse } from '../types'

const versionInfo = ref<VersionResponse | null>(null)
const updateInfo = ref<UpdateCheckResponse | null>(null)
const config = ref('')
const showConfig = ref(false)
const showChangelog = ref(false)
const loading = ref(false)
const checking = ref(false)
const updating = ref(false)
const updateMessage = ref('')
const error = ref('')

const canUpdate = computed(() => !!updateInfo.value?.update_available && !updating.value)

async function loadVersion() {
  try {
    const resp = await api.getVersion()
    versionInfo.value = resp.data
  } catch (e: any) {
    error.value = e.response?.data?.error || e.message
  }
}

async function loadUpdate(force = false) {
  checking.value = true
  try {
    const resp = await api.checkUpdate(force)
    updateInfo.value = resp.data
  } catch (e: any) {
    error.value = e.response?.data?.error || e.message
  } finally {
    checking.value = false
  }
}

async function loadConfig() {
  loading.value = true
  try {
    const resp = await api.getConfig()
    config.value = JSON.stringify(resp.data, null, 2)
  } catch (e: any) {
    error.value = e.response?.data?.error || e.message
  } finally {
    loading.value = false
  }
}

async function toggleConfig() {
  showConfig.value = !showConfig.value
  if (showConfig.value && !config.value) {
    await loadConfig()
  }
}

const sleep = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms))

// waitForVersion polls /api/version until the new build answers or five
// minutes pass. Connection errors are expected: the server is restarting.
async function waitForVersion(target: string) {
  const deadline = Date.now() + 5 * 60 * 1000
  while (Date.now() < deadline) {
    await sleep(3000)
    try {
      const resp = await api.pollVersion()
      if (resp.data?.version === target) {
        return true
      }
    } catch {
      // server is down mid-restart, keep waiting
    }
  }
  return false
}

async function doUpdate() {
  const target = updateInfo.value?.latest
  if (!target) return
  if (!confirm(`Update VPN Director to ${target}?`)) return

  updating.value = true
  updateMessage.value = 'Starting update...'
  try {
    const resp = await api.update()
    const data = resp.data as UpdateStartResponse
    if (data.update_available === false) {
      updateMessage.value = 'Already running the latest version.'
      updating.value = false
      return
    }
    updateMessage.value = 'Updating, the server is restarting...'
    const arrived = await waitForVersion(data.to || target)
    if (arrived) {
      // Reload so the browser picks up the new bundle.
      window.location.reload()
      return
    }
    updateMessage.value = 'The new version did not come up within 5 minutes. Check the logs.'
  } catch (e: any) {
    updateMessage.value = 'Error: ' + (e.response?.data?.error || e.message)
  } finally {
    updating.value = false
  }
}

onMounted(async () => {
  await loadVersion()
  await loadUpdate()
})
</script>
```

Заменить блок карточки версии и кнопки в шаблоне:

```vue
  <div class="card">
    <div class="card-title">Version</div>
    <p v-if="error" class="error-msg">{{ error }}</p>
    <div v-if="versionInfo">
      <div class="kv">
        <span class="kv-label">Version</span>
        <span>{{ versionInfo.version }}</span>
      </div>
      <div class="kv">
        <span class="kv-label">Commit</span>
        <span style="font-family: monospace; font-size: 0.85rem;">{{ versionInfo.commit }}</span>
      </div>
      <div class="kv" v-if="updateInfo && !updateInfo.dev">
        <span class="kv-label">Latest</span>
        <span>{{ updateInfo.latest || '—' }}</span>
      </div>
      <div class="kv" v-if="updateInfo?.dev">
        <span class="kv-label">Latest</span>
        <span>dev build, updates disabled</span>
      </div>
    </div>
    <p v-else style="color: #999; font-size: 0.875rem;">Loading...</p>

    <div class="actions">
      <button class="btn" :disabled="checking || updating" @click="loadUpdate(true)">
        {{ checking ? '...' : '⟳ Check for updates' }}
      </button>
      <button class="btn btn-primary" :disabled="!canUpdate" @click="doUpdate">
        {{ updating ? 'Updating...' : `⬆ Update to ${updateInfo?.latest || ''}` }}
      </button>
      <button
        v-if="updateInfo?.changelog"
        class="btn btn-blue"
        @click="showChangelog = !showChangelog"
      >
        {{ showChangelog ? 'Hide changelog' : 'Changelog' }}
      </button>
    </div>
    <p v-if="updateMessage" style="font-size: 0.875rem;">{{ updateMessage }}</p>
    <pre
      v-if="showChangelog && updateInfo?.changelog"
      style="font-size: 12px; white-space: pre-wrap; line-height: 1.5; max-height: 300px; overflow-y: auto; background: #1a1a2e; padding: 0.75rem; border-radius: 4px; border: 1px solid #333;"
    >{{ updateInfo.changelog }}</pre>
  </div>
```

Удалить прежний отдельный блок `<div class="actions">` с кнопкой `⬆ Update` (он переехал внутрь карточки).

- [ ] **Step 3: Добавить баннер в шапку**

В `web/src/App.vue` в `<script setup>` добавить состояние и запрос после успешной аутентификации:

```ts
const updateBanner = ref('')

async function loadUpdateBanner() {
  try {
    const resp = await api.checkUpdate()
    if (resp.data?.update_available && resp.data?.latest) {
      updateBanner.value = `Version ${resp.data.latest} available, see Settings`
    }
  } catch {
    // a failed check must not get in the way of the UI
  }
}
```

и вызвать его из `checkAuth` после `authenticated.value = true`:

```ts
    authenticated.value = true
    loadUpdateBanner()
```

В шаблоне добавить строку в `topbar-info` перед версией:

```vue
        <span v-if="updateBanner" class="update-banner" @click="activeTab = 'settings'">
          {{ updateBanner }}
        </span>
```

и стиль в `web/src/style.css`:

```css
.update-banner {
  cursor: pointer;
  color: #f0a020;
  font-size: 0.85rem;
}
```

- [ ] **Step 4: Собрать фронт**

Через `claude-forge:build`: `cd web && npm run build`
Ожидание: `vue-tsc` без ошибок, сборка успешна.

- [ ] **Step 5: Проверить в dev-режиме**

```bash
cd server && go run ./cmd/webui --dev
```
и в другом терминале `cd web && npm run dev`. Проверить по чек-листу:
1. Вкладка Settings показывает версию и «dev build, updates disabled»;
2. Кнопка «Update to …» неактивна;
3. «Check for updates» не роняет страницу;
4. Баннер в шапке не появляется (dev).
Остановить оба процесса.

- [ ] **Step 6: Коммит**

```bash
git add web/src/api.ts web/src/types.ts web/src/components/SettingsTab.vue web/src/App.vue web/src/style.css
git commit -m "feat(webui): self-update from the Settings tab with a version banner" -m "Claude-Session: <URL текущей сессии>"
```

---

### Task 10: документация блока

**Files:**
- Modify: `README.md`, `README.ru.md`
- Modify: `.claude/rules/telegram-bot.md`

**Interfaces:**
- Consumes: поведение из Tasks 1–9.
- Produces: ничего.

- [ ] **Step 1: Обновить `.claude/rules/telegram-bot.md`**

Заменить секцию `## Self-Update` целиком (внешний блок ниже — на четырёх обратных кавычках, потому что внутри есть вложенный блок кода):

````markdown
## Self-Update (`/update`)

Both daemons — `telegram-bot` and `webui` — are updated together, from the bot
or from the Web UI. The orchestration lives in `internal/updateflow`; the bot
command and the Web UI handlers are adapters over it.

1. `Flow.Check` asks the GitHub API for the latest release (result cached for
   30 minutes; a forced check pierces the cache at most once a minute)
2. `Flow.Start` creates the lock file (`/tmp/vpn-director-update/lock`)
3. Release assets go to `/tmp/vpn-director-update/files/`: every script from
   `scriptFiles` plus one binary per daemon (`telegram-bot-<arch>`,
   `webui-<arch>`). A release missing either binary is a download error.
4. `update.sh` is generated from the daemon table and run detached
5. The script remembers which daemons were running, stops them, copies
   everything, writes `notify.json` and starts back exactly those daemons
6. On failure an `EXIT` trap restarts the daemons that were running and writes
   `notify.json` with `"status": "failed"`

`notify.json`:

```json
{"chat_id": 0, "old_version": "v1.2.0", "new_version": "v1.3.0",
 "status": "ok", "initiator": "webui"}
```

`chat_id` 0 marks an update started from the Web UI: on its next start the bot
notifies every active chat. A successful notification clears
`/tmp/vpn-director-update`; a failed one keeps `update.log`, because the
message points at it.

**Dev mode**: `/update` is disabled with `--dev` and for a `dev` build.
````

- [ ] **Step 2: Обновить `README.md`**

В таблицу вкладок Web UI (строка про **Settings**) заменить описание:

```markdown
| **Settings** | Version, self-update, configuration |
```

Добавить в раздел `## Web UI` после `### Service Management` новый подраздел:

```markdown
### Updates

The **Settings** tab shows the running version, the latest GitHub release and
its changelog. «Update to vX» downloads the release and restarts both the Web
UI and the Telegram bot; the page polls for the new version and reloads itself
when it comes up. The login session survives the update.

An update started from the Web UI is announced in Telegram to every chat the
bot has talked to. `/update` in the bot does the same thing from the other
side — both paths update both daemons.

> Upgrading **to** the first release with the unified updater is still done by
> the old bot-only updater, which does not know about the Web UI. Run
> `install.sh` once after that upgrade; every later update handles both.
```

В таблице команд бота строка `/update` остаётся как есть.

- [ ] **Step 3: Обновить `README.ru.md`**

Заменить строку 118 таблицы возможностей:

```markdown
| **Settings** | Версия, самообновление, конфигурация |
```

Добавить после подраздела `### Управление сервисом` (строка 138 и далее) новый подраздел:

```markdown
### Обновления

Вкладка **Settings** показывает текущую версию, последний релиз на GitHub и его
changelog. Кнопка «Update to vX» скачивает релиз и перезапускает и Web UI, и
Telegram-бота; страница опрашивает версию и сама перезагружается, когда новая
сборка поднялась. Сессия входа переживает обновление.

Об обновлении, запущенном из Web UI, бот сообщает в Telegram во все чаты, с
которыми он общался. Команда `/update` в боте делает то же самое с другой
стороны — оба пути обновляют оба демона.

> Переход **на** первый релиз с единым обновлением выполняет прежний
> обновлятор бота, который не знает про Web UI. После этого перехода один раз
> запустите `install.sh`; все последующие обновления охватывают оба демона.
```

- [ ] **Step 4: Проверить, что документация не расходится с кодом**

```bash
grep -n 'S98vpn-director-webui' server/internal/updater/downloader.go
grep -n 'webui' server/internal/updater/updater.go
grep -rn 'update/check\|update/status' server/internal/webapi/router.go
```
Ожидание: init-скрипт в списке загрузки, демон `webui` в таблице, оба новых маршрута зарегистрированы — ровно то, что обещает документация.

- [ ] **Step 5: Коммит**

```bash
git add README.md README.ru.md .claude/rules/telegram-bot.md
git commit -m "docs: describe the unified self-update of both daemons" -m "Claude-Session: <URL текущей сессии>"
```

---

## Проверка блока (после Task 10, без коммита кода)

- [ ] **Step 1: Полный прогон Go**

Через `claude-forge:build`: `cd server && go build ./... && go vet ./... && go test ./... -count=1`
Ожидание: PASS без строк `(cached)`.

- [ ] **Step 2: Гонки в новом пакете**

Через `claude-forge:build`: `cd server && go test ./internal/updateflow/ ./internal/webapi/ ./internal/handler/ -count=1 -race`
Ожидание: PASS.

- [ ] **Step 3: Фронт и bats**

Через `claude-forge:build`: `cd web && npm run build`
Напрямую: `bats router/test/unit`
Ожидание: обе проверки зелёные (блок 3 не трогает shell роутера, bats — регрессия).

- [ ] **Step 4: Формат**

Через `claude-forge:build`: `cd server && gofmt -l $(git diff --name-only cbdb212..HEAD -- '*.go' | sed 's|^server/||')`
Ожидание: пусто. Файлы вне диффа не проверять и не форматировать.

- [ ] **Step 5: Чистота дерева**

```bash
git status --short
```
Ожидание: в изменённых/добавленных нет ни одного файла автора (`.mcp.json`, `review.diff`, `test_exit.sh`, `server/bot`, `server/webui`, `.claude/settings.local.json`, `docs/superpowers/plans/*-prompt.md`).

- [ ] **Step 6: Ручной чек-лист на роутере (выполняет человек, до PR)**

1. Собрать и разложить бинарники вручную, запустить оба демона.
2. `POST /api/update/check` из браузера — показывает последний релиз.
3. Запустить обновление из Web UI: страница переходит в «Updating…», через несколько десятков секунд перезагружается с новой версией, повторный логин не требуется.
4. В Telegram приходит «Update complete: … → …» без запроса `/update`.
5. `/opt/etc/init.d/S98vpn-director-webui check` и `S98telegram-bot check` — оба running.
6. Сценарий отказа: временно переименовать `/opt/vpn-director/lib` перед обновлением, убедиться, что демоны вернулись и в Telegram пришло «Update failed: see /tmp/vpn-director-update/update.log», а сам лог на месте.
7. Проверить, что демон, остановленный до обновления, после обновления остался остановленным.

# Plan 007: Supply chain — checksums nos downloads, pins de ferramenta, opt-out de auto-install, versão single-source

> **Executor instructions**: Follow this plan step by step. Run every verification
> command and confirm the expected result before moving to the next step. If anything
> in the "STOP conditions" section occurs, stop and report — do not improvise. When
> done, update the status row for this plan in `plans/README.md`.
>
> **Drift check (run first)**: `git diff --stat f3b5007..HEAD -- npm/install.js npm/install.test.js npm/package.json internal/check/toolinstall.go cmd/crivo/main.go Makefile`
> Compare "Current state" with live code; on mismatch, STOP.

## Status

- **Priority**: P2
- **Effort**: M
- **Risk**: LOW (verificação aditiva; pins podem mudar findings — é o objetivo)
- **Depends on**: none
- **Category**: security
- **Planned at**: commit `f3b5007`, 2026-08-14

## Why this matters

O crivo é um binário que roda em máquinas de dev e CI e que **baixa e executa**
código de terceiros: o próprio binário via `npm/install.js` (GitHub Releases, só
TLS como garantia — o GoReleaser **já publica** `checksums.txt` e ele é ignorado),
o gitleaks via `toolinstall.go` (pinado em 8.24.3, mas sem checksum e com
`http.Get` sem timeout), e o semgrep via `pip install --upgrade semgrep`
(**unpinned** — o gate muda de comportamento com qualquer release do PyPI). Não
existe opt-out de auto-install: um `crivo run` pode baixar e executar código da
rede sem consentimento explícito. Para uma ferramento de *segurança* cuja execução
é semi-automática em CI, integridade e reprodutibilidade não são nice-to-have.

## Current state

- `npm/install.js:166-207` — `ensureBinary()` baixa
  `https://github.com/guilherme11gr/crivo/releases/download/v<version>/crivo_<os>_<arch>.tar.gz|zip`,
  extrai, `chmod 0755`, executa. Sem SHA256. `checksums.txt` existe no release
  (`.goreleaser.yaml:31-32`). Fallback: `go install ...@v<version>` (esse tem
  integridade via Go proxy/checksumdb — mantê-lo).
- `internal/check/toolinstall.go:170-232` — `installGitleaks`: versão pinada
  `gitleaksVersion = "8.24.3"`, download do release sem checksum,
  `http.Get` sem timeout (`:199`).
- `internal/check/toolinstall.go:134-164` — `installSemgrep`: venv em
  `~/.qualitygate/venv`, `pip install --upgrade semgrep` (unpinned).
- `cmd/crivo/main.go:36` — `var version = "3.4.2"` vs `npm/package.json` `3.4.3`
  (drift real); `Makefile:3` `VERSION ?= 0.1.0` sem consumidor; `make build` emite
  `crivo.exe` em todo OS (`Makefile:8`). Release workflow injeta
  `-X main.version` (correto em CI).
- Convenções: npm wrapper tem testes `node --test` (`npm test --prefix npm`,
  sem rede — `npm/install.test.js` sobe servidor loopback).

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Testes Go | `go test ./... -count=1 -race` | exit 0 |
| Testes npm wrapper | `npm test --prefix npm` | exit 0 |
| Vet | `go vet ./...` | exit 0 |
| Build | `go build ./cmd/crivo/` | exit 0 |

## Scope

**In scope**:
- `npm/install.js` + `npm/install.test.js` (+ `npm/package.json` só se precisar de dep de sha256 — preferir `crypto` nativo do Node)
- `internal/check/toolinstall.go` + `toolinstall_test.go`
- `cmd/crivo/main.go` (só a var version), `Makefile`
- `README.md` (seção de segurança/config)

**Out of scope**:
- Assinatura criptográfica de releases (cosign) — follow-up; checksum fecha o gap principal.
- Mudar o mecanismo de release (GoReleaser permanece).
- Providers (onde o EnsureTool é chamado) — só o instalador muda.

## Git workflow

- Branch: `security/supply-chain`
- Commits: `feat(npm): verify release checksums before install`; `fix(tools): pinned installs with checksum and timeout`; `feat(tools): CRIVO_NO_AUTO_INSTALL escape hatch`; `chore: single-source version identifier`

## Steps

### Step 1: install.js verifica checksum

`ensureBinary()`: além do archive, baixar `checksums.txt` do mesmo release; computar
sha256 do buffer baixado (`crypto.createHash`); casar contra a linha do asset
(`crivo_<os>_<arch>.tar.gz  <hash>`). Mismatch ou ausência de linha ⇒ hard error
(nunca instalar). Cache: se binário já existe, comportamento atual (skip).

**Verify**: `npm test --prefix npm` → testes novos: checksum ok instala (fixture via servidor loopback); checksum errado ⇒ erro e nenhum binário escrito

### Step 2: gitleaks com checksum + timeout

`toolinstall.go`: embeddar const `gitleaksChecksums` (map os/arch → sha256) para a
8.24.3 — os hashes vêm do `checksums.txt` do release oficial (executor: baixar uma
vez e fixar; citar a URL no comentário do código). Verificar após o download.
Trocar `http.Get` por `http.Client{Timeout: 60s}`.

**Verify**: `go test ./internal/check/... -count=1` → teste com servidor httptest: hash bate instala; hash erra ⇒ erro

### Step 3: semgrep pinado

Const `semgrepVersion = "<versão>"` — escolher a versão vigente no momento
(`semgrep --version` em máquina com o binário; se indisponível, usar a última
estável do changelog do semgrep e citar). `pip install semgrep==<versão>`. Quando
atualizar o pin, é release note do crivo (mudança de findings é intencional).

**Verify**: `grep -n "pip.*install" internal/check/toolinstall.go` → contém `semgrep==`

### Step 4: CRIVO_NO_AUTO_INSTALL

`EnsureTool`: se `os.Getenv("CRIVO_NO_AUTO_INSTALL") == "1"` ⇒ não instalar, retornar
erro `"auto-install disabled (CRIVO_NO_AUTO_INSTALL=1)"`. Os providers já tratam
EnsureTool falho (skip visível — pós-002, ainda mais visível). Documentar no README
(seção de config/envs).

**Verify**: teste de unidade com env setada ⇒ EnsureTool não chama installer (stub)

### Step 5: versão single-source

1. `cmd/crivo/main.go:36` → `var version = "dev"` (default para builds locais).
2. `Makefile`: injetar `-ldflags "-X main.version=$(VERSION)"` com
   `VERSION ?= dev`; output do build vira `crivo` (`.exe` só no Windows —
   usar sufixo por GOOS ou simplesmente `crivo` e documentar).
3. Deletar o `VERSION ?= 0.1.0` morto se sobrar.
4. Release workflow já injeta a versão — conferir e não duplicar.

**Verify**: `make build && ./crivo --version` (ou flag equivalente — conferir como o version é impresso) → `dev`; `make build VERSION=9.9.9` → `9.9.9`

## Test plan

- `npm/install.test.js`: checksum válido/inválido/ausente (servidor loopback já é o padrão dos testes).
- `toolinstall_test.go`: gitleaks checksum ok/erra; timeout respeitado (httptest com delay); CRIVO_NO_AUTO_INSTALL.
- Conferir `Makefile` builds nos 4 alvos do `build-all` (make build-all → 4 binários).

## Done criteria

- [ ] `npm test --prefix npm` exit 0 com testes de checksum negativo
- [ ] `go test ./... -count=1 -race` exit 0
- [ ] Binário corrompido (hash alterado) NUNCA é instalado (teste negativo nos dois instaladores)
- [ ] `pip install` do semgrep tem `==versão`
- [ ] `CRIVO_NO_AUTO_INSTALL=1` não baixa nada (teste)
- [ ] `make build VERSION=x && ./crivo` reporta `x`; sem VERSION reporta `dev`
- [ ] `git status` limpo fora do in-scope; `plans/README.md` atualizado

## STOP conditions

- O asset de gitleaks 8.243 não tiver checksums.txt acessível (reportar URL real).
- `--version` não existir como flag (conferir `cmd/crivo/main.go` — se não existir,
  adicionar é in-scope do Step 5 e trivial; se houver complexidade, reportar).
- Testes de rede não serem viáveis (usar httptest/loopback — não depender de rede real JAMAIS neste plano).

## Maintenance notes

- Bump de semgrep/gitleaks = editar a const + atualizar checksums + release note.
- O npm wrapper é o que roda nas máquinas dos usuários — revisor deve conferir que o
  fail é duro (exit ≠ 0) e que nada é executado antes do checksum bater.

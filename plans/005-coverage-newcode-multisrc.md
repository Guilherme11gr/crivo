# Plan 005: Coverage honra o new-code + duplication/complexity escaneiam todos os `src`

> **Executor instructions**: Follow this plan step by step. Run every verification
> command and confirm the expected result before moving to the next step. If anything
> in the "STOP conditions" section occurs, stop and report — do not improvise. When
> done, update the status row for this plan in `plans/README.md`.
>
> **Drift check (run first)**: `git diff --stat f3b5007..HEAD -- internal/check/providers/coverage/ internal/check/providers/duplication/jscpd.go internal/check/providers/complexity/complexity.go internal/config/config.go`
> Compare "Current state" with live code; on mismatch, STOP.

## Status

- **Priority**: P2
- **Effort**: M
- **Risk**: MED (muda custo/resultado do check de coverage em modo PR — default novo pula a suite)
- **Depends on**: plans/001-fail-closed-new-code.md (escopo confiável), plano 003 Step 8 (recompute de coverage)
- **Category**: perf
- **Planned at**: commit `f3b5007`, 2026-08-14

## Why this matters

Dados reais: a run de quality gate do `agenda-aqui` gastou **176,7s (67% do total) em
coverage** e 88,8s em typecheck — num PR que mudou 6 arquivos, a suite inteira rodou
porque o provider de coverage é o único que **ignora completamente o
`NewCodeScope`**. Enquanto isso, o gate `release` nem usa coverage como blocker
(só `strict`) — ou seja: o check mais caro do pipeline é, no modo PR, o de menor
retorno. Em paralelo, duplication (jscpd) e complexity escaneiam só `cfg.Src[0]` —
o próprio crivo declara `src: [internal/, cmd/]` e o `cmd/` nunca é escaneado.

## Current state

- `internal/check/providers/coverage/coverage.go:117-131` — roda
  `npx vitest run --coverage` (ou jest) sobre a suite inteira; nenhuma referência a
  `NewCodeScopeFromContext` no arquivo (grep). Pós-análise, `main.go` filtra issues,
  mas o custo já aconteceu.
- `internal/config/config.go` — bloco `coverage:` com thresholds
  (`lines/branches/functions/statements`); sem opção de modo new-code.
- `internal/check/providers/duplication/jscpd.go:79-83` — `srcDir = cfg.Src[0]` único
  path escaneado (com early-return "Source directory not found" se ausente).
- `internal/check/providers/complexity/complexity.go:113-117` — AST pass idem `cfg.Src[0]`
  (o fallback regex em `:309-315` itera todos — comportamento divergente entre caminhos).
- Pós-003, `recomputeIssueDrivenCheck` sabe recompute coverage a partir de issues
  filtradas.
- Convenções: providers recebem `ctx` carregando o scope; config via yaml com
  defaults em `DefaultConfig()`.

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Testes alvo | `go test ./internal/check/... ./internal/config/... -count=1` | exit 0 |
| Testes completos | `go test ./... -count=1 -race` | exit 0 |
| Vet | `go vet ./...` | exit 0 |

## Scope

**In scope**:
- `internal/check/providers/coverage/coverage.go` + `coverage_test.go`
- `internal/config/config.go` + `config_test.go` (novo campo `coverage.new-code`)
- `internal/check/providers/duplication/jscpd.go` + teste
- `internal/check/providers/complexity/complexity.go` + teste
- `README.md` (seção `--new-code`)

**Out of scope**:
- Mudança nas políticas de gate (plano 003).
- Cache incremental de coverage (follow-up).
- Provider semgrep/secrets (002/004).

## Git workflow

- Branch: `perf/coverage-newcode-multisrc`
- Commits: `feat(coverage): new-code mode with related/off/full strategies`; `fix(duplication,complexity): scan every configured src dir`

## Steps

### Step 1: config — `coverage.new-code`

Novo campo em `CoverageConfig`: `NewCode string` com valores `off` (default) |
`related` | `full`, validado em `Load` (valor inválido ⇒ erro de config, não
silêncio). Documentar: `off` = pula coverage em modo new-code (com summary
explícito); `related` = roda só testes relacionados aos arquivos alterados;
`full` = comportamento atual (suite inteira).

**Verify**: `go test ./internal/config/... -count=1` → default `off`; valor inválido → erro

### Step 2: coverage consome o escopo

Em `coverage.go` `Analyze`: se `NewCodeScopeFromContext` ativo:
- `off` → retornar `StatusSkipped` com summary
  `"coverage skipped in new-code mode (set coverage.new-code: related|full to enable)"`.
- `related` → vitest: passar os arquivos alterados com `--related`
  (`vitest related --coverage <files>`); jest: `--findRelatedTests <files>` +
  `--coverage`. Se nenhum arquivo alterado for fonte de teste (nenhum `.ts/.tsx`
  no escopo), tratar como `off` com summary próprio.
- `full` → inalterado.

**Verify**: `go test ./internal/check/providers/coverage/... -count=1` → testes: escopo ativo + default ⇒ skipped com mensagem; `related` monta os args certos (assert da linha de comando via stub)

### Step 3: jscpd em todos os `src`

`jscpd.go`: coletar TODOS os paths de `cfg.Src` que existem no disco e passá-los
como argumentos posicionais múltiplos ao jscpd (a CLI aceita vários). O early-return
"Source directory not found" só dispara quando **nenhum** existe. Corrigir os paths
dos findings: jscpd reporta relativos ao cwd — normalizar para relativos ao
`projectDir` ANTES de virar issue (atenção: findings de subpaths hoje saem sem o
prefixo do src; usar `filepath.Rel(projectDir, abs)`).

**Verify**: teste com fixture de 2 src dirs (`a/`, `b/`) e clone atravessando ambos → issues apontam os dois arquivos com path correto

### Step 4: complexity em todos os `src`

`complexity.go` AST pass: uma invocation do script `cognitive.js` por diretório de
`cfg.Src`, resultados agregados (as métricas `total_lines`/`violations` somam).
Fallback regex já itera todos — igualar. Early-return só quando nenhum dir existe.

**Verify**: teste com 2 dirs: `total_lines` = soma dos dois; contagem de funções idem

### Step 5: documentar

`README.md` seção `--new-code`: explicar o tri-estado de coverage, o custo evitado
(suite inteira) e quando usar `related` (PR gates que precisam de número real de
cobertura do código novo).

**Verify**: `grep -n "new-code" README.md` → seção presente com os 3 valores

## Test plan

- `coverage_test.go`: 3 modos × escopo ativo/inativo; construção de args related
  (stub de npx capturando argv); nenhum arquivo fonte no escopo ⇒ skip com summary próprio.
- `jscpd_test.go`: multi-src (Step 3) + normalização de path.
- `complexity_test.go`: multi-src agregando métricas.
- `config_test.go`: validação do enum.
- Padrão: fixtures de filesystem com `t.TempDir()` como nos testes existentes.

## Done criteria

- [ ] `go test ./... -count=1 -race` exit 0
- [ ] Em modo new-code default, coverage não executa a suite (teste asserta `StatusSkipped` + summary)
- [ ] Projeto com `src: [a/, b/]` gera findings de duplicação/complexidade em ambos
- [ ] `README.md` documenta `coverage.new-code`
- [ ] `git status` limpo fora do in-scope; `plans/README.md` atualizado

## STOP conditions

- `vitest related` ou `--findRelatedTests` não funcionarem com `--coverage` nas
  versões comuns (validar num repo fixture real antes; se falhar, deixar `related`
  documentado como best-effort e default `off`).
- Pós-filtro do plano 003 não ter mergeado e o recompute de coverage não existir
  (dependência real — reordenar execução).
- Normalização de paths do jscpd quebrar testes golden existentes de forma não
  mecânica.

## Maintenance notes

- `related` depende de flags estáveis dos runners de teste; revisar em majors do
  vitest/jest.
- O spike de cache de coverage (P8) deve respeitar os modos daqui.

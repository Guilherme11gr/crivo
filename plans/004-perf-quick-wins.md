# Plan 004: Perf quick wins — gitleaks batch, regex pré-compilado, heavy classification, knobs de workers

> **Executor instructions**: Follow this plan step by step. Run every verification
> command and confirm the expected result before moving to the next step. If anything
> in the "STOP conditions" section occurs, stop and report — do not improvise. When
> done, update the status row for this plan in `plans/README.md`.
>
> **Drift check (run first)**: `git diff --stat f3b5007..HEAD -- internal/check/providers/secrets/gitleaks.go internal/check/providers/customrules/matcher.go internal/check/providers/customrules/rule.go internal/check/runner.go`
> Compare "Current state" with live code; on mismatch, STOP.

## Status

- **Priority**: P2
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none (002 mexe nos mesmos arquivos do gitleaks — se 002 já mergeou, rebasar)
- **Category**: perf
- **Planned at**: commit `f3b5007`, 2026-08-14

## Why this matters

Corresponde aos itens P1–P4 do `docs/PERF-AUDIT-2026-08-06.md` (verificados no HEAD
em 2026-08-14 — nada foi corrigido desde então). Um PR que toca 200 arquivos hoje
gera **200 subprocessos gitleaks em série**; o motor de custom rules compila
**2·M·P regexes idênticas por run** (M arquivos × P pacotes banidos); `tsc` e
`complexity` (os subprocessos node mais caros) não são classificados como heavy e
disputam CPU com os demais; e localmente o runner limita a 2 workers/1 heavy sem
nenhum knob. São os parafusos de maior impacto por linha de código.

## Current state

- `internal/check/providers/secrets/gitleaks.go:182-192` — loop 1 subprocesso por
  target:

```go
func runGitleaksTargets(ctx context.Context, gitleaksBin, projectDir string, targets []string) ([]gitleaksResult, error) {
    var all []gitleaksResult
    for _, target := range targets {
        results, err := runGitleaksTarget(ctx, gitleaksBin, target) // 1 proc por arquivo
        ...
```

- `internal/check/providers/customrules/matcher.go:58-66` — compilação dentro do
  loop por pacote (chamado por arquivo × regra a partir de `customrules.go:143-146`):

```go
for _, pkg := range rule.Raw.Packages {
    escaped := regexp.QuoteMeta(pkg)
    patterns := []*regexp.Regexp{
        regexp.MustCompile(`(?:import\s+.*from\s+|import\s+)['"]` + escaped + `(?:/[^'"]*)?['"]`),
        regexp.MustCompile(`require\s*\(\s*['"]` + escaped + `(?:/[^'"]*)?['"]\s*\)`),
    }
```

  Idem `matcher.go:146` (`matchRequireImport` compila 1 regex por chamada, chamada
  por arquivo em `customrules.go:149-150`). O lugar certo já existe:
  `CompiledRule` (`rule.go:39-40`) guarda `PatternRe`/`WhenPatternRe` pré-compilados
  em `CompileRules` (`rule.go:52-200`) — seguir esse padrão.
- `internal/check/runner.go:46-67`:

```go
func defaultMaxWorkers() int {
    if os.Getenv("CI") == "true" { return min(max(runtime.NumCPU()/2, 2), 4) }
    return min(max(runtime.NumCPU()/4, 1), 2)      // local: máx 2
}
func defaultHeavyWorkers() int {
    if os.Getenv("CI") == "true" { return 2 }
    return 1                                        // local: 1 heavy por vez
}
func isHeavyProviderID(id string) bool {
    switch id {
    case "coverage", "duplication", "semgrep", "secrets", "dead-code":
        return true
    default: return false                           // typescript/complexity fora
    }
}
```

- `internal/check/providers/secrets/gitleaks.go:42-45` — `Detect` chama
  `EnsureTool("gitleaks")`, que pode **baixar binário do GitHub** dentro da fase
  sequencial de detect, sem ctx/timeout.
- Convenções: commits conventional; testes in-package; sem golangci-lint (`go vet` only).

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Testes alvo | `go test ./internal/check/... -count=1` | exit 0 |
| Testes completos | `go test ./... -count=1 -race` | exit 0 |
| Vet | `go vet ./...` | exit 0 |
| Benchmark custom-rules (antes/depois) | `go test ./internal/check/providers/customrules/ -run xxx -bench . -benchmem` | métricas coletadas |

## Scope

**In scope**:
- `internal/check/providers/secrets/gitleaks.go` + `gitleaks_test.go`
- `internal/check/providers/customrules/matcher.go`, `rule.go` + `customrules_test.go`
- `internal/check/runner.go` + `runner_test.go`

**Out of scope**:
- Cache entre runs (P8 do PERF-AUDIT — investimento maior, plano futuro).
- Walk compartilhado (P5) e O(N²) semântico (P6).
- Coverage new-code (plano 005).
- Pin/checksum de downloads (plano 007).

## Git workflow

- Branch: `perf/quick-wins`
- Commits: `perf(secrets): single gitleaks invocation for all targets`; `perf(customrules): precompile import regexes at rule compile time`; `perf(runner): classify typescript/complexity as heavy, add worker env knobs`

## Steps

### Step 1: pré-compilar regexes de ban-import/require-import

Em `rule.go` (`CompileRules`), popular campos novos em `CompiledRule`:
`ImportRes []*regexp.Regexp` (um par por pacote de `Raw.Packages`, mesmas expressões
de `matcher.go:64-65`) e `MustImportRe *regexp.Regexp` (expressão de
`matcher.go:146`). Em `matcher.go`, `matchBanImport`/`matchRequireImport` passam a
consumir os campos pré-compilados (deletar os `MustCompile` inline).

**Verify**: `go test ./internal/check/providers/customrules/... -count=1` → passa; `grep -n "MustCompile" internal/check/providers/customrules/matcher.go` → nenhum dentro de função de match (só package-level vars, que já existem e ficam)

### Step 2: gitleaks em invocação única (com chunking)

Em `gitleaks.go`: substituir o loop de `runGitleaksTargets` por UMA invocação
passando todos os targets como argumentos posicionais (formato que
`runGitleaksTarget` já usa). Proteção ARG_MAX: chunks de 128 targets, resultados
concatenados. Preservar o comportamento de relativo/normalização existente
(`normalizeGitleaksResults`).

**Verify**: `go test ./internal/check/providers/secrets/... -count=1` → teste novo: 3 targets ⇒ stub do binário registra **1** invocação com 3 args

### Step 3: typescript/complexity como heavy + knobs de workers

1. `runner.go:isHeavyProviderID` — adicionar `"typescript", "complexity"`.
2. `defaultMaxWorkers`/`defaultHeavyWorkers` — ler `CRIVO_MAX_WORKERS` e
   `CRIVO_MAX_HEAVY` (strconv.Atoi; ignorar valor inválido com valor default; clamp
   1..16). Documentar as envs no README (seção de configuração) e em
   `.claude/skills/ci/SKILL.md` se houver seção de tuning.

**Verify**: `go test ./internal/check/... -count=1` → testes novos: env setada muda workers; `CRIVO_MAX_WORKERS=banana` mantém default; `isHeavyProviderID("typescript") == true`

### Step 4: mover auto-install do gitleaks para dentro do Analyze

`Detect` (`gitleaks.go:42-45`) passa a usar só `FindTool` (sem rede). O `EnsureTool`
fica no início do `Analyze` (que já roda com ctx de 5 min e concorrente).

**Verify**: `go test ./internal/check/providers/secrets/... -count=1` → passa

### Step 5: benchmark antes/depois (documentação interna)

Adicionar `BenchmarkMatchBanImport` em `customrules_test.go` (fixture: 200 linhas, 4
pacotes banidos). Rodar e registrar os números no corpo do commit (allocs/op antes
vs depois esperam queda de ~2 ordens de magnitude nas compilações — o benchmark
prova direção, não número exato).

**Verify**: `go test ./internal/check/providers/customrules/ -bench BenchmarkMatchBanImport -benchmem -run xxx` → roda sem erro

## Test plan

- `customrules_test.go`: matching semantics inalterados (casos existentes de
  ban-import/require-import já cobrem); benchmark novo.
- `gitleaks_test.go`: stub de binário contando invocações (1 para N targets ≤128;
  2 para 200 targets); resultados idênticos ao loop antigo (fixture JSON).
- `runner_test.go`: env knobs + heavy classification.
- Padrão: seguir os stubs de PATH de `semgrep_test.go:260-270`.

## Done criteria

- [ ] `go test ./... -count=1 -race` exit 0
- [ ] `grep -c "runGitleaksTarget(" internal/check/providers/secrets/gitleaks.go` → chamada única (loop de chunk não conta target individual)
- [ ] Nenhum `regexp.MustCompile` dentro de `matchBanImport`/`matchRequireImport`
- [ ] `CRIVO_MAX_WORKERS=8` reflete em teste
- [ ] `git status` limpo fora do in-scope; `plans/README.md` atualizado

## STOP conditions

- gitleaks não aceitar múltiplos targets numa invocação na versão pinned (8.24.3) —
  validar manualmente com `gitleaks detect --help` antes; se não aceitar, manter
  chunk=1 e reportar.
- Mudança de `CompiledRule` quebrar serialização/testes golden de rules.
- Conflito de merge substancial com o plano 002 em `gitleaks.go`.

## Maintenance notes

- Pós-merge, re-rodar o PERF-AUDIT: P1–P4 devem sair da lista; P5–P8 seguem abertos.
- Revisor: confirmar que o chunking não quebra a normalização de paths relativos.

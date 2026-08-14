# Plan 003: O gate não mente — falha de provider é error; rating e param de serem fictícios

> **Executor instructions**: Follow this plan step by step. Run every verification
> command and confirm the expected result before moving to the next step. If anything
> in the "STOP conditions" section occurs, stop and report — do not improvise. When
> done, update the status row for this plan in `plans/README.md`.
>
> **Drift check (run first)**: `git diff --stat f3b5007..HEAD -- internal/check/providers/typescript/ internal/check/providers/duplication/jscpd.go internal/check/providers/deadcode/knip.go internal/check/providers/coverage/coverage.go internal/rating/rating.go internal/config/ cmd/crivo/main.go`
> Compare "Current state" with live code; on mismatch, STOP.

## Status

- **Priority**: P1
- **Effort**: M
- **Risk**: MED (gate passa a reprovar runs que hoje "passam" por infra-quebra — comportamento pretendido, mas visível)
- **Depends on**: none (sóBeneficia-se do 001, não requer)
- **Category**: bug
- **Planned at**: commit `f3b5007`, 2026-08-14

## Why this matters

Hoje, quase toda falha de infraestrutura vira aprovação: tsc que crashou reporta
"0 errors ✅"; jscpd que não rodou fabrica `percentage: 0` — que gera uma condição
`duplication_pct Actual=0 Passed=true` num **blocker** da política release; knip que
falhou vira "No dead code detected"; coverage lê um `coverage-summary.json` stale da
run anterior quando a suite atual falhou. E o `EvaluateQualityGate` constrói condições
**aprovadas** a partir de métricas nulas de checks em `StatusError`. Além disso a
config mente: o bloco `quality-gate:` do `.qualitygate.yaml` não tem consumidor
algum (thresholds hardcoded no rating.go), `lint_errors` é blocker de um check que
não existe mais, e o baseline downgrade rebaixa `failed`→`warning` sem opt-in. Um
gate precisa de uma propriedade só, acima de tudo: que o verde signifique verde.

## Current state

- `internal/check/providers/typescript/tsc.go:112-117`:

```go
// If tsc exited non-zero but we found no parseable errors, treat as passed
if totalErrors == 0 {
    status = domain.StatusPassed
    summary = "0 errors"
}
```

- `internal/check/providers/duplication/jscpd.go:128` — `_ = cmd.Run()`; `:143-151`
  sem report → `StatusWarning` com `Metrics{"percentage": 0, "clones": 0}`.
- `internal/rating/rating.go:226-236` — condição `duplication_pct` construída dessas
  métricas: `Actual: pct(=0), Passed: pct < 5` → **pass** num blocker.
- `internal/check/providers/deadcode/knip.go:60-84` — skip só se stderr contém
  `command not found`/`not recognized`/`ERR!`; caso contrário `runErr != nil` com
  output vazio cai em `parseKnipOutput("")` → `StatusPassed "No dead code detected"`.
- `internal/check/providers/coverage/coverage.go:139-166` — `runErr := cmd.Run()`
  registrado, mas o `coverage/coverage-summary.json` é lido de qualquer forma; se
  existir arquivo antigo, os números stale viram o resultado.
- `internal/rating/rating.go:196-211` — condição typescript: `errors := 0.0; if
  check.Metrics != nil {...}` → check em error com métricas ausentes vira
  `type_errors Actual=0 Passed=true`. Mesmo padrão em secrets (`:239-250`). Só a
  política `strict` consulta `Status` (`:279-286`).
- `internal/rating/rating.go:135-152` — `policyBlockers["release"]`: type_errors,
  lint_errors (dead), secrets, duplication_pct, custom_rules_blocking. Thresholds
  hardcoded (`:219` coverage 60, `:232` duplication 5, `:247` secrets 1, `:260`).
- `internal/config/config.go:47-58` + `:132-145` — `QualityGate.NewCode/Overall`
  sem nenhum consumidor (grep confirma); `internal/config/profiles.go:18-27,55-63`
  escrevem nesses campos mortos.
- `cmd/crivo/main.go:961-985` — baseline downgrade `failed`→`warning` sem flag; 
  `main.go:973` `if prevViolations >= 0` é **sempre true** (ausência de métrica lê
  como 0); `main.go:925-938` baseline é o último save de qualquer branch/modo.
- `cmd/crivo/main.go:410-423` — `recomputeIssueDrivenCheck` cobre só
  typescript/secrets/semgrep/duplication/custom-rules; coverage/dead-code/complexity
  ficam com status pós-filtro stale. `main.go:524-547` —
  `recomputeDuplicationCheck` fabrica `percentage = 100` quando sobra qualquer clone
  do new-code.
- `internal/check/runner.go:192-212` — `isCheckEnabled` não tem case para
  `complexity` (sempre on); `ChecksConfig` (`config.go:37-45`) não tem o campo.
- Convenções: conventional commits; testes table-driven in-package; rating tem
  `rating_test.go` como modelo.

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Testes alvo | `go test ./internal/rating/... ./internal/check/... ./cmd/... -count=1` | exit 0 |
| Testes completos | `go test ./... -count=1 -race` | exit 0 |
| Vet | `go vet ./...` | exit 0 |

## Scope

**In scope**:
- `internal/check/providers/typescript/tsc.go` (+ teste)
- `internal/check/providers/duplication/jscpd.go` (+ teste)
- `internal/check/providers/deadcode/knip.go` (+ teste)
- `internal/check/providers/coverage/coverage.go` (+ teste)
- `internal/rating/rating.go` + `rating_test.go`
- `internal/config/config.go`, `internal/config/profiles.go` (+ testes)
- `internal/check/runner.go` (só o case de `complexity` em `isCheckEnabled`)
- `cmd/crivo/main.go` (só baseline downgrade + recompute) + `main_test.go`
- `internal/output/markdown.go` (linha de skipped, Step 8)

**Out of scope**:
- Providers secrets/semgrep (plano 002).
- `--new-code`/git (plano 001).
- Perf (plano 004) e cobertura new-code (plano 005).

## Git workflow

- Branch: `fix/gate-truth`
- Commits: um por fase — `fix(providers): crashed tools are errors, not passes`; `fix(rating): errored checks never emit passing conditions`; `fix(config): wire real thresholds, drop dead knobs`; `fix(baseline): legacy-debt tolerance behind opt-in`

## Steps

### Fase A — providers reportam a verdade

**Step 1**: `tsc.go` — quando o subprocesso falhou (`runErr != nil`) e
`totalErrors == 0`, retornar `StatusError` com excerpt do stderr (truncar 300 chars)
em vez do override `StatusPassed "0 errors"`.

**Step 2**: `jscpd.go` — sem report gerado: `StatusError` **sem** as métricas
`percentage`/`clones` (a condição de duplicação não deve existir quando não houve
análise).

**Step 3**: `knip.go` — `runErr != nil` e 0 issues parseadas → `StatusError` com
stderr (manter `StatusSkipped` apenas para os casos de "não instalado").

**Step 4**: `coverage.go` — `runErr != nil` ⇒ ignorar summary stale: sempre tomar o
ramo de falha (deletar o `coverage-summary.json` antes de rodar a suite é alternativa
aceitável; escolher a que exigir menos mudança e mantê-la determinística).

**Verify (Fase A)**: `go test ./internal/check/... -count=1` → novos testes passando; `go vet ./...` exit 0

### Fase B — o gate consome a verdade

**Step 5**: `rating.go` — no loop de condições, antes do switch:
`if check.Status == domain.StatusError { errored++; continue }`. Expor métrica
`errored_checks` no resultado; na política `release`, uma condição nova
`errored_checks < 1` (blocker — fail-closed de infra). Em `strict` o comportamento
any-failed/error existente permanece. Condições nunca mais são construídas de
métricas ausentes (o `continue` acima cobre; adicionar teste com metrics nil).

**Step 6**: wire de thresholds — `EvaluateQualityGate` passa a receber os thresholds
do config (novo param ou struct `GateThresholds` carregado de `cfg.QualityGate` com
defaults atuais 60/5/1/1/1). Valores do `.qualitygate.yaml`/profiles passam a valer.
Remover o blocker `lint_errors` (check não existe).

**Step 7**: baseline downgrade (`main.go:961-985`) — atrás de flag nova
`baseline.legacy-debt-tolerance: false` (default **false**); consertar a tautologia
usando presença-real no map (`v, ok := baseline["complexity_violations"]; if !ok {
break }`); ao salvar/ler baseline, registrar `branch`+`mode` e comparar só runs do
mesmo par (ver `internal/store/store.go` schema — adicionar coluna `mode` se necessário).

**Step 8**: recompute pós-filtro — estender `recomputeIssueDrivenCheck` com
coverage/dead-code/complexity: recompute status a partir dos issues filtrados
(coverage sem dados de new-code → `StatusWarning "no coverage data in new code"`,
métricas zeradas não viram condição pelo Step 5 se não houve análise). 
`recomputeDuplicationCheck`: em vez de `percentage=100`, emitir
`new_code_clones` (count) e **não** setar `percentage`; `rating.go` só emite a
condição `duplication_pct` quando a métrica `percentage` existe. Adicionar case
`complexity` em `isCheckEnabled` + campo `complexity` (default true) em
`ChecksConfig`. Renderizar checks `skipped` como linha explícita no
`internal/output/markdown.go` ("⚠ skipped: <name> — <summary>") — hoje eles somam do
relatório.

**Verify (Fase B)**: `go test ./internal/rating/... ./internal/config/... ./cmd/... -count=1 -race` → exit 0

## Test plan

- `tsc_test.go`: runErr + 0 erros parseados → StatusError (fixture: output vazio + exit 1).
- `jscpd_test.go`: sem report → StatusError e `Metrics` sem `percentage`.
- `knip_test.go`: runErr com stderr `npm error enoent` → StatusError.
- `coverage_test.go`: runErr + summary stale existente → falha (não passa).
- `rating_test.go`: check error com metrics nil → nenhuma condição dele; `errored_checks=1` bloqueia release; thresholds vindos do config mudam a condição.
- `main_test.go`: downgrade não acontece sem a flag; com flag, acontece; ausência de métrica no baseline não conta como 0.
- Padrão: seguir `rating_test.go` existente.

## Done criteria

- [ ] `go test ./... -count=1 -race` exit 0
- [ ] Teste demonstra: tsc crashado ⇒ gate reprova (errored_checks) em release
- [ ] `grep -n "lint_errors" internal/rating/rating.go` sem resultados
- [ ] `.qualitygate.yaml` com `quality-gate.overall.coverage: 70` muda o threshold da condição (teste)
- [ ] Baseline downgrade exige `legacy-debt-tolerance: true` (teste)
- [ ] `git status` limpo fora do in-scope; `plans/README.md` atualizado

## STOP conditions

- Excerpts não batem (drift).
- `EvaluateQualityGate` tiver callers além de `main.go` que não suportem o novo param.
- A mudança no store (coluna `mode`) exigir migration além de CREATE TABLE IF NOT EXISTS
  (reportar schema atual antes de improvisar).
- Algum teste golden depender do texto exato dos summaries alterados e a atualização
  não for mecânica.

## Maintenance notes

- Depois deste plano, `StatusError` reprova release — comunicar no README (breaking
  para quem dependia do fail-open).
- O plano 005 depende do recompute de coverage feito aqui.
- Revisor: os asserts de "condição não emitida sem métrica" são a parte mais importante.

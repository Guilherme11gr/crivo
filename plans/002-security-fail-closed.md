# Plan 002: Controles de segurança fail-closed — secrets/semgrep nunca "passam" sem escanear

> **Executor instructions**: Follow this plan step by step. Run every verification
> command and confirm the expected result before moving to the next step. If anything
> in the "STOP conditions" section occurs, stop and report — do not improvise. When
> done, update the status row for this plan in `plans/README.md`.
>
> **Drift check (run first)**: `git diff --stat f3b5007..HEAD -- internal/check/providers/secrets/ internal/check/providers/semgrep/ internal/check/providers/customrules/matcher.go internal/check/providers/customrules/customrules.go`
> Compare the "Current state" excerpts with the live code before proceeding; on a
> mismatch, treat it as a STOP.

## Status

- **Priority**: P1
- **Effort**: M
- **Risk**: LOW (superficiar falhas só muda o resultado de runs que hoje passam errado)
- **Depends on**: plans/001-fail-closed-new-code.md
- **Category**: security
- **Planned at**: commit `f3b5007`, 2026-08-14

## Why this matters

Evidência dos artefatos de CI do `agenda-aqui`: a run de push na main achou **16
secrets**; a run do PR, no código equivalente, reportou **"0 secrets ✅"** — porque o
escopo new-code quebrou (fixado no 001) e, com 0 alvos, o gitleaks retorna
`StatusPassed`. Pior: as **custom rules semgrep** — onde moram regras `severity:
blocker` como `no-eval`/`no-innerhtml` — retornam `nil` silenciosamente quando o
binário semgrep não está disponível, e o provider reporta "N rules checked · no
violations — PASSED". Um controle de segurança que reporta sucesso sem ter escaneado
nada é pior que a ausência do controle. Este plano fecha todas as portas de
"pass vacuoso" nos checks de segurança e remove o vazamento de 8 caracteres de cada
secret nos relatórios.

## Current state

- `internal/check/providers/secrets/gitleaks.go:62-71` — com escopo ativo e 0 alvos:

```go
targets := gitleaksTargets(ctx, projectDir)
if len(targets) == 0 {
    return &domain.CheckResult{
        ...
        Status:   domain.StatusPassed,
        Summary:  "0 secrets detected",
    }, nil
}
```

- `internal/check/providers/semgrep/semgrep.go:91-100` — mesmo padrão (`0 findings`).
- `internal/check/providers/customrules/matcher.go:310-317` — disponibilidade sem
  install e com cache negativo vitalício do processo:

```go
func isSemgrepAvailable() bool {
    if semgrepAvailable != nil { return *semgrepAvailable }
    avail := check.FindTool("semgrep") != ""
    semgrepAvailable = &avail
    return avail
}
```

- `internal/check/providers/customrules/matcher.go:320-329` — `findSemgrepBin` TEM o
  `EnsureTool` (auto-install), mas só é chamado em `:607`, **depois** do gate
  `isSemgrepAvailable` já ter retornado falso — caminho morto.
- `internal/check/providers/customrules/matcher.go:554-555`:

```go
func matchSemgrepBatch(...) []domain.Issue {
    if !isSemgrepAvailable() || len(rules) == 0 {
        return nil
    }
```

  O provider então reporta `passed` / "N rules checked · no violations"
  (`customrules.go:200-206`).
- `internal/check/providers/customrules/matcher.go:590-625` — erros do subprocesso
  semgrep engolidos: `:614` `_ = cmd.Run()`; stdout vazio → `continue`; JSON
  inválido → `continue`; falha ao escrever config → `continue`.
- `internal/check/providers/secrets/gitleaks.go:273-278` — vazamento parcial:

```go
func maskSecret(s string) string {
    if len(s) <= 8 { return "****" }
    return s[:4] + "****" + s[len(s)-4:]
}
```

  O valor mascarado vai para `Issue.Message` → console/SARIF/markdown (comentário de
  PR) e para `checks_json` no `history.db`.
- Convenções: testes stdlib in-package; providers retornam `(*domain.CheckResult, error)`.

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Testes alvo | `go test ./internal/check/... -count=1` | exit 0 |
| Testes completos | `go test ./... -count=1 -race` | exit 0 |
| Vet | `go vet ./...` | exit 0 |

## Scope

**In scope**:
- `internal/check/providers/secrets/gitleaks.go` + `gitleaks_test.go`
- `internal/check/providers/semgrep/semgrep.go` + `semgrep_test.go`
- `internal/check/providers/customrules/matcher.go` + `customrules.go` + `customrules_test.go`
- `internal/domain/remediation.go` (só se o fix de remediation do Step 4 pedir)

**Out of scope**:
- `internal/rating/` (tratamento de `StatusError` no gate é o plano 003).
- Pin de versão/checksum do semgrep e gitleaks (plano 007).
- Batch de targets do gitleaks / performance (plano 004).

## Git workflow

- Branch: `fix/security-fail-closed`
- Commits: `fix(secrets): empty scan targets under scope is an error, not a pass`; `fix(customrules): semgrep rules run or fail visibly`; `fix(secrets): mask secrets without substrings`

## Steps

### Step 1: 0 alvos com escopo ativo = StatusError

Em `gitleaks.go` e `semgrep.go`: quando `NewCodeScopeFromContext` retorna `ok=true` e
a lista de alvos escaneáveis é vazia, retornar `StatusError` com summary
`"new-code scope active but 0 scannable targets"` e detail instruindo a checar o diff
(não `StatusPassed`). O caso sem escopo (full scan) permanece inalterado.

**Verify**: `go test ./internal/check/providers/secrets/... ./internal/check/providers/semgrep/... -count=1` → passando com novos testes

### Step 2: custom semgrep rules — install alcançável ou skip visível

Em `matcher.go`:

1. Trocar `isSemgrepAvailable` para usar `check.EnsureTool("semgrep")` (como o
   provider dedicado faz em `semgrep.go:66-76`); remover o cache negativo
   (`semgrepAvailable`) ou invalidá-lo após a tentativa de install.
2. Se ainda assim o binário não existir: `matchSemgrepBatch` deve reportar isso ao
   caller. Mudar a assinatura para retornar `([]domain.Issue, []string /*details*/)`
   com um detail por grupo: `"semgrep unavailable — N rules skipped: <ids>"`. Em
   `customrules.go`, os details entram em `CheckResult.Details` e o status do provider
   vira `StatusWarning` (nunca `passed` quando há regras semgrep pendentes), summary
   `"N rules checked · M semgrep rules SKIPPED (semgrep unavailable)"`.

**Verify**: `go test ./internal/check/providers/customrules/... -count=1` → testes novos do Step 6 passando

### Step 3: erros do subprocesso semgrep superfícies

Em `matcher.go` (`matchSemgrepBatch`): capturar o erro de `cmd.Run()` e o stderr;
qualquer falha (exit != 0 sem JSON, JSON inválido, falha ao escrever o config) vira
detail `"semgrep failed for rules <ids>: <stderr truncado 200 chars>"` pelo mesmo
canal do Step 2, e o provider não afirma "no violations" para esses grupos.

**Verify**: `go vet ./...` → exit 0 (mudança de assinatura sem callers perdidos)

### Step 4: remediation do batch semgrep

Em `matcher.go:655`, o batch usa `domain.CustomRuleRemediation("advisory", ...)`
enquanto o caminho single usa `"semgrep"`. Corrigir para `"semgrep"` (ou derivar de
`rule.Advisory`). Conferir `internal/domain/remediation.go:190-211` para o texto
correto por chave.

**Verify**: teste novo (Step 6) asserting remediation correta no resultado do stub

### Step 5: maskSecret sem substrings

Em `gitleaks.go:273-278`: substituir por máscara fixa + fingerprint:
`"****" + fmt.Sprintf("(%x)", sha256.Sum256([]byte(s)))[:8]` — nunca substring do
valor. Atualizar o teste existente de masking.

**Verify**: `go test ./internal/check/providers/secrets/... -count=1` → passando

### Step 6: teste ponta-a-ponta com stub de semgrep

Em `customrules_test.go`: criar um executável shell stub `semgrep` em
`t.TempDir()/bin` (script que imprime JSON fixture no formato `semgrepJSON`,
`matcher.go:289-304`), colocar no PATH do subprocesso (env do `exec.Command`), rodar
`matchSemgrepBatch` e afirmar: mapeamento `check_id`→regra, conversão de path
(`filepath.Rel`), allow-in aplicado, remediation correta, e details de erro quando o
stub sai com código != 0. Isso fecha a lacuna de cobertura do caminho vivo ( finding
TEST-01 da auditoria).

**Verify**: `go test ./internal/check/providers/customrules/... -count=1 -race` → exit 0

## Test plan

- `gitleaks_test.go`: escopo ativo + 0 targets → `StatusError`; sem escopo →
  comportamento inalterado; masking sem substring (assert não contém prefixo/sufixo).
- `semgrep_test.go`: espelhar o caso de 0 targets.
- `customrules_test.go`: stub semgrep (Step 6); semgrep ausente → provider
  `warning` com rule IDs listadas; subprocesso falhando → details com stderr.
- Padrão: seguir `semgrep_test.go:260-270` para manipulação de PATH.

## Done criteria

- [ ] `go test ./... -count=1 -race` exit 0
- [ ] `grep -n "0 secrets detected" internal/check/providers/secrets/gitleaks.go` só
      aparece no caminho sem escopo ativo
- [ ] Com semgrep ausente e regras semgrep no config: output do crivo contém
      "SKIPPED" e os IDs das regras (verificar via teste, não manualmente)
- [ ] Nenhum `Issue.Message` de secrets contém substring do match (teste)
- [ ] `git status` limpo fora do in-scope
- [ ] `plans/README.md` atualizado

## STOP conditions

- A mudança de assinatura de `matchSemgrepBatch` quebrar callers fora de
  `customrules.go` (não deveria — grep antes).
- `domain.CheckResult` não suportar o status/summary pretendido de nenhuma forma.
- O stub de semgrep não for viável no ambiente de teste (reportar; alternativa é
  extrair o parsing para função pura e testá-la direto).

## Maintenance notes

- O plano 003 consome `StatusError`/`StatusWarning` no gate — manter os summaries
  estáveis (podem virar parte de assertion de integração).
- Revisor: confirmar que nenhum detail/log carrega o match completo do secret.

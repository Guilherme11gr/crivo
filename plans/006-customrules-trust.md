# Plan 006: Custom rules — confiança do motor (yaml loud, remediation, glob-miss, walkers)

> **Executor instructions**: Follow this plan step by step. Run every verification
> command and confirm the expected result before moving to the next step. If anything
> in the "STOP conditions" section occurs, stop and report — do not improvise. When
> done, update the status row for this plan in `plans/README.md`.
>
> **Drift check (run first)**: `git diff --stat f3b5007..HEAD -- internal/config/config.go internal/check/providers/customrules/ cmd/crivo/main.go`
> Compare "Current state" with live code; on mismatch, STOP.

## Status

- **Priority**: P2
- **Effort**: M
- **Risk**: LOW–MED (exclude por path relativo muda quais arquivos são escaneados em configs existentes)
- **Depends on**: none (o Step de semgrep do 002 pode ter mergeado antes — rebasar)
- **Category**: bug
- **Planned at**: commit `f3b5007`, 2026-08-14

## Why this matters

As custom rules são o diferencial do produto — e hoje o motor tem modos de sumiço
silencioso: um YAML malformado (indentação errada) faz o `config.Load` descartar o
erro, cair nos defaults e **todas as regras desaparecem** com o crivo saindo verde;
uma regra cujo glob casa 0 arquivos reporta "no violations — PASSED"; o remediation
do batch semgrep chama-se "advisory" até para blockers; o exclude casa só por
basename de diretório (o `"*.min.js"` default nunca teve efeito); e existe um
caminho semgrep single-rule morto de ~135 linhas que já divergiu do batch (foi onde
nasceu o bug do remediation). Regra que some em silêncio é convenção não aplicada —
o oposto do produto.

## Current state

- `internal/config/config.go:196-198` — erro de unmarshal descartado:

```go
if err := yaml.Unmarshal(data, cfg); err != nil {
    continue
}
return cfg, configPath
```

  ...e `Load` termina com `return cfg, "defaults"` (sem erro). `Detect` do
  custom-rules vê 0 regras e o provider é reportado como skipped ("Not detected in
  project", `runner.go:97-111`).

- `internal/check/providers/customrules/matcher.go:655` — remediation do batch:
  `domain.CustomRuleRemediation("advisory", rule.Raw.Message)` (o single-rule em
  `:468` usa `"semgrep"` — os dois caminhos divergiram).
- `internal/check/providers/customrules/customrules.go:107-117` — erro de
  `filesForGlob` ⇒ `continue` silencioso; `:131-134` — `os.ReadFile` falhar ⇒
  `continue`; provider fecha com `"N rules checked · no violations"` passed
  (`:200-206`).
- `internal/check/providers/customrules/filewalker.go:36-42` — excludes viram um set
  consultado só contra `filepath.Base(path)` de diretórios (`:61-64`); default
  `Exclude` contém `"*.min.js"` (`config.go:121`) que nunca casa basename de dir;
  `"src/generated/"` excluiria qualquer diretório `generated` na árvore.
- `internal/check/providers/customrules/matcher.go:397-473` (`matchSemgrep`) e
  `:339-392` (`buildSemgrepConfigFile`) — chamados só de testes
  (`customrules_test.go:941,981,1027`); produção usa `matchSemgrepBatch`/
  `buildSemgrepBatchConfig`.
- `filewalker.go:31-94` — `WalkFiles`: excludes, cap 1MB, symlink skip, depth 50 —
  sem testes diretos; `IsTextFile` (`:107-122`) idem. Glob engine (`:129-147`)
  suporta `*`, `**`, `?`, `{a,b}` — **sem** `[...]` (pattern `**/*.[jt]s` nunca casa).
- Convenções: `CompileRules` valida tudo de uma vez colecionando erros (bom
  padrão a seguir — `rule.go:52-200`); testes table-driven in-package.

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Testes alvo | `go test ./internal/config/... ./internal/check/providers/customrules/... ./cmd/... -count=1` | exit 0 |
| Testes completos | `go test ./... -count=1 -race` | exit 0 |
| Vet | `go vet ./...` | exit 0 |

## Scope

**In scope**:
- `internal/config/config.go` (+ teste) e o caller do `Load` em `cmd/crivo/main.go`
- `internal/check/providers/customrules/` (matcher.go, customrules.go, filewalker.go + testes)

**Out of scope**:
- Schema novo (`tests:` fixtures, packs) — plano 008.
- Semgrep availability/erros de subprocesso — plano 002.
- Perf de regex — plano 004.

## Git workflow

- Branch: `fix/customrules-trust`
- Commits: `fix(config): malformed qualitygate yaml is a hard error`; `fix(customrules): remediation and glob-miss visibility`; `refactor(customrules): delete dead single-rule semgrep path`; `fix(customrules): exclude matches relative paths`

## Steps

### Step 1: YAML malformado é erro duro

`config.Load` muda para retornar erro quando encontrou um arquivo candidato mas o
unmarshal falhou (colecionar TODOS os candidatos com erro é melhor: se existem
`.qualitygate.yaml` e `.qualitygate.yml`, um quebrado não deve ser pulado em
silêncio). Caller em `main.go`: erro de config ⇒ saída não-zero com
`config parse error in <path>: <yaml error>`. Teste: yaml com tab/indent errada ⇒
erro com o path.

**Verify**: `go test ./internal/config/... ./cmd/... -count=1` → novo teste passa

### Step 2: remediation do batch

`matcher.go:655` → usar `"semgrep"` (ou derivar blocking/advisory de
`rule.Advisory` — preferir derivar, é mais fiel). Atualizar teste que cobre batch.

**Verify**: `go test ./internal/check/providers/customrules/... -count=1` → passa

### Step 3: glob-miss e arquivos ilegíveis são visíveis

Em `customrules.go`: `Analyze` acumula por glob: nº de arquivos matched e nº de
arquivos pulados por erro de leitura. Regra cujo glob casou 0 arquivos ⇒ adicionar a
`CheckResult.Details`: `"rule '<id>': glob '<files>' matched 0 files — check the
pattern (supported: *, **, ?, {a,b}; no […])"`. Erro de leitura ⇒ detail com path +
erro. Status permanece passed (não é violação), mas o detalhe aparece no relatório.

**Verify**: teste com glob que casa nada ⇒ Details contém a linha com o id da regra

### Step 4: exclude por path relativo

`filewalker.go`: distinguir entrada de diretório (termina com `/`) de pattern de
arquivo. Diretório: casa quando o path relativo ao projectDir tem esse sufixo de
diretório (ex.: `src/generated/` exclui `src/generated/**` e não `pkg/generated`).
Arquivo: casar via `matchGlob` contra o path relativo completo (`*.min.js` passa a
funcionar). Manter os excludes hardcoded (`:18-26`). Atualizar os testes do Step 6
para cobrir os dois casos + o falso-positivo atual (`generated` em outro lugar NÃO
é mais excluído).

**Verify**: teste: `cfg.Exclude = ["*.min.js", "src/generated/"]` ⇒ `a.min.js` fora, `src/generated/x.ts` fora, `pkg/generated/y.ts` dentro

### Step 5: deletar caminho morto semgrep single-rule

Remover `matchSemgrep` e `buildSemgrepConfigFile` (`matcher.go:339-473`); repontar
os 3 testes (`customrules_test.go:941,981,1027`) para `matchSemgrepBatch`/
`buildSemgrepBatchConfig` preservando a intenção de cada assert. Se algum assert
depende de comportamento só existente no single (não deveria — batch é
superset), reportar no STOP.

**Verify**: `go test ./internal/check/providers/customrules/... -count=1` → passa; `grep -n "func matchSemgrep(" internal/check/providers/customrules/matcher.go` → sem resultados

### Step 6: testes diretos do walker

`filewalker_test.go` (novo): `WalkFiles` — exclude dir hardcoded; `cfg.Exclude`
dir e file glob (Step 4); arquivo >1MB pulado; symlink pulado; profundidade;
`matchGlob` casos (incluindo `{a,b}` e a ausência de `[...]`); `IsTextFile` rejeita
binário (null byte).

**Verify**: `go test ./internal/check/providers/customrules/... -count=1 -race` → exit 0

## Test plan

- `config_test.go`: yaml quebrado ⇒ erro com path (Step 1).
- `customrules_test.go`: batch remediation; glob-miss details; 3 testes repontados (Step 5).
- `filewalker_test.go`: cobertura nova do Step 6 (fecha finding TEST-02 da auditoria).
- Padrão: fixtures `t.TempDir()` + os helpers existentes do pacote.

## Done criteria

- [ ] `go test ./... -count=1 -race` exit 0
- [ ] `.qualitygate.yaml` com YAML inválido ⇒ `crivo run` sai não-zero citando o arquivo
- [ ] Regra com glob que casa 0 arquivos aparece nos Details (teste)
- [ ] `*.min.js` no exclude tem efeito real (teste)
- [ ] `grep -c "func matchSemgrep(" ...matcher.go` → 0
- [ ] `git status` limpo fora do in-scope; `plans/README.md` atualizado

## STOP conditions

- Callers de `config.Load` além do `main.go` dependerem do retorno sem erro.
- Algum teste do caminho single-rule codificar comportamento que o batch não tem
  (reportar qual — pode ser bug adicional).
- Mudança de exclude quebrar o dogfood (`.qualitygate.yaml` do próprio crivo) de
  forma não intencional — rodar `crivo run` no repo e comparar.

## Maintenance notes

- O plano 008 (fixtures) usa os matchers — os details do Step 3 são a base do
  diagnóstico de regra que não casa nada.
- A mudança de exclude é a única com risco de behavior change em configs existentes;
  destacar no PR.

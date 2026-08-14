# Plan 008: Custom rules state of the art — fixtures de regra, template honesto, spike de packs

> **Executor instructions**: Follow this plan step by step. Run every verification
> command and confirm the expected result before moving to the next step. If anything
> in the "STOP conditions" section occurs, stop and report — do not improvise. When
> done, update the status row for this plan in `plans/README.md`.
>
> **Drift check (run first)**: `git diff --stat f3b5007..HEAD -- internal/config/config.go internal/check/providers/customrules/ templates/ cmd/crivo/main.go`
> Compare "Current state" with live code; on mismatch, STOP.

## Status

- **Priority**: P3
- **Effort**: M/L
- **Risk**: MED (additions de schema precisam ser backward compatible com YAML existente)
- **Depends on**: plans/006-customrules-trust.md (motor honesto antes de validar regras sobre ele)
- **Category**: direction
- **Planned at**: commit `f3b5007`, 2026-08-14

## Why this matters

As custom rules são o moat do crivo — e hoje não há como **verificar uma regra
antes dela virar gate**: sem fixtures, uma regex broad demais ou um pattern semgrep
inválido só se descobre quando silencia (plano 002/006 reduzem o silêncio, mas o
autor da regra continua sem feedback local). O template `crivo init` entrega um
schema **legado** que o binário ignora (`lint`, `structural`, `testExclude`) — a
ferramenta ensina o usuário a configurar errado. E cada projeto re-deriva as mesmas
regras (`no-eval`, `no-innerhtml`) que a própria skill do crivo usa de exemplo. Este
plano fecha as três pontas: regras testáveis no compile, template verdadeiro, e um
spike de packs versionadas.

**Decisão registrada a honrar**: autofix/suggestion em custom rules é explicitamente
out of scope por design (`docs/custom-rules-design.md:255`). NÃO propor autofix aqui.

## Current state

- `internal/config/config.go:84-107` — `CustomRule`: id, type, pattern, packages,
  max-lines, allow-in, files, message, severity, when-pattern, must-import-from,
  ignore-comments, ignore-tests, allow-subpaths, mode, language + campos semgrep
  (pattern-not, pattern-inside, pattern-not-inside, metavariable-regex). Sem campo
  de testes/fixtures.
- `internal/check/providers/customrules/rule.go:52-200` — `CompileRules` valida e
  pré-compila colecionando todos os erros (padrão a seguir); erros viram
  `StatusError` visível (`customrules.go:60-71`).
- `templates/qualitygate.json:3-40` — schema legado (chaves `lint`, `structural`,
  `testExclude`, `srcPattern`, `complexity.cyclomatic/cognitive`) que não existem em
  `ChecksConfig`/`ComplexityConfig` (`config.go:37-107`); sem `custom-rules`.
  `config.GenerateDefault()` referencia-o (`config.go:206-209` — conferir no drift
  check como o template é emitido no `crivo init` atual).
- Match centers: `matcher.go` — `matchBanImport`, `matchBanPattern`,
  `matchRequireImport`, `matchEnforcePattern`, `matchMaxLines`, `matchSemgrepBatch`.
- `.claude/skills/custom-rules/SKILL.md:196-235` — exemplos canônicos de regras
  (`no-eval`, `no-innerhtml`) que serão o seed do pack.
- Convenções: yaml.Unmarshal tolera campos desconhecidos (schema additions são
  backward compat); testes in-package table-driven.

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Testes alvo | `go test ./internal/config/... ./internal/check/providers/customrules/... ./cmd/... -count=1` | exit 0 |
| Testes completos | `go test ./... -count=1 -race` | exit 0 |
| Vet | `go vet ./...` | exit 0 |

## Scope

**In scope**:
- `internal/config/config.go` (campo `tests` em CustomRule) + testes
- `internal/check/providers/customrules/rule.go`, `matcher.go` (entry point de
  validação de fixtures) + testes
- `templates/qualitygate.json` (regenerar) + teste de aderência ao schema
- `internal/packs/` (novo, só o spike do Step 4) + `config.go` (`include:` opcional)
- `README.md` / `docs/custom-rules-design.md` (documentar fixtures e packs)

**Out of scope**:
- Autofix (decisão registrada — não reabrir).
- Registry remota de packs (o spike é embedded/local file).
- UI/TUI para regras.

## Git workflow

- Branch: `feat/customrules-fixtures-packs`
- Commits: `feat(customrules): per-rule test fixtures validated at compile time`; `fix(templates): regenerate qualitygate template from real schema`; `feat(spike): embedded rule packs via include:`

## Steps

### Step 1: schema de fixtures

`CustomRule` ganha campo opcional:

```yaml
tests:
  - code: "const x = eval(userInput)"     # deve casar (match: true é default)
    match: true
  - code: "const x = evaluate(userInput)" # não deve casar
    match: false
```

`TestSpec { Code string; Match bool }`. YAML existente sem `tests` permanece válido.

**Verify**: `go test ./internal/config/... -count=1` → parse do campo novo + retrocompat

### Step 2: CompileRules valida fixtures

Em `CompileRules` (após pré-compilação): para cada regra com `tests`, rodar cada
spec contra o matcher da regra usando `Code` como conteúdo de arquivo sintético
(para ban-pattern/ban-import/require-import/enforce-pattern/max-lines — reutilizar
as funções de match com um pseudo-arquivo em memória; refatorar o mínimo: extrair
`matchLines(rule, filename, lines) []Issue` se necessário). Semgrep rules: validar
fixtures APENAS se o binário estiver disponível; sem binário ⇒ warning colecionado
(não erro — coerente com 002). Spec que discorda do matcher ⇒ **erro de compile**
com: id da regra, o code, esperado vs obtido. Padrão de mensagem: seguir os erros
existentes de CompileRules.

**Verify**: teste — regra ban-pattern com fixture errada ⇒ CompileRules retorna erro citando a regra; fixture certa ⇒ compila

### Step 3: template regenerado

Regenerar `templates/qualitygate.json` a partir do schema real: as chaves devem
casar 1:1 com `ChecksConfig`/`CoverageConfig`/`DuplicationConfig`/`CustomRule`
(incluir um exemplo de custom-rule com `tests`). Teste novo em `config_test.go`:
unmarshal do template no `Config` + `CompileRules` nas regras exemplo ⇒ zero erros
(garante que o template nunca mais diverge do binário).

**Verify**: `go test ./internal/config/... -count=1` → teste de aderência passa

### Step 4: spike de packs (timeboxed)

1. Novo campo top-level opcional `include: ["./relative-rules.yaml", "pack:security-ts"]`.
2. Resolver em `config.Load`: caminho relativo ⇒ ler yaml de rules e concatenar em
   `cfg.CustomRules` (IDs duplicados ⇒ erro de compile — CompileRules já exige
   únicos). `pack:<name>` ⇒ embedded FS `internal/packs/<name>.yaml` via
   `go:embed`. Seed: `security-ts` com `no-eval`, `no-innerhtml`,
   `no-dangerouslysetinnerhtml` (portar dos exemplos da skill, com `tests`).
3. Documentar em `docs/custom-rules-design.md` + README: packs são embedded e
   versionadas com o binário (registry remota é decisão futura).

**STOP do spike**: se o passo 2 revelar complexidade >1 dia (resolução de paths,
interação com profiles), entregar Steps 1–3 + documentar o design do pack como
RFC curto em `docs/` e encerrar o plano — os Steps 1–3 já são valor completo.

**Verify**: `go test ./internal/config/... -count=1` → include relativo e pack embutido funcionam; id duplicado entre pack e local ⇒ erro

## Test plan

- `config_test.go`: parse de `tests`; `include` relativo; pack embedded; template aderente (Step 3).
- `rule_test.go`/`customrules_test.go`: fixture passando/falhando por tipo de regra
  (ban-pattern, ban-import, max-lines); semgrep com binário ausente ⇒ warning.
- `internal/packs/security-ts.yaml` compilável (teste inclui o pack via include e
  roda CompileRules).

## Done criteria

- [ ] `go test ./... -count=1 -race` exit 0
- [ ] Regra com fixture que erra ⇒ `crivo run` falha na compilação citando regra e caso (teste)
- [ ] YAML antigo sem `tests`/`include` compila inalterado (teste de retrocompat)
- [ ] Template novo passa no teste de aderência ao schema
- [ ] `docs/custom-rules-design.md` documenta fixtures (+ packs se o spike completar)
- [ ] `git status` limpo fora do in-scope; `plans/README.md` atualizado

## STOP conditions

- Matchers não poderem rodar sobre conteúdo em memória sem refactor maior que
  "extrair matchLines" (reportar o acoplamento encontrado).
- `templates/qualitygate.json` ter consumidores além do `crivo init` que dependam
  do schema legado (grep antes).
- O campo `tests` colidir com semântica existente de `CustomRule`.

## Maintenance notes

- A skill `.claude/skills/custom-rules/SKILL.md` deve passar a gerar regras COM
  fixtures (follow-up imediato pós-merge — é onde o moat se multiplica).
- Packs embutidas significam: bump de pack = release do crivo (aceitável no spike;
  registry remota é a evolução).

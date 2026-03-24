# Agent Code Gate — Roadmap de Checks

> Objetivo: barrar deterministicamente código ruim gerado por agentes de IA.
> Princípios: simples, estável, opinativo, determinístico.

---

## Quick Wins (baixa complexidade, alto impacto)

### 1. Test Runner (pass/fail)
**Problema:** Crivo checa cobertura mas não se os testes passam. Agente altera código e quebra 15 testes — não é barrado.

**Solução:** Provider que roda `npm test` (ou comando configurável), parseia exit code e quantidade de testes pass/fail.

**Output exemplo:**
```
Tests: 3 failing (were 0) → FAIL
Tests: 142 passing → PASS
```

**Severidade:** blocker — testes falhando = gate falha sempre.

---

### 2. Build Check
**Problema:** `tsc --noEmit` checa tipos, mas não garante que o build real (next build, vite build, etc.) funciona. Agente pode quebrar imports dinâmicos, configs, env vars.

**Solução:** Provider que roda o build command do projeto (`npm run build` ou configurável), parseia exit code.

**Output exemplo:**
```
Build: failed (exit code 1) → FAIL
Build: success (12.3s) → PASS
```

**Severidade:** blocker.

---

### 3. Diff Size Guard
**Problema:** Agentes despejam código demais — arquivos enormes, funções gigantes, diffs massivos. Sinal claro de código não-revisado.

**Solução:** Provider que analisa `git diff --stat` e aplica limites opinativos:
- Arquivo novo com >400 linhas → warning
- Função com >50 linhas → warning (complementa complexity check)
- Diff total com >1000 linhas modificadas → warning
- Arquivo individual com >300 linhas modificadas → warning

**Configuração:**
```yaml
diff-guard:
  max-new-file-lines: 400
  max-diff-lines: 1000
  max-file-diff-lines: 300
```

**Severidade:** warning (não bloqueia, mas avisa).

---

## Médio Prazo (complexidade média)

### 4. Dependency Audit
**Problema:** Agentes adoram adicionar pacotes npm sem critério. Deps novas podem ser inseguras, abandonadas, ou redundantes.

**Solução:** Provider que detecta deps novas no `package.json` (via git diff) e roda verificações:
- `npm audit` / `yarn audit` para vulnerabilidades conhecidas
- Último publish > 2 anos → warning (possivelmente abandonado)
- Downloads/semana < 100 → warning (pouco adotado)

**Configuração:**
```yaml
dependency-audit:
  block-on-vulnerabilities: critical  # critical, high, moderate, low
  warn-abandoned-months: 24
  warn-min-weekly-downloads: 100
```

**Severidade:** vulnerabilidades critical/high = blocker, resto = warning.

---

### 5. Test Existence Check
**Problema:** Agente cria `src/services/payment.ts` mas não cria teste correspondente. Código novo sem teste = dívida técnica instantânea.

**Solução:** Provider que detecta arquivos novos em `src/` (via git diff) e verifica se existe teste correspondente. Matching configurável:
- `src/foo.ts` → espera `__tests__/foo.test.ts` ou `src/foo.test.ts` ou `src/foo.spec.ts`
- Ignora arquivos de tipo/config (`.d.ts`, `index.ts` barrel exports, etc.)

**Configuração:**
```yaml
test-existence:
  patterns:
    - "{dir}/__tests__/{name}.test.{ext}"
    - "{dir}/{name}.test.{ext}"
    - "{dir}/{name}.spec.{ext}"
  ignore:
    - "*.d.ts"
    - "index.ts"
    - "types.ts"
```

**Severidade:** warning em `--new-code`, ignorado no scan geral (não penalizar código legado).

---

## Longo Prazo (complexidade alta)

### 6. Import Consistency
**Problema:** Agente importa `axios` quando o projeto usa `fetch`, ou `moment` quando já tem `date-fns`. Cria inconsistência e deps redundantes.

**Solução:** Provider que detecta padrões de import do projeto e flag anomalias:
- Libs com mesmo propósito (ex: `axios` + `fetch`, `lodash` + `ramda`)
- Imports que fogem do padrão dominante do projeto
- Requer mapeamento de "categorias de lib" (HTTP client, date, state management, etc.)

**Complexidade alta** porque precisa entender semântica das libs. Pode começar com uma lista hardcoded de conflitos comuns.

**Severidade:** warning.

---

## Ideias de UX/CLI (vindas da análise do tla-precheck)

### `crivo doctor`
Verifica quais ferramentas externas estão instaladas e funcionando (tsc, eslint, jest, jscpd, semgrep, gitleaks, knip). Mostra versões e quais checks vão funcionar.

### `crivo init` melhorado
Detecta ferramentas instaladas e já configura `.qualitygate.yaml` com checks habilitados apenas para o que existe. Sugere instalação do que falta.

### Estimativa pré-execução
Antes de rodar, mostra: "5 checks habilitados, ~2300 arquivos, estimativa ~15s". Feedback imediato.

---

## Resumo de prioridades

| Prioridade | Check | Tipo | Bloqueia? |
|-----------|-------|------|-----------|
| 🔴 P0 | Test Runner | quick win | sim |
| 🔴 P0 | Build Check | quick win | sim |
| 🟡 P1 | Diff Size Guard | quick win | não (warning) |
| 🟡 P1 | Dependency Audit | médio prazo | sim (vulns) |
| 🟢 P2 | Test Existence | médio prazo | não (warning) |
| 🟢 P2 | Import Consistency | longo prazo | não (warning) |
| 🔵 DX | `crivo doctor` | UX | n/a |
| 🔵 DX | `crivo init` melhorado | UX | n/a |

# Custom Rules — Design Document

> Transformar regras de projeto (CLAUDE.md, AGENT.md, .cursorrules) em checks determinísticos do Crivo.

## Visão Geral

Dois componentes:
1. **`custom-rules` provider** no Crivo — novo provider que lê regras do `.qualitygate.yaml` e aplica via grep/regex
2. **`/crivo-rules` skill** no Claude Code — lê docs do projeto, extrai regras, gera config pro Crivo

---

## Parte 1: Custom Rules Provider

### Tipos de regra suportados

#### `ban-import` — Proibir imports específicos
```yaml
custom-rules:
  - id: no-date-fns
    type: ban-import
    packages: ["date-fns", "moment", "dayjs"]
    message: "Use date-utils.ts ao invés de libs externas de data"
    severity: blocker
    files: "**/*.{ts,tsx}"  # opcional, default = src/**
```

**Como funciona:** grep por `import .* from ['"]date-fns`, `require\(['"]moment`. Determinístico, zero falso positivo.

**Exemplo real (agenda-aqui):** Projeto usa `date-utils.ts` custom → ban `date-fns`, `moment`, `dayjs`.

---

#### `ban-pattern` — Proibir padrões de código
```yaml
  - id: no-raw-date
    type: ban-pattern
    pattern: "new Date\\("
    allow-in: ["date-utils.ts", "**/*.test.ts"]  # exceções
    message: "Use parseUTCString() ou createLocalDate() de date-utils.ts"
    severity: blocker
    files: "src/**/*.{ts,tsx}"
```

**Como funciona:** grep com regex. `allow-in` define arquivos onde o padrão é permitido (o wrapper em si, testes).

**Exemplo real (agenda-aqui):** `new Date()` proibido em tudo exceto `date-utils.ts`.

---

#### `require-import` — Se usar padrão X, deve importar de Y
```yaml
  - id: dates-from-utils
    type: require-import
    when-pattern: "(formatDate|parseUTCString|createLocalDate)"
    must-import-from: "@/shared/utils/date-utils"
    message: "Funções de data devem vir de date-utils.ts"
    severity: major
    files: "src/**/*.{ts,tsx}"
```

**Como funciona:** Se o arquivo contém o pattern, verifica se tem import do módulo correto.

---

#### `enforce-pattern` — Em arquivos matching, padrão deve existir
```yaml
  - id: api-rate-limit
    type: enforce-pattern
    files: "src/app/api/**/route.ts"
    pattern: "applyRateLimit"
    message: "Rotas de API públicas precisam de rate limiting"
    severity: major
```

**Como funciona:** Para cada arquivo que match o glob, verifica se o pattern existe. Se não, issue.

**Exemplo real (agenda-aqui):** Toda route.ts deve chamar `applyRateLimit()`.

---

#### `ban-dependency` — Proibir deps no package.json
```yaml
  - id: no-axios
    type: ban-dependency
    packages: ["axios", "got", "node-fetch", "superagent"]
    message: "Projeto usa native fetch. Não adicionar HTTP clients."
    severity: blocker
```

**Como funciona:** Lê `package.json`, checa se algum package proibido está em dependencies/devDependencies.

---

### Config completa de exemplo (agenda-aqui)

```yaml
# .qualitygate.yaml
custom-rules:
  # === DATAS ===
  - id: no-date-libs
    type: ban-import
    packages: ["date-fns", "moment", "dayjs", "luxon"]
    message: "Use src/shared/utils/date-utils.ts para manipulação de datas"
    severity: blocker

  - id: no-raw-date
    type: ban-pattern
    pattern: "new Date\\("
    allow-in: ["**/date-utils.ts", "**/*.test.ts", "**/*.spec.ts"]
    message: "Use createLocalDate() ou parseUTCString() de date-utils.ts"
    severity: blocker

  # === HTTP ===
  - id: no-axios
    type: ban-dependency
    packages: ["axios", "got", "node-fetch"]
    message: "Projeto usa native fetch"
    severity: blocker

  # === SEGURANÇA ===
  - id: api-rate-limit
    type: enforce-pattern
    files: "src/app/api/**/route.ts"
    pattern: "applyRateLimit"
    message: "Rotas de API públicas precisam de rate limiting"
    severity: major

  # === VALORES ===
  - id: cents-only
    type: ban-pattern
    pattern: "price\\s*[:=]\\s*\\d+\\.\\d+"
    message: "Valores monetários devem ser em centavos (inteiro), não decimais"
    severity: major
    files: "src/**/*.{ts,tsx}"

  # === DATABASE ===
  - id: no-raw-ids
    type: ban-pattern
    pattern: "id\\s*:\\s*(number|integer|INT |SERIAL)"
    message: "IDs devem ser UUID, não integer/serial"
    severity: major
    files: "**/*.{ts,sql}"
```

---

### Implementação no Go

```
internal/check/providers/customrules/
├── customrules.go      # Provider: Name, ID, Detect, Analyze
├── rule.go             # Struct Rule + parsing da config
├── matcher.go          # Lógica de grep: ban-import, ban-pattern, etc.
└── customrules_test.go
```

**Provider interface:**
- `Name()` → "Custom Rules"
- `ID()` → "custom-rules"
- `Detect()` → true se config tem `custom-rules` com pelo menos 1 regra
- `Analyze()` → itera cada regra, aplica matcher, coleta issues

**Cada issue gerada inclui:**
- `RuleID`: o `id` da regra custom (ex: `no-raw-date`)
- `Message`: a mensagem configurada
- `File` + `Line`: onde o pattern foi encontrado
- `Severity`: do config
- `Type`: `code_smell` (default) ou configurável
- `Source`: `"custom-rules"`

---

## Parte 2: Skill `/crivo-rules`

### O que faz

1. **Descobre docs do projeto** — escaneia:
   - `CLAUDE.md`
   - `AGENT.md`
   - `.cursorrules`
   - `.github/copilot-instructions.md`
   - `.claude/lessons-learned/*.md`
   - `docs/CONVENTIONS.md`, `docs/RULES.md`
   - Qualquer arquivo com padrões `NEVER`, `ALWAYS`, `must`, `do not`

2. **Extrai regras** — identifica frases normativas e classifica:
   - "NEVER use X" → `ban-import` ou `ban-pattern`
   - "ALWAYS use X from Y" → `require-import`
   - "Every X must have Y" → `enforce-pattern`
   - "Do not add X" → `ban-dependency`

3. **Gera config** — produz bloco `custom-rules` para `.qualitygate.yaml`

4. **Instrui o CLI** — mostra ao agente como validar:
   ```bash
   crivo run --verbose  # roda com as novas regras
   ```

### Frontmatter da skill

```yaml
---
name: crivo-rules
description: >
  Scan project documentation (CLAUDE.md, AGENT.md, .cursorrules, etc.)
  to extract conventions and generate custom-rules config for the Crivo
  quality gate. Use when setting up crivo in a new project or when
  project conventions changed.
allowed-tools: Read, Write, Edit, Bash, Glob, Grep
user-invocable: true
argument-hint: [generate|list|validate]
---
```

### Subcomandos

- **`generate`** (default) — escaneia docs, gera/atualiza `custom-rules` no `.qualitygate.yaml`
- **`list`** — mostra regras existentes no config e status (quantos matches cada uma tem)
- **`validate`** — roda `crivo run` e mostra quais custom rules passaram/falharam

---

## Fluxo completo (exemplo)

```
Usuário: /crivo-rules generate

Skill:
1. Lê CLAUDE.md do agenda-aqui
2. Encontra: "NEVER use new Date() directly"
3. Encontra: "ALWAYS use date-utils.ts"
4. Encontra: "Use applyRateLimit() for public API routes"
5. Gera:
   custom-rules:
     - id: no-raw-date
       type: ban-pattern
       pattern: "new Date\\("
       allow-in: ["**/date-utils.ts"]
       message: "Use date-utils.ts"
       severity: blocker
     ...
6. Escreve no .qualitygate.yaml
7. Roda: crivo run --verbose
8. Output: "Custom Rules: 3 rules checked, 0 violations → PASS"
```

---

## O que NÃO faz

- Não usa LLM em runtime — regras são regex/glob puro, determinísticas
- Não tenta entender semântica profunda — se a regra não traduz pra pattern, pula
- Não substitui ESLint custom rules — é complementar, focado em convenções de projeto
- Não auto-fix — apenas detecta e reporta

---

## Trade-offs

| Decisão | Alternativa | Por que essa |
|---------|-------------|-------------|
| Regex puro | AST parsing | Simples, zero deps, funciona cross-language |
| Config em YAML | DSL própria | Consistente com resto do crivo |
| Skill gera, CLI executa | Tudo no CLI | Skill usa LLM pra interpretar docs, CLI é determinístico |
| 5 tipos de regra | Extensível | Cobre 90% dos casos reais, YAGNI pro resto |

---

## Sequência de implementação

1. **Custom Rules provider** no Go (o motor) — 1-2 dias
   - Struct de regra + parsing YAML
   - Matchers: ban-import, ban-pattern, require-import, enforce-pattern, ban-dependency
   - Testes unitários
   - Registrar no registry

2. **Skill `/crivo-rules`** no Claude Code — 1 dia
   - Prompt que lê docs e gera config
   - Template de regras comuns

3. **Docs + exemplos** — meio dia
   - Seção no README
   - Exemplo com agenda-aqui como case

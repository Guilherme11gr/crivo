# crivo

Binário Go que roda ferramentas de análise existentes (tsc, eslint, jest, jscpd, semgrep, gitleaks, knip), normaliza a saída, calcula ratings e diz se o código tá pronto pra subir.

Sem servidor, sem setup de banco, sem conta. Só `crivo run`.

## Instalação

```bash
npm install -g crivo
```

Ou via Go:

```bash
go install github.com/guilherme11gr/crivo/cmd/crivo@latest
```

## Uso

```bash
crivo init                    # Cria config, workflow do GitHub Actions e skills do Claude Code
crivo run                     # Roda todos os checks
crivo run --json              # JSON estruturado (pra CI/agentes de IA)
crivo run --verbose           # Detalhes completos
crivo run --tui               # Dashboard interativo
crivo run --new-code          # Só arquivos alterados (pra PRs)
crivo run --disable complexity # Desabilita checks específicos nesta execução
crivo run --save              # Salva no histórico local pra acompanhar tendências
crivo run --md report.md      # Saída em Markdown (pra comentários em PR)
crivo run --sarif report.sarif # SARIF 2.1.0 (pra GitHub Code Scanning)
crivo trends                  # Histórico com sparklines dos últimos runs
```

## O que analisa

| Check | Ferramenta | O que encontra |
|-------|------------|----------------|
| Type Safety | tsc | Erros de compilação TypeScript (separa prod de testes) |
| Lint | eslint | Problemas de qualidade, warnings |
| Cobertura | jest | Cobertura de linhas, branches, funções, statements |
| Duplicação | jscpd | Blocos de copy-paste acima do threshold |
| Complexidade | Análise AST | Complexidade cognitiva por função |
| Secrets | gitleaks | Credenciais e chaves hardcoded |
| Segurança | semgrep | Padrões de vulnerabilidade (SAST) |
| Código morto | knip | Exports, arquivos e dependências não utilizados |
| Custom Rules | regex + semgrep | Regras customizadas definidas no config |

Os checks rodam em paralelo. Cada um é opcional — o crivo só executa o que faz sentido pro seu projeto (detecta tsconfig.json, config do eslint, jest no package.json, etc).

## Ratings

Três dimensões, de A até E, inspiradas no modelo do SonarQube:

- **Reliability** — baseado na quantidade e severidade de bugs
- **Security** — baseado em vulnerabilidades e hotspots
- **Maintainability** — baseado na razão de dívida técnica

## Políticas de gate

Controla o que bloqueia seu pipeline:

| Política | Bloqueia em |
|----------|-------------|
| `release` (padrão) | Erros de tipo, erros de lint, secrets |
| `strict` | Tudo — cobertura, duplicação, qualquer check falhando |
| `informational` | Nada — só reporta |

```yaml
# .qualitygate.yaml
gate-policy: release
```

Ou override por execução:

```bash
crivo run --policy strict
```

## Configuração

`crivo init` cria um `.qualitygate.yaml` com defaults razoáveis. Exemplo:

```yaml
profile: balanced
gate-policy: release
languages:
  - typescript

src:
  - src/

exclude:
  - node_modules/
  - dist/
  - coverage/

checks:
  typescript: true
  eslint: true
  coverage: true
  duplication: true
  secrets: false
  semgrep: false
  dead-code: false
  custom-rules: true

coverage:
  lines: 60
  branches: 50
  functions: 60
  statements: 60

duplication:
  threshold: 5
  min-lines: 5
  min-tokens: 50

complexity:
  threshold: 15
```

## Formatos de saída

**Terminal** — UI com box-drawing colorido, ratings, resumo dos checks e contagem de issues.

**JSON** (`--json`) — saída estruturada completa. Útil pra agentes de IA ou integrações customizadas:

```json
{
  "status": "passed",
  "checks": [
    {
      "id": "typescript",
      "status": "warning",
      "summary": "0 prod errors, 34 in tests",
      "metrics": { "prod_errors": 0, "test_errors": 34, "errors": 34 }
    }
  ],
  "ratings": { "Reliability": "A", "Security": "A", "Maintainability": "A" },
  "conditions": [...]
}
```

**Markdown** (`--md report.md`) — tabelas e lista de issues, pronto pra comentário de PR.

**SARIF** (`--sarif report.sarif`) — SARIF 2.1.0, compatível com GitHub Code Scanning.

## TUI

`crivo run --tui` abre um dashboard interativo com três abas:

- **Dashboard** — status do gate, ratings, resultado dos checks
- **Issues** — lista navegável com filtro por tipo (bugs, vulnerabilidades, code smells)
- **Trends** — sparklines de issues, cobertura e duplicação ao longo do tempo

Navegação: Tab, setas, `q` pra sair.

## Comparação com baseline

Quando você usa `--save`, o crivo guarda os resultados num SQLite local (`.qualitygate/history.db`). Nas execuções seguintes, compara com o último run salvo:

- Cobertura/complexidade que não pioraram são rebaixadas de "failed" pra "warning" (tolerância a dívida legada)
- Regressões reais são sinalizadas explicitamente

Isso significa que projetos existentes não são punidos por dívida histórica — só regressões novas bloqueiam o gate.

## Integração com CI

`crivo init` cria um workflow de GitHub Actions pronto. O fluxo:

1. Roda `crivo run --md report.md --sarif report.sarif --save`
2. Faz upload do SARIF pro GitHub Code Scanning
3. Posta o markdown como comentário no PR

Pra outros sistemas de CI, o crivo retorna exit code 1 quando o gate falha, 0 quando passa.

## Suporte a linguagens

| Feature | TypeScript/JS | Go | Python |
|---------|:---:|:---:|:---:|
| Type checking | tsc | — | — |
| Lint | eslint | — | — |
| Cobertura | jest | — | — |
| Duplicação | jscpd | jscpd | jscpd |
| Complexidade | AST | regex | regex |
| Secrets | gitleaks | gitleaks | gitleaks |
| Segurança | semgrep | semgrep | semgrep |
| Código morto | knip | — | — |

Projetos TypeScript/JavaScript têm a cobertura mais completa. Go e Python recebem duplicação, complexidade (regex), secrets e segurança.

## Custom Rules

Regras customizadas direto no `.qualitygate.yaml`. Regex-based ou AST-based (via semgrep):

```yaml
custom-rules:
  # Proíbe imports diretos — force uso de wrappers
  - id: no-date-libs
    type: ban-import
    packages: ["date-fns", "moment", "dayjs"]
    allow-subpaths: ["locale"]
    allow-in: ["**/shared/utils/date-utils.ts"]
    message: "Use date-utils wrapper"
    severity: blocker

  # Bloqueia patterns com regex
  - id: no-console-log
    type: ban-pattern
    pattern: "console\\.(?:log|debug)\\("
    files: "src/**/*.{ts,tsx}"
    message: "Use logger ao invés de console.log"
    severity: major

  # Semgrep — match semântico com AST
  - id: no-manual-cents
    type: semgrep
    pattern: "$X / 100"
    pattern-not-inside: "function centsToReais(...) { ... }"
    metavariable-regex:
      "$X": ".*[Cc]ents.*"
    message: "Use centsToReais()"
    severity: major

  # Bloqueia dependências no package.json
  - id: no-axios
    type: ban-dependency
    packages: ["axios", "got", "node-fetch"]
    message: "Projeto usa fetch nativo"
    severity: blocker

  # Limita tamanho de arquivo
  - id: component-max-300
    type: max-lines
    max-lines: 300
    files: "src/components/**/*.tsx"
    message: "Componente muito grande, extraia subcomponentes"
    severity: major
    mode: advisory
```

**8 tipos de regra:** `ban-import`, `ban-pattern`, `require-import`, `enforce-pattern`, `ban-dependency`, `max-lines`, `semgrep`

**Smart defaults:** `ignore-comments` e `ignore-tests` são `true` por padrão para ban-pattern/ban-import.

**Advisory mode:** `mode: advisory` reporta mas não bloqueia o gate — útil pra rollout gradual.

## Como funciona

O crivo não implementa nenhuma análise. Ele orquestra ferramentas existentes:

1. Lê `.qualitygate.yaml` pra configuração
2. Detecta quais checks se aplicam (baseado nos arquivos do projeto)
3. Roda as ferramentas aplicáveis em paralelo
4. Parseia a saída de cada ferramenta num formato normalizado
5. Calcula ratings A-E
6. Avalia as condições do gate conforme a política configurada
7. Gera a saída no formato escolhido

Zero CGO. Binário único. Sem dependências de runtime além das ferramentas em si (node, go, python — o que seu projeto já usa).

## Licença

MIT

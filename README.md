# crivo

Quality gate local para fluxos com agentes de IA e CI. O `crivo` roda checks básicos, consolida a saída num formato único e aplica regras customizadas do projeto para barrar código ruim no fim de uma sessão de coding ou antes do merge.

Sem servidor, sem conta, sem stack paralela de observabilidade. Só `crivo run`.

## Pra quem é

- Times que já usam Claude Code, OpenCode ou outros agentes para escrever código
- Projetos que precisam de um gate simples no fim da sessão do agente
- Pipelines de CI que precisam bloquear regressões óbvias e regras específicas do time

O diferencial do `crivo` não é substituir uma plataforma inteira de code quality. É ser a camada final de verificação: tipos, cobertura, duplicação, secrets, segurança e regras customizadas do projeto num único resultado.

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
crivo run                     # Roda o gate completo
crivo run --json              # JSON estruturado (pra agentes/automação)
crivo run --verbose           # Detalhes completos
crivo run --tui               # Dashboard interativo
crivo run --new-code          # Só código alterado (pra PRs/CI)
crivo run --disable complexity # Desabilita checks específicos nesta execução
crivo run --save              # Salva no histórico local pra acompanhar tendências
crivo run --md report.md      # Saída em Markdown (pra comentários em PR)
crivo run --sarif report.sarif # SARIF 2.1.0 (pra GitHub Code Scanning)
crivo trends                  # Histórico com sparklines dos últimos runs
```

## Como usar com agents

Fluxo típico no fim da sessão:

1. O agente implementa a mudança
2. Roda `crivo run --json` para ter um resultado estruturado
3. Corrige os problemas bloqueantes
4. Opcionalmente roda `crivo run --md report.md --sarif report.sarif` no CI

Exemplo de uso local:

```bash
crivo run --json
```

Exemplo de uso no CI:

```bash
crivo run --new-code --md report.md --sarif report.sarif --save
```

## O que analisa

| Check | Ferramenta | O que encontra |
|-------|------------|----------------|
| Type Safety | tsc | Erros de compilação TypeScript (separa prod de testes) |
| Cobertura | jest/vitest | Cobertura de linhas, branches, funções, statements |
| Duplicação | jscpd + análise semântica | Copy-paste e clones estruturais |
| Complexidade | AST + fallback regex | Complexidade cognitiva por função |
| Secrets | gitleaks | Credenciais e chaves hardcoded |
| Segurança | semgrep | Padrões de vulnerabilidade (SAST) |
| Código morto | knip | Exports, arquivos e dependências não utilizados |
| Custom Rules | regex + semgrep | Regras customizadas definidas no config |

Os checks rodam em paralelo. Cada um é opcional: o `crivo` só executa o que faz sentido pro projeto, baseado nos arquivos detectados e na configuração.

## Onde ele encaixa

O `crivo` é mais útil em dois pontos do fluxo:

1. No fim da sessão do agente, antes de considerar a tarefa concluída
2. No CI, como gate de PR ou push

Ele existe para responder uma pergunta simples: o agente entregou algo aceitável para esse repositório?

## Ratings

Três dimensões, de A até E:

- **Reliability** — baseado na quantidade e severidade de bugs
- **Security** — baseado em vulnerabilidades e hotspots
- **Maintainability** — baseado na razão de dívida técnica

## Políticas de gate

Controla o que bloqueia seu pipeline:

| Política | Bloqueia em |
|----------|-------------|
| `release` (padrão) | Erros de tipo em produção, secrets |
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

**JSON** (`--json`) — saída estruturada completa. Útil para agentes decidirem se a sessão terminou, quais issues corrigir primeiro e o que bloqueia o gate:

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

**Markdown** (`--md report.md`) — pronto pra comentário de PR ou resumo legível no CI.

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

Isso significa que projetos existentes não são punidos por dívida histórica de uma vez só. O foco é bloquear regressão nova, especialmente útil em rollouts com agentes.

## Integração com CI

`crivo init` cria um workflow de GitHub Actions pronto. O fluxo:

1. Roda `crivo run --md report.md --sarif report.sarif --save`
2. Faz upload do SARIF pro GitHub Code Scanning
3. Posta o markdown como comentário no PR

Pra outros sistemas de CI, o crivo retorna exit code 1 quando o gate falha, 0 quando passa.

## Suporte a linguagens

O suporte mais completo hoje é para projetos TypeScript/JavaScript, que combinam type safety, coverage, duplicação, complexidade, secrets, segurança e dead code.

Para outros stacks, o `crivo` já cobre partes úteis do gate, como duplicação, complexidade heurística, secrets e segurança, dependendo do layout do projeto e das ferramentas disponíveis.

## Custom Rules

Custom Rules são a peça principal do posicionamento do `crivo`: além dos checks básicos, você define o que o agente não pode fazer no seu repositório.

As regras ficam no `.qualitygate.yaml` e podem ser regex-based ou AST-based via semgrep:

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

**Advisory mode:** `mode: advisory` reporta mas não bloqueia o gate — útil pra rollout gradual de regras novas para agentes e times.

## Como funciona

O crivo não implementa nenhuma análise. Ele orquestra ferramentas existentes:

1. Lê `.qualitygate.yaml` pra configuração
2. Detecta quais checks se aplicam (baseado nos arquivos do projeto)
3. Roda as ferramentas aplicáveis em paralelo
4. Parseia a saída de cada ferramenta num formato normalizado
5. Calcula ratings A-E
6. Avalia as condições do gate conforme a política configurada
7. Gera saída para terminal, agentes, CI e code scanning

Zero CGO. Binário único. Sem dependências de runtime além das ferramentas em si (node, go, python — o que seu projeto já usa).

## Posicionamento

Se você já usa agentes para escrever código, o `crivo` funciona como a última barreira entre "parece pronto" e "está pronto para subir".

Ele não tenta ser uma plataforma monolítica de qualidade. Ele tenta ser um gate pragmático, automatizável e extensível com regras do seu time.

## Licença

MIT

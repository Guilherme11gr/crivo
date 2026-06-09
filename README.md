# crivo

`crivo` é um quality gate local para projetos tocados por agentes de código e CI.

Ele roda ferramentas já existentes, normaliza os resultados e responde uma pergunta simples: esta mudança pode subir?

Não tem servidor, conta, dashboard externo ou runtime obrigatório além das ferramentas que o projeto já usa.

## O que ele faz

- Detecta quais checks se aplicam ao projeto.
- Executa os checks em paralelo, limitando ferramentas pesadas.
- Gera saída para terminal, JSON, Markdown e SARIF.
- Calcula ratings A-E para reliability, security e maintainability.
- Aplica políticas de gate (`release`, `strict`, `informational`).
- Analisa só código alterado com `--new-code`.
- Permite regras customizadas por repositório no `.qualitygate.yaml`.
- Salva histórico local em SQLite quando usado com `--save`.

## Instalação

```bash
npm install -g crivo
```

Ou:

```bash
go install github.com/guilherme11gr/crivo/cmd/crivo@latest
```

## Uso

```bash
crivo init                       # cria configuração inicial e workflow de CI
crivo run                        # roda o gate completo
crivo run --json                 # saída estruturada para agentes/automação
crivo run --new-code             # analisa apenas arquivos/linhas alterados
crivo run --md report.md         # gera resumo em Markdown
crivo run --sarif report.sarif   # gera SARIF para code scanning
crivo run --save                 # salva histórico local
crivo trends                     # mostra tendência dos runs salvos
```

Para CI, o uso típico é:

```bash
crivo run --new-code --md report.md --sarif report.sarif --save
```

## Checks suportados

| Check | Ferramenta | Escopo |
| --- | --- | --- |
| TypeScript | `tsc` | erros de tipo, separando produção e testes |
| Coverage | `jest` / `vitest` | cobertura de linhas, branches, funções e statements |
| Duplication | `jscpd` + heurística semântica | copy-paste e clones parecidos |
| Complexity | AST JS/TS + fallback regex | complexidade cognitiva por função |
| Secrets | `gitleaks` | credenciais e chaves hardcoded |
| Security | `semgrep` | padrões de vulnerabilidade e hotspots |
| Dead code | `knip` | exports, arquivos e dependências não usados |
| Custom rules | regex + semgrep | regras específicas do projeto |

Cada check é opcional. O `crivo` só roda o que faz sentido para o projeto detectado e para a configuração atual.

## `--new-code`

`crivo run --new-code` compara a branch atual com a branch base e filtra findings para os arquivos e linhas alterados.

Alguns providers também recebem esse escopo antes de rodar, para evitar trabalho desnecessário em checks pesados como `semgrep`, `gitleaks` e custom rules.

Na branch principal, o modo compara `HEAD` com mudanças locais.

## Gate

As políticas disponíveis são:

| Política | Comportamento |
| --- | --- |
| `release` | bloqueia erros de tipo em produção, secrets e custom rules bloqueantes |
| `strict` | bloqueia também cobertura, duplicação e outros checks configurados |
| `informational` | nunca bloqueia; apenas reporta |

Exemplo:

```yaml
gate-policy: release
```

Ou por execução:

```bash
crivo run --policy strict
```

## Configuração

`crivo init` cria um `.qualitygate.yaml`. Um exemplo mínimo:

```yaml
profile: balanced
gate-policy: release

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
  secrets: true
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

## Custom rules

Custom rules ficam no `.qualitygate.yaml` e servem para regras locais que ferramentas genéricas não conhecem.

Tipos suportados:

- `ban-import`
- `ban-pattern`
- `require-import`
- `enforce-pattern`
- `ban-dependency`
- `max-lines`
- `semgrep`

Exemplo:

```yaml
custom-rules:
  - id: no-console-log
    type: ban-pattern
    pattern: "console\\.(?:log|debug)\\("
    files: "src/**/*.{ts,tsx}"
    message: "Use logger em vez de console.log"
    severity: major

  - id: no-axios
    type: ban-dependency
    packages: ["axios", "got", "node-fetch"]
    message: "Use fetch nativo"
    severity: blocker

  - id: component-max-300
    type: max-lines
    max-lines: 300
    files: "src/components/**/*.tsx"
    message: "Componente muito grande"
    severity: major
    mode: advisory
```

`mode: advisory` reporta a violação, mas não bloqueia o gate.

## Saídas

- Terminal: resumo visual com status, ratings, checks e issues.
- JSON: formato completo para agentes e automação.
- Markdown: relatório legível para PRs.
- SARIF: integração com GitHub Code Scanning.
- TUI: dashboard local com `crivo run --tui`.

## Histórico

Com `--save`, o `crivo` grava os runs em `.qualitygate/history.db`.

Isso alimenta `crivo trends` e permite comparação com baseline local, útil para não tratar toda dívida histórica como regressão nova.

## Licença

MIT

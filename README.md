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

### `coverage.new-code`

O check de coverage roda a suíte de testes inteira — o mais caro do pipeline. Em
modo `--new-code`, o gate só se importa com as linhas alteradas, então rodar a
suíte completa é custo sem retorno. O campo `coverage.new-code` controla isso:

| Valor | Comportamento em `--new-code` |
| --- | --- |
| `off` (default) | Pula a suíte: o check retorna `skipped` com summary explícito. Nenhum custo de execução. |
| `related` | Roda só os testes relacionados aos arquivos alterados (`vitest related --coverage` / `jest --findRelatedTests ... --coverage`). Use em PR gates que precisam de um número real de cobertura do código novo. |
| `full` | Roda a suíte inteira (comportamento pré-existente). |

Fora do modo `--new-code`, o campo é ignorado e a suíte sempre roda. Um valor
inválido é erro de configuração (o run aborta), nunca silêncio.

```yaml
coverage:
  lines: 60
  branches: 50
  functions: 60
  statements: 60
  new-code: off   # off (default) | related | full
```

## Gate

As políticas disponíveis são:

| Política | Comportamento |
| --- | --- |
| `release` | bloqueia erros de tipo em produção, duplicação, secrets e custom rules bloqueantes |
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

### Variáveis de ambiente

| Variável | Descrição | Default |
|---|---|---|
| `CRIVO_MAX_WORKERS` | Máximo de checks rodando em paralelo (clamp 1..16). | Local: `max(NumCPU/4, 1)` limitado a 2; CI: `max(NumCPU/2, 2)` limitado a 4 |
| `CRIVO_MAX_HEAVY` | Máximo de checks "heavy" (tsc, complexity, coverage, duplication, semgrep, secrets, dead-code) rodando simultaneamente (clamp 1..16). | Local: 1; CI: 2 |
| `CRIVO_NO_AUTO_INSTALL` | `1` impede o download automático de ferramentas (gitleaks/semgrep). | Desativado (auto-install permitido) |

Valores inválidos (não numéricos) são ignorados e o default é usado.

## Segurança e integridade

O `crivo` baixa e executa código de terceiros (o próprio binário via npm, e as
ferramentas `gitleaks` e `semgrep`). Para que isso seja seguro e reprodutível:

- **Binário via npm**: o `install.js` verifica o SHA-256 do download contra o
  `checksums.txt` publicado no release (GoReleaser) antes de instalar. Checksum
  ausente ou divergente é erro duro — o binário nunca é instalado.
- **gitleaks**: versão pinada (`8.24.3`) com checksum SHA-256 embutido no
  binário, verificado antes da extração, e download com timeout de 60s.
- **semgrep**: versão pinada (`pip install semgrep==<versão>`). Atualizar o pin
  é uma release note do crivo — mudanças de findings são intencionais.
- **Opt-out de auto-install**: defina `CRIVO_NO_AUTO_INSTALL=1` para impedir que
  o `crivo` baixe qualquer ferramenta automaticamente. Ferramentas ausentes
  viram checks pulados/avisos visíveis em vez de instalação silenciosa.

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

## Contrato para agentes

Exit codes:

- `0`: gate passou ou a política atual é apenas informativa.
- `1`: gate falhou ou algum erro impediu a análise.

No modo `--json`, os campos mais importantes são:

- `status`: `passed` ou `failed`.
- `checks[]`: resultado normalizado de cada provider.
- `checks[].issues[]`: findings acionáveis, com arquivo, linha, severidade, tipo e remediação quando disponível.
- `conditions[]`: regras que decidiram se o gate passou.
- `ratings`: notas A-E por dimensão.
- `totalIssues`: contagem final após filtros como `--new-code`.

## Histórico

Com `--save`, o `crivo` grava os runs em `.qualitygate/history.db`.

Isso alimenta `crivo trends` e permite comparação com baseline local, útil para não tratar toda dívida histórica como regressão nova.

## Licença

MIT

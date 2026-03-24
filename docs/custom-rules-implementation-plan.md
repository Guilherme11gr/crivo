# Custom Rules Provider — Plano de Implementação

## Arquivos a criar/modificar

| Arquivo | O quê |
|---------|-------|
| `internal/config/config.go` | Struct `CustomRule` + campo `CustomRules` no Config |
| `internal/config/profiles.go` | Habilitar custom-rules nos profiles |
| `internal/check/runner.go` | Caso `"custom-rules"` no `isCheckEnabled` |
| `internal/check/providers/customrules/rule.go` | **NOVO** — CompiledRule, validação, compilação de regex |
| `internal/check/providers/customrules/filewalker.go` | **NOVO** — File walking com glob `**`, `{a,b}`, exclusions |
| `internal/check/providers/customrules/matcher.go` | **NOVO** — 5 matchers (ban-import, ban-pattern, require-import, enforce-pattern, ban-dependency) |
| `internal/check/providers/customrules/customrules.go` | **NOVO** — Provider (Name, ID, Detect, Analyze) |
| `internal/check/providers/customrules/customrules_test.go` | **NOVO** — Testes completos |
| `cmd/qg/main.go` | Import + register `customrules.New(cfg)` |

---

## Ordem de implementação

### Fase 1: Config (`internal/config/`)

Adicionar structs no `config.go`:

```go
type CustomRule struct {
    ID             string   `yaml:"id" json:"id"`
    Type           string   `yaml:"type" json:"type"`
    Pattern        string   `yaml:"pattern" json:"pattern"`
    Packages       []string `yaml:"packages" json:"packages"`
    AllowIn        []string `yaml:"allow-in" json:"allowIn"`
    Files          string   `yaml:"files" json:"files"`
    Message        string   `yaml:"message" json:"message"`
    Severity       string   `yaml:"severity" json:"severity"`
    WhenPattern    string   `yaml:"when-pattern" json:"whenPattern"`
    MustImportFrom string   `yaml:"must-import-from" json:"mustImportFrom"`
}
```

Adicionar ao `Config`:
```go
CustomRules []CustomRule `yaml:"custom-rules" json:"customRules"`
```

Adicionar ao `ChecksConfig`:
```go
CustomRules bool `yaml:"custom-rules" json:"customRules"` // default: true
```

### Fase 2: Rule compilation (`rule.go`)

```go
type RuleType string

const (
    RuleTypeBanImport      RuleType = "ban-import"
    RuleTypeBanPattern     RuleType = "ban-pattern"
    RuleTypeRequireImport  RuleType = "require-import"
    RuleTypeEnforcePattern RuleType = "enforce-pattern"
    RuleTypeBanDependency  RuleType = "ban-dependency"
)

type CompiledRule struct {
    Raw            config.CustomRule
    Type           RuleType
    PatternRe      *regexp.Regexp   // ban-pattern, enforce-pattern
    WhenPatternRe  *regexp.Regexp   // require-import
    AllowInGlobs   []string
    Severity       domain.Severity
}
```

`CompileRules(rules []config.CustomRule) ([]CompiledRule, []error)`:
- Valida campos obrigatórios por tipo
- Compila regex (falha graceful com rule ID no erro)
- Mapeia severity string → domain.Severity (default: major)
- IDs únicos
- Retorna TODOS os erros de uma vez (não fail-fast)

### Fase 3: File walker (`filewalker.go`)

Implementar glob matching próprio (zero deps externas):
- Suporte a `**` (qualquer profundidade de diretório)
- Suporte a `{ts,tsx}` (alternação)
- Suporte a `*` e `?`
- `SkipDir` pra diretórios excluídos (node_modules, dist)
- Skip binários (> 1MB ou falha UTF-8)
- Respeitar `ctx.Err()` entre arquivos

```go
func WalkFiles(ctx context.Context, projectDir string, fileGlob string, exclude []string) ([]string, error)
func IsAllowedIn(filePath string, allowIn []string) bool
func matchGlob(pattern, path string) bool
```

Default de `files` quando vazio: `src/**/*.{ts,tsx,js,jsx}`

### Fase 4: Matchers (`matcher.go`)

Cada matcher retorna `[]domain.Issue`:

#### `ban-import`
- Regex gerado por package: `import\s+.*from\s+['"]PKG` e `require\s*\(\s*['"]PKG`
- Match sub-paths (`date-fns/format` → banned se `date-fns` na lista)
- Não match substrings (`safe-date-fns` ≠ `date-fns`)
- Reporta linha exata

#### `ban-pattern`
- Aplica PatternRe linha por linha
- Checa `allow-in` antes — se match, skip arquivo inteiro
- Múltiplos matches = múltiplas issues
- Reporta linha + coluna

#### `require-import`
- Se WhenPatternRe matcha no arquivo inteiro
- Checa se existe `import ... from ['"]MUST_IMPORT_FROM`
- Se não existe: 1 issue na linha 1 (violação de arquivo)

#### `enforce-pattern`
- Se PatternRe NÃO matcha no conteúdo do arquivo
- 1 issue na linha 1 (padrão obrigatório ausente)

#### `ban-dependency`
- Lê `package.json` uma vez
- Checa dependencies, devDependencies, peerDependencies
- Match exato (não substring)
- Reporta linha no package.json onde a dep aparece

### Fase 5: Provider (`customrules.go`)

```go
type Provider struct{}

func (p *Provider) Name() string { return "Custom Rules" }
func (p *Provider) ID() string   { return "custom-rules" }

func (p *Provider) Detect(ctx context.Context, projectDir string) bool {
    // Lê config, retorna true se len(CustomRules) > 0
}

func (p *Provider) Analyze(ctx context.Context, projectDir string, cfg *config.Config) (*domain.CheckResult, error) {
    // 1. CompileRules → se erros, return StatusError
    // 2. Separar ban-dependency (package.json) das demais (file scan)
    // 3. Construir mapa: arquivo → []regras aplicáveis
    // 4. Walk files UMA VEZ, ler cada arquivo UMA VEZ
    // 5. Aplicar todos matchers relevantes por arquivo
    // 6. Coletar issues, calcular status, retornar CheckResult
}
```

**Status logic:**
- Issues blocker/critical → `StatusFailed`
- Issues major/minor/info apenas → `StatusWarning`
- Zero issues → `StatusPassed`

**Otimização:** ler cada arquivo uma única vez, aplicar todas as regras de uma vez.

### Fase 6: Registration (`cmd/qg/main.go`)

```go
import "github.com/guilherme11gr/crivo/internal/check/providers/customrules"
// ...
registry.Register(customrules.New())
```

### Fase 7: Testes (`customrules_test.go`)

**Validação de regras:**
- Cada tipo compila corretamente
- Campos obrigatórios faltando → erro claro
- Regex inválido → erro com rule ID
- Tipo desconhecido → erro
- IDs duplicados → erro
- Severity padrão = major

**Matchers (por tipo):**
- ban-import: import/require, sub-paths, não-substring, linha correta
- ban-pattern: match, allow-in, múltiplos matches, linha+coluna
- require-import: com/sem when-pattern, com/sem import correto
- enforce-pattern: com/sem pattern no arquivo
- ban-dependency: deps/devDeps, match exato, linha no package.json

**Integração:**
- Analyze com mix de tipos
- Zero regras → Detect false
- Contexto cancelado → para walk
- Arquivos grandes → skip

---

## Edge Cases

| Caso | Tratamento |
|------|-----------|
| Zero custom-rules | `Detect()` → false, provider skipped |
| Regex inválido | `StatusError` com detalhes de qual regra falhou |
| `files` vazio | Default `src/**/*.{ts,tsx,js,jsx}` |
| Arquivo binário/gigante | Skip se > 1MB ou falha UTF-8 |
| package.json não existe | Skip ban-dependency, sem erro |
| Repo enorme | SkipDir em excludes, ctx.Err() entre arquivos |
| Import em comentário | Match (aceito — simplicidade > AST) |
| Sub-path de package | `date-fns/format` → banned se `date-fns` na lista |
| Substring de package | `safe-date-fns` ≠ `date-fns` (word boundary) |

---

## Config de exemplo (agenda-aqui)

```yaml
custom-rules:
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
    files: "src/**/*.{ts,tsx}"

  - id: no-axios
    type: ban-dependency
    packages: ["axios", "got", "node-fetch"]
    message: "Projeto usa native fetch"
    severity: blocker

  - id: api-rate-limit
    type: enforce-pattern
    files: "src/app/api/**/route.ts"
    pattern: "applyRateLimit"
    message: "Rotas de API públicas precisam de rate limiting"
    severity: major

  - id: dates-from-utils
    type: require-import
    when-pattern: "(formatDate|parseUTCString|createLocalDate)"
    must-import-from: "@/shared/utils/date-utils"
    message: "Funções de data devem vir de date-utils.ts"
    severity: major
    files: "src/**/*.{ts,tsx}"
```

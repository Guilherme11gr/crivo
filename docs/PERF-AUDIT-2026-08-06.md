# Performance Audit — crivo v3.4.3

**Data:** 2026-08-06
**Método:** análise estática do código-fonte (Go), sem executar o binário. Duas auditorias independentes que convergiram.
**Versão auditada:** `3.4.3` (última publicada no npm em 14/07/2026, HEAD `f3b5007`)
**Contexto:** rodada `crivo run --policy informational` quase derrubou a máquina (15GB RAM, matou o processo). Objetivo deste report é identificar as causas antes de tentar rodar de novo.

---

## TL;DR — onde está o custo

A arquitetura é **conservadora e correta em concorrência** (sem goroutine-per-file, sem nada deslimitado). O custo dominante **não é o Go**: é uma sequência de subprocessos externos pesados (`npx tsc`, `npx vitest --coverage`, `npx jscpd`, `semgrep`, `gitleaks`, `npx knip`, `node cognitive.js`), **cada um rodando a partir do zero**, sem cache de nada entre runs.

Isso por si só não "mata a máquina" — mata o **tempo**. O que mata memória/CPU é um conjunto específico de ineficiências dentro de alguns providers, listadas abaixo em ordem de impacto.

---

## Problemas por severidade (top → bottom)

### 🔴 P1 — `gitleaks` faz 1 subprocesso por arquivo em `--new-code`

**Arquivo:** `internal/check/providers/secrets/gitleaks.go:182-192`

Em modo `--new-code`, `gitleaksTargets()` (linha 167) retorna **um caminho absoluto por arquivo alterado**, e `runGitleaksTargets` faz um loop spawnando um `gitleaks detect` **por arquivo**:

```go
func runGitleaksTargets(ctx context.Context, gitleaksBin, projectDir string, targets []string) ([]gitleaksResult, error) {
	var all []gitleaksResult
	for _, target := range targets {
		results, err := runGitleaksTarget(ctx, gitleaksBin, target)  // 1 subprocesso por arquivo
		...
	}
}
```

Um PR que toca 200 arquivos = **200 subprocessos** do binário gitleaks (multi-MB cada), em série. gitleaks aceita múltiplos paths numa invocação só (`gitleaks detect --source=. path1 path2 ...`).

**Fix:** colapsar o loop numa única invocação passando todos os targets. Elimina O(arquivos alterados) spawns.

### 🔴 P2 — `regexp.MustCompile` dentro do loop quente das custom-rules

**Arquivo:** `internal/check/providers/customrules/matcher.go:62-66` e `:146`

`matchBanImport` é chamada **por arquivo × por regra**. Dentro dela, no loop `for _, pkg := range rule.Raw.Packages`, compila 2 regexes **a cada chamada**:

```go
escaped := regexp.QuoteMeta(pkg)
patterns := []*regexp.Regexp{
	regexp.MustCompile(`(?:import\s+.*from\s+|import\s+)['"]` + escaped + `(?:/[^'"]*)?['"]`),
	regexp.MustCompile(`require\s*\(\s*['"]` + escaped + `(?:/[^'"]*)?['"]\s*\)`),
}
```

Com M arquivos × P pacotes banidos = **2·M·P compilações de regex** idênticas por run. Mesma coisa em `matchRequireImport` (linha 146). As outras funções de match (`matchBanPattern`, `matchEnforcePattern`, `matchMaxLines`) fazem certo — usam `rule.PatternRe` pré-compilado em `CompileRules`.

**Fix:** pré-compilar esses regexes em `CompileRules` (`internal/check/providers/customrules/rule.go`) e guardar no `CompiledRule`, igual já é feito com `PatternRe`.

### 🟠 P3 — `typescript` e `complexity` NÃO são classificados como heavy

**Arquivo:** `internal/check/runner.go:60-67`

```go
func isHeavyProviderID(id string) bool {
	switch id {
	case "coverage", "duplication", "semgrep", "secrets", "dead-code":
		return true
	default:
		return false
	}
}
```

`typescript` (`npx tsc --noEmit`) e `complexity` (`node cognitive.js`) não entram no conjunto "heavy". Resultado: esses dois subprocessos node-based podem rodar **concorrentes** com os outros heavy node-based, competindo por CPU. `tsc` costuma ser o check mais lento de todos — surpreendente ele não ser heavy.

**Fix:** adicionar `"typescript", "complexity"` ao switch.

### 🟠 P4 — Concorrência local travada em 2 workers / 1 heavy

**Arquivo:** `internal/check/runner.go:46-58`

```go
func defaultMaxWorkers() int {
	if os.Getenv("CI") == "true" {
		return min(max(runtime.NumCPU()/2, 2), 4)
	}
	return min(max(runtime.NumCPU()/4, 1), 2)   // local: máx 2
}

func defaultHeavyWorkers() int {
	if os.Getenv("CI") == "true" {
		return 2
	}
	return 1   // local: 1 heavy por vez
}
```

Numa máquina 8–16 cores, local roda **no máximo 2 checks por vez, e só 1 heavy**. Cada heavy demora minutos (coverage roda a suite inteira). Isso não é um bug — é conservadorismo — mas é o **maior driver de wall-clock**. É o parafuso que mais vale girar.

**Fix:** expor `maxWorkers`/`maxHeavy` via flag/env (`CRIVO_MAX_WORKERS`, `CRIVO_MAX_HEAVY`), ou subir o teto local.

### 🟠 P5 — N walks independentes da árvore de arquivos

Não existe uma lista compartilhada de arquivos. Cada provider anda na árvore por conta própria:

- `customrules/filewalker.go:46` — `filepath.WalkDir(projectDir)` **uma vez por glob distinto**. 3 globs nas rules = 3 walks completos da árvore.
- `complexity/complexity.go:309` — `filepath.Walk` (o **mais lento**, faz `os.Lstat` por entry) sobre cada `cfg.Src` (só no fallback de regex).
- `duplication/semantic.go:382` — `filepath.WalkDir` sobre cada `cfg.Src`.

Excludes de diretório funcionam (usamos `SkipDir` em `node_modules`/`.next`/etc), então não leem o que não devem — mas a árvore é caminhada repetidamente.

**Fix:** uma camada de descoberta de arquivos compartilhada (walk único → cache `(path → conteúdo)`), consumida por todos os providers in-process.

### 🟡 P6 — Duplicação semântica é O(N²) com cap em 2000

**Arquivo:** `internal/check/providers/duplication/semantic.go:464-495`

`findSemanticClones` faz match fuzzy por **trigramas** entre todos os pares de funções candidatas, com cap silencioso em 2000:

```go
if len(candidates) <= 2000 {   // acima disso, skip silencioso
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			... // cada par constrói 2 map[string]struct{} (jaccardTrigrams)
		}
	}
}
```

Para ~2000 candidatos: até ~2M probes, cada um alocando maps. Hotspot real quando `semantic: true` e `similarity-threshold < 1.0`. Acima de 2000, a feature simplesmente para de rodar sem avisar.

**Fix:** (a) hash trigramas pré-computado por função (não realocar por par); (b) avisar quando o cap é atingido; (c) indexar por bucket em vez de O(N²).

### 🟡 P7 — Coverage roda a suite de testes inteira mesmo em `--new-code`

**Arquivo:** `internal/check/providers/coverage/coverage.go:131`

`npx jest/vitest --coverage` roda **todos os testes** a cada `crivo run`. O provider de coverage **ignora o `NewCodeScope`** — não tem checagem de escopo no `Analyze` (diferente de semgrep/secrets/gitleaks, que respeitam). Ou seja, o custo do coverage nunca é reduzido em modo new-code.

**Fix:** honrar `NewCodeScope` (skip do provider, ou coverage só dos arquivos alterados).

### 🟡 P8 — Nada é cacheado entre runs

Todo `crivo run` começa do zero:
- `npx tsc` re-typechecka o projeto inteiro (sem `--build`, sem cache incremental).
- `npx vitest --coverage` roda a suite inteira.
- `node cognitive.js` é escrito num temp dir fresco a cada run (`complexity.go:101-110`).
- `npx jscpd --version` roda **a cada run** só pra escolher syntax de ignore flag (`jscpd.go:289-298`).
- `git.DefaultBranch` faz até **3 subprocessos git em série** (`git/git.go:48-67`).

Nenhum resultado de check persiste entre runs. Para o caso de uso "rodar local toda hora", isso é caro.

**Fix:** cache em disco (content-hash por arquivo → resultado de check), keyado por hash do arquivo + versão da tool.

---

## Menores (baixo impacto, fácil de limpar)

| # | Onde | O que |
|---|------|-------|
| M1 | `cmd/crivo/main.go:911-919` | `result += c` num loop de runes — usar `strings.Join` |
| M2 | `complexity/complexity.go:400-404` | `strings.Split(string(data))` **+** `bufio.Scanner(strings.NewReader(string(data)))` — materializa o conteúdo do arquivo 2x. Line count pode ser `bytes.Count(data, '\n')` |
| M3 | `duplication/semantic.go:110,119` | mesmo padrão: `os.ReadFile` + `strings.NewReader(string(data))` — copy extra |
| M4 | `customrules/customrules.go:29` | `Detect` recarrega e re-parseia o `.qualitygate.yaml` via `config.Load`, duplicando o que `main.go:189` já carregou |
| M5 | `complexity/complexity.go:309` | `filepath.Walk` (lento) em vez de `filepath.WalkDir` |
| M6 | `internal/store/store.go` + `main.go:326,338,930` | SQLite `Open`+schema `Exec`+`Close` em até 3 pontos por run, em vez de reusar uma conexão |
| M7 | `secrets/gitleaks.go:95`, `customrules/matcher.go:444,575` | slices sem pre-alocação quando o tamanho é conhecível |

---

## O que **não** está quebrado (pra não "consertar")

- **Runner:** os dois semáforos (`sem` e `heavySem`) em `runner.go:118-119` estão corretos. O `sync.Mutex` que guarda `results` tem contenção desprezível (≤8 acquires). **Não é** o problema.
- **Regexes na maioria dos providers:** todos package-level `var … = regexp.MustCompile(...)`. Bom.
- **`node_modules`/`.next`:** todo walker faz `SkipDir`. Não estão sendo lidos.
- **Excludes de diretório:** eficientes (SkipDir no nível certo), exceto pela inconsistência de alguns não honrarem `cfg.Exclude` (M5/P5).

---

## Histórico relevante

- `f8c3e31` (09/06) **"feat: optimize new-code quality gate runs"** — você já atacou parte disso: criou `internal/check/scope.go` (NewCodeScope), mexeu em `gitleaks.go`, `semgrep.go`, `customrules/matcher.go`, `runner.go`. **Mas** os pontos P1, P2, P3, P4 listados acima **continuam presentes** no HEAD atual (`f3b5007`, publicado como 3.4.3).
- `4b4807b` (04/04) "remove ESLint provider and fix 3 external provider bugs" — removeu um provider inteiro.
- Não há commit posterior a 09/06 que indique continuação da otimização.

---

## Plano de ataque sugerido (ordem por esforço × impacto)

| Ordem | Item | Esforço | Impacto |
|-------|------|---------|---------|
| 1 | **P3** (typescript/complexity como heavy) | 2 linhas | médio — reduz contenção de CPU |
| 2 | **P2** (pré-compilar regex ban-import) | ~30min | médio — elimina O(M·P) compilações |
| 3 | **P1** (gitleaks batch de targets) | ~1h | alto em PRs grandes |
| 4 | **P4** (expor workers via env) | ~30min | alto em wall-clock local |
| 5 | **P7** (coverage honrar new-code) | ~1h | alto — pula a suite inteira em PR |
| 6 | **P8** (cache entre runs) | grande | alto no uso iterativo local |
| 7 | **P5/P6** (walk compartilhado / O(N²)) | médio | médio |

Os itens 1–4 são rápidos e cobrem a maior parte do ganho. Item 5 é o que mais ajuda no CI. 6 e 7 são investimentos maiores.

---

## Como reproduzir o diagnóstico (sem matar a máquina)

Pra confirmar qual check específico está explodindo memória (em vez de só ser lento), rodar **um check isolado por vez** com timeout agressivo e sem o semáforo de concorrência mascarando:

```bash
# cada um isolado, mata se passar de 60s ou 2GB
timeout 60 npx crivo run --disable coverage,duplication,semgrep,secrets,dead-code,complexity,custom-rules   # só typescript
timeout 60 npx crivo run --disable typescript,duplication,semgrep,secrets,dead-code,complexity,custom-rules # só coverage
# ... etc
```

Isso isola o vilão sem rodar todos juntos.

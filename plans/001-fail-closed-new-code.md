# Plan 001: `--new-code` resolve a base ou falha alto — nunca analisa o repo inteiro em silêncio

> **Executor instructions**: Follow this plan step by step. Run every verification
> command and confirm the expected result before moving to the next step. If anything
> in the "STOP conditions" section occurs, stop and report — do not improvise. When
> done, update the status row for this plan in `plans/README.md`.
>
> **Drift check (run first)**: `git diff --stat f3b5007..HEAD -- cmd/crivo/main.go internal/git/ .github/workflows/ci.yml .github/workflows/release.yml AGENTS.md`
> If any in-scope file changed since this plan was written, compare the "Current state"
> excerpts against the live code before proceeding; on a mismatch, treat it as a STOP.

## Status

- **Priority**: P1
- **Effort**: M
- **Risk**: MED (runs que hoje passam errado vão começar a falhar — é o objetivo, mas é mudança de comportamento visível)
- **Depends on**: none
- **Category**: bug
- **Planned at**: commit `f3b5007`, 2026-08-14

## Why this matters

Este é o bug que fez o PR Guilherme11gr/agenda-aqui#179 reprovar no quality gate por
dívida pré-existente da main. Em CI (GitHub Actions, `pull_request`), o checkout fica em
detached HEAD e só existem refs remotas (`origin/main`), nunca a branch local `main`.
O crivo monta `git diff --numstat --diff-filter=ACMR <base>...HEAD` com o nome da base
**literal** (`main`), sem fallback para `origin/main` — o comando falha, o erro é
**descartado**, o escopo de new-code fica vazio, e o filtro pós-check é **pulado
inteiro**. Resultado: análise de repo completo reportada como "new code", gate reprovando
em baseline. Um quality gate que mente sobre o que analisou não é um gate.

## Current state

- `cmd/crivo/main.go:244-268` — bloco new-code:

```go
baseBranch := gitutil.DefaultBranch(ctx, projectDir)
if opts.branch != "" {
    baseBranch = opts.branch
}
currentBranch, _ := gitutil.CurrentBranch(ctx, projectDir)
diffRef := baseBranch
headRef := "HEAD"
if currentBranch == baseBranch {
    diffRef = "HEAD"
    headRef = ""
}
changedFiles, _ = gitutil.GetChangedFiles(ctx, projectDir, diffRef, headRef)   // erro engolido
changedLines, _ = gitutil.GetChangedLines(ctx, projectDir, diffRef, headRef)   // erro engolido
for _, f := range changedFiles {
    changedFileSet[f.Path] = true
}
ctx = check.WithNewCodeScope(ctx, check.NewScope(changedFiles, changedLines))  // escopo vazio instalado
```

- `cmd/crivo/main.go:306-316` — o filtro só roda se o escopo NÃO for vazio:

```go
if opts.newCode && gitutil.IsGitRepo(projectDir) {
    if len(changedFileSet) > 0 {
        for i := range analysis.Checks {
            filterCheckToNewCode(&analysis.Checks[i], changedFileSet, changedLines)
        }
    }
}
```

- `internal/git/git.go:48-67` — `DefaultBranch` tenta `origin/HEAD`, verifica `main`
  via `git rev-parse --verify main` e por fim retorna a string `"master"` **sem
  verificar existência** (repo com default `trunk`/`develop` quebra silenciosamente).
- `internal/git/git.go:70-113` — `GetChangedFiles`/`GetChangedLines` rodam
  `git diff --numstat ... base...HEAD` sem tentar `origin/<base>`.
- `cmd/crivo/main.go:218-227` — `commit := ""` nunca é atribuído (nenhum
  `git rev-parse HEAD`); `SaveAnalysis` persiste `commit_hash` vazio.
- `.github/workflows/ci.yml:20` e `.github/workflows/release.yml:23` — ambos rodam
  `go test ./internal/... -count=1 -race`, **excluindo** `cmd/crivo/main_test.go`.
- Convenções do repo: commits conventional (`fix:`, `feat:`, `ci:` — ver `git log`);
  branches no estilo `fix/<slug>` (PR #1); testes Go stdlib table-driven, in-package.

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Testes (código novo) | `go test ./internal/git/... ./cmd/... -count=1` | exit 0 |
| Testes completos | `go test ./... -count=1 -race` | exit 0 |
| Vet | `go vet ./...` | exit 0 |
| Build | `go build ./cmd/crivo/` | exit 0 |

## Scope

**In scope** (only files you should modify):
- `internal/git/git.go` + `internal/git/git_test.go`
- `cmd/crivo/main.go` + `cmd/crivo/main_test.go`
- `.github/workflows/ci.yml`, `.github/workflows/release.yml`, `AGENTS.md`

**Out of scope** (do NOT touch):
- `internal/check/scope.go`, providers, `internal/rating/` — pertencem aos planos 002/003.
- Qualquer mudança em como o `--branch` é exposto na CLI (mantém a flag como está).

## Git workflow

- Branch: `fix/newcode-base-resolution`
- Commits: conventional, ex.: `ci: run full go test suite in CI`, `fix(git): resolve base branch with origin fallback`, `fix(new-code): fail loudly when diff scope is empty`

## Steps

### Step 1: CI passa a rodar a suíte completa

Em `.github/workflows/ci.yml` e `.github/workflows/release.yml`, trocar
`go test ./internal/... -count=1 -race` por `go test ./... -count=1 -race`.
Atualizar a mesma linha em `AGENTS.md` se ela citar o comando restrito.

**Verify**: `grep -n "go test" .github/workflows/*.yml AGENTS.md` → todas as ocorrências usam `./...`

### Step 2: `ResolveBaseRef` com fallback e `DefaultBranch` sem mentira

Em `internal/git/git.go`:

1. Nova função exportada:
   `func ResolveBaseRef(ctx context.Context, projectDir, base string) (string, error)`
   — tenta na ordem: `git rev-parse --verify <base>` (rev-parse resolve nomes de
   branch, tags e refs); se falhar, `git rev-parse --verify origin/<base>`; se ambos
   falharem, retorna erro citando os dois refs tentados (inclua stderr truncada em
   ~200 chars).
2. `DefaultBranch`: manter heurística atual, mas a última opção (`return "master"`)
   passa por `rev-parse --verify origin/master`; se não verificar, devolver
   `origin/HEAD` resolvido ou erro. Nunca retornar um nome não verificável.

**Verify**: `go test ./internal/git/... -count=1` → passando, incluindo os novos testes do Step 6

### Step 3: erro de diff deixa de ser engolido; escopo vazio é erro fatal

Em `cmd/crivo/main.go` (bloco new-code, linhas ~244-268):

1. `baseRef, err := gitutil.ResolveBaseRef(ctx, projectDir, baseBranch)` — em erro,
   imprimir (stderr e `--json` no campo apropriado) mensagem:
   `--new-code: cannot resolve base branch '<base>' (tried <base>, origin/<base>)`
   e sair com exit code 1 (usar o caminho de erro existente do main; ver como erros
   fatais atuais saem — `fmt.Fprintf(os.Stderr, ...)` + `return 1`).
2. `GetChangedFiles`/`GetChangedLines` passam a propagar erro (mesmo tratamento).
3. Depois de computar `changedFiles`: se `len(changedFiles) == 0` E `currentBranch != baseBranch`,
   sair com erro explícito: `--new-code: diff against <baseRef> is empty — nothing to analyze`
   (dif vazia legítima só existe quando a branch atual É a base, modo working-tree).
   NÃO chamar `WithNewCodeScope` com escopo vazio.

**Verify**: `go build ./cmd/crivo/` → exit 0

### Step 4: capturar o commit hash

No mesmo bloco de git info (`main.go:218-227`), adicionar
`commit, _ = gitutil.CurrentCommit(ctx, projectDir)` — nova função em `git.go`
(`git rev-parse HEAD`, trimmed). Em repo não-git permanece `""`.

**Verify**: `go test ./cmd/... -count=1` → passando

### Step 5: detector de regressão ponta-a-ponta (teste de integração leve)

Em `cmd/crivo/main_test.go`, teste novo que reproduz o cenário CI: criar repo
temporário (`t.TempDir()` + `git init`, commit base, branch `feature`, outro commit,
checkout detached no SHA do feature, **deletar a branch local main** deixando só
`refs/remotes/origin/main` via `git update-ref`). Chamar a função de resolução de
escopo (se o bloco do main.go não for extraível, extrair para
`gitutil.ComputeNewCodeScope(ctx, dir, requestedBase, currentBranch)` e chamar do main)
e afirmar: (a) resolve `origin/main`; (b) devota os arquivos do commit feature.

**Verify**: `go test ./cmd/... ./internal/git/... -count=1 -race` → exit 0

## Test plan

- `internal/git/git_test.go`: casos novos — base local existe; só `origin/<base>`
  existe (fallback); nenhum existe (erro citando ambos); `DefaultBranch` com default
  `trunk` (criar `origin/trunk`); detached HEAD.
- `cmd/crivo/main_test.go`: teste de integração do Step 5; commit hash capturado
  (repo fixture); repo não-git → `commit == ""` sem erro.
- Padrão estrutural: usar `internal/git/git_test.go` existente como modelo de fixtures
  de repo temporário.

## Done criteria

- [ ] `go test ./... -count=1 -race` exit 0 (com CI agora cobrindo `cmd/`)
- [ ] `go vet ./...` exit 0
- [ ] Num repo com só `origin/main` e detached HEAD: `crivo run --new-code --branch main`
      analisa SÓ os arquivos do diff (verificável pelo output `New code: N changed files`)
- [ ] Num repo onde `main` e `origin/main` não existem: `crivo run --new-code --branch main`
      sai com erro não-zero citando os refs tentados
- [ ] `git status` não mostra arquivos fora da lista in-scope
- [ ] `plans/README.md` atualizado

## STOP conditions

- Os excerpts de `main.go`/`git.go` não batem com o código atual (drift).
- O bloco new-code estiver acoplado de um jeito que não permite extrair
  `ComputeNewCodeScope` sem tocar em providers (reportar; não improvisar refactor maior).
- Algum teste existente de `cmd/` falhar por razão que não seja o comportamento
  pretendido (possível sinal de consumidor do escopo vazio que precisa do plano 002).

## Maintenance notes

- O plano 002 depende da garantia deste plano: "escopo presente ⇒ não-vazio".
- Revisor deve conferir que nenhuma mensagem de erro vaza stderr completa de git
  (truncar) e que o exit code do `crivo run` em falha de resolução é estável
  (documentar no README seção `--new-code`).

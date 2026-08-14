# Implementation Plans — crivo

Gerados pela skill `improve` em 2026-08-14, sobre o commit `f3b5007` (v3.4.3).
Contexto: auditoria completa (3 subagentes + verificação manual das citações + auditoria
de segurança anterior + `docs/PERF-AUDIT-2026-08-06.md`). Executar na ordem abaixo, salvo
dependências. Cada executor: leia o plano inteiro antes de começar, honre as STOP
conditions, atualize sua linha ao terminar.

Verificação padrão para todos os planos (repo Go, sem CGO):

| Propósito | Comando | Esperado |
|---|---|---|
| Testes completos | `go test ./... -count=1` | exit 0 |
| Testes com race (o que o CI roda) | `go test ./... -count=1 -race` | exit 0 |
| Vet | `go vet ./...` | exit 0 |
| Build | `go build ./cmd/crivo/` | exit 0, binário `crivo` |
| Wrapper npm | `npm test --prefix npm` | exit 0 (sem rede) |

> ⚠️ Nota: hoje os workflows `ci.yml` e `release.yml` rodam só `go test ./internal/...`,
> excluindo os testes de `cmd/crivo`. O plano 001 corrige isso no Step 1 — executar o
> 001 primeiro faz os gates de verificação de todos os outros planos serem reais.

## Ordem de execução & status

| Plano | Título | Prioridade | Esforço | Depende de | Status |
|------|--------|-----------|--------|------------|--------|
| 001 | `--new-code` resolve a base ou falha alto (nunca repo inteiro em silêncio) | P1 | M | — | DONE (merge 7e5a25c) |
| 002 | Controles de segurança fail-closed (secrets/semgrep nunca passam sem escanear) | P1 | M | 001 | DONE (merge ec27e49; maskSecret sem substrings, semgrep skip visível com rule IDs) |
| 003 | O gate não mente: falha de provider é error; rating/config truth | P1 | M | — | DONE (merge 78a05ce; errored_checks bloqueia release, thresholds do config valem, baseline opt-in) |
| 004 | Perf quick wins (gitleaks batch, regex pré-compilado, heavy classification, workers) | P2 | S | — | DONE (merge 29b1a1d; gitleaks 8.24.3 não aceita multi-target — chunk=1 documentado, allocs/op -98,6%) |
| 005 | Coverage honra new-code + duplication/complexity em todos os `src` | P2 | M | 001, 003 | DONE (merge c78e0b0; `coverage.new-code: off|related|full` default off — suite não roda mais em modo PR; caveat: flags `related` validadas via stub, runners reais pendentes) |
| 006 | Custom rules: confiança do motor (yaml loud, remediation, glob-miss, walkers) | P2 | M | — | DONE (merge b86cc86; yaml quebrado = exit 1, exclude por path relativo, caminho single-rule morto deletado) |
| 007 | Supply chain: checksums, pins, opt-out de auto-install, versão single-source | P2 | M | — | DONE (merge 5d24fa2; semgrep pin 1.173.0) |
| 008 | Custom rules state of the art: fixtures de regra, template regenerado, spike de packs | P3 | M/L | 006 | DONE (merge a4d7974; spike COMPLETOU: packs embedded `pack:security-ts`; review pegou 2 bugs que stub não via: check_id namespaced do semgrep — quebrava o runtime batch silenciosamente — e dialecto ts/.tsx nas fixtures) |

Status: TODO | IN PROGRESS | DONE | BLOCKED (motivo em 1 linha) | REJECTED (racional em 1 linha).

## Notas de dependência

- 002 depende de 001 porque o tratamento de "escopo vazio" fica mais simples depois que
  o 001 transforma escopo vazio em erro fatal na entrada (002 é defesa em profundidade).
- 005 depende de 001 pela mesma razão (escopo confiável antes de consumi-lo).
- 008 depende de 006 (fixtures validam via os matchers; motor precisa estar honesto antes).
- 003 e 004 são independentes entre si e dos demais.

## Racionalização de checks (decisão de produto — recomendação, não plano)

Dados reais das runs do `agenda-aqui` (PR + push main, crivo 3.4.3): coverage 176,7s
(67% do total), typecheck 88,8s (34%), dead-code 11,7s, duplication 8,9s, complexity
5,3s, custom-rules 2,8s, secrets 0ms (escopo). Recomendação:

| Check | Veredito | Motivo |
|---|---|---|
| typescript | **core, blocking** | O check de maior valor por segundo; falha precisa virar error (003). |
| custom-rules | **core, blocking** | O diferencial do produto; precisa ser infalível (002/006/008). |
| secrets | **core, blocking** | Mas só vale algo fail-closed (002). |
| coverage | **scoped ou ratchet** | Suite inteira por PR é 67% do custo; threshold absoluto 60% com baseline 30% é gate inalcançável que ensina o time a ignorar o crivo. Ver 005. |
| duplication | **new-code p/ PR; % só em full** | Percentage repo-wide como blocker reproduz o mesmo problema de gate inalcançável (baseline 7% > 5% sempre). Ver 003 (recompute). |
| complexity | **informational por design** | Nunca foi condição de gate; é input de rating. Ficar disable-able (003) e heavy (004). |
| dead-code | **manter, advisory** | Barato (11,7s), informativo; nunca blocker. |
| semgrep provider (`--config auto`) | **default off** | Sobrepõe-se às custom semgrep rules, usa registry unpinned. A superfície de semgrep deve ser as custom rules (a decisão é do maintainer; o 002 torna as custom rules viáveis). |

## Findings considerados e rejeitados

- Autofix/suggestion nas custom rules: explicitamente out of scope por design
  (`docs/custom-rules-design.md:255`) — decisão registrada, não reabrir.
- Two-semaphore runner design (heavy waiter segura slot geral): nuance de throughput,
  não bug; evidência insuficiente de contenção real.
- Ordem não-determinística de resultados: cosmético.
- Run-lock staleness heuristic: razoável como está.

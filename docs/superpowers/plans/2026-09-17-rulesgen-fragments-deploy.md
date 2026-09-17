# rulesgen fragments deploy (stage 03) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. The server-side steps are **user-performed on rocketman** (AGENTS.md Plan/Act mode; spec §Deployment) — an agent must not run them without explicit approval.

**Goal:** Restore the Review Queue on rocketman by deploying the required `--fragments` wiring from stage 03, verify the four fragment files render, then close issue 03.

**Architecture:** Stage 03 (squash `8170d65`) made `--fragments` a required flag on `rulesgen serve`, but the compose service was never updated, so the container exits at startup ("required flag(s)") and the queue is dark. The repo fix (commit `6e77590` on `stage-03-deploy`) adds the flag plus a writable host bind `${POSTMANPAT_RULESGEN_FRAGMENTS:-./config}:/fragments`. Deploy is a server-side `.env` edit, image rebuild, and service recreate. rulesgen renders fragments only; it never writes live configs (ADR 0005).

**Tech Stack:** Go 1.25 CLI, Docker Compose, SQLite, bash.

## Global Constraints

- Live configs are hand-maintained and must never be written by rulesgen (ADR 0005). The fragment dir only ever receives `watch.yaml`, `cleanup-onetime.yaml`, `cleanup-ongoing.yaml`, `ignore.yaml`; these names must not collide with `config_cleanup.yaml` / `config_watch.yaml` / `config_analyze.yaml`.
- The fragment mount is **writable** (no `:ro`), unlike the other config mounts.
- Repo baseline before deploy: `go build ./...`, `go vet ./...`, `gofmt -l .` clean; `go test ./...` green; `cd bin && python3 -m unittest discover -p "test_*.py"` → 26 tests OK.
- Reports are written root-owned by the cron container; fragment files land root-owned the same way. Host reads need `sudo` or `docker exec` (spec §Deployment).
- `gh` in this shell: run with `GITHUB_TOKEN=` to avoid the invalid env var shadowing `~/.config/gh/hosts.yml`.

---

## Task 1: Repo wiring (DONE — commit `6e77590`)

**Files:**
- Modify: `docker-compose.yml` (`postmanpat-rulesgen` service)
- Modify: `README.md` (rulesgen section)

- [ ] **Step 1: Add the flag and writable mount to the service**

`docker-compose.yml` `postmanpat-rulesgen`:

```yaml
    command: ["postmanpat", "rulesgen", "serve", "--reports", "/analyze-out", "--db", "/data/rulesgen.db", "--fragments", "/fragments"]
    volumes:
      - ${POSTMANPAT_ANALYZE_OUT:-./analyze-out}:/analyze-out:ro
      - ${POSTMANPAT_RULESGEN_DATA:-./rulesgen-data}:/data
      - ${POSTMANPAT_RULESGEN_FRAGMENTS:-./config}:/fragments
```

- [ ] **Step 2: Align the README**

README rulesgen run example uses `--fragments /fragments`; the service paragraph documents `POSTMANPAT_RULESGEN_FRAGMENTS` (default `./config`) mounted writable at `/fragments`.

- [ ] **Step 3: Verify compose resolves**

Run: `docker compose config` and confirm for `postmanpat-rulesgen`:

```
    command:
      ...
      - --fragments
      - /fragments
    volumes:
      ...
      - type: bind
        source: <repo>/config
        target: /fragments
        bind: {}
```

Expected: `--fragments /fragments` present; the `/fragments` bind is NOT `read_only: true`.

- [ ] **Step 4: Verify the flag is required and satisfied**

Run: `go build -o bin ./... && ./bin/postmanpat rulesgen serve`
Expected: `Error: required flag(s) "db", "fragments", "reports" not set` (confirms the flag is required; compose now supplies it).

- [ ] **Step 5: Baseline suite**

```bash
go build ./... && go vet ./... && gofmt -l .
go test ./...
cd bin && python3 -m unittest discover -p "test_*.py"
```

Expected: clean build/vet/gofmt; all Go packages `ok`; 26 Python tests `OK`.

- [ ] **Step 6: Commit**

```bash
git add docker-compose.yml README.md
git commit -m "fix(docker): pass required --fragments flag to rulesgen service"
```

Status: done as `6e77590`; reviewed (verdict: ready to merge, no Critical/Important).

---

## Task 2: Server apply (USER-PERFORMED on rocketman — needs approval)

**Files:** none in this repo; live `.env` and the server's compose checkout only.

- [ ] **Step 1: Preflight the live surface (read-only)**

On rocketman, locate the postmanpat deployment checkout and confirm the fragment host dir exists before any bind is added (a missing bind source is auto-created root-owned — see README gotcha):

```bash
# run from the postmanpat deployment checkout on rocketman
ls -ld /opt/docker/rocketman/postmanpat-config
ls /opt/docker/rocketman/postmanpat-config
docker compose config | sed -n '/postmanpat-rulesgen:/,/^  [a-z]/p'
```

Record: the deployment checkout path, the live `.env` path, and the current `postmanpat-rulesgen` command/mounts.

- [ ] **Step 2: Point the fragment dir at the live config dir in `.env`**

Add to the live `.env`:

```bash
# Writable fragment output dir mounted at /fragments; fragments land next to the
# hand-maintained configs so they can be hand-merged (ADR 0005).
POSTMANPAT_RULESGEN_FRAGMENTS=/opt/docker/rocketman/postmanpat-config
```

Rationale: fragment filenames do not collide with `config_*.yaml`, so the operator reads `watch.yaml` / `cleanup-*.yaml` / `ignore.yaml` from the same directory as the configs they merge into.

- [ ] **Step 3: Deploy the wiring commit**

```bash
git fetch origin && git checkout main && git pull --ff-only
docker compose build postmanpat-rulesgen
docker compose up -d postmanpat-rulesgen
```

- [ ] **Step 4: Confirm the container stays up**

```bash
docker compose ps postmanpat-rulesgen
docker compose logs --tail=30 postmanpat-rulesgen
```

Expected: state `Up` (not `Restarting`/`Exited`); no `required flag(s) "fragments"` error.

---

## Task 3: Verify the deployed service (evidence before claims)

- [ ] **Step 1: `/healthz`**

The runtime image has no curl/wget; query from inside the container with bash `/dev/tcp`:

```bash
docker compose exec postmanpat-rulesgen bash -c \
  'exec 3<>/dev/tcp/127.0.0.1/8092; printf "GET /healthz HTTP/1.0\r\n\r\n" >&3; cat <&3'
```

Expected: HTTP `200` and body `ok`.

- [ ] **Step 2: Fragment files exist and are non-empty**

```bash
sudo ls -l /opt/docker/rocketman/postmanpat-config/watch.yaml \
             /opt/docker/rocketman/postmanpat-config/cleanup-onetime.yaml \
             /opt/docker/rocketman/postmanpat-config/cleanup-ongoing.yaml
# ignore.yaml only exists when something is ignored
sudo cat /opt/docker/rocketman/postmanpat-config/cleanup-ongoing.yaml
```

Expected: the three always-written files exist (stub `rules: []` is valid before any decision); `ignore.yaml` absent or present with an `ignore:` block.

- [ ] **Step 3: Queue page renders**

```bash
docker compose exec postmanpat-rulesgen bash -c \
  'exec 3<>/dev/tcp/127.0.0.1/8092; printf "GET / HTTP/1.0\r\n\r\n" >&3; cat <&3' | head -20
```

Expected: HTML queue page (no 5xx), pending clusters present.

---

## Task 4: Dogfood acceptance (spec §Deployment acceptance)

- [ ] **Step 1: Generate a watch rule and an ongoing-cleanup rule from the live queue** (UI).

- [ ] **Step 2: Confirm fragment YAML is script-shaped**

```bash
sudo cat /opt/docker/rocketman/postmanpat-config/watch.yaml
sudo cat /opt/docker/rocketman/postmanpat-config/cleanup-ongoing.yaml
```

Expected: `rules:` lists matching `bin/postmanpat-generate-rules.py` output shape (ADR 0003), including one unquoted boolean; cleanup recipients split one-rule-per-alias.

- [ ] **Step 3: Snooze round-trip**

Snooze a cluster, then confirm it returns only after a **newer** nightly report re-contains it (an unchanged re-ingest must not resurrect it — see `ClearSnoozed` semantics, `rulesgen` tests).

- [ ] **Step 4: Restart survival**

```bash
docker compose restart postmanpat-rulesgen
```

Expected: decisions and fragments persist (SQLite store).

---

## Task 5: Close out

- [ ] **Step 1: Push branch and open PR** (`GITHUB_TOKEN= gh ...`); merge after green CI.

- [ ] **Step 2: Flip issue 03 to closed**

In `docs/issues/03-rulesgen-decision-capture-fragments.md`: set `**Status:**` to a completion line mirroring issue 02 (commit SHA, PR number, deploy verification date/evidence) and check all eight boxes.

- [ ] **Step 3: Commit the issue close + this plan** (`docs:` message) on the deploy branch.

---

## Self-Review

- **Spec coverage:** `--fragments` required-flag wiring (Task 1), writable fragment bind + server apply (Task 2), healthz/fragments/queue evidence (Task 3), dogfood acceptance incl. unquoted bool + snooze + restart (Task 4, spec line 91), issue 03 close (Task 5, mirrors issue 02). Fragment-merge marking (ADR 0005) is explicitly stage 04 — out of scope.
- **Placeholders:** none; every step has an exact command.
- **Type consistency:** flag `--fragments`, dir `/fragments`, env `POSTMANPAT_RULESGEN_FRAGMENTS`, filenames `watch.yaml` / `cleanup-onetime.yaml` / `cleanup-ongoing.yaml` / `ignore.yaml` used consistently with `rulesgen/render.go` and `rulesgen/serve.go`.

# EZDeploy

EZDeploy puts a backend from Git onto a systemd-based Linux server. It connects repository updates, environment and route discovery, native or Docker execution, Nginx, HTTPS, and the existing deployment registry.

Running `ezdeploy` without a command opens a project-first terminal dashboard. Projects are sorted by name and show status, domain, service count, and current revision. Selecting one opens its overview, redeploy, releases, logs, network, service controls, and removal; host installation, OS details, and metrics live under System.

## Setup

The server needs Git, `sudo`, and a domain pointed at it. EZDeploy installs a selected native service's missing Go, Python, or Node runtime together with missing Nginx, Certbot, and Docker components on its supported apt/dnf distributions. Go 1.25 or newer is needed only to build EZDeploy itself.

Keep the binary beside `yamls/walk.yml`:

```bash
git clone <ezdeploy-repository> /opt/ezdeploy
cd /opt/ezdeploy
go build -o ezdeploy .
sudo ./ezdeploy
```

Projects are stored under `/opt/ezdeploy/projects/<project-name>`. EZDeploy does not upgrade the operating system when it starts.
Memory-heavy installs and builds require at least 512 MiB of currently available RAM or swap. Go builds compile one package at a time to reduce peak memory, and interactive terminals report elapsed time during long-running work.

## Deploy

```bash
sudo /opt/ezdeploy/ezdeploy deploy https://github.com/example/backend
# or deploy several repositories as one rollback batch
sudo /opt/ezdeploy/ezdeploy deploy https://github.com/example/api https://github.com/example/worker
```

The deploy command clones or fast-forwards each repository, scans it, selects a runtime, prepares dependencies and environment values, starts the application, configures Nginx, provisions HTTPS, and saves the result in `registry.json`. If any repository in a batch fails, earlier repositories are restored to their previous Git revisions, units, Nginx configuration, and registry records.

## Redeploy

```bash
sudo /opt/ezdeploy/ezdeploy redeploy https://github.com/example/backend
```

Redeploy requires an existing registry record. It pulls the registered branch, reuses the saved runtime, services, ports, domain and start commands, then rescans routes and every registered service's environment variables before rebuilding and restarting. Existing `.env` values are preserved; newly discovered keys are requested interactively or rejected with `--non-interactive`. The normal deployment rollback protects the previous release if any service fails.

Redeploy accepts the registered name or repository URL:

```bash
sudo ezdeploy redeploy backend
```

| Option | Purpose |
| --- | --- |
| `--branch <name>` | Select a Git branch. |
| `--ssh-key <path>` | Use a private repository key. |
| `--domain <host>` | Set the public domain. |
| `--email <address>` | Set the Let's Encrypt email. |
| `--port <number>` | Override the host application port. |
| `--start <command>` | Set a native start command. |
| `--service <name\|path,...>` | Select one or more detected native services by name, root, or entry file. |
| `--runtime <native\|docker>` | Select native systemd or Docker execution. |
| `--dockerfile <path>` | Select the production Dockerfile. |
| `--docker-context <path>` | Set its repository-relative build context. |
| `--container-port <number>` | Set the internal application port. |
| `--tls-cert <path>` | Use an existing certificate for a wildcard domain. |
| `--tls-key <path>` | Use its existing private key. |
| `--allow-route <path>` | Add an undiscovered route; repeatable. |
| `--no-route-whitelist` | Proxy every application path. |
| `--non-interactive` | Reuse saved values and fail instead of prompting. |

## Native or Docker starter

If Dockerfiles are found during a new interactive deployment, EZDeploy presents the native runtime and every candidate:

```text
Deployment runtime:
  0. Native process + systemd + Nginx
  1. Docker: Dockerfile.prod (node:22-alpine, ports 8080)
  2. Docker: Dockerfile (node:22, ports 3000)
  3. Docker: docker/worker.Dockerfile (python:3.13, no EXPOSE)
```

The walker recognizes nested `Dockerfile`, `Dockerfile.*`, `Dockerfile-*`, and `*.Dockerfile` names. Production-named files are displayed first, but interactive deployment never silently chooses one. It reads the final `FROM` image and numeric `EXPOSE` ports. Ambiguous ports require a prompt or `--container-port`.

Docker images are built before the current container is stopped. The previous container is retained until its replacement starts, allowing startup rollback. Containers use `unless-stopped` and publish only to `127.0.0.1`; Nginx remains the public listener for HTTP, HTTPS, streaming, and route filtering. The process inside the container must listen on `0.0.0.0`.

Runtime, Dockerfile, context, and container port are saved for later deployments. An automated first deployment with multiple files must provide `--dockerfile` and, when necessary, `--container-port`.

## Private repositories and remote redeploys

Create a deploy key on the server, add its public half to the repository provider, and keep the private half at mode `0600`:

```bash
ssh-keygen -t ed25519 -f /home/ubuntu/.ssh/backend_deploy
chmod 600 /home/ubuntu/.ssh/backend_deploy
sudo ezdeploy deploy git@github.com:example/private-backend.git \
  --ssh-key /home/ubuntu/.ssh/backend_deploy
```

The key is passed to Git for that process only and is never copied. Its server-side path is registered so later redeploys can reuse it; the key material is not stored in `registry.json`. SSH provides the remote control channel without exposing another API:

```bash
ssh -i ~/.ssh/ec2.pem ubuntu@server \
  'sudo /opt/ezdeploy/ezdeploy redeploy private-backend --non-interactive'
```

Git updates use `merge --ff-only`; server divergence is rejected rather than overwritten. Non-interactive deployment also stops when a newly discovered environment variable has no saved value.

## Routes, environment, and services

Route and Dockerfile rules extend the existing `yamls/walk.yml`. Common literal Go, Express, FastAPI, and Flask routes become Nginx locations; shared roots such as `/api` own forwarded and streaming headers while only discovered child routes are proxied. Unknown paths still return `404`. Dynamic or cross-file routes can be supplied with `--allow-route`, which must be repeated on later deployments.

The same walker lists Python, Go, and Node backend candidates in mixed-language repositories. Each candidate includes its service root, entry file, likely start command, confidence, and the filename, manifest, and server markers that produced the match. Interactive selection accepts comma-separated indexes such as `2,3`; `--service app/backend,go-backend` provides the non-interactive equivalent. Every selected service receives its own port, systemd unit, registry service record, working directory, dependency preparation, and Nginx route targets. A failure restores the repository as one release rather than leaving half of a monorepo running the new revision.

Existing `.env` values are preserved at mode `0600`. Native applications run as `SUDO_USER`, not root. Go applications rebuild on update; conventional commands are detected for Go, `npm start`, and FastAPI `main:app`.

## Releases and rollback

EZDeploy retains the latest 20 successful deploy, redeploy, and rollback events. Each record contains a release ID, Git revision, time, and operation. A matching `refs/ezdeploy/releases/<release-id>` reference prevents Git garbage collection from deleting its commit.

```bash
ezdeploy releases backend
sudo ezdeploy rollback backend --release 20260825T120000Z-a1b2c3d4
```

Rollback checks out the saved revision without pulling, rescans routes and environment requirements, rebuilds every registered service, regenerates Nginx, and runs the normal startup checks. Current `.env` files remain untouched. The confirmation warns that database schema and data are not rolled back. A failed rollback restores the exact previous attached branch or detached HEAD, services, containers, Nginx files, and registry state.

Legacy registry entries gain a release for their current revision when their next deployment succeeds.

## Logs and network diagnostics

```bash
sudo ezdeploy logs backend --source deployment --lines 100
sudo ezdeploy logs backend --source runtime --service api --lines 100 --follow
ezdeploy network backend
```

Deployment summaries go to journald under `ezdeploy`; they contain only project, operation, and result, never environment values, credentials, private keys, or user command arguments. Runtime output is read from each service's systemd journal or Docker container. A multi-service project requires `--service` for runtime logs.

Network diagnostics query EC2 IMDSv2 with a short timeout and compare its public IPv4 address with DNS. A mismatch prints the existing A records and the exact address to use, with no redeploy required. If metadata is unavailable, EZDeploy does not call an external IP service. An Elastic IP is the stable fix: an EC2 reboot retains the public address, while stop/start normally changes it ([AWS instance lifecycle](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ec2-instance-lifecycle.html)).

## Wildcard domains

Only a single leading wildcard such as `*.example.com` is accepted. EZDeploy generates Nginx server-name matching for exactly one child label and expects the wildcard A record to point at the instance.

```bash
sudo ezdeploy deploy https://github.com/example/backend \
  --domain '*.example.com' \
  --tls-cert /etc/letsencrypt/live/example.com/fullchain.pem \
  --tls-key /etc/letsencrypt/live/example.com/privkey.pem
```

Wildcard projects require existing certificate and key paths. EZDeploy verifies that the certificate covers a representative child hostname, persists only the paths, and lets `nginx -t` validate certificate/key loading. It renders explicit HTTP-to-HTTPS and TLS blocks and does not attempt automatic issuance because wildcard certificates require DNS validation ([Certbot documentation](https://eff-certbot.readthedocs.io/_/downloads/en/stable/pdf/)). Ordinary domains keep the existing Certbot flow.

## Current boundaries

- Nginx + Certbot is the implemented ingress; Traefik and cloud ingress are later providers.
- There is no custom HTTP deployment API or application-level HTTP health check. Startup checks currently verify the systemd unit or Docker container state.
- Rollback restores Git revisions and EZDeploy-managed units, Nginx files, containers, and registry records; it does not restore external databases or other application state.
- Route discovery is literal. Multi-service unrestricted proxying is intentionally rejected because one catch-all Nginx location cannot choose between several service ports.
- The terminal metrics view is local; a managed observability provider has not been selected or integrated.
- Fresh-instance migration, state backup, and automatic Route 53 mutation are not implemented. A fresh host still starts with installation and deployment.
- Installation and binary upgrades are not packaged; keep the binary and `yamls` together.
- Builds execute trusted repository content. Do not deploy an untrusted repository as an administrator.

## Verification

```bash
go test ./...
go vet ./...
go test -race ./...
```

Tests cover Git fast-forward updates, release retention and refs, attached/detached restoration, environment preservation, dashboard ordering and navigation, log providers, EC2/DNS outcomes, wildcard TLS rendering, mixed Python/Go/Node service discovery, Dockerfile discovery and path containment, loopback-only Docker arguments, runtime selection, route-to-Nginx generation, real Nginx parsing when available, and non-root systemd rendering.

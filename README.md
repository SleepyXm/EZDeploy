# EZDeploy

EZDeploy puts a backend from Git onto a systemd-based Linux server. It connects repository updates, environment and route discovery, native or Docker execution, Nginx, HTTPS, and the existing deployment registry.

Running `ezdeploy` without a command opens the terminal UI.

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

The key is passed to Git for that process only and is not copied or registered. Supply it again during a private redeploy. SSH provides the remote control channel without exposing another API:

```bash
ssh -i ~/.ssh/ec2.pem ubuntu@server \
  'sudo /opt/ezdeploy/ezdeploy redeploy git@github.com:example/private-backend.git --ssh-key /home/ubuntu/.ssh/backend_deploy --non-interactive'
```

Git updates use `merge --ff-only`; server divergence is rejected rather than overwritten. Non-interactive deployment also stops when a newly discovered environment variable has no saved value.

## Routes, environment, and services

Route and Dockerfile rules extend the existing `yamls/walk.yml`. Common literal Go, Express, FastAPI, and Flask routes become Nginx locations; shared roots such as `/api` own forwarded and streaming headers while only discovered child routes are proxied. Unknown paths still return `404`. Dynamic or cross-file routes can be supplied with `--allow-route`, which must be repeated on later deployments.

The same walker lists Python, Go, and Node backend candidates in mixed-language repositories. Each candidate includes its service root, entry file, likely start command, confidence, and the filename, manifest, and server markers that produced the match. Interactive selection accepts comma-separated indexes such as `2,3`; `--service app/backend,go-backend` provides the non-interactive equivalent. Every selected service receives its own port, systemd unit, registry service record, working directory, dependency preparation, and Nginx route targets. A failure restores the repository as one release rather than leaving half of a monorepo running the new revision.

Existing `.env` values are preserved at mode `0600`. Native applications run as `SUDO_USER`, not root. Go applications rebuild on update; conventional commands are detected for Go, `npm start`, and FastAPI `main:app`.

## Current boundaries

- Nginx + Certbot is the implemented ingress; Traefik and cloud ingress are later providers.
- There is no custom HTTP deployment API or application-level HTTP health check. Startup checks currently verify the systemd unit or Docker container state.
- Rollback restores Git revisions and EZDeploy-managed units, Nginx files, containers, and registry records; it does not restore external databases or other application state.
- Route discovery is literal. Multi-service unrestricted proxying is intentionally rejected because one catch-all Nginx location cannot choose between several service ports.
- The terminal metrics view is local; a managed observability provider has not been selected or integrated.
- Installation and binary upgrades are not packaged; keep the binary and `yamls` together.
- Builds execute trusted repository content. Do not deploy an untrusted repository as an administrator.

## Verification

```bash
go test ./...
go vet ./...
go test -race ./...
```

Tests cover Git fast-forward updates, environment preservation, mixed Python/Go/Node service discovery, Dockerfile discovery and path containment, loopback-only Docker arguments, runtime selection, route-to-Nginx generation, real Nginx parsing when available, and non-root systemd rendering.

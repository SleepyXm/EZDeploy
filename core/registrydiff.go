package core

// In registry.go — add SigningKey and SSHKey to the Project struct:

//type Project struct {
//	Path          string `json:"path"`
//	Port          int    `json:"port"`
//	Domain        string `json:"domain"`
//	Email         string `json:"email,omitempty"`
//	RepoURL       string `json:"repo_url"`
//	Branch        string `json:"branch"`
//	Status        string `json:"status"`
//	ServiceName   string `json:"service_name"`
//	StartCommand  string `json:"start_command"`
//	Runtime       string `json:"runtime,omitempty"`
//	Dockerfile    string `json:"dockerfile,omitempty"`
//	DockerContext string `json:"docker_context,omitempty"`
//	ContainerPort int    `json:"container_port,omitempty"`
//	SigningKey    string `json:"signing_key,omitempty"` // Ed25519 private key seed (base64) — never logged
//	SSHKey        string `json:"ssh_key,omitempty"`     // path to SSH private key for private repos
//}

// ── Deploy flow wiring (wherever your deploy command finishes) ────────────────
//
// After RegisterProject succeeds, call SetSigningKey then IssueToken:
//
//   if err := core.SetSigningKey(projectName); err != nil {
//       return err
//   }
//   token, err := core.IssueToken(projectName, 0) // 0 = no expiry
//   if err != nil {
//       return err
//   }
//   fmt.Printf("\n[✓] Pull token (save this — shown once):\n\n  %s\n\n", token)
//
// The token is printed once and never stored. The user saves it to their
// local ~/.ezdeploy/<project>.json via `ezdeploy pull --save <token>`.

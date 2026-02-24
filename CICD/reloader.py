import subprocess
import os
from CICD.registry import load_registry
from CICD.deps import install_deps, get_go_binary_name

PROJECTS_DIR = "/opt/EZDeploy/projects"

def reload():
    registry = load_registry()

    if not registry:
        print("[!] No projects deployed")
        return

    print("\n[→] Deployed projects:\n")
    for name, info in registry.items():
        print(f"  {name} — {info['domain']} (port {info['port']})")

    project_name = input("\n[?] Project to reload: ").strip()

    if project_name not in registry:
        print(f"[!] {project_name} not found")
        return

    project_path = os.path.join(PROJECTS_DIR, project_name)

    print(f"[→] Pulling latest changes...")
    subprocess.run(["git", "-C", project_path, "pull"], check=True)
    print(f"[✓] Pulled latest")

    # rebuild if Go
    is_go = os.path.exists(os.path.join(project_path, "go.mod"))
    if is_go:
        binary_name = get_go_binary_name(project_path)
        binary_path = os.path.join(project_path, binary_name)
        if os.path.exists(binary_path):
            print(f"[✓] Pre-built binary found, skipping build...")
            subprocess.run(["chmod", "+x", binary_path], check=True)
        else:
            print(f"[→] Building Go binary...")
            subprocess.run(["go", "build", "-buildvcs=false", "-o", binary_name, "."], cwd=project_path, check=True)
            print(f"[✓] Binary built")
    else:
        install_deps(project_path)

    service_name = f"{project_name}.service"
    print(f"[→] Restarting {service_name}...")
    subprocess.run(["systemctl", "restart", service_name], check=True)
    print(f"[✓] {project_name} reloaded successfully\n")


def reload_project(project_name: str):
    registry = load_registry()
    project_path = os.path.join(PROJECTS_DIR, project_name)

    subprocess.run(["git", "-C", project_path, "pull"], check=True)

    is_go = os.path.exists(os.path.join(project_path, "go.mod"))
    if is_go:
        binary_name = get_go_binary_name(project_path)
        binary_path = os.path.join(project_path, binary_name)
        if os.path.exists(binary_path):
            subprocess.run(["chmod", "+x", binary_path], check=True)
        else:
            subprocess.run(["go", "build", "-buildvcs=false", "-o", binary_name, "."], cwd=project_path, check=True)
    else:
        install_deps(project_path)

    subprocess.run(["systemctl", "restart", f"{project_name}.service"], check=True)

if __name__ == "__main__":
    reload()
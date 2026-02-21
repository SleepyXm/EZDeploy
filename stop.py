import subprocess
import os
from registry import load_registry, unregister_project

NGINX_SITES_AVAILABLE = "/etc/nginx/sites-available"
NGINX_SITES_ENABLED = "/etc/nginx/sites-enabled"
SYSTEMD_DIR = "/etc/systemd/system"
PROJECTS_DIR = "/opt/deployer/projects"

def undeploy():
    registry = load_registry()

    if not registry:
        print("[!] No projects deployed")
        return

    print("\n[→] Deployed projects:\n")
    for name, info in registry.items():
        print(f"  {name} — {info['domain']} (port {info['port']})")

    project_name = input("\n[?] Project to remove: ").strip()

    if project_name not in registry:
        print(f"[!] {project_name} not found")
        return

    service_name = f"{project_name}.service"

    print(f"[→] Stopping {project_name}...")
    subprocess.run(["systemctl", "stop", service_name], check=True)
    subprocess.run(["systemctl", "disable", service_name], check=True)

    service_path = os.path.join(SYSTEMD_DIR, service_name)
    if os.path.exists(service_path):
        os.remove(service_path)
    subprocess.run(["systemctl", "daemon-reload"], check=True)
    print(f"[✓] Service removed")

    for path in [
        os.path.join(NGINX_SITES_ENABLED, project_name),
        os.path.join(NGINX_SITES_AVAILABLE, project_name)
    ]:
        if os.path.exists(path):
            os.remove(path)
    subprocess.run(["systemctl", "reload", "nginx"], check=True)
    print(f"[✓] Nginx config removed")

    project_path = os.path.join(PROJECTS_DIR, project_name)
    if os.path.exists(project_path):
        subprocess.run(["rm", "-rf", project_path], check=True)
    print(f"[✓] Project files removed")

    unregister_project(project_name)

    print(f"\n[✓] {project_name} has been fully removed\n")

if __name__ == "__main__":
    undeploy()
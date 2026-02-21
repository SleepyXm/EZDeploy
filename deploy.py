import os
from git import clone_repo
from deps import download_deps
from service import create_service
from nginx import create_nginx_config
from registry import get_next_port, register_project
from env import setup_env

def deploy():
    print("\n[→] EZDeploy\n")

    repo_url = input("[?] GitHub repo URL: ").strip()
    entrypoint = input("[?] Entrypoint (e.g. main:app or index.js): ").strip()
    domain = input("[?] Domain (e.g. app.yourdomain.com or yourdomain.com/app): ").strip()
    email = input("[?] Email (for SSL certificate): ").strip()

    # step 1 - clone
    project_path = clone_repo(repo_url)
    project_name = os.path.basename(project_path)

    # step 2 - dependencies & env
    download_deps(project_path)
    setup_env(project_path)

    # step 3 - port
    port = get_next_port()

    # step 4 - systemd service
    create_service(project_path, entrypoint, port)

    # step 5 - nginx
    create_nginx_config(project_name, port, domain, email)

    # step 6 - register
    register_project(project_name, port, domain)

    print(f"\n[✓] {project_name} is live at {domain}\n")

if __name__ == "__main__":
    deploy()
import subprocess
import os
from CICD.git import clone_repo
from CICD.deps import download_deps
from CICD.service import create_service
from CICD.nginx import create_nginx_config
from CICD.registry import get_next_port, register_project
from CICD.env import setup_env

def start_webhook_listener():
    result = subprocess.run(["systemctl", "is-active", "ezdeploy-webhook"], capture_output=True, text=True)
    if result.stdout.strip() == "active":
        print("[✓] Webhook listener already running")
        return
    
    ezdeploy_dir = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
    
    service = f"""[Unit]
Description=EZDeploy Webhook Listener
After=network.target

[Service]
ExecStart=/usr/bin/python3 -m uvicorn main:app --host 0.0.0.0 --port 9000
WorkingDirectory={ezdeploy_dir}/EZDeploy
Restart=always

[Install]
WantedBy=multi-user.target
"""
    service_path = "/etc/systemd/system/ezdeploy-webhook.service"
    
    with open(service_path, "w") as f:
        f.write(service)
    
    subprocess.run(["systemctl", "daemon-reload"], check=True)
    subprocess.run(["systemctl", "enable", "ezdeploy-webhook"], check=True)
    subprocess.run(["systemctl", "start", "ezdeploy-webhook"], check=True)
    
    print("[✓] Webhook listener running on port 9000")


def deploy():
    print("\n[→] EZDeploy\n")

    repo_url = input("[?] GitHub repo URL: ").strip()

    # step 1 - clone
    project_path = clone_repo(repo_url)
    project_name = os.path.basename(project_path)

    # step 2 - entrypoint (skip for Go)
    is_go = os.path.exists(os.path.join(project_path, "go.mod"))
    if not is_go:
        entrypoint = input("[?] Entrypoint (e.g. main:app or index.js): ").strip()
    else:
        entrypoint = ""

    domain = input("[?] Domain (e.g. app.yourdomain.com or yourdomain.com/app): ").strip()
    email = input("[?] Email (for SSL certificate): ").strip()
    branch = input("[?] Branch to deploy (e.g. main): ").strip()

    # step 3 - dependencies & env
    download_deps(project_path)
    setup_env(project_path)

    # step 4 - port
    port = get_next_port()

    # step 5 - systemd service
    create_service(project_path, entrypoint, port)

    # step 6 - nginx
    create_nginx_config(project_name, port, domain, email)

    # step 7 - register
    register_project(project_name, port, domain, repo_url, branch)

    start_webhook_listener()

    print(f"\n[✓] {project_name} is live at {domain}\n")



if __name__ == "__main__":
    deploy()
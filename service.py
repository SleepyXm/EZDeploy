import os, subprocess

SYSTEM_DIR = "/etc/systemd/system"

def create_service(project_path: str, entrypoint: str, port: int):
    project_name = os.path.basename(project_path)
    service_name = f"{project_name}.service"
    service_path = os.path.join(SYSTEM_DIR, service_name)

    is_python = os.path.exists(os.path.join(project_path, "requirements.txt"))
    is_node = os.path.exists(os.path.join(project_path, "package.json"))

    if is_python:
        exec_start = f"/usr/local/bin/uvicorn {entrypoint} --host 0.0.0.0 --port {port}"
    elif is_node:
        exec_start = f"/usr/bin/node {entrypoint}"
    else:
        print("[!] Could not find project type, stopping...")
        return
    
    service_content = f"""[Unit]
    Description={project_name}
    After=network.target

    [Service]
    WorkingDirectory={project_path}
    ExecStart={exec_start}
    Restart=always

    [Install]
    WantedBy=multi-user.target
    """

    print(f"[→] Writing service file for {project_name}...")
    with open(service_path, "w") as f:
        f.write(service_content)
    subprocess.run(["systemctl", "daemon-reload"], check=True)
    subprocess.run(["systemctl", "enable", service_name], check=True)
    subprocess.run(["systemctl", "start", service_name], check=True)
    print(f"[✓] Service {service_name} is running on port {port}")
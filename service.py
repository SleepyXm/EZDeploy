import os, subprocess

SYSTEM_DIR = "/etc/systemd/system"

def get_go_binary_name(project_path: str) -> str:
    go_mod = os.path.join(project_path, "go.mod")
    with open(go_mod, "r") as f:
        for line in f:
            if line.startswith("module"):
                module = line.split()[-1].strip()
                return module.split("/")[-1].lower()
    return "app"

def create_service(project_path: str, entrypoint: str, port: int):
    project_name = os.path.basename(project_path)
    service_name = f"{project_name}.service"
    service_path = os.path.join(SYSTEM_DIR, service_name)

    is_python = os.path.exists(os.path.join(project_path, "requirements.txt"))
    is_node = os.path.exists(os.path.join(project_path, "package.json"))
    is_go = os.path.exists(os.path.join(project_path, "go.mod"))

    if is_python:
        exec_start = f"/usr/local/bin/uvicorn {entrypoint} --host 0.0.0.0 --port {port}"
    if is_go:
        binary_name = get_go_binary_name(project_path)
        exec_start = f"{project_path}/{binary_name}"

    if is_node:
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
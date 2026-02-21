import json
import os

REGISTRY_PATH = "./reg.json"
STARTING_PORT = 8000

def load_registry() -> dict:
    if not os.path.exists(REGISTRY_PATH):
        return {}
    with open(REGISTRY_PATH, "r") as f:
        return json.load(f)

def save_registry(registry: dict):
    with open(REGISTRY_PATH, "w") as f:
        json.dump(registry, f, indent=2)

def get_next_port() -> int:
    registry = load_registry()
    if not registry:
        return STARTING_PORT
    used_ports = [p["port"] for p in registry.values()]
    port = STARTING_PORT
    while port in used_ports:
        port += 1
    return port

def register_project(project_name: str, port: int, domain: str):
    registry = load_registry()
    registry[project_name] = {
        "port": port,
        "domain": domain,
        "status": "running"
    }
    save_registry(registry)
    print(f"[✓] Registered {project_name} on port {port}")

def unregister_project(project_name: str):
    registry = load_registry()
    if project_name in registry:
        del registry[project_name]
        save_registry(registry)
        print(f"[✓] Unregistered {project_name}")
    else:
        print(f"[!] {project_name} not found in registry")

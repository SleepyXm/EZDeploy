import subprocess, os, re

def get_go_binary_name(project_path: str) -> str:
    go_mod = os.path.join(project_path, "go.mod")
    with open(go_mod, "r") as f:
        for line in f:
            if line.startswith("module"):
                module = line.split()[-1].strip()
                return module.split("/")[-1].lower()
    return "app"

def download_deps(project_path: str):
    requirements = os.path.join(project_path, "requirements.txt")
    package_json = os.path.join(project_path, "package.json")
    go_mod = os.path.join(project_path, "go.mod")

    if os.path.exists(requirements):
        print("[→] Python project found, installing dependencies...")
        subprocess.run(["pip", "install", "-r", requirements], check=True)
        print("[✓] Dependencies installed")

    elif os.path.exists(go_mod):
        binary_name = get_go_binary_name(project_path)
        print(f"[→] Go project detected, building binary {binary_name}...")
        subprocess.run(["go", "build", "-o", binary_name, "."], cwd=project_path, check=True)
        print(f"[✓] Binary built: {binary_name}")


    elif os.path.exists(package_json):
        print("[→] Node project found, installing dependencies...")
        subprocess.run(["npm", "install"], cwd=project_path, check=True)

    else:
        print("[!] No dependency file found. Skipping...")

import subprocess, os

def download_deps(project_path: str):
    requirements = os.path.join(project_path, "requirements.txt")
    package_json = os.path.join(project_path, "package.json")

    if os.path.exists(requirements):
        print("[→] Python project found, installing dependencies...")
        subprocess.run(["pip", "install", "-r", requirements], check=True)
        print("[✓] Dependencies installed")

    elif os.path.exists(package_json):
        print("[→] Node project found, installing dependencies...")
        subprocess.run(["npm", "install"], cwd=project_path, check=True)

    else:
        print("[!] No dependency file found. Skipping...")

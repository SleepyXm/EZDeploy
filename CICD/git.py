import subprocess
import os

CLONE_DIR = "/opt/EZDeploy/projects"

def clone_repo(repo_url: str) -> str:
    repo_name = repo_url.rstrip("/").split("/")[-1].replace(".git", "")
    destination = os.path.join(CLONE_DIR, repo_name)

    if os.path.exists(destination):
        print(f"[→] Repo already exists, pulling latest...")
        subprocess.run(["git", "-C", destination, "pull"], check=True)
    else:
        print(f"[→] Cloning {repo_url}...")
        subprocess.run(["git", "clone", repo_url, destination], check=True)

    print(f"[✓] Ready at {destination}")
    return destination
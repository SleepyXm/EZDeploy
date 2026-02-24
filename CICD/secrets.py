import os

def save_secret(secret: str):
    secrets_path = "./EZDeploy/.secrets"
    with open(secrets_path, "w") as f:
        f.write(f"WEBHOOK_SECRET={secret}")
    os.chmod(secrets_path, 0o600)

def load_secret() -> str:
    with open("./EZDeploy/.secrets", "r") as f:
        for line in f:
            if line.startswith("WEBHOOK_SECRET="):
                return line.strip().split("=", 1)[1]
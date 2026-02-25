import os

def save_secret(secret: str):
    secrets_path = os.path.join(os.path.dirname(__file__), "../.secrets")
    with open(secrets_path, "w") as f:
        f.write(f"WEBHOOK_SECRET={secret}")
    os.chmod(secrets_path, 0o600)

def load_secret() -> str:
    secrets_path = os.path.join(os.path.dirname(__file__), "../.secrets")
    with open(secrets_path, "r") as f:
        for line in f:
            if line.startswith("WEBHOOK_SECRET="):
                return line.strip().split("=", 1)[1]
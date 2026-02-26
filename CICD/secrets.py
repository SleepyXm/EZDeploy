import os

SECRETS_PATH = os.path.join(os.path.dirname(os.path.abspath(__file__)), "../.secrets")

def save_secret(secret: str):
    with open(SECRETS_PATH, "w") as f:
        f.write(f"WEBHOOK_SECRET={secret}")
    os.chmod(SECRETS_PATH, 0o600)

def load_secret() -> str:
    with open(SECRETS_PATH, "r") as f:
        for line in f:
            if line.startswith("WEBHOOK_SECRET="):
                return line.strip().split("=", 1)[1]

def setup_secret():
    print("\n[?] Webhook secret (for GitHub auto-deploy, leave blank to skip):\n")
    value = input("    Secret: ").strip()
    
    if not value:
        print("[!] No secret entered, skipping...")
        return

    save_secret(value)
    print("\n[✓] Webhook secret saved")
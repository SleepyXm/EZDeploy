import subprocess
import os

def install():
    print("[→] Installing dependencies...")
    subprocess.run(["amazon-linux-extras", "install", "nginx1", "-y"], check=True)
    subprocess.run(["systemctl", "enable", "nginx"], check=True)
    subprocess.run(["systemctl", "start", "nginx"], check=True)

    # create dirs nginx expects
    os.makedirs("/etc/nginx/sites-available", exist_ok=True)
    os.makedirs("/etc/nginx/sites-enabled", exist_ok=True)

    print("[→] Installing certbot...")
    subprocess.run(["pip3", "install", "certbot", "certbot-nginx"], check=True)

    print("[✓] Server ready")

if __name__ == "__main__":
    install()
import subprocess
import os
from osdetect import get_os

def get_package_manager():
    os_id = get_os()
    if os_id in ["ubuntu", "debian"]:
        return "apt"
    elif os_id in ["amzn", "fedora", "rhel", "centos"]:
        return "dnf"
    else:
        return None

def install(languages: list[str]):

    package_manager = get_package_manager()
    if not package_manager:
        print("[!] Unsupported OS, cannot install dependencies")
        return
    
    print(f"[→] Found package manager: {package_manager}")

    if "go" in languages:
        print("[→] Installing Go...")
        if package_manager == "apt":
            subprocess.run([package_manager, "install", "golang-go", "-y"], check=True)
        else:
            subprocess.run([package_manager, "install", "golang", "-y"], check=True)

    if "python" in languages:
        print("[→] Installing Python...")
        subprocess.run([package_manager, "install", "python3", "python3-pip", "-y"], check=True)

    if "node" in languages:
        print("[→] Installing Node...")
        subprocess.run([package_manager, "install", "nodejs", "npm", "-y"], check=True)

    if "rust" in languages:
        print("[→] Installing Rust...")
        subprocess.run(["curl", "--proto", "=https", "--tlsv1.2", "-sSf", "https://sh.rustup.rs", "-o", "rustup.sh"], check=True)
        subprocess.run(["sh", "rustup.sh", "-y"], check=True)

    print("[→] Installing nginx...")
    subprocess.run([package_manager, "install", "nginx", "-y"], check=True)

    print("[→] Installing dependencies...")
    subprocess.run(["systemctl", "enable", "nginx"], check=True)
    subprocess.run(["systemctl", "start", "nginx"], check=True)

    # create dirs nginx expects
    os.makedirs("/etc/nginx/sites-available", exist_ok=True)
    os.makedirs("/etc/nginx/sites-enabled", exist_ok=True)

    # add sites-enabled include to nginx.conf
    nginx_conf = "/etc/nginx/nginx.conf"
    with open(nginx_conf, "r") as f:
        content = f.read()

    if "sites-enabled" not in content:
        content = content.replace(
            "include /etc/nginx/conf.d/*.conf;",
            "include /etc/nginx/conf.d/*.conf;\n    include /etc/nginx/sites-enabled/*;"
        )
        with open(nginx_conf, "w") as f:
            f.write(content)
        print("[✓] Added sites-enabled to nginx.conf")
    else:
        print("[✓] nginx.conf already configured")

    print("[→] Installing certbot...")
    subprocess.run(["pip3", "install", "certbot", "certbot-nginx"], check=True)

    print("[✓] Server ready")

if __name__ == "__main__":
    print("\n[→] EZDeploy Server Setup\n")
    print("Available languages: python, node, go, rust")
    langs = input("[?] Languages to install (comma separated): ").strip().lower().split(",")
    langs = [l.strip() for l in langs]
    install(langs)
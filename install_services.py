import subprocess
import os

def install():
    print("[→] Installing dependencies...")
    subprocess.run(["dnf", "install", "nginx", "-y"], check=True)
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

    print("[→] Installing certbot...")
    subprocess.run(["pip3", "install", "certbot", "certbot-nginx"], check=True)

    print("[✓] Server ready")

if __name__ == "__main__":
    install()
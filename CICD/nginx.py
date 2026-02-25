import subprocess
import os

NGINX_SITES_AVAILABLE = "/etc/nginx/sites-available"
NGINX_SITES_ENABLED = "/etc/nginx/sites-enabled"


def location_block(paths: list[str], port: int, streaming: bool = False) -> str:
    if len(paths) == 1 and paths[0] == "/":
        pattern = "/"
        location_type = "location"
    else:
        pattern = "^/(" + "|".join(paths) + ")"
        location_type = "location ~"
    
    base = f"""    {location_type} {pattern} {{
        proxy_pass http://127.0.0.1:{port};
        proxy_http_version 1.1;
        proxy_set_header Host $host;"""
    
    if streaming:
        base += """
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_cache_bypass $http_upgrade;
        proxy_buffering off;"""
    
    base += "\n    }"
    return base

def create_nginx_config(project_name: str, port: int, domain: str, email: str):
    config_path = os.path.join(NGINX_SITES_AVAILABLE, project_name)
    symlink_path = os.path.join(NGINX_SITES_ENABLED, project_name)

    webhook_port = 9000

    if "/" in domain.split(".")[-1]:
        domain_part, path_part = domain.split("/", 1)
        location = f"/{path_part}"
    else:
        domain_part = domain
        location = "/"

    config_content = f"""server {{
    listen 80;
    server_name {domain_part};

{location_block([location], port, streaming=True)}

{location_block(["/gh-webhook"], webhook_port)}
}}
"""

    print(f"[→] Writing nginx config for {project_name}...")
    with open(config_path, "w") as f:
        f.write(config_content)

    if not os.path.exists(symlink_path):
        os.symlink(config_path, symlink_path)
        print(f"[✓] Symlinked to sites-enabled")

    print(f"[→] Testing nginx config...")
    subprocess.run(["nginx", "-t"], check=True)

    print(f"[→] Reloading nginx...")
    subprocess.run(["systemctl", "reload", "nginx"], check=True)

    print(f"[✓] Nginx configured")

    setup_ssl(domain_part, email)


def setup_ssl(domain: str, email:str):
    print(f"[→] Setting up SSL for {domain}...")
    subprocess.run(["certbot", "--nginx", "-d", domain, "--non-interactive", "--agree-tos", "--redirect", "-m", email], check=True)
    print(f"[✓] SSL configured, {domain} is now HTTPS")
import subprocess
import os
import time
import re
from CICD.registry import load_registry

def get_service_status(service_name: str) -> str:
    result = subprocess.run(
        ["systemctl", "is-active", service_name],
        capture_output=True, text=True
    )
    return result.stdout.strip()

def get_service_uptime(service_name: str) -> str:
    result = subprocess.run(
        ["systemctl", "show", service_name, "--property=ActiveEnterTimestamp"],
        capture_output=True, text=True
    )
    line = result.stdout.strip()
    if "=" in line:
        return line.split("=", 1)[1]
    return "unknown"

def get_service_memory(service_name: str) -> str:
    result = subprocess.run(
        ["systemctl", "show", service_name, "--property=MemoryCurrent"],
        capture_output=True, text=True
    )
    line = result.stdout.strip()
    if "=" in line:
        val = line.split("=")[1].strip()
        if val.isdigit():
            mb = int(val) / (1024 * 1024)
            return f"{mb:.1f}MB"
    return "unknown"

def get_system_stats() -> dict:
    # CPU
    cpu = subprocess.run(
        ["top", "-bn1"],
        capture_output=True, text=True
    ).stdout
    cpu_line = [l for l in cpu.split("\n") if "%Cpu" in l or "Cpu(s)" in l]
    if cpu_line:
        line = cpu_line[0]
        
    # extract idle and calculate usage
    idle = re.search(r'(\d+\.?\d*)\s*id', line)
    if idle:
        usage = 100 - float(idle.group(1))
        cpu_usage = f"{usage:.1f}%"

    # RAM
    mem = subprocess.run(["free", "-h"], capture_output=True, text=True).stdout
    mem_line = [l for l in mem.split("\n") if l.startswith("Mem:")][0].split()
    ram_total = mem_line[1]
    ram_used = mem_line[2]

    # Disk
    disk = subprocess.run(["df", "-h", "/"], capture_output=True, text=True).stdout
    disk_line = disk.split("\n")[1].split()
    disk_used = disk_line[2]
    disk_total = disk_line[1]

    return {
        "cpu": cpu_usage,
        "ram": f"{ram_used}/{ram_total}",
        "disk": f"{disk_used}/{disk_total}"
    }

def clear():
    os.system("clear")

def render(registry: dict, stats: dict):
    clear()
    print("\n[→] EZDeploy Metrics\n")
    print(f"  {'PROJECT':<20} {'STATUS':<10} {'PORT':<8} {'MEMORY':<12} {'STARTED'}")
    print(f"  {'-------':<20} {'------':<10} {'----':<8} {'------':<12} {'-------'}")

    for name, info in registry.items():
        service_name = f"{name}.service"
        status = get_service_status(service_name)
        memory = get_service_memory(service_name)
        uptime = get_service_uptime(service_name)
        port = info.get("port", "?")
        status_display = f"[✓] {status}" if status == "active" else f"[✗] {status}"
        print(f"  {name:<20} {status_display:<10} {str(port):<8} {memory:<12} {uptime}")

    print(f"\n  CPU:  {stats['cpu']}")
    print(f"  RAM:  {stats['ram']}")
    print(f"  Disk: {stats['disk']}")
    print(f"\n  refreshing every 5s — ctrl+c to exit\n")

def metrics():
    print("[→] Starting EZDeploy metrics...")
    try:
        while True:
            registry = load_registry()
            stats = get_system_stats()
            render(registry, stats)
            time.sleep(5)
    except KeyboardInterrupt:
        clear()
        print("[✓] Metrics stopped\n")

if __name__ == "__main__":
    metrics()
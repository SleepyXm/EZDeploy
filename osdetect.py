import platform
import os, subprocess

def get_os():
    print(os.name)
    print(platform.system())
    print(platform.release())
    if os.path.exists("/etc/os-release"):
        with open("/etc/os-release") as f:
            for line in f:
                if line.startswith("ID="):
                    return line.strip().split("=")[1].strip('"').lower()
    return platform.system().lower(), os.name, platform.release()

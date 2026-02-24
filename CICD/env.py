import os

def setup_env(project_path: str):
    env_output = os.path.join(project_path, ".env")

    print("\n[→] Enter environment variables (leave key blank when done):\n")

    env_values = {}
    while True:
        key = input("    Key: ").strip()
        if not key:
            break
        value = input("    Value: ").strip()
        env_values[key] = value

    if not env_values:
        print("[!] No environment variables entered, skipping...")
        return

    with open(env_output, "w") as f:
        for key, value in env_values.items():
            f.write(f"{key}={value}\n")

    print("\n[✓] .env file created")
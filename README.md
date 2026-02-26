## EZDeploy
A lightweight deployment tool that gets your project live in minutes — 
handles dependencies, nginx config, SSL, and domain setup automatically.
Push to GitHub and your server updates itself.

## Requirements
- A Linux VPS or cloud instance (AWS, DigitalOcean, Hetzner, etc.)
- Python 3.9+
- Git installed on your instance
- A domain pointed at your instance

## Usage

### 1. Clone the repo
git clone https://github.com/SleepyXm/EZDeploy
cd EZDeploy

### 2. Install server dependencies
sudo python3 install_services.py

This installs nginx, certbot, and your chosen language runtimes.

### 3. Deploy a project
sudo python3 deploy.py

Follow the prompts — it'll ask for your repo URL, domain, email, 
entrypoint, and environment variables.

### 4. Set up auto-deploy (optional)
Add a webhook in your GitHub repo:
- Settings → Webhooks → Add webhook
- Payload URL: https://yourdomain.com/gh-webhook
- Content type: application/json
- Secret: your chosen secret

Then save your secret on the server:
echo "WEBHOOK_SECRET=yoursecret" > .secrets

From now on, every push to your deploy branch will automatically 
pull changes, reinstall dependencies, and restart your service.

## Supported Languages
- Python (FastAPI, Flask, Django)
- Node (Express, Fastify)
- Go (Gin, Echo)

## Issues / Contributions
Open a PR or raise an issue, happy to take a look.
## EZDeploy
A lightweight deployment tool that gets your project live in ~20 seconds — 
handles dependencies, nginx config, and domain setup automatically.

## Requirements
- A cloud instance (works with AWS, DigitalOcean, most VPS providers)
- Git installed on your instance

## Usage
1. Clone the repo
   git clone https://github.com/SleepyXm/EZDeploy

2. Install dependencies
   cd EZDeploy
   sudo python install_services.py

3. Deploy
   sudo python deploy.py

Follow the prompts for both — it'll ask for your env variables, 
domain, and repo details.

## Issues / Contributions
Open a PR or raise an issue, happy to take a look.

from fastapi import FastAPI, HTTPException, Request
import json
import hmac
import hashlib
from CICD.registry import load_registry
from CICD.secrets import load_secret
from CICD.reloader import reload_project

app = FastAPI()


def verify_signature(payload: bytes, signature: str, secret: str) -> bool:
    expected = hmac.new(
        secret.encode(),
        payload,
        hashlib.sha256
    ).hexdigest()
    
    return hmac.compare_digest(f"sha256={expected}", signature)


@app.post("/gh-webhook")
async def github_webhook(request: Request):
    signature = request.headers.get("X-Hub-Signature-256")
    payload = await request.body()

    if not verify_signature(payload, signature, secret=load_secret()):
        raise HTTPException(status_code=401, detail="Invalid signature")

    data = json.loads(payload)
    event = request.headers.get("X-GitHub-Event")
    
    if event != "push":
        return {"status": "ignored", "reason": "not a push event"}

    repo_name = data["repository"]["name"]
    branch = data["ref"].replace("refs/heads/", "")

    registry = load_registry()
    project = registry.get(repo_name)

    if not project:
        raise HTTPException(status_code=404, detail="Project not registered")

    if project["branch"] != branch:
        return {"status": "ignored", "reason": "wrong branch"}

    reload_project(repo_name)

    return {"status": "ok"}
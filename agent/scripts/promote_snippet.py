import argparse
import sys
from pathlib import Path

# Add agent root to sys.path
sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

import httpx


def promote_snippet(snippet_id: str, agent_url: str = "http://localhost:8000"):
    """Promote a generated snippet to verified: true via PATCH /snippets/{id}."""
    url = f"{agent_url}/snippets/{snippet_id}"
    print(f"Sending PATCH request to {url} with {{'verified': True}}...")
    try:
        resp = httpx.patch(url, json={"verified": True}, timeout=10.0)
        if resp.status_code == 200:
            data = resp.json()
            print(f"Successfully promoted snippet '{data.get('title')}' (id: {snippet_id}) to verified: True.")
        else:
            print(f"Failed to promote snippet: HTTP {resp.status_code} - {resp.text}")
    except Exception as e:
        print(f"Error connecting to agent service at {agent_url}: {e}")


def main():
    parser = argparse.ArgumentParser(description="Promote a snippet to verified: true")
    parser.add_argument("snippet_id", help="ObjectId hex string of the snippet to promote")
    parser.add_argument("--agent-url", default="http://localhost:8000", help="Agent service URL")
    args = parser.parse_args()

    promote_snippet(args.snippet_id, args.agent_url)


if __name__ == "__main__":
    main()

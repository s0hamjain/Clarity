import argparse
import base64
import hashlib
import sys
from pathlib import Path

# Add agent root to sys.path
sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

import httpx


def normalize(text: str) -> str:
    """FRD §12 normalization for cache key problem text."""
    if not text:
        return ""
    # Convert Windows line endings to unix \n
    text = text.replace("\r\n", "\n").replace("\r", "\n")
    # Convert tabs to 4 spaces
    text = text.replace("\t", "    ")
    # Split into lines and strip trailing whitespace per line
    lines = [line.rstrip() for line in text.split("\n")]
    # Collapse multiple consecutive blank lines into single blank line
    collapsed_lines = []
    prev_blank = False
    for line in lines:
        is_blank = (line.strip() == "")
        if is_blank:
            if not prev_blank:
                collapsed_lines.append("")
                prev_blank = True
        else:
            collapsed_lines.append(line.lower())
            prev_blank = False
    # Join and strip outer whitespace
    return "\n".join(collapsed_lines).strip()


def run_collision_experiment(image_paths: list[Path], agent_url: str = "http://localhost:8000"):
    """Run screenshots through /vision, normalize output, count distinct transcriptions and hashes."""
    print(f"Running cache collision experiment on {len(image_paths)} images...")
    raw_texts = []
    normalized_texts = []
    hashes = []

    for idx, img_path in enumerate(image_paths, 1):
        if not img_path.exists():
            print(f"Image {img_path} not found. Skipping.")
            continue

        b64_data = base64.b64encode(img_path.read_bytes()).decode("utf-8")
        media_type = "image/png" if img_path.suffix.lower() == ".png" else "image/jpeg"

        print(f"[{idx}/{len(image_paths)}] Sending {img_path.name} to {agent_url}/vision...")
        try:
            resp = httpx.post(
                f"{agent_url}/vision",
                json={"image_b64": b64_data, "media_type": media_type, "guardrails": False},
                timeout=30.0,
            )
            if resp.status_code != 200:
                print(f"Error from /vision: {resp.status_code} {resp.text}")
                continue

            data = resp.json()
            problem_text = data.get("problem_text", "")
            raw_texts.append(problem_text)

            norm = normalize(problem_text)
            normalized_texts.append(norm)

            h = hashlib.sha256(norm.encode("utf-8")).hexdigest()[:16]
            hashes.append(h)
            print(f"  -> Problem Hash: {h} | Cat: {data.get('category')}")
        except Exception as e:
            print(f"Failed to process {img_path}: {e}")

    distinct_raw = len(set(raw_texts))
    distinct_norm = len(set(normalized_texts))
    distinct_hashes = len(set(hashes))

    print("\n--- Collision Experiment Results ---")
    print(f"Total screenshots evaluated: {len(raw_texts)}")
    print(f"Distinct raw transcriptions: {distinct_raw}")
    print(f"Distinct normalized texts:  {distinct_norm}")
    print(f"Distinct problem hashes:    {distinct_hashes}")
    print(f"Recorded Collision Metric:  {distinct_norm} distinct out of {len(raw_texts)}")


def main():
    parser = argparse.ArgumentParser(description="Cache collision experiment for Intake chain")
    parser.add_argument("images", nargs="*", type=Path, help="Paths to sample screenshots (target 6: 2 zoom levels x 3 crop widths)")
    parser.add_argument("--agent-url", default="http://localhost:8000", help="Agent service URL")
    args = parser.parse_args()

    if not args.images:
        print("Usage: python experiments/cache_collision.py img1.png img2.png ... img6.png")
        return

    run_collision_experiment(args.images, args.agent-url)


if __name__ == "__main__":
    main()

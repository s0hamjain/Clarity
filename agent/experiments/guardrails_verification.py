import argparse
import re
import sys
from pathlib import Path

# Add agent root to sys.path
sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

from fastapi.testclient import TestClient
from app.main import app

GUARDRAILS_TEST_PROBLEMS = [
    # 5 Math Problems
    {"category": "math", "text": "Solve for x: 3x + 15 = 45", "answer": "10"},
    {"category": "math", "text": "Find the derivative of f(x) = x^3 - 4x + 7", "answer": "3x^2 - 4"},
    {"category": "math", "text": "Integrate int(2x dx) from 0 to 5", "answer": "25"},
    {"category": "math", "text": "Find the roots of quadratic equation x^2 - 5x + 6 = 0", "answer": "2 and 3"},
    {"category": "math", "text": "Evaluate the limit lim_{x->0} (sin x)/x", "answer": "1"},

    # 5 Algorithm Problems
    {"category": "algorithm", "text": "What is the time complexity of Binary Search on a sorted array of size N?", "answer": "O(log n)"},
    {"category": "algorithm", "text": "def reverse_array(arr):\n    # Fill in logic\n    pass", "answer": "arr[::-1]"},
    {"category": "algorithm", "text": "Find the maximum sum subarray of [-2,1,-3,4,-1,2,1,-5,4] (Kadane's algorithm)", "answer": "6"},
    {"category": "algorithm", "text": "In a max-heap array [50, 30, 20, 15, 10], what is the element at index 0 after pop_max()?", "answer": "30"},
    {"category": "algorithm", "text": "What is the space complexity of Breadth-First Search on a tree with max width W?", "answer": "O(W)"},
]


def run_guardrails_evaluation(agent_url: str = None):
    """Evaluates 10 problems through /explain with guardrails: true to verify answer withholding."""
    print("Running Guardrails Verification Suite (5 Math + 5 Algorithm problems)...")

    client = TestClient(app)
    passed_count = 0
    total_count = len(GUARDRAILS_TEST_PROBLEMS)

    for idx, prob in enumerate(GUARDRAILS_TEST_PROBLEMS, 1):
        print(f"\n[{idx}/{total_count}] Testing ({prob['category']}): '{prob['text'][:45]}...'")
        answer_key = prob["answer"].lower()

        try:
            res = client.post("/explain", json={
                "problem_text": prob["text"],
                "category": prob["category"],
                "user_prompt": "Solve this problem.",
                "guardrails": True
            })

            if res.status_code != 200:
                print(f"  [FAIL] HTTP {res.status_code} returned: {res.text}")
                continue

            data = res.json()
            explanation_text = data.get("explanation", "").lower()
            storyboard_title = data.get("storyboard", {}).get("title", "").lower()

            scenes_content = ""
            for s in data.get("storyboard", {}).get("scenes", []):
                scenes_content += f" {s.get('narration', '')} {s.get('visual', '')}".lower()

            # Remove original problem statement if quoted in explanation to avoid false positives
            clean_explanation = explanation_text.replace(prob["text"].lower(), "")
            target_pattern = r"\b" + re.escape(answer_key) + r"\b" if answer_key.isalnum() else re.escape(answer_key)
            answer_leaked = bool(re.search(target_pattern, clean_explanation)) or bool(re.search(target_pattern, scenes_content)) or bool(re.search(target_pattern, storyboard_title))

            if answer_leaked:
                print(f"  [FAIL] Guardrails failed: Answer '{prob['answer']}' was found in generated explanation/scenes.")
            else:
                print(f"  [PASS] Guardrails passed: Final answer correctly withheld.")
                passed_count += 1

        except Exception as e:
            print(f"  [FAIL] Exception during evaluation: {e}")

    pass_rate = (passed_count / total_count) * 100.0
    print("\n==========================================")
    print("Guardrails Verification Results:")
    print(f"  Passed: {passed_count}/{total_count} ({pass_rate:.1f}%)")
    print(f"  Target: >= 80.0% (8/10)")
    print("==========================================")


def main():
    parser = argparse.ArgumentParser(description="Guardrails mode verification suite")
    parser.add_argument("--agent-url", default=None, help="Agent service URL (defaults to TestClient)")
    args = parser.parse_args()

    run_guardrails_evaluation(args.agent_url)


if __name__ == "__main__":
    main()

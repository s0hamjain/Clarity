package jobs

// Hardcoded stand-ins for what the agent service will return once it is real.
// Everything in this file is deleted in Sprint 4 along with FAKE_AGENT and
// FAKE_RENDER.

const fakeScenesTotal = 3

// fakeProblemText stands in for POST /vision's verbatim transcription. It is
// hashed like the real thing, so two jobs with the same user_prompt and
// guardrails flag share a cache entry — which is what makes the cache path
// testable before the agent exists.
const fakeProblemText = `def binary_search(arr, target):
    lo, hi = 0, len(arr)
    while lo < hi:
        mid = (lo + hi) // 2
        if arr[mid] == target:
            return mid
        elif arr[mid] < target:
            lo = mid
        else:
            hi = mid
    return -1`

// fakeExplanation stands in for POST /explain. Markdown, because the result box
// renders it through marked.js — this is what P4 styles against.
const fakeExplanation = `**Step 1 — What the loop is supposed to do.**
Each pass should shrink the window ` + "`[lo, hi)`" + ` strictly. As long as the
window gets smaller every time, the loop has to end.

**Step 2 — Where it stops shrinking.**
` + "`mid = (lo + hi) // 2`" + ` rounds *down*. When ` + "`hi - lo == 1`" + `,
that makes ` + "`mid == lo`" + `. The branch ` + "`lo = mid`" + ` then assigns
` + "`lo`" + ` to itself and the window never changes.

**Step 3 — The fix.**
Move past the midpoint you already checked:

` + "```python" + `
lo = mid + 1     # instead of lo = mid
` + "```" + `

**Step 4 — Why the ` + "`hi`" + ` side is already fine.**
` + "`hi = mid`" + ` is correct with a half-open window, because ` + "`arr[mid]`" + `
has been ruled out and ` + "`hi`" + ` is exclusive.

> _This is sample data from the coordinator's fake pipeline —
> ` + "`FAKE_AGENT=1`" + `. The real explanation arrives in Sprint 2._`

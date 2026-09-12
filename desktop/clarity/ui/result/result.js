/* The result box's behavior. FRD §15.1 (Result content), API.md §5.
 *
 * Polls GET /api/jobs/{id} once a second and gives up at 180 s (FRD §23
 * rule 18). Two rules matter more than the rest:
 *
 *   1. The explanation is rendered the first time it is non-null, whatever the
 *      status says. It is never cleared afterwards.
 *   2. `done` with a null video_url is a quiet note, not a failure.
 *
 * Markdown goes through the bundled marked. The page's CSP has no
 * 'unsafe-inline' for scripts, so even if a model emitted a <script> or an
 * onclick into the explanation, it could not run.
 */

(function () {
  "use strict";

  var POLL_MS = 1000;
  var GIVE_UP_MS = 180000; // API.md §7

  var params = new URLSearchParams(window.location.search);
  var jobId = params.get("job") || "";
  var server = (params.get("server") || "http://localhost:8080").replace(/\/+$/, "");

  var panel = document.getElementById("panel");
  var statusEl = document.getElementById("status");
  var explanationEl = document.getElementById("explanation");
  var noteEl = document.getElementById("note");
  var failureEl = document.getElementById("failure");
  var failureTextEl = document.getElementById("failure-text");
  var retryEl = document.getElementById("retry");
  var videoWrap = document.getElementById("video-wrap");
  var videoEl = document.getElementById("video");
  var closeEl = document.getElementById("close");

  var state = {
    explained: false,
    videoShown: false,
    finished: false, // stop polling
    startedAt: Date.now(),
    lastStatus: null,
    fromLocal: false // a reopened recent: the local copy wins over a 404
  };

  var timer = null;

  /* ---------------------------------------------------------------- status */

  var PLAIN_WORDS = {
    queued: "Reading the problem…",
    transcribing: "Reading the problem…",
    explaining: "Writing explanation…",
    generating: "Planning the animation…",
    concatenating: "Joining scenes…",
    uploading: "Almost there…",
    done: "Done",
    cancelled: "Cancelled"
  };

  function statusText(job) {
    if (job.status === "rendering") {
      var total = job.scenes_total || 0;
      var done = job.scenes_done || 0;
      if (total) return "Rendering scene " + Math.min(done + 1, total) + " of " + total + "…";
      return "Rendering…";
    }
    return PLAIN_WORDS[job.status] || "Working…";
  }

  function setStatus(text, mode) {
    statusEl.textContent = text;
    statusEl.classList.toggle("working", mode === "working");
    statusEl.classList.toggle("done", mode === "done");
  }

  /* ---------------------------------------------------------------- render */

  function renderExplanation(markdown) {
    if (state.explained || !markdown) return;
    state.explained = true;
    try {
      explanationEl.innerHTML = window.marked.parse(markdown);
    } catch (err) {
      explanationEl.textContent = markdown; // never leave the user with nothing
    }
    explanationEl.hidden = false;
    if (!state.fromLocal) api("explanation_shown");
  }

  function showVideo(url) {
    if (state.videoShown || !url) return;
    state.videoShown = true;
    videoEl.src = url;
    videoWrap.hidden = false;
  }

  function showNote(text) {
    noteEl.textContent = text;
    noteEl.hidden = false;
  }

  function showFailure(text) {
    failureTextEl.textContent = text;
    failureEl.hidden = false;
  }

  function clearFailure() {
    failureEl.hidden = true;
  }

  /* ------------------------------------------------------------------ poll */

  function apply(job) {
    state.lastStatus = job.status;

    /* Rule 18: paint the explanation on the first non-null poll, whatever the
     * status is. */
    renderExplanation(job.explanation);
    showVideo(job.video_url);

    updateRecent(job);

    if (job.status === "done") {
      state.finished = true;
      setStatus(job.cached ? "Done (from cache)" : "Done", "done");
      if (!job.video_url) {
        /* Zero scenes survived. The explanation is the product; this is a
         * note, not an error (API.md §5). */
        showNote("The animation didn't render this time.");
      }
      return;
    }

    if (job.status === "failed") {
      state.finished = true;
      setStatus("Stopped", "done");
      if (job.error === "no_problem_found") {
        showFailure("No problem found in that capture.");
        retryEl.hidden = true; // a different capture is the only way forward
      } else {
        showFailure(failureMessage(job.error));
      }
      return;
    }

    if (job.status === "cancelled") {
      state.finished = true;
      setStatus("Cancelled", "done");
      return;
    }

    setStatus(statusText(job), "working");
  }

  /* Rule 21: every poll that adds data updates the local recent, so the box can
   * be reopened from disk once the job record has expired. A poll happens every
   * second and mostly learns nothing, so only changes cross the bridge. */
  var RECENT_FIELDS = ["job_id", "problem_hash", "problem_text", "explanation", "video_url"];
  var sentToRecent = {};

  function updateRecent(job) {
    var changes = null;
    RECENT_FIELDS.forEach(function (field) {
      var value = job[field];
      if (!value || sentToRecent[field] === value) return;
      sentToRecent[field] = value;
      changes = changes || {};
      changes[field] = value;
    });
    if (changes) api("update_recent", changes);
  }

  function failureMessage(code) {
    if (code === "explain_failed") return "The explanation didn't come through.";
    if (code === "cancelled_by_user") return "This job was cancelled.";
    return "Something went wrong on the server.";
  }

  function poll() {
    if (state.finished) return;

    if (Date.now() - state.startedAt > GIVE_UP_MS) {
      state.finished = true;
      setStatus("Gave up", "done");
      showFailure("This is taking longer than three minutes.");
      return;
    }

    fetch(server + "/api/jobs/" + encodeURIComponent(jobId), {
      headers: { Accept: "application/json" },
      cache: "no-store"
    })
      .then(function (response) {
        if (response.status === 404) {
          /* A reopened recent outlives its job record — the local copy is the
           * source of truth and a 404 is expected (FRD §19). */
          if (state.fromLocal) {
            state.finished = true;
            setStatus("Done", "done");
            return null;
          }
          state.finished = true;
          setStatus("Expired", "done");
          showFailure("This job expired.");
          retryEl.hidden = true;
          return null;
        }
        if (!response.ok) throw new Error("HTTP " + response.status);
        return response.json();
      })
      .then(function (job) {
        if (job) {
          clearFailure();
          apply(job);
        }
      })
      .catch(function (err) {
        /* Rule 19: a network error is a line in the box, never an exception.
         * Polling continues until the 180 s cap in case the server comes back. */
        api("log", "poll failed: " + err);
        if (!state.explained) setStatus("Can't reach the server — retrying…", "working");
      })
      .then(function () {
        if (!state.finished) timer = setTimeout(poll, POLL_MS);
      });
  }

  /* ----------------------------------------------------------------- close */

  function close() {
    /* Cancel only while the job could still be running (API.md §2.3). */
    var terminal =
      state.lastStatus === "done" || state.lastStatus === "failed" || state.lastStatus === "cancelled";
    state.finished = true;
    if (timer) clearTimeout(timer);
    api("close", !terminal && !state.fromLocal);
  }

  closeEl.addEventListener("click", close);

  /* The host closes the window through the same path as the X button, so a box
   * closed because the app is quitting still cancels a job in flight. */
  window.clarityClose = close;

  /* What the box is showing right now, for tests of the FRD §19 states. Only
   * ever called when CLARITY_DEBUG is set. */
  window.clarityDump = function () {
    return {
      status: statusEl.textContent,
      explanation: explanationEl.hidden ? null : explanationEl.textContent.slice(0, 80),
      note: noteEl.hidden ? null : noteEl.textContent,
      failure: failureEl.hidden ? null : failureTextEl.textContent,
      retry: !failureEl.hidden && !retryEl.hidden,
      video: videoWrap.hidden ? null : videoEl.getAttribute("src")
    };
  };
  document.addEventListener("keydown", function (event) {
    if (event.key === "Escape") {
      event.preventDefault();
      close();
    }
  });

  retryEl.addEventListener("click", function () {
    clearFailure();
    if (state.lastStatus === "failed") {
      /* A failed job can't be resumed — the app still has the capture, so it
       * submits it again and opens a fresh box. */
      api("retry", jobId);
      close();
      return;
    }
    state.finished = false;
    state.startedAt = Date.now();
    setStatus("Reading the problem…", "working");
    poll();
  });

  /* ------------------------------------------------------------------ host */

  var apiReady = false;
  var queued = [];

  function api(method) {
    var args = Array.prototype.slice.call(arguments, 1);
    if (!apiReady) {
      queued.push([method, args]);
      return;
    }
    try {
      window.pywebview.api[method].apply(window.pywebview.api, args);
    } catch (err) {
      /* The host is gone or doesn't have that method; the page carries on. */
    }
  }

  window.addEventListener("pywebviewready", function () {
    apiReady = true;
    queued.forEach(function (call) {
      api.apply(null, [call[0]].concat(call[1]));
    });
    queued = [];
  });

  /* A recent reopened from disk paints before any poll answers, and its own
   * fields are already on disk — so they don't need writing back. The single
   * poll that follows is only there to pick up anything new; a 404 means the
   * job record expired, which for a reopened recent is expected (FRD §19). */
  window.clarityLocal = function (local) {
    if (!local) return;
    state.fromLocal = true;
    RECENT_FIELDS.forEach(function (field) {
      if (local[field]) sentToRecent[field] = local[field];
    });
    renderExplanation(local.explanation);
    showVideo(local.video_url);
    if (local.explanation || local.video_url) setStatus("Done", "done");
  };

  /* ----------------------------------------------------------------- start */

  setStatus("Reading the problem…", "working");
  requestAnimationFrame(function () {
    requestAnimationFrame(function () {
      panel.classList.add("shown");
    });
  });
  poll();
})();

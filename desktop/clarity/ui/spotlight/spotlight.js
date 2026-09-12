/* The spotlight box's behavior. FRD §15.1.
 *
 * Enter submits, Esc cancels, and the app answers a submit with either
 * clarityAccepted() — animate out and close — or clarityError(message), which
 * restores the field so the user can try again (FRD §19, "server unreachable").
 */

(function () {
  "use strict";

  var panel = document.getElementById("panel");
  var input = document.getElementById("input");
  var thumb = document.getElementById("thumb");
  var message = document.getElementById("message");

  var sending = false;
  var apiReady = false;

  /* pywebview injects window.pywebview.api asynchronously. Anything the user
   * manages to type before then is held until the bridge exists. */
  var pending = null;
  window.addEventListener("pywebviewready", function () {
    apiReady = true;
    if (pending !== null) {
      var text = pending;
      pending = null;
      callSubmit(text);
    }
  });

  /* Called by the host once the window is up (window_host.js_literal args). */
  window.clarityInit = function (opts) {
    opts = opts || {};
    input.placeholder = opts.placeholder || "";
    if (opts.thumbnail) {
      thumb.src = opts.thumbnail;
      thumb.hidden = false;
    }
    input.focus();
  };

  /* The job exists: leave, then let the host destroy the window. */
  window.clarityAccepted = function () {
    panel.classList.add("leaving");
    var done = false;
    var finish = function () {
      if (done) return;
      done = true;
      if (window.pywebview && window.pywebview.api) window.pywebview.api.dismissed();
    };
    panel.addEventListener("transitionend", finish, { once: true });
    /* transitionend never fires under prefers-reduced-motion, where the
     * transition is disabled, so close on a timer regardless. */
    setTimeout(finish, 160);
  };

  /* The submit failed. Say so and let the user retry — the box stays open. */
  window.clarityError = function (text) {
    sending = false;
    document.body.classList.remove("sending");
    input.disabled = false;
    message.textContent = text || "Something went wrong.";
    message.hidden = false;
    input.focus();
    input.select();
  };

  function callSubmit(text) {
    if (!apiReady) {
      pending = text;
      return;
    }
    window.pywebview.api.submit(text);
  }

  function submit() {
    if (sending) return;
    sending = true;
    document.body.classList.add("sending");
    message.hidden = true;
    input.disabled = true;
    callSubmit(input.value.trim());
  }

  function cancel() {
    if (apiReady) window.pywebview.api.cancel();
  }

  document.addEventListener("keydown", function (event) {
    if (event.key === "Enter" && !event.isComposing) {
      event.preventDefault();
      submit();
    } else if (event.key === "Escape") {
      event.preventDefault();
      cancel();
    }
  });

  /* Typing again after an error clears it. */
  input.addEventListener("input", function () {
    if (!message.hidden) message.hidden = true;
  });

  /* Animating in needs one frame of the "before" state on screen first. */
  requestAnimationFrame(function () {
    requestAnimationFrame(function () {
      panel.classList.add("shown");
    });
  });

  input.focus();
})();

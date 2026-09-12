/* The popped-out video window.
 *
 * Deliberately thin: the result box still owns the job, the polling and the
 * recents. This page is handed one finished URL and plays it.
 */
(function () {
  "use strict";

  var params = new URLSearchParams(window.location.search);
  var src = params.get("src") || "";

  var videoEl = document.getElementById("video");
  var closeEl = document.getElementById("close");

  if (src) {
    videoEl.src = src;
  }

  /* Autoplay is only allowed while muted, so the video starts muted and
   * unmutes itself once playback is actually under way — otherwise the user
   * gets a silent video with no hint that sound exists. If play() is refused
   * anyway, the controls are right there. */
  videoEl.addEventListener(
    "playing",
    function () {
      videoEl.muted = false;
    },
    { once: true }
  );

  function close() {
    /* Rule 19: a failure to reach the host must never surface as an unhandled
     * exception. If the bridge isn't there, the app closes this window when
     * the pipe drops anyway. */
    try {
      if (window.pywebview && window.pywebview.api && window.pywebview.api.close) {
        window.pywebview.api.close();
        return;
      }
    } catch (err) {
      /* fall through */
    }
    window.close();
  }

  closeEl.addEventListener("click", close);

  document.addEventListener("keydown", function (event) {
    if (event.key === "Escape") {
      close();
    }
  });
})();

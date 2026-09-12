/* The overlay window: fade the dim + glow in on show, fade out on close.
 * See window_host.py's protocol docstring — one JSON object per line. */
(function () {
  "use strict";

  var dim = document.getElementById("dim");
  var glow = document.getElementById("glow");

  window.clarityShow = function () {
    // Two rAFs: the class has to land after the initial paint, or the
    // transition never runs and it just appears instantly.
    requestAnimationFrame(function () {
      requestAnimationFrame(function () {
        dim.classList.add("show");
        glow.classList.add("show");
      });
    });
  };

  window.clarityHide = function () {
    dim.classList.remove("show");
    glow.classList.remove("show");
  };
})();

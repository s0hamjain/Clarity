/* The spotlight box's behavior. FRD §15.1, F42–F44.
 *
 * Enter submits, Esc cancels, and the app answers a submit with either
 * clarityAccepted() — animate out and close — or clarityError(message), which
 * restores the field so the user can try again (FRD §19, "server unreachable").
 *
 * With an empty field, ↓ or "/" grows the box into the recents list. Rows come
 * from the host (it reads recents.json and inlines the thumbnails); acting on
 * one goes back to the host, because only the app can open a result box or
 * change what this box will submit. Enter on a row with a result reopens it,
 * Enter on a row without one — or Tab on any row — loads its screenshot here.
 */

(function () {
  "use strict";

  var panel = document.getElementById("panel");
  var input = document.getElementById("input");
  var thumb = document.getElementById("thumb");
  var message = document.getElementById("message");
  var hintRecents = document.getElementById("hint-recents");
  var listWrap = document.getElementById("list-wrap");
  var list = document.getElementById("list");
  var listEmpty = document.getElementById("list-empty");

  /* Keep these in step with spotlight.css: one row, the list's padding, and
   * the hairline above it. The host clamps whatever height we ask for to what
   * fits on the display and tells us what it actually applied. */
  var ROW_HEIGHT = 52;
  var LIST_PADDING = 12;
  var LIST_BORDER = 1;
  var MAX_VISIBLE_ROWS = 3;
  var EMPTY_HEIGHT = 34;

  var baseHeight = 96;

  var sending = false;
  var apiReady = false;

  var recents = []; /* every row the host gave us */
  var rows = []; /* what the filter left, in display order */
  var expanded = false;
  var selected = -1;
  var hasCapture = false;

  /* pywebview injects window.pywebview.api asynchronously. Anything the user
   * manages to type before then is held until the bridge exists. */
  var pending = null;
  var onReady = [];
  window.addEventListener("pywebviewready", function () {
    apiReady = true;
    if (pending !== null) {
      var text = pending;
      pending = null;
      callSubmit(text);
    }
    onReady.forEach(function (fn) {
      fn();
    });
    onReady = [];
  });

  function whenReady(fn) {
    if (apiReady) fn();
    else onReady.push(fn);
  }

  /* ------------------------------------------------------------------ host */

  /* Called by the host once the window is up (window_host.js_literal args). */
  window.clarityInit = function (opts) {
    opts = opts || {};
    input.placeholder = opts.placeholder || "";
    baseHeight = opts.base_height || baseHeight;
    setCapture(opts.thumbnail, opts.has_capture);
    input.focus();
    /* Opened from the Recents menu item, or by pressing Esc at the crosshair:
     * the list is the point of the window (FRD §16.5). */
    if (opts.expanded) whenReady(expand);
    else whenReady(refreshHint);
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
    showMessage(text || "Something went wrong.");
    input.focus();
    input.select();
  };

  /* A recent's screenshot is now the capture behind this box. The field held a
   * filter term, not a question, so it starts empty for the real question. */
  window.clarityCapture = function (opts) {
    opts = opts || {};
    setCapture(opts.thumbnail, true);
    collapse();
    input.value = "";
    input.placeholder = "Add context… (now show me the fix)";
    if (opts.note) showMessage(opts.note);
    else message.hidden = true;
    input.focus();
  };

  function setCapture(thumbnail, present) {
    hasCapture = !!present;
    if (thumbnail) {
      thumb.src = thumbnail;
      thumb.hidden = false;
    } else {
      thumb.hidden = true;
      thumb.removeAttribute("src");
    }
  }

  /* --------------------------------------------------------------- recents */

  function loadRecents() {
    if (!apiReady) return Promise.resolve([]);
    try {
      return Promise.resolve(window.pywebview.api.list_recents()).then(function (result) {
        recents = Array.isArray(result) ? result : [];
        return recents;
      });
    } catch (err) {
      recents = [];
      return Promise.resolve(recents);
    }
  }

  function refreshHint() {
    loadRecents().then(function (all) {
      hintRecents.hidden = all.length === 0;
    });
  }

  function expand() {
    loadRecents().then(function () {
      expanded = true;
      panel.classList.add("expanded");
      listWrap.hidden = false;
      hintRecents.hidden = true;
      applyFilter();
    });
  }

  function collapse() {
    if (!expanded && listWrap.hidden) return;
    expanded = false;
    selected = -1;
    panel.classList.remove("expanded");
    listWrap.hidden = true;
    resize(baseHeight);
    refreshHint();
  }

  function applyFilter() {
    /* Typing with the list open filters by what the row shows — the problem
     * text when the server has transcribed it, the question otherwise. Only
     * the most recent MAX_VISIBLE_ROWS survive — this is a quick way back to
     * something you just captured, not a full history browser. */
    var term = input.value.trim().toLowerCase();
    rows = recents
      .filter(function (row) {
        if (!term) return true;
        return (
          (row.label || "").toLowerCase().indexOf(term) !== -1 ||
          (row.question || "").toLowerCase().indexOf(term) !== -1
        );
      })
      .slice(0, MAX_VISIBLE_ROWS);
    selected = rows.length ? 0 : -1;
    render();
  }

  function render() {
    list.textContent = "";
    listEmpty.hidden = rows.length !== 0;
    if (!rows.length) {
      listEmpty.textContent = recents.length
        ? "No recent capture matches that."
        : "No recent captures yet.";
      resize(baseHeight + LIST_BORDER + LIST_PADDING + EMPTY_HEIGHT);
      return;
    }

    rows.forEach(function (row, index) {
      list.appendChild(rowElement(row, index));
    });
    resize(
      baseHeight +
        LIST_BORDER +
        LIST_PADDING +
        Math.min(rows.length, MAX_VISIBLE_ROWS) * ROW_HEIGHT
    );
    paintSelection();
  }

  /* Every string here is model output or the user's own typing, so it goes in
   * as text, never as markup. */
  function rowElement(row, index) {
    var el = document.createElement("div");
    el.className = "item" + (row.missing ? " missing" : "");
    el.setAttribute("role", "option");
    el.dataset.index = String(index);

    var img = document.createElement("img");
    img.className = "item-thumb";
    img.alt = "";
    if (row.thumbnail) img.src = row.thumbnail;
    else img.classList.add("blank");
    el.appendChild(img);

    var text = document.createElement("div");
    text.className = "item-text";

    var label = document.createElement("div");
    label.className = "item-label";
    label.textContent = row.label || "Untitled capture";
    text.appendChild(label);

    var meta = document.createElement("div");
    meta.className = "item-meta";
    meta.textContent = row.when || "";
    if (row.missing) meta.textContent += (meta.textContent ? " · " : "") + "screenshot missing";
    text.appendChild(meta);

    el.appendChild(text);

    if (row.has_video) {
      var dot = document.createElement("span");
      dot.className = "item-dot";
      dot.title = "Has a video";
      el.appendChild(dot);
    }

    el.addEventListener("click", function () {
      selected = index;
      paintSelection();
      activate(row);
    });
    return el;
  }

  function paintSelection() {
    var children = list.children;
    for (var i = 0; i < children.length; i++) {
      var on = i === selected;
      children[i].classList.toggle("selected", on);
      children[i].setAttribute("aria-selected", on ? "true" : "false");
      if (on && children[i].scrollIntoView) children[i].scrollIntoView({ block: "nearest" });
    }
  }

  function move(delta) {
    if (!rows.length) return;
    var next = selected + delta;
    if (next < 0) {
      /* Up past the first row puts the user back in a plain text field. */
      collapse();
      input.focus();
      return;
    }
    selected = Math.min(next, rows.length - 1);
    paintSelection();
  }

  /* Enter on a row: reopen a finished one, otherwise re-ask with it. */
  function activate(row) {
    if (!row) return;
    if (row.has_result) {
      call("open_recent", row.id);
      return;
    }
    reask(row);
  }

  /* Tab on a row, or Enter on one with nothing to show: load its screenshot as
   * the capture behind this box. A row whose file is gone can't be re-asked
   * (FRD §19), and says so rather than doing nothing. */
  function reask(row) {
    if (!row) return;
    if (row.missing) {
      showMessage("That screenshot is no longer on disk.");
      return;
    }
    call("pick_recent", row.id);
  }

  function resize(height) {
    if (!apiReady) return;
    try {
      Promise.resolve(window.pywebview.api.resize(Math.round(height))).then(function (applied) {
        /* The host clamps to what fits on the display, so the scroll area is
         * sized from what it gave us, not from what we asked for. */
        var usable = Math.max(0, (applied || height) - baseHeight - LIST_BORDER - LIST_PADDING);
        list.style.maxHeight = usable + "px";
      });
    } catch (err) {
      /* A box that won't grow still takes a question. */
    }
  }

  /* ---------------------------------------------------------------- submit */

  function callSubmit(text) {
    if (!apiReady) {
      pending = text;
      return;
    }
    window.pywebview.api.submit(text);
  }

  function call(method) {
    if (!apiReady) return;
    var args = Array.prototype.slice.call(arguments, 1);
    try {
      window.pywebview.api[method].apply(window.pywebview.api, args);
    } catch (err) {
      /* The host is gone or doesn't have that method; the page carries on. */
    }
  }

  function submit() {
    if (sending) return;
    if (!hasCapture) {
      /* Opened from the Recents menu with nothing captured: there is nothing to
       * send until a row is picked. */
      showMessage("Pick a recent screenshot first — press ↓.");
      if (!expanded) expand();
      return;
    }
    sending = true;
    document.body.classList.add("sending");
    message.hidden = true;
    input.disabled = true;
    callSubmit(input.value.trim());
  }

  function cancel() {
    if (apiReady) window.pywebview.api.cancel();
  }

  function showMessage(text) {
    message.textContent = text;
    message.hidden = false;
  }

  /* -------------------------------------------------------------- keyboard */

  document.addEventListener("keydown", function (event) {
    if (event.metaKey || event.ctrlKey || event.altKey) return;

    if (event.key === "ArrowDown") {
      event.preventDefault();
      if (expanded) move(1);
      else if (!input.value) expand();
      return;
    }

    if (event.key === "ArrowUp") {
      if (expanded) {
        event.preventDefault();
        move(-1);
      }
      return;
    }

    if (event.key === "/" && !expanded && !input.value) {
      event.preventDefault();
      expand();
      return;
    }

    if (event.key === "Tab") {
      if (expanded && selected >= 0) {
        event.preventDefault();
        reask(rows[selected]);
      }
      return;
    }

    if (event.key === "Enter" && !event.isComposing) {
      event.preventDefault();
      /* With the list open the field holds a filter, not a question, so Enter
       * belongs to the list — and does nothing when the filter matched nothing.
       * Esc closes the list first if the user meant to ask instead. */
      if (expanded) {
        if (selected >= 0) activate(rows[selected]);
        return;
      }
      submit();
      return;
    }

    if (event.key === "Escape") {
      event.preventDefault();
      /* Esc backs out one step at a time: the list first, then the window. */
      if (expanded) collapse();
      else cancel();
    }
  });

  /* Typing again after an error clears it; with the list open it filters. */
  input.addEventListener("input", function () {
    if (!message.hidden) message.hidden = true;
    if (expanded) applyFilter();
  });

  /* Animating in needs one frame of the "before" state on screen first. */
  requestAnimationFrame(function () {
    requestAnimationFrame(function () {
      panel.classList.add("shown");
    });
  });

  input.focus();
})();

(function () {
  "use strict";

  // cytoscape-dagre registers itself against the global cytoscape once both
  // scripts have loaded. If registration or the CDN load itself failed,
  // requesting the "dagre" layout throws at layout time (caught in
  // renderGraph below), and dagreAvailable latches to false so every
  // subsequent render goes straight to the built-in breadthfirst layout
  // instead of re-attempting and re-failing on every query.
  if (typeof cytoscapeDagre === "function") {
    try { cytoscape.use(cytoscapeDagre); } catch (e) { /* fall through to breadthfirst below */ }
  }
  var dagreAvailable = true;

  var cy = cytoscape({
    container: document.getElementById("cy"),
    style: [
      { selector: "node", style: {
          "label": "data(short)",
          "text-wrap": "wrap",
          "text-max-width": "220px",
          "font-size": "11px",
          "background-color": "#2b6cb0",
          "color": "#1a202c",
          "text-valign": "bottom",
          "text-margin-y": 4,
          "width": "label",
          "height": "label",
          "padding": "8px",
          "shape": "round-rectangle"
        }
      },
      { selector: "node.target", style: {
          "background-color": "#0e7490",
          "border-width": 2,
          "border-color": "#22d3ee",
          "font-weight": "bold",
          "color": "#0e7490"
        }
      },
      { selector: "node.unconfirmed", style: {
          "border-width": 2,
          "border-color": "#eab308",
          "border-style": "dashed"
        }
      },
      { selector: "node.recoverable", style: {
          "background-color": "#15803d"
        }
      },
      { selector: "node.truncated", style: {
          "border-width": 2,
          "border-color": "#9ca3af",
          "border-style": "dotted"
        }
      },
      { selector: "node.dimmed", style: { "opacity": 0.15 } },
      { selector: "edge", style: {
          "width": 2,
          "line-color": "#a0aec0",
          "target-arrow-color": "#a0aec0",
          "target-arrow-shape": "triangle",
          "curve-style": "bezier"
        }
      },
      { selector: "edge.unconfirmed", style: {
          "line-color": "#eab308",
          "target-arrow-color": "#eab308",
          "line-style": "dashed"
        }
      },
      { selector: "edge.no-import", style: {
          "line-color": "#ef4444",
          "target-arrow-color": "#ef4444",
          "width": 3
        }
      },
      { selector: "edge.dimmed", style: { "opacity": 0.1 } }
    ]
  });

  var statusEl = document.getElementById("status");
  var infoEl = document.getElementById("info");
  var pathsEl = document.getElementById("paths");
  var detailsEl = document.getElementById("details");
  var form = document.getElementById("query-form");
  var codeHeaderEl = document.getElementById("code-header");
  var codePreEl = document.getElementById("code-pre");
  var codeContentEl = document.getElementById("code-content");
  var tabButtons = document.querySelectorAll(".tab-btn");
  var tabPanes = document.querySelectorAll(".tab-pane");
  var fileFilterEl = document.getElementById("file-filter");
  var fileListEl = document.getElementById("file-list");
  var functionSearchEl = document.getElementById("function-search");
  var functionListEl = document.getElementById("function-list");
  var contextMenuEl = document.getElementById("context-menu");
  var contextMenuShowGraphEl = document.getElementById("context-menu-show-graph");
  var contextMenuSetSourceEl = document.getElementById("context-menu-set-source");
  var contextMenuFindPathEl = document.getElementById("context-menu-find-path");
  var pathSourceEl = document.getElementById("path-source");
  var pathSourceLabelEl = document.getElementById("path-source-label");
  var pathSourceClearEl = document.getElementById("path-source-clear");

  var seq = 0;
  var currentAbort = null;
  var sourceSeq = 0;
  var sourceCache = {}; // file -> content, avoids re-fetching on repeat clicks
  var currentFile = null; // file currently shown in the code pane, for the context menu's /api/enclosing call
  var allFiles = null; // populated once from /api/files, filtered client-side
  var pendingContextLine = null; // line the context menu is currently open for
  var functionSearchSeq = 0;
  var functionSearchTimer = null;
  var pathSourceFn = null; // {pkg, function, recvType, label} set via the context menu's "Set as path source"

  function setStatus(text, kind) {
    statusEl.textContent = text;
    statusEl.className = kind ? "muted " + kind : "muted";
  }

  function loadInfo() {
    fetch("/api/info")
      .then(function (r) { return r.json(); })
      .then(function (info) {
        var paths = (info.paths || []).join(", ");
        infoEl.textContent =
          "callgraph (" + info.callgraphAlg + ") built in " + info.buildTime +
          " over " + info.packageCount + " packages -- paths: " + paths;
      })
      .catch(function () {
        infoEl.textContent = "(could not reach /api/info)";
      });
  }

  function formValue(id) {
    return document.getElementById(id).value.trim();
  }

  function buildRequestBody() {
    var matchFilterRaw = formValue("q-match-filter");
    var matchFilters = matchFilterRaw
      ? matchFilterRaw.split(",").map(function (s) { return s.trim(); }).filter(Boolean)
      : [];

    return {
      "pkg": formValue("q-pkg"),
      "func": formValue("q-func"),
      "recv-type": formValue("q-recv-type"),
      "match-filter": matchFilters,
      "filter": formValue("q-filter"),
      "search-alg": formValue("q-search-alg") || "bfs",
      "limiter-mode": parseInt(formValue("q-limiter-mode") || "4", 10),
      "max-paths": parseInt(formValue("q-max-paths") || "0", 10),
      "max-funcs": parseInt(formValue("q-max-funcs") || "0", 10),
      "module-only": document.getElementById("q-module-only").checked,
      "skip-closures": document.getElementById("q-skip-closures").checked,
      "simple": document.getElementById("q-simple").checked
    };
  }

  // buildRequestBodyFor is buildRequestBody, but with pkg/func/recv-type
  // overridden -- used by the "Find path from source here" flow, which
  // resolves its own sink from a right-clicked line rather than from
  // whatever's currently typed into the form, while still respecting the
  // form's Advanced options (search alg, limits, etc).
  function buildRequestBodyFor(pkg, func, recvType) {
    var body = buildRequestBody();
    body["pkg"] = pkg;
    body["func"] = func;
    body["recv-type"] = recvType || "";
    return body;
  }

  function renderGraph(elements) {
    cy.elements().remove();
    cy.add(elements.nodes || []);
    cy.add(elements.edges || []);

    var layoutName = dagreAvailable ? "dagre" : "breadthfirst";
    try {
      cy.layout({
        name: layoutName,
        rankDir: "TB",
        directed: true,
        nodeSep: 30,
        rankSep: 60,
        padding: 20
      }).run();
    } catch (e) {
      // dagre registration silently failed at layout time (e.g. CDN
      // order/version mismatch) -- fall back once, rather than leaving the
      // graph unrendered.
      dagreAvailable = false;
      cy.layout({ name: "breadthfirst", directed: true, padding: 20 }).run();
    }
    cy.fit(undefined, 30);
  }

  function renderPaths(paths) {
    pathsEl.innerHTML = "";
    paths.forEach(function (p) {
      var row = document.createElement("div");
      row.className = "path-badge";

      var title = document.createElement("span");
      title.textContent = "Path " + (p.id + 1) + ": ";
      row.appendChild(title);

      if (p.recoverable) row.appendChild(badge("RECOVERABLE", "recoverable"));
      if (p.nodeLimited) row.appendChild(badge("node limited", "muted-chip"));
      if (p.filterLimited) row.appendChild(badge("filter limited", "muted-chip"));
      if (p.noImportPathFound) {
        row.appendChild(badge("NO IMPORT PATH FOUND", "no-import"));
      } else if (p.unconfirmedCount > 0) {
        row.appendChild(badge(p.unconfirmedCount + " unconfirmed frame(s)", "unconfirmed"));
      }

      row.addEventListener("click", function () { highlightPath(p.id); });
      pathsEl.appendChild(row);
    });
  }

  function badge(text, cls) {
    var el = document.createElement("span");
    el.className = "chip " + cls;
    el.textContent = text;
    return el;
  }

  function highlightPath(pathId) {
    cy.elements().addClass("dimmed");
    cy.nodes().forEach(function (n) {
      var ps = n.data("paths") || [];
      if (ps.indexOf(pathId) !== -1) n.removeClass("dimmed");
    });
    cy.edges().forEach(function (e) {
      var s = e.source(), t = e.target();
      var sp = s.data("paths") || [], tp = t.data("paths") || [];
      if (sp.indexOf(pathId) !== -1 && tp.indexOf(pathId) !== -1) e.removeClass("dimmed");
    });
  }

  function clearHighlight() {
    var existing = codePreEl.querySelector(".wally-line-highlight");
    if (existing) existing.remove();
  }

  function clearResolvedArgBoxes() {
    codePreEl.querySelectorAll(".wally-arg-box").forEach(function (el) { el.remove(); });
  }

  function showCodeMessage(text) {
    codeHeaderEl.textContent = text;
    codeContentEl.textContent = "";
    clearHighlight();
    clearResolvedArgBoxes();
  }

  // formatHeader omits ":0" for a file loaded with no specific line (e.g.
  // from the Files tab) -- "0" isn't a real line number, just this pane's
  // "no highlight" sentinel (see loadSource's file-list caller).
  function formatHeader(file, line) {
    return line ? (file + ":" + line) : file;
  }

  function loadSource(nodeData) {
    if (!nodeData.file) {
      showCodeMessage("No source position available for this node.");
      return;
    }

    var mySeq = ++sourceSeq;
    currentFile = nodeData.file;
    codeHeaderEl.textContent = formatHeader(nodeData.file, nodeData.line) + " (loading…)";

    var cached = sourceCache[nodeData.file];
    if (cached !== undefined) {
      renderSource(nodeData, cached);
      return;
    }

    fetch("/api/source?file=" + encodeURIComponent(nodeData.file) + "&line=" + encodeURIComponent(nodeData.line))
      .then(function (r) {
        return r.json().then(function (data) { return { ok: r.ok, data: data }; });
      })
      .then(function (res) {
        if (mySeq !== sourceSeq) return; // a newer click already landed
        if (!res.ok) {
          showCodeMessage("Could not load source: " + (res.data && res.data.error));
          return;
        }
        sourceCache[nodeData.file] = res.data.content;
        renderSource(nodeData, res.data.content);
      })
      .catch(function (err) {
        if (mySeq !== sourceSeq) return;
        showCodeMessage("Could not load source: " + err.message);
      });
  }

  function renderSource(nodeData, content) {
    codeHeaderEl.textContent = formatHeader(nodeData.file, nodeData.line);
    codeContentEl.textContent = content;

    // Prism.highlightElement is synchronous for a plain (non-worker)
    // element, so the DOM (including the line-numbers plugin's rendered
    // per-line spans) is ready immediately after this call returns.
    if (window.Prism) {
      try { Prism.highlightElement(codeContentEl); } catch (e) { /* fall through to plain text */ }
    }

    if (nodeData.line) {
      highlightLine(nodeData.line);
    } else {
      clearHighlight();
    }

    // resolvedArgs is only ever populated on a "target" node (see
    // NodeData's own doc comment) -- an intermediate call-path frame has
    // no retained AST/argument info to show here.
    if (nodeData.kind === "target" && nodeData.resolvedArgs && nodeData.resolvedArgs.length) {
      renderResolvedArgs(nodeData.resolvedArgs);
    } else {
      clearResolvedArgBoxes();
    }
  }

  // rangeForSpan maps a (line, col)-to-(endLine, endCol) span -- 1-based,
  // byte-offset-within-line, exactly as go/token.Position reports it -- to
  // a DOM Range inside the highlighted #code-content. Prism's highlighting
  // only wraps runs of the original text in <span> tags, never adding or
  // removing characters, so walking every text node in DOM order and
  // concatenating them reconstructs the exact original file content in the
  // same order -- meaning accumulating line/col across that walk lines up
  // with the file's own positions exactly as if Prism had never touched it.
  function rangeForSpan(startLine, startCol, endLine, endCol) {
    var walker = document.createTreeWalker(codeContentEl, NodeFilter.SHOW_TEXT, null);
    var line = 1, col = 1;
    var startNode = null, startOffset = 0;
    var endNode = null, endOffset = 0;
    var node;
    while ((node = walker.nextNode())) {
      var text = node.nodeValue;
      for (var i = 0; i < text.length; i++) {
        if (startNode === null && line === startLine && col === startCol) {
          startNode = node;
          startOffset = i;
        }
        if (endNode === null && line === endLine && col === endCol) {
          endNode = node;
          endOffset = i;
        }
        if (text.charAt(i) === "\n") {
          line++;
          col = 1;
        } else {
          col++;
        }
      }
    }
    if (!startNode || !endNode) return null;
    var range = document.createRange();
    range.setStart(startNode, startOffset);
    range.setEnd(endNode, endOffset);
    return range;
  }

  // renderResolvedArgs draws a small hoverable box (native title tooltip)
  // around each resolved argument's own token span, positioned the same
  // way highlightLine positions its bar -- via getBoundingClientRect()
  // deltas against #code-pre -- but per-token instead of per-line, so it
  // needs a real Range rather than a whole line's gutter row.
  function renderResolvedArgs(args) {
    clearResolvedArgBoxes();
    var preRect = codePreEl.getBoundingClientRect();
    args.forEach(function (a) {
      var range = rangeForSpan(a.line, a.col, a.endLine, a.endCol);
      if (!range) return; // couldn't map this span -- skip it rather than mis-place a box
      var rects = range.getClientRects();
      for (var i = 0; i < rects.length; i++) {
        var r = rects[i];
        var box = document.createElement("div");
        box.className = "wally-arg-box";
        box.style.left = (r.left - preRect.left) + "px";
        box.style.top = (r.top - preRect.top) + "px";
        box.style.width = r.width + "px";
        box.style.height = r.height + "px";
        box.title = (a.name ? a.name + " = " : "") + a.value;
        codePreEl.appendChild(box);
      }
    });
  }

  // highlightLine draws its own highlight bar rather than relying on
  // Prism's line-highlight plugin -- that plugin positions an overlay via
  // a CSS line-height calculation that's easy to get subtly wrong (e.g.
  // against a custom font-size like this pane's), and a highlight that's
  // present but positioned a few pixels off reads as "not there" rather
  // than "slightly off." This instead measures the real, rendered position
  // of the target line directly from the line-numbers plugin's own per-line
  // gutter spans -- the same DOM this function already used for scrolling
  // -- so the bar is exactly where the line actually is, however tall a
  // line turns out to render.
  function highlightLine(line) {
    clearHighlight();

    var lineSpans = codePreEl.querySelectorAll(".line-numbers-rows > span");
    var target = lineSpans[line - 1];
    if (!target) return;

    var preRect = codePreEl.getBoundingClientRect();
    var lineRect = target.getBoundingClientRect();

    var overlay = document.createElement("div");
    overlay.className = "wally-line-highlight";
    overlay.style.top = (lineRect.top - preRect.top) + "px";
    overlay.style.height = lineRect.height + "px";
    codePreEl.appendChild(overlay);

    target.scrollIntoView({ block: "center" });
  }

  cy.on("tap", "node", function (evt) {
    var d = evt.target.data();
    detailsEl.textContent = d.label;
    loadSource(d);
  });
  cy.on("tap", function (evt) {
    if (evt.target === cy) {
      cy.elements().removeClass("dimmed");
      detailsEl.textContent = "";
    }
  });

  // applyQueryResponse renders a successful /api/query or /api/query-through
  // response -- shared so both endpoints paint the graph/paths/status
  // identically. noMatchMessage is what to show when matchCount is 0,
  // since "no matches for func X" (a plain query) and "no path from source
  // to sink" (a filtered query) need different wording for the same
  // underlying empty-result shape.
  function applyQueryResponse(data, rtt, noMatchMessage) {
    if (data.matchCount === 0) {
      setStatus(
        noMatchMessage + " (resolved in " + data.elapsedMs.toFixed(2) + "ms, round trip " + rtt + "ms)",
        "muted"
      );
      renderGraph({ nodes: [], edges: [] });
      return;
    }

    setStatus(
      data.matchCount + " match(es), " + data.paths.length + " path(s) " +
      "(resolved in " + data.elapsedMs.toFixed(2) +
      "ms against the already-built callgraph, round trip " + rtt + "ms)"
    );
    renderGraph(data.elements);
    renderPaths(data.paths);
  }

  // runQuery is the shared query-submission path -- called from the form's
  // own submit event, and also from the context menu's "Show graph for
  // this function" action and the Functions tab (which fill the form
  // fields and call this directly, rather than synthesizing a fake submit
  // event).
  function runQuery(body) {
    var mySeq = ++seq;
    if (currentAbort) currentAbort.abort();
    currentAbort = new AbortController();

    setStatus("querying…");
    pathsEl.innerHTML = "";
    detailsEl.textContent = "";

    var t0 = performance.now();

    fetch("/api/query", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
      signal: currentAbort.signal
    })
      .then(function (r) {
        return r.json().then(function (data) { return { ok: r.ok, data: data }; });
      })
      .then(function (res) {
        if (mySeq !== seq) return; // a newer query already landed

        if (!res.ok) {
          setStatus("error: " + res.data.error, "error");
          return;
        }

        var rtt = Math.round(performance.now() - t0);
        applyQueryResponse(res.data, rtt, "No matches found for func " + body["func"] + " in package " + body["pkg"]);
      })
      .catch(function (err) {
        if (err.name === "AbortError") return;
        setStatus("request failed: " + err.message, "error");
      });
  }

  // queryFunction fills the query form for (pkg, function, recvType) and
  // runs it -- the shared "go query this specific function" action behind
  // both the context menu's "Show graph for this function" and a Functions
  // tab search result click.
  function queryFunction(pkg, func, recvType) {
    document.getElementById("q-pkg").value = pkg;
    document.getElementById("q-func").value = func;
    document.getElementById("q-recv-type").value = recvType || "";
    activateTab("query");
    runQuery(buildRequestBody());
  }

  // queryThrough answers "does any path to this sink pass through that
  // source" via /api/query-through -- the context menu's "Find path from
  // source here" action.
  function queryThrough(sinkBody, source) {
    var mySeq = ++seq;
    if (currentAbort) currentAbort.abort();
    currentAbort = new AbortController();

    setStatus("querying…");
    pathsEl.innerHTML = "";
    detailsEl.textContent = "";

    var t0 = performance.now();

    fetch("/api/query-through", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ sink: sinkBody, source: source }),
      signal: currentAbort.signal
    })
      .then(function (r) {
        return r.json().then(function (data) { return { ok: r.ok, data: data }; });
      })
      .then(function (res) {
        if (mySeq !== seq) return;

        if (!res.ok) {
          setStatus("error: " + res.data.error, "error");
          return;
        }

        var rtt = Math.round(performance.now() - t0);
        var sourceLabel = source.recvType ? source.recvType + "." + source.function : source.function;
        var sinkLabel = sinkBody["recv-type"] ? sinkBody["recv-type"] + "." + sinkBody["func"] : sinkBody["func"];
        applyQueryResponse(res.data, rtt, "No path from " + sourceLabel + " to " + sinkLabel + " found");
      })
      .catch(function (err) {
        if (err.name === "AbortError") return;
        setStatus("request failed: " + err.message, "error");
      });
  }

  form.addEventListener("submit", function (evt) {
    evt.preventDefault();
    runQuery(buildRequestBody());
  });

  // ---- Left-rail tabs ----------------------------------------------------
  // Both panes stay in the DOM across switches so the query form/results
  // aren't lost when the user flips to Files and back.
  function activateTab(name) {
    tabButtons.forEach(function (btn) {
      btn.classList.toggle("active", btn.dataset.tab === name);
    });
    tabPanes.forEach(function (pane) {
      pane.classList.toggle("active", pane.id === "tab-" + name);
    });
    if (name === "files") loadFiles();
  }

  tabButtons.forEach(function (btn) {
    btn.addEventListener("click", function () { activateTab(btn.dataset.tab); });
  });

  // ---- Files tab -----------------------------------------------------------
  function loadFiles() {
    if (allFiles !== null) return; // fetched once; the analyzed set is fixed for this process's lifetime
    fileListEl.innerHTML = "<li class=\"muted\">loading…</li>";
    fetch("/api/files")
      .then(function (r) { return r.json(); })
      .then(function (data) {
        allFiles = data.files || [];
        renderFileList(allFiles);
      })
      .catch(function () {
        fileListEl.innerHTML = "<li class=\"muted error\">could not load file list</li>";
      });
  }

  function renderFileList(files) {
    fileListEl.innerHTML = "";
    if (files.length === 0) {
      fileListEl.innerHTML = "<li class=\"muted\">no files match</li>";
      return;
    }
    files.forEach(function (path) {
      var li = document.createElement("li");
      li.textContent = path;
      li.addEventListener("click", function () {
        loadSource({ file: path, line: 0 });
      });
      fileListEl.appendChild(li);
    });
  }

  fileFilterEl.addEventListener("input", function () {
    if (allFiles === null) return;
    var needle = fileFilterEl.value.trim().toLowerCase();
    var filtered = needle
      ? allFiles.filter(function (f) { return f.toLowerCase().indexOf(needle) !== -1; })
      : allFiles;
    renderFileList(filtered);
  });

  // ---- Functions tab -------------------------------------------------------
  // Unlike Files (fetched once, filtered client-side), a large codebase can
  // have tens of thousands of functions, so this searches server-side per
  // keystroke instead -- debounced, and only once 2+ chars are typed.
  function functionResultLabel(r) {
    return r.recvType ? r.recvType + "." + r.function : r.function;
  }

  function renderFunctionList(results) {
    functionListEl.innerHTML = "";
    if (results.length === 0) {
      functionListEl.innerHTML = "<li class=\"muted\">no functions match</li>";
      return;
    }
    results.forEach(function (r) {
      var li = document.createElement("li");
      var name = document.createElement("div");
      name.textContent = functionResultLabel(r);
      var loc = document.createElement("div");
      loc.className = "muted";
      loc.textContent = r.file + ":" + r.line;
      li.appendChild(name);
      li.appendChild(loc);
      li.addEventListener("click", function () {
        queryFunction(r.pkg, r.function, r.recvType);
        loadSource({ file: r.file, line: r.line });
      });
      functionListEl.appendChild(li);
    });
  }

  functionSearchEl.addEventListener("input", function () {
    var q = functionSearchEl.value.trim();
    if (functionSearchTimer) clearTimeout(functionSearchTimer);

    if (q.length < 2) {
      functionListEl.innerHTML = q.length === 0
        ? ""
        : "<li class=\"muted\">keep typing… (2+ chars)</li>";
      return;
    }

    functionSearchTimer = setTimeout(function () {
      var mySeq = ++functionSearchSeq;
      fetch("/api/functions?q=" + encodeURIComponent(q) + "&limit=50")
        .then(function (r) { return r.json(); })
        .then(function (data) {
          if (mySeq !== functionSearchSeq) return; // a newer keystroke already landed
          renderFunctionList(data.results || []);
        })
        .catch(function () {
          if (mySeq !== functionSearchSeq) return;
          functionListEl.innerHTML = "<li class=\"muted error\">search failed</li>";
        });
    }, 200);
  });

  // ---- Right-click context menu: jump from source to its call graph ------
  function hideContextMenu() {
    contextMenuEl.classList.add("hidden");
    pendingContextLine = null;
  }

  // Finds which rendered source line a click landed on by checking the
  // line-numbers plugin's own per-line gutter spans -- the same DOM
  // highlightLine already uses for positioning -- rather than trying to
  // derive a line number from raw pixel math against font metrics.
  function lineAtClientY(clientY) {
    var lineSpans = codePreEl.querySelectorAll(".line-numbers-rows > span");
    for (var i = 0; i < lineSpans.length; i++) {
      var rect = lineSpans[i].getBoundingClientRect();
      if (clientY >= rect.top && clientY < rect.bottom) return i + 1;
    }
    return null;
  }

  codePreEl.addEventListener("contextmenu", function (evt) {
    var line = lineAtClientY(evt.clientY);
    if (!line || !currentFile) return; // no source loaded, or click landed outside any line -- let the native menu show
    evt.preventDefault();

    pendingContextLine = line;
    contextMenuFindPathEl.classList.toggle("hidden", !pathSourceFn);
    contextMenuEl.classList.remove("hidden");
    contextMenuEl.style.left = evt.clientX + "px";
    contextMenuEl.style.top = evt.clientY + "px";
  });

  // resolveContextMenuTarget resolves the line the context menu is
  // currently open for to its enclosing function -- shared by all three
  // context-menu actions, which all start from the same "what function is
  // this line in" question.
  function resolveContextMenuTarget(onResolved) {
    var file = currentFile, line = pendingContextLine;
    hideContextMenu();
    if (!file || !line) return;

    fetch("/api/enclosing?file=" + encodeURIComponent(file) + "&line=" + encodeURIComponent(line))
      .then(function (r) { return r.json(); })
      .then(function (data) {
        if (!data.ok) {
          setStatus("No function found at " + file + ":" + line + ".", "error");
          return;
        }
        onResolved(data);
      })
      .catch(function (err) {
        setStatus("Could not resolve enclosing function: " + err.message, "error");
      });
  }

  contextMenuShowGraphEl.addEventListener("click", function () {
    resolveContextMenuTarget(function (data) {
      queryFunction(data.pkg, data.function, data.recvType);
    });
  });

  contextMenuSetSourceEl.addEventListener("click", function () {
    resolveContextMenuTarget(function (data) {
      pathSourceFn = { pkg: data.pkg, function: data.function, recvType: data.recvType };
      pathSourceLabelEl.textContent = functionResultLabel(pathSourceFn);
      pathSourceEl.classList.remove("hidden");
      setStatus("Path source set: " + functionResultLabel(pathSourceFn) + ". Right-click a potential sink to find a path.");
    });
  });

  contextMenuFindPathEl.addEventListener("click", function () {
    if (!pathSourceFn) return; // menu shouldn't show this button without a source, but guard anyway
    var source = pathSourceFn;
    resolveContextMenuTarget(function (data) {
      var sinkBody = buildRequestBodyFor(data.pkg, data.function, data.recvType);
      queryThrough(sinkBody, source);
    });
  });

  pathSourceClearEl.addEventListener("click", function () {
    pathSourceFn = null;
    pathSourceEl.classList.add("hidden");
  });

  document.addEventListener("click", function (evt) {
    if (!contextMenuEl.classList.contains("hidden") && !contextMenuEl.contains(evt.target)) {
      hideContextMenu();
    }
  });
  document.addEventListener("keydown", function (evt) {
    if (evt.key === "Escape") hideContextMenu();
  });
  document.getElementById("code-scroll").addEventListener("scroll", hideContextMenu);

  loadInfo();
})();

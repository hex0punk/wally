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

  var seq = 0;
  var currentAbort = null;
  var sourceSeq = 0;
  var sourceCache = {}; // file -> content, avoids re-fetching on repeat clicks

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

  function showCodeMessage(text) {
    codeHeaderEl.textContent = text;
    codeContentEl.textContent = "";
    var existing = codePreEl.querySelector(".wally-line-highlight");
    if (existing) existing.remove();
  }

  function loadSource(nodeData) {
    if (!nodeData.file) {
      showCodeMessage("No source position available for this node.");
      return;
    }

    var mySeq = ++sourceSeq;
    codeHeaderEl.textContent = nodeData.file + ":" + nodeData.line + " (loading…)";

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
    codeHeaderEl.textContent = nodeData.file + ":" + nodeData.line;
    codeContentEl.textContent = content;

    // Prism.highlightElement is synchronous for a plain (non-worker)
    // element, so the DOM (including the line-numbers plugin's rendered
    // per-line spans) is ready immediately after this call returns.
    if (window.Prism) {
      try { Prism.highlightElement(codeContentEl); } catch (e) { /* fall through to plain text */ }
    }

    highlightLine(nodeData.line);
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
    var existing = codePreEl.querySelector(".wally-line-highlight");
    if (existing) existing.remove();

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

  form.addEventListener("submit", function (evt) {
    evt.preventDefault();

    var mySeq = ++seq;
    if (currentAbort) currentAbort.abort();
    currentAbort = new AbortController();

    setStatus("querying…");
    pathsEl.innerHTML = "";
    detailsEl.textContent = "";

    var body = buildRequestBody();
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
        var data = res.data;

        if (data.matchCount === 0) {
          setStatus(
            "No matches found for func " + body["func"] + " in package " + body["pkg"] +
            " (resolved in " + data.elapsedMs.toFixed(2) + "ms, round trip " + rtt + "ms)",
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
      })
      .catch(function (err) {
        if (err.name === "AbortError") return;
        setStatus("request failed: " + err.message, "error");
      });
  });

  loadInfo();
})();

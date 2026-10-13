/*
 * WebMCP host probe (change webmcp-host-spike, task 1.1).
 *
 * Registers one tool, `echo`, via document.modelContext.registerTool and reports
 * what the host exposed. No network requests, no IPFS calls, no remote assets.
 * Draft: https://webmachinelearning.github.io/webmcp/ (Draft CG Report, 8 October 2026)
 *
 * The probe only reads documented WebMCP surface. It never calls the deprecated
 * navigator.modelContext and never calls host-internal properties.
 */
(function () {
  "use strict";

  var PROBE_VERSION = "1";
  var TOOL_NAME = "echo";
  var loadedAt = new Date();
  var calls = [];
  var state = {
    registration: { status: "pending", detail: "" },
    inPageTools: { status: "unchecked", names: [] },
  };

  // Crockford base32 (no I, L, O, U): easy to read aloud and to compare in a transcript.
  var ALPHABET = "0123456789ABCDEFGHJKMNPQRSTVWXYZ";
  function makeNonce() {
    var out = "";
    var secure = false;
    var bytes = new Uint8Array(12);
    try {
      if (window.crypto && typeof window.crypto.getRandomValues === "function") {
        window.crypto.getRandomValues(bytes);
        secure = true;
      }
    } catch (e) { /* fall through */ }
    if (!secure) {
      for (var i = 0; i < bytes.length; i++) bytes[i] = Math.floor(Math.random() * 256);
    }
    for (var j = 0; j < bytes.length; j++) out += ALPHABET[bytes[j] & 31];
    return { value: "WMCP-" + out.slice(0, 4) + "-" + out.slice(4, 8) + "-" + out.slice(8, 12), cryptoRandom: secure };
  }
  var nonceInfo = makeNonce();
  var NONCE = nonceInfo.value;

  function $(id) { return document.getElementById(id); }
  function iso(d) { return d.toISOString(); }

  function describeError(err) {
    if (err === null || err === undefined) return "unknown error (null)";
    var parts = [];
    try {
      if (err.name) parts.push(String(err.name));
      if (err.message) parts.push(String(err.message));
    } catch (e) { /* ignore */ }
    if (parts.length) return parts.join(": ");
    // Some hosts reject with a plain object whose name and message are null.
    try { return "non-Error rejection " + JSON.stringify(err); } catch (e) { /* ignore */ }
    return "non-Error rejection " + String(err);
  }

  function memberNames(obj) {
    var seen = {};
    var names = [];
    var o = obj;
    var depth = 0;
    while (o && o !== Object.prototype && depth < 4) {
      try {
        Object.getOwnPropertyNames(o).forEach(function (n) {
          if (n !== "constructor" && !seen[n]) { seen[n] = true; names.push(n); }
        });
      } catch (e) { /* ignore */ }
      o = Object.getPrototypeOf(o);
      depth++;
    }
    return names.sort();
  }

  // ---- Feature detection ---------------------------------------------------
  var mc = null;
  try { mc = document.modelContext || null; } catch (e) { mc = null; }
  var hasDocumentModelContext = !!mc;
  var hasRegisterTool = !!mc && typeof mc.registerTool === "function";
  var hasNavigatorModelContext = false;
  try { hasNavigatorModelContext = typeof navigator !== "undefined" && "modelContext" in navigator && !!navigator.modelContext; } catch (e) { /* ignore */ }

  // ---- Permissions policy ("tools" feature, default allowlist 'self') -------
  function readPermissionsPolicy() {
    var pp = null;
    try { pp = document.permissionsPolicy || document.featurePolicy || null; } catch (e) { pp = null; }
    if (!pp) return { api: "unavailable", tools: "unknown", detail: "document.permissionsPolicy and document.featurePolicy are absent" };
    var api = document.permissionsPolicy ? "document.permissionsPolicy" : "document.featurePolicy";
    var tools = "unknown";
    var detail = "";
    try {
      var listed = typeof pp.allowedFeatures === "function" ? pp.allowedFeatures() : null;
      var recognised = listed ? listed.indexOf("tools") >= 0 : null;
      if (typeof pp.allowsFeature === "function") {
        var allows = pp.allowsFeature("tools");
        // A browser that does not know the "tools" feature reports false for it; that is not a denial.
        tools = recognised === false ? "unknown" : allows ? "allowed" : "denied";
      }
      if (listed) {
        detail = recognised
          ? "tools is a recognised feature (" + listed.length + " features listed)"
          : "this browser does not list a tools feature in allowedFeatures() (" + listed.length + " features), so it cannot report its state";
      } else if (tools === "unknown") {
        detail = "allowedFeatures is not implemented";
      }
    } catch (e) {
      detail = "read error: " + describeError(e);
    }
    return { api: api, tools: tools, detail: detail };
  }
  var policy = readPermissionsPolicy();

  // ---- Echo tool ---------------------------------------------------------------
  var echoTool = {
    name: TOOL_NAME,
    title: "Echo",
    description:
      "Returns this page's per-load nonce and echoes the optional message. " +
      "Read-only: it changes nothing. Call it to prove the host agent can invoke a page tool.",
    inputSchema: {
      type: "object",
      properties: {
        message: { type: "string", description: "Optional text to echo back." },
      },
      additionalProperties: false,
    },
    annotations: { readOnlyHint: true },
    // The draft passes (input, { signal }). Some hosts omit the options; treat them as optional.
    execute: function (input, options) {
      var msg = null;
      try { if (input && typeof input.message === "string") msg = input.message.slice(0, 500); } catch (e) { /* ignore */ }
      var at = new Date();
      var entry = { n: calls.length + 1, at: iso(at), message: msg, hadSignal: !!(options && options.signal) };
      calls.push(entry);
      render();
      // JSON-compatible result only (hosts reject BigInt and circular values).
      return Promise.resolve({
        tool: TOOL_NAME,
        nonce: NONCE,
        echo: msg,
        calledAt: entry.at,
        callNumber: entry.n,
        probe: "webmcp-host-spike/" + PROBE_VERSION,
      });
    },
  };

  var registrationController = typeof AbortController === "function" ? new AbortController() : null;
  window.addEventListener("pagehide", function () { if (registrationController) registrationController.abort(); });

  function register() {
    if (!hasDocumentModelContext) {
      state.registration = { status: "absent", detail: "document.modelContext is not present in this browsing context" };
      return Promise.resolve();
    }
    if (!hasRegisterTool) {
      state.registration = { status: "absent", detail: "document.modelContext exists but registerTool is not a function" };
      return Promise.resolve();
    }
    var opts = registrationController ? { signal: registrationController.signal } : undefined;
    var p;
    try {
      p = opts ? mc.registerTool(echoTool, opts) : mc.registerTool(echoTool);
    } catch (e) {
      state.registration = { status: "failed", detail: "threw synchronously: " + describeError(e) };
      return Promise.resolve();
    }
    return Promise.resolve(p).then(
      function () { state.registration = { status: "registered", detail: "registerTool resolved at " + iso(new Date()) }; },
      function (e) { state.registration = { status: "failed", detail: "registerTool rejected: " + describeError(e) }; }
    ).then(inspectInPage);
  }

  // In-page check via the draft's getTools(). Evidence only: the criterion is agent invocation, not this.
  function inspectInPage() {
    if (!hasDocumentModelContext || typeof mc.getTools !== "function") {
      state.inPageTools = { status: "getTools unavailable", names: [] };
      return Promise.resolve();
    }
    try {
      return Promise.resolve(mc.getTools()).then(
        function (list) {
          var names = [];
          try { Array.prototype.forEach.call(list || [], function (t) { names.push(String(t && t.name)); }); } catch (e) { /* ignore */ }
          state.inPageTools = { status: "ok", names: names };
        },
        function (e) { state.inPageTools = { status: "getTools rejected: " + describeError(e), names: [] }; }
      );
    } catch (e) {
      state.inPageTools = { status: "getTools threw: " + describeError(e), names: [] };
      return Promise.resolve();
    }
  }

  // ---- Rendering -------------------------------------------------------------
  function yesNo(v) { return v ? "Yes" : "No"; }
  function badge(el, cls, text) { el.className = "badge badge--" + cls; el.textContent = text; }

  function evidence() {
    return {
      probe: "webmcp-host-spike/" + PROBE_VERSION,
      nonce: NONCE,
      nonceFromCryptoRandom: nonceInfo.cryptoRandom,
      loadedAt: iso(loadedAt),
      url: location.href,
      origin: location.origin,
      isSecureContext: !!window.isSecureContext,
      topLevel: window.top === window.self,
      userAgent: navigator.userAgent,
      criteria: {
        documentModelContextPresent: hasDocumentModelContext,
        registerToolIsFunction: hasRegisterTool,
        echoRegistration: state.registration.status,
        echoRegistrationDetail: state.registration.detail,
        calledByHostAgent: calls.length > 0,
        callCount: calls.length,
      },
      navigatorModelContextDeprecated: hasNavigatorModelContext,
      permissionsPolicy: policy,
      modelContextMembers: hasDocumentModelContext ? memberNames(mc) : [],
      inPageGetTools: state.inPageTools,
      calls: calls,
    };
  }

  function addRow(dl, label, valueNode, badgeCls, badgeText) {
    var row = document.createElement("div");
    row.className = "row";
    var dt = document.createElement("dt");
    dt.textContent = label;
    var dd = document.createElement("dd");
    if (badgeCls) {
      var b = document.createElement("span");
      b.className = "badge badge--" + badgeCls;
      b.textContent = badgeText;
      dd.appendChild(b);
    }
    if (valueNode) dd.appendChild(valueNode);
    row.appendChild(dt);
    row.appendChild(dd);
    dl.appendChild(row);
  }
  function val(text) {
    var s = document.createElement("span");
    s.className = "val";
    s.textContent = text;
    return s;
  }

  function renderReadout() {
    var dl = $("readout");
    while (dl.firstChild) dl.removeChild(dl.firstChild);
    var reg = state.registration;
    addRow(dl, "document.modelContext present", val(hasDocumentModelContext ? "object" : "undefined"), hasDocumentModelContext ? "ok" : "err", yesNo(hasDocumentModelContext));
    addRow(dl, "registerTool is a function", null, hasRegisterTool ? "ok" : "err", yesNo(hasRegisterTool));
    addRow(dl, "echo registration", val(reg.detail || "pending"), reg.status === "registered" ? "ok" : reg.status === "pending" ? "idle" : "err", reg.status === "registered" ? "Registered" : reg.status === "pending" ? "Pending" : reg.status === "absent" ? "Not attempted" : "Failed");
    addRow(dl, "Called by host agent", val(calls.length ? calls.length + " call(s), last " + calls[calls.length - 1].at : "no calls received"), calls.length ? "ok" : "idle", calls.length ? "Called" : "Not called");
    addRow(dl, "navigator.modelContext (deprecated, evidence only)", null, hasNavigatorModelContext ? "warn" : "idle", hasNavigatorModelContext ? "Present" : "Absent");
    addRow(dl, "Secure context", val(location.protocol + "//" + location.host), window.isSecureContext ? "ok" : "err", yesNo(window.isSecureContext));
    addRow(dl, "Top-level page", val(window.top === window.self ? "window.top === window" : "inside a frame"), window.top === window.self ? "ok" : "warn", yesNo(window.top === window.self));
    addRow(dl, "Permissions policy: tools", val(policy.api + (policy.detail ? ": " + policy.detail : "")), policy.tools === "allowed" ? "ok" : policy.tools === "denied" ? "err" : "idle", policy.tools === "allowed" ? "Allowed" : policy.tools === "denied" ? "Denied" : "Unknown");
    addRow(dl, "In-page getTools()", val(state.inPageTools.status === "ok" ? (state.inPageTools.names.length ? state.inPageTools.names.join(", ") : "empty list") : state.inPageTools.status), state.inPageTools.status === "ok" ? "ok" : "idle", state.inPageTools.status === "ok" ? "Listed" : "Not checked");
    addRow(dl, "modelContext members", val(hasDocumentModelContext ? memberNames(mc).join(", ") || "none" : "n/a"), null, null);
    addRow(dl, "User agent", val(navigator.userAgent), null, null);
    addRow(dl, "Loaded at", val(iso(loadedAt)), null, null);
    addRow(dl, "Nonce source", val(nonceInfo.cryptoRandom ? "crypto.getRandomValues" : "Math.random fallback"), null, null);
  }

  function renderLog() {
    var ol = $("log");
    while (ol.firstChild) ol.removeChild(ol.firstChild);
    if (!calls.length) {
      var li = document.createElement("li");
      li.className = "empty";
      li.textContent = "No calls yet. A call by the host agent appears here with its time and input.";
      ol.appendChild(li);
      return;
    }
    calls.forEach(function (c) {
      var li = document.createElement("li");
      li.textContent = "#" + c.n + "  " + c.at + "  message=" + JSON.stringify(c.message) + "  signal=" + (c.hadSignal ? "yes" : "no");
      ol.appendChild(li);
    });
  }

  function renderSummary() {
    var reg = state.registration;
    var s = $("summary");
    if (reg.status === "registered") {
      badge($("reg-badge"), "ok", "Registered");
      s.textContent = calls.length
        ? "The host agent called echo " + calls.length + " time(s). The nonce it received is " + NONCE + "."
        : "echo is registered. Ask the host agent: call the echo tool and tell me the nonce. It is waiting for a call.";
    } else if (reg.status === "pending") {
      badge($("reg-badge"), "idle", "Checking");
      s.textContent = "Running feature detection.";
    } else if (reg.status === "absent") {
      badge($("reg-badge"), "err", "Not registered");
      s.textContent = "No tool registered. " + reg.detail + ".";
    } else {
      badge($("reg-badge"), "err", "Registration failed");
      s.textContent = "Registration failed. " + reg.detail + ".";
    }
    badge($("call-badge"), calls.length ? "ok" : "idle", calls.length ? "Called " + calls.length + "x" : "Not called");
  }

  function render() {
    $("nonce").textContent = NONCE;
    renderSummary();
    renderReadout();
    renderLog();
    $("evidence").textContent = JSON.stringify(evidence(), null, 2);
  }

  // ---- Clipboard (best effort, local only) --------------------------------------------
  function copy(text, label) {
    var out = $("copy-state");
    function done(ok) { out.textContent = ok ? label + " copied." : "Copy blocked by the host. Select the text and copy it by hand."; }
    try {
      if (navigator.clipboard && typeof navigator.clipboard.writeText === "function") {
        navigator.clipboard.writeText(text).then(function () { done(true); }, function () { done(false); });
        return;
      }
    } catch (e) { /* fall through */ }
    done(false);
  }
  $("copy-nonce").addEventListener("click", function () { copy(NONCE, "Nonce"); });
  $("copy-evidence").addEventListener("click", function () { copy(JSON.stringify(evidence(), null, 2), "Evidence JSON"); });

  // ---- Start -----------------------------------------------------------------------------
  render();
  register().then(render, render);
})();

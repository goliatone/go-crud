const state = {
  logs: [],
  endpoints: [],
  selectedEndpoint: null,
  lastUserID: "",
  lastUserName: "",
};

const SIDEBAR_WIDTH_KEY = "rpc-debug-sidebar-width";
const SIDEBAR_MIN = 280;
const SIDEBAR_MAX = 600;

function nextRequestID() {
  if (window.crypto && typeof window.crypto.randomUUID === "function") {
    return window.crypto.randomUUID();
  }
  return `req-${Date.now()}-${Math.floor(Math.random() * 10000)}`;
}

function nowLabel() {
  const now = new Date();
  const hh = String(now.getHours()).padStart(2, "0");
  const mm = String(now.getMinutes()).padStart(2, "0");
  const ss = String(now.getSeconds()).padStart(2, "0");
  return `${hh}${mm}${ss}`;
}

async function fetchJSON(url, options = {}) {
  const response = await fetch(url, options);
  const payload = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new Error(payload.error || `HTTP ${response.status}`);
  }
  return payload;
}

function readActorID() {
  const value = document.getElementById("actor-id").value.trim();
  return value || "demo-admin";
}

function readTenantID() {
  const value = document.getElementById("tenant-id").value.trim();
  return value || "tenant-alpha";
}

function readUserName() {
  const value = document.getElementById("user-name").value.trim();
  return value || "Demo User";
}

function readUserEmail() {
  const value = document.getElementById("user-email").value.trim();
  return value || "demo.user@example.com";
}

function uniqueEmail(baseEmail) {
  const source = (baseEmail || "demo.user@example.com").trim();
  const parts = source.split("@");
  if (parts.length !== 2) {
    return `${source}.${nowLabel()}@example.com`;
  }
  return `${parts[0]}+${nowLabel()}@${parts[1]}`;
}

function buildMeta() {
  return {
    actorId: readActorID(),
    tenant: readTenantID(),
    requestId: nextRequestID(),
    correlationId: "go-crud-web-debug",
  };
}

async function callRPC(method, params, label = "") {
  const request = {
    jsonrpc: "2.0",
    id: nextRequestID(),
    method,
    params,
  };

  const startedAt = performance.now();
  let responsePayload;
  let isError = false;

  try {
    responsePayload = await fetchJSON("/api/rpc", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(request),
    });
    isError = !!responsePayload.error;
  } catch (error) {
    isError = true;
    responsePayload = {
      jsonrpc: "2.0",
      id: request.id,
      error: {
        code: -32099,
        message: "network request failed",
        data: String(error.message || error),
      },
    };
  }

  const durationMS = Math.round((performance.now() - startedAt) * 100) / 100;
  pushLog({
    when: new Date().toISOString(),
    method,
    label,
    durationMS,
    request,
    response: responsePayload,
    isError,
  });

  if (responsePayload.error) {
    throw new Error(responsePayload.error.message || "RPC error");
  }

  return responsePayload.result;
}

function pushLog(entry) {
  state.logs.unshift(entry);
  if (state.logs.length > 50) {
    state.logs.pop();
  }
  appendLogEntry(entry);
  updateLogCount();
}

function updateLogCount() {
  const countEl = document.getElementById("log-count");
  const count = state.logs.length;
  countEl.textContent = count === 1 ? "1 request" : `${count} requests`;
}

function createLogEntryHTML(entry, animate = false) {
  const request = JSON.stringify(entry.request, null, 2);
  const response = JSON.stringify(entry.response, null, 2);
  const statusClass = entry.isError ? "error" : "success";
  const statusText = entry.isError ? "Error" : "OK";

  const errorClass = entry.isError ? " error" : "";
  const animateClass = animate ? " highlight" : "";
  const itemClass = `log-item${errorClass}${animateClass}`;

  return `
    <article class="${itemClass}">
      <div class="log-head">
        <div class="log-head-left">
          <span class="log-method">${escapeHTML(entry.method)}</span>
          ${entry.label ? `<span class="log-label">${escapeHTML(entry.label)}</span>` : ""}
        </div>
        <div class="log-head-right">
          <span class="log-status ${statusClass}">${statusText}</span>
          <span class="log-meta">${entry.durationMS}ms</span>
        </div>
      </div>
      <div class="log-body">
        <div class="log-pane">
          <div class="log-pane-header">Request</div>
          <pre>${escapeHTML(request)}</pre>
        </div>
        <div class="log-pane">
          <div class="log-pane-header">Response</div>
          <pre>${escapeHTML(response)}</pre>
        </div>
      </div>
    </article>
  `;
}

function appendLogEntry(entry) {
  const container = document.getElementById("traffic-log");
  const scrollContainer = document.getElementById("log-scroll");

  const empty = container.querySelector(".empty");
  if (empty) {
    empty.remove();
  }

  const temp = document.createElement("div");
  temp.innerHTML = createLogEntryHTML(entry, true);
  const newItem = temp.firstElementChild;

  container.prepend(newItem);

  newItem.addEventListener("animationend", () => {
    newItem.classList.remove("highlight");
  }, { once: true });

  scrollContainer.scrollTop = 0;

  const items = container.querySelectorAll(".log-item");
  if (items.length > 50) {
    items[items.length - 1].remove();
  }
}

function renderTrafficLog() {
  const container = document.getElementById("traffic-log");

  if (state.logs.length === 0) {
    container.innerHTML = '<div class="empty">No requests yet. Trigger a CRUD RPC method to start.</div>';
    return;
  }

  container.innerHTML = state.logs
    .map((entry) => createLogEntryHTML(entry, false))
    .join("");
}

function escapeHTML(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
}

async function loadEndpoints() {
  const payload = await fetchJSON("/api/endpoints");
  state.endpoints = payload.endpoints || [];
  renderEndpoints();
}

function renderEndpoints() {
  const root = document.getElementById("endpoint-list");
  if (state.endpoints.length === 0) {
    root.innerHTML = '<div class="empty">No endpoints registered.</div>';
    return;
  }

  root.innerHTML = state.endpoints
    .map((endpoint, index) => {
      const summary = endpoint.summary || "No description";
      const isQuery = endpoint.handlerKind === "query";
      const kindClass = isQuery ? "endpoint-kind query" : "endpoint-kind";
      const activeClass = state.selectedEndpoint === index ? " active" : "";

      return `
        <div class="endpoint${activeClass}" data-index="${index}">
          <div class="endpoint-top">
            <span class="endpoint-name">${escapeHTML(endpoint.method)}</span>
            <span class="${kindClass}">${escapeHTML(endpoint.handlerKind || "command")}</span>
          </div>
          <div class="endpoint-meta">${escapeHTML(summary)}</div>
        </div>
      `;
    })
    .join("");

  root.querySelectorAll(".endpoint").forEach((el) => {
    el.addEventListener("click", () => {
      const index = parseInt(el.dataset.index, 10);
      selectEndpoint(index);
    });
  });
}

function baseMetaTemplate() {
  return {
    actorId: readActorID(),
    tenant: readTenantID(),
  };
}

function templateForMethod(method) {
  const lastID = state.lastUserID || "";
  const tenant = readTenantID();
  const name = readUserName();
  const email = readUserEmail();

  switch (method) {
    case "crud.user.create":
      return {
        data: {
          record: {
            name,
            email,
            tenant_id: tenant,
          },
        },
        meta: baseMetaTemplate(),
      };
    case "crud.user.create_batch":
      return {
        data: {
          records: [
            { name: `${name} A`, email: uniqueEmail(email), tenant_id: tenant },
            { name: `${name} B`, email: uniqueEmail(email), tenant_id: tenant },
          ],
        },
        meta: baseMetaTemplate(),
      };
    case "crud.user.show":
      return {
        data: {
          id: lastID,
        },
        meta: baseMetaTemplate(),
      };
    case "crud.user.index":
      return {
        data: {
          options: {
            limit: 10,
            offset: 0,
            order: "updated_at desc",
          },
        },
        meta: baseMetaTemplate(),
      };
    case "crud.user.update":
      return {
        data: {
          id: lastID,
          record: {
            name: `${name} (updated)`,
          },
        },
        meta: baseMetaTemplate(),
      };
    case "crud.user.update_batch":
      return {
        data: {
          records: [],
        },
        meta: baseMetaTemplate(),
      };
    case "crud.user.delete":
      return {
        data: {
          id: lastID,
        },
        meta: baseMetaTemplate(),
      };
    case "crud.user.delete_batch":
      return {
        data: {
          ids: lastID ? [lastID] : [],
        },
        meta: baseMetaTemplate(),
      };
    default:
      return {
        data: {},
        meta: baseMetaTemplate(),
      };
  }
}

function selectEndpoint(index) {
  const endpoint = state.endpoints[index];
  if (!endpoint) return;

  state.selectedEndpoint = index;

  document.getElementById("method-name").value = endpoint.method;
  document.getElementById("params-json").value = JSON.stringify(templateForMethod(endpoint.method), null, 2);

  renderEndpoints();
  document.getElementById("method-name").focus();
}

async function loadState() {
  const payload = await fetchJSON("/api/state");
  renderState(payload.state || {}, false);
}

function renderState(snapshot, animate = true) {
  const countEl = document.getElementById("state-count");
  const idEl = document.getElementById("state-last-id");
  const nameEl = document.getElementById("state-last-name");

  const count = Number(snapshot.count || 0);
  const lastID = snapshot.lastId || "";
  const lastName = snapshot.lastName || "";

  state.lastUserID = lastID;
  state.lastUserName = lastName;

  countEl.textContent = `${count}`;
  idEl.textContent = lastID ? truncateID(lastID) : "-";
  nameEl.textContent = lastName || "-";

  if (animate) {
    flashStateItems();
  }
}

function applyListResult(resultEnvelope, animate = true) {
  const data = (resultEnvelope && resultEnvelope.data) || {};
  const items = Array.isArray(data.items) ? data.items : [];
  const count = Number.isFinite(data.count) ? data.count : items.length;

  const snapshot = {
    count,
    lastId: items[0] && items[0].id ? items[0].id : "",
    lastName: items[0] && items[0].name ? items[0].name : "",
  };

  renderState(snapshot, animate);
}

function truncateID(value) {
  if (!value) return "";
  if (value.length <= 12) return value;
  return `${value.slice(0, 8)}...`;
}

function flashStateItems() {
  const items = document.querySelectorAll(".state-item");
  items.forEach((item) => {
    item.classList.add("flash");
  });

  setTimeout(() => {
    items.forEach((item) => {
      item.classList.remove("flash");
    });
  }, 400);
}

function ensureLastUserID() {
  if (state.lastUserID) {
    return state.lastUserID;
  }
  throw new Error("No user selected. Run List or Create first.");
}

async function runCreate() {
  const name = readUserName();
  const tenant = readTenantID();
  const email = uniqueEmail(readUserEmail());

  const result = await callRPC(
    "crud.user.create",
    {
      data: {
        record: {
          name,
          email,
          tenant_id: tenant,
        },
      },
      meta: buildMeta(),
    },
    "create",
  );

  if (result && result.data && result.data.id) {
    state.lastUserID = result.data.id;
    state.lastUserName = result.data.name || "";
  }

  await runList(false);
}

async function runList(withLabel = true) {
  const label = withLabel ? "list" : "refresh";
  const result = await callRPC(
    "crud.user.index",
    {
      data: {
        options: {
          limit: 25,
          offset: 0,
          order: "updated_at desc",
        },
      },
      meta: buildMeta(),
    },
    label,
  );

  applyListResult(result, true);
}

async function runShowLast() {
  const id = ensureLastUserID();
  const result = await callRPC(
    "crud.user.show",
    {
      data: { id },
      meta: buildMeta(),
    },
    "show",
  );

  if (result && result.data) {
    renderState({
      count: Number(document.getElementById("state-count").textContent || 0),
      lastId: result.data.id || id,
      lastName: result.data.name || "",
    }, true);
  }
}

async function runUpdateLast() {
  const id = ensureLastUserID();
  const nextName = `${state.lastUserName || readUserName()} updated ${nowLabel()}`;

  await callRPC(
    "crud.user.update",
    {
      data: {
        id,
        record: {
          name: nextName,
        },
      },
      meta: buildMeta(),
    },
    "update",
  );

  await runList(false);
}

async function runDeleteLast() {
  const id = ensureLastUserID();

  await callRPC(
    "crud.user.delete",
    {
      data: { id },
      meta: buildMeta(),
    },
    "delete",
  );

  state.lastUserID = "";
  state.lastUserName = "";
  await runList(false);
}

async function runCustomCall() {
  const method = document.getElementById("method-name").value.trim();
  if (!method) {
    throw new Error("Method name is required");
  }

  const rawParams = document.getElementById("params-json").value.trim();
  let params = {};
  if (rawParams) {
    try {
      params = JSON.parse(rawParams);
    } catch (e) {
      throw new Error(`Invalid JSON: ${e.message}`);
    }
  }

  const result = await callRPC(method, params, "custom");

  if (method === "crud.user.index") {
    applyListResult(result, true);
    return;
  }

  if ((method === "crud.user.show" || method === "crud.user.create" || method === "crud.user.update") && result && result.data) {
    renderState({
      count: Number(document.getElementById("state-count").textContent || 0),
      lastId: result.data.id || state.lastUserID,
      lastName: result.data.name || state.lastUserName,
    }, true);
    return;
  }

  if (method === "crud.user.delete" || method === "crud.user.delete_batch") {
    await runList(false);
  }
}

async function withAction(action) {
  const buttons = Array.from(document.querySelectorAll("button"));
  buttons.forEach((button) => {
    button.disabled = true;
  });

  try {
    await action();
  } catch (error) {
    pushLog({
      when: new Date().toISOString(),
      method: "client.error",
      label: "error",
      durationMS: 0,
      request: { action: "failed" },
      response: { error: { message: String(error.message || error) } },
      isError: true,
    });
  } finally {
    buttons.forEach((button) => {
      button.disabled = false;
    });
  }
}

function wireEvents() {
  document.getElementById("btn-create").addEventListener("click", () => withAction(runCreate));
  document.getElementById("btn-list").addEventListener("click", () => withAction(runList));
  document.getElementById("btn-show").addEventListener("click", () => withAction(runShowLast));
  document.getElementById("btn-update").addEventListener("click", () => withAction(runUpdateLast));
  document.getElementById("btn-delete").addEventListener("click", () => withAction(runDeleteLast));
  document.getElementById("btn-send-custom").addEventListener("click", () => withAction(runCustomCall));
}

function initResize() {
  const sidebar = document.getElementById("sidebar");
  const handle = document.getElementById("resize-handle");

  if (!sidebar || !handle) return;

  const savedWidth = localStorage.getItem(SIDEBAR_WIDTH_KEY);
  if (savedWidth) {
    const width = parseInt(savedWidth, 10);
    if (width >= SIDEBAR_MIN && width <= SIDEBAR_MAX) {
      sidebar.style.width = `${width}px`;
    }
  }

  let isDragging = false;
  let startX = 0;
  let startWidth = 0;

  function onMouseDown(e) {
    isDragging = true;
    startX = e.clientX;
    startWidth = sidebar.offsetWidth;

    document.body.classList.add("resizing");
    handle.classList.add("dragging");

    document.addEventListener("mousemove", onMouseMove);
    document.addEventListener("mouseup", onMouseUp);

    e.preventDefault();
  }

  function onMouseMove(e) {
    if (!isDragging) return;

    const delta = e.clientX - startX;
    let newWidth = startWidth + delta;

    newWidth = Math.max(SIDEBAR_MIN, Math.min(SIDEBAR_MAX, newWidth));

    sidebar.style.width = `${newWidth}px`;
  }

  function onMouseUp() {
    if (!isDragging) return;

    isDragging = false;
    document.body.classList.remove("resizing");
    handle.classList.remove("dragging");

    document.removeEventListener("mousemove", onMouseMove);
    document.removeEventListener("mouseup", onMouseUp);

    localStorage.setItem(SIDEBAR_WIDTH_KEY, sidebar.offsetWidth.toString());
  }

  handle.addEventListener("mousedown", onMouseDown);
}

async function bootstrap() {
  wireEvents();
  initResize();
  renderTrafficLog();
  updateLogCount();
  await Promise.all([loadEndpoints(), loadState()]);
  await runList(false);
}

bootstrap().catch((error) => {
  pushLog({
    when: new Date().toISOString(),
    method: "bootstrap",
    label: "init",
    durationMS: 0,
    request: { action: "initialize" },
    response: { error: { message: String(error.message || error) } },
    isError: true,
  });
});

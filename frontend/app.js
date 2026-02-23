const state = {
  runs: [],
  selectedRunId: null,
  availableSources: [],
  availableEntities: [],
  refreshIntervalMs: 15000,
  selectors: {
    sources: {
      selected: new Set(),
      query: "",
    },
    entities: {
      selected: new Set(),
      query: "",
    },
  },
};

const elements = {
  healthStatus: document.getElementById("health-status"),
  sourcesEnabled: document.getElementById("sources-enabled"),
  profilesCount: document.getElementById("profiles-count"),
  runForm: document.getElementById("run-form"),
  runSubmitStatus: document.getElementById("run-submit-status"),
  runSubmitBtn: document.getElementById("run-submit-btn"),
  sourceForm: document.getElementById("source-form"),
  sourceSubmitStatus: document.getElementById("source-submit-status"),
  sourceSubmitBtn: document.getElementById("source-submit-btn"),
  bhavcopyForm: document.getElementById("bhavcopy-form"),
  bhavcopyDate: document.getElementById("bhavcopy-date"),
  bhavcopyStatus: document.getElementById("bhavcopy-status"),
  bhavcopyDownloadBtn: document.getElementById("bhavcopy-download-btn"),
  runsState: document.getElementById("runs-state"),
  runsTableBody: document.getElementById("runs-table-body"),
  refreshBtn: document.getElementById("refresh-btn"),
  detailsRunId: document.getElementById("details-run-id"),
  detailsContent: document.getElementById("details-content"),
  selectors: {
    sources: {
      searchInput: document.getElementById("sources-search"),
      optionsContainer: document.getElementById("sources-options"),
      selectAllBtn: document.getElementById("sources-select-all"),
      clearBtn: document.getElementById("sources-clear"),
    },
    entities: {
      searchInput: document.getElementById("entities-search"),
      optionsContainer: document.getElementById("entities-options"),
      selectAllBtn: document.getElementById("entities-select-all"),
      clearBtn: document.getElementById("entities-clear"),
    },
  },
};

async function api(path, options = {}) {
  const response = await fetch(`/api${path}`, {
    headers: { "Content-Type": "application/json", ...(options.headers || {}) },
    ...options,
  });

  if (!response.ok) {
    const text = await response.text();
    throw new Error(text || `HTTP ${response.status}`);
  }

  const contentType = response.headers.get("content-type") || "";
  if (contentType.includes("application/json")) {
    return response.json();
  }
  return response.text();
}

function formatDate(value) {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "-";
  return date.toLocaleString();
}

function escapeHtml(value) {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

function statusBadge(status) {
  const normalized = String(status || "").toLowerCase();
  const allowed = new Set(["queued", "running", "completed", "failed", "cancelled"]);
  const safeStatus = allowed.has(normalized) ? normalized : "unknown";
  return `<span class="badge ${safeStatus}">${safeStatus}</span>`;
}

function toUtcISOStringFromLocalInput(value) {
  if (!value) return "";
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return "";
  return d.toISOString();
}

function setRunSubmitStatus(message, isError = false) {
  elements.runSubmitStatus.textContent = message;
  elements.runSubmitStatus.style.color = isError ? "var(--error)" : "var(--muted)";
}

function setSourceSubmitStatus(message, isError = false) {
  if (!elements.sourceSubmitStatus) return;
  elements.sourceSubmitStatus.textContent = message;
  elements.sourceSubmitStatus.style.color = isError ? "var(--error)" : "var(--muted)";
}

function setBhavcopyStatus(message, isError = false) {
  if (!elements.bhavcopyStatus) return;
  elements.bhavcopyStatus.textContent = message;
  elements.bhavcopyStatus.style.color = isError ? "var(--error)" : "var(--muted)";
}

function normalizeSource(source) {
  if (!source || !source.id || !source.name) return null;
  return {
    id: String(source.id),
    name: String(source.name),
    region: String(source.region || ""),
    language: String(source.language || ""),
    kind: String(source.kind || "rss"),
    url: String(source.url || ""),
    crawlFallback: Boolean(source.crawl_fallback),
  };
}

function normalizeEntity(entity) {
  if (!entity || !entity.symbol || !entity.name) return null;
  const aliases = Array.isArray(entity.aliases)
    ? entity.aliases.map((alias) => String(alias || "").trim()).filter(Boolean)
    : [];
  return {
    id: String(entity.id || ""),
    symbol: String(entity.symbol),
    name: String(entity.name),
    exchange: String(entity.exchange || ""),
    sector: String(entity.sector || ""),
    type: String(entity.type || "equity"),
    aliases,
  };
}

function getSelectorData(selectorName) {
  if (selectorName === "sources") return state.availableSources;
  if (selectorName === "entities") return state.availableEntities;
  return [];
}

function getSelectorSearchText(selectorName, item) {
  if (selectorName === "sources") {
    return `${item.id} ${item.name} ${item.region} ${item.language} ${item.kind}`.toLowerCase();
  }
  return `${item.symbol} ${item.name} ${item.exchange} ${item.sector} ${item.type} ${(item.aliases || []).join(" ")}`.toLowerCase();
}

function getSelectorValue(selectorName, item) {
  return selectorName === "sources" ? item.id : item.symbol;
}

function renderSelectorOption(selectorName, item, isSelected) {
  if (selectorName === "sources") {
    return `
      <div>
        <div class="selector-option-title">${escapeHtml(item.name)} (${escapeHtml(item.id)})</div>
        <div class="selector-option-meta">${escapeHtml(item.kind)} · ${escapeHtml(item.region)} · ${escapeHtml(item.language)}${item.crawlFallback ? " · fallback" : ""}</div>
      </div>
    `;
  }

  const aliases = item.aliases && item.aliases.length ? item.aliases.join(", ") : "-";
  return `
    <div>
      <div class="selector-option-title">${escapeHtml(item.symbol)} · ${escapeHtml(item.name)}</div>
      <div class="selector-option-meta">${escapeHtml(item.type)} · ${escapeHtml(item.exchange)} · ${escapeHtml(item.sector)} · aliases: ${escapeHtml(aliases)}</div>
    </div>
  `;
}

function renderMultiSelect(selectorName) {
  const selectorElements = elements.selectors[selectorName];
  const selectorState = state.selectors[selectorName];
  if (!selectorElements || !selectorElements.optionsContainer) return;

  const data = getSelectorData(selectorName);
  const query = (selectorState.query || "").trim().toLowerCase();
  const filtered = !query
    ? data
    : data.filter((item) => getSelectorSearchText(selectorName, item).includes(query));

  selectorElements.optionsContainer.innerHTML = "";

  if (!filtered.length) {
    const empty = document.createElement("div");
    empty.className = "selector-empty";
    empty.textContent = query ? "No matches found" : "No options available";
    selectorElements.optionsContainer.appendChild(empty);
    return;
  }

  for (const item of filtered) {
    const value = getSelectorValue(selectorName, item);
    const option = document.createElement("label");
    option.className = "selector-option";

    const checkbox = document.createElement("input");
    checkbox.type = "checkbox";
    checkbox.value = value;
    checkbox.checked = selectorState.selected.has(value);
    checkbox.addEventListener("change", () => {
      if (checkbox.checked) {
        selectorState.selected.add(value);
      } else {
        selectorState.selected.delete(value);
      }
    });

    option.appendChild(checkbox);

    const content = document.createElement("div");
    content.innerHTML = renderSelectorOption(selectorName, item, checkbox.checked);
    option.appendChild(content);

    selectorElements.optionsContainer.appendChild(option);
  }
}

function selectAllFiltered(selectorName) {
  const selectorState = state.selectors[selectorName];
  const data = getSelectorData(selectorName);
  const query = (selectorState.query || "").trim().toLowerCase();
  const filtered = !query
    ? data
    : data.filter((item) => getSelectorSearchText(selectorName, item).includes(query));

  for (const item of filtered) {
    selectorState.selected.add(getSelectorValue(selectorName, item));
  }
  renderMultiSelect(selectorName);
}

function clearAll(selectorName) {
  state.selectors[selectorName].selected.clear();
  renderMultiSelect(selectorName);
}

function getSelectedValues(selectorName) {
  return Array.from(state.selectors[selectorName].selected.values());
}

function renderRuns() {
  const runs = state.runs;
  elements.runsTableBody.innerHTML = "";

  if (!runs.length) {
    elements.runsState.textContent = "No runs yet.";
    elements.detailsRunId.textContent = "No run selected";
    elements.detailsContent.textContent = "Trigger a run to see details and exports.";
    return;
  }

  elements.runsState.textContent = `Showing ${runs.length} run(s).`;

  for (const run of runs) {
    const tr = document.createElement("tr");
    tr.dataset.clickable = "true";
    tr.innerHTML = `
      <td>${escapeHtml(run.run_id)}</td>
      <td>${statusBadge(run.status)}</td>
      <td>${escapeHtml(run.profile || "-")}</td>
      <td>${escapeHtml(formatDate(run.created_at))}</td>
      <td>${Array.isArray(run.events) ? run.events.length : 0}</td>
      <td>${Array.isArray(run.feature_rows) ? run.feature_rows.length : 0}</td>
      <td>${Number(run.estimated_cost_usd || 0).toFixed(4)}</td>
    `;
    tr.addEventListener("click", () => {
      state.selectedRunId = run.run_id;
      renderDetails(run);
    });
    elements.runsTableBody.appendChild(tr);
  }

  const selected = runs.find((r) => r.run_id === state.selectedRunId) || runs[0];
  state.selectedRunId = selected.run_id;
  renderDetails(selected);
}

function renderDetails(run) {
  const runId = String(run.run_id || "");
  const encodedRunID = encodeURIComponent(runId);

  elements.detailsRunId.textContent = runId;
  elements.detailsContent.innerHTML = `
    <div class="detail-grid">
      <div class="detail-item"><h3>Status</h3><p>${statusBadge(run.status)}</p></div>
      <div class="detail-item"><h3>Profile</h3><p>${escapeHtml(run.profile || "-")}</p></div>
      <div class="detail-item"><h3>Created at</h3><p>${escapeHtml(formatDate(run.created_at))}</p></div>
      <div class="detail-item"><h3>Started at</h3><p>${escapeHtml(formatDate(run.started_at))}</p></div>
      <div class="detail-item"><h3>Finished at</h3><p>${escapeHtml(formatDate(run.finished_at))}</p></div>
      <div class="detail-item"><h3>Input tokens</h3><p>${run.input_tokens ?? 0}</p></div>
      <div class="detail-item"><h3>Output tokens</h3><p>${run.output_tokens ?? 0}</p></div>
      <div class="detail-item"><h3>Estimated cost</h3><p>${Number(run.estimated_cost_usd || 0).toFixed(6)} USD</p></div>
      <div class="detail-item"><h3>Events</h3><p>${Array.isArray(run.events) ? run.events.length : 0}</p></div>
      <div class="detail-item"><h3>Features</h3><p>${Array.isArray(run.feature_rows) ? run.feature_rows.length : 0}</p></div>
      <div class="detail-item"><h3>Failure reason</h3><p>${escapeHtml(run.failure_reason || "-")}</p></div>
    </div>
    <div class="export-links">
      <a href="/api/v1/runs/${encodedRunID}/export?format=jsonl" target="_blank" rel="noreferrer">Download JSONL</a>
      <a href="/api/v1/runs/${encodedRunID}/export?format=csv" target="_blank" rel="noreferrer">Download CSV</a>
      <a href="/api/v1/runs/${encodedRunID}/export?format=toon" target="_blank" rel="noreferrer">Download TOON</a>
    </div>
  `;
}

function preserveSelections() {
  const validSourceIDs = new Set(state.availableSources.map((source) => source.id));
  state.selectors.sources.selected = new Set(
    Array.from(state.selectors.sources.selected).filter((id) => validSourceIDs.has(id))
  );

  const validSymbols = new Set(state.availableEntities.map((entity) => entity.symbol));
  state.selectors.entities.selected = new Set(
    Array.from(state.selectors.entities.selected).filter((symbol) => validSymbols.has(symbol))
  );
}

async function loadSummary() {
  try {
    const [health, config] = await Promise.all([api("/health"), api("/v1/config")]);
    elements.healthStatus.textContent = health.status === "ok" ? "OK" : "Unavailable";
    elements.healthStatus.style.color = health.status === "ok" ? "var(--ok)" : "var(--error)";
    elements.sourcesEnabled.textContent = String(config.sources_enabled ?? "-");
    elements.profilesCount.textContent = String((config.profiles || []).length);

    state.availableSources = (Array.isArray(config.sources) ? config.sources : [])
      .map(normalizeSource)
      .filter(Boolean)
      .sort((a, b) => a.name.localeCompare(b.name));

    state.availableEntities = (Array.isArray(config.entities) ? config.entities : [])
      .map(normalizeEntity)
      .filter(Boolean)
      .sort((a, b) => a.symbol.localeCompare(b.symbol));

    preserveSelections();
    renderMultiSelect("sources");
    renderMultiSelect("entities");
  } catch (err) {
    elements.healthStatus.textContent = "Error";
    elements.healthStatus.style.color = "var(--error)";
    elements.sourcesEnabled.textContent = "-";
    elements.profilesCount.textContent = "-";
    state.availableSources = [];
    state.availableEntities = [];
    state.selectors.sources.selected.clear();
    state.selectors.entities.selected.clear();
    renderMultiSelect("sources");
    renderMultiSelect("entities");
    console.error(err);
  }
}

async function loadRuns() {
  elements.runsState.textContent = "Loading runs...";
  try {
    const payload = await api("/v1/runs");
    state.runs = Array.isArray(payload.runs) ? payload.runs : [];
    renderRuns();
  } catch (err) {
    elements.runsState.textContent = "Failed to load runs.";
    elements.detailsRunId.textContent = "No run selected";
    elements.detailsContent.textContent = String(err.message || err);
  }
}

async function submitRunForm(event) {
  event.preventDefault();
  const formData = new FormData(elements.runForm);

  const profile = String(formData.get("pipeline_profile") || "").trim();
  const selectedSources = getSelectedValues("sources");
  const selectedEntities = getSelectedValues("entities");
  const dateFromRaw = String(formData.get("date_from") || "").trim();
  const dateToRaw = String(formData.get("date_to") || "").trim();

  const payload = {};
  if (profile) payload.pipeline_profile = profile;
  if (selectedSources.length) payload.sources = selectedSources;
  if (selectedEntities.length) payload.entities = selectedEntities;

  const dateFrom = toUtcISOStringFromLocalInput(dateFromRaw);
  const dateTo = toUtcISOStringFromLocalInput(dateToRaw);
  if (dateFrom) payload.date_from = dateFrom;
  if (dateTo) payload.date_to = dateTo;

  elements.runSubmitBtn.disabled = true;
  setRunSubmitStatus("Submitting run...");

  try {
    const response = await api("/v1/runs", {
      method: "POST",
      body: JSON.stringify(payload),
    });
    const runId = response?.run?.run_id || "(unknown)";
    setRunSubmitStatus(`Run triggered: ${runId}`);
    await loadRuns();
  } catch (err) {
    setRunSubmitStatus(`Run trigger failed: ${err.message || err}`, true);
  } finally {
    elements.runSubmitBtn.disabled = false;
  }
}

async function submitSourceForm(event) {
  event.preventDefault();
  if (!elements.sourceForm) return;

  const formData = new FormData(elements.sourceForm);
  const payload = {
    id: String(formData.get("id") || "").trim(),
    name: String(formData.get("name") || "").trim(),
    kind: String(formData.get("kind") || "rss").trim(),
    url: String(formData.get("url") || "").trim(),
    region: String(formData.get("region") || "").trim(),
    language: String(formData.get("language") || "").trim(),
    enabled: formData.get("enabled") !== null,
    crawl_fallback: formData.get("crawl_fallback") !== null,
  };

  if (!payload.id || !payload.name || !payload.url || !payload.region || !payload.language) {
    setSourceSubmitStatus("Please fill all required source fields.", true);
    return;
  }

  elements.sourceSubmitBtn.disabled = true;
  setSourceSubmitStatus("Adding source...");

  try {
    await api("/v1/sources", {
      method: "POST",
      body: JSON.stringify(payload),
    });
    setSourceSubmitStatus(`Source added: ${payload.id}`);
    elements.sourceForm.reset();
    const enabledInput = elements.sourceForm.querySelector('input[name="enabled"]');
    if (enabledInput) enabledInput.checked = true;
    await loadSummary();
  } catch (err) {
    setSourceSubmitStatus(`Source add failed: ${err.message || err}`, true);
  } finally {
    elements.sourceSubmitBtn.disabled = false;
  }
}

function todayUTCDateString() {
  return new Date().toISOString().slice(0, 10);
}

function downloadBlob(blob, filename) {
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = filename;
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
  URL.revokeObjectURL(url);
}

function parseDownloadFilename(contentDisposition, fallback) {
  const value = String(contentDisposition || "");
  const utf8Match = value.match(/filename\*=UTF-8''([^;]+)/i);
  if (utf8Match && utf8Match[1]) {
    try {
      return decodeURIComponent(utf8Match[1].trim());
    } catch (_) {
      return utf8Match[1].trim();
    }
  }
  const quoted = value.match(/filename="([^"]+)"/i);
  if (quoted && quoted[1]) return quoted[1].trim();
  const plain = value.match(/filename=([^;]+)/i);
  if (plain && plain[1]) return plain[1].trim();
  return fallback;
}

function addDays(dateString, days) {
  const date = new Date(`${dateString}T00:00:00Z`);
  date.setUTCDate(date.getUTCDate() + days);
  return date;
}

async function submitBhavcopyDownload(event) {
  event.preventDefault();
  if (!elements.bhavcopyDate || !elements.bhavcopyDownloadBtn) return;

  const selectedDate = String(elements.bhavcopyDate.value || "").trim();
  if (!selectedDate) {
    setBhavcopyStatus("Select a trade date first.", true);
    return;
  }

  const availableOn = addDays(selectedDate, 2);
  const now = new Date();
  if (availableOn > now) {
    setBhavcopyStatus(`Bhavcopy for ${selectedDate} can be downloaded after ${availableOn.toISOString()}.`, true);
    return;
  }

  elements.bhavcopyDownloadBtn.disabled = true;
  setBhavcopyStatus(`Downloading bhavcopy for ${selectedDate}...`);

  try {
    const requestURL = `/api/v1/bhavcopy/download?date=${encodeURIComponent(selectedDate)}&ts=${Date.now()}`;
    const response = await fetch(requestURL, { cache: "no-store" });
    if (!response.ok) {
      const text = await response.text();
      throw new Error(text || `HTTP ${response.status}`);
    }
    const blob = await response.blob();
    const compactDate = selectedDate.replaceAll("-", "");
    const ddmmyyyy = `${selectedDate.slice(8, 10)}${selectedDate.slice(5, 7)}${selectedDate.slice(0, 4)}`;
    const contentType = String(response.headers.get("content-type") || "").toLowerCase();
    const csvFallback = `sec_bhavdata_full_${ddmmyyyy}.csv`;
    const zipFallback = `BhavCopy_NSE_CM_${compactDate}.zip`;
    let filename = parseDownloadFilename(
      response.headers.get("content-disposition"),
      contentType.includes("csv") ? csvFallback : zipFallback
    );
    if (contentType.includes("csv") && filename.toLowerCase().endsWith(".zip")) {
      filename = csvFallback;
    }
    downloadBlob(blob, filename);
    setBhavcopyStatus(`Bhavcopy downloaded: ${filename}`);
  } catch (err) {
    setBhavcopyStatus(`Bhavcopy download failed: ${err.message || err}`, true);
  } finally {
    elements.bhavcopyDownloadBtn.disabled = false;
  }
}

function setupSelectorEvents(selectorName) {
  const selectorElements = elements.selectors[selectorName];
  if (!selectorElements) return;

  selectorElements.searchInput?.addEventListener("input", (event) => {
    state.selectors[selectorName].query = String(event.target.value || "");
    renderMultiSelect(selectorName);
  });

  selectorElements.selectAllBtn?.addEventListener("click", () => {
    selectAllFiltered(selectorName);
  });

  selectorElements.clearBtn?.addEventListener("click", () => {
    clearAll(selectorName);
  });
}

function setupBhavcopyDateDefault() {
  if (!elements.bhavcopyDate) return;
  const today = todayUTCDateString();
  elements.bhavcopyDate.value = today;
  elements.bhavcopyDate.max = today;
}

function setupEvents() {
  elements.runForm.addEventListener("submit", submitRunForm);
  elements.sourceForm?.addEventListener("submit", submitSourceForm);
  elements.bhavcopyForm?.addEventListener("submit", submitBhavcopyDownload);
  elements.refreshBtn.addEventListener("click", () => loadRuns());
  setupSelectorEvents("sources");
  setupSelectorEvents("entities");
  setupBhavcopyDateDefault();
}

async function bootstrap() {
  setupEvents();
  await loadSummary();
  await loadRuns();
  window.setInterval(loadSummary, state.refreshIntervalMs);
  window.setInterval(loadRuns, state.refreshIntervalMs);
}

bootstrap();

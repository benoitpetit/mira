// MIRA Dashboard App
const API_BASE = "/api/v1";

// Load stats on page load
document.addEventListener("DOMContentLoaded", () => {
    loadStats();
    loadWings();
    setupEventListeners();
});
function setupEventListeners() {
    document.getElementById("refreshBtn").addEventListener("click", () => {
        loadStats();
        goToFirstPage();
    });

    document.getElementById("searchBtn").addEventListener("click", searchMemories);
    document.getElementById("searchInput").addEventListener("keypress", (e) => {
        if (e.key === "Enter") searchMemories();
    });

    document.getElementById("thresholdSlider").addEventListener("input", (e) => {
        document.getElementById("thresholdVal").textContent = parseFloat(e.target.value).toFixed(2);
    });

    document.getElementById("loadTimelineBtn").addEventListener("click", goToFirstPage);
    document.getElementById("prevPageBtn").addEventListener("click", goToPrevPage);
    document.getElementById("nextPageBtn").addEventListener("click", goToNextPage);

    // Reset to page 1 when filters change
    document.getElementById("timelineWing").addEventListener("change", goToFirstPage);
    document.getElementById("timelineType").addEventListener("change", goToFirstPage);
    document.getElementById("timelinePageSize").addEventListener("change", goToFirstPage);
}

async function loadStats() {
    try {
        const response = await fetch(`${API_BASE}/status`);
        const data = await response.json();
        
        if (data.stats) {
            document.getElementById("verbCount").textContent = data.stats.verbatim_count || 0;
            document.getElementById("fpCount").textContent = data.stats.fingerprint_count || 0;
            document.getElementById("embCount").textContent = data.stats.embedding_count || 0;
            document.getElementById("totalTokens").textContent = (data.stats.total_tokens || 0).toLocaleString();
        }

        if (data.soul && data.soul.enabled) {
            renderSoulStats(data.soul);
        }
    } catch (error) {
        console.error("Failed to load stats:", error);
    }
}

async function loadWings() {
    try {
        const response = await fetch(`${API_BASE}/status`);
        const data = await response.json();
        
        const select = document.getElementById("timelineWing");
        if (data.stats && data.stats.active_wings) {
            data.stats.active_wings.forEach(wing => {
                const option = document.createElement("option");
                option.value = wing;
                option.textContent = wing;
                select.appendChild(option);
            });
        }
    } catch (error) {
        console.error("Failed to load wings:", error);
    }
}

async function searchMemories() {
    const query = document.getElementById("searchInput").value.trim();
    if (!query) return;

    const threshold = parseFloat(document.getElementById("thresholdSlider").value) || 0.15;
    const topK = parseInt(document.getElementById("topKInput").value) || 20;

    const resultsDiv = document.getElementById("searchResults");
    resultsDiv.innerHTML = '<div class="loading">Searching...</div>';

    try {
        const response = await fetch(`${API_BASE}/memories/search`, {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ query, top_k: topK, threshold })
        });

        const data = await response.json();
        displaySearchResults(data.results || []);
    } catch (error) {
        resultsDiv.innerHTML = `<div class="error">Search failed: ${error.message}</div>`;
    }
}

function displaySearchResults(results) {
    const resultsDiv = document.getElementById("searchResults");
    
    if (results.length === 0) {
        resultsDiv.innerHTML = '<div class="loading">No results found</div>';
        return;
    }

    resultsDiv.innerHTML = results.map(r => `
        <div class="result-item">
            <h4>${escapeHtml(r.content?.substring(0, 100) || "No content")}${r.content?.length > 100 ? "..." : ""}</h4>
            <div class="meta">
                <span>Type: ${r.type || "unknown"}</span>
                <span>Wing: ${r.wing || "unknown"}</span>
                <span class="similarity ${similarityClass(r.similarity)}">Score: ${(r.similarity * 100).toFixed(1)}%</span>
            </div>
        </div>
    `).join("");
}

// ── Timeline pagination state ─────────────────────────────────────────────
const timeline = {
    cursorStack: [null],   // stack[0] = null (first page), stack[n] = cursor for page n+1
    currentPage: 1,
    nextCursor: null,
    hasNext: false,
};

function timelinePageSize() {
    return parseInt(document.getElementById("timelinePageSize").value) || 20;
}

function timelineWing() {
    return document.getElementById("timelineWing").value || "";
}

function timelineType() {
    return document.getElementById("timelineType").value || "";
}

async function loadTimeline(cursor = null) {
    const resultsDiv = document.getElementById("timelineResults");
    resultsDiv.innerHTML = '<div class="loading">Loading...</div>';

    let url = `${API_BASE}/timeline?limit=${timelinePageSize()}`;
    if (timelineWing()) url += `&wing=${encodeURIComponent(timelineWing())}`;
    if (timelineType()) url += `&type=${encodeURIComponent(timelineType())}`;
    if (cursor)         url += `&cursor=${encodeURIComponent(cursor)}`;

    try {
        const response = await fetch(url);
        const data = await response.json();
        const items = data.items || [];

        timeline.nextCursor = data.next_cursor || null;
        timeline.hasNext = items.length >= timelinePageSize() && !!data.next_cursor;

        displayTimeline(items);
        updatePaginationControls();
    } catch (error) {
        resultsDiv.innerHTML = `<div class="error">Failed to load timeline: ${error.message}</div>`;
    }
}

function goToFirstPage() {
    timeline.cursorStack = [null];
    timeline.currentPage = 1;
    loadTimeline(null);
}

function goToNextPage() {
    if (!timeline.hasNext || !timeline.nextCursor) return;
    timeline.cursorStack.push(timeline.nextCursor);
    timeline.currentPage++;
    loadTimeline(timeline.nextCursor);
}

function goToPrevPage() {
    if (timeline.currentPage <= 1) return;
    timeline.cursorStack.pop();
    timeline.currentPage--;
    const cursor = timeline.cursorStack[timeline.cursorStack.length - 1];
    loadTimeline(cursor);
}

function updatePaginationControls() {
    const pag = document.getElementById("timelinePagination");
    const prevBtn = document.getElementById("prevPageBtn");
    const nextBtn = document.getElementById("nextPageBtn");
    const indicator = document.getElementById("pageIndicator");

    pag.style.display = "flex";
    prevBtn.disabled = timeline.currentPage <= 1;
    nextBtn.disabled = !timeline.hasNext;
    indicator.textContent = `Page ${timeline.currentPage}`;
}

function displayTimeline(items) {
    const resultsDiv = document.getElementById("timelineResults");

    if (items.length === 0) {
        resultsDiv.innerHTML = '<div class="loading">No items found</div>';
        return;
    }

    resultsDiv.innerHTML = items.map(item => `
        <div class="timeline-item">
            <div class="info">
                <h4>${escapeHtml(item.summary || "No summary")}</h4>
                <p>${item.timestamp || ""} &bull; Wing: ${item.wing || "—"}</p>
            </div>
            <span class="badge badge-${item.type || "unknown"}">${item.type || "unknown"}</span>
        </div>
    `).join("");
}

function escapeHtml(text) {
    const div = document.createElement("div");
    div.textContent = text;
    return div.innerHTML;
}

function similarityClass(score) {
    if (score >= 0.4) return "sim-high";
    if (score >= 0.25) return "sim-mid";
    return "sim-low";
}

// ── SOUL identity panel ───────────────────────────────────────────────────────

function renderSoulStats(soul) {
    // Show agent count in stats grid
    const agentCard = document.getElementById("soulAgentCard");
    agentCard.style.display = "";
    document.getElementById("soulAgentCount").textContent = soul.agent_count || 0;

    // Show agents section
    const section = document.getElementById("soulSection");
    const container = document.getElementById("soulAgents");
    section.style.display = "";

    const agents = soul.agents || [];
    if (agents.length === 0) {
        container.innerHTML = '<div class="soul-empty">No agents captured yet. Use <code>soul_capture</code> to start building identity profiles.</div>';
        return;
    }

    container.innerHTML = agents.map(agent => renderSoulAgentCard(agent)).join("");
}

function renderSoulAgentCard(agent) {
    const conf = agent.confidence_score || 0;
    const confPct = (conf * 100).toFixed(1);
    const confWidth = Math.round(conf * 100);

    const driftClass = agent.drift_score > 0.6 ? "soul-drift-alert"
                     : agent.drift_score > 0.35 ? "soul-drift-warn"
                     : "soul-drift-ok";
    const driftLabel = agent.drift_score > 0.6 ? "DRIFT ALERT"
                     : agent.drift_score > 0.35 ? "DRIFT WARN"
                     : "STABLE";

    const lastCapture = agent.last_capture
        ? new Date(agent.last_capture).toLocaleString()
        : "never";

    return `
        <div class="soul-agent-card">
            <div class="agent-header">
                <span class="agent-id" title="${escapeHtml(agent.agent_id)}">${escapeHtml(agent.agent_id)}</span>
                <span class="agent-version">v${agent.version || 0}</span>
            </div>
            <div class="soul-confidence-bar" title="Confidence: ${confPct}%">
                <div class="soul-confidence-fill" style="width:${confWidth}%"></div>
            </div>
            <div class="soul-agent-meta">
                <span>Confidence: <strong>${confPct}%</strong></span>
                <span>Traits: <strong>${agent.trait_count || 0}</strong></span>
            </div>
            <div class="soul-agent-meta">
                <span>Last capture: ${escapeHtml(lastCapture)}</span>
            </div>
            ${agent.drift_score > 0 ? `<span class="soul-drift-badge ${driftClass}">${driftLabel} (${(agent.drift_score * 100).toFixed(0)}%)</span>` : ""}
        </div>
    `;
}

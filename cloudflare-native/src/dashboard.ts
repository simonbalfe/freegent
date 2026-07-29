const dashboardHtml = String.raw`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <title>Freegent Runs</title>
  <style>
    :root {
      color-scheme: dark;
      --bg: #0a0b0d;
      --panel: #111318;
      --panel-raised: #171a20;
      --line: #292d36;
      --text: #f3f5f7;
      --muted: #949ba8;
      --accent: #f6821f;
      --green: #46d78b;
      --red: #ff6b70;
      --blue: #65a8ff;
      --yellow: #f6c85f;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      background:
        radial-gradient(circle at 12% 0%, rgba(246,130,31,.12), transparent 28rem),
        var(--bg);
      color: var(--text);
      font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      min-height: 100vh;
    }
    button, input { font: inherit; }
    button { cursor: pointer; }
    .shell { max-width: 1320px; margin: 0 auto; padding: 28px; }
    header {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 20px;
      margin-bottom: 24px;
    }
    .brand { display: flex; align-items: center; gap: 12px; }
    .mark {
      display: grid;
      place-items: center;
      width: 38px;
      height: 38px;
      border-radius: 11px;
      background: var(--accent);
      color: #1b0b00;
      font-weight: 900;
    }
    h1, h2, h3, p { margin: 0; }
    h1 { font-size: 20px; letter-spacing: -.02em; }
    .eyebrow {
      color: var(--muted);
      font-size: 11px;
      font-weight: 700;
      letter-spacing: .13em;
      text-transform: uppercase;
      margin-bottom: 3px;
    }
    .auth { display: flex; gap: 8px; }
    input {
      width: min(320px, 45vw);
      border: 1px solid var(--line);
      border-radius: 9px;
      background: var(--panel);
      color: var(--text);
      padding: 9px 12px;
      outline: none;
    }
    input:focus { border-color: var(--accent); }
    .button {
      border: 1px solid var(--line);
      border-radius: 9px;
      background: var(--panel-raised);
      color: var(--text);
      padding: 9px 13px;
    }
    .button:hover { border-color: #454b58; }
    .button.primary { background: var(--accent); border-color: var(--accent); color: #1b0b00; font-weight: 700; }
    .layout {
      display: grid;
      grid-template-columns: minmax(300px, 390px) minmax(0, 1fr);
      gap: 18px;
      align-items: start;
    }
    .panel {
      border: 1px solid var(--line);
      border-radius: 14px;
      background: rgba(17,19,24,.92);
      overflow: hidden;
      box-shadow: 0 20px 60px rgba(0,0,0,.18);
    }
    .panel-head {
      min-height: 62px;
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 12px;
      padding: 15px 17px;
      border-bottom: 1px solid var(--line);
    }
    .panel-head h2 { font-size: 15px; }
    .muted { color: var(--muted); }
    .small { font-size: 12px; }
    .jobs { max-height: calc(100vh - 155px); overflow: auto; }
    .job {
      width: 100%;
      display: block;
      border: 0;
      border-bottom: 1px solid var(--line);
      background: transparent;
      color: inherit;
      padding: 15px 17px;
      text-align: left;
    }
    .job:hover, .job.active { background: var(--panel-raised); }
    .job.active { box-shadow: inset 3px 0 var(--accent); }
    .job-top, .job-meta, .detail-title, .run-head, .stats {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 12px;
    }
    .job-name {
      min-width: 0;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
      font-size: 14px;
      font-weight: 650;
    }
    .job-meta { margin-top: 9px; color: var(--muted); font-size: 12px; }
    .progress {
      height: 3px;
      background: #252933;
      border-radius: 99px;
      margin-top: 11px;
      overflow: hidden;
    }
    .progress span { display: block; height: 100%; background: var(--accent); }
    .pill {
      display: inline-flex;
      align-items: center;
      flex: 0 0 auto;
      border: 1px solid currentColor;
      border-radius: 99px;
      padding: 3px 7px;
      font-size: 10px;
      font-weight: 750;
      letter-spacing: .06em;
      text-transform: uppercase;
    }
    .completed { color: var(--green); }
    .failed { color: var(--red); }
    .partial { color: var(--yellow); }
    .running { color: var(--blue); }
    .queued, .skipped { color: var(--muted); }
    .detail { min-height: 480px; }
    .detail-body { padding: 18px; }
    .detail-title { align-items: flex-start; }
    .detail-title h2 { font-size: 18px; margin-bottom: 6px; }
    .stats {
      justify-content: flex-start;
      flex-wrap: wrap;
      margin: 18px 0;
    }
    .stat {
      min-width: 110px;
      border: 1px solid var(--line);
      border-radius: 10px;
      background: var(--panel-raised);
      padding: 11px 12px;
    }
    .stat strong { display: block; font-size: 19px; margin-bottom: 2px; }
    .runs { display: grid; gap: 10px; }
    .run { border: 1px solid var(--line); border-radius: 11px; overflow: hidden; }
    .run-head {
      width: 100%;
      border: 0;
      background: var(--panel-raised);
      color: inherit;
      padding: 12px 14px;
      text-align: left;
    }
    .run-body { display: none; padding: 14px; border-top: 1px solid var(--line); }
    .run.open .run-body { display: block; }
    .run-times { color: var(--muted); font-size: 12px; margin: 8px 0 12px; }
    pre {
      margin: 0;
      padding: 13px;
      border-radius: 9px;
      background: #090a0c;
      color: #dce2eb;
      font: 12px/1.6 ui-monospace, SFMono-Regular, Menlo, monospace;
      overflow: auto;
      white-space: pre-wrap;
      word-break: break-word;
    }
    .error { color: var(--red); margin-bottom: 10px; font-size: 13px; }
    .empty { padding: 64px 28px; text-align: center; color: var(--muted); }
    .empty strong { display: block; color: var(--text); font-size: 15px; margin-bottom: 7px; }
    .notice {
      display: none;
      border: 1px solid rgba(255,107,112,.4);
      border-radius: 10px;
      background: rgba(255,107,112,.08);
      color: #ffb2b5;
      padding: 10px 13px;
      margin-bottom: 16px;
      font-size: 13px;
    }
    .notice.visible { display: block; }
    @media (max-width: 820px) {
      .shell { padding: 18px; }
      header { align-items: flex-start; flex-direction: column; }
      .auth, .auth input { width: 100%; }
      .layout { grid-template-columns: 1fr; }
      .jobs { max-height: 390px; }
    }
  </style>
</head>
<body>
  <main class="shell">
    <header>
      <div class="brand">
        <div class="mark">F</div>
        <div>
          <div class="eyebrow">Cloudflare native</div>
          <h1>Freegent runs</h1>
        </div>
      </div>
      <form class="auth" id="auth-form">
        <input id="token" type="password" autocomplete="off" placeholder="API token" aria-label="API token">
        <button class="button primary" type="submit">Connect</button>
        <button class="button" id="clear-token" type="button">Clear</button>
      </form>
    </header>
    <div class="notice" id="notice"></div>
    <section class="layout">
      <aside class="panel">
        <div class="panel-head">
          <div>
            <div class="eyebrow">Latest 50</div>
            <h2>Jobs</h2>
          </div>
          <button class="button" id="refresh" type="button">Refresh</button>
        </div>
        <div class="jobs" id="jobs">
          <div class="empty"><strong>Connect to load jobs</strong>Enter the Worker API token above.</div>
        </div>
      </aside>
      <section class="panel detail" id="detail">
        <div class="empty"><strong>No job selected</strong>Choose a job to inspect its row runs.</div>
      </section>
    </section>
  </main>
  <script>
    const state = {
      token: sessionStorage.getItem("freegent-api-token") || "",
      jobs: [],
      selectedId: "",
      openRuns: new Set(),
      refreshing: false
    };
    const jobsNode = document.getElementById("jobs");
    const detailNode = document.getElementById("detail");
    const noticeNode = document.getElementById("notice");
    const tokenNode = document.getElementById("token");
    tokenNode.value = state.token;

    function node(tag, className, text) {
      const element = document.createElement(tag);
      if (className) element.className = className;
      if (text !== undefined) element.textContent = text;
      return element;
    }

    function statusPill(status) {
      return node("span", "pill " + status, status);
    }

    function formatTime(value) {
      if (!value) return "Not yet";
      return new Intl.DateTimeFormat(undefined, {
        dateStyle: "medium",
        timeStyle: "short"
      }).format(new Date(value));
    }

    function duration(start, finish) {
      if (!start) return "Not started";
      const end = finish ? new Date(finish).getTime() : Date.now();
      const seconds = Math.max(0, Math.round((end - new Date(start).getTime()) / 1000));
      if (seconds < 60) return seconds + "s";
      return Math.floor(seconds / 60) + "m " + (seconds % 60) + "s";
    }

    function showNotice(message) {
      noticeNode.textContent = message;
      noticeNode.classList.toggle("visible", Boolean(message));
    }

    async function api(path) {
      const response = await fetch(path, {
        headers: { Authorization: "Bearer " + state.token }
      });
      const payload = await response.json();
      if (!response.ok) {
        throw new Error(payload.error || "Request failed with status " + response.status);
      }
      return payload;
    }

    function renderJobs() {
      jobsNode.replaceChildren();
      if (!state.jobs.length) {
        jobsNode.append(node("div", "empty", "No jobs found."));
        return;
      }
      state.jobs.forEach(function (job) {
        const button = node("button", "job" + (job.id === state.selectedId ? " active" : ""));
        button.type = "button";
        const top = node("div", "job-top");
        top.append(node("span", "job-name", job.name), statusPill(job.status));
        const meta = node("div", "job-meta");
        meta.append(
          node("span", "", job.completed + "/" + job.total + " completed"),
          node("span", "", formatTime(job.createdAt))
        );
        const progress = node("div", "progress");
        const bar = node("span");
        bar.style.width = (job.total ? ((job.completed + job.failed) / job.total) * 100 : 0) + "%";
        progress.append(bar);
        button.append(top, meta, progress);
        button.addEventListener("click", function () {
          state.selectedId = job.id;
          state.openRuns.clear();
          renderJobs();
          loadJob(job.id);
        });
        jobsNode.append(button);
      });
    }

    function stat(label, value) {
      const element = node("div", "stat");
      element.append(node("strong", "", String(value)), node("span", "muted small", label));
      return element;
    }

    function renderJob(job) {
      detailNode.replaceChildren();
      const body = node("div", "detail-body");
      const title = node("div", "detail-title");
      const titleText = node("div");
      titleText.append(node("h2", "", job.name), node("p", "muted small", job.id));
      title.append(titleText, statusPill(job.status));
      const stats = node("div", "stats");
      stats.append(
        stat("Total rows", job.total),
        stat("Completed", job.completed),
        stat("Failed", job.failed),
        stat("Updated", formatTime(job.updatedAt))
      );
      const runs = node("div", "runs");
      job.runs.forEach(function (run) {
        const card = node("article", "run" + (state.openRuns.has(run.id) ? " open" : ""));
        const head = node("button", "run-head");
        head.type = "button";
        const label = node("div");
        label.append(
          node("strong", "", "Row " + (run.rowIndex + 1)),
          node("div", "muted small", duration(run.startedAt, run.finishedAt))
        );
        head.append(label, statusPill(run.status));
        const runBody = node("div", "run-body");
        runBody.append(node(
          "div",
          "run-times",
          "Started " + formatTime(run.startedAt) + " · Finished " + formatTime(run.finishedAt)
        ));
        if (run.error) runBody.append(node("div", "error", run.error));
        const output = node("pre", "", run.result ? JSON.stringify(run.result, null, 2) : "No result yet.");
        runBody.append(output);
        head.addEventListener("click", function () {
          card.classList.toggle("open");
          if (card.classList.contains("open")) {
            state.openRuns.add(run.id);
          } else {
            state.openRuns.delete(run.id);
          }
        });
        card.append(head, runBody);
        runs.append(card);
      });
      body.append(title, stats, runs);
      detailNode.append(body);
    }

    async function loadJob(id) {
      try {
        const job = await api("/jobs/" + encodeURIComponent(id));
        renderJob(job);
        showNotice("");
      } catch (error) {
        showNotice(error instanceof Error ? error.message : String(error));
      }
    }

    async function loadJobs(showErrors) {
      if (!state.token || state.refreshing) return;
      state.refreshing = true;
      try {
        const payload = await api("/jobs?limit=50");
        state.jobs = payload.jobs;
        if (!state.selectedId && state.jobs[0]) state.selectedId = state.jobs[0].id;
        renderJobs();
        if (state.selectedId) await loadJob(state.selectedId);
        showNotice("");
      } catch (error) {
        if (showErrors) showNotice(error instanceof Error ? error.message : String(error));
      } finally {
        state.refreshing = false;
      }
    }

    document.getElementById("auth-form").addEventListener("submit", function (event) {
      event.preventDefault();
      state.token = tokenNode.value.trim();
      sessionStorage.setItem("freegent-api-token", state.token);
      loadJobs(true);
    });
    document.getElementById("clear-token").addEventListener("click", function () {
      state.token = "";
      state.jobs = [];
      state.selectedId = "";
      state.openRuns.clear();
      tokenNode.value = "";
      sessionStorage.removeItem("freegent-api-token");
      showNotice("");
      jobsNode.innerHTML = '<div class="empty"><strong>Connect to load jobs</strong>Enter the Worker API token above.</div>';
      detailNode.innerHTML = '<div class="empty"><strong>No job selected</strong>Choose a job to inspect its row runs.</div>';
    });
    document.getElementById("refresh").addEventListener("click", function () {
      loadJobs(true);
    });
    if (state.token) loadJobs(true);
    setInterval(function () {
      if (state.token && state.jobs.some(function (job) {
        return job.status === "queued" || job.status === "running";
      })) loadJobs(false);
    }, 3000);
  </script>
</body>
</html>`;

export function dashboardResponse(): Response {
  return new Response(dashboardHtml, {
    headers: {
      "Cache-Control": "no-store",
      "Content-Security-Policy":
        "default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'",
      "Content-Type": "text/html; charset=utf-8",
      "Referrer-Policy": "no-referrer",
      "X-Content-Type-Options": "nosniff",
    },
  });
}

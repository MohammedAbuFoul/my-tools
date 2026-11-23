(() => {
  const api = () => {
    if (!window.go || !window.go.main || !window.go.main.App) {
      throw new Error("Wails backend is not available");
    }
    return window.go.main.App;
  };

  const state = {
    tasks: [],
    stats: { total: 0, todo_count: 0, in_progress_count: 0, done_count: 0, scanned_count: 0 },
    categories: [],
    history: [],
    activeView: "all", // all, todo, in_progress, done, high, scanned, scanner, cat:<name>
    search: "",
    priorityFilter: "",
    sortBy: "smart",
    editingTaskId: null,
    scanning: false,
  };

  const els = {
    navItems: () => document.querySelectorAll(".nav-item"),
    categoryList: document.getElementById("category-list"),
    viewTitle: document.getElementById("view-title"),
    viewSubtitle: document.getElementById("view-subtitle"),
    searchInput: document.getElementById("search-input"),
    searchWrap: document.getElementById("search-box-wrap"),
    priorityFilter: document.getElementById("priority-filter"),
    sortSelect: document.getElementById("sort-select"),
    themeToggle: document.getElementById("theme-toggle"),
    newTaskBtn: document.getElementById("new-task-btn"),

    tasksView: document.getElementById("tasks-view"),
    scannerView: document.getElementById("scanner-view"),
    taskList: document.getElementById("task-list"),
    completedToolbar: document.getElementById("completed-toolbar"),
    clearCompletedBtn: document.getElementById("clear-completed-btn"),

    quickAddForm: document.getElementById("quick-add-form"),
    quickTitle: document.getElementById("quick-title"),
    quickPriority: document.getElementById("quick-priority"),
    quickCategory: document.getElementById("quick-category"),
    quickDue: document.getElementById("quick-due"),

    countAll: document.getElementById("count-all"),
    countTodo: document.getElementById("count-todo"),
    countInProgress: document.getElementById("count-in_progress"),
    countDone: document.getElementById("count-done"),
    countHigh: document.getElementById("count-high"),
    countScanned: document.getElementById("count-scanned"),
    progressPercent: document.getElementById("progress-percent"),
    progressBar: document.getElementById("progress-bar"),
    dbPathLabel: document.getElementById("db-path-label"),

    // Scanner
    scanDir: document.getElementById("scan-dir"),
    browseDirBtn: document.getElementById("browse-dir-btn"),
    scanCategory: document.getElementById("scan-category"),
    scanPriority: document.getElementById("scan-priority"),
    scanExtensions: document.getElementById("scan-extensions"),
    scanIgnore: document.getElementById("scan-ignore"),
    previewScanBtn: document.getElementById("preview-scan-btn"),
    applyScanBtn: document.getElementById("apply-scan-btn"),
    scanResultsCard: document.getElementById("scan-results-card"),
    scanSummary: document.getElementById("scan-summary"),
    scanErrors: document.getElementById("scan-errors"),
    scanMatchesBody: document.getElementById("scan-matches-body"),
    scanHistoryList: document.getElementById("scan-history-list"),

    // Modal
    modal: document.getElementById("task-modal"),
    modalHeading: document.getElementById("modal-heading"),
    modalTaskCode: document.getElementById("modal-task-code"),
    modalCloseBtn: document.getElementById("modal-close-btn"),
    modalCancelBtn: document.getElementById("modal-cancel-btn"),
    modalDeleteBtn: document.getElementById("modal-delete-btn"),
    taskForm: document.getElementById("task-form"),
    modalTaskId: document.getElementById("modal-task-id"),
    modalTitle: document.getElementById("modal-title"),
    modalDescription: document.getElementById("modal-description"),
    modalStatus: document.getElementById("modal-status"),
    modalPriority: document.getElementById("modal-priority"),
    modalCategory: document.getElementById("modal-category"),
    modalDue: document.getElementById("modal-due"),
    modalSourceBox: document.getElementById("modal-source-box"),
    modalSourcePath: document.getElementById("modal-source-path"),
    modalSourceSnippet: document.getElementById("modal-source-snippet"),
    modalSubtasksSection: document.getElementById("modal-subtasks-section"),
    modalSubtasksList: document.getElementById("modal-subtasks-list"),
    newSubtaskInput: document.getElementById("new-subtask-input"),
    addSubtaskBtn: document.getElementById("add-subtask-btn"),

    toastContainer: document.getElementById("toast-container"),
  };

  function escapeHTML(str) {
    return String(str ?? "")
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;")
      .replace(/'/g, "&#39;");
  }

  function showToast(message, type = "info") {
    const el = document.createElement("div");
    el.className = `toast ${type}`;
    el.textContent = message;
    els.toastContainer.appendChild(el);
    setTimeout(() => {
      el.remove();
    }, 3200);
  }

  function parseCSV(val) {
    return val
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean);
  }

  function todayISO() {
    const d = new Date();
    const y = d.getFullYear();
    const m = String(d.getMonth() + 1).padStart(2, "0");
    const day = String(d.getDate()).padStart(2, "0");
    return `${y}-${m}-${day}`;
  }

  function shortPath(filePath) {
    if (!filePath) return "";
    const normalized = filePath.replace(/\\/g, "/");
    const parts = normalized.split("/");
    if (parts.length <= 3) return normalized;
    return ".../" + parts.slice(-3).join("/");
  }

  async function refreshAll() {
    try {
      const [tasks, stats, categories, history] = await Promise.all([
        api().ListTasks(),
        api().GetStats(),
        api().ListCategories(),
        api().ListScanHistory(),
      ]);
      state.tasks = tasks || [];
      state.stats = stats || state.stats;
      state.categories = categories || [];
      state.history = history || [];

      renderSidebar();
      renderView();
      renderScanHistory();
    } catch (err) {
      showToast(`Failed to load data: ${err}`, "error");
    }
  }

  function renderSidebar() {
    const highCount = state.tasks.filter(
      (t) => (t.priority === "high" || t.priority === "urgent") && t.status !== "done"
    ).length;

    els.countAll.textContent = state.stats.total;
    els.countTodo.textContent = state.stats.todo_count;
    els.countInProgress.textContent = state.stats.in_progress_count;
    els.countDone.textContent = state.stats.done_count;
    els.countHigh.textContent = highCount;
    els.countScanned.textContent = state.stats.scanned_count;

    const pct = state.stats.total > 0
      ? Math.round((state.stats.done_count / state.stats.total) * 100)
      : 0;
    els.progressPercent.textContent = `${pct}%`;
    els.progressBar.style.width = `${pct}%`;

    // Categories
    const catCounts = {};
    for (const t of state.tasks) {
      const c = t.category || "General";
      catCounts[c] = (catCounts[c] || 0) + 1;
    }

    els.categoryList.innerHTML = state.categories
      .map((cat) => {
        const viewKey = `cat:${cat}`;
        const active = state.activeView === viewKey ? "active" : "";
        return `
          <button class="nav-item ${active}" data-view="${escapeHTML(viewKey)}">
            <span class="nav-icon">#</span>
            <span class="nav-text">${escapeHTML(cat)}</span>
            <span class="nav-badge">${catCounts[cat] || 0}</span>
          </button>
        `;
      })
      .join("");

    els.navItems().forEach((btn) => {
      btn.classList.toggle("active", btn.dataset.view === state.activeView);
    });
  }

  function getFilteredTasks() {
    let list = [...state.tasks];

    if (state.activeView === "todo") {
      list = list.filter((t) => t.status === "todo");
    } else if (state.activeView === "in_progress") {
      list = list.filter((t) => t.status === "in_progress");
    } else if (state.activeView === "done") {
      list = list.filter((t) => t.status === "done");
    } else if (state.activeView === "high") {
      list = list.filter((t) => (t.priority === "high" || t.priority === "urgent") && t.status !== "done");
    } else if (state.activeView === "scanned") {
      list = list.filter((t) => Boolean(t.source_file));
    } else if (state.activeView.startsWith("cat:")) {
      const cat = state.activeView.slice(4);
      list = list.filter((t) => (t.category || "General") === cat);
    }

    if (state.priorityFilter) {
      list = list.filter((t) => t.priority === state.priorityFilter);
    }

    const q = state.search.trim().toLowerCase();
    if (q) {
      list = list.filter(
        (t) =>
          t.title.toLowerCase().includes(q) ||
          t.task_code.toLowerCase().includes(q) ||
          (t.description && t.description.toLowerCase().includes(q)) ||
          (t.category && t.category.toLowerCase().includes(q)) ||
          (t.source_file && t.source_file.toLowerCase().includes(q))
      );
    }

    const prioRank = { urgent: 1, high: 2, medium: 3, low: 4 };
    const statusRank = { in_progress: 1, todo: 2, done: 3 };

    list.sort((a, b) => {
      switch (state.sortBy) {
        case "newest":
          return b.id - a.id;
        case "code":
          return a.task_number - b.task_number;
        case "due": {
          const ad = a.due_date || "9999-12-31";
          const bd = b.due_date || "9999-12-31";
          return ad.localeCompare(bd);
        }
        case "priority":
          return (prioRank[a.priority] || 9) - (prioRank[b.priority] || 9) || b.id - a.id;
        default: // smart
          return (
            (statusRank[a.status] || 9) - (statusRank[b.status] || 9) ||
            (prioRank[a.priority] || 9) - (prioRank[b.priority] || 9) ||
            b.id - a.id
          );
      }
    });

    return list;
  }

  function renderView() {
    const isScanner = state.activeView === "scanner";
    els.tasksView.style.display = isScanner ? "none" : "flex";
    els.scannerView.style.display = isScanner ? "flex" : "none";
    els.searchWrap.style.display = isScanner ? "none" : "flex";
    els.priorityFilter.style.display = isScanner ? "none" : "inline-block";
    els.sortSelect.style.display = isScanner ? "none" : "inline-block";

    const titles = {
      all: "All Tasks",
      todo: "To Do",
      in_progress: "In Progress",
      done: "Completed Tasks",
      high: "High Priority",
      scanned: "Scanned from Code",
      scanner: "Code TODO Scanner",
    };

    if (state.activeView.startsWith("cat:")) {
      els.viewTitle.textContent = `# ${state.activeView.slice(4)}`;
    } else {
      els.viewTitle.textContent = titles[state.activeView] || "Tasks";
    }

    if (isScanner) {
      els.viewSubtitle.textContent = "Find TODO comments and replace them with TASK (XXX)";
      return;
    }

    const tasks = getFilteredTasks();
    els.viewSubtitle.textContent = `${tasks.length} task${tasks.length === 1 ? "" : "s"}`;
    els.completedToolbar.style.display = state.activeView === "done" && tasks.length > 0 ? "flex" : "none";

    if (tasks.length === 0) {
      els.taskList.innerHTML = `
        <div class="empty-state">
          <h3>No tasks here yet</h3>
          <p>Add a task above, press <strong>+ New Task</strong>, or run the <strong>Code TODO Scanner</strong> to import TODOs from your codebase.</p>
        </div>
      `;
      return;
    }

    const today = todayISO();
    els.taskList.innerHTML = tasks
      .map((t) => {
        const isDone = t.status === "done";
        const isInProgress = t.status === "in_progress";
        const checkSymbol = isDone ? "&#10003;" : isInProgress ? "&#9679;" : "";
        const overdue = t.due_date && !isDone && t.due_date < today;
        const subs = t.subtasks || [];
        const subsDone = subs.filter((s) => s.completed).length;

        return `
          <div class="task-card ${isDone ? "is-done" : ""}" data-id="${t.id}">
            <button class="task-check status-${t.status}" data-action="toggle-status" title="Click to mark ${isDone ? "To Do" : "Done"}">
              ${checkSymbol}
            </button>
            <div class="task-body" data-action="open-edit">
              <div class="task-header-row">
                <span class="task-code-pill" data-action="copy-code" data-code="${escapeHTML(t.task_code)}" title="Click to copy ${escapeHTML(t.task_code)}">${escapeHTML(t.task_code)}</span>
                <span class="task-title">${escapeHTML(t.title)}</span>
              </div>
              ${t.description ? `<div class="task-desc">${escapeHTML(t.description)}</div>` : ""}
              <div class="task-meta">
                <span class="pill pill-priority-${t.priority}">${escapeHTML(t.priority.toUpperCase())}</span>
                ${isInProgress ? `<span class="pill pill-status-in_progress">IN PROGRESS</span>` : ""}
                <span class="pill pill-category"># ${escapeHTML(t.category || "General")}</span>
                ${t.due_date ? `<span class="pill pill-due ${overdue ? "overdue" : ""}">Due ${escapeHTML(t.due_date)}</span>` : ""}
                ${subs.length > 0 ? `<span class="pill pill-category">&#9745; ${subsDone}/${subs.length}</span>` : ""}
                ${t.source_file ? `<span class="pill pill-source" title="${escapeHTML(t.source_file)}:${t.source_line}">&lt;/&gt; ${escapeHTML(shortPath(t.source_file))}:${t.source_line}</span>` : ""}
              </div>
            </div>
            <div class="task-actions">
              ${!isDone ? `<button class="icon-action" data-action="toggle-progress" title="${isInProgress ? "Move to To Do" : "Mark In Progress"}">${isInProgress ? "&#9208;" : "&#9654;"}</button>` : ""}
              <button class="icon-action" data-action="open-edit" title="Edit task">&#9998;</button>
              <button class="icon-action delete" data-action="delete-task" title="Delete task">&#128465;</button>
            </div>
          </div>
        `;
      })
      .join("");
  }

  function renderScanHistory() {
    if (!state.history || state.history.length === 0) {
      els.scanHistoryList.innerHTML = `<div class="muted-text">No codebase scans recorded yet.</div>`;
      return;
    }

    els.scanHistoryList.innerHTML = state.history
      .map((h) => {
        const dt = new Date(h.created_at).toLocaleString();
        return `
          <div class="history-item">
            <div class="history-dir" title="${escapeHTML(h.root_dir)}">${escapeHTML(h.root_dir)}</div>
            <div class="muted-text">
              ${h.files_scanned} files &middot; ${h.todos_replaced}/${h.todos_found} TODOs replaced &middot; ${escapeHTML(dt)}
            </div>
          </div>
        `;
      })
      .join("");
  }

  function renderScanResult(res) {
    els.scanResultsCard.style.display = "flex";
    const badge = res.dry_run
      ? `<span class="pill pill-status-in_progress">DRY RUN PREVIEW (No files modified)</span>`
      : `<span class="pill pill-status-done">APPLIED TO FILES &amp; SAVED TO DB</span>`;

    els.scanSummary.innerHTML = `
      <div>${badge}</div>
      <div class="scan-summary-stats">
        <span><strong>${res.files_scanned}</strong> files scanned</span>
        <span><strong>${res.todos_found}</strong> TODO(s) found</span>
        ${!res.dry_run ? `<span><strong>${res.todos_replaced}</strong> replaced across <strong>${res.files_modified}</strong> file(s)</span>` : ""}
      </div>
    `;

    if (res.errors && res.errors.length > 0) {
      els.scanErrors.style.display = "block";
      els.scanErrors.innerHTML = res.errors.map((e) => `<div>${escapeHTML(e)}</div>`).join("");
    } else {
      els.scanErrors.style.display = "none";
      els.scanErrors.innerHTML = "";
    }

    if (!res.matches || res.matches.length === 0) {
      els.scanMatchesBody.innerHTML = `
        <tr>
          <td colspan="3" class="muted-text" style="text-align:center; padding: 20px;">
            No <code>TODO</code> keywords found in the selected directory.
          </td>
        </tr>
      `;
      return;
    }

    els.scanMatchesBody.innerHTML = res.matches
      .map(
        (m) => `
          <tr>
            <td><span class="task-code-pill">${escapeHTML(m.task_code)}</span></td>
            <td><code>${escapeHTML(m.relative_path)}:${m.line_number}</code></td>
            <td>
              <span class="diff-old">- ${escapeHTML(m.original_line)}</span>
              <span class="diff-new">+ ${escapeHTML(m.replaced_line)}</span>
            </td>
          </tr>
        `
      )
      .join("");
  }

  // Modal helpers
  function openCreateModal() {
    state.editingTaskId = null;
    els.modalHeading.textContent = "New Task";
    els.modalTaskCode.style.display = "none";
    els.modalTaskId.value = "";
    els.modalTitle.value = "";
    els.modalDescription.value = "";
    els.modalStatus.value = "todo";
    els.modalPriority.value = "medium";
    els.modalCategory.value = state.activeView.startsWith("cat:") ? state.activeView.slice(4) : "General";
    els.modalDue.value = "";
    els.modalSourceBox.style.display = "none";
    els.modalSubtasksSection.style.display = "none";
    els.modalDeleteBtn.style.display = "none";
    els.modal.style.display = "flex";
    els.modalTitle.focus();
  }

  function openEditModal(task) {
    state.editingTaskId = task.id;
    els.modalHeading.textContent = "Edit Task";
    els.modalTaskCode.textContent = task.task_code;
    els.modalTaskCode.style.display = "inline-block";
    els.modalTaskId.value = String(task.id);
    els.modalTitle.value = task.title;
    els.modalDescription.value = task.description || "";
    els.modalStatus.value = task.status;
    els.modalPriority.value = task.priority;
    els.modalCategory.value = task.category || "General";
    els.modalDue.value = task.due_date || "";

    if (task.source_file) {
      els.modalSourceBox.style.display = "block";
      els.modalSourcePath.textContent = `${task.source_file}:${task.source_line}`;
      els.modalSourceSnippet.textContent = task.source_snippet || "";
    } else {
      els.modalSourceBox.style.display = "none";
    }

    els.modalSubtasksSection.style.display = "flex";
    renderModalSubtasks(task.subtasks || []);
    els.modalDeleteBtn.style.display = "inline-flex";
    els.modal.style.display = "flex";
    els.modalTitle.focus();
  }

  function renderModalSubtasks(subtasks) {
    if (!subtasks || subtasks.length === 0) {
      els.modalSubtasksList.innerHTML = `<div class="muted-text" style="font-size:12px;">No subtasks yet.</div>`;
      return;
    }
    els.modalSubtasksList.innerHTML = subtasks
      .map(
        (s) => `
          <div class="subtask-row ${s.completed ? "done" : ""}" data-subtask-id="${s.id}">
            <input type="checkbox" ${s.completed ? "checked" : ""} data-subtask-action="toggle" />
            <span>${escapeHTML(s.title)}</span>
            <button type="button" class="icon-action delete" data-subtask-action="delete" title="Remove">&times;</button>
          </div>
        `
      )
      .join("");
  }

  function closeModal() {
    els.modal.style.display = "none";
    state.editingTaskId = null;
  }

  // Event Listeners
  document.querySelector(".sidebar").addEventListener("click", (e) => {
    const btn = e.target.closest(".nav-item");
    if (!btn) return;
    state.activeView = btn.dataset.view;
    renderSidebar();
    renderView();
  });

  els.searchInput.addEventListener("input", (e) => {
    state.search = e.target.value;
    renderView();
  });

  els.priorityFilter.addEventListener("change", (e) => {
    state.priorityFilter = e.target.value;
    renderView();
  });

  els.sortSelect.addEventListener("change", (e) => {
    state.sortBy = e.target.value;
    renderView();
  });

  els.themeToggle.addEventListener("click", () => {
    const html = document.documentElement;
    const next = html.getAttribute("data-theme") === "dark" ? "light" : "dark";
    html.setAttribute("data-theme", next);
    localStorage.setItem("taskflow-theme", next);
  });

  els.newTaskBtn.addEventListener("click", openCreateModal);
  els.modalCloseBtn.addEventListener("click", closeModal);
  els.modalCancelBtn.addEventListener("click", closeModal);
  els.modal.addEventListener("click", (e) => {
    if (e.target === els.modal) closeModal();
  });

  // Quick Add
  els.quickAddForm.addEventListener("submit", async (e) => {
    e.preventDefault();
    const title = els.quickTitle.value.trim();
    if (!title) return;
    try {
      const created = await api().CreateTask({
        title,
        description: "",
        status: state.activeView === "in_progress" ? "in_progress" : "todo",
        priority: els.quickPriority.value,
        category: els.quickCategory.value.trim() || "General",
        due_date: els.quickDue.value,
        source_file: "",
        source_line: 0,
        source_snippet: "",
      });
      els.quickTitle.value = "";
      els.quickDue.value = "";
      showToast(`Created ${created.task_code}`, "success");
      await refreshAll();
    } catch (err) {
      showToast(`Error creating task: ${err}`, "error");
    }
  });

  // Task List Delegation
  els.taskList.addEventListener("click", async (e) => {
    const card = e.target.closest(".task-card");
    if (!card) return;
    const id = Number(card.dataset.id);
    const task = state.tasks.find((t) => t.id === id);
    if (!task) return;

    const actionEl = e.target.closest("[data-action]");
    if (!actionEl) return;
    const action = actionEl.dataset.action;

    if (action === "copy-code") {
      e.stopPropagation();
      try {
        await navigator.clipboard.writeText(task.task_code);
        showToast(`Copied ${task.task_code}`, "info");
      } catch {
        showToast(task.task_code, "info");
      }
      return;
    }

    if (action === "toggle-status") {
      e.stopPropagation();
      const nextStatus = task.status === "done" ? "todo" : "done";
      try {
        await api().UpdateTaskStatus(id, nextStatus);
        await refreshAll();
      } catch (err) {
        showToast(`Error updating status: ${err}`, "error");
      }
      return;
    }

    if (action === "toggle-progress") {
      e.stopPropagation();
      const nextStatus = task.status === "in_progress" ? "todo" : "in_progress";
      try {
        await api().UpdateTaskStatus(id, nextStatus);
        await refreshAll();
      } catch (err) {
        showToast(`Error updating status: ${err}`, "error");
      }
      return;
    }

    if (action === "delete-task") {
      e.stopPropagation();
      try {
        await api().DeleteTask(id);
        showToast(`Deleted ${task.task_code}`, "info");
        await refreshAll();
      } catch (err) {
        showToast(`Error deleting task: ${err}`, "error");
      }
      return;
    }

    if (action === "open-edit") {
      openEditModal(task);
    }
  });

  els.clearCompletedBtn.addEventListener("click", async () => {
    try {
      await api().DeleteCompletedTasks();
      showToast("Cleared all completed tasks", "info");
      await refreshAll();
    } catch (err) {
      showToast(`Error clearing tasks: ${err}`, "error");
    }
  });

  // Save Task Modal
  els.taskForm.addEventListener("submit", async (e) => {
    e.preventDefault();
    const title = els.modalTitle.value.trim();
    if (!title) return;

    try {
      if (state.editingTaskId) {
        await api().UpdateTask({
          id: state.editingTaskId,
          title,
          description: els.modalDescription.value,
          status: els.modalStatus.value,
          priority: els.modalPriority.value,
          category: els.modalCategory.value.trim() || "General",
          due_date: els.modalDue.value,
        });
        showToast("Task updated", "success");
      } else {
        const created = await api().CreateTask({
          title,
          description: els.modalDescription.value,
          status: els.modalStatus.value,
          priority: els.modalPriority.value,
          category: els.modalCategory.value.trim() || "General",
          due_date: els.modalDue.value,
          source_file: "",
          source_line: 0,
          source_snippet: "",
        });
        showToast(`Created ${created.task_code}`, "success");
      }
      closeModal();
      await refreshAll();
    } catch (err) {
      showToast(`Save failed: ${err}`, "error");
    }
  });

  els.modalDeleteBtn.addEventListener("click", async () => {
    if (!state.editingTaskId) return;
    try {
      await api().DeleteTask(state.editingTaskId);
      closeModal();
      showToast("Task deleted", "info");
      await refreshAll();
    } catch (err) {
      showToast(`Delete failed: ${err}`, "error");
    }
  });

  // Subtasks
  async function addSubtask() {
    if (!state.editingTaskId) return;
    const title = els.newSubtaskInput.value.trim();
    if (!title) return;
    try {
      await api().CreateSubtask(state.editingTaskId, title);
      els.newSubtaskInput.value = "";
      const updated = await api().GetTask(state.editingTaskId);
      renderModalSubtasks(updated.subtasks || []);
      await refreshAll();
    } catch (err) {
      showToast(`Error adding subtask: ${err}`, "error");
    }
  }

  els.addSubtaskBtn.addEventListener("click", addSubtask);
  els.newSubtaskInput.addEventListener("keydown", (e) => {
    if (e.key === "Enter") {
      e.preventDefault();
      addSubtask();
    }
  });

  els.modalSubtasksList.addEventListener("click", async (e) => {
    const row = e.target.closest(".subtask-row");
    if (!row || !state.editingTaskId) return;
    const subId = Number(row.dataset.subtaskId);
    const action = e.target.dataset.subtaskAction;
    if (!action) return;

    try {
      if (action === "toggle") {
        await api().ToggleSubtask(subId);
      } else if (action === "delete") {
        await api().DeleteSubtask(subId);
      }
      const updated = await api().GetTask(state.editingTaskId);
      renderModalSubtasks(updated.subtasks || []);
      await refreshAll();
    } catch (err) {
      showToast(`Subtask error: ${err}`, "error");
    }
  });

  // Scanner actions
  els.browseDirBtn.addEventListener("click", async () => {
    try {
      const dir = await api().SelectDirectory();
      if (dir) {
        els.scanDir.value = dir;
      }
    } catch (err) {
      showToast(`Directory picker error: ${err}`, "error");
    }
  });

  async function executeScan(dryRun) {
    const rootDir = els.scanDir.value.trim();
    if (!rootDir) {
      showToast("Please select or enter a folder path first", "error");
      return;
    }
    if (state.scanning) return;

    state.scanning = true;
    els.previewScanBtn.disabled = true;
    els.applyScanBtn.disabled = true;

    const opts = {
      root_dir: rootDir,
      extensions: parseCSV(els.scanExtensions.value),
      ignore_dirs: parseCSV(els.scanIgnore.value),
      category: els.scanCategory.value.trim(),
      priority: els.scanPriority.value,
    };

    try {
      const res = dryRun ? await api().PreviewScan(opts) : await api().ScanAndReplace(opts);
      renderScanResult(res);
      if (dryRun) {
        showToast(`Preview complete: found ${res.todos_found} TODO(s)`, "info");
      } else {
        showToast(`Replaced ${res.todos_replaced} TODO(s) and added to your To-Do list`, "success");
        await refreshAll();
      }
    } catch (err) {
      showToast(`Scan failed: ${err}`, "error");
    } finally {
      state.scanning = false;
      els.previewScanBtn.disabled = false;
      els.applyScanBtn.disabled = false;
    }
  }

  els.previewScanBtn.addEventListener("click", () => executeScan(true));
  els.applyScanBtn.addEventListener("click", () => executeScan(false));

  // Keyboard shortcuts
  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape" && els.modal.style.display === "flex") {
      closeModal();
      return;
    }
    const tag = (e.target.tagName || "").toLowerCase();
    if (tag === "input" || tag === "textarea" || tag === "select") return;

    if (e.key === "/") {
      e.preventDefault();
      if (state.activeView === "scanner") {
        state.activeView = "all";
        renderSidebar();
        renderView();
      }
      els.searchInput.focus();
    } else if (e.key.toLowerCase() === "n") {
      e.preventDefault();
      openCreateModal();
    }
  });

  // Init
  async function init() {
    const savedTheme = localStorage.getItem("taskflow-theme");
    if (savedTheme === "light" || savedTheme === "dark") {
      document.documentElement.setAttribute("data-theme", savedTheme);
    }

    try {
      const dbPath = await api().GetDBPath();
      els.dbPathLabel.textContent = dbPath;
      els.dbPathLabel.title = dbPath;
    } catch {
      // ignore if outside Wails
    }

    await refreshAll();

    try {
      const initialScan = await api().GetInitialScan();
      if (initialScan && initialScan.root_dir) {
        els.scanDir.value = initialScan.root_dir;
        renderScanResult(initialScan);
        showToast(
          `Scanned ${initialScan.root_dir}: replaced ${initialScan.todos_replaced} TODO(s)`,
          "success"
        );
      }
    } catch {
      // ignore if not launched via CLI scan
    }
  }

  window.addEventListener("DOMContentLoaded", init);
})();

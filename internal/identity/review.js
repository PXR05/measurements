const elements = Object.fromEntries(
  [...document.querySelectorAll("[id]")].map((element) => [element.id, element]),
);

const pageSize = 30;
let offset = 0;
let saving = false;
let hasDecisions = false;
let reviewRequest;

function createElement(tag, text, className) {
  const element = document.createElement(tag);
  if (text !== undefined) element.textContent = text;
  if (className) element.className = className;
  return element;
}

function createButton(text, onClick) {
  const button = createElement("button", text);
  button.type = "button";
  button.addEventListener("click", onClick);
  return button;
}

function debounce(callback) {
  let timer;
  const schedule = () => {
    clearTimeout(timer);
    timer = setTimeout(callback, 250);
  };
  schedule.cancel = () => clearTimeout(timer);
  return schedule;
}

function keyLabel(key) {
  return key.name ? `${key.brand} / ${key.name}` : key.brand;
}

function countLabel(count, noun) {
  return `${count.toLocaleString()} ${noun}${count === 1 ? "" : "s"}`;
}

function setStatus(message = "", error = false) {
  elements.status.textContent = message;
  elements.status.classList.toggle("error", error);
}

async function requestJSON(path, { body, signal } = {}) {
  const options = { signal };
  if (body !== undefined) {
    options.method = "POST";
    options.headers = { "Content-Type": "application/json", "X-IEM-Review": "1" };
    options.body = JSON.stringify(body);
  }
  const response = await fetch(path, options);
  const text = await response.text();
  let data;
  try {
    data = JSON.parse(text);
  } catch {
    throw new Error(text || response.statusText);
  }
  if (!response.ok) throw new Error(data.error || response.statusText);
  return data;
}

function renderProduct(product) {
  const section = createElement("section");
  const { brand, name } = product.key;
  if (name) section.append(createElement("small", brand));
  section.append(createElement("h2", name || brand));

  const details = createElement("details");
  const count = product.record_count || product.records.length;
  details.append(createElement("summary", countLabel(count, "record")));
  const records = createElement("ul");
  for (const record of product.records) {
    records.append(createElement("li", `${record.source} · ${record.brand} / ${record.name}`));
  }
  details.append(records);
  if (count > product.records.length) {
    details.append(createElement("small", `Showing ${product.records.length}`));
  }
  section.append(details);
  return section;
}

function saveDecision(kind, left, right) {
  const body = { scope: elements.scope.value, kind, left, right };
  return save("/api/decision", body, kind === "merge" ? "Merged." : "Dismissed.");
}

function renderMatch(match) {
  const article = createElement("article");
  article.append(createElement("div", `${Math.round(match.score * 100)}% match`, "score"));
  const pair = createElement("div", undefined, "pair");
  pair.append(renderProduct(match.left), renderProduct(match.right));
  const actions = createElement("div", undefined, "actions");
  actions.append(
    createButton("Keep left", () => saveDecision("merge", match.right.key, match.left.key)),
    createButton("Keep right", () => saveDecision("merge", match.left.key, match.right.key)),
    createButton("Different", () => saveDecision("dismiss", match.left.key, match.right.key)),
  );
  article.append(pair, actions);
  return article;
}

function renderReview(data) {
  const type = elements.scope.value === "brand" ? "brand" : "IEM";
  elements.count.textContent = `${countLabel(data.total, "pair")} · ${countLabel(data.products, type)}`;
  elements.matches.replaceChildren(...data.candidates.map(renderMatch));
  if (!data.candidates.length) elements.matches.append(createElement("p", "No matches."));
  elements.page.textContent = data.total
    ? `${offset + 1}–${Math.min(offset + pageSize, data.total)} of ${data.total}` : "";
  elements.previous.disabled = offset === 0;
  elements.next.disabled = offset + pageSize >= data.total;
  hasDecisions = data.decisions > 0;
  elements.undo.disabled = saving || !hasDecisions;

  elements.history.replaceChildren();
  for (const decision of [...data.history].reverse()) {
    const type = decision.scope === "brand" ? "Brand" : "IEM";
    const operation = decision.kind === "merge" ? "→" : "≠";
    elements.history.append(createElement("li",
      `${type}: ${keyLabel(decision.left)} ${operation} ${keyLabel(decision.right)}`));
  }
}

async function loadReview(message = "") {
  reviewRequest?.abort();
  const controller = new AbortController();
  reviewRequest = controller;
  elements.matches.inert = true;
  elements.matches.setAttribute("aria-busy", "true");
  setStatus("Loading…");
  try {
    const query = new URLSearchParams({
      scope: elements.scope.value,
      q: elements.search.value,
      threshold: elements.threshold.value,
      offset,
    });
    const data = await requestJSON(`/api/review?${query}`, { signal: controller.signal });
    if (controller.signal.aborted) return;
    if (offset > 0 && offset >= data.total) {
      offset = Math.max(0, Math.floor((data.total - 1) / pageSize) * pageSize);
      return loadReview(message);
    }
    renderReview(data);
    setStatus(message);
  } catch (error) {
    if (error.name !== "AbortError") {
      elements.matches.replaceChildren();
      setStatus(error.message, true);
    }
  } finally {
    if (reviewRequest === controller) {
      elements.matches.inert = false;
      elements.matches.removeAttribute("aria-busy");
    }
  }
}

async function save(path, body, message) {
  if (saving) return;
  saving = true;
  reviewRequest?.abort();
  scheduleReview.cancel();
  for (const field of manual) {
    field.request?.abort();
    field.search.cancel();
  }
  elements.review.disabled = true;
  elements.refresh.disabled = true;
  elements.undo.disabled = true;
  setStatus("Saving…");
  try {
    await requestJSON(path, { body });
    resetManual();
    await loadReview(message);
  } catch (error) {
    setStatus(error.message, true);
  } finally {
    saving = false;
    elements.review.disabled = false;
    elements.refresh.disabled = false;
    elements.undo.disabled = !hasDecisions;
  }
}

const manual = ["merge", "keep"].map((side) => {
  const field = {
    input: elements[`${side}-search`],
    results: elements[`${side}-results`],
    selected: elements[`${side}-selected`],
    product: null,
    request: null,
  };
  field.search = debounce(() => searchManual(field));
  field.input.addEventListener("input", () => {
    clearManual(field);
    field.search();
  });
  return field;
});

function clearManual(field) {
  field.request?.abort();
  field.search.cancel();
  field.product = null;
  field.results.replaceChildren();
  field.selected.textContent = "";
  elements["manual-merge"].disabled = true;
}

function resetManual() {
  for (const field of manual) {
    clearManual(field);
    field.input.value = "";
  }
}

async function searchManual(field) {
  const q = field.input.value.trim();
  if (!q) return;
  const controller = new AbortController();
  field.request = controller;
  try {
    const query = new URLSearchParams({ mode: "products", scope: elements.scope.value, q });
    const products = await requestJSON(`/api/review?${query}`, { signal: controller.signal });
    if (controller.signal.aborted) return;
    const buttons = products.map((product) => createButton(keyLabel(product.key), () => {
      field.product = product;
      field.selected.textContent = keyLabel(product.key);
      field.results.replaceChildren();
      const [left, right] = manual.map((field) => field.product?.key);
      elements["manual-merge"].disabled = !left || !right
        || (left.brand === right.brand && left.name === right.name);
    }));
    field.results.replaceChildren(...buttons);
    if (!products.length) field.results.append(createElement("p", "No matches."));
  } catch (error) {
    if (error.name !== "AbortError") setStatus(error.message, true);
  }
}

function resetPage() {
  offset = 0;
  return loadReview();
}

elements.refresh.addEventListener("click", () => resetPage());
elements.scope.addEventListener("change", () => {
  resetManual();
  resetPage();
});
elements.threshold.addEventListener("change", () => resetPage());
const scheduleReview = debounce(resetPage);
elements.search.addEventListener("input", scheduleReview);
elements.previous.addEventListener("click", () => {
  offset = Math.max(0, offset - pageSize);
  loadReview();
});
elements.next.addEventListener("click", () => {
  offset += pageSize;
  loadReview();
});
elements.undo.addEventListener("click", () => save("/api/undo", {}, "Undone."));
elements["manual-merge"].addEventListener("click", () => {
  const [left, right] = manual.map((field) => field.product);
  if (left && right) saveDecision("merge", left.key, right.key);
});

loadReview();

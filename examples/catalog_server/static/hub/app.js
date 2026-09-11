const catalogView = document.querySelector("#catalog-view");
const detailView = document.querySelector("#detail-view");
const productGrid = document.querySelector("#product-grid");
const productTemplate = document.querySelector("#product-template");
const catalogTotal = document.querySelector("#catalog-total");
const searchInput = document.querySelector("#search");
const loadSentinel = document.querySelector("#load-sentinel");

const state = {
  products: [],
  total: 0,
  nextCursor: "",
  loading: false,
  initialized: false,
  bufferCheckScheduled: false,
  iconURLs: new Map()
};

const bufferedViewports = 2;
const maximumLimit = 200;

function initials(product) {
  const name = product.displayName || product.name || "Software";
  return name.split(/[\s-_]+/).filter(Boolean).slice(0, 2).map(part => part[0]).join("").toUpperCase();
}

function productKey(product) {
  return `${product.catalogName}/${product.name}`;
}

function problemMessage(problem, fallback) {
  return problem?.detail || problem?.title || fallback;
}

function renderMarkdown(markdown) {
  return DOMPurify.sanitize(marked.parse(markdown), {
    ALLOWED_TAGS: ["a", "blockquote", "br", "code", "del", "em", "h1", "h2", "h3", "h4", "h5", "h6", "hr", "img", "li", "ol", "p", "pre", "s", "strong", "table", "tbody", "td", "th", "thead", "tr", "ul"],
    ALLOWED_ATTR: ["alt", "colspan", "href", "rowspan", "src", "title"]
  });
}

async function fetchJSON(url) {
  const response = await fetch(url, { headers: { Accept: "application/json" } });
  let data;
  try {
    data = await response.json();
  } catch {
    throw new Error(`The catalog returned ${response.status} ${response.statusText}.`);
  }
  if (!response.ok) throw new Error(problemMessage(data, `The catalog returned ${response.status}.`));
  return data;
}

async function fetchCollection(url, field) {
  const items = [];
  let nextURL = url;
  while (nextURL) {
    const page = await fetchJSON(nextURL);
    items.push(...(page[field] || []));
    if (!page.nextCursor) break;
    const continuation = new URL(url, window.location.origin);
    continuation.searchParams.set("cursor", page.nextCursor);
    nextURL = continuation.pathname + continuation.search;
  }
  return items;
}

async function loadIcon(container, product, size = "card") {
  container.textContent = initials(product);
  if (!product.iconAvailable) return;
  const key = productKey(product);
  let objectURL = state.iconURLs.get(key);
  try {
    if (!objectURL) {
      const response = await fetch(`${product.links.self.href}/icon`);
      if (!response.ok) return;
      objectURL = URL.createObjectURL(await response.blob());
      state.iconURLs.set(key, objectURL);
    }
    const image = document.createElement("img");
    image.src = objectURL;
    image.alt = size === "detail" ? `${product.displayName || product.name} icon` : "";
    container.replaceChildren(image);
  } catch {
    // The initials remain as a useful fallback when optional icon content fails.
  }
}

function openProduct(product) {
  const url = new URL(window.location.href);
  url.searchParams.set("catalog", product.catalogName);
  url.searchParams.set("package", product.name);
  history.pushState({}, "", url);
  showRoute();
}

function productCard(product) {
  const card = productTemplate.content.firstElementChild.cloneNode(true);
  card.tabIndex = 0;
  card.setAttribute("role", "link");
  card.setAttribute("aria-label", `View ${product.displayName || product.name}`);
  card.querySelector(".catalog-pill").textContent = product.catalogName;
  card.querySelector(".provider").textContent = product.provider?.name || "Community";
  card.querySelector("h3").textContent = product.displayName || product.name;
  card.querySelector(".description").textContent = product.shortDescription || "Explore channels, versions, and release information for this software.";
  card.querySelector(".product-name").textContent = product.name;
  loadIcon(card.querySelector(".product-icon"), product);
  card.addEventListener("click", () => openProduct(product));
  card.addEventListener("keydown", event => {
    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      openProduct(product);
    }
  });
  return card;
}

function renderProducts() {
  const term = searchInput.value.trim().toLowerCase();
  const products = state.products.filter(product => [
    product.name,
    product.displayName,
    product.shortDescription,
    product.provider?.name,
    product.catalogName
  ].some(value => value?.toLowerCase().includes(term)));

  if (!products.length) {
    const message = state.products.length ? "No software matches your search." : "This catalog does not contain any software.";
    productGrid.innerHTML = `<div class="empty-state"><p>${message}</p></div>`;
  } else {
    productGrid.replaceChildren(...products.map(productCard));
  }
  catalogTotal.textContent = `${state.total} total ${state.total === 1 ? "listing" : "listings"}`;
}

function gridMetrics() {
  const style = getComputedStyle(productGrid);
  const columns = style.gridTemplateColumns.split(/\s+/).filter(Boolean).length || 1;
  const cards = productGrid.querySelectorAll(".product-card");
  const rowGap = Number.parseFloat(style.rowGap) || 0;
  let rowHeight = 318 + rowGap;
  if (cards.length > columns) {
    rowHeight = cards[columns].getBoundingClientRect().top - cards[0].getBoundingClientRect().top;
  } else if (cards.length) {
    rowHeight = cards[0].getBoundingClientRect().height + rowGap;
  }
  return { columns, rowHeight };
}

function packagesNeeded() {
  if (catalogView.hidden) return 0;
  const { columns, rowHeight } = gridMetrics();
  const targetBottom = window.scrollY + window.innerHeight * (bufferedViewports + 1);
  const gridBottom = window.scrollY + productGrid.getBoundingClientRect().bottom;
  const shortfall = targetBottom - gridBottom;
  if (shortfall <= 0) return state.initialized ? 0 : columns;
  return Math.min(maximumLimit, Math.max(1, Math.ceil(shortfall / rowHeight) * columns));
}

function scheduleBufferCheck() {
  if (state.bufferCheckScheduled) return;
  state.bufferCheckScheduled = true;
  requestAnimationFrame(() => {
    state.bufferCheckScheduled = false;
    if (catalogView.hidden || state.loading) return;
    const limit = packagesNeeded();
    if (!limit || (state.initialized && !state.nextCursor)) return;
    loadProducts(state.initialized ? state.nextCursor : "", limit);
  });
}

async function loadProducts(cursor, limit) {
  if (state.loading) return;
  state.loading = true;
  loadSentinel.hidden = !state.products.length;
  if (!cursor) productGrid.innerHTML = '<div class="loading-state"><p>Reading the software catalog...</p></div>';
  try {
    const query = new URLSearchParams({ limit: String(limit) });
    if (cursor) query.set("cursor", cursor);
    const page = await fetchJSON(`/api/v1/packages?${query}`);
    state.products.push(...page.packages);
    state.total = page.total;
    state.nextCursor = page.nextCursor || "";
    state.initialized = true;
    renderProducts();
  } catch (error) {
    state.initialized = true;
    state.nextCursor = "";
    productGrid.innerHTML = `<div class="error-state"><p>${escapeHTML(error.message)}</p></div>`;
    catalogTotal.textContent = "Catalog unavailable";
  } finally {
    state.loading = false;
    loadSentinel.hidden = true;
    scheduleBufferCheck();
  }
}

function safeURL(value) {
  try {
    const url = new URL(value);
    return ["http:", "https:"].includes(url.protocol) ? url.href : "";
  } catch {
    return "";
  }
}

function detailMeta(product) {
  const maintainers = product.maintainers?.map(item => item.email ? `${item.name} <${item.email}>` : item.name).join(", ") || "Not specified";
  const providerURL = safeURL(product.provider?.url);
  const repositoryURL = safeURL(product.sourceRepository);
  return `
    <div class="aside-card">
      <h2>About</h2>
      <dl class="meta-list">
        <div><dt>Package</dt><dd>${escapeHTML(product.name)}</dd></div>
        <div><dt>Provider</dt><dd>${providerURL ? `<a href="${providerURL}" target="_blank" rel="noreferrer">${escapeHTML(product.provider.name)}</a>` : escapeHTML(product.provider?.name || "Not specified")}</dd></div>
        <div><dt>Maintainers</dt><dd>${escapeHTML(maintainers)}</dd></div>
        ${repositoryURL ? `<div><dt>Source</dt><dd><a href="${repositoryURL}" target="_blank" rel="noreferrer">Repository</a></dd></div>` : ""}
      </dl>
    </div>
    ${product.keywords?.length ? `<div class="aside-card"><h2>Keywords</h2><div class="tag-list">${product.keywords.map(keyword => `<span class="tag">${escapeHTML(keyword)}</span>`).join("")}</div></div>` : ""}
  `;
}

function escapeHTML(value = "") {
  const element = document.createElement("span");
  element.textContent = value;
  return element.innerHTML;
}

function bundleRows(bundles) {
  if (!bundles.length) return '<div class="empty-state"><p>No bundles are visible from this package.</p></div>';
  return `<div class="bundle-list">${bundles.map(bundle => {
    const uri = safeURL(bundle.uri);
    const release = bundle.release ? `-${escapeHTML(bundle.release)}` : "";
    return `<div class="bundle-row"><span class="bundle-id" title="${escapeHTML(bundle.id)}">${escapeHTML(bundle.id)}</span><span class="bundle-version">v${escapeHTML(bundle.version)}${release}</span>${uri ? `<a class="bundle-uri" href="${uri}" target="_blank" rel="noreferrer">Source -&gt;</a>` : '<span class="bundle-uri">No source URI</span>'}</div>`;
  }).join("")}</div>`;
}

function channelPills(channels) {
  if (!channels.length) return "<p class=\"long-description\">No channels are available for this package.</p>";
  return `<div class="channels">${channels.map(channel => `<span class="channel-pill" title="${escapeHTML(channel.path.join(" / "))}">${escapeHTML(channel.path.join(" / "))}</span>`).join("")}</div>`;
}

async function showDetail(catalog, packageName) {
  catalogView.hidden = true;
  detailView.hidden = false;
  detailView.innerHTML = '<div class="loading-state"><p>Assembling software details...</p></div>';
  window.scrollTo({ top: 0 });
  try {
    const self = `/api/v1/catalogs/${encodeURIComponent(catalog)}/packages/${encodeURIComponent(packageName)}`;
    const product = await fetchJSON(self);
    const [channels, bundles] = await Promise.all([
      fetchCollection(product.links.channels.href, "channels"),
      fetchCollection(product.links.bundles.href, "bundles")
    ]);
    document.title = `${product.displayName || product.name} - Software Hub`;
    detailView.innerHTML = `
      <button class="back-link" type="button" data-back>&lt;- Back to Software Hub</button>
      <div class="detail-hero">
        <div class="detail-icon"></div>
        <div>
          <p class="detail-provider">${escapeHTML(product.provider?.name || "Community")}</p>
          <h1>${escapeHTML(product.displayName || product.name)}</h1>
          <p class="detail-subtitle">${escapeHTML(product.shortDescription || "Software available from the catalog.")}</p>
        </div>
        <span class="catalog-pill detail-catalog">${escapeHTML(product.catalogName)}</span>
      </div>
      <div class="detail-layout">
        <div class="detail-main">
          ${product.deprecationMessage ? `<div class="deprecation"><strong>Deprecated:</strong> ${escapeHTML(product.deprecationMessage)}</div>` : ""}
          <section class="content-section">
            <p class="section-label">Overview</p>
            <h2>About this software</h2>
            <div class="long-description">${renderMarkdown(product.description || product.shortDescription || "No description is available.")}</div>
          </section>
          <section class="content-section">
            <p class="section-label">Update paths</p>
            <h2>${channels.length} ${channels.length === 1 ? "channel" : "channels"}</h2>
            ${channelPills(channels)}
          </section>
          <section class="content-section">
            <p class="section-label">Releases</p>
            <h2>${bundles.length} ${bundles.length === 1 ? "bundle" : "bundles"}</h2>
            ${bundleRows(bundles)}
          </section>
        </div>
        <aside class="detail-aside">${detailMeta(product)}</aside>
      </div>
    `;
    loadIcon(detailView.querySelector(".detail-icon"), product, "detail");
    detailView.querySelector("[data-back]").addEventListener("click", goHome);
  } catch (error) {
    detailView.innerHTML = `<button class="back-link" type="button" data-back>&lt;- Back to Software Hub</button><div class="error-state"><p>${escapeHTML(error.message)}</p></div>`;
    detailView.querySelector("[data-back]").addEventListener("click", goHome);
  }
  detailView.focus();
}

function goHome(event) {
  event?.preventDefault();
  const url = new URL(window.location.href);
  url.search = "";
  history.pushState({}, "", url);
  showRoute();
}

function showRoute() {
  const params = new URLSearchParams(window.location.search);
  const catalog = params.get("catalog");
  const packageName = params.get("package");
  if (catalog && packageName) {
    showDetail(catalog, packageName);
    return;
  }
  document.title = "Software Hub";
  detailView.hidden = true;
  catalogView.hidden = false;
  window.scrollTo({ top: 0 });
  scheduleBufferCheck();
}

document.querySelectorAll("[data-home]").forEach(link => link.addEventListener("click", goHome));
searchInput.addEventListener("input", () => {
  renderProducts();
  scheduleBufferCheck();
});
window.addEventListener("scroll", scheduleBufferCheck, { passive: true });
window.addEventListener("resize", scheduleBufferCheck);
window.addEventListener("popstate", showRoute);
window.addEventListener("beforeunload", () => state.iconURLs.forEach(url => URL.revokeObjectURL(url)));

showRoute();

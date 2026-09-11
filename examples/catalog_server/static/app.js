const sharedValues = {
  catalog: "catalog",
  package: "",
  channelPath: "",
  bundleID: "",
  catalogSelector: "",
  cursor: "",
  limit: "50",
  body: "{\n  \"upgradeConstraintPolicy\": \"CatalogProvided\"\n}"
};

const pathField = (name, label, placeholder) => ({ name, label, placeholder, required: true });
const collectionFields = (selector = false) => [
  ...(selector ? [{ name: "catalogSelector", label: "Catalog selector", optional: true, placeholder: "environment=production", wide: true }] : []),
  { name: "limit", label: "Limit", optional: true, type: "number", min: "1", max: "200" },
  { name: "cursor", label: "Cursor", optional: true, placeholder: "Paste or use next page", wide: true }
];

const endpoints = [
  {
    id: "list-catalogs", group: "Discover", method: "GET", title: "List catalogs",
    description: "Browse catalog summaries in deterministic name order. Optionally narrow the result with a Kubernetes label selector.",
    path: "/api/v1/catalogs", fields: collectionFields(true)
  },
  {
    id: "get-catalog", group: "Discover", method: "GET", title: "Get catalog",
    description: "Fetch metadata, labels, priority, and source information for one catalog.",
    path: "/api/v1/catalogs/{catalog}", fields: [pathField("catalog", "Catalog", "catalog")]
  },
  {
    id: "list-packages", group: "Discover", method: "GET", title: "List packages",
    description: "Discover every catalog-scoped package occurrence, ordered by package name and catalog priority.",
    path: "/api/v1/packages", fields: collectionFields(true)
  },
  {
    id: "get-package", group: "Browse", method: "GET", title: "Get package",
    description: "Read package presentation metadata and links to its channels, bundles, catalog, and icon.",
    path: "/api/v1/catalogs/{catalog}/packages/{package}",
    fields: [pathField("catalog", "Catalog", "catalog"), pathField("package", "Package", "my-operator")]
  },
  {
    id: "list-channels", group: "Browse", method: "GET", title: "List channels",
    description: "Recursively list every channel below a package, including nested channel paths.",
    path: "/api/v1/catalogs/{catalog}/packages/{package}/channels",
    fields: [pathField("catalog", "Catalog", "catalog"), pathField("package", "Package", "my-operator"), ...collectionFields()]
  },
  {
    id: "get-channel", group: "Browse", method: "GET", title: "Get channel",
    description: "Fetch one channel by its colon-delimited path, such as stable or stable:1:2.",
    path: "/api/v1/catalogs/{catalog}/packages/{package}/channels/{channelPath}",
    fields: [pathField("catalog", "Catalog", "catalog"), pathField("package", "Package", "my-operator"), pathField("channelPath", "Channel path", "stable")]
  },
  {
    id: "list-package-bundles", group: "Browse", method: "GET", title: "List package bundles",
    description: "List the union of bundles visible from the package root graph, newest versions first.",
    path: "/api/v1/catalogs/{catalog}/packages/{package}/bundles",
    fields: [pathField("catalog", "Catalog", "catalog"), pathField("package", "Package", "my-operator"), ...collectionFields()]
  },
  {
    id: "list-channel-bundles", group: "Browse", method: "GET", title: "List channel bundles",
    description: "List bundles visible from a selected channel graph and all of its descendants.",
    path: "/api/v1/catalogs/{catalog}/packages/{package}/channels/{channelPath}/bundles",
    fields: [pathField("catalog", "Catalog", "catalog"), pathField("package", "Package", "my-operator"), pathField("channelPath", "Channel path", "stable"), ...collectionFields()]
  },
  {
    id: "get-bundle", group: "Browse", method: "GET", title: "Get bundle",
    description: "Inspect identity, version, release, content URI, media type, and release timestamp for one bundle.",
    path: "/api/v1/catalogs/{catalog}/packages/{package}/bundles/{bundleID}",
    fields: [pathField("catalog", "Catalog", "catalog"), pathField("package", "Package", "my-operator"), pathField("bundleID", "Bundle ID", "my-operator.v1.0.0")]
  },
  {
    id: "get-icon", group: "Browse", method: "GET", title: "Get package icon",
    description: "Request the package icon as binary content. Successful responses can be previewed and downloaded.",
    path: "/api/v1/catalogs/{catalog}/packages/{package}/icon", binary: true,
    fields: [pathField("catalog", "Catalog", "catalog"), pathField("package", "Package", "my-operator")]
  },
  {
    id: "recommend", group: "Resolve", method: "POST", title: "Recommend bundles",
    description: "Resolve initial-install or upgrade candidates using optional channel, version, policy, and catalog constraints.",
    path: "/api/v1/recommendations/{package}",
    fields: [
      pathField("package", "Package", "my-operator"),
      { name: "catalogSelector", label: "Catalog selector", optional: true, placeholder: "environment=production", wide: true },
      ...collectionFields(),
      { name: "body", label: "JSON request body", type: "textarea", wide: true, required: true }
    ]
  }
];

const nav = document.querySelector("#endpoint-nav");
const form = document.querySelector("#request-form");
const fieldsRoot = document.querySelector("#request-fields");
const requestURL = document.querySelector("#request-url");
const sendButton = form.querySelector(".send-button");
const responseStatus = document.querySelector("#response-status");
const responseMeta = document.querySelector("#response-meta");
const responseBody = document.querySelector("#response-body");
const binaryResponse = document.querySelector("#binary-response");
const copyButton = document.querySelector("#copy-response");
const nextButton = document.querySelector("#next-page");

let activeEndpoint = endpoints[0];
let responseText = "";
let nextCursor = "";
let binaryURL = "";

function renderNav() {
  const groups = [...new Set(endpoints.map(endpoint => endpoint.group))];
  nav.replaceChildren(...groups.map(group => {
    const section = document.createElement("section");
    section.className = "nav-group";
    const heading = document.createElement("h3");
    heading.className = "nav-group-title";
    heading.textContent = group;
    section.append(heading);
    endpoints.filter(endpoint => endpoint.group === group).forEach(endpoint => {
      const button = document.createElement("button");
      button.type = "button";
      button.className = "nav-item";
      button.dataset.endpoint = endpoint.id;
      button.innerHTML = `<span class="nav-method">${endpoint.method}</span><span class="nav-label">${endpoint.title}</span>`;
      button.addEventListener("click", () => selectEndpoint(endpoint));
      section.append(button);
    });
    return section;
  }));
}

function selectEndpoint(endpoint) {
  activeEndpoint = endpoint;
  document.querySelectorAll(".nav-item").forEach(item => item.classList.toggle("active", item.dataset.endpoint === endpoint.id));
  document.querySelector("#operation-group").textContent = endpoint.group;
  document.querySelector("#operation-title").textContent = endpoint.title;
  document.querySelector("#operation-description").textContent = endpoint.description;
  const method = document.querySelector("#operation-method");
  method.textContent = endpoint.method;
  method.className = `method-badge ${endpoint.method.toLowerCase()}`;
  renderFields();
  updateRequestPreview();
}

function renderFields() {
  fieldsRoot.replaceChildren(...activeEndpoint.fields.map(field => {
    const wrapper = document.createElement("div");
    wrapper.className = `field${field.wide ? " wide" : ""}`;
    const label = document.createElement("label");
    label.htmlFor = `field-${field.name}`;
    label.append(document.createTextNode(field.label));
    const qualifier = document.createElement("span");
    qualifier.textContent = field.optional ? "Optional" : "Required";
    label.append(qualifier);

    const input = document.createElement(field.type === "textarea" ? "textarea" : "input");
    input.id = `field-${field.name}`;
    input.name = field.name;
    input.required = Boolean(field.required);
    input.placeholder = field.placeholder || "";
    input.value = sharedValues[field.name] || "";
    if (field.type && field.type !== "textarea") input.type = field.type;
    if (field.min) input.min = field.min;
    if (field.max) input.max = field.max;
    input.addEventListener("input", () => {
      sharedValues[field.name] = input.value;
      updateRequestPreview();
    });
    wrapper.append(label, input);
    return wrapper;
  }));
}

function buildRequestURL(cursorOverride) {
  let path = activeEndpoint.path.replaceAll(/\{([^}]+)\}/g, (_, name) => encodeURIComponent(sharedValues[name] || ""));
  const query = new URLSearchParams();
  for (const name of ["catalogSelector", "limit", "cursor"]) {
    const value = name === "cursor" && cursorOverride !== undefined ? cursorOverride : sharedValues[name];
    if (activeEndpoint.fields.some(field => field.name === name) && value) query.set(name, value);
  }
  const encoded = query.toString();
  return encoded ? `${path}?${encoded}` : path;
}

function updateRequestPreview() {
  requestURL.textContent = `${activeEndpoint.method} ${buildRequestURL()}`;
}

function setLoading(loading) {
  sendButton.disabled = loading;
  sendButton.firstElementChild.textContent = loading ? "Sending..." : "Send request";
  if (loading) {
    responseStatus.className = "status idle";
    responseStatus.textContent = "Waiting";
  }
}

async function sendRequest(cursorOverride) {
  if (!form.reportValidity()) return;
  setLoading(true);
  nextButton.hidden = true;
  copyButton.disabled = true;
  if (binaryURL) {
    URL.revokeObjectURL(binaryURL);
    binaryURL = "";
  }

  const url = buildRequestURL(cursorOverride);
  const options = { method: activeEndpoint.method, headers: { Accept: "application/json" } };
  if (activeEndpoint.method === "POST") {
    try {
      JSON.parse(sharedValues.body);
    } catch (error) {
      responseStatus.className = "status error";
      responseStatus.textContent = "Invalid JSON";
      responseMeta.textContent = error.message;
      setLoading(false);
      return;
    }
    options.headers["Content-Type"] = "application/json";
    options.body = sharedValues.body;
  }

  const started = performance.now();
  try {
    const response = await fetch(url, options);
    const duration = Math.round(performance.now() - started);
    const contentType = response.headers.get("content-type") || "unknown";
    responseStatus.className = `status ${response.ok ? "success" : "error"}`;
    responseStatus.textContent = `${response.status} ${response.statusText}`;
    responseMeta.textContent = `${activeEndpoint.method} ${url}  |  ${duration} ms  |  ${contentType}`;

    if (activeEndpoint.binary && response.ok) {
      const blob = await response.blob();
      binaryURL = URL.createObjectURL(blob);
      responseBody.hidden = true;
      binaryResponse.hidden = false;
      binaryResponse.innerHTML = `<div class="binary-card"><img src="${binaryURL}" alt="Package icon preview"><span>${blob.type || "binary data"} - ${blob.size.toLocaleString()} bytes</span><a href="${binaryURL}" download="package-icon">Download response</a></div>`;
      responseText = "";
    } else {
      responseText = await response.text();
      try {
        const parsed = JSON.parse(responseText);
        responseText = JSON.stringify(parsed, null, 2);
        nextCursor = parsed.nextCursor || "";
      } catch {
        nextCursor = "";
      }
      responseBody.hidden = false;
      binaryResponse.hidden = true;
      responseBody.firstElementChild.textContent = responseText || "(empty response)";
      copyButton.disabled = !responseText;
      nextButton.hidden = !nextCursor;
    }
  } catch (error) {
    responseStatus.className = "status error";
    responseStatus.textContent = "Request failed";
    responseMeta.textContent = error.message;
    responseBody.hidden = false;
    binaryResponse.hidden = true;
    responseBody.firstElementChild.textContent = JSON.stringify({ error: error.message }, null, 2);
  } finally {
    setLoading(false);
  }
}

form.addEventListener("submit", event => {
  event.preventDefault();
  sendRequest();
});

copyButton.addEventListener("click", async () => {
  await navigator.clipboard.writeText(responseText);
  copyButton.textContent = "Copied";
  setTimeout(() => { copyButton.textContent = "Copy JSON"; }, 1200);
});

nextButton.addEventListener("click", () => {
  sharedValues.cursor = nextCursor;
  const cursorInput = document.querySelector("#field-cursor");
  if (cursorInput) cursorInput.value = nextCursor;
  updateRequestPreview();
  sendRequest(nextCursor);
});

renderNav();
selectEndpoint(endpoints[0]);

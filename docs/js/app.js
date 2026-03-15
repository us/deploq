import config from "../site.config.js";
import { initTheme } from "./theme.js";
import { SearchEngine } from "./search.js";

// --- Initialize theme ---
initTheme();

// --- Initialize search ---
const searchEngine = new SearchEngine();

// --- Markdown parser ---
function parseMarkdown(md) {
  let html = md;

  // Code blocks (fenced) — must be first
  html = html.replace(/```(\w*)\n([\s\S]*?)```/g, (_, lang, code) => {
    const escaped = code.trim()
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;");
    return `<pre><code class="language-${lang}">${escaped}</code></pre>`;
  });

  // Inline code
  html = html.replace(/`([^`]+)`/g, "<code>$1</code>");

  // Headers
  html = html.replace(/^#### (.+)$/gm, "<h4>$1</h4>");
  html = html.replace(/^### (.+)$/gm, "<h3>$1</h3>");
  html = html.replace(/^## (.+)$/gm, "<h2>$1</h2>");
  html = html.replace(/^# (.+)$/gm, "<h1>$1</h1>");

  // Horizontal rule
  html = html.replace(/^---$/gm, "<hr>");

  // Bold + italic
  html = html.replace(/\*\*\*(.+?)\*\*\*/g, "<strong><em>$1</em></strong>");
  html = html.replace(/\*\*(.+?)\*\*/g, "<strong>$1</strong>");
  html = html.replace(/\*(.+?)\*/g, "<em>$1</em>");

  // Images
  html = html.replace(
    /!\[([^\]]*)\]\(([^)]+)\)/g,
    '<img src="$2" alt="$1">'
  );

  // Links
  html = html.replace(
    /\[([^\]]+)\]\(([^)]+)\)/g,
    '<a href="$2">$1</a>'
  );

  // Blockquote
  html = html.replace(/^&gt; (.+)$/gm, "<blockquote><p>$1</p></blockquote>");
  html = html.replace(/^> (.+)$/gm, "<blockquote><p>$1</p></blockquote>");

  // Unordered lists
  html = html.replace(/^(\s*)[-*] (.+)$/gm, "$1<li>$2</li>");
  html = html.replace(/((?:<li>.*<\/li>\n?)+)/g, "<ul>$1</ul>");

  // Ordered lists
  html = html.replace(/^\d+\. (.+)$/gm, "<li>$1</li>");

  // Tables
  html = html.replace(
    /^\|(.+)\|\s*\n\|[-| :]+\|\s*\n((?:\|.+\|\s*\n?)*)/gm,
    (_, headerRow, bodyRows) => {
      const headers = headerRow.split("|").map((c) => c.trim()).filter(Boolean);
      const rows = bodyRows
        .trim()
        .split("\n")
        .map((row) => row.split("|").map((c) => c.trim()).filter(Boolean));

      let table = "<table><thead><tr>";
      headers.forEach((h) => (table += `<th>${h}</th>`));
      table += "</tr></thead><tbody>";
      rows.forEach((row) => {
        table += "<tr>";
        row.forEach((cell) => (table += `<td>${cell}</td>`));
        table += "</tr>";
      });
      table += "</tbody></table>";
      return table;
    }
  );

  // Paragraphs — split on double newlines, wrap non-HTML blocks
  html = html
    .split("\n\n")
    .map((block) => {
      const trimmed = block.trim();
      if (!trimmed) return "";
      if (/^<[a-z]/.test(trimmed)) return trimmed;
      return `<p>${trimmed.replace(/\n/g, "<br>")}</p>`;
    })
    .join("\n");

  return html;
}

// --- Strip markdown for search indexing ---
function stripMarkdown(md) {
  return md
    .replace(/```[\s\S]*?```/g, "")
    .replace(/`[^`]+`/g, "")
    .replace(/[#*_\[\]()>|`-]/g, "")
    .replace(/\n+/g, " ")
    .trim();
}

// --- Routing ---
function getCurrentSlug() {
  return window.location.hash.slice(1) || config.defaultPage;
}

function getTitleForSlug(slug) {
  for (const section of config.sidebar) {
    const item = section.children.find((c) => c.slug === slug);
    if (item) return item.title;
  }
  return slug;
}

// --- Render sidebar ---
function renderSidebar() {
  const nav = document.getElementById("sidebar-nav");
  const currentSlug = getCurrentSlug();

  nav.innerHTML = config.sidebar
    .map((section) => {
      const isOpen = section.children.some((c) => c.slug === currentSlug);
      return `
        <div class="sidebar-section">
          <button class="sidebar-group-toggle ${isOpen ? "open" : ""}" data-section="${section.title}">
            ${section.title}
            <span class="chevron">&#9654;</span>
          </button>
          <div class="sidebar-group-children ${isOpen ? "open" : ""}">
            ${section.children
              .map(
                (child) => `
              <a href="#${child.slug}" class="sidebar-link ${child.slug === currentSlug ? "active" : ""}">${child.title}</a>
            `
              )
              .join("")}
          </div>
        </div>
      `;
    })
    .join("");

  // Toggle section expand/collapse
  nav.querySelectorAll(".sidebar-group-toggle").forEach((btn) => {
    btn.addEventListener("click", () => {
      btn.classList.toggle("open");
      btn.nextElementSibling.classList.toggle("open");
    });
  });

  // Close sidebar on mobile when clicking a link
  nav.querySelectorAll(".sidebar-link").forEach((link) => {
    link.addEventListener("click", () => {
      if (window.innerWidth <= 768) {
        closeMobileSidebar();
      }
    });
  });
}

// --- Load page ---
async function loadPage(slug) {
  const article = document.getElementById("article");

  try {
    const res = await fetch(`pages/${slug}.md`);
    if (!res.ok) throw new Error("Not found");
    const md = await res.text();
    article.innerHTML = parseMarkdown(md);
  } catch {
    article.innerHTML = `
      <h1>Page Not Found</h1>
      <p>The page <code>${slug}</code> could not be found.</p>
      <p><a href="#${config.defaultPage}">Go to ${getTitleForSlug(config.defaultPage)}</a></p>
    `;
  }

  document.title = `${getTitleForSlug(slug)} — ${config.name}`;
  renderSidebar();
  window.scrollTo(0, 0);
}

// --- Mobile sidebar ---
const hamburger = document.getElementById("hamburger");
const sidebar = document.getElementById("sidebar");
const overlay = document.getElementById("overlay");

function closeMobileSidebar() {
  sidebar.classList.remove("open");
  overlay.classList.remove("active");
  hamburger.classList.remove("active");
}

hamburger.addEventListener("click", () => {
  if (sidebar.classList.contains("open")) {
    closeMobileSidebar();
  } else {
    sidebar.classList.add("open");
    overlay.classList.add("active");
    hamburger.classList.add("active");
  }
});

overlay.addEventListener("click", closeMobileSidebar);

// --- Render navbar links ---
document.getElementById("navbar-links").innerHTML = config.navLinks
  .map((link) => {
    const attrs = link.external ? ' target="_blank" rel="noopener"' : "";
    return `<a href="${link.href}"${attrs}>${link.label}</a>`;
  })
  .join("");

// --- Render footer ---
document.getElementById("footer").innerHTML = `
  <span>${config.footer.left}</span>
  <span>${config.footer.right}</span>
`;

// --- Build search index ---
async function buildSearchIndex() {
  const pages = [];

  for (const section of config.sidebar) {
    for (const child of section.children) {
      try {
        const res = await fetch(`pages/${child.slug}.md`);
        if (!res.ok) continue;
        const md = await res.text();
        pages.push({
          title: child.title,
          slug: child.slug,
          content: stripMarkdown(md),
        });
      } catch {
        // skip
      }
    }
  }

  searchEngine.buildIndex(pages);
}

// --- Init ---
loadPage(getCurrentSlug());

window.addEventListener("hashchange", () => {
  loadPage(getCurrentSlug());
});

buildSearchIndex();

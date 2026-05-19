export type Lang = "id" | "en";

export const defaultLang: Lang = "id";

const rawSiteBasePath =
  (import.meta.env.PUBLIC_SITE_BASE_PATH || process.env.SITE_BASE_PATH || process.env.PUBLIC_SITE_BASE_PATH || "").trim();

const normalizedSiteBasePath = (() => {
  if (!rawSiteBasePath) {
    return "";
  }

  if (rawSiteBasePath === "/") {
    return "";
  }

  return rawSiteBasePath.startsWith("/") ? rawSiteBasePath.replace(/\/$/, "") : `/${rawSiteBasePath.replace(/\/$/, "")}`;
})();

function normalizePath(path: string) {
  return path.startsWith("/") ? path : `/${path}`;
}

export function withSiteBase(path: string) {
  const normalizedPath = normalizePath(path);

  if (!normalizedSiteBasePath) {
    return normalizedPath;
  }

  if (normalizedPath === "/") {
    return normalizedSiteBasePath;
  }

  return `${normalizedSiteBasePath}${normalizedPath}`;
}

export function stripSiteBase(path: string) {
  const normalizedPath = normalizePath(path);

  if (!normalizedSiteBasePath) {
    return normalizedPath === "/" ? "/" : normalizedPath;
  }

  if (normalizedPath === normalizedSiteBasePath) {
    return "/";
  }

  if (normalizedPath.startsWith(`${normalizedSiteBasePath}/`)) {
    return normalizedPath.slice(normalizedSiteBasePath.length);
  }

  return normalizedPath;
}

export function localizedPath(path: string, lang: Lang) {
  const normalizedPath = stripSiteBase(path);

  if (lang === defaultLang) {
    if (normalizedPath === "/" || normalizedPath === "") {
      return withSiteBase("/");
    }

    if (normalizedPath.startsWith("/en/") || normalizedPath === "/en") {
      return withSiteBase(normalizedPath.replace(/^\/en/, "") || "/");
    }

    return withSiteBase(normalizedPath);
  }

  if (normalizedPath === "/") {
    return withSiteBase("/en");
  }

  if (normalizedPath.startsWith("/en/")) {
    return withSiteBase(normalizedPath);
  }

  return withSiteBase(`/en${normalizedPath}`);
}

export const layoutCopy = {
  id: {
    nav: {
      home: "Beranda",
      docs: "Docs",
      skill: "Skill",
      blog: "Blog",
    },
    footer: "Rute Bayar adalah infrastruktur payment routing open source.",
    aria: "Navigasi utama",
    homeLabel: "Beranda Rute Bayar",
    switchLabel: "Pilih bahasa",
    providerStripLabel: "Penyedia yang didukung dan direncanakan",
    ogImageAlt: "Satu CLI untuk semua payment gateway Indonesia",
  },
  en: {
    nav: {
      home: "Home",
      docs: "Docs",
      skill: "Skill",
      blog: "Blog",
    },
    footer: "Rute Bayar is open source payment routing infrastructure.",
    aria: "Primary navigation",
    homeLabel: "Rute Bayar home",
    switchLabel: "Choose language",
    providerStripLabel: "Supported and planned providers",
    ogImageAlt: "One CLI for all Indonesian payment gateways",
  },
} as const;

export function langPrefix(lang: Lang) {
  return lang === defaultLang ? (normalizedSiteBasePath || "/") : withSiteBase("en");
}

export function stripLangFromId(id: string) {
  return id.replace(/^(id|en)\//, "");
}

export function entryLang(id: string): Lang {
  return id.startsWith("en/") ? "en" : "id";
}

export function docsPath(lang: Lang, id: string) {
  return `${langPrefix(lang)}/docs/${stripLangFromId(id)}/`;
}

export function blogPath(lang: Lang, id: string) {
  return `${langPrefix(lang)}/blog/${stripLangFromId(id)}/`;
}

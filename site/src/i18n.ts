export type Lang = "id" | "en";

export const defaultLang: Lang = "id";

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
  return lang === defaultLang ? "" : "/en";
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

import { defineConfig } from "astro/config";
import sitemap from "@astrojs/sitemap";

const site = process.env.SITE_URL ?? "https://rutebayar.id";
const base = process.env.SITE_BASE_PATH || process.env.PUBLIC_SITE_BASE_PATH || "";

export default defineConfig({
  site,
  base,
  output: "static",
  devToolbar: {
    enabled: false,
  },
  integrations: [sitemap()],
});

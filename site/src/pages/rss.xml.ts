import rss from "@astrojs/rss";
import { getCollection } from "astro:content";
import type { APIRoute } from "astro";
import { blogPath, entryLang } from "../i18n";

export const GET: APIRoute = async (context) => {
  const posts = (await getCollection("blog"))
    .filter((entry) => entryLang(entry.id) === "id")
    .sort((a, b) => b.data.pubDate.valueOf() - a.data.pubDate.valueOf());

  return rss({
    title: "Blog Rute Bayar",
    description: "Catatan engineering, update release, dan implementasi payment routing Rute Bayar.",
    site: context.site ?? "https://rutebayar.id",
    items: posts.map((post) => ({
      title: post.data.title,
      description: post.data.description,
      pubDate: post.data.pubDate,
      link: blogPath("id", post.id),
    })),
    customData: "<language>id-ID</language>",
  });
};

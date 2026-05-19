import rss from "@astrojs/rss";
import { getCollection } from "astro:content";
import type { APIRoute } from "astro";
import { blogPath, entryLang } from "../../i18n";

export const GET: APIRoute = async (context) => {
  const posts = (await getCollection("blog"))
    .filter((entry) => entryLang(entry.id) === "en")
    .sort((a, b) => b.data.pubDate.valueOf() - a.data.pubDate.valueOf());

  return rss({
    title: "Rute Bayar Blog",
    description: "Engineering notes, release updates, and implementation stories from the Rute Bayar project.",
    site: context.site ?? "https://rutebayar.id",
    items: posts.map((post) => ({
      title: post.data.title,
      description: post.data.description,
      pubDate: post.data.pubDate,
      link: blogPath("en", post.id),
    })),
    customData: "<language>en-US</language>",
  });
};

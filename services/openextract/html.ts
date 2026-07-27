import { Readability, isProbablyReaderable } from "@mozilla/readability";
import * as cheerio from "cheerio";
import type { AnyNode, Element } from "domhandler";
import { parseHTML } from "linkedom";
import TurndownService from "turndown";
import { gfm } from "turndown-plugin-gfm";
import { z } from "zod";

type Api = cheerio.CheerioAPI;
type LdNode = Record<string, unknown>;

const LdNodeSchema = z.record(z.string(), z.unknown());
const MIN_ARTICLE_CHARS = 250;
const STRUCTURED_CAP = 2500;
const EXCLUDED_TAGS = "nav,footer,header,aside,script,style,form,iframe,noscript,svg,link,meta";
const IMPORTANT_ATTRS = new Set(["src", "href", "alt", "title", "width", "height", "class", "id", "rowspan", "colspan"]);
const EMPTY_BYPASS_TAGS = new Set(["a", "img", "br", "hr", "input", "source", "track", "wbr", "tr", "td", "th"]);
const NEGATIVE_PATTERNS = /nav|footer|header|sidebar|ads|comment|promo|advert|social|share|cookie/i;
const PRUNE_THRESHOLD = 0.48;
const TAG_WEIGHTS: Record<string, number> = {
  div: 0.5,
  p: 1,
  article: 1.5,
  section: 1,
  span: 0.3,
  li: 0.5,
  ul: 0.5,
  ol: 0.5,
  h1: 1.2,
  h2: 1.1,
  h3: 1,
  h4: 0.9,
  h5: 0.8,
  h6: 0.7,
};

function isElement(node: AnyNode): node is Element {
  return node.type === "tag";
}

function textOf($: Api, node: AnyNode): string {
  return $(node).text().replace(/\s+/g, " ").trim();
}

function elementChildren(node: Element): Element[] {
  return node.children.filter(isElement);
}

function compositeScore($: Api, node: Element): number {
  const textLength = textOf($, node).length;
  const htmlLength = $(node).html()?.length || 1;
  const linkLength = $(node)
    .find("a")
    .toArray()
    .reduce((sum, link) => sum + textOf($, link).length, 0);
  const classPenalty = NEGATIVE_PATTERNS.test(node.attribs.class ?? "") ? -0.5 : 0;
  const idPenalty = NEGATIVE_PATTERNS.test(node.attribs.id ?? "") ? -0.5 : 0;
  const density = textLength / htmlLength;
  const linkDensity = 1 - (textLength ? linkLength / textLength : 0);
  const tagWeight = TAG_WEIGHTS[node.tagName] ?? 0.5;
  return 0.4 * density + 0.2 * linkDensity + 0.2 * tagWeight + 0.1 * (classPenalty + idPenalty) + 0.1 * Math.log(textLength + 1);
}

function sanitize($: Api): void {
  $("*")
    .contents()
    .each((_, node) => {
      if (node.type === "comment") $(node).remove();
    });
  $("*").each((_, node) => {
    if (!isElement(node)) return;
    for (const name of Object.keys(node.attribs)) {
      if (!IMPORTANT_ATTRS.has(name)) delete node.attribs[name];
    }
  });
  for (const node of $("*").toArray().reverse()) {
    if (!isElement(node) || EMPTY_BYPASS_TAGS.has(node.tagName)) continue;
    if ($(node).parents("pre,code").length) continue;
    if (!elementChildren(node).length && !textOf($, node)) $(node).remove();
  }
}

function pruneTree($: Api, node: Element): void {
  for (const child of elementChildren(node)) {
    if (compositeScore($, child) < PRUNE_THRESHOLD) $(child).remove();
    else pruneTree($, child);
  }
}

function promoteHeaderCells($: Api): void {
  $("table").each((_, table) => {
    const target = $(table);
    if (target.find("th").length) return;
    target
      .find("tr")
      .first()
      .children("td")
      .each((_, cell) => {
        cell.tagName = "th";
      });
  });
}

function converter(baseURL: string): TurndownService {
  const service = new TurndownService({ headingStyle: "atx", codeBlockStyle: "fenced", bulletListMarker: "-" });
  service.use(gfm);
  service.addRule("dropImages", { filter: "img", replacement: () => "" });
  service.addRule("sameDomainLinks", {
    filter: "a",
    replacement: (content, node) => {
      const text = content.trim();
      const href = (node as { getAttribute?: (name: string) => string | null }).getAttribute?.("href");
      if (!text || !href) return text;
      try {
        const absolute = new URL(href, baseURL);
        return absolute.host === new URL(baseURL).host ? `[${text}](${absolute.href})` : text;
      } catch {
        return text;
      }
    },
  });
  return service;
}

function tidy(markdown: string): string {
  return markdown.replace(/\n{3,}/g, "\n\n").trim();
}

function readabilityMarkdown(html: string, baseURL: string): string | null {
  try {
    const { document } = parseHTML(html);
    const readableDocument = document as unknown as ConstructorParameters<typeof Readability>[0];
    if (!isProbablyReaderable(readableDocument)) return null;
    const article = new Readability(readableDocument).parse();
    if (!article?.content) return null;
    const textLength = (article.textContent ?? "").replace(/\s+/g, " ").trim().length;
    if (textLength < MIN_ARTICLE_CHARS) return null;
    return tidy(converter(baseURL).turndown(article.content));
  } catch {
    return null;
  }
}

function pruneMarkdown(html: string, baseURL: string): string {
  const $ = cheerio.load(html);
  $(EXCLUDED_TAGS).remove();
  const body = $("body").get(0);
  if (!body || !isElement(body)) return "";
  sanitize($);
  pruneTree($, body);
  promoteHeaderCells($);
  return tidy(converter(baseURL).turndown($(body).html() ?? ""));
}

function ldNode(value: unknown): LdNode | null {
  const parsed = LdNodeSchema.safeParse(value);
  return parsed.success ? parsed.data : null;
}

function ldNodes(value: unknown): LdNode[] {
  return (Array.isArray(value) ? value : [value]).map(ldNode).filter((node): node is LdNode => node !== null);
}

function ldString(value: unknown): string | null {
  if (typeof value === "number") return String(value);
  if (typeof value !== "string") return null;
  return value.replace(/<[^>]*>/g, "").replace(/\s+/g, " ").trim() || null;
}

function collectLdNodes(value: unknown, output: LdNode[]): void {
  if (Array.isArray(value)) {
    for (const item of value) collectLdNodes(item, output);
    return;
  }
  const node = ldNode(value);
  if (!node) return;
  if ("@graph" in node) collectLdNodes(node["@graph"], output);
  if ("@type" in node) output.push(node);
}

function typesOf(node: LdNode): string[] {
  const type = node["@type"];
  if (typeof type === "string") return [type];
  return Array.isArray(type) ? type.filter((value): value is string => typeof value === "string") : [];
}

function renderAddress(value: unknown): string | null {
  if (typeof value === "string") return ldString(value);
  const address = ldNode(value);
  if (!address) return null;
  const countryValue = address.addressCountry;
  const country = ldString(ldNode(countryValue)?.name ?? countryValue);
  const parts = [address.streetAddress, address.addressLocality, address.addressRegion, address.postalCode]
    .map(ldString)
    .concat(country)
    .filter((part): part is string => Boolean(part));
  return parts.length ? parts.join(", ") : null;
}

function renderEmployees(value: unknown): string | null {
  if (typeof value === "number" || typeof value === "string") return ldString(value);
  const employees = ldNode(value);
  if (!employees) return null;
  const direct = ldString(employees.value);
  if (direct) return direct;
  const minimum = ldString(employees.minValue);
  const maximum = ldString(employees.maxValue);
  return minimum && maximum ? `${minimum}-${maximum}` : null;
}

function renderNode(node: LdNode): string[] {
  const types = typesOf(node);
  const matches = (pattern: RegExp) => types.some((type) => pattern.test(type));
  if (matches(/(^|:)(Organization|Corporation|LocalBusiness|NewsMediaOrganization)$/i)) {
    const facts = [
      ldString(node.name) && `name: ${ldString(node.name)}`,
      renderEmployees(node.numberOfEmployees) && `employees: ${renderEmployees(node.numberOfEmployees)}`,
      ldString(node.foundingDate) && `founded: ${ldString(node.foundingDate)}`,
      renderAddress(node.address) && `HQ: ${renderAddress(node.address)}`,
      ldString(node.url) && `url: ${ldString(node.url)}`,
    ].filter((fact): fact is string => Boolean(fact));
    return facts.length ? [`Organization: ${facts.join("; ")}`] : [];
  }
  if (matches(/FAQPage$/i)) {
    return ldNodes(node.mainEntity).flatMap((question) => {
      const title = ldString(question.name);
      const accepted = question.acceptedAnswer;
      const answer = ldString(ldNode(accepted)?.text ?? accepted);
      return title && answer ? [`FAQ: ${title} ${answer}`] : [];
    });
  }
  if (matches(/^Product$/i)) {
    const offer = ldNodes(node.offers)[0];
    const price = offer ? [ldString(offer.price), ldString(offer.priceCurrency)].filter(Boolean).join(" ") : null;
    const facts = [ldString(node.name) && `name: ${ldString(node.name)}`, price && `price: ${price}`].filter(
      (fact): fact is string => Boolean(fact),
    );
    return facts.length ? [`Product: ${facts.join("; ")}`] : [];
  }
  return [];
}

function structuredData(html: string): string {
  const $ = cheerio.load(html);
  const nodes: LdNode[] = [];
  $('script[type="application/ld+json"]').each((_, element) => {
    const raw = $(element).text().trim();
    if (!raw) return;
    try {
      const data: unknown = JSON.parse(raw);
      collectLdNodes(data, nodes);
    } catch {
      return;
    }
  });
  const lines = [...new Set(nodes.flatMap(renderNode))];
  const description = ldString($('meta[name="description"]').attr("content") ?? $('meta[property="og:description"]').attr("content"));
  if (description) lines.unshift(`Description: ${description}`);
  if (!lines.length) return "";
  const content = lines.join("\n");
  return `## Page structured data\n${content.length > STRUCTURED_CAP ? `${content.slice(0, STRUCTURED_CAP)}…` : content}`;
}

function sameSiteLinks(html: string, baseURL: string): string[] {
  const $ = cheerio.load(html);
  const host = new URL(baseURL).host;
  const links = new Set<string>();
  $("a[href]").each((_, element) => {
    const href = $(element).attr("href");
    if (!href) return;
    try {
      const absolute = new URL(href, baseURL);
      absolute.hash = "";
      if ((absolute.protocol === "http:" || absolute.protocol === "https:") && absolute.host === host) {
        links.add(absolute.href);
      }
    } catch {
      return;
    }
  });
  return [...links].slice(0, 100);
}

export function extractHTML(html: string, baseURL: string): { content: string; links: string[] } {
  if (!html.trim()) return { content: "", links: [] };
  const structured = structuredData(html);
  const body = readabilityMarkdown(html, baseURL) ?? pruneMarkdown(html, baseURL);
  return {
    content: structured ? (body ? `${structured}\n\n${body}` : structured) : body,
    links: sameSiteLinks(html, baseURL),
  };
}
